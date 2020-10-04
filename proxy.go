package bsck

import (
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
	"github.com/codingeasygo/util/xmap"
)

// //AuthOption is a pojo struct to login auth.
// type AuthOption struct {
// 	//the channel index
// 	Index int `json:"index"`
// 	//the chnnale name
// 	Name string `json:"name"`
// 	//the auth token
// 	Token string `json:"token"`
// }

// //ChannelOption is a pojo struct for adding channel to Router
// type ChannelOption struct {
// 	//enable
// 	Enable bool `json:"enable"`
// 	//the auth token
// 	Token string `json:"token"`
// 	//local tcp address to connection master
// 	Local string `json:"local"`
// 	//the remote address to login
// 	Remote string `json:"remote"`
// 	//the channel index
// 	Index int `json:"index"`
// }

//DialRawF is a function type to dial raw connection.
type DialRawF func(sid uint64, uri string) (raw Conn, err error)

//DialRaw will dial raw connection
func (d DialRawF) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	raw, err = d(sid, uri)
	return
}

//RawDialer is dialer to dial raw by uri
type RawDialer interface {
	DialRaw(sid uint64, uri string) (raw Conn, err error)
}

//NormalAcessHandler is normal access handler for proxy handler
type NormalAcessHandler struct {
	Name        string            //the access name
	LoginAccess map[string]string //the access control
	loginLocker sync.RWMutex      //the access control
	DialAccess  [][]string
	Dialer      RawDialer
}

//NewNormalAcessHandler will return new handler
func NewNormalAcessHandler(name string, dialer RawDialer) (handler *NormalAcessHandler) {
	handler = &NormalAcessHandler{
		Name:        name,
		LoginAccess: map[string]string{},
		loginLocker: sync.RWMutex{},
		Dialer:      dialer,
	}
	return
}

//DialRaw is proxy handler to dail remove
func (n *NormalAcessHandler) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	if n.Dialer == nil {
		err = fmt.Errorf("not supported")
		return
	}
	raw, err = n.Dialer.DialRaw(sid, uri)
	return
}

//OnConnLogin is proxy handler to handle login
func (n *NormalAcessHandler) OnConnLogin(channel Conn, args string) (name string, index int, result xmap.M, err error) {
	var option = xmap.M{}
	err = json.Unmarshal([]byte(args), &option)
	if err != nil {
		ErrorLog("NormalAcessHandler(%v) unmarshal login option fail with %v", n.Name, err)
		err = fmt.Errorf("parse login opiton fail with " + err.Error())
		return
	}
	var having string
	err = option.ValidFormat(`
		index,R|I,R:-1;
		name,R|S,L:0;
		token,R|S,L:0;
	`, &index, &name, &having)
	if err != nil {
		ErrorLog("NormalAcessHandler(%v) login option fail with name/token is required", n.Name)
		return
	}
	n.loginLocker.RLock()
	var mustbe string
	for access, t := range n.LoginAccess {
		reg, err := regexp.Compile(access)
		if err != nil {
			WarnLog("NormalAcessHandler(%v) compile acl name regexp(%v) fail with %v", n.Name, n, err)
			continue
		}
		if reg.MatchString(name) {
			mustbe = t
		}
	}
	n.loginLocker.RUnlock()
	if len(mustbe) < 1 || having != mustbe {
		WarnLog("NormalAcessHandler(%v) login %v fail with auth fail, expect %v, but %v", n.Name, name, mustbe, having)
		err = fmt.Errorf("access denied ")
		return
	}
	InfoLog("NormalAcessHandler(%v) channel %v login success on %v ", n.Name, name, channel)
	channel.Context()["option"] = option
	return
}

