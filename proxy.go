package bsck

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/util/xnet"
	"golang.org/x/net/websocket"
)

// //AuthOption is a struct struct to login auth.
// type AuthOption struct {
// 	//the channel index
// 	Index int `json:"index"`
// 	//the channel name
// 	Name string `json:"name"`
// 	//the auth token
// 	Token string `json:"token"`
// }

// //ChannelOption is a struct struct for adding channel to Router
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
type DialRawF func(channel Conn, sid uint64, uri string) (raw Conn, err error)

//DialRaw will dial raw connection
func (d DialRawF) DialRaw(channel Conn, sid uint64, uri string) (raw Conn, err error) {
	raw, err = d(channel, sid, uri)
	return
}

//RawDialer is dialer to dial raw by uri
type RawDialer interface {
	DialRaw(channel Conn, sid uint64, uri string) (raw Conn, err error)
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

//DialRaw is proxy handler to dial remove
func (n *NormalAcessHandler) DialRaw(channel Conn, sid uint64, uri string) (raw Conn, err error) {
	if n.Dialer == nil {
		err = fmt.Errorf("not supported")
		return
	}
	raw, err = n.Dialer.DialRaw(channel, sid, uri)
	return
}

//OnConnLogin is proxy handler to handle login
func (n *NormalAcessHandler) OnConnLogin(channel Conn, args string) (name string, index int, result xmap.M, err error) {
	var option = xmap.M{}
	err = json.Unmarshal([]byte(args), &option)
	if err != nil {
		ErrorLog("NormalAcessHandler(%v) unmarshal login option fail with %v", n.Name, err)
		err = fmt.Errorf("parse login option fail with " + err.Error())
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
	var matched string
	for key, val := range n.LoginAccess {
		keyPattern, err := regexp.Compile(key)
		if err != nil {
			WarnLog("NormalAcessHandler(%v) compile acl key regexp(%v) fail with %v", n.Name, key, err)
			continue
		}
		valPattern, err := regexp.Compile(val)
		if err != nil {
			WarnLog("NormalAcessHandler(%v) compile acl token regexp(%v) fail with %v", n.Name, val, err)
			continue
		}
		if keyPattern.MatchString(name) && valPattern.MatchString(having) {
			matched = key
			break
		}
	}
	n.loginLocker.RUnlock()
	if len(matched) < 1 {
		WarnLog("NormalAcessHandler(%v) login %v/%v fail with auth fail", n.Name, name, having)
		err = fmt.Errorf("access denied ")
		return
	}
	InfoLog("NormalAcessHandler(%v) channel %v login success on %v ", n.Name, name, channel)
	channel.Context()["option"] = option
	return
}

//OnConnDialURI is proxy handler to handle dial uri
func (n *NormalAcessHandler) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	_, isLogin := channel.Context()["option"]
	if !isLogin {
		err = fmt.Errorf("not login")
		return
	}
	name := channel.Name()
	for _, entry := range n.DialAccess {
		if len(entry) != 2 {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with entry must be [<source>,<target>], but %v", n.Name, entry)
			continue
		}
		source, sourceError := regexp.Compile(entry[0])
		if sourceError != nil {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with %v by entry source %v", n.Name, sourceError, entry[0])
			continue
		}
		if !source.MatchString(name) {
			continue
		}
		target, targetError := regexp.Compile(entry[1])
		if targetError != nil {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with %v by entry target %v", n.Name, targetError, entry[1])
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
func (n *NoneHandler) DialRaw(channel Conn, sid uint64, uri string) (raw Conn, err error) {
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
	DialRaw(channel Conn, sid uint64, uri string) (raw Conn, err error)
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
	Dir            string        //the work dir
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
	var tlsConfig *tls.Config
	if len(p.Cert) > 0 {
		InfoLog("Proxy(%v) load x509 cert:%v,key:%v", p.Name, p.Cert, p.Key)
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(p.Cert, p.Key)
		if err != nil {
			ErrorLog("Proxy(%v) load cert fail with %v", p.Name, err)
			return
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
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
		go server.Serve(p.master)
		InfoLog("Proxy(%v) listen web master on %v", p.Name, addr)
	default:
		go p.loopMaster(p.master)
		InfoLog("Proxy(%v) listen tcp master on %v", p.Name, addr)
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
		sp := socks.NewServer()
		sp.Dialer = xio.PiperDialerF(func(uri string, bufferSize int) (raw xio.Piper, err error) {
			raw, err = p.DialPiper(strings.Replace(router, "${HOST}", uri, -1), bufferSize)
			return
		})
		listener, err = sp.Start(listen.Host)
		if err == nil {
			p.forwards[name] = []interface{}{listener, listen, router}
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
	forward := p.forwards[name]
	delete(p.forwards, name)
	p.forwardsLck.Unlock()
	if len(forward) > 0 {
		err = forward[0].(io.Closer).Close()
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
		p.Router.Accept(NewInfoRWC(conn, frame.NewReadWriteCloser(conn, p.BufferSize), conn.RemoteAddr().String()))
	}
	l.Close()
	InfoLog("Proxy(%v) master accept on %v is stopped", p.Name, l.Addr())
}

func (p *Proxy) AcceptWsConn(conn *websocket.Conn) {
	DebugLog("Proxy(%v) master accepting connection from %v", p.Name, conn.RemoteAddr())
	p.Router.AcceptSync(NewInfoRWC(conn, frame.NewReadWriteCloser(conn, p.BufferSize), conn.RemoteAddr().String()))
}

func (p *Proxy) loopForward(l net.Listener, name string, listen *url.URL, uri string) {
	var err error
	var sid uint64
	var conn net.Conn
	InfoLog("Proxy(%v) proxy forward(%v->%v) accept runner is starting", p.Name, l.Addr(), uri)
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
	InfoLog("Proxy(%v) proxy forward(%v->%v) accept runner is stopped", p.Name, l.Addr(), uri)
	p.forwardsLck.Lock()
	delete(p.forwards, name)
	p.forwardsLck.Unlock()
}

//Close will close the tcp listen
func (p *Proxy) Close() (err error) {
	InfoLog("Proxy(%v) is closing", p.Name)
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
func (p *Proxy) DialRaw(channel Conn, sid uint64, uri string) (raw Conn, err error) {
	if p.Handler == nil {
		err = fmt.Errorf("not supported")
		return
	}
	raw, err = p.Handler.DialRaw(channel, sid, uri)
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
		InfoLog("Proxy(%v) the channel(%v) is closed, will reconnect it", p.Name, channel)
	} else {
		InfoLog("Proxy(%v) the channel(%v) is closed by %v, remove it", p.Name, channel, err)
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
	var conn net.Conn
	if strings.HasPrefix(remote, "ws://") || strings.HasPrefix(remote, "wss://") {
		var rawConn io.ReadWriteCloser
		dialer := xnet.NewWebsocketDialer()
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
		if len(tlsCert) > 0 {
			if !filepath.IsAbs(tlsCert) {
				tlsCert = filepath.Join(p.Dir, tlsCert)
			}
			if !filepath.IsAbs(tlsKey) {
				tlsKey = filepath.Join(p.Dir, tlsKey)
			}
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
	channel, result, err = p.JoinConn(NewInfoRWC(conn, frame.NewReadWriteCloser(conn, p.BufferSize), conn.RemoteAddr().String()), index, auth)
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
	Raw  interface{}
	Info string
}

//NewInfoRWC will return new nfoRWC
func NewInfoRWC(raw interface{}, rwc frame.ReadWriteCloser, info string) *InfoRWC {
	return &InfoRWC{Raw: raw, ReadWriteCloser: rwc, Info: info}
}

func (i *InfoRWC) RawValue() interface{} {
	return i.Raw
}

func (i *InfoRWC) String() string {
	return i.Info
}

//EncodeWebURI will replace string in () as base64 encoding
func EncodeWebURI(format string, args ...interface{}) string {
	return regexp.MustCompile(`\([^\\)]*\)`).ReplaceAllStringFunc(fmt.Sprintf(format, args...), func(having string) string {
		having = strings.Trim(having, "()")
		return "base64-" + base64.RawURLEncoding.EncodeToString([]byte(having))
	})
}
