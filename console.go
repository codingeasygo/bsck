package bsck

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/proxy"
	"github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
)

//Console is node console to dial connection
type Console struct {
	Client        *xhttp.Client
	SlaverAddress string
	BufferSize    int
}

//NewConsole will return new Console
func NewConsole(slaverAddress string) (console *Console) {
	console = &Console{
		SlaverAddress: slaverAddress,
		BufferSize:    32 * 1024,
	}
	client := &http.Client{
		Transport: &http.Transport{
			Dial: console.dialNet,
		},
	}
	console.Client = xhttp.NewClient(client)
	return
}

func (c *Console) dialAll(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	conn, err := socks.DialType(c.SlaverAddress, 0x05, uri)
	if err != nil {
		if waiter, ok := raw.(ReadyWaiter); ok {
			waiter.Ready(err, nil)
		}
		raw.Close()
		return
	}
	proc := func(err error) {
		if err != nil {
			conn.Close()
			raw.Close()
			return
		}
		go xio.CopyBuffer(conn, raw, make([]byte, c.BufferSize))
		xio.CopyBuffer(raw, conn, make([]byte, c.BufferSize))
	}
	if waiter, ok := raw.(ReadyWaiter); ok {
		waiter.Ready(nil, proc)
	} else {
		go proc(nil)
	}
	return
}

//dialNet is net dialer to router
func (c *Console) dialNet(network, addr string) (conn net.Conn, err error) {
	addr = strings.TrimSuffix(addr, ":80")
	addr = strings.TrimSuffix(addr, ":443")
	if strings.HasPrefix(addr, "base64-") {
		var realAddr []byte
		realAddr, err = base64.RawURLEncoding.DecodeString(strings.TrimPrefix(addr, "base64-"))
		if err != nil {
			return
		}
		addr = string(realAddr)
	}
	conn, raw, err := dialer.CreatePipedConn()
	if err == nil {
		_, err = c.dialAll(addr, raw)
		if err != nil {
			conn.Close()
			raw.Close()
		}
	}
	return
}

//Redirect will redirect connection to reader/writer/closer
func (c *Console) Redirect(uri string, reader io.Reader, writer io.Writer, closer io.Closer) (err error) {
	raw := xio.NewCombinedReadWriteCloser(reader, writer, closer)
	_, err = c.dialAll(uri, raw)
	return
}

//Dial will dial connection by uri
func (c *Console) Dial(uri string) (conn io.ReadWriteCloser, err error) {
	conn, raw, err := dialer.CreatePipedConn()
	if err == nil {
		_, err = c.dialAll(uri, raw)
		if err != nil {
			conn.Close()
			raw.Close()
		}
	}
	return
}

//StartPing will start ping to uri
func (c *Console) StartPing(uri string, delay time.Duration) (closer func()) {
	if !strings.Contains(uri, "tcp://echo") {
		if len(uri) > 0 {
			uri += "->"
		}
		uri += "tcp://echo"
	}
	var err error
	var running bool = true
	var runc uint64
	var conn io.ReadWriteCloser
	buf := make([]byte, 64)
	waiter := sync.WaitGroup{}
	closer = func() {
		running = false
		if conn != nil {
			conn.Close()
		}
		waiter.Wait()
	}
	waiter.Add(1)
	go func() {
		for running {
			pingStart := time.Now()
			conn, err = c.Dial(uri)
			if err != nil {
				fmt.Printf("Ping to %v fail with dial error %v", uri, err)
				time.Sleep(delay)
				continue
			}
			runc++
			fmt.Fprintf(bytes.NewBuffer(buf), "%v", runc)
			_, err = conn.Write(buf)
			if err != nil {
				fmt.Printf("Ping to %v fail with write error %v", uri, err)
				conn.Close()
				time.Sleep(delay)
				continue
			}
			err = xio.FullBuffer(conn, buf, 64, nil)
			if err != nil {
				fmt.Printf("Ping to %v fail with read error %v", uri, err)
				conn.Close()
				time.Sleep(delay)
				continue
			}
			pingUsed := time.Now().Sub(pingStart)
			fmt.Printf("%v Bytes from %v time=%v\n", len(buf), uri, pingUsed)
			conn.Close()
			time.Sleep(delay)
		}
		waiter.Done()
	}()
	return
}

//PrintState will print state from uri
func (c *Console) PrintState(uri, query string) (err error) {
	if !strings.Contains(uri, "tcp://state") {
		if len(uri) > 0 {
			uri += "->"
		}
		uri += "http://state"
	}
	if query == "*" || len(query) < 1 {
		query = "*=*"
	}
	state, err := c.Client.GetMap(EncodeWebURI("http://(%v)?%v", uri, query))
	if err != nil {
		return
	}
	fmt.Printf("[Channels]\n")
	channels := state.Map("channels")
	for name := range channels {
		fmt.Printf(" ->%v\n", name)
		bond := channels.Map(name)
		for idx := range bond {
			val := bond.Map(idx)
			idxVal, _ := strconv.ParseInt(strings.Replace(idx, "_", "", -1), 10, 64)
			heartbeat := val.Int64Def(0, "heartbeat")
			hs := time.Unix(0, heartbeat*1e6).Format("2006-01-02 15:04:05")
			fmt.Printf("   %d % 4d   %v   %v\n", idxVal, int(val["used"].(float64)), hs, val["connect"])
		}
	}
	fmt.Printf("\n\n[Table]\n")
	table := state.ArrayStrDef(nil, "table")
	for _, t := range table {
		fmt.Printf(" %v\n", t)
	}
	fmt.Printf("\n")
	return
}

//DialPiper will dial uri on router and return piper
func (c *Console) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	piper := NewWaitedPiper()
	_, err = c.dialAll(uri, piper)
	raw = piper
	return
}

//Proxy will start shell by runner and rewrite all tcp connection by console
func (c *Console) Proxy(uri string, proxyRunner, proxyKey string, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) (err error) {
	server := proxy.NewServer(xio.PiperDialerF(func(target string, bufferSize int) (raw xio.Piper, err error) {
		targetURI := uri
		targetURI = strings.ReplaceAll(targetURI, "${URI}", target)
		targetURI = strings.ReplaceAll(targetURI, "${HOST}", strings.TrimPrefix(target, "tcp://"))
		raw, err = c.DialPiper(targetURI, bufferSize)
		return
	}))
	listener, err := server.Start("127.0.0.1:0")
	if err == nil {
		cmd := exec.Command(proxyRunner, args...)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%v=%v", proxyKey, listener.Addr()))
		cmd.Env = append(cmd.Env, env...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
		err = cmd.Run()
		server.Close()
		listener.Close()
	}
	return
}
