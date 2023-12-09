package dialer

import (
	"time"

	"github.com/codingeasygo/tun2conn/dnsgw"
	"github.com/codingeasygo/util/xmap"
)

type DnsGwDialer struct {
	MTU     int
	Runner  int
	Timeout time.Duration
	conf    xmap.M
}

// NewDnsGwDialer will return new DnsGwDialer
func NewDnsGwDialer() *DnsGwDialer {
	return &DnsGwDialer{
		conf:    xmap.M{},
		MTU:     2048,
		Runner:  3,
		Timeout: 5 * time.Second,
	}
}

// Name will return dialer name
func (d *DnsGwDialer) Name() string {
	return "dns"
}

// Bootstrap the dialer.
func (d *DnsGwDialer) Bootstrap(options xmap.M) (err error) {
	d.conf = options
	d.MTU = options.IntDef(2048, "mtu")
	d.Runner = options.IntDef(3, "runner")
	d.Timeout, err = time.ParseDuration(options.StrDef("5s", "timeout"))
	return
}

// Options is options getter
func (d *DnsGwDialer) Options() xmap.M {
	return d.conf
}

// Matched will return whether the uri is invalid tcp uri.
func (d *DnsGwDialer) Matched(uri string) bool {
	switch uri {
	case "tcp://dnsgw", "dnsgw", "dns://resolver", "resolver", "tcp://127.0.0.1:53", "127.0.0.1:53":
		return true
	default:
		return false
	}
}

// Dial one connection by uri
func (d *DnsGwDialer) Dial(channel Channel, sid uint16, uri string) (conn Conn, err error) {
	gw := dnsgw.NewGateway(d.Runner)
	gw.MTU = d.MTU
	gw.Timeout = d.Timeout
	conn = gw
	return
}

func (d *DnsGwDialer) String() string {
	return "DnsGwDialer"
}

// Shutdown will shutdown dial
func (d *DnsGwDialer) Shutdown() (err error) {
	return
}
