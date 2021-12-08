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

	"github.com/codingeasygo/util/xmap"
	"golang.org/x/net/webdav"
)

//WebDialer is an implementation of the Dialer interface for dial to web server
type WebDialer struct {
	stopped bool
	accept  chan net.Conn
	consLck sync.RWMutex
	cons    map[string]*WebDialerConn
	conf    xmap.M
	host    string
	Handler http.Handler
}

//NewWebDialer will return new WebDialer
func NewWebDialer(host string, handler http.Handler) (dialer *WebDialer) {
	dialer = &WebDialer{
		accept:  make(chan net.Conn, 10),
		consLck: sync.RWMutex{},
		cons:    map[string]*WebDialerConn{},
		conf:    xmap.M{},
		host:    host,
		Handler: handler,
	}
	return
}

//Name will return dialer name
func (web *WebDialer) Name() string {
	return web.host
}

//Bootstrap the web dialer
func (web *WebDialer) Bootstrap(options xmap.M) error {
	go func() {
		http.Serve(web, web.Handler)
		web.consLck.Lock()
		close(web.accept)
		web.stopped = true
		web.consLck.Unlock()
	}()
	return nil
}

//Options is options getter
func (web *WebDialer) Options() xmap.M {
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
	return err == nil && target.Scheme == "http" && target.Host == web.host
}

//Dial to web server
func (web *WebDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (raw Conn, err error) {
	web.consLck.Lock()
	defer web.consLck.Unlock()
	if web.stopped {
		err = fmt.Errorf("stopped")
		return
	}
	conn, basic, err := PipeWebDialerConn(sid, uri)
	if err != nil {
		return
	}
	web.cons[fmt.Sprintf("%v", sid)] = conn
	web.accept <- conn
	raw = NewCopyPipable(basic)
	if pipe != nil {
		assert(raw.Pipe(pipe) == nil)
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

//FindConn will find connection by id
func (web *WebDialer) FindConn(sid string) (conn *WebDialerConn) {
	web.consLck.Lock()
	conn = web.cons[sid]
	web.consLck.Unlock()
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

//WebDialerConn is an implementation of the net.Conn interface for pipe WebDialerConn to raw connection.
type WebDialerConn struct {
	*PipedConn        //the piped connection
	SID        uint64 //session id
	URI        string //target uri
}

//PipeWebDialerConn will return new WebDialerConn and piped raw connection.
func PipeWebDialerConn(sid uint64, uri string) (conn *WebDialerConn, raw io.ReadWriteCloser, err error) {
	conn = &WebDialerConn{
		SID: sid,
		URI: uri,
	}
	conn.PipedConn, raw, err = CreatePipedConn()
	return
}

//LocalAddr return self
func (w *WebDialerConn) LocalAddr() net.Addr {
	return NewWebDialerAddr(fmt.Sprintf("%v", w.SID), w.URI)
}

//RemoteAddr return self
func (w *WebDialerConn) RemoteAddr() net.Addr {
	return NewWebDialerAddr(fmt.Sprintf("%v", w.SID), w.URI)
}

//Network return WebDialerConn
func (w *WebDialerConn) Network() string {
	return "WebDialerConn"
}

//String will info
func (w *WebDialerConn) String() string {
	return fmt.Sprintf("%v", w.SID)
}

//WebDialerAddr is net.Addr implement
type WebDialerAddr struct {
	Net  string
	Info string
}

//NewWebDialerAddr will return new web dialer address
func NewWebDialerAddr(net, info string) (addr *WebDialerAddr) {
	addr = &WebDialerAddr{Net: net, Info: info}
	return
}

//Network return WebDialerConn
func (w *WebDialerAddr) Network() string {
	return w.Net
}

//String will info
func (w *WebDialerAddr) String() string {
	return w.Info
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

//WebdavHandler is webdav handler
type WebdavHandler struct {
	davsLck sync.RWMutex
	davs    map[string]*WebdavFileHandler
	dirs    xmap.M
}

//NewWebdavHandler will return new WebdavHandler
func NewWebdavHandler(dirs xmap.M) (handler *WebdavHandler) {
	handler = &WebdavHandler{
		davsLck: sync.RWMutex{},
		davs:    map[string]*WebdavFileHandler{},
		dirs:    dirs,
	}
	return
}

func (web *WebdavHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	DebugLog("WebdavHandler access %v from %v", req.URL.RequestURI(), req.RemoteAddr)
	args, err := url.Parse(req.RemoteAddr)
	if err != nil {
		WarnLog("WebdavHandler parset remote address %v fail with %v", req.RemoteAddr, err)
		resp.WriteHeader(404)
		fmt.Fprintf(resp, "%v", err)
		return
	}
	dir := args.Query().Get("dir")
	if len(dir) < 1 {
		dir = req.URL.Query().Get("dir")
	}
	if len(dir) < 1 {
		err = fmt.Errorf("the dir argument is required")
		WarnLog("WebdavHandler parset remote address %v fail with %v", req.RemoteAddr, err)
		fmt.Fprintf(resp, "%v", err)
		return
	}
	web.davsLck.Lock()
	dir = web.dirs.StrDef(dir, "dir")
	dav := web.davs[dir]
	if dav == nil {
		dav = NewWebdavFileHandler(dir)
		web.davs[dir] = dav
	}
	web.davsLck.Unlock()
	dav.ServeHTTP(resp, req)
}

//WebdavFileHandler is an implementation of the http.Handler interface for handling web GET/DAV
type WebdavFileHandler struct {
	dav webdav.Handler
	fs  http.Handler
}

//NewWebdavFileHandler will return new WebdavHandler
func NewWebdavFileHandler(dir string) *WebdavFileHandler {
	return &WebdavFileHandler{
		dav: webdav.Handler{
			FileSystem: webdav.Dir(dir),
			LockSystem: webdav.NewMemLS(),
		},
		fs: http.FileServer(http.Dir(dir)),
	}
}

func (w *WebdavFileHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	DebugLog("WebdavFileHandler proc %v", req.RequestURI)
	if req.Method == "GET" {
		w.fs.ServeHTTP(resp, req)
	} else {
		w.dav.ServeHTTP(resp, req)
	}
}

// type ResponseWriter struct {
// 	http.ResponseWriter
// }

// func (r *ResponseWriter) Write(p []byte) (n int, err error) {
// 	n, err = r.ResponseWriter.Write(p)
// 	if err == nil {
// 		os.Stdout.Write(p)
// 	}
// 	return
// }

// func (r *ResponseWriter) WriteHeader(statusCode int) {
// 	r.ResponseWriter.WriteHeader(statusCode)
// 	fmt.Printf("--->%v\n%v\n", r.ResponseWriter.Header(), statusCode)
// }
