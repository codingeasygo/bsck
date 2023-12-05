package router

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/proxy"
	sproxy "github.com/codingeasygo/util/proxy/socks"
	wproxy "github.com/codingeasygo/util/proxy/ws"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/util/xtime"
)

type Rewrite struct {
	Wildcard map[string]string
	Single   map[string]string
}

func NewRewrite() (rewrite *Rewrite) {
	rewrite = &Rewrite{
		Wildcard: map[string]string{},
		Single:   map[string]string{},
	}
	return
}

func (r *Rewrite) Read(filename string) (err error) {
	hostData, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	space := regexp.MustCompile(`\s+`)
	lines := strings.Split(string(hostData), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.SplitN(line, "#", 2)[0]
		line = strings.TrimSpace(line)
		parts := space.Split(line, -1)
		if len(parts) < 2 {
			continue
		}
		for _, host := range parts[1:] {
			if strings.HasPrefix(host, "*.") {
				r.Wildcard[strings.TrimPrefix(host, "*")] = parts[0]
			} else {
				r.Single[host] = parts[0]
			}
		}
	}
	return
}

func (r *Rewrite) Match(host string) (rewrited string, ok bool) {
	rewrited, ok = r.Single[host]
	if ok {
		return
	}
	for w, v := range r.Wildcard {
		if strings.HasSuffix(host, w) {
			rewrited, ok = v, true
			break
		}
	}
	if !ok {
		rewrited = host
	}
	return
}

// Console is node console to dial connection
type Console struct {
	Client     *xhttp.Client
	SlaverURI  string
	BufferSize int
	Env        []string
	Rewrite    *Rewrite //domain to rewrite
	conns      map[string]net.Conn
	running    map[string]io.Closer
	locker     sync.RWMutex
}

// NewConsole will return new Console
func NewConsole(slaverURI string) (console *Console) {
	console = &Console{
		SlaverURI:  slaverURI,
		BufferSize: 32 * 1024,
		Rewrite:    NewRewrite(),
		conns:      map[string]net.Conn{},
		running:    map[string]io.Closer{},
		locker:     sync.RWMutex{},
	}
	client := &http.Client{
		Transport: &http.Transport{
			Dial: console.dialNet,
		},
	}
	console.Client = xhttp.NewClient(client)
	return
}

func NewConsoleByConfig(config *Config) (console *Console, err error) {
	uri := ""
	if len(config.Console.SOCKS) > 0 {
		uri = config.Console.SOCKS
		if !strings.HasPrefix(uri, "socks5://") {
			uri = "socks5://" + uri
		}
	} else if len(config.Console.WS) > 0 {
		uri = config.Console.WS
		if !strings.HasPrefix(uri, "ws://") && !strings.HasPrefix(uri, "wss://") {
			uri = "ws://" + uri
		}
	} else {
		err = fmt.Errorf("not console config found")
		return
	}
	console = NewConsole(uri)
	return
}

// Close will close all connection
func (c *Console) Close() (err error) {
	all := []io.Closer{}
	c.locker.Lock()
	for _, conn := range c.conns {
		all = append(all, conn)
	}
	for _, conn := range c.running {
		all = append(all, conn)
	}
	c.locker.Unlock()
	for _, conn := range all {
		conn.Close()
	}
	return
}

