package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/bsck"
	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/xmap"
)

//Web is pojo for web configure
type Web struct {
	Suffix string `json:"suffix"`
	Listen string `json:"listen"`
	Auth   string `json:"auth"`
}

//Config is pojo for all configure
type Config struct {
	Name      string                `json:"name"`
	Cert      string                `json:"cert"`
	Key       string                `json:"key"`
	Listen    string                `json:"listen"`
	ACL       map[string]string     `json:"acl"`
	Socks5    string                `json:"socks5"`
	Web       Web                   `json:"web"`
	ShowLog   int                   `json:"showlog"`
	LogFlags  int                   `json:"logflags"`
	LogOut    string                `json:"log_out"`
	LogErr    string                `json:"log_err"`
	Forwards  map[string]string     `json:"forwards"`
	Channels  []*bsck.ChannelOption `json:"channels"`
	Dialer    xmap.M                `json:"dialer"`
	Proxy     xmap.M                `json:"proxy"`
	Reconnect int64                 `json:"reconnect"`
	RDPDir    string                `json:"rdp_dir"`
	VNCDir    string                `json:"vnc_dir"`
}

//Version is bsrouter version
const Version = "1.4.2"

//RDPTmpl is the template string for rdp file
const RDPTmpl = `
screen mode id:i:2
use multimon:i:1
session bpp:i:24
full address:s:%v
audiomode:i:0
username:s:%v
disable wallpaper:i:0
disable full window drag:i:0
disable menu anims:i:0
disable themes:i:0
alternate shell:s:
shell working directory:s:
authentication level:i:2
connect to console:i:0
gatewayusagemethod:i:0
disable cursor setting:i:0
allow font smoothing:i:1
allow desktop composition:i:1
redirectprinters:i:0
prompt for credentials on client:i:0
bookmarktype:i:3
use redirection server name:i:0
`

//VNCTmpl is the template string for vnc file
const VNCTmpl = `
FriendlyName=%v
FullScreen=1
Host=%v
Password=%v
RelativePtr=0
Scaling=100%%
`

func readConfig(path string) (config *Config, last int64, err error) {
	var data []byte
	var dataFile *os.File
	var dataStat os.FileInfo
	dataFile, err = os.Open(path)
	if err == nil {
		dataStat, err = dataFile.Stat()
		if err == nil {
			last = dataStat.ModTime().Local().UnixNano() / 1e6
			data, err = ioutil.ReadAll(dataFile)
		}
		dataFile.Close()
	}
	if err == nil {
		user, _ := user.Current()
		config = &Config{
			RDPDir: filepath.Join(user.HomeDir, "Desktop"),
			VNCDir: filepath.Join(user.HomeDir, "Desktop"),
		}
		err = json.Unmarshal(data, config)
	}
	return
}

