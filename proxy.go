package bsck

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/util/xio/frame"
)

//AuthOption is a pojo struct to login auth.
type AuthOption struct {
	//the channel index
	Index int `json:"index"`
	//the chnnale name
	Name string `json:"name"`
	//the auth token
	Token string `json:"token"`
}

//ChannelOption is a pojo struct for adding channel to Router
type ChannelOption struct {
	//enable
	Enable bool `json:"enable"`
	//the auth token
	Token string `json:"token"`
	//local tcp address to connection master
	Local string `json:"local"`
	//the remote address to login
	Remote string `json:"remote"`
	//the channel index
	Index int `json:"index"`
}

//DialRawF is a function type to dial raw connection.
type DialRawF func(sid uint64, uri string) (raw Conn, err error)

//OnConnClose will be called when connection is closed
func (d DialRawF) OnConnClose(conn Conn) error {
	return nil
}

//DialRaw will dial raw connection
func (d DialRawF) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	raw, err = d(sid, uri)
	return
}

//BufferConn is an implementation of buffer connecton
type BufferConn struct {
	*bufio.Reader                    //the buffer reader.
	io.Writer                        //the writer.
	Raw           io.ReadWriteCloser //the raw connection
}

//NewBufferConn will return new BufferConn
func NewBufferConn(raw io.ReadWriteCloser, bufferSize int) (buf *BufferConn) {
	buf = &BufferConn{
		Reader: bufio.NewReaderSize(raw, bufferSize),
		Writer: raw,
		Raw:    raw,
	}
	return
}

//Close will close raw.
func (b *BufferConn) Close() (err error) {
	err = b.Raw.Close()
	return
}

func (b *BufferConn) String() string {
	if conn, ok := b.Raw.(net.Conn); ok {
		return conn.RemoteAddr().String()
	}
	return fmt.Sprintf("%v", b.Raw)
}

//ProxyHandler is proxy handler
type ProxyHandler interface {
	//dial raw connection
	DialRaw(sid uint64, uri string) (raw Conn, err error)
	//on connection is closed
	OnConnClose(conn Conn) error
}

//ForwardEntry is the forward entry
type ForwardEntry []interface{}

//Proxy is an implementation of proxy router
type Proxy struct {
	*Router           //the router
	PerConnBufferSize int
	Running           bool              //proxy is running.
	ReconnectDelay    time.Duration     //reconnect delay
	Cert              string            //the tls cert
	Key               string            //the tls key
	ACL               map[string]string //the access control
	aclLock           sync.RWMutex      //the access control
	master            net.Listener
	forwards          map[string]ForwardEntry
	forwardsLck       sync.RWMutex
	Handler           ProxyHandler
}

//NewProxy will return new Proxy by name
func NewProxy(name string) (proxy *Proxy) {
	proxy = &Proxy{
		Router:            NewRouter(name),
		PerConnBufferSize: 1024,
		forwards:          map[string]ForwardEntry{},
		forwardsLck:       sync.RWMutex{},
		Handler:           nil,
		Running:           true,
		ReconnectDelay:    3 * time.Second,
		ACL:               map[string]string{},
		aclLock:           sync.RWMutex{},
	}
	proxy.Router.Handler = proxy
	return
}

//ListenMaster will listen master router on address
func (p *Proxy) ListenMaster(addr string) (err error) {
	if len(p.Cert) > 0 {
		InfoLog("Proxy(%v) load x509 cert:%v,key:%v", p.Name, p.Cert, p.Key)
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(p.Cert, p.Key)
		if err != nil {
			ErrorLog("Proxy(%v) load cert fail with %v", p.Name, err)
			return
		}
		config := &tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
		config.Rand = rand.Reader
		p.master, err = tls.Listen("tcp", addr, config)
	} else {
		p.master, err = net.Listen("tcp", addr)
	}
	if err == nil {
		go p.loopMaster(p.master)
		InfoLog("Proxy(%v) listen master on %v", p.Name, addr)
	}
	return
}

