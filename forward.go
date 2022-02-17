package bsck

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/xmap"
	"golang.org/x/net/websocket"
)

type ForwardUri []string

func (f ForwardUri) String() string {
	return strings.Join(f, "->")
}

type forwardTransport http.Transport

func (f *forwardTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	req.Close = true
	transport := (*http.Transport)(f)
	res, err = transport.RoundTrip(req)
	return
}

type Forward struct {
	webMapping map[string]ForwardUri
	wsMapping  map[string]ForwardUri
	lck        sync.RWMutex
	WebSuffix  string
	WebAuth    string
	Dialer     func(uri string, raw io.ReadWriteCloser) (sid uint64, err error)
}

func NewForward() *Forward {
	return &Forward{
		webMapping: map[string]ForwardUri{},
		wsMapping:  map[string]ForwardUri{},
		lck:        sync.RWMutex{},
		// Dialer:     dialer,
	}
}

func (f *Forward) ProcWebSubsH(w http.ResponseWriter, req *http.Request) {
	parts := strings.SplitN(req.URL.Path, "/", 4)
	req.URL.Path = "/"
	if len(parts) == 4 {
		req.URL.Path = "/" + parts[3]
	}
	if len(parts) > 2 && len(parts[2]) > 0 {
		f.ProcName(parts[2], w, req)
		return
	}
	router := req.URL.Query().Get("router")
	if len(router) < 1 {
		w.WriteHeader(404)
		fmt.Fprintf(w, "router argument is not found")
		return
	}
	f.ProcRouter([]string{"loc://args", router}, w, req)
}

func (f *Forward) HostForwardF(w http.ResponseWriter, req *http.Request) {
	host := req.Host
	if len(f.WebSuffix) > 0 && strings.HasSuffix(host, f.WebSuffix) {
		name := strings.Trim(strings.TrimSuffix(host, f.WebSuffix), ". ")
		if len(name) > 0 {
			f.ProcName(name, w, req)
			return
		}
	}
	w.WriteHeader(404)
	fmt.Fprintf(w, "%v is not supported\n", req.URL.Path)
}

func (f *Forward) ProcName(name string, w http.ResponseWriter, req *http.Request) {
	connection := req.Header.Get("Connection")
	DebugLog("Forward proc web by name(%v),Connection(%v)", name, connection)
	var router ForwardUri
	f.lck.RLock()
	if connection == "Upgrade" {
		router = f.wsMapping[name]
	} else {
		router = f.webMapping[name]
	}
	f.lck.RUnlock()
	if len(router) < 1 {
		w.WriteHeader(404)
		WarnLog("Forward proc web by name(%v),Connection(%v) fail with not found", name, connection)
		fmt.Fprintf(w, "alias not exist by name:%v", name)
		return
	}
	f.ProcRouter(router, w, req)
}

func (f *Forward) ProcRouter(router ForwardUri, w http.ResponseWriter, req *http.Request) {
	connection := req.Header.Get("Connection")
	local, err := url.Parse(router[0])
	if err != nil {
		w.WriteHeader(500)
		WarnLog("Forward proc web by router(%v),Connection(%v) fail with %v", router, connection, err)
		fmt.Fprintf(w, "Error:%v", err)
		return
	}
	if len(f.WebAuth) > 0 && local.Query().Get("auth") != "0" {
		username, password, ok := req.BasicAuth()
		if !(ok && f.WebAuth == fmt.Sprintf("%v:%s", username, password)) {
			w.Header().Set("WWW-Authenticate", "Basic realm=Reverse Server")
			w.WriteHeader(401)
			fmt.Fprintf(w, "%v", "401 Unauthorized")
			return
		}
	}
	remoteURI := router[1]
	remoteURI = xmap.ReplaceAll(func(key string) interface{} { return req.URL.Query().Get(key) }, remoteURI, true, true)
	parts := strings.Split(remoteURI, "->")
	remote, err := url.Parse(parts[len(parts)-1])
	if err != nil {
		w.WriteHeader(500)
		WarnLog("Forward proc web by router(%v),Connection(%v) fail with parse remote error %v", router, connection, err)
		fmt.Fprintf(w, "Error:%v", err)
		return
	}
	if connection == "Upgrade" {
		websocket.Handler(func(conn *websocket.Conn) {
			f.runWebsocket(conn, remoteURI)
		}).ServeHTTP(w, req)
		return
	}
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Host = req.Host
			req.URL.Scheme = remote.Scheme
		},
		Transport: &forwardTransport{
			Dial: func(network, addr string) (raw net.Conn, err error) {
				return f.procDial(network, addr, remoteURI)
			},
			DialTLS: func(network, addr string) (raw net.Conn, err error) {
				return f.procDialTLS(network, addr, remoteURI)
			},
		},
	}
	proxy.ServeHTTP(w, req)
}