var exitf = func(code int) {
	// if smartio.Stdout != nil {
	// 	smartio.Stdout.Flush()
	// }
	// if smartio.Stderr != nil {
	// 	smartio.Stderr.Flush()
	// }
	os.Exit(code)
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Println(Version)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "-h" {
		fmt.Fprintf(os.Stderr, "Bond Socket Router Version %v\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:  %v configure\n", "bsrouter")
		fmt.Fprintf(os.Stderr, "        %v /etc/bsrouter.json'\n", "bsrouter")
		fmt.Fprintf(os.Stderr, "bsrouter options:\n")
		fmt.Fprintf(os.Stderr, "        name\n")
		fmt.Fprintf(os.Stderr, "             the router name\n")
		fmt.Fprintf(os.Stderr, "        listen\n")
		fmt.Fprintf(os.Stderr, "             the master listen address\n")
		fmt.Fprintf(os.Stderr, "        forwards\n")
		fmt.Fprintf(os.Stderr, "             the forward uri by 'listen address':'uri'\n")
		fmt.Fprintf(os.Stderr, "        showlog\n")
		fmt.Fprintf(os.Stderr, "             the log level\n")
		fmt.Fprintf(os.Stderr, "        channels\n")
		fmt.Fprintf(os.Stderr, "             the channel configure\n")
		fmt.Fprintf(os.Stderr, "        channels.token\n")
		fmt.Fprintf(os.Stderr, "             the auth token to master\n")
		fmt.Fprintf(os.Stderr, "        channels.local\n")
		fmt.Fprintf(os.Stderr, "             the binded local address connect to master\n")
		fmt.Fprintf(os.Stderr, "        channels.remote\n")
		fmt.Fprintf(os.Stderr, "             the master address\n")
		os.Exit(1)
	}
	var config *Config
	var configLast int64
	var configPath string
	var err error
	if len(os.Args) > 1 {
		configPath = os.Args[1]
		config, configLast, err = readConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config from %v fail with %v\n", os.Args[1], err)
			exitf(1)
		}
	} else {
		u, _ := user.Current()
		for _, path := range []string{"./.bsrouter.json", "./bsrouter.json", u.HomeDir + "/.bsrouter/bsrouter.json", u.HomeDir + "/.bsrouter.json", "/etc/bsrouter/bsrouter.json", "/etc/bsrouer.json"} {
			config, configLast, err = readConfig(path)
			if err == nil || configLast > 0 {
				configPath = path
				fmt.Printf("bsrouter using config from %v\n", path)
				break
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config from .bsrouter.json or ~/.bsrouter.json or /etc/bsrouter/bsrouter.json or /etc/bsrouter.json fail with %v\n", err)
			exitf(1)
		}
	}
	// if len(config.LogOut) > 0 || len(config.LogErr) > 0 {
	// 	log.Redirect(config.LogOut, config.LogErr)
	// 	bsck.Log.SetOutput(os.Stdout)
	// }
	// if config.LogFlags > 0 {
	// 	bsck.Log.SetFlags(config.LogFlags)
	// 	log.SharedLogger().SetFlags(config.LogFlags)
	// }
	bsck.ShowLog = config.ShowLog
	socks5 := bsck.NewSocksProxy()
	forward := bsck.NewForward()
	handler := bsck.NewNormalAcessHandler(config.Name, nil)
	node := bsck.NewProxy(config.Name, handler)
	if !filepath.IsAbs(config.Cert) {
		config.Cert = filepath.Join(filepath.Dir(configPath), config.Cert)
	}
	if !filepath.IsAbs(config.Key) {
		config.Key = filepath.Join(filepath.Dir(configPath), config.Key)
	}
	node.Cert, node.Key = config.Cert, config.Key
	if config.Reconnect > 0 {
		node.ReconnectDelay = time.Duration(config.Reconnect) * time.Millisecond
	}
	if len(config.ACL) > 0 {
		handler.LoginAccess = config.ACL
	}
	dialer.NewDialer = func(t string) dialer.Dialer {
		if t == "router" {
			return NewRouterDialer(node.Router)
		}
		return dialer.DefaultDialerCreator(t)
	}
	dialerPool := dialer.NewPool()
	dialerPool.AddDialer(dialer.NewStateDialer("router", node.Router))
	err = dialerPool.Bootstrap(config.Dialer)
	if err != nil {
		bsck.ErrorLog("boot dialer pool fail with %v\n", err)
		exitf(1)
	}
	dialerProxy := dialer.NewBalancedDialer()
	if config.Proxy != nil && len(config.Proxy.Str("id")) > 0 {
		err = dialerProxy.Bootstrap(config.Proxy)
		if err != nil {
			bsck.ErrorLog("boot proxy dialer fail with %v\n", err)
			os.Exit(1)
		}
	}
	var protocolMatcher = regexp.MustCompile("^[A-Za-z0-9]*://.*$")
	var dialerAll = func(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
		if strings.Contains(uri, "->") {
			sid, err = node.Dial(uri, raw)
			return
		}
		if protocolMatcher.MatchString(uri) {
			sid = node.UniqueSid()
			_, err = dialerPool.Dial(sid, uri, raw)
			return
		}
		parts := strings.SplitN(uri, "?", 2)
		router := forward.FindForward(parts[0])
		if len(router) < 1 {
			err = fmt.Errorf("forward not found by %v", uri)
			return
		}
		dialURI := router[1]
		if len(parts) > 1 {
			if strings.Contains(dialURI, "?") {
				dialURI += "&" + parts[1]
			} else {
				dialURI += "?" + parts[1]
			}
		}
		sid, err = node.Dial(dialURI, raw)
		return
	}
	socks5.Dialer = func(utype int, uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
		switch utype {
		case bsck.SocksUriTypeBS:
			sid, err = dialerAll(uri, raw)
		default:
			sid = node.UniqueSid()
			_, err = dialerProxy.Dial(sid, "tcp://"+uri, raw)
		}
		return
	}
	forward.Dialer = dialerAll
	handler.Dialer = bsck.DialRawF(func(sid uint64, uri string) (conn bsck.Conn, err error) {
		raw, err := dialerPool.Dial(sid, uri, nil)
		if err == nil {
			conn = bsck.NewRawConn(raw, node.BufferSize, sid, uri)
		}
		return
	})
	var confLck = sync.RWMutex{}
	var addFoward = func(loc, uri string) (err error) {
		locParts := strings.SplitN(loc, "~", 2)
		if len(locParts) < 2 {
			err = fmt.Errorf("local uri must be alias~*://*, but %v", loc)
			return
		}
		target, err := url.Parse(locParts[1])
		if err != nil {
			return
		}
		var rdp, vnc bool
		var listener net.Listener
		hostParts := strings.SplitAfterN(target.Host, ":", 2)
		if len(hostParts) < 2 {
			target.Host += ":0"
		}
		switch target.Scheme {
		case "socks":
			target.Scheme = "socks"
			listener, err = node.StartForward(locParts[0], target, uri)
		case "tcp":
			target.Scheme = "tcp"
			listener, err = node.StartForward(locParts[0], target, uri)
		case "rdp":
			rdp = true
			target.Scheme = "tcp"
			listener, err = node.StartForward(locParts[0], target, uri)
		case "vnc":
			vnc = true
			target.Scheme = "tcp"
			listener, err = node.StartForward(locParts[0], target, uri)
		case "web":
			fallthrough
		case "ws":
			fallthrough
		case "wss":
			target.Host = locParts[0]
			err = forward.AddForward(target.String(), uri)
		default:
			err = fmt.Errorf("not supported scheme %v", target.Scheme)
		}
		if err == nil && rdp && len(config.RDPDir) > 0 {
			confLck.Lock()
			fileData := fmt.Sprintf(RDPTmpl, listener.Addr(), target.User.Username())
			os.MkdirAll(config.RDPDir, os.ModePerm)
			savepath := filepath.Join(config.RDPDir, locParts[0]+".rdp")
			err := ioutil.WriteFile(savepath, []byte(fileData), os.ModePerm)
			confLck.Unlock()
			if err != nil {
				bsck.WarnLog("bsrouter save rdp info to %v faile with %v", savepath, err)
			} else {
				bsck.InfoLog("bsrouter save rdp info to %v success", savepath)
			}
		}
		if err == nil && vnc && len(config.VNCDir) > 0 {
			confLck.Lock()
			password, _ := target.User.Password()
			fileData := fmt.Sprintf(VNCTmpl, locParts[0], listener.Addr(), password)
			os.MkdirAll(config.VNCDir, os.ModePerm)
			savepath := filepath.Join(config.VNCDir, locParts[0]+".vnc")
			err := ioutil.WriteFile(savepath, []byte(fileData), os.ModePerm)
			confLck.Unlock()
			if err != nil {
				bsck.WarnLog("bsrouter save vnc info to %v faile with %v", savepath, err)
			} else {
				bsck.InfoLog("bsrouter save vnc info to %v success", savepath)
			}
		}
		return
	}
	var removeFoward = func(loc string) (err error) {
		locParts := strings.SplitN(loc, "~", 2)
		if len(locParts) < 2 {
			err = fmt.Errorf("local uri must be alias~*://*, but %v", loc)
			return
		}
		target, err := url.Parse(locParts[1])
		if err != nil {
			return
		}
		var rdp, vnc bool
		switch target.Scheme {
		case "socks":
			err = node.StopForward(locParts[0])
		case "tcp":
			err = node.StopForward(locParts[0])
		case "rdp":
			rdp = true
			err = node.StopForward(locParts[0])
		case "vnc":
			vnc = true
			err = node.StopForward(locParts[0])
		default:
			err = forward.RemoveForward(locParts[0])
		}
		if rdp && len(config.RDPDir) > 0 {
			confLck.Lock()
			savepath := filepath.Join(config.RDPDir, locParts[0]+".rdp")
			err := os.Remove(savepath)
			confLck.Unlock()
			if err != nil {
				bsck.WarnLog("bsrouter remove rdp file on %v faile with %v", savepath, err)
			} else {
				bsck.InfoLog("bsrouter remove rdp file on %v success", savepath)
			}
		}
		if vnc && len(config.VNCDir) > 0 {
			confLck.Lock()
			savepath := filepath.Join(config.VNCDir, locParts[0]+".vnc")
			err := os.Remove(savepath)
			confLck.Unlock()
			if err != nil {
				bsck.WarnLog("bsrouter remove vnc file on %v faile with %v", savepath, err)
			} else {
				bsck.InfoLog("bsrouter remove vnc file on %v success", savepath)
			}
		}
		return
	}
	//
	//
	if len(config.Listen) > 0 {
		err := node.ListenMaster(config.Listen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start master on %v fail with %v\n", config.Listen, err)
			os.Exit(1)
		}
	}
	if len(config.Channels) > 0 {
		go node.LoginChannel(true, config.Channels...)
	}
	for loc, uri := range config.Forwards {
		err = addFoward(loc, uri)
		if err != nil {
			bsck.ErrorLog("bsrouter add forward by %v->%v fail with %v", loc, uri, err)
			// os.Exit(1)
		}
	}
	node.StartHeartbeat()
	if len(config.Socks5) > 0 {
		err := socks5.Start(config.Socks5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start socks5 by %v fail with %v\n", config.Socks5, err)
			os.Exit(1)
		}
	}
	http.HandleFunc("/dav/", forward.ProcWebSubsH)
	http.HandleFunc("/ws/", forward.ProcWebSubsH)
	http.HandleFunc("/", forward.HostForwardF)
	forward.WebAuth = config.Web.Auth
	forward.WebSuffix = config.Web.Suffix
	server := &http.Server{Addr: config.Web.Listen}
	if len(config.Web.Listen) > 0 {
		go func() {
			bsck.Log.Printf("I bsrouter listen web on %v\n", config.Web.Listen)
			fmt.Println(server.ListenAndServe())
		}()
	}
	go func() { //auto reload forward
		for {
			time.Sleep(3 * time.Second)
			fileInfo, err := os.Stat(configPath)
			if err != nil {
				bsck.DebugLog("bsrouter reload configure fail with %v", err)
				break
			}
			newLast := fileInfo.ModTime().Local().UnixNano() / 1e6
			if newLast == configLast {
				continue
			}
			newConfig, newLast, err := readConfig(configPath)
			if err != nil {
				bsck.DebugLog("bsrouter reload configure fail with %v", err)
				break
			}
			bsck.DebugLog("bsrouter will reload modified configure %v by old(%v),new(%v)", configPath, configLast, newLast)
			//remove missing
			for loc := range config.Forwards {
				if _, ok := newConfig.Forwards[loc]; ok {
					continue
				}
				err = removeFoward(loc)
				if err != nil {
					bsck.ErrorLog("bsrouter remove forward by %v fail with %v", loc, err)
					// os.Exit(1)
				}
			}
			for loc, uri := range newConfig.Forwards {
				if config.Forwards[loc] == uri {
					continue
				}
				err = addFoward(loc, uri)
				if err != nil {
					bsck.ErrorLog("bsrouter add forward by %v->%v fail with %v", loc, uri, err)
					// os.Exit(1)
				}
			}
			config.Forwards = newConfig.Forwards
			configLast = newLast
		}
	}()
	wc := make(chan os.Signal, 1)
	signal.Notify(wc, os.Interrupt, os.Kill)
	<-wc
	fmt.Println("clear bsrouter...")
	node.Close()
	server.Close()
}

