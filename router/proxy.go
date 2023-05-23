package router

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/util/proxy"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/util/xnet"
	"golang.org/x/net/websocket"
)

// Proxy is an implementation of proxy router
type Proxy struct {
	*Router        //the router
	*proxy.Forward //the forward
	Name           string
	ReconnectDelay time.Duration //reconnect delay
	Dir            string        //the work dir
	Cert           string        //the tls cert
	Key            string        //the tls key
	CA             string        //the tls ca
	Insecure       bool          //the tls use insecure
	Handler        Handler
	master         net.Listener
	stopping       bool
	exiter         chan int
	waiter         sync.WaitGroup
}

// NewProxy will return new Proxy by name
func NewProxy(name string, handler Handler) (px *Proxy) {
	px = &Proxy{
		Router:         NewRouter(name, nil),
		Forward:        proxy.NewForward(name),
		Name:           name,
		Handler:        handler,
		ReconnectDelay: 3 * time.Second,
		exiter:         make(chan int, 10),
		waiter:         sync.WaitGroup{},
	}
	px.Router.Handler = px
	px.Forward.Dialer = px
	return
}

// Listen will listen master router on address
func (p *Proxy) Listen(addr string) (err error) {
	var tlsConfig *tls.Config
	if len(p.Cert) > 0 {
		InfoLog("Proxy(%v) load x509 cert:%v,key:%v", p.Name, p.Cert, p.Key)
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(p.Cert, p.Key)
		if err != nil {
			ErrorLog("Proxy(%v) load cert fail with %v", p.Name, err)
			return
		}
		tlsConfig = &tls.Config{InsecureSkipVerify: p.Insecure}
		tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
		tlsConfig.Rand = rand.Reader
	}
	addrParts := strings.SplitN(addr, "://", 2)
	addrListen := addr
	if len(addrParts) > 1 {
		addrListen = addrParts[1]
	}
	if tlsConfig != nil {
		p.master, err = tls.Listen("tcp", addrListen, tlsConfig)
	} else {
		p.master, err = net.Listen("tcp", addrListen)
	}
	if err != nil {
		return
	}
	switch addrParts[0] {
	case "ws", "wss":
		server := http.Server{
			Handler: websocket.Server{
				Handler: p.AcceptWsConn,
				Handshake: func(c *websocket.Config, r *http.Request) (xerr error) {
					c.Origin, xerr = url.Parse("tcp://" + r.RemoteAddr)
					if xerr == nil {
						c.Origin.Scheme = ""
					}
					return xerr
				},
			},
		}
		p.waiter.Add(1)
		go func() {
			server.Serve(p.master)
			p.waiter.Done()
		}()
		InfoLog("Proxy(%v) listen web master on %v", p.Name, addr)
	default:
		p.waiter.Add(1)
		go p.loopMaster(p.master)
		InfoLog("Proxy(%v) listen tcp master on %v", p.Name, addr)
	}
	return
}

func (p *Proxy) loopMaster(l net.Listener) {
	defer p.waiter.Done()
	var err error
	var conn net.Conn
	for {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		DebugLog("Proxy(%v) master accepting connection from %v", p.Name, conn.RemoteAddr())
		p.Router.Accept(NewInfoRWC(conn, conn.RemoteAddr().String()), false)
	}
	l.Close()
	InfoLog("Proxy(%v) master accept on %v is stopped", p.Name, l.Addr())
}

func (p *Proxy) AcceptWsConn(conn *websocket.Conn) {
	DebugLog("Proxy(%v) master accepting connection from %v", p.Name, conn.RemoteAddr())
	p.Router.Accept(NewInfoRWC(conn, conn.RemoteAddr().String()), true)
}

// Stop will stop all
func (p *Proxy) Stop() (err error) {
	InfoLog("Proxy(%v) is closing", p.Name)
	p.stopping = true
	p.exiter <- 1
	if p.master != nil {
		err = p.master.Close()
		InfoLog("Proxy(%v) master is closed", p.Name)
	}
	if p.Forward != nil {
		p.Forward.Stop()
	}
	p.Router.Stop()
	p.waiter.Wait()
	InfoLog("Proxy(%v) router is closed", p.Name)
	return
}

