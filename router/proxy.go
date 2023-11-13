package router

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/proxy"
	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/util/xnet"
	"github.com/quic-go/quic-go"
	"golang.org/x/net/websocket"
)

// Proxy is an implementation of proxy router
type Proxy struct {
	*Router        //the router
	*proxy.Forward //the forward
	Name           string
	KeepDelay      time.Duration     //keep login delay
	Channels       map[string]xmap.M //the all keep channels
	Dir            string            //the work dir
	Insecure       bool
	Handler        Handler
	listenerAll    map[string]net.Listener
	listenerLck    sync.RWMutex
	stopping       bool
	exiter         chan int
	waiter         sync.WaitGroup
}

// NewProxy will return new Proxy by name
func NewProxy(name string, bufferSize int, handler Handler) (px *Proxy) {
	px = &Proxy{
		Router:      NewRouter(name, nil),
		Forward:     proxy.NewForward(name),
		Name:        name,
		Handler:     handler,
		KeepDelay:   3 * time.Second,
		Channels:    map[string]xmap.M{},
		listenerAll: map[string]net.Listener{},
		listenerLck: sync.RWMutex{},
		exiter:      make(chan int, 10),
		waiter:      sync.WaitGroup{},
	}
	px.Router.Handler = px
	px.Router.BufferSize = bufferSize
	px.Forward.Dialer = px
	px.Forward.BufferSize = bufferSize
	return
}

func (p *Proxy) loadServerConfig(tlsCert, tlsKey string) (config *tls.Config, err error) {
	if len(tlsCert) < 1 || len(tlsKey) < 1 {
		err = fmt.Errorf("tls_cert/tls_key is required")
		return
	}
	var cert tls.Certificate
	cert, err = LoadX509KeyPair(p.Dir, tlsCert, tlsKey)
	if err != nil {
		ErrorLog("Proxy(%v) load cert fail with %v by cert:%v,key:%v", p.Name, err, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey))
		return
	}
	config = &tls.Config{InsecureSkipVerify: p.Insecure}
	config.Certificates = append(config.Certificates, cert)
	config.Rand = rand.Reader
	return
}

func (p *Proxy) listenAddr(network, addr, tlsCert, tlsKey string) (ln net.Listener, err error) {
	var config *tls.Config
	switch network {
	case "wss", "tls":
		config, err = p.loadServerConfig(tlsCert, tlsKey)
		if err == nil {
			ln, err = tls.Listen("tcp", addr, config)
		}
	case "ws", "tcp":
		ln, err = net.Listen("tcp", addr)
	case "quic":
		config, err = p.loadServerConfig(tlsCert, tlsKey)
		if err == nil {
			config.NextProtos = []string{"bs"}
			ln, err = quicListen(addr, config)
		}
	default:
		err = fmt.Errorf("%v is not supported", network)
	}
	return
}

// Listen will listen master router on address
func (p *Proxy) Listen(addr, tlsCert, tlsKey string) (err error) {
	addrParts := strings.SplitN(addr, "://", 2)
	var addrNetwork, addrListen string
	if len(addrParts) > 1 {
		addrNetwork = addrParts[0]
		addrListen = addrParts[1]
	} else {
		addrNetwork = "tcp"
		addrListen = addr
	}
	ln, err := p.listenAddr(addrNetwork, addrListen, tlsCert, tlsKey)
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
		p.listenerLck.Lock()
		p.listenerAll[fmt.Sprintf("%p", ln)] = ln
		p.listenerLck.Unlock()
		p.waiter.Add(1)
		go func() {
			server.Serve(ln)
			p.waiter.Done()
			p.listenerLck.Lock()
			delete(p.listenerAll, fmt.Sprintf("%p", ln))
			p.listenerLck.Unlock()
		}()
	default:
		p.listenerLck.Lock()
		p.listenerAll[fmt.Sprintf("%p", ln)] = ln
		p.listenerLck.Unlock()
		p.waiter.Add(1)
		go func() {
			p.loopMaster(addr, ln)
			p.waiter.Done()
			p.listenerLck.Lock()
			delete(p.listenerAll, fmt.Sprintf("%p", ln))
			p.listenerLck.Unlock()
		}()
	}
	InfoLog("Proxy(%v) listen %v master on %v by addr:%v,cert:%v,key:%v", p.Name, addrNetwork, ln.Addr(), addr, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey))
	return
}

func (p *Proxy) loopMaster(addr string, l net.Listener) {
	var err error
	var conn net.Conn
	for {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		InfoLog("Proxy(%v) master %v accept connection from %v", p.Name, addr, conn.RemoteAddr())
		p.Router.Accept(NewInfoRWC(conn, conn.RemoteAddr().String()), false)
	}
	l.Close()
	InfoLog("Proxy(%v) master accept on %v is stopped", p.Name, l.Addr())
}

func (p *Proxy) AcceptWsConn(conn *websocket.Conn) {
	InfoLog("Proxy(%v) master %v accept ws connection from %v", p.Name, conn.LocalAddr(), conn.RemoteAddr())
	p.Router.Accept(NewInfoRWC(conn, conn.RemoteAddr().String()), true)
}

