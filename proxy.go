package bsck

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"
)

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
	Handler        RouterHandler
}

//NewProxy will return new Proxy by name
func NewProxy(name string) (proxy *Proxy) {
	proxy = &Proxy{
		Router:         NewRouter(name),
		forwards:       map[string]ForwardEntry{},
		forwardsLck:    sync.RWMutex{},
		Handler:        NewTCPDialer(),
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
		sp.Dialer = func(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
			sid, err = p.Dial(router+"->tcp://"+uri, raw)
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
		p.Router.Accept(NewBufferConn(conn, 4096))
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

func (p *Proxy) runReconnect(option *ChannelOption) {
	for p.Running {
		err := p.Login(option)
		if err == nil {
			break
		}
		time.Sleep(p.ReconnectDelay)
	}
}

//OnConnClose will be called when connection is closed
func (p *Proxy) OnConnClose(conn Conn) error {
	if channel, ok := conn.(*Channel); ok && p.Running && channel.Option != nil && channel.Option.Remote != "" {
		InfoLog("Proxy(%v) the chnnale(%v) is closed, will reconect it", p.Name, channel)
		go p.runReconnect(channel.Option)
	}
	return nil
}

//DialRaw will dial raw connection
func (p *Proxy) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	raw, err = p.Handler.DialRaw(sid, uri)
	return
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
		InfoLog("Router(%v) start dial to %v by x509 cert:%v,key:%v", p.Name, option.Remote, p.Cert, p.Key)
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
	err = p.JoinConn(NewInfoRWC(conn, conn.RemoteAddr().String()), option)
	return
}

//InfoRWC is external ReadWriteCloser to get info to String
type InfoRWC struct {
	io.ReadWriteCloser
	Info string
}

//NewInfoRWC will return new nfoRWC
func NewInfoRWC(raw io.ReadWriteCloser, info string) *InfoRWC {
	return &InfoRWC{ReadWriteCloser: raw, Info: info}
}

func (i *InfoRWC) String() string {
	return i.Info
}
