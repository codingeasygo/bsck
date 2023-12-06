package dialer

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xmap"
)

type Conn = io.ReadWriteCloser

type Channel interface {
	//the connection id
	ID() uint16
	//the channel name
	Name() string
	//conn context getter
	Context() xmap.M
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
	Dial(channel Channel, sid uint16, uri string) (r Conn, err error)
}

// Pool is the set of Dialer
type Pool struct {
	Name        string
	Dialers     []Dialer
	Webs        map[string]http.Handler
	conns       map[string]Conn
	connsLocker sync.RWMutex
}

// NewPool will return new Pool
func NewPool(name string) (pool *Pool) {
	pool = &Pool{
		Name:        name,
		conns:       map[string]Conn{},
		connsLocker: sync.RWMutex{},
	}
	return
}

// AddDialer will append dialer which is bootstraped to pool
func (p *Pool) AddDialer(dialers ...Dialer) (err error) {
	p.Dialers = append(p.Dialers, dialers...)
	return
}

// Bootstrap will bootstrap all supported dialer
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
	if options.Value("udpgw") != nil {
		udpgw := NewUdpGwDialer()
		udpgw.Bootstrap(options.MapDef(xmap.M{}, "udpgw"))
		p.Dialers = append(p.Dialers, udpgw)
		InfoLog("Pool(%v) add tcp dialer to pool", p.Name)
	}
	if options.Value("ssh") != nil {
		conf := options.MapDef(xmap.M{}, "ssh")
		conf["dir"] = options.StrDef(".", "dir")
		ssh := NewSshDialer()
		ssh.Bootstrap(conf)
		p.Dialers = append(p.Dialers, ssh)
		InfoLog("Pool(%v) add ssh dialer to pool", p.Name)
	}
	if options.Value("tcp") != nil || options.IntDef(0, "standard") > 0 || options.IntDef(0, "std") > 0 {
		tcp := NewTCPDialer()
		tcp.Bootstrap(options.MapDef(xmap.M{}, "tcp"))
		p.Dialers = append(p.Dialers, tcp)
		InfoLog("Pool(%v) add udpgw dialer to pool", p.Name)
	}
	return nil
}

// Dial the uri by dialer poo
func (p *Pool) Dial(channel Channel, sid uint16, uri string) (r Conn, err error) {
	DebugLog("Pool(%v) try dial to %v", p.Name, uri)
	for _, dialer := range p.Dialers {
		if dialer.Matched(uri) {
			r, err = dialer.Dial(channel, sid, uri)
			return
		}
	}
	err = fmt.Errorf("uri(%v) is not supported(not matched dialer)", uri)
	return
}

// Shutdown will shutdown all dialer
func (p *Pool) Shutdown() (err error) {
	for _, dialer := range p.Dialers {
		dialer.Shutdown()
	}
	p.Dialers = []Dialer{}
	return
}

// DefaultDialerCreator is default all dialer creator
func DefaultDialerCreator(t string) (dialer Dialer) {
	creator := dialerAll[t]
	if creator != nil {
		dialer = creator()
	}
	return
}

var dialerAll = map[string]func() Dialer{}

// NewDialer is default all dialer creator
var NewDialer = DefaultDialerCreator

func RegisterDialerCreator(t string, creator func() Dialer) {
	dialerAll[t] = creator
}
