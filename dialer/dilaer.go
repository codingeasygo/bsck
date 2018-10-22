package dialer

import (
	"fmt"
	"io"
	"sync/atomic"

	"github.com/Centny/gwf/util"
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

// Dialer is the interface that wraps the dialer
type Dialer interface {
	Name() string
	//initial dialer
	Bootstrap(options util.Map) error
	//
	Options() util.Map
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

func (p *Pool) Bootstrap(options util.Map) error {
	dialerOptions := options.AryMapVal("dialers")
	for _, option := range dialerOptions {
		dtype := option.StrVal("type")
		dialer := NewDialer(dtype)
		if dialer == nil {
			return fmt.Errorf("create dialer fail by %v", util.S2Json(option))
		}
		err := dialer.Bootstrap(option)
		if err != nil {
			return err
		}
		p.Dialers = append(p.Dialers, dialer)
	}
	if options.Val("cmd") != nil || options.IntValV("standard", 0) > 0 {
		cmd := NewCmdDialer()
		cmd.Bootstrap(options.MapValV("cmd", util.Map{}))
		p.Dialers = append(p.Dialers, cmd)
	}
	if options.Val("echo") != nil || options.IntValV("standard", 0) > 0 {
		echo := NewEchoDialer()
		echo.Bootstrap(options.MapValV("echo", util.Map{}))
		p.Dialers = append(p.Dialers, echo)
	}
	if options.Val("web") != nil || options.IntValV("standard", 0) > 0 {
		web := NewWebDialer()
		web.Bootstrap(options.MapValV("web", util.Map{}))
		p.Dialers = append(p.Dialers, web)
	}
	if options.Val("tcp") != nil || options.IntValV("standard", 0) > 0 {
		tcp := NewTCPDialer()
		tcp.Bootstrap(options.MapValV("tcp", util.Map{}))
		p.Dialers = append(p.Dialers, tcp)
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
	case "cmd":
		dialer = NewCmdDialer()
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