func (p *Proxy) Start() {
	p.Router.Start()
	p.waiter.Add(1)
	go p.loopKeep()
}

// Stop will stop all
func (p *Proxy) Stop() (err error) {
	InfoLog("Proxy(%v) is closing", p.Name)
	p.stopping = true
	p.exiter <- 1
	p.exiter <- 1
	p.listenerLck.RLock()
	for _, ln := range p.listenerAll {
		ln.Close()
		InfoLog("Proxy(%v) listener %v is closed", p.Name, ln.Addr())
	}
	p.listenerLck.RUnlock()
	if p.Forward != nil {
		p.Forward.Stop()
	}
	p.Router.Stop()
	p.waiter.Wait()
	InfoLog("Proxy(%v) router is closed", p.Name)
	return
}

func (p *Proxy) loopKeep() {
	defer p.waiter.Done()
	p.procKeep()
	ticker := time.NewTicker(p.KeepDelay)
	running := true
	for running {
		select {
		case <-ticker.C:
			p.procKeep()
		case <-p.exiter:
			running = false
		}
	}
}

func (p *Proxy) procKeep() {
	defer func() {
		if perr := recover(); perr != nil {
			ErrorLog("Proxy(%v) proc keep is panic with %v, callstack is \n%v", p.Name, perr, xdebug.CallStack())
		}
	}()
	p.Keep()
}

// DialRawConn will dial raw connection
func (p *Proxy) DialRawConn(channel Conn, sid uint16, uri string) (conn Conn, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	conn, err = p.Handler.DialRawConn(channel, sid, uri)
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
	if p.Handler != nil {
		err = p.Handler.OnConnClose(conn)
	}
	InfoLog("Proxy(%v) the channel(%v) is closed by %v, remove it", p.Name, conn, err)
	return nil
}

func (p *Proxy) OnConnJoin(channel Conn, option interface{}, result xmap.M) {
	if p.Handler != nil {
		p.Handler.OnConnJoin(channel, option, result)
	}
}

func (p *Proxy) OnConnNotify(channel Conn, message []byte) {
	if p.Handler != nil {
		p.Handler.OnConnNotify(channel, message)
	}
}

