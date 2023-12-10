package dialer

import (
	"context"
	"net"
	"net/url"
	"regexp"
	"syscall"
	"time"

	"github.com/codingeasygo/util/xmap"
)

// TCPDialer is an implementation of the Dialer interface for dial tcp connections.
type TCPDialer struct {
	portMatcher *regexp.Regexp
	conf        xmap.M
}

// NewTCPDialer will return new TCPDialer
func NewTCPDialer() *TCPDialer {
	return &TCPDialer{
		portMatcher: regexp.MustCompile("^.*:[0-9]+$"),
		conf:        xmap.M{},
	}
}

// Name will return dialer name
func (t *TCPDialer) Name() string {
	return "tcp"
}

// Bootstrap the dialer.
func (t *TCPDialer) Bootstrap(options xmap.M) error {
	t.conf = options
	return nil
}

// Options is options getter
func (t *TCPDialer) Options() xmap.M {
	return t.conf
}

// Matched will return whether the uri is invalid tcp uri.
func (t *TCPDialer) Matched(uri string) bool {
	_, err := url.Parse(uri)
	return err == nil
}

// Dial one connection by uri
func (t *TCPDialer) Dial(channel Channel, sid uint16, uri string) (raw Conn, err error) {
	remote, err := url.Parse(uri)
	if err == nil {
		var dialer net.Dialer
		dialer.Control = func(network, address string, c syscall.RawConn) error {
			return c.Control(RawConnControl)
		}
		bind := remote.Query().Get("bind")
		if len(bind) < 1 && t.conf != nil {
			bind = t.conf.Str("bind")
		}
		if len(bind) > 0 {
			dialer.LocalAddr, err = net.ResolveTCPAddr("tcp", bind)
			if err != nil {
				return
			}
		}
		network := remote.Scheme
		host := remote.Host
		switch network {
		case "http":
			if !t.portMatcher.MatchString(host) {
				host += ":80"
			}
		case "https":
			if !t.portMatcher.MatchString(host) {
				host += ":443"
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		raw, err = dialer.DialContext(ctx, "tcp", host)
		cancel()
	}
	return
}

func (t *TCPDialer) String() string {
	return "TCPDialer"
}

// Shutdown will shutdown dial
func (t *TCPDialer) Shutdown() (err error) {
	return
}
