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
	"os"
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
	nproxy "golang.org/x/net/proxy"
	"golang.org/x/net/websocket"
)

// Node is an implementation of proxy router
type Node struct {
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

// NewNode will return new Node by name
func NewNode(name string, bufferSize int, handler Handler) (px *Node) {
	px = &Node{
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

func (p *Node) loadServerConfig(tlsCert, tlsKey string) (config *tls.Config, err error) {
	if len(tlsCert) < 1 || len(tlsKey) < 1 {
		err = fmt.Errorf("tls_cert/tls_key is required")
		return
	}
	var cert tls.Certificate
	cert, err = LoadX509KeyPair(p.Dir, tlsCert, tlsKey)
	if err != nil {
		ErrorLog("Node(%v) load cert fail with %v by cert:%v,key:%v", p.Name, err, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey))
		return
	}
	config = &tls.Config{InsecureSkipVerify: p.Insecure}
	config.Certificates = append(config.Certificates, cert)
	config.Rand = rand.Reader
	return
}

func (p *Node) listenAddr(network, addr, tlsCert, tlsKey string) (ln net.Listener, err error) {
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
func (p *Node) Listen(addr, tlsCert, tlsKey string) (err error) {
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
	InfoLog("Node(%v) listen %v master on %v by addr:%v,cert:%v,key:%v", p.Name, addrNetwork, ln.Addr(), addr, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey))
	return
}

func (p *Node) loopMaster(addr string, l net.Listener) {
	var err error
	var conn net.Conn
	for {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		InfoLog("Node(%v) master %v accept connection from %v", p.Name, addr, conn.RemoteAddr())
		p.Router.Accept(NewInfoRWC(conn, conn.RemoteAddr().String()), false)
	}
	l.Close()
	InfoLog("Node(%v) master accept on %v is stopped", p.Name, l.Addr())
}

func (p *Node) AcceptWsConn(conn *websocket.Conn) {
	InfoLog("Node(%v) master %v accept ws connection from %v", p.Name, conn.LocalAddr(), conn.RemoteAddr())
	p.Router.Accept(NewInfoRWC(conn, conn.RemoteAddr().String()), true)
}

func (p *Node) Start() {
	p.Router.Start()
	p.waiter.Add(1)
	go p.loopKeep()
}

// Stop will stop all
func (p *Node) Stop() (err error) {
	InfoLog("Node(%v) is closing", p.Name)
	p.stopping = true
	p.exiter <- 1
	p.exiter <- 1
	p.listenerLck.RLock()
	for _, ln := range p.listenerAll {
		ln.Close()
		InfoLog("Node(%v) listener %v is closed", p.Name, ln.Addr())
	}
	p.listenerLck.RUnlock()
	if p.Forward != nil {
		p.Forward.Stop()
	}
	p.Router.Stop()
	p.waiter.Wait()
	InfoLog("Node(%v) router is closed", p.Name)
	return
}

func (p *Node) loopKeep() {
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

func (p *Node) procKeep() {
	defer func() {
		if perr := recover(); perr != nil {
			ErrorLog("Node(%v) proc keep is panic with %v, callstack is \n%v", p.Name, perr, xdebug.CallStack())
		}
	}()
	p.Keep()
}

// DialRawConn will dial raw connection
func (p *Node) DialRawConn(channel Conn, sid uint16, uri string) (conn Conn, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	conn, err = p.Handler.DialRawConn(channel, sid, uri)
	return
}

// OnConnDialURI is on connection dial uri
func (p *Node) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	err = p.Handler.OnConnDialURI(channel, conn, parts)
	return
}

// OnConnLogin is on connection login
func (p *Node) OnConnLogin(channel Conn, args string) (name string, result xmap.M, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	name, result, err = p.Handler.OnConnLogin(channel, args)
	return
}

// OnConnClose will be called when connection is closed
func (p *Node) OnConnClose(conn Conn) (err error) {
	if p.stopping {
		return
	}
	if p.Handler != nil {
		err = p.Handler.OnConnClose(conn)
	}
	InfoLog("Node(%v) the channel(%v) is closed by %v, remove it", p.Name, conn, err)
	return nil
}

func (p *Node) OnConnJoin(channel Conn, option interface{}, result xmap.M) {
	if p.Handler != nil {
		p.Handler.OnConnJoin(channel, option, result)
	}
}

func (p *Node) OnConnNotify(channel Conn, message []byte) {
	if p.Handler != nil {
		p.Handler.OnConnNotify(channel, message)
	}
}

func (p *Node) loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify string) (config *tls.Config, err error) {
	if len(tlsCA) < 1 && len(tlsCert) < 1 && (len(tlsVerify) < 1 || tlsVerify == "1") {
		err = fmt.Errorf("at least one of tls_ca/tls_cert is required")
		return
	}
	config = &tls.Config{InsecureSkipVerify: p.Insecure}
	config.Rand = rand.Reader

	if len(tlsCert) > 0 && len(tlsKey) > 0 {
		var cert tls.Certificate
		cert, err = LoadX509KeyPair(p.Dir, tlsCert, tlsKey)
		if err != nil {
			ErrorLog("Node(%v) load cert fail with %v", p.Name, err)
			return
		}
		config.Certificates = append(config.Certificates, cert)
	}

	if len(tlsCA) > 0 {
		var certPEM []byte
		certPEM, err = LoadPEMBlock(p.Dir, tlsCA)
		if err != nil && !os.IsNotExist(err) {
			ErrorLog("Node(%v) load ca fail with %v", p.Name, err)
			return
		}
		if err == nil {
			certPool := x509.NewCertPool()
			ok := certPool.AppendCertsFromPEM([]byte(certPEM))
			if !ok {
				ErrorLog("Node(%v) append ca fail", p.Name)
				err = fmt.Errorf("load %v fail", tlsCA)
				return
			}
			config.RootCAs = certPool
		}
		err = nil
	}
	if len(tlsVerify) > 0 {
		config.InsecureSkipVerify = tlsVerify == "0"
	}
	return
}

// Keep will keep channel connection
func (p *Node) Keep() (err error) {
	for name, channel := range p.Channels {
		for {
			connected := p.CountChannel(name)
			keep := channel.IntDef(3, "keep")
			if connected >= keep {
				break
			}
			channel, _, xerr := p.Login(channel)
			if xerr != nil {
				err = xerr
				break
			}
			if channel.Name() != name {
				ErrorLog("Node(%v) keep login remote name %v must equal to local name %v", p.Name, channel.Name(), name)
				channel.Close()
				err = fmt.Errorf("remote name %v must equal to local name %v", channel.Name(), name)
				break
			}
		}
	}
	return
}

func (p *Node) dialConn(remote, proxy, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify string) (conn net.Conn, err error) {
	if !strings.Contains(remote, "://") {
		remote = "tcp://" + remote
	}
	remoteURI, xerr := url.Parse(remote)
	if xerr != nil {
		err = xerr
		return
	}
	var proxyDailer xnet.RawDialer
	if len(proxy) > 0 {
		proxyURL, xerr := url.Parse(proxy)
		if xerr != nil {
			err = xerr
			return
		}
		var proxyAuth *nproxy.Auth
		if proxyURL.User != nil {
			proxyAuth = &nproxy.Auth{}
			proxyAuth.User = proxyURL.User.Username()
			proxyAuth.Password, _ = proxyURL.User.Password()
		}
		proxyDailer, _ = nproxy.SOCKS5("tcp", proxyURL.Host, proxyAuth, Dialer)
	}
	var config *tls.Config
	switch remoteURI.Scheme {
	case "ws", "wss":
		InfoLog("Node(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey), TlsConfigShow(tlsCA), tlsVerify)
		dialer := xnet.NewWebsocketDialer()
		dialer.Dialer = Dialer
		if remoteURI.Scheme == "wss" {
			dialer.TlsConfig, err = p.loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify)
			if err != nil {
				ErrorLog("Node(%v) load tls config fail with %v", p.Name, err)
				return
			}
		}
		if proxyDailer != nil {
			dialer.Dialer = proxyDailer
		}
		var rawConn io.ReadWriteCloser
		rawConn, err = dialer.Dial(remote)
		if err == nil {
			conn = rawConn.(net.Conn)
		}
	case "tls":
		InfoLog("Node(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey), TlsConfigShow(tlsCA), tlsVerify)
		config, err = p.loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify)
		if err != nil {
			ErrorLog("Node(%v) load tls config fail with %v", p.Name, err)
			return
		}
		config.ServerName = remoteURI.Hostname()
		if proxyDailer != nil {
			rawConn, xerr := proxyDailer.Dial("tcp", remoteURI.Host)
			if xerr != nil {
				err = xerr
				return
			}
			tlsConn := tls.Client(rawConn, config)
			err = tlsConn.HandshakeContext(context.Background())
			if err != nil {
				rawConn.Close()
			}
			conn = tlsConn
		} else {
			conn, err = tls.DialWithDialer(Dialer, "tcp", remoteURI.Host, config)
		}
	case "tcp":
		InfoLog("Router(%v) start dial to %v", p.Name, remote)
		if len(proxy) > 0 {
			conn, err = proxyDailer.Dial("tcp", remoteURI.Host)
		} else {
			conn, err = Dialer.Dial("tcp", remoteURI.Host)
		}
	case "quic":
		InfoLog("Node(%v) start dial to %v by cert:%v,key:%v,ca:%v,verify:%v", p.Name, remote, TlsConfigShow(tlsCert), TlsConfigShow(tlsKey), TlsConfigShow(tlsCA), tlsVerify)
		config, err = p.loadClientConfig(tlsCert, tlsKey, tlsCA, tlsVerify)
		if err != nil {
			ErrorLog("Node(%v) load tls config fail with %v", p.Name, err)
			return
		}
		config.NextProtos = []string{"bs"}
		config.ServerName = tlsHost
		if len(config.ServerName) < 1 {
			config.ServerName = remoteURI.Hostname()
		}
		conn, err = quicDial(context.Background(), "0.0.0.0:0", remoteURI.Host, config)
	}
	return
}

// Login will add channel by local address, master address, auth token, channel index.
func (p *Node) Login(option xmap.M) (channel Conn, result xmap.M, err error) {
	var remoteAll, proxy, tlsCert, tlsKey, tlsHost, tlsVerify string
	var tlsCA = "bsrouterCA.pem"
	err = option.ValidFormat(`
		remote,R|S,L:0;
		proxy,O|S,L:0;
		tls_cert,O|S,L:0;
		tls_key,O|S,L:0;
		tls_host,O|S,L:0;
		tls_ca,O|S,L:0;
	`, &remoteAll, &proxy, &tlsCert, &tlsKey, &tlsHost, &tlsCA)
	if err != nil {
		return
	}
	tlsVerify = option.StrDef("", "tls_verify")
	for _, remote := range strings.Split(remoteAll, ",") {
		conn, xerr := p.dialConn(remote, proxy, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify)
		if xerr != nil {
			err = xerr
			WarnLog("Node(%v) dial to %v fail with %v", p.Name, remote, err)
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
func (p *Node) Ping(option xmap.M) (speed xmap.M, err error) {
	var remoteAll, proxy, tlsCert, tlsKey, tlsHost, tlsCA, tlsVerify string
	err = option.ValidFormat(`
		remote,R|S,L:0;
		proxy,O|S,L:0;
		tls_cert,O|S,L:0;
		tls_key,O|S,L:0;
		tls_host,O|S,L:0;
		tls_ca,O|S,L:0;
	`, &remoteAll, &proxy, &tlsCert, &tlsKey, &tlsHost, &tlsCA)
	if err != nil {
		return
	}
	tlsVerify = option.StrDef("", "tls_verify")
	speed = xmap.M{}
	for _, remote := range strings.Split(remoteAll, ",") {
		begin := time.Now()
		conn, xerr := p.dialConn(remote, proxy, tlsCert, tlsKey, tlsCA, tlsHost, tlsVerify)
		if xerr == nil {
			xerr = p.PingConn(NewInfoRWC(conn, conn.RemoteAddr().String()))
		}
		used := time.Since(begin)
		if xerr == nil {
			InfoLog("Node(%v) ping to %v success with %v", p.Name, remote, used)
		} else {
			WarnLog("Node(%v) ping to %v fail with %v,%v", p.Name, remote, used, xerr)
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

func (q *quicConn) Close() (err error) {
	q.Stream.Close()
	q.Stream.CancelRead(0)
	return
}

type quicListener struct {
	*quic.Listener
}

func quicListen(addr string, tls *tls.Config) (ln net.Listener, err error) {
	quicConf := &quic.Config{EnableDatagrams: true}
	quicConf.KeepAlivePeriod = time.Second
	base, err := quic.ListenAddr(addr, tls, quicConf)
	if err == nil {
		ln = &quicListener{Listener: base}
	}
	return
}

func quicDial(ctx context.Context, laddr, raddr string, tls *tls.Config) (conn net.Conn, err error) {
	c := &quicConn{}
	conn = c
	quicConf := &quic.Config{EnableDatagrams: true}
	quicConf.KeepAlivePeriod = time.Second
	localAddr, err := net.ResolveUDPAddr("udp", laddr)
	if err != nil {
		return nil, err
	}
	udpAddr, err := net.ResolveUDPAddr("udp", raddr)
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, err
	}
	if sc, _ := udpConn.SyscallConn(); sc != nil {
		sc.Control(RawConnControl)
	}
	tr := &quic.Transport{Conn: udpConn}
	c.Connection, err = tr.Dial(ctx, udpAddr, tls, quicConf)
	if err == nil {
		c.Stream, err = c.Connection.OpenStream()
	}
	return
}

func (q *quicListener) Accept() (conn net.Conn, err error) {
	c := &quicConn{}
	conn = c
	c.Connection, err = q.Listener.Accept(context.Background())
	if err == nil {
		c.Stream, err = c.Connection.AcceptStream(context.Background())
	}
	return c, err
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