func (f *Forward) runWebsocket(conn *websocket.Conn, router string) {
	if strings.Contains(router, "?") {
		router += "&" + conn.Request().URL.RawQuery
	} else {
		router += "?" + conn.Request().URL.RawQuery
	}
	wait := NewWaitReadWriteCloser(conn)
	_, err := f.Dialer(router, wait)
	if err != nil {
		InfoLog("Forward proxy %v to %v fail with %v", conn.RemoteAddr(), router, err)
		wait.Close()
	}
	wait.Wait()
}

func (f *Forward) procDial(network, addr string, router string) (raw net.Conn, err error) {
	raw, piped := dialer.CreatePipedConn()
	_, err = f.Dialer(router, piped)
	if err != nil {
		raw.Close()
		piped.Close()
	}
	return
}

func (f *Forward) procDialTLS(network, addr string, router string) (raw net.Conn, err error) {
	rawConn, piped := dialer.CreatePipedConn()
	_, err = f.Dialer(router, piped)
	if err != nil {
		rawConn.Close()
		piped.Close()
	}
	if err != nil {
		return
	}
	tlsConn := tls.Client(rawConn, &tls.Config{InsecureSkipVerify: true})
	err = tlsConn.Handshake()
	if err == nil {
		raw = tlsConn
	} else {
		rawConn.Close()
		tlsConn.Close()
	}
	return
}

//AddForward by local uri and remote uri
func (f *Forward) AddForward(loc, uri string) (err error) {
	f.lck.Lock()
	defer f.lck.Unlock()
	forward := ForwardUri([]string{loc, uri})
	local, err := url.Parse(loc)
	if err != nil {
		return
	}
	if f.webMapping[local.Host] != nil || f.wsMapping[local.Host] != nil {
		err = fmt.Errorf("alias(%v) is exist", local.Host)
		return
	}
	switch local.Scheme {
	case "web":
		f.webMapping[local.Host] = forward
		InfoLog("Forward add web forward by %v:%v", loc, uri)
	case "ws":
		fallthrough
	case "wss":
		f.wsMapping[local.Host] = forward
		InfoLog("Forward add ws forward by %v:%v", loc, uri)
	default:
		err = fmt.Errorf("scheme %v is not suppored", local.Scheme)
	}
	return
}

//RemoveForward by alias name
func (f *Forward) RemoveForward(name string) (err error) {
	f.lck.Lock()
	defer f.lck.Unlock()
	delete(f.webMapping, name)
	delete(f.wsMapping, name)
	return
}

//FindForward will return the forward
func (f *Forward) FindForward(name string) (uri ForwardUri) {
	f.lck.Lock()
	defer f.lck.Unlock()
	uri, ok := f.wsMapping[name]
	if !ok {
		uri = f.webMapping[name]
	}
	return
}

type WaitReadWriteCloser struct {
	io.ReadWriteCloser
	wc     chan int
	closed uint32
}

func NewWaitReadWriteCloser(raw io.ReadWriteCloser) *WaitReadWriteCloser {
	return &WaitReadWriteCloser{
		ReadWriteCloser: raw,
		wc:              make(chan int),
	}
}

func (w *WaitReadWriteCloser) Close() (err error) {
	if atomic.CompareAndSwapUint32(&w.closed, 0, 1) {
		err = w.ReadWriteCloser.Close()
		close(w.wc)
	}
	return
}

func (w *WaitReadWriteCloser) Wait() {
	if atomic.LoadUint32(&w.closed) == 0 {
		<-w.wc
	}
}

func (w *WaitReadWriteCloser) String() string {
	if conn, ok := w.ReadWriteCloser.(net.Conn); ok {
		return conn.RemoteAddr().String()
	}
	return fmt.Sprintf("%v", w.ReadWriteCloser)
}