//RouterDialer is an impl for dialer.Dialder to dialer the connection by bsck.Router.
type RouterDialer struct {
	basic   *bsck.Router
	ID      string
	conf    xmap.M
	matcher *regexp.Regexp
	router  string
	args    string
}

//NewRouterDialer is default creator by basic Router.
func NewRouterDialer(basic *bsck.Router) *RouterDialer {
	return &RouterDialer{
		basic:   basic,
		conf:    xmap.M{},
		matcher: regexp.MustCompile("^.*$"),
	}
}

//Name return dialer name.
func (r *RouterDialer) Name() string {
	return r.ID
}

//Bootstrap will initial dialer
func (r *RouterDialer) Bootstrap(options xmap.M) (err error) {
	r.conf = options
	r.ID = options.Str("id")
	if len(r.ID) < 1 {
		return fmt.Errorf("the dialer id is required")
	}
	matcher := options.Str("matcher")
	if len(matcher) > 0 {
		r.matcher, err = regexp.Compile(matcher)
	}
	r.router = options.Str("router")
	if len(r.router) < 1 {
		err = fmt.Errorf("the dialer router is required")
	}
	r.args = options.Str("args")
	return
}

//Options return current configure
func (r *RouterDialer) Options() xmap.M {
	return r.conf
}

//Matched will verify uri is matched
func (r *RouterDialer) Matched(uri string) bool {
	return r.matcher.MatchString(uri)
}

//Dial to uri by router
func (r *RouterDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (rw dialer.Conn, err error) {
	if pipe == nil {
		err = fmt.Errorf("pipe is nil")
		return
	}
	sid, err = r.basic.SyncDial(strings.Replace(r.router, "${URI}", uri, -1), NewNotCloseRWC(pipe))
	return
}

//Pipe to uri by router
func (r *RouterDialer) Pipe(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	err = fmt.Errorf("not supported")
	return
}

func (r *RouterDialer) String() string {
	return r.ID
}

//NotCloseRWC is struct to prevent close base ReadWriteCloser
type NotCloseRWC struct {
	io.ReadWriteCloser
}

//NewNotCloseRWC is default creator by base NewNotCloseRWC
func NewNotCloseRWC(base io.ReadWriteCloser) *NotCloseRWC {
	return &NotCloseRWC{
		ReadWriteCloser: base,
	}
}

//Close is imple to io.Closer
func (n *NotCloseRWC) Close() (err error) {
	return
}
