package bsck

import (
	"bufio"
	"io"
	"net"
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

type forwad []interface{}

//Proxy is an implementation of proxy router
type Proxy struct {
	*Router             //the router
	Running        bool //proxy is running.
	ReconnectDelay time.Duration
	master         net.Listener
	forwards       map[string]forwad
	forwardsLck    sync.RWMutex
	Handler        RouterHandler
}

//NewProxy will return new Proxy by name
func NewProxy(name string) (proxy *Proxy) {
	proxy = &Proxy{
		Router:         NewRouter(name),
		forwards:       map[string]forwad{},
		forwardsLck:    sync.RWMutex{},
		Handler:        NewTCPDailer(),
		Running:        true,
		ReconnectDelay: 3 * time.Second,
	}
	proxy.Router.Handler = proxy
	return
}

//ListenMaster will listen master router on address
func (p *Proxy) ListenMaster(addr string) (err error) {
	p.master, err = net.Listen("tcp", addr)
	if err == nil {
		go p.loopMaster(p.master)
		infoLog("Proxy(%v) listen master on %v", p.Name, addr)
	}
	return
}

//StartForward will forward address to uri
func (p *Proxy) StartForward(addr, uri string) (err error) {
	listener, err := net.Listen("tcp", addr)
	if err == nil {
		p.forwardsLck.Lock()
		p.forwards[listener.Addr().String()] = []interface{}{listener, addr, uri}
		p.forwardsLck.Unlock()
		go p.loopForward(listener, uri)
		infoLog("Proxy(%v) start forward by %v->%v", p.Name, addr, uri)
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
		debugLog("Proxy(%v) master accepting connection from %v", p.Name, conn.RemoteAddr())
		p.Router.Accept(NewBufferConn(conn, 4096))
	}
	l.Close()
	infoLog("Proxy(%v) master aceept on %v is stopped", p.Name, l.Addr())
}

func (p *Proxy) loopForward(l net.Listener, uri string) {
	var err error
	var sid uint64
	var conn net.Conn
	for p.Running {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		debugLog("Proxy(%v) accepting forward(%v->%v) connection from %v", p.Name, l.Addr(), uri, conn.RemoteAddr())
		sid, err = p.Dial(uri, conn)
		if err == nil {
			debugLog("Proxy(%v) proxy forward(%v->%v) success on session(%v)", p.Name, l.Addr(), uri, sid)
		} else {
			warnLog("Proxy(%v) proxy forward(%v->%v) fail with %v", p.Name, l.Addr(), uri, err)
			conn.Close()
		}
	}
	l.Close()
	infoLog("Proxy(%v) proxy forward(%v->%v) aceept runner is stopped", p.Name, l.Addr(), uri)
	p.forwardsLck.Lock()
	delete(p.forwards, l.Addr().String())
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
		infoLog("Proxy(%v) the chnnale(%v) is closed, will reconect it", p.Name, channel)
		go p.runReconnect(channel.Option)
	}
	return nil
}

//DialRaw will dial raw connection
func (p *Proxy) DialRaw(sid uint64, uri string) (raw Conn, err error) {
	raw, err = p.Handler.DialRaw(sid, uri)
	return
}
