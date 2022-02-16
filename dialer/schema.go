package dialer

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xmap"
)

//SchemaDialer is an implementation of the Dialer interface for dial tcp connections.
type SchemaDialer struct {
	portMatcher *regexp.Regexp
	conf        xmap.M
	supported   map[string]string
}

//NewSchemaDialer will return new SchemaDialer
func NewSchemaDialer() *SchemaDialer {
	return &SchemaDialer{
		portMatcher: regexp.MustCompile("^.*:[0-9]+$"),
		conf:        xmap.M{},
		supported:   map[string]string{},
	}
}

//Name will return dialer name
func (t *SchemaDialer) Name() string {
	return "schema"
}

//Bootstrap the dialer.
func (t *SchemaDialer) Bootstrap(options xmap.M) error {
	t.conf = options
	for key, val := range options.MapDef(nil, "mapping") {
		uri := converter.String(val)
		if len(key) > 0 && len(uri) > 0 {
			t.supported[key] = uri
		}
	}
	return nil
}

//Options is options getter
func (t *SchemaDialer) Options() xmap.M {
	return t.conf
}

//Matched will return whether the uri is invalid tcp uri.
func (t *SchemaDialer) Matched(uri string) bool {
	checkURI, err := url.Parse(uri)
	if err != nil {
		return false
	}
	remoteAddr := t.supported[fmt.Sprintf("%v://%v", checkURI.Scheme, checkURI.Host)]
	if len(remoteAddr) < 1 {
		remoteAddr = t.supported[fmt.Sprintf("%v://%v", checkURI.Scheme, "*")]
	}
	return len(remoteAddr) > 0
}

//Dial one connection by uri
func (t *SchemaDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (raw Conn, err error) {
	targetURI, err := url.Parse(uri)
	if err != nil {
		return
	}
	remoteAddr := t.supported[fmt.Sprintf("%v://%v", targetURI.Scheme, targetURI.Host)]
	if len(remoteAddr) < 1 {
		remoteAddr = t.supported[fmt.Sprintf("%v://%v", targetURI.Scheme, "*")]
	}
	if len(remoteAddr) < 1 {
		err = fmt.Errorf("schema %v is not supported", targetURI.Scheme)
		return
	}
	remoteURI, err := url.Parse(remoteAddr)
	if err != nil {
		return
	}
	var dialer net.Dialer
	network := remoteURI.Scheme
	host := remoteURI.Host
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
	var basic net.Conn
	basic, err = dialer.Dial("tcp", host)
	if err == nil {
		raw = NewCopyPipable(basic)
		if pipe != nil {
			assert(raw.Pipe(pipe) == nil)
		}
	}
	return
}

func (t *SchemaDialer) String() string {
	return "SchemaDialer"
}

//Shutdown will shutdown dial
func (t *SchemaDialer) Shutdown() (err error) {
	return
}