func (c *Console) DialConn(raw io.ReadWriteCloser, uri string) (sid uint16, err error) {
	DebugLog("Console start dial to %v on slaver %v", uri, c.SlaverURI)
	var conn net.Conn
	if strings.HasPrefix(c.SlaverURI, "socks5://") {
		conn, err = sproxy.DialType(c.SlaverURI, 0x05, uri)
	} else if strings.HasPrefix(c.SlaverURI, "ws://") || strings.HasPrefix(c.SlaverURI, "wss://") {
		conn, err = wproxy.Dial(c.SlaverURI, uri)
	} else {
		err = fmt.Errorf("not supported slaver %v", c.SlaverURI)
	}
	if err != nil {
		if waiter, ok := raw.(ReadyWaiter); ok {
			waiter.Ready(err, nil)
		}
		raw.Close()
		DebugLog("Console dial to %v on slaver %v fail with %v", uri, c.SlaverURI, err)
		return
	}
	DebugLog("Console dial to %v on slaver %v success", uri, c.SlaverURI)
	proc := func(err error) {
		defer func() {
			conn.Close()
			raw.Close()
		}()
		if err == nil {
			go xio.CopyBuffer(conn, raw, make([]byte, c.BufferSize))
			xio.CopyBuffer(raw, conn, make([]byte, c.BufferSize))
			c.locker.Lock()
			delete(c.conns, fmt.Sprintf("%p", conn))
			c.locker.Unlock()
		}
	}
	c.locker.Lock()
	c.conns[fmt.Sprintf("%p", conn)] = conn
	c.locker.Unlock()
	if waiter, ok := raw.(ReadyWaiter); ok {
		waiter.Ready(nil, proc)
	} else {
		go proc(nil)
	}
	return
}

// dialNet is net dialer to router
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
	conn, raw := dialer.CreatePipedConn()
	if err == nil {
		_, err = c.DialConn(raw, addr)
		if err != nil {
			conn.Close()
			raw.Close()
		}
	}
	return
}

// Redirect will redirect connection to reader/writer/closer
func (c *Console) Redirect(uri string, reader io.Reader, writer io.Writer, closer io.Closer) (err error) {
	raw := xio.NewCombinedReadWriteCloser(reader, writer, closer)
	_, err = c.DialConn(raw, uri)
	return
}

// Dial will dial connection by uri
func (c *Console) Dial(uri string) (conn io.ReadWriteCloser, err error) {
	conn, raw := dialer.CreatePipedConn()
	if err == nil {
		_, err = c.DialConn(raw, uri)
		if err != nil {
			conn.Close()
			raw.Close()
		}
	}
	return
}

// Ping will start ping to uri
func (c *Console) Ping(uri string, delay time.Duration, max uint64) (err error) {
	if !strings.Contains(uri, "tcp://echo") {
		if len(uri) > 0 {
			uri += "->"
		}
		uri += "tcp://echo"
	}
	var running bool = true
	var runCount uint64
	var conn io.ReadWriteCloser
	buf := make([]byte, 64)
	closer := xio.CloserF(func() (err error) {
		running = false
		if conn != nil {
			conn.Close()
		}
		return
	})
	c.locker.Lock()
	c.running[fmt.Sprintf("%p", closer)] = closer
	c.locker.Unlock()
	for running && (max < 1 || runCount < max) {
		runCount++
		pingStart := time.Now()
		conn, err = c.Dial(uri)
		if err == nil {
			fmt.Fprintf(bytes.NewBuffer(buf), "%v", runCount)
			_, err = conn.Write(buf)
			if err == nil {
				err = xio.FullBuffer(conn, buf, 64, nil)
			}
		}
		if err != nil {
			fmt.Printf("Ping to %v fail with read error %v\n", uri, err)
			conn.Close()
			time.Sleep(delay)
			continue
		}
		pingUsed := time.Since(pingStart)
		fmt.Printf("%v Bytes from %v time=%v\n", len(buf), uri, pingUsed)
		conn.Close()
		time.Sleep(delay)
	}
	c.locker.Lock()
	delete(c.running, fmt.Sprintf("%p", closer))
	c.locker.Unlock()
	closer.Close()
	return
}

