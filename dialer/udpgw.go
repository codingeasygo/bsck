package dialer

import (
	"net"
	"time"

	"github.com/codingeasygo/tun2conn/udpgw"
	"github.com/codingeasygo/util/xmap"
)

type UdpGwDialer struct {
	MTU      int
	DNS      *net.UDPAddr
	MaxConn  int
	MaxAlive time.Duration
	Delay    time.Duration
}

func NewUdpGwDialer() (dialer *UdpGwDialer) {
	dialer = &UdpGwDialer{
		MTU:     1600,
		MaxConn: 256,
		Delay:   time.Second,
	}
	return
}

func (u *UdpGwDialer) Name() string {
	return "UdpGw"
}

// initial dialer
func (u *UdpGwDialer) Bootstrap(options xmap.M) (err error) {
	var dns string
	var maxaLive time.Duration = 60000
	err = options.ValidFormat(`
		mtu,o|i,r:1500;
		dns,o|s,l:0;
		max_conn,o|i,r:0;
		max_alive,o|i,r:0;
	`, &u.MTU, &dns, &u.MaxConn, &maxaLive)
	if err != nil {
		return
	}
	u.MaxAlive = maxaLive * time.Millisecond
	if len(dns) > 0 {
		u.DNS, err = net.ResolveUDPAddr("", dns)
		if err != nil {
			return
		}
	}
	return
}

// shutdown
func (u *UdpGwDialer) Shutdown() (err error) {
	return
}

func (u *UdpGwDialer) Options() xmap.M {
	return xmap.M{}
}

// match uri
func (u *UdpGwDialer) Matched(uri string) bool {
	switch uri {
	case "tcp://udpgw", "udpgw", "tcp://127.0.0.3:10", "127.0.0.3:10":
		return true
	default:
		return false
	}
}

// dial raw connection
func (u *UdpGwDialer) Dial(channel Channel, sid uint16, uri string) (conn Conn, err error) {
	gw := udpgw.NewGateway()
	gw.MTU = u.MTU
	gw.DNS = u.DNS
	gw.MaxConn = u.MaxConn
	conn = gw
	return
}
