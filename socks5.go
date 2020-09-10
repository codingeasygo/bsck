package bsck

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"

	"github.com/codingeasygo/util/xio"
)

//Codable is interface for get current code
type Codable interface {
	Code() byte
}

//PendingConn is an implementation of io.ReadWriteCloser
type PendingConn struct {
	Raw     io.ReadWriteCloser
	pending uint32
	wc      chan int
}

//NewPendingConn will return new endingConn
func NewPendingConn(raw io.ReadWriteCloser) (conn *PendingConn) {
	conn = &PendingConn{
		Raw:     raw,
		pending: 1,
		wc:      make(chan int),
	}
	return
}

//Start pending connection
func (p *PendingConn) Start() {
	if atomic.CompareAndSwapUint32(&p.pending, 1, 0) {
		close(p.wc)
	}
}

func (p *PendingConn) Write(b []byte) (n int, err error) {
	if p.pending == 1 {
		<-p.wc
	}
	n, err = p.Raw.Write(b)
	return
}

func (p *PendingConn) Read(b []byte) (n int, err error) {
	if p.pending == 1 {
		<-p.wc
	}
	n, err = p.Raw.Read(b)
	return
}

//Close pending connection.
func (p *PendingConn) Close() (err error) {
	if atomic.CompareAndSwapUint32(&p.pending, 1, 0) {
		close(p.wc)
	}
	err = p.Raw.Close()
	return
}

const (
	SocksUriTypeNormal = 0
	SocksUriTypeBS     = 1
)

//SocksProxy is an implementation of socks5 proxy
type SocksProxy struct {
	net.Listener
	Dialer func(utype int, uri string, raw io.ReadWriteCloser) (sid uint64, err error)
}

//NewSocksProxy will return new SocksProxy
func NewSocksProxy() (socks *SocksProxy) {
	socks = &SocksProxy{}
	return
}

//Start proxy listener
func (s *SocksProxy) Start(addr string) (err error) {
	s.Listener, err = net.Listen("tcp", addr)
	if err == nil {
		InfoLog("SocksProxy listen socks5 proxy on %v", addr)
		go s.loopAccept(s.Listener)
	}
	return
}

func (s *SocksProxy) loopAccept(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			break
		}
		go s.procConn(conn)
	}
}

func (s *SocksProxy) procConn(conn net.Conn) {
	var err error
	DebugLog("SocksProxy proxy connection from %v", conn.RemoteAddr())
	defer func() {
		if err != nil {
			DebugLog("SocksProxy proxy connection from %v is done with %v", conn.RemoteAddr(), err)
			conn.Close()
		}
	}()
	buf := make([]byte, 1024*64)
	//
	//Procedure method
	err = xio.FullBuffer(conn, buf, 2, nil)
	if err != nil {
		return
	}
	if buf[0] != 0x05 {
		err = fmt.Errorf("only ver 0x05 is supported, but %x", buf[0])
		return
	}
	err = xio.FullBuffer(conn, buf[2:], uint32(buf[1]), nil)
	if err != nil {
		return
	}
	_, err = conn.Write([]byte{0x05, 0x00})
	if err != nil {
		return
	}
	//
	//Procedure request
	err = xio.FullBuffer(conn, buf, 5, nil)
	if err != nil {
		return
	}
	if buf[0] != 0x05 {
		err = fmt.Errorf("only ver 0x05 is supported, but %x", buf[0])
		return
	}
	var uri string
	var utype int
	switch buf[3] {
	case 0x01:
		err = xio.FullBuffer(conn, buf[5:], 5, nil)
		if err == nil {
			remote := fmt.Sprintf("%v.%v.%v.%v", buf[4], buf[5], buf[6], buf[7])
			port := uint16(buf[8])*256 + uint16(buf[9])
			uri = fmt.Sprintf("%v:%v", remote, port)
			utype = SocksUriTypeNormal
		}
	case 0x03:
		err = xio.FullBuffer(conn, buf[5:], uint32(buf[4]+2), nil)
		if err == nil {
			remote := string(buf[5 : buf[4]+5])
			port := uint16(buf[buf[4]+5])*256 + uint16(buf[buf[4]+6])
			uri = fmt.Sprintf("%v:%v", remote, port)
			utype = SocksUriTypeNormal
		}
	case 0x13:
		err = xio.FullBuffer(conn, buf[5:], uint32(buf[4]+2), nil)
		if err == nil {
			uri = string(buf[5 : buf[4]+5])
			utype = SocksUriTypeBS
		}
	default:
		err = fmt.Errorf("ATYP %v is not supported", buf[3])
		return
	}
	DebugLog("SocksProxy start dial to %v on %v", uri, conn.RemoteAddr())
	pending := NewPendingConn(conn)
	_, err = s.Dialer(utype, uri, pending)
	if err != nil {
		buf[0], buf[1], buf[2], buf[3] = 0x05, 0x04, 0x00, 0x01
		buf[4], buf[5], buf[6], buf[7] = 0x00, 0x00, 0x00, 0x00
		buf[8], buf[9] = 0x00, 0x00
		if cerr, ok := err.(Codable); ok {
			buf[1] = cerr.Code()
		}
		conn.Write(buf[:10])
		InfoLog("SocksProxy dial to %v on %v fail with %v", uri, conn.RemoteAddr(), err)
		pending.Close()
		return
	}
	buf[0], buf[1], buf[2], buf[3] = 0x05, 0x00, 0x00, 0x01
	buf[4], buf[5], buf[6], buf[7] = 0x00, 0x00, 0x00, 0x00
	buf[8], buf[9] = 0x00, 0x00
	_, err = conn.Write(buf[:10])
	if err == nil {
		pending.Start()
	} else {
		pending.Close()
	}
}
