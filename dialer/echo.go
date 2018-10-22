package dialer

import (
	"io"
	"net/url"
	"sync"

	"github.com/Centny/gwf/util"
)

//EchoDialer is an implementation of the Dialer interface for echo tcp connection.
type EchoDialer struct {
	conf util.Map
}

//NewEchoDialer will return new EchoDialer
func NewEchoDialer() (dialer *EchoDialer) {
	dialer = &EchoDialer{
		conf: util.Map{},
	}
	return
}

//Name will return dialer name
func (e *EchoDialer) Name() string {
	return "echo"
}

//Bootstrap the dialer
func (e *EchoDialer) Bootstrap(options util.Map) error {
	e.conf = options
	return nil
}

func (e *EchoDialer) Options() util.Map {
	return e.conf
}

//Matched will return whetheer uri is invalid
func (e *EchoDialer) Matched(uri string) bool {
	target, err := url.Parse(uri)
	return err == nil && target.Scheme == "tcp" && target.Host == "echo"
}

//Dial one echo connection.
func (e *EchoDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (r Conn, err error) {
	r = NewEchoReadWriteCloser()
	if pipe != nil {
		err = r.Pipe(pipe)
	}
	return
}

//EchoReadWriteCloser is an implementation of the io.ReadWriteCloser interface for pipe write to read.
type EchoReadWriteCloser struct {
	pipe chan []byte
	lck  sync.RWMutex
}

//NewEchoReadWriteCloser will return new EchoReadWriteCloser
func NewEchoReadWriteCloser() *EchoReadWriteCloser {
	return &EchoReadWriteCloser{
		pipe: make(chan []byte, 1),
		lck:  sync.RWMutex{},
	}
}

func (e *EchoReadWriteCloser) Write(p []byte) (n int, err error) {
	if e.pipe == nil {
		err = io.EOF
		return
	}
	n = len(p)
	e.pipe <- p[:]
	return
}

func (e *EchoReadWriteCloser) Read(p []byte) (n int, err error) {
	if e.pipe == nil {
		err = io.EOF
		return
	}
	buf := <-e.pipe
	if buf == nil {
		err = io.EOF
		return
	}
	n = copy(p, buf)
	return
}

//Close echo read writer closer.
func (e *EchoReadWriteCloser) Close() (err error) {
	e.lck.Lock()
	if e.pipe != nil {
		e.pipe <- nil
		close(e.pipe)
		e.pipe = nil
	}
	e.lck.Unlock()
	return
}

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