func (p *Proxy) runReconnect(args xmap.M) {
	ticker := time.NewTicker(p.ReconnectDelay)
	running := true
	for running {
		_, _, err := p.Login(args)
		running = err != nil //stop
		if running {
			select {
			case <-ticker.C:
			case <-p.exiter:
				running = false
			}
		}
	}
}

// DialRaw will dial raw connection
func (p *Proxy) DialRaw(channel Conn, sid uint16, uri string) (raw Conn, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	raw, err = p.Handler.DialRaw(channel, sid, uri)
	return
}

// OnConnDialURI is on connection dial uri
func (p *Proxy) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	err = p.Handler.OnConnDialURI(channel, conn, parts)
	return
}

// OnConnLogin is on connection login
func (p *Proxy) OnConnLogin(channel Conn, args string) (name string, result xmap.M, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	name, result, err = p.Handler.OnConnLogin(channel, args)
	return
}

// OnConnClose will be called when connection is closed
func (p *Proxy) OnConnClose(conn Conn) (err error) {
	if p.stopping {
		return
	}
	context := conn.Context()
	if p.Handler != nil {
		err = p.Handler.OnConnClose(conn)
	}
	if err == nil && context.IntDef(-1, "login_conn") == 1 {
		go p.runReconnect(context.Map("option"))
		InfoLog("Proxy(%v) the channel(%v) is closed, will reconnect it", p.Name, conn)
	} else {
		InfoLog("Proxy(%v) the channel(%v) is closed by %v, remove it", p.Name, conn, err)
	}
	return nil
}

func (p *Proxy) OnConnJoin(channel Conn, option interface{}, result xmap.M) {
	if p.Handler != nil {
		p.Handler.OnConnJoin(channel, option, result)
	}
}

func (p *Proxy) loadTlsConfig(tlsCert, tlsKey, tlsCA string) (config *tls.Config, err error) {
	if len(tlsCert) > 0 && !filepath.IsAbs(tlsCert) {
		tlsCert = filepath.Join(p.Dir, tlsCert)
	}
	if len(tlsKey) > 0 && !filepath.IsAbs(tlsKey) {
		tlsKey = filepath.Join(p.Dir, tlsKey)
	}
	if len(tlsCA) > 0 && !filepath.IsAbs(tlsCA) {
		tlsCA = filepath.Join(p.Dir, tlsCA)
	}
	fmt.Println(tlsCA)
	config = &tls.Config{InsecureSkipVerify: p.Insecure}
	config.Rand = rand.Reader
	if len(tlsCert) > 0 {
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			ErrorLog("Proxy(%v) load cert fail with %v", p.Name, err)
			return
		}
		config.Certificates = append(config.Certificates, cert)
	}
	if len(tlsCA) > 0 {
		var certPEM []byte
		certPEM, err = ioutil.ReadFile(tlsCA)
		if err != nil {
			ErrorLog("Proxy(%v) load ca fail with %v", p.Name, err)
			return
		}
		certPool := x509.NewCertPool()
		ok := certPool.AppendCertsFromPEM([]byte(certPEM))
		if !ok {
			ErrorLog("Proxy(%v) append ca fail", p.Name)
			return
		}
		config.RootCAs = certPool
	}
	return
}

// LoginChannel will login all channel by options.
func (p *Proxy) LoginChannel(reconnect bool, channels ...xmap.M) (err error) {
	for _, channel := range channels {
		if channel.Int("enable") < 1 {
			continue
		}
		_, _, err = p.Login(channel)
		if err == nil {
			continue
		}
		WarnLog("Proxy(%v) login to %v fail with %v", p.Name, channel.StrDef("", "remote"), err)
		if reconnect {
			go p.runReconnect(channel)
		} else {
			return
		}
	}
	return
}