//StartForward will forward address to uri
func (p *Proxy) StartForward(name string, listen *url.URL, router string) (listener net.Listener, err error) {
	// target, err := url.Parse(listen)
	// if err != nil {
	// 	return
	// }
	p.forwardsLck.Lock()
	defer p.forwardsLck.Unlock()
	if p.forwards[name] != nil || len(name) < 1 {
		err = fmt.Errorf("the name(%v) is already used", name)
		InfoLog("Proxy(%v) start forward by %v->%v fail with %v", p.Name, listen, router, err)
		return
	}
	switch listen.Scheme {
	case "socks":
		sp := NewSocksProxy()
		sp.Dialer = func(utype int, uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
			sid, err = p.Dial(strings.Replace(router, "${HOST}", uri, -1), raw)
			return
		}
		err = sp.Start(listen.Host)
		if err == nil {
			listener = sp
			p.forwards[name] = []interface{}{sp, listen, router}
			InfoLog("Proxy(%v) start socket forward on %v success by %v->%v", p.Name, listener.Addr(), listen, router)
		}
	default:
		listener, err = net.Listen(listen.Scheme, listen.Host)
		if err == nil {
			p.forwards[name] = []interface{}{listener, listen, router}
			go p.loopForward(listener, name, listen, router)
			InfoLog("Proxy(%v) start tcp forward on %v success by %v->%v", p.Name, listener.Addr(), listen, router)
		}
	}
	return
}

//StopForward will forward address to uri
func (p *Proxy) StopForward(name string) (err error) {
	InfoLog("Proxy(%v) stop forward by name:%v", p.Name, name)
	p.forwardsLck.Lock()
	vals := p.forwards[name]
	delete(p.forwards, name)
	p.forwardsLck.Unlock()
	if len(vals) > 0 {
		err = vals[0].(io.Closer).Close()
	}
	return
}

func (p *Proxy) loopMaster(l net.Listener) {
	var err error
	var conn net.Conn
	for p.Running {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		DebugLog("Proxy(%v) master accepting connection from %v", p.Name, conn.RemoteAddr())
		p.Router.Accept(frame.NewReadWriteCloser(conn, 4096))
	}
	l.Close()
	InfoLog("Proxy(%v) master aceept on %v is stopped", p.Name, l.Addr())
}

func (p *Proxy) loopForward(l net.Listener, name string, listen *url.URL, uri string) {
	var err error
	var sid uint64
	var conn net.Conn
	InfoLog("Proxy(%v) proxy forward(%v->%v) aceept runner is starting", p.Name, l.Addr(), uri)
	for p.Running {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		DebugLog("Proxy(%v) accepting forward(%v->%v) connection from %v", p.Name, l.Addr(), uri, conn.RemoteAddr())
		sid, err = p.Dial(uri, conn)
		if err == nil {
			DebugLog("Proxy(%v) proxy forward(%v->%v) success on session(%v)", p.Name, l.Addr(), uri, sid)
		} else {
			WarnLog("Proxy(%v) proxy forward(%v->%v) fail with %v", p.Name, l.Addr(), uri, err)
			conn.Close()
		}
	}
	l.Close()
	InfoLog("Proxy(%v) proxy forward(%v->%v) aceept runner is stopped", p.Name, l.Addr(), uri)
	p.forwardsLck.Lock()
	delete(p.forwards, name)
	p.forwardsLck.Unlock()
}

//Close will close the tcp listen
func (p *Proxy) Close() (err error) {
	p.Running = false
	if p.master != nil {
		err = p.master.Close()
	}
	p.forwardsLck.RLock()
	for _, f := range p.forwards {
		f[0].(net.Listener).Close()
	}
	p.forwardsLck.RUnlock()
	p.Router.Close()
	return
}

func (p *Proxy) runReconnect(args *ChannelOption) {
	for p.Running {
		err := p.Login(args)
		if err == nil {
			break
		}
		time.Sleep(p.ReconnectDelay)
	}
}

//DialRaw will dial raw connection
func (p *Proxy) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	raw, err = p.Handler.DialRaw(sid, uri)
	return
}

//WhetherConnLogin is check connection is login
func (p *Proxy) WhetherConnLogin(channel Conn) (ok bool) {
	_, ok = channel.Context().(*AuthOption)
	return
}