func (p *Proxy) loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify string) (config *tls.Config, err error) {
	if len(tlsCA) < 1 && len(tlsCert) < 1 && (len(tlsVerify) < 1 || tlsVerify == "1") {
		err = fmt.Errorf("at least one of tls_ca/tls_cert is required")
		return
	}
	config = &tls.Config{InsecureSkipVerify: p.Insecure}
	config.Rand = rand.Reader

	if len(tlsCert) < 1 {
		tlsCert = "bsrouter.pem"
	}
	if len(tlsKey) < 1 {
		tlsKey = "bsrouter.key"
	}
	var cert tls.Certificate
	cert, err = LoadX509KeyPair(p.Dir, tlsCert, tlsKey)
	if err != nil {
		ErrorLog("Proxy(%v) load cert fail with %v", p.Name, err)
		return
	}
	config.Certificates = append(config.Certificates, cert)

	if len(tlsCA) > 0 {
		var certPEM []byte
		certPEM, err = LoadPEMBlock(p.Dir, tlsCA)
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
	if len(tlsVerify) > 0 {
		config.InsecureSkipVerify = tlsVerify == "0"
	}
	return
}

// Keep will keep channel connection
func (p *Proxy) Keep() (err error) {
	for name, channel := range p.Channels {
		for {
			connected := p.CountChannel(name)
			keep := channel.IntDef(1, "keep")
			if connected >= keep {
				break
			}
			_, _, err = p.Login(channel)
			if err != nil {
				break
			}
		}
	}
	return
}

func (p *Proxy) dialConn(remote, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify string) (conn net.Conn, err error) {
	if !strings.Contains(remote, "://") {
		remote = "tcp://" + remote
	}
	remoteURI, xerr := url.Parse(remote)
	if xerr != nil {
		err = xerr
		return
	}
	var config *tls.Config
	switch remoteURI.Scheme {
	case "ws", "wss":
		InfoLog("Proxy(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey), TlsConfigShow(tlsCA), tlsVerify)
		dialer := xnet.NewWebsocketDialer()
		if remoteURI.Scheme == "wss" {
			dialer.TlsConfig, err = p.loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify)
			if err != nil {
				ErrorLog("Proxy(%v) load tls config fail with %v", p.Name, err)
				return
			}
		}
		var rawConn io.ReadWriteCloser
		rawConn, err = dialer.Dial(remote)
		if err == nil {
			conn = rawConn.(net.Conn)
		}
	case "tls":
		InfoLog("Proxy(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey), TlsConfigShow(tlsCA), tlsVerify)
		config, err = p.loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify)
		if err != nil {
			ErrorLog("Proxy(%v) load tls config fail with %v", p.Name, err)
			return
		}
		var dialer net.Dialer
		conn, err = tls.DialWithDialer(&dialer, "tcp", remoteURI.Host, config)
	case "tcp":
		InfoLog("Router(%v) start dial to %v", p.Name, remote)
		conn, err = net.Dial("tcp", remoteURI.Host)
	case "quic":
		InfoLog("Proxy(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey), TlsConfigShow(tlsCA), tlsVerify)
		config, err = p.loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify)
		if err != nil {
			ErrorLog("Proxy(%v) load tls config fail with %v", p.Name, err)
			return
		}
		config.NextProtos = []string{"bs"}
		config.ServerName = tlsHost
		if len(config.ServerName) < 1 {
			config.ServerName = remoteURI.Hostname()
		}
		conn, err = quicDial(remoteURI.Host, config)
	}
	return
}

// Login will add channel by local address, master address, auth token, channel index.
func (p *Proxy) Login(option xmap.M) (channel Conn, result xmap.M, err error) {
	var remoteAll, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify string
	err = option.ValidFormat(`
		remote,R|S,L:0;
		tls_cert,O|S,L:0;
		tls_key,O|S,L:0;
		tls_host,O|S,L:0;
		tls_ca,O|S,L:0;
	`, &remoteAll, &tlsCert, &tlsKey, &tlsHost, &tlsCA)
	if err != nil {
		return
	}
	tlsVerify = option.StrDef("", "tls_verify")
	for _, remote := range strings.Split(remoteAll, ",") {
		conn, xerr := p.dialConn(remote, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify)
		if xerr != nil {
			err = xerr
			WarnLog("Proxy(%v) dial to %v fail with %v", p.Name, remote, err)
			return
		}
		auth := xmap.M{}
		for key, val := range option {
			if key == "remote" || key == "tls_cert" || key == "tls_key" || key == "tls_ca" || key == "tls_host" {
				continue
			}
			auth[key] = val
		}
		auth["name"] = p.Name
		channel, result, err = p.JoinConn(NewInfoRWC(conn, conn.RemoteAddr().String()), auth)
		if err == nil {
			channel.Context()["option"] = option
			channel.Context()["login_conn"] = 1
			p.Handler.OnConnJoin(channel, option, result)
			break
		}
	}
	return
}

// Ping will ping channel
func (p *Proxy) Ping(option xmap.M) (speed xmap.M, err error) {
	var remoteAll, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify string
	err = option.ValidFormat(`
		remote,R|S,L:0;
		tls_cert,O|S,L:0;
		tls_key,O|S,L:0;
		tls_host,O|S,L:0;
		tls_ca,O|S,L:0;
	`, &remoteAll, &tlsCert, &tlsKey, &tlsHost, &tlsCA)
	if err != nil {
		return
	}
	tlsVerify = option.StrDef("", "tls_verify")
	speed = xmap.M{}
	for _, remote := range strings.Split(remoteAll, ",") {
		begin := time.Now()
		conn, xerr := p.dialConn(remote, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify)
		if xerr == nil {
			xerr = p.PingConn(NewInfoRWC(conn, conn.RemoteAddr().String()))
		}
		used := time.Since(begin)
		if xerr == nil {
			InfoLog("Proxy(%v) ping to %v success with %v", p.Name, remote, used)
		} else {
			WarnLog("Proxy(%v) ping to %v fail with %v,%v", p.Name, remote, used, xerr)
		}
		result := xmap.M{}
		if xerr == nil {
			result["code"] = 0
			result["speed"] = used.Milliseconds()
		} else {
			result["code"] = -1
			result["speed"] = used.Milliseconds()
			result["error"] = xerr.Error()
		}
		speed[remote] = result
		if xerr == nil && conn != nil {
			conn.Close()
		}
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

type quicConn struct {
	quic.Connection
	quic.Stream
}

type quicListener struct {
	*quic.Listener
}

func quicListen(addr string, tls *tls.Config) (ln net.Listener, err error) {
	quicConf := &quic.Config{EnableDatagrams: true}
	quicConf.KeepAlivePeriod = time.Second
	base, err := quic.ListenAddr(addr, tls, quicConf)
	if err != nil {
		return
	}
	ln = &quicListener{Listener: base}
	return
}

func quicDial(addr string, tls *tls.Config) (conn net.Conn, err error) {
	quicConf := &quic.Config{EnableDatagrams: true}
	quicConf.KeepAlivePeriod = time.Second
	base, err := quic.DialAddr(context.Background(), addr, tls, quicConf)
	if err != nil {
		return
	}
	stream, err := base.OpenStream()
	if err != nil {
		return
	}
	return &quicConn{Connection: base, Stream: stream}, nil
}

func (q *quicListener) Accept() (net.Conn, error) {
	conn, err := q.Listener.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return nil, err
	}
	return &quicConn{Connection: conn, Stream: stream}, nil
}

func TlsConfigShow(from string) (info string) {
	if strings.HasPrefix(from, "-----BEGIN") {
		info = strings.SplitN(from, "\n", 2)[0]
	} else if strings.HasPrefix(from, "0x") {
		if len(from) > 32 {
			info = from[0:32]
		} else {
			info = from
		}
	} else {
		info = from
	}
	return
}

var LoadPEMBlock = dialer.LoadPEMBlock

var LoadX509KeyPair = dialer.LoadX509KeyPair