// PrintState will print state from uri
func (c *Console) PrintState(uri, query string) (err error) {
	if !strings.Contains(uri, "http://state") {
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
		bondAll := channels.ArrayMapDef(nil, name)
		for _, bond := range bondAll {
			ping := bond.MapDef(xmap.M{}, "ping")
			pingSpeed := time.Duration(ping.Int64Def(0, "speed")) * time.Millisecond
			pingLast := ping.Int64Def(0, "last")
			pingLastStr := xtime.TimeUnix(pingLast).Format("2006-01-02 15:04:05")
			fmt.Printf("   %v	%v   %v   %v\n", bond.Value("id"), pingSpeed, pingLastStr, bond.StrDef("", "connect"))
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

// DialPiper will dial uri on router and return piper
func (c *Console) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	piper := NewRouterPiper()
	_, err = c.DialConn(piper, uri)
	raw = piper
	return
}

func (c *Console) parseProxyURI(uri, target string) (string, error) {
	targetURI := uri
	if strings.HasPrefix(target, "tcp://tcp-") || strings.HasPrefix(target, "tcp://http-") || strings.HasPrefix(target, "tcp-") || strings.HasPrefix(target, "http-") {
		target = strings.TrimPrefix(target, "tcp://")
		target = strings.TrimSuffix(target, ":80")
		target = strings.TrimSuffix(target, ":443")
		parts := strings.SplitN(target, "-", 2)
		targetURI = strings.ReplaceAll(targetURI, "${HOST}", parts[1])
		targetURI = strings.ReplaceAll(targetURI, "${URI}", parts[0]+"://"+parts[1])
	} else if strings.Contains(target, ".") {
		targetURL, err := url.Parse(target)
		if err != nil {
			return "", err
		}
		if rewrite, ok := c.Rewrite.Match(targetURL.Hostname()); ok && len(rewrite) > 0 {
			targetURL.Host = rewrite + ":" + targetURL.Port()
			InfoLog("Console rewrite %v to %v", target, targetURL)
		}
		targetURI = strings.ReplaceAll(targetURI, "${URI}", targetURL.String())
		targetURI = strings.ReplaceAll(targetURI, "${HOST}", targetURL.Host)
	} else {
		target = strings.TrimPrefix(target, "tcp://")
		target = strings.TrimSuffix(target, ":80")
		target = strings.TrimSuffix(target, ":443")
		targetURI = strings.ReplaceAll(targetURI, "${HOST}", target)
		targetURI = strings.ReplaceAll(targetURI, "${URI}", "http://"+target)
	}
	return targetURI, nil
}

func (c *Console) StartForward(loc string, uri string) (listener net.Listener, err error) {
	listener, err = net.Listen("tcp", loc)
	if err != nil {
		return
	}
	c.locker.Lock()
	c.running[fmt.Sprintf("%p", listener)] = listener
	c.locker.Unlock()
	InfoLog("Console start forward %v to %v", loc, uri)
	go func() {
		defer func() {
			c.locker.Lock()
			delete(c.running, fmt.Sprintf("%p", listener))
			c.locker.Unlock()
		}()
		for {
			conn, err := listener.Accept()
			if err != nil {
				break
			}
			_, err = c.DialConn(conn, uri)
			if err == nil {
				InfoLog("Console start transfer %v to %v", conn.RemoteAddr(), uri)
			} else {
				WarnLog("Console transfer %v to %v fail with %v", loc, uri, err)
			}
		}
	}()
	return
}

// StartProxy will start proxy local to uri
func (c *Console) StartProxy(loc, uri string) (server *proxy.Server, listener net.Listener, err error) {
	server = proxy.NewServer(xio.PiperDialerF(func(target string, bufferSize int) (raw xio.Piper, err error) {
		targetURI, err := c.parseProxyURI(uri, target)
		if err == nil {
			raw, err = c.DialPiper(targetURI, bufferSize)
		}
		return
	}))
	listener, err = server.Start(loc)
	return
}

// Proxy will start shell by runner and rewrite all tcp connection by console
func (c *Console) Proxy(uri string, process func(listener net.Listener) (err error)) (err error) {
	server, listener, err := c.StartProxy("127.0.0.1:0", uri)
	if err == nil {
		c.locker.Lock()
		c.running[fmt.Sprintf("%p", server)] = server
		c.running[fmt.Sprintf("%p", listener)] = listener
		c.locker.Unlock()
		err = process(listener)
		c.locker.Lock()
		delete(c.running, fmt.Sprintf("%p", server))
		delete(c.running, fmt.Sprintf("%p", listener))
		c.locker.Unlock()
		server.Close()
		listener.Close()
	}
	return
}

// ProxyExec will exec command by runner and rewrite all tcp connection by console
func (c *Console) ProxyExec(uri string, stdin io.Reader, stdout, stderr io.Writer, prepare func(listener net.Listener) (env []string, runner string, args []string, err error)) (err error) {
	err = c.Proxy(uri, func(listener net.Listener) (err error) {
		env, runner, args, err := prepare(listener)
		if err != nil {
			return
		}
		cmd := exec.Command(runner, args...)
		cmd.Env = append(cmd.Env, c.Env...)
		cmd.Env = append(cmd.Env, env...)
		// InfoLog("Console proxy command %v by\narg:%v\nenv:%v\n", runner, args, cmd.Env)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
		c.locker.Lock()
		c.running[fmt.Sprintf("%p", cmd)] = xio.CloserF(cmd.Process.Kill)
		c.locker.Unlock()
		err = cmd.Run()
		c.locker.Lock()
		delete(c.running, fmt.Sprintf("%p", cmd))
		c.locker.Unlock()
		return
	})
	return
}

// ProxyProcess will start process by runner and rewrite all tcp connection by console
func (c *Console) ProxyProcess(uri string, stdin, stdout, stderr *os.File, prepare func(listener net.Listener) (env []string, runner string, args []string, err error)) (err error) {
	err = c.Proxy(uri, func(listener net.Listener) (err error) {
		env, runner, args, err := prepare(listener)
		if err != nil {
			return
		}
		// InfoLog("Console proxy process %v by\narg:%v\nenv:%v\n", runner, args, env)
		process, err := os.StartProcess(runner, append([]string{runner}, args...), &os.ProcAttr{
			Env:   append(c.Env, env...),
			Files: []*os.File{stdin, stdout, stderr},
		})
		if err == nil {
			c.locker.Lock()
			c.running[fmt.Sprintf("%p", process)] = xio.CloserF(process.Kill)
			c.locker.Unlock()
			_, err = process.Wait()
			c.locker.Lock()
			delete(c.running, fmt.Sprintf("%p", process))
			c.locker.Unlock()
		}
		return
	})
	return
}

// ProxySSH will start ssh client command and connect to uri by proxy command.
func (c *Console) ProxySSH(uri string, stdin io.Reader, stdout, stderr io.Writer, proxyCommand, command string, args ...string) (err error) {
	replaceURI := uri
	if len(replaceURI) < 1 {
		replaceURI = "ssh://server"
	}
	replaceURI = strings.ReplaceAll(replaceURI, "->", "_")
	replaceURI = strings.ReplaceAll(replaceURI, "://", "_")
	replaceURI = strings.ReplaceAll(replaceURI, ":", "_")
	for i, a := range args {
		args[i] = strings.ReplaceAll(a, "bshost", replaceURI)
	}
	if !strings.Contains(uri, "://") {
		if len(uri) > 0 {
			uri += "->"
		}
		uri += "ssh://server"
	}
	allArgs := []string{}
	allArgs = append(allArgs, "-o", fmt.Sprintf("ProxyCommand=%v", strings.ReplaceAll(proxyCommand, "${URI}", uri)))
	//
	if command == "ssh" {
		allArgs = append(allArgs, replaceURI)
	}
	allArgs = append(allArgs, args...)
	cmd := exec.Command(command, allArgs...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = c.Env
	c.locker.Lock()
	c.running[fmt.Sprintf("%p", cmd)] = xio.CloserF(cmd.Process.Kill)
	c.locker.Unlock()
	err = cmd.Run()
	c.locker.Lock()
	delete(c.running, fmt.Sprintf("%p", cmd))
	c.locker.Unlock()
	return
}
