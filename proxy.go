package bsck

import (
	"bufio"
	"io"
	"net"
)

type BufferConn struct {
	*bufio.Reader
	io.Writer
	Raw net.Conn
}

func NewBufferConn(raw net.Conn, bufferSize int) (buf *BufferConn) {
	buf = &BufferConn{
		Reader: bufio.NewReaderSize(raw, bufferSize),
		Writer: raw,
		Raw:    raw,
	}
	return
}

func (b *BufferConn) Close() (err error) {
	err = b.Raw.Close()
	return
}

type Proxy struct {
	*Router
	listener net.Listener
}

func NewProxy(name string) (proxy *Proxy) {
	proxy = &Proxy{
		Router: NewRouter(name),
	}
	return
}

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

func (p *Proxy) Close() (err error) {
	err = p.listener.Close()
	return
}

// func (p *Proxy)
