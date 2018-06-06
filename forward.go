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

	"github.com/sutils/dialer"
	"golang.org/x/net/websocket"
)

type ForwardUri []string

func (f ForwardUri) URL() (local *url.URL, remote *url.URL, err error) {
	local, err = url.Parse(f[0])
	if err == nil {
		parts := strings.Split(f[1], "->")
		remote, err = url.Parse(parts[len(parts)-1])
	}
	return
}

func (f ForwardUri) String() string {
	return strings.Join(f, "->")
}

type Forward struct {
	webMapping map[string]ForwardUri
	wsMapping  map[string]ForwardUri
	lck        sync.RWMutex
	WebSuffix  string
	WebAuth    string
	Dailer     func(uri string, raw io.ReadWriteCloser) (sid uint64, err error)
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
	local, remote, err := router.URL()
	if err != nil {
		w.WriteHeader(500)
		WarnLog("Forward proc web by router(%v),Connection(%v) fail with %v", router, connection, err)
		fmt.Fprintf(w, "Error:%v", err)
		return
	}
	if connection == "Upgrade" {
		websocket.Handler(func(conn *websocket.Conn) {
			f.runWebsocket(conn, router[1])
		}).ServeHTTP(w, req)
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
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Host = req.Host
			req.URL.Scheme = remote.Scheme
		},
		Transport: &http.Transport{
			Dial: func(network, addr string) (raw net.Conn, err error) {
				return f.procDial(network, addr, router[1])
			},
			DialTLS: func(network, addr string) (raw net.Conn, err error) {
				return f.procDialTLS(network, addr, router[1])
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
	_, err := f.Dailer(router, wait)
	if err != nil {
		InfoLog("Forward proxy %v to %v fail with %v", conn.RemoteAddr(), router, err)
		wait.Close()
	}
	wait.Wait()
}

func (f *Forward) procDial(network, addr string, router string) (raw net.Conn, err error) {
	raw, piped, err := dialer.CreatePipedConn()
	if err == nil {
		_, err = f.Dailer(router, piped)
	}
	return
}

func (f *Forward) procDialTLS(network, addr string, router string) (raw net.Conn, err error) {
	rawConn, piped, err := dialer.CreatePipedConn()
	if err == nil {
		_, err = f.Dailer(router, piped)
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

func (f *Forward) AddForward(loc, uri string) (err error) {
	f.lck.Lock()
	defer f.lck.Unlock()
	forward := ForwardUri([]string{loc, uri})
	local, _, err := forward.URL()
	if err != nil {
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

func (f *Forward) RemoveForward(local string) (err error) {
	rurl, err := url.Parse(local)
	if err != nil {
		return
	}
	f.lck.Lock()
	defer f.lck.Unlock()
	switch rurl.Scheme {
	case "web":
		delete(f.webMapping, rurl.Host)
		InfoLog("Forward remove web forward by %v", local)
	case "ws":
		fallthrough
	case "wss":
		delete(f.wsMapping, rurl.Host)
		InfoLog("Forward remove ws forward by %v", local)
	default:
		err = fmt.Errorf("scheme %v is not suppored", rurl.Scheme)
	}
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
