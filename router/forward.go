package router

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/codingeasygo/bsck/dialer"
	sproxy "github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xio"
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

type WebForward struct {
	webMapping map[string]ForwardUri
	wsMapping  map[string]ForwardUri
	lck        sync.RWMutex
	WebSuffix  string
	WebAuth    string
	Dialer     func(uri string, raw io.ReadWriteCloser) (sid uint16, err error)
}

func NewWebForward() *WebForward {
	return &WebForward{
		webMapping: map[string]ForwardUri{},
		wsMapping:  map[string]ForwardUri{},
		lck:        sync.RWMutex{},
		// Dialer:     dialer,
	}
}

func (f *WebForward) ProcWebSubsH(w http.ResponseWriter, req *http.Request) {
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

func (f *WebForward) HostForwardF(w http.ResponseWriter, req *http.Request) {
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

func (f *WebForward) ProcName(name string, w http.ResponseWriter, req *http.Request) {
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

func (f *WebForward) ProcRouter(router ForwardUri, w http.ResponseWriter, req *http.Request) {
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

func (f *WebForward) runWebsocket(conn *websocket.Conn, router string) {
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

func (f *WebForward) procDial(network, addr string, router string) (raw net.Conn, err error) {
	raw, piped := dialer.CreatePipedConn()
	_, err = f.Dialer(router, piped)
	if err != nil {
		raw.Close()
		piped.Close()
	}
	return
}

func (f *WebForward) procDialTLS(network, addr string, router string) (raw net.Conn, err error) {
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

// AddForward by local uri and remote uri
func (f *WebForward) AddForward(loc, uri string) (err error) {
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

// RemoveForward by alias name
func (f *WebForward) RemoveForward(name string) (err error) {
	f.lck.Lock()
	defer f.lck.Unlock()
	delete(f.webMapping, name)
	delete(f.wsMapping, name)
	return
}

// FindForward will return the forward
func (f *WebForward) FindForward(name string) (uri ForwardUri) {
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

func ipAddrPrefix(ip net.IP, mask net.IPMask) (prefix net.IP) {
	prefix = make(net.IP, len(mask))
	if len(prefix) == 4 {
		copy(prefix, ip.To4())
	} else {
		copy(prefix, ip.To16())
	}
	for i := 0; i < len(prefix); i++ {
		prefix[i] = prefix[i] & mask[i]
	}
	return
}

func ipAddrSuffix(ip net.IP, mask net.IPMask) (suffix net.IP) {
	suffix = make(net.IP, len(mask))
	if len(suffix) == 4 {
		copy(suffix, ip.To4())
	} else {
		copy(suffix, ip.To16())
	}
	for i := 0; i < len(suffix); i++ {
		suffix[i] = suffix[i] & (^mask[i])
	}
	return
}

func ipAddrMerge(prefix, suffix net.IP) (ip net.IP) {
	ip = make(net.IP, len(prefix))
	for i := 0; i < len(ip); i++ {
		ip[i] = prefix[i] | suffix[i]
	}
	return
}

type HostItem struct {
	LocalAddr  net.IP
	LocalNet   *net.IPNet
	LocalMask  int
	LocalGW    net.IP
	URI        string
	RemoteNet  *net.IPNet
	RemoteMask int
}

func (h *HostItem) Match(ip net.IP) (uri string, remote net.IP) {
	prefix := ipAddrPrefix(ip, h.LocalNet.Mask)
	if h.LocalNet.IP.Equal(prefix) {
		uri = h.URI
		suffix := ipAddrSuffix(ip, h.RemoteNet.Mask)
		remote = ipAddrMerge(h.RemoteNet.IP, suffix)
	}
	return
}

type HostMap struct {
	items map[string]*HostItem
	lock  sync.RWMutex
}

func NewHostMap() (host *HostMap) {
	host = &HostMap{
		items: map[string]*HostItem{},
		lock:  sync.RWMutex{},
	}
	return
}

// AddForward by local uri and remote uri
func (h *HostMap) AddForward(name, loc, uri string) (item *HostItem, err error) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.items[name] != nil {
		err = fmt.Errorf("forward %v is exists", name)
		return
	}
	localAddr, localNet, err := net.ParseCIDR(strings.TrimPrefix(loc, "host://"))
	if err != nil {
		return
	}
	localGW := ipAddrPrefix(localAddr, localNet.Mask)
	localGW[len(localGW)-1] = 1
	localMask, _ := localNet.Mask.Size()
	uriParts := strings.Split(uri, "->")
	_, remoteNet, err := net.ParseCIDR(strings.TrimPrefix(uriParts[len(uriParts)-1], "host://"))
	if err != nil {
		return
	}
	remoteMask, _ := remoteNet.Mask.Size()
	item = &HostItem{
		LocalAddr:  localAddr,
		LocalNet:   localNet,
		LocalMask:  localMask,
		LocalGW:    localGW,
		URI:        strings.Join(uriParts[:len(uriParts)-1], "->"),
		RemoteNet:  remoteNet,
		RemoteMask: remoteMask,
	}
	h.items[name] = item
	return
}

// RemoveForward by alias name
func (h *HostMap) RemoveForward(name string) (item *HostItem) {
	h.lock.Lock()
	defer h.lock.Unlock()
	item = h.items[name]
	delete(h.items, name)
	return
}

func (h *HostMap) Match(ip net.IP) (uri string, remote net.IP) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	for _, item := range h.items {
		uri, remote = item.Match(ip)
		if remote != nil {
			break
		}
	}
	return
}

type HostForward struct {
	Name    string
	Dialer  xio.PiperDialer
	console *sproxy.Server
	runner  *exec.Cmd
	sender  net.Conn
	ready   chan int
	lock    sync.RWMutex
}

func NewHostForward(name string, dialer xio.PiperDialer) (forward *HostForward) {
	forward = &HostForward{
		Name:   name,
		Dialer: dialer,
		ready:  make(chan int, 1),
		lock:   sync.RWMutex{},
	}
	return
}

func (h *HostForward) checkStart() (err error) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.runner != nil {
		return
	}
	InfoLog("HostForward(%v) host backend forward is starting", h.Name)
	console := sproxy.NewServer()
	console.Dialer = h
	ln, _ := console.Start("127.0.0.1:0")
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	exe := filepath.Join(dir, "bsconsole")
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("osascript", "-e", fmt.Sprintf(`do shell script "BS_CONSOLE_URI=127.0.0.1:%v BS_CONSOLE_CMD=1 %v host" with administrator privileges`, ln.Addr().(*net.TCPAddr).Port, exe))
		// cmd = exec.Command("bash", "-c", fmt.Sprintf("sudo -E %v host", exe))
	default:
		cmd = exec.Command("bash", "-c", fmt.Sprintf("sudo -E %v host", exe))
	}
	cmd.Dir, _ = os.Getwd()
	cmd.Env = append(cmd.Env, fmt.Sprintf("BS_CONSOLE_URI=127.0.0.1:%v", ln.Addr().(*net.TCPAddr).Port))
	cmd.Env = append(cmd.Env, "BS_CONSOLE_CMD=1")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Start()
	if err != nil {
		WarnLog("HostForward(%v) host backend forward start error %v", h.Name, err)
		return
	}
	exiter := make(chan int, 1)
	go func() {
		err := cmd.Wait()
		WarnLog("HostForward(%v) host backend forward is stopped by error %v", h.Name, err)
		exiter <- 1
	}()
	select {
	case <-h.ready:
		h.console, h.runner = console, cmd
		InfoLog("HostForward(%v) host backend forward is ready", h.Name)
	case <-exiter:
		err = fmt.Errorf("stopped")
	}
	return
}

func (h *HostForward) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	if strings.HasPrefix(uri, "tcp://cmd") {
		if h.sender != nil {
			h.Close()
		}
		sender, conn, _ := xio.CreatePipedConn()
		raw = xio.NewCopyPiper(conn, bufferSize)
		h.sender = sender
		select {
		case h.ready <- 1:
		default:
		}
	} else {
		raw, err = h.Dialer.DialPiper(uri, bufferSize)
	}
	return
}

func (h *HostForward) Close() (err error) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.runner == nil {
		return
	}
	h.console.Stop()
	h.sender.Close()
	h.runner.Process.Kill()
	err = h.runner.Wait()
	h.console = nil
	h.sender = nil
	h.runner = nil
	return
}

// AddForward by local uri and remote uri
func (h *HostForward) AddForward(name, loc, uri string) (err error) {
	err = h.checkStart()
	if err != nil {
		return
	}
	_, err = fmt.Fprintf(h.sender, "@add %v %v %v\n", name, loc, uri)
	return
}

// RemoveForward by alias name
func (h *HostForward) RemoveForward(name string) (err error) {
	err = h.checkStart()
	if err != nil {
		return
	}
	_, err = fmt.Fprintf(h.sender, "@remove %v\n", name)
	return
}
