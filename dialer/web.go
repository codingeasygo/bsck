package dialer

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"

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
func (web *WebDialer) Dial(channel Channel, sid uint64, uri string, pipe io.ReadWriteCloser) (raw Conn, err error) {
	web.consLck.Lock()
	defer web.consLck.Unlock()
	if web.stopped {
		err = fmt.Errorf("stopped")
		return
	}
	conn, basic, err := PipeWebDialerConn(channel, sid, uri)
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

//FindConnByID will find connection by id
func (web *WebDialer) FindConnByID(sid string) (conn *WebDialerConn, err error) {
	web.consLck.Lock()
	conn = web.cons[sid]
	web.consLck.Unlock()
	if conn == nil {
		err = fmt.Errorf("conn is not exits by %v", sid)
	}
	return
}

//FindConnByRequest will find connection by id
func (web *WebDialer) FindConnByRequest(req *http.Request) (conn *WebDialerConn, err error) {
	remoteArgs, err := url.ParseQuery(req.RemoteAddr)
	if err != nil {
		return
	}
	sid := remoteArgs.Get("session_id")
	if len(sid) < 1 {
		err = fmt.Errorf("session_id is not exits on RemoteAddr, req is not WebDialer Request")
		return
	}
	conn, err = web.FindConnByID(sid)
	return
}

//FindConnByRequest will find connection by id
func (web *WebDialer) FindChannelByRequest(req *http.Request) (channel Channel, err error) {
	conn, err := web.FindConnByRequest(req)
	if err != nil {
		return
	}
	if conn.Channel == nil {
		err = fmt.Errorf("channel is nil on conn %v", conn.SID)
		return
	}
	channel = conn.Channel
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
	*PipedConn //the piped connection
	Channel    Channel
	SID        uint64 //session id
	URI        string //target uri
}

//PipeWebDialerConn will return new WebDialerConn and piped raw connection.
func PipeWebDialerConn(channel Channel, sid uint64, uri string) (conn *WebDialerConn, raw io.ReadWriteCloser, err error) {
	conn = &WebDialerConn{
		Channel: channel,
		SID:     sid,
		URI:     uri,
	}
	conn.PipedConn, raw = CreatePipedConn()
	return
}

//LocalAddr return self
func (w *WebDialerConn) LocalAddr() net.Addr {
	return NewWebDialerAddr("bs", "local")
}

//RemoteAddr return self
func (w *WebDialerConn) RemoteAddr() net.Addr {
	args := url.Values{}
	if w.Channel != nil {
		args.Set("channel_id", fmt.Sprintf("%d", w.Channel.ID()))
	} else {
		args.Set("channel_id", fmt.Sprintf("%d", 0))
	}
	args.Set("session_id", fmt.Sprintf("%d", w.SID))
	args.Set("uri", w.URI)
	return NewWebDialerAddr("bs", args.Encode())
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
	net.Conn
	Info string
}

func (p *PipedConn) String() string {
	return p.Info
}

//CreatePipedConn will return two piped connection.
func CreatePipedConn(info ...string) (a, b *PipedConn) {
	a = &PipedConn{Info: "piped"}
	if len(info) > 0 {
		a.Info = info[0]
	}
	b = &PipedConn{Info: "piped"}
	if len(info) > 1 {
		b.Info = info[1]
	}
	a.Conn, b.Conn = net.Pipe()
	return
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
	remoteArgs, err := url.ParseQuery(req.RemoteAddr)
	if err != nil {
		WarnLog("WebdavHandler parset remote address %v fail with %v", req.RemoteAddr, err)
		resp.WriteHeader(404)
		fmt.Fprintf(resp, "%v", err)
		return
	}
	dir := req.URL.Query().Get("dir")
	if uriStr := remoteArgs.Get("uri"); len(uriStr) > 0 {
		uri, err := url.Parse(uriStr)
		if err != nil {
			WarnLog("WebdavHandler parset uri %v fail with %v", uri, err)
			resp.WriteHeader(404)
			fmt.Fprintf(resp, "%v", err)
			return
		}
		dir = uri.Query().Get("dir")
	}
	if len(dir) < 1 {
		err = fmt.Errorf("the dir argument is required")
		WarnLog("WebdavHandler parset remote address %v fail with %v", req.RemoteAddr, err)
		resp.WriteHeader(http.StatusBadRequest)
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
