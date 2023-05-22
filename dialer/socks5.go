package dialer

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/codingeasygo/util/xmap"
)

type CodeError struct {
	Inner    error
	ByteCode byte
}

func (c *CodeError) Code() byte {
	return c.ByteCode
}

func (c *CodeError) Error() string {
	return c.Inner.Error()
}

// SocksProxyAddressPooler is an interface to handler proxy server address get/set
type SocksProxyAddressPooler interface {
	//Get will return the proxy server address
	Get(uri string) (address string, err error)
	//Done will mark one address is free
	Done(address, uri string, err error)
}

// StringAddressPooler is an implementation of the SocksProxyAddressPooler interface for one string address.
type StringAddressPooler string

// Get will return the proxy server address
func (s StringAddressPooler) Get(uri string) (address string, err error) {
	address = string(s)
	return
}

// Done will mark one address is fress
func (s StringAddressPooler) Done(address, uri string, err error) {
}

// SocksProxyDialer is an implementation of the Dialer interface for dial by socks proxy.
type SocksProxyDialer struct {
	ID      string
	Pooler  SocksProxyAddressPooler
	matcher *regexp.Regexp
	conf    xmap.M
}

// NewSocksProxyDialer will return new SocksProxyDialer
func NewSocksProxyDialer() *SocksProxyDialer {
	return &SocksProxyDialer{
		matcher: regexp.MustCompile("^.*:[0-9]+$"),
		conf:    xmap.M{},
	}
}

// Name will return dialer name
func (s *SocksProxyDialer) Name() string {
	return s.ID
}

// Bootstrap the dialer.
func (s *SocksProxyDialer) Bootstrap(options xmap.M) (err error) {
	s.ID = options.Str("id")
	if len(s.ID) < 1 {
		return fmt.Errorf("the dialer id is required")
	}
	if options != nil {
		s.Pooler = StringAddressPooler(options.Str("address"))
	}
	s.conf = options
	matcher := options.Str("matcher")
	if len(matcher) > 0 {
		s.matcher, err = regexp.Compile(matcher)
	}
	return
}

// Options is options getter
func (s *SocksProxyDialer) Options() xmap.M {
	return s.conf
}

// Matched will return whether the uri is invalid tcp uri.
func (s *SocksProxyDialer) Matched(uri string) bool {
	remote, err := url.Parse(uri)
	return err == nil && s.matcher.MatchString(remote.Host)
}

// Dial one connection by uri
func (s *SocksProxyDialer) Dial(channel Channel, sid uint64, uri string, pipe io.ReadWriteCloser) (raw Conn, err error) {
	remote, err := url.Parse(uri)
	if err != nil {
		return
	}
	var host string
	var port int64
	parts := strings.SplitN(remote.Host, ":", 2)
	if len(parts) < 2 {
		// err = fmt.Errorf("not supported address:%v", remote.Host)
		// return
		host = parts[0]
		port = 0
	} else {
		host = parts[0]
		port, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			err = fmt.Errorf("parse address:%v error:%v", remote.Host, err)
			return
		}
	}
	address, err := s.Pooler.Get(uri)
	if err != nil {
		return
	}
	var doneErr error
	defer s.Pooler.Done(address, uri, doneErr)
	DebugLog("SocksProxyDialer dial to %v", address)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		doneErr = &CodeError{Inner: err, ByteCode: 0x10}
		return
	}
	conn.Write([]byte{0x05, 0x01, 0x00})
	buf := make([]byte, 1024*64)
	err = fullBuf(conn, buf, 2, nil)
	if err != nil {
		conn.Close()
		doneErr = &CodeError{Inner: err, ByteCode: 0x10}
		return
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		err = fmt.Errorf("unsupported %x", buf)
		conn.Close()
		doneErr = &CodeError{Inner: err, ByteCode: 0x10}
		return
	}
	blen := len(host) + 7
	buf[0], buf[1], buf[2] = 0x05, 0x01, 0x00
	buf[3], buf[4] = 0x03, byte(len(host))
	copy(buf[5:], []byte(host))
	buf[blen-2] = byte(port / 256)
	buf[blen-1] = byte(port % 256)
	conn.Write(buf[:blen])
	err = fullBuf(conn, buf, 5, nil)
	if err != nil {
		conn.Close()
		doneErr = &CodeError{Inner: err, ByteCode: 0x10}
		return
	}
	switch buf[3] {
	case 0x01:
		err = fullBuf(conn, buf[5:], 5, nil)
	case 0x03:
		err = fullBuf(conn, buf[5:], uint32(buf[4])+2, nil)
	case 0x04:
		err = fullBuf(conn, buf[5:], 17, nil)
	default:
		err = fmt.Errorf("reply address type is not supported:%v", buf[3])
	}
	if err != nil {
		conn.Close()
		doneErr = &CodeError{Inner: err, ByteCode: 0x10}
		return
	}
	if buf[1] != 0x00 {
		conn.Close()
		err = fmt.Errorf("response code(%x)", buf[1])
		if buf[1] >= 0x10 {
			doneErr = &CodeError{Inner: err, ByteCode: 0x10}
		}
		return
	}
	raw = NewCopyPipable(conn)
	if pipe != nil {
		assert(raw.Pipe(pipe) == nil)
	}
	return
}

// Shutdown will shutdown dial
func (s *SocksProxyDialer) Shutdown() (err error) {
	return
}

func (s *SocksProxyDialer) String() string {
	return fmt.Sprintf("SocksProxyDialer-%v", s.ID)
}