//OnConnDialURI is proxy handler to handle dial uri
func (n *NormalAcessHandler) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	_, islogin := channel.Context()["option"]
	if !islogin {
		err = fmt.Errorf("not login")
		return
	}
	name := channel.Name()
	for _, entry := range n.DialAccess {
		if len(entry) != 2 {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with entry must be [<source>,<target>], but %v", n.Name, entry)
			continue
		}
		source, xerr := regexp.Compile(entry[0])
		if xerr != nil {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with %v by entry source %v", n.Name, xerr, entry[0])
			continue
		}
		if !source.MatchString(name) {
			continue
		}
		target, xerr := regexp.Compile(entry[1])
		if xerr != nil {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with %v by entry target %v", n.Name, xerr, entry[1])
			continue
		}
		if target.MatchString(name) {
			return nil
		}
	}
	err = fmt.Errorf("not access")
	return
}

//OnConnClose is proxy handler when connection is closed
func (n *NormalAcessHandler) OnConnClose(conn Conn) (err error) {
	return nil
}

//OnConnJoin is proxy handler when channel join
func (n *NormalAcessHandler) OnConnJoin(channel *Channel, option, result xmap.M) (err error) {
	return
}

//NoneHandler is proxy handler
type NoneHandler struct {
}

//NewNoneHandler will return new NoneHandler
func NewNoneHandler() (handler *NoneHandler) {
	handler = &NoneHandler{}
	return
}

//DialRaw will dial raw connection by uri
func (n *NoneHandler) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	err = fmt.Errorf("not supported")
	return
}

//OnConnLogin is event on connection login
func (n *NoneHandler) OnConnLogin(channel Conn, args string) (name string, index int, result xmap.M, err error) {
	err = fmt.Errorf("not supported")
	return
}

//OnConnDialURI is event on connection dial to remote
func (n *NoneHandler) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	err = fmt.Errorf("not supported")
	return
}

//OnConnClose is event on connection close
func (n *NoneHandler) OnConnClose(conn Conn) (err error) {
	return
}

//OnConnJoin is event on channel join
func (n *NoneHandler) OnConnJoin(channel *Channel, option, result xmap.M) (err error) {
	return
}

//ProxyHandler is proxy handler
type ProxyHandler interface {
	//DialRaw will dial raw connection by uri
	DialRaw(sid uint64, uri string) (raw Conn, err error)
	//OnConnLogin is event on connection login
	OnConnLogin(channel Conn, args string) (name string, index int, result xmap.M, err error)
	//OnConnDialURI is event on connection dial to remote
	OnConnDialURI(channel Conn, conn string, parts []string) (err error)
	//OnConnClose is event on connection close
	OnConnClose(conn Conn) (err error)
	//OnConnJoin is event on channel join
	OnConnJoin(channel *Channel, option, result xmap.M) (err error)
}

//ForwardEntry is the forward entry
type ForwardEntry []interface{}

//Proxy is an implementation of proxy router
type Proxy struct {
	*Router                      //the router
	Running        bool          //proxy is running.
	ReconnectDelay time.Duration //reconnect delay
	Cert           string        //the tls cert
	Key            string        //the tls key
	master         net.Listener
	forwards       map[string]ForwardEntry
	forwardsLck    sync.RWMutex
	Handler        ProxyHandler
}

//NewProxy will return new Proxy by name
func NewProxy(name string, handler ProxyHandler) (proxy *Proxy) {
	proxy = &Proxy{
		Router:         NewRouter(name),
		forwards:       map[string]ForwardEntry{},
		forwardsLck:    sync.RWMutex{},
		Handler:        handler,
		Running:        true,
		ReconnectDelay: 3 * time.Second,
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
		p.Router.Accept(NewInfoRWC(frame.NewReadWriteCloser(conn, p.BufferSize), conn.RemoteAddr().String()))
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
	InfoLog("Proxy(%v) %p is closing", p.Name, p)
	p.Running = false
	if p.master != nil {
		err = p.master.Close()
		InfoLog("Proxy(%v) master is closed", p.Name)
	}
	p.forwardsLck.RLock()
	for key, f := range p.forwards {
		f[0].(net.Listener).Close()
		InfoLog("Proxy(%v) forwad %v is closed", p.Name, key)
	}
	p.forwardsLck.RUnlock()
	p.Router.Close()
	InfoLog("Proxy(%v) router is closed", p.Name)
	return
}

func (p *Proxy) runReconnect(args xmap.M) {
	for p.Running {
		_, _, err := p.Login(args)
		if err == nil {
			break
		}
		time.Sleep(p.ReconnectDelay)
	}
}

//DialRaw will dial raw connection
func (p *Proxy) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	raw, err = p.Handler.DialRaw(sid, uri)
	return
}

