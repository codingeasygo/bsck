package dialer

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"

	"github.com/Centny/gwf/util"
)

//Statable is interface for get current state.
type Statable interface {
	State() util.Map
}

//StateBuffer is an io.ReadWriteCloser by read from bytes.Buffer and write discard
type StateBuffer struct {
	alias string
	buf   *bytes.Buffer
}

//NewStateBuffer is the default creator by buffer.
func NewStateBuffer(alias string, buf *bytes.Buffer) *StateBuffer {
	return &StateBuffer{
		alias: alias,
		buf:   buf,
	}
}

func (s *StateBuffer) Read(p []byte) (n int, err error) {
	n, err = s.buf.Read(p)
	return
}

func (s *StateBuffer) Write(p []byte) (n int, err error) {
	//discard
	n = len(p)
	return
}

//Close is impl to io.Closer
func (s *StateBuffer) Close() (err error) {
	return
}

func (s *StateBuffer) String() string {
	return "state-" + s.alias
}

//StateDialer is an impl of dialer.Dialer for get router status
type StateDialer struct {
	Alias string
	State Statable
	conf  util.Map
}

//NewStateDialer is the default creator by router.
func NewStateDialer(alias string, s Statable) *StateDialer {
	return &StateDialer{
		Alias: alias,
		State: s,
		conf:  util.Map{},
	}
}

//Name return dialer name.s
func (s *StateDialer) Name() string {
	return "state-" + s.Alias
}

//Bootstrap the dialer by configure
func (s *StateDialer) Bootstrap(options util.Map) error {
	s.conf = options
	return nil
}

//Options return the current configure of dialer.
func (s *StateDialer) Options() util.Map {
	return s.conf
}

//Matched return if uri is supported for this dialer.
func (s *StateDialer) Matched(uri string) bool {
	target, err := url.Parse(uri)
	return err == nil && target.Scheme == "state" && target.Host == s.Alias
}

//Dial raw connection
func (s *StateDialer) Dial(sid uint64, uri string, raw io.ReadWriteCloser) (conn Conn, err error) {
	data, _ := json.Marshal(s.State.State())
	conn = NewCopyPipable(NewStateBuffer(s.Alias, bytes.NewBuffer(data)))
	if raw != nil {
		conn.Pipe(raw)
	}
	return
}
