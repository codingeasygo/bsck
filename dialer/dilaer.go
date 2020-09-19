package dialer

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xmap"
)

type Pipable interface {
	Pipe(r io.ReadWriteCloser) error
}

type Conn interface {
	Pipable
	io.ReadWriteCloser
}

type CopyPipable struct {
	io.ReadWriteCloser
	piped uint32
}

func NewCopyPipable(raw io.ReadWriteCloser) *CopyPipable {
	return &CopyPipable{ReadWriteCloser: raw}
}

func (c *CopyPipable) Pipe(r io.ReadWriteCloser) (err error) {
	if atomic.CompareAndSwapUint32(&c.piped, 0, 1) {
		go c.copyAndClose(c, r)
		go c.copyAndClose(r, c)
	} else {
		err = fmt.Errorf("piped")
	}
	return
}

func (c *CopyPipable) copyAndClose(src io.ReadWriteCloser, dst io.ReadWriteCloser) {
	io.Copy(dst, src)
	dst.Close()
	src.Close()
}

func (c *CopyPipable) String() string {
	if conn, ok := c.ReadWriteCloser.(net.Conn); ok {
		return conn.RemoteAddr().String()
	}
	return fmt.Sprintf("%v", c.ReadWriteCloser)
}

// Dialer is the interface that wraps the dialer
type Dialer interface {
	Name() string
	//initial dialer
	Bootstrap(options xmap.M) error
	//
	Options() xmap.M
	//match uri
	Matched(uri string) bool
	//dial raw connection
	Dial(sid uint64, uri string, raw io.ReadWriteCloser) (r Conn, err error)
}

//Pool is the set of Dialer
type Pool struct {
	Dialers []Dialer
}

//NewPool will return new Pool
func NewPool() (pool *Pool) {
	pool = &Pool{}
	return
}

//AddDialer will append dialer which is bootstraped to pool
func (p *Pool) AddDialer(dialers ...Dialer) (err error) {
	p.Dialers = append(p.Dialers, dialers...)
	return
}

func (p *Pool) Bootstrap(options xmap.M) error {
	dialerOptions := options.ArrayMapDef(nil, "dialers")
	for _, option := range dialerOptions {
		dtype := option.Str("type")
		dialer := NewDialer(dtype)
		if dialer == nil {
			return fmt.Errorf("create dialer fail by %v", converter.JSON(option))
		}
		err := dialer.Bootstrap(option)
		if err != nil {
			return err
		}
		p.Dialers = append(p.Dialers, dialer)
	}
	if options.Value("echo") != nil || options.IntDef(0, "standard") > 0 || options.IntDef(0, "std") > 0 {
		echo := NewEchoDialer()
		echo.Bootstrap(options.Map("echo"))
		p.Dialers = append(p.Dialers, echo)
		InfoLog("Pool add echo dialer to pool")
	}
	if options.Value("web") != nil || options.IntDef(0, "standard") > 0 || options.IntDef(0, "std") > 0 {
		web := NewWebDialer()
		web.Bootstrap(options.MapDef(xmap.M{}, "web"))
		p.Dialers = append(p.Dialers, web)
		InfoLog("Pool add web dialer to pool")
	}
	if options.Value("tcp") != nil || options.IntDef(0, "standard") > 0 || options.IntDef(0, "std") > 0 {
		tcp := NewTCPDialer()
		tcp.Bootstrap(options.MapDef(xmap.M{}, "tcp"))
		p.Dialers = append(p.Dialers, tcp)
		InfoLog("Pool add tcp dialer to pool")
	}
	return nil
}

//Dial the uri by dialer poo
func (p *Pool) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (r Conn, err error) {
	for _, dialer := range p.Dialers {
		if dialer.Matched(uri) {
			r, err = dialer.Dial(sid, uri, pipe)
			return
		}
	}
	err = fmt.Errorf("uri(%v) is not supported(not matched dialer)", uri)
	return
}

func DefaultDialerCreator(t string) (dialer Dialer) {
	switch t {
	case "balance":
		dialer = NewBalancedDialer()
	case "echo":
		dialer = NewEchoDialer()
	case "socks":
		dialer = NewSocksProxyDialer()
	case "tcp":
		dialer = NewTCPDialer()
	case "web":
		dialer = NewWebDialer()
	}
	return
}

var NewDialer = DefaultDialerCreator