//OnConnDialURI is on connection dial uri
func (p *Proxy) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	err = p.Handler.OnConnDialURI(channel, conn, parts)
	return
}

//OnConnLogin is on connection login
func (p *Proxy) OnConnLogin(channel Conn, args string) (name string, index int, result xmap.M, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	name, index, result, err = p.Handler.OnConnLogin(channel, args)
	return
}

//OnConnClose will be called when connection is closed
func (p *Proxy) OnConnClose(conn Conn) (err error) {
	if !p.Running {
		return
	}
	channel, ok := conn.(*Channel)
	if !p.Running || !ok {
		return
	}
	context := channel.Context()
	if p.Handler != nil {
		err = p.Handler.OnConnClose(conn)
	}
	if err == nil && context.IntDef(-1, "login_conn") == 1 {
		go p.runReconnect(context.Map("option"))
		InfoLog("Proxy(%v) the chnnale(%v) is closed, will reconect it", p.Name, channel)
	} else {
		InfoLog("Proxy(%v) the chnnale(%v) is closed by %v, remove it", p.Name, channel, err)
	}
	return nil
}

//LoginChannel will login all channel by options.
func (p *Proxy) LoginChannel(reconnect bool, channels ...xmap.M) (err error) {
	for _, channel := range channels {
		if channel.Int("enable") < 1 {
			continue
		}
		_, _, err = p.Login(channel)
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
func (p *Proxy) Login(option xmap.M) (channel *Channel, result xmap.M, err error) {
	var index int
	var local, remote, tlsCert, tlsKey string
	err = option.ValidFormat(`
		index,R|I,R:-1;
		local,O|S,L:0;
		remote,R|S,L:0;
		tls_cert,O|S,L:0;
		tls_key,O|S,L:0;
	`, &index, &local, &remote, &tlsCert, &tlsKey)
	if err != nil {
		return
	}
	var dialer net.Dialer
	if len(local) > 0 {
		dialer.LocalAddr, err = net.ResolveTCPAddr("tcp", local)
		if err != nil {
			return
		}
	}
	var conn net.Conn
	if len(tlsCert) > 0 {
		InfoLog("Proxy(%v) start dial to %v by x509 cert:%v,key:%v", p.Name, remote, tlsCert, tlsKey)
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			ErrorLog("Proxy(%v) load cert fail with %v", p.Name, err)
			return
		}
		config := &tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
		config.Rand = rand.Reader
		conn, err = tls.DialWithDialer(&dialer, "tcp", remote, config)
	} else {
		InfoLog("Router(%v) start dial to %v", p.Name, remote)
		conn, err = dialer.Dial("tcp", remote)
	}
	if err != nil {
		WarnLog("Proxy(%v) dial to %v fail with %v", p.Name, remote, err)
		return
	}
	auth := xmap.M{}
	for key, val := range option {
		auth[key] = val
	}
	auth["index"] = index
	auth["name"] = p.Name
	channel, result, err = p.JoinConn(NewInfoRWC(frame.NewReadWriteCloser(conn, p.BufferSize), conn.RemoteAddr().String()), index, auth)
	if err == nil {
		channel.Context()["option"] = option
		channel.Context()["login_conn"] = 1
		p.Handler.OnConnJoin(channel, option, result)
	}
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
