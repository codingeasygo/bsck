package dialer

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/Centny/gwf/log"
	"github.com/Centny/gwf/util"
	"golang.org/x/net/webdav"
)

//WebDialer is an implementation of the Dialer interface for dial to web server
type WebDialer struct {
	accept  chan net.Conn
	consLck sync.RWMutex
	cons    map[string]*WebDialerConn
	davsLck sync.RWMutex
	davs    map[string]*WebdavHandler
	conf    util.Map
}

//NewWebDialer will return new WebDialer
func NewWebDialer() (dialer *WebDialer) {
	dialer = &WebDialer{
		accept:  make(chan net.Conn, 10),
		consLck: sync.RWMutex{},
		cons:    map[string]*WebDialerConn{},
		davsLck: sync.RWMutex{},
		davs:    map[string]*WebdavHandler{},
		conf:    util.Map{},
	}
	return
}

//Name will return dialer name
func (web *WebDialer) Name() string {
	return "web"
}

//Bootstrap the web dialer
func (web *WebDialer) Bootstrap(options util.Map) error {
	go func() {
		http.Serve(web, web)
		close(web.accept)
	}()
	return nil
}

func (web *WebDialer) Options() util.Map {
	return web.conf
}

//Shutdown the web dialer
func (web *WebDialer) Shutdown() error {
	web.accept <- nil
	return nil
}

//Matched will return whether the uri is a invalid uri
func (web *WebDialer) Matched(uri string) bool {
	target, err := url.Parse(uri)
	return err == nil && target.Scheme == "http" && target.Host == "web"
}

//Dial to web server
func (web *WebDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (raw Conn, err error) {
	conn, basic, err := PipeWebDialerConn(sid, uri)
	if err != nil {
		return
	}
	web.consLck.Lock()
	web.cons[fmt.Sprintf("%v", sid)] = conn
	web.consLck.Unlock()
	web.accept <- conn
	raw = NewCopyPipable(basic)
	if pipe != nil {
		err = raw.Pipe(pipe)
		if err != nil {
			conn.Close()
			basic.Close()
		}
	}
	return
}

//Accept one connection to process web server.
func (web *WebDialer) Accept() (conn net.Conn, err error) {
	conn = <-web.accept
	if conn == nil {
		err = fmt.Errorf("WebDial is closed")
	}
	return
}

//Close is not used
func (web *WebDialer) Close() error {
	return nil
}

//Addr return the web dialer address, it always return dialer
func (web *WebDialer) Addr() net.Addr {
	return web
}

//Network return "tcp"
func (web *WebDialer) Network() string {
	return "tcp"
}

func (web *WebDialer) String() string {
	return "WebDialer(0:0)"
}

func (web *WebDialer) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	cid := req.RemoteAddr
	web.consLck.Lock()
	conn := web.cons[cid]
	web.consLck.Unlock()
	if conn == nil {
		resp.WriteHeader(404)
		fmt.Fprintf(resp, "WebDialConn is not exist by cid(%v)", cid)
		return
	}
	web.davsLck.Lock()
	dav := web.davs[conn.DIR]
	if dav == nil {
		dav = NewWebdavHandler(conn.DIR)
		web.davs[conn.DIR] = dav
	}
	web.davsLck.Unlock()
	dav.ServeHTTP(resp, req)
}

//WebDialerConn is an implementation of the net.Conn interface for pipe WebDialerConn to raw connection.
type WebDialerConn struct {
	*PipedConn        //the piped connection
	SID        uint64 //session id
	URI        string //target uri
	DIR        string //work directory
}

//PipeWebDialerConn will return new WebDialerConn and piped raw connection.
func PipeWebDialerConn(sid uint64, uri string) (conn *WebDialerConn, raw io.ReadWriteCloser, err error) {
	args, err := url.Parse(uri)
	if err != nil {
		return
	}
	dir := args.Query().Get("dir")
	if len(dir) < 1 {
		err = fmt.Errorf("the dir arguemnt is required")
		return
	}
	conn = &WebDialerConn{
		SID: sid,
		URI: uri,
		DIR: dir,
	}
	conn.PipedConn, raw, err = CreatePipedConn()
	return
}

//LocalAddr return self
func (w *WebDialerConn) LocalAddr() net.Addr {
	return w
}

//RemoteAddr return self
func (w *WebDialerConn) RemoteAddr() net.Addr {
	return w
}

//Network return WebDialerConn
func (w *WebDialerConn) Network() string {
	return "WebDialerConn"
}

//
func (w *WebDialerConn) String() string {
	return fmt.Sprintf("%v", w.SID)
}

//WebdavHandler is an implementation of the http.Handler interface for handling web GET/DAV
type WebdavHandler struct {
	dav webdav.Handler
	fs  http.Handler
}

//NewWebdavHandler will return new WebdavHandler
func NewWebdavHandler(dir string) *WebdavHandler {
	return &WebdavHandler{
		dav: webdav.Handler{
			FileSystem: webdav.Dir(dir),
			LockSystem: webdav.NewMemLS(),
		},
		fs: http.FileServer(http.Dir(dir)),
	}
}

func (w *WebdavHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	log.D("WebdavHandler proc %v", req.RequestURI)
	if req.Method == "GET" {
		w.fs.ServeHTTP(resp, req)
	} else {
		w.dav.ServeHTTP(resp, req)
	}
}

//PipedConn is an implementation of the net.Conn interface for piped two connection.
type PipedConn struct {
	piped *DuplexPiped
	up    bool
}

//CreatePipedConn will return two piped connection.
func CreatePipedConn() (a, b *PipedConn, err error) {
	piped := &DuplexPiped{}
	piped.UpReader, piped.DownWriter, err = os.Pipe()
	if err == nil {
		piped.DownReader, piped.UpWriter, err = os.Pipe()
	}
	if err == nil {
		a = &PipedConn{
			piped: piped,
			up:    true,
		}
		b = &PipedConn{
			piped: piped,
			up:    false,
		}
	}
	return
}

func (p *PipedConn) Read(b []byte) (n int, err error) {
	if p.up {
		n, err = p.piped.UpReader.Read(b)
	} else {
		n, err = p.piped.DownReader.Read(b)
	}
	return
}

func (p *PipedConn) Write(b []byte) (n int, err error) {
	if p.up {
		n, err = p.piped.UpWriter.Write(b)
	} else {
		n, err = p.piped.DownWriter.Write(b)
	}
	return
}

//Close the piped connection
func (p *PipedConn) Close() error {
	return p.piped.Close()
}

//LocalAddr return self
func (p *PipedConn) LocalAddr() net.Addr {
	return p
}

//RemoteAddr return self
func (p *PipedConn) RemoteAddr() net.Addr {
	return p
}

//SetDeadline is empty
func (p *PipedConn) SetDeadline(t time.Time) error {
	return nil
}

//SetReadDeadline is empty
func (p *PipedConn) SetReadDeadline(t time.Time) error {
	return nil
}

//SetWriteDeadline is empty
func (p *PipedConn) SetWriteDeadline(t time.Time) error {
	return nil
}

//Network return "piped"
func (p *PipedConn) Network() string {
	return "piped"
}

func (p *PipedConn) String() string {
	return "piped"
}
