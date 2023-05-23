package dialer

import (
	"io"
	"net/url"

	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
)

// EchoDialer is an implementation of the Dialer interface for echo tcp connection.
type EchoDialer struct {
	conf xmap.M
}

// NewEchoDialer will return new EchoDialer
func NewEchoDialer() (dialer *EchoDialer) {
	dialer = &EchoDialer{
		conf: xmap.M{},
	}
	return
}

// Name will return dialer name
func (e *EchoDialer) Name() string {
	return "echo"
}

// Bootstrap the dialer
func (e *EchoDialer) Bootstrap(options xmap.M) error {
	e.conf = options
	return nil
}

// Options is options getter
func (e *EchoDialer) Options() xmap.M {
	return e.conf
}

// Matched will return whetheer uri is invalid
func (e *EchoDialer) Matched(uri string) bool {
	target, err := url.Parse(uri)
	return err == nil && target.Scheme == "tcp" && target.Host == "echo"
}

// Dial one echo connection.
func (e *EchoDialer) Dial(channel Channel, sid uint16, uri string, pipe io.ReadWriteCloser) (r Conn, err error) {
	r = NewEchoReadWriteCloser()
	if pipe != nil {
		err = r.Pipe(pipe)
	}
	return
}

// Shutdown will shutdown dial
func (e *EchoDialer) Shutdown() (err error) {
	return
}

// EchoReadWriteCloser is an implementation of the io.ReadWriteCloser interface for pipe write to read.
type EchoReadWriteCloser struct {
	*xio.PipedChan
}

// NewEchoReadWriteCloser will return new EchoReadWriteCloser
func NewEchoReadWriteCloser() *EchoReadWriteCloser {
	return &EchoReadWriteCloser{
		PipedChan: xio.NewPipedChan(),
	}
}

// Pipe is Pipable implment
func (e *EchoReadWriteCloser) Pipe(raw io.ReadWriteCloser) (err error) {
	go e.copyAndClose(e, raw)
	go e.copyAndClose(raw, e)
	return
}

func (e *EchoReadWriteCloser) copyAndClose(src io.ReadWriteCloser, dst io.ReadWriteCloser) {
	io.Copy(dst, src)
	dst.Close()
	src.Close()
}

func (e *EchoReadWriteCloser) String() string {
	return "echo"
}
