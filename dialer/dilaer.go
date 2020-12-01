package dialer

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
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
	//shutdown
	Shutdown() error
	//
	Options() xmap.M
	//match uri
	Matched(uri string) bool
	//dial raw connection
	Dial(sid uint64, uri string, raw io.ReadWriteCloser) (r Conn, err error)
}

//Pool is the set of Dialer
type Pool struct {
	Name        string
	Dialers     []Dialer
	Webs        map[string]http.Handler
	conns       map[string]Conn
	connsLocker sync.RWMutex
}

//NewPool will return new Pool
func NewPool(name string) (pool *Pool) {
	pool = &Pool{
		Name:        name,
		conns:       map[string]Conn{},
		connsLocker: sync.RWMutex{},
	}
	return
}

//AddDialer will append dialer which is bootstraped to pool
func (p *Pool) AddDialer(dialers ...Dialer) (err error) {
	p.Dialers = append(p.Dialers, dialers...)
	return
}

//Bootstrap will bootstrap all supported dialer
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
		InfoLog("Pool(%v) add echo dialer to pool", p.Name)
	}
	if options.Value("dav") != nil || options.IntDef(0, "standard") > 0 || options.IntDef(0, "std") > 0 {
		conf := options.MapDef(xmap.M{}, "dav")
		web := NewWebDialer("dav", NewWebdavHandler(conf.MapDef(xmap.M{}, "dirs")))
		web.Bootstrap(conf)
		p.Dialers = append(p.Dialers, web)
		InfoLog("Pool(%v) add dav dialer to pool", p.Name)
	}
	if options.Value("web") != nil || options.IntDef(0, "standard") > 0 || options.IntDef(0, "std") > 0 {
		for n, w := range p.Webs {
			web := NewWebDialer(n, w)
			web.Bootstrap(options.MapDef(xmap.M{}, n))
			p.Dialers = append(p.Dialers, web)
			InfoLog("Pool(%v) add web/%v dialer to pool", p.Name, n)
		}
	}
	if options.Value("tcp") != nil || options.IntDef(0, "standard") > 0 || options.IntDef(0, "std") > 0 {
		tcp := NewTCPDialer()
		tcp.Bootstrap(options.MapDef(xmap.M{}, "tcp"))
		p.Dialers = append(p.Dialers, tcp)
		InfoLog("Pool(%v) add tcp dialer to pool", p.Name)
	}
	return nil
}

//Dial the uri by dialer poo
func (p *Pool) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (r Conn, err error) {
	DebugLog("Pool(%v) try dial to %v", p.Name, uri)
	for _, dialer := range p.Dialers {
		if dialer.Matched(uri) {
			r, err = dialer.Dial(sid, uri, pipe)
			return
		}
	}
	err = fmt.Errorf("uri(%v) is not supported(not matched dialer)", uri)
	return
}

//Shutdown will shutdown all dialer
func (p *Pool) Shutdown() (err error) {
	return
}

//DefaultDialerCreator is default all dialer creator
func DefaultDialerCreator(t string) (dialer Dialer) {
	switch t {
	case "balance":
		dialer = NewBalancedDialer()
	case "socks":
		dialer = NewSocksProxyDialer()
	}
	return
}

//NewDialer is default all dialer creator
var NewDialer = DefaultDialerCreator