// Login will add channel by local address, master address, auth token, channel index.
func (p *Proxy) Login(option xmap.M) (channel Conn, result xmap.M, err error) {
	var local, remote, tlsCert, tlsKey, tlsCA, tlsVerify string
	err = option.ValidFormat(`
		local,O|S,L:0;
		remote,R|S,L:0;
		tls_cert,O|S,L:0;
		tls_key,O|S,L:0;
		tls_ca,O|S,L:0;
	`, &local, &remote, &tlsCert, &tlsKey, &tlsCA)
	if err != nil {
		return
	}
	tlsVerify = option.StrDef("", "tls_verify")
	var conn net.Conn
	if strings.HasPrefix(remote, "ws://") || strings.HasPrefix(remote, "wss://") {
		InfoLog("Proxy(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, tlsCert, tlsKey, tlsCA, tlsVerify)
		var rawConn io.ReadWriteCloser
		dialer := xnet.NewWebsocketDialer()
		if len(tlsCert) > 0 || len(tlsCA) > 0 {
			var config *tls.Config
			config, err = p.loadTlsConfig(tlsCert, tlsKey, tlsCA)
			if err != nil {
				ErrorLog("Proxy(%v) load tls config fail with %v", p.Name, err)
				return
			}
			dialer.TlsConfig = config
		}
		if len(tlsVerify) > 0 {
			dialer.TlsConfig.InsecureSkipVerify = tlsVerify == "0"
		}
		rawConn, err = dialer.Dial(remote)
		if err == nil {
			conn = rawConn.(net.Conn)
		}
	} else {
		var dialer net.Dialer
		if len(local) > 0 {
			dialer.LocalAddr, err = net.ResolveTCPAddr("tcp", local)
			if err != nil {
				return
			}
		}
		InfoLog("Proxy(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, tlsCert, tlsKey, tlsCA, tlsVerify)
		if len(tlsCert) > 0 || len(tlsCA) > 0 {
			var config *tls.Config
			config, err = p.loadTlsConfig(tlsCert, tlsKey, tlsCA)
			if err != nil {
				ErrorLog("Proxy(%v) load tls config fail with %v", p.Name, err)
				return
			}
			if len(tlsVerify) > 0 {
				config.InsecureSkipVerify = tlsVerify == "0"
			}
			conn, err = tls.DialWithDialer(&dialer, "tcp", remote, config)
		} else {
			InfoLog("Router(%v) start dial to %v", p.Name, remote)
			conn, err = dialer.Dial("tcp", remote)
		}
	}
	if err != nil {
		WarnLog("Proxy(%v) dial to %v fail with %v", p.Name, remote, err)
		return
	}
	auth := xmap.M{}
	for key, val := range option {
		auth[key] = val
	}
	auth["name"] = p.Name
	channel, result, err = p.JoinConn(NewInfoRWC(conn, conn.RemoteAddr().String()), auth)
	if err == nil {
		channel.Context()["option"] = option
		channel.Context()["login_conn"] = 1
		p.Handler.OnConnJoin(channel, option, result)
	}
	return
}

// InfoRWC is external ReadWriteCloser to get info to String
type InfoRWC struct {
	io.ReadWriteCloser
	Info string
}

// NewInfoRWC will return new nfoRWC
func NewInfoRWC(raw io.ReadWriteCloser, info string) *InfoRWC {
	return &InfoRWC{ReadWriteCloser: raw, Info: info}
}

func (i *InfoRWC) RawValue() interface{} {
	return i.ReadWriteCloser
}

func (i *InfoRWC) String() string {
	return i.Info
}

// EncodeWebURI will replace string in () as base64 encoding
func EncodeWebURI(format string, args ...interface{}) string {
	return regexp.MustCompile(`\([^\\)]*\)`).ReplaceAllStringFunc(fmt.Sprintf(format, args...), func(having string) string {
		having = strings.Trim(having, "()")
		return "base64-" + base64.RawURLEncoding.EncodeToString([]byte(having))
	})
}
