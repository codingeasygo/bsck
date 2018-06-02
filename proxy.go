package bsck

import (
	"bufio"
	"io"
	"net"
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

//Proxy is an implementation of proxy router
type Proxy struct {
	*Router  //the router
	listener net.Listener
}

//NewProxy will return new Proxy by name
func NewProxy(name string) (proxy *Proxy) {
	proxy = &Proxy{
		Router: NewRouter(name),
	}
	return
}

//Listen will proxy router on address
func (p *Proxy) Listen(addr string) (err error) {
	p.listener, err = net.Listen("tcp", addr)
	if err == nil {
		go p.loopAccept(p.listener)
	}
	return
}

func (p *Proxy) loopAccept(l net.Listener) {
	var err error
	var conn net.Conn
	for {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		debugLog("Proxy accepting connection from %v", conn.RemoteAddr())
		p.Router.Accept(NewBufferConn(conn, 4096))
	}
	infoLog("Proxy aceept on %v is stopped", l.Addr())
}

//Close will close the tcp listen
func (p *Proxy) Close() (err error) {
	err = p.listener.Close()
	return
}