//OnConnDialURI is on connection dial uri
func (p *Proxy) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	return
}

//OnConnLogin is on connection login
func (p *Proxy) OnConnLogin(channel Conn, args string) (name string, index int, err error) {
	var option AuthOption
	err = json.Unmarshal([]byte(args), &option)
	if err != nil {
		ErrorLog("Proxy(%v) unmarshal login option fail with %v", p.Name, err)
		err = fmt.Errorf("parse login opiton fail with " + err.Error())
		return
	}
	if len(option.Name) < 1 || len(option.Token) < 1 {
		ErrorLog("Proxy(%v) login option fail with name/token is required", p.Name)
		err = fmt.Errorf("name/token is requried")
		return
	}
	p.aclLock.RLock()
	var token string
	for n, t := range p.ACL {
		reg, err := regexp.Compile(n)
		if err != nil {
			WarnLog("Proxy(%v) compile acl name regexp(%v) fail with %v", p.Name, n, err)
			continue
		}
		if reg.MatchString(option.Name) {
			token = t
		}
	}
	p.aclLock.RUnlock()
	if len(token) < 1 || token != option.Token {
		WarnLog("Proxy(%v) login fail with auth fail", p.Name)
		err = fmt.Errorf("access denied ")
	}
	channel.SetContext(option)
	return
}

//OnConnClose will be called when connection is closed
func (p *Proxy) OnConnClose(conn Conn) (err error) {
	channel, ok := conn.(*Channel)
	if !p.Running || !ok {
		return
	}
	option, ok := channel.LoginArgs.(*ChannelOption)
	if !p.Running || !ok {
		return
	}
	InfoLog("Proxy(%v) the chnnale(%v) is closed, will reconect it", p.Name, channel)
	go p.runReconnect(option)
	return nil
}

//LoginChannel will login all channel by options.
func (p *Proxy) LoginChannel(reconnect bool, channels ...*ChannelOption) (err error) {
	for _, channel := range channels {
		if !channel.Enable {
			continue
		}
		err = p.Login(channel)
		if err == nil {
			continue
		}
		if reconnect {
			go p.runReconnect(channel)
		} else {
			return
		}
	}
	return
}

//Login will add channel by local address, master address, auth token, channel index.
func (p *Proxy) Login(option *ChannelOption) (err error) {
	var dialer net.Dialer
	if len(option.Local) > 0 {
		dialer.LocalAddr, err = net.ResolveTCPAddr("tcp", option.Local)
		if err != nil {
			return
		}
	}
	var conn net.Conn
	if len(p.Cert) > 0 {
		InfoLog("Proxy(%v) start dial to %v by x509 cert:%v,key:%v", p.Name, option.Remote, p.Cert, p.Key)
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(p.Cert, p.Key)
		if err != nil {
			ErrorLog("Proxy(%v) load cert fail with %v", p.Name, err)
			return
		}
		config := &tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
		config.Rand = rand.Reader
		conn, err = tls.DialWithDialer(&dialer, "tcp", option.Remote, config)
	} else {
		InfoLog("Router(%v) start dial to %v", p.Name, option.Remote)
		conn, err = dialer.Dial("tcp", option.Remote)
	}
	if err != nil {
		WarnLog("Proxy(%v) dial to %v fail with %v", p.Name, option.Remote, err)
		return
	}
	auth := &AuthOption{
		Index: option.Index,
		Name:  p.Name,
		Token: option.Token,
	}
	err = p.JoinConn(NewInfoRWC(frame.NewReadWriteCloser(conn, p.PerConnBufferSize), conn.RemoteAddr().String()), option.Index, auth)
	return
}

//InfoRWC is external ReadWriteCloser to get info to String
type InfoRWC struct {
	frame.ReadWriteCloser
	Info string
}

//NewInfoRWC will return new nfoRWC
func NewInfoRWC(raw frame.ReadWriteCloser, info string) *InfoRWC {
	return &InfoRWC{ReadWriteCloser: raw, Info: info}
}

func (i *InfoRWC) String() string {
	return i.Info
}
