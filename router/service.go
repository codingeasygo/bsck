// Package bsck provider tcp socket proxy router
//
// the supported router is client->(slaver->master->slaver)*-server,
//
// the channel of slaver to master can be multi physical tcp connect by different router
package router

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/proxy"
	sproxy "github.com/codingeasygo/util/proxy/socks"
	wproxy "github.com/codingeasygo/util/proxy/ws"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
	"golang.org/x/crypto/ssh"
)

// RDPTemplate is the template string for rdp file
const RDPTemplate = `
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

// VNCTemplate is the template string for vnc file
const VNCTemplate = `
FriendlyName=%v
FullScreen=1
Host=%v
Password=%v
RelativePtr=0
Scaling=100%%
`

// Web is struct for web configure
type Web struct {
	Suffix string `json:"suffix"`
	Listen string `json:"listen"`
	Auth   string `json:"auth"`
}

// Config is struct for all configure
type Config struct {
	Name    string            `json:"name"`
	Dir     string            `json:"dir"`
	Cert    string            `json:"cert"`
	Key     string            `json:"key"`
	Listen  string            `json:"listen"`
	ACL     map[string]string `json:"acl"`
	Access  [][]string        `json:"access"`
	Console struct {
		SOCKS string `json:"socks"`
		WS    string `json:"ws"`
	} `json:"console"`
	Web       Web               `json:"web"`
	Log       int               `json:"log"`
	Forwards  map[string]string `json:"forwards"`
	Channels  []xmap.M          `json:"channels"`
	Dialer    xmap.M            `json:"dialer"`
	Reconnect int64             `json:"reconnect"`
	RDPDir    string            `json:"rdp_dir"`
	VNCDir    string            `json:"vnc_dir"`
}

// ReadConfig will read configure from file
func ReadConfig(filename string) (config *Config, last int64, err error) {
	configData, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	if stat, xerr := os.Stat(filename); xerr != nil {
		last = stat.ModTime().Local().UnixNano() / 1e6
	}
	user, _ := user.Current()
	config = &Config{}
	err = json.Unmarshal(configData, config)
	if err != nil {
		return
	}
	if len(config.Dir) < 1 {
		config.Dir = filepath.Dir(filename)
	}
	if len(config.RDPDir) < 1 {
		config.RDPDir = filepath.Join(user.HomeDir, "Desktop")
	}
	if len(config.VNCDir) < 1 {
		config.VNCDir = filepath.Join(user.HomeDir, "Desktop")
	}
	return
}

// ErrForwardNotExist is forward is not exist error
var ErrForwardNotExist = fmt.Errorf("%v", "forward not exist")

// ForwardFinder is forward finder
type ForwardFinder interface {
	FindForward(uri string) (target string, err error)
}

// ForwardFinderF is func implement ForwardFinder
type ForwardFinderF func(uri string) (target string, err error)

// FindForward is implement ForwardFinder
func (f ForwardFinderF) FindForward(uri string) (target string, err error) {
	target, err = f(uri)
	return
}

// Service is bound socket service
type Service struct {
	Name    string
	Node    *Proxy
	Console struct {
		SOCKS *sproxy.Server
		WS    *wproxy.Server
	}
	Web        net.Listener
	Forward    *Forward
	Dialer     *dialer.Pool
	Handler    Handler
	Finder     ForwardFinder
	Config     *Config
	ConfigPath string
	Client     *xhttp.Client
	Webs       map[string]http.Handler
	BufferSize int
	OnReady    func()
	configLock sync.RWMutex
	configLast int64
	alias      map[string]string
	aliasLock  sync.RWMutex
}

// NewService will return new Service
func NewService() (s *Service) {
	s = &Service{
		BufferSize: 32 * 1024,
		configLock: sync.RWMutex{},
		alias:      map[string]string{},
		aliasLock:  sync.RWMutex{},
		Webs:       map[string]http.Handler{},
	}
	client := &http.Client{
		Transport: &http.Transport{
			Dial: s.DialNet,
		},
	}
	s.Client = xhttp.NewClient(client)
	return
}

// ReloadConfig will check configure modify time and reload
func (s *Service) ReloadConfig() (err error) {
	fileInfo, err := os.Stat(s.ConfigPath)
	if err != nil {
		DebugLog("Server(%v) reload configure fail with %v", s.Name, err)
		return
	}
	newLast := fileInfo.ModTime().Local().UnixNano() / 1e6
	if newLast == s.configLast {
		return
	}
	config := s.Config
	newConfig, newLast, err := ReadConfig(s.ConfigPath)
	if err != nil {
		DebugLog("Server(%v) read configure %v fail with %v", s.Name, s.ConfigPath, err)
		return
	}
	DebugLog("Server(%v) will reload modified configure %v by old(%v),new(%v)", s.Name, s.ConfigPath, s.configLast, newLast)
	//remove missing
	for loc := range config.Forwards {
		if _, ok := newConfig.Forwards[loc]; ok {
			continue
		}
		err = s.RemoveForward(loc)
		if err != nil {
			ErrorLog("Server(%v) remove forward by %v fail with %v", s.Name, loc, err)
		}
	}
	for loc, uri := range newConfig.Forwards {
		if config.Forwards[loc] == uri {
			continue
		}
		err = s.AddForward(loc, uri)
		if err != nil {
			ErrorLog("Server(%v) add forward by %v->%v fail with %v", s.Name, loc, uri, err)
		}
	}
	config.Forwards = newConfig.Forwards
	s.configLast = newLast
	return
}

// AddForward will add forward by local and remote
func (s *Service) AddForward(loc, uri string) (err error) {
	locParts := strings.SplitN(loc, "~", 2)
	if len(locParts) < 2 {
		s.aliasLock.Lock()
		_, having := s.alias[loc]
		if !having {
			s.alias[loc] = uri
		}
		s.aliasLock.Unlock()
		if having {
			err = fmt.Errorf("local uri is exists %v", loc)
		}
		// err = fmt.Errorf("local uri must be alias~*://*, but %v", loc)
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
		listener, err = s.Node.StartForward(locParts[0], target, uri)
	case "proxy":
		target.Scheme = "proxy"
		listener, err = s.Node.StartForward(locParts[0], target, uri)
	case "tcp":
		target.Scheme = "tcp"
		listener, err = s.Node.StartForward(locParts[0], target, uri)
	case "rdp":
		rdp = true
		target.Scheme = "tcp"
		listener, err = s.Node.StartForward(locParts[0], target, uri)
	case "vnc":
		vnc = true
		target.Scheme = "tcp"
		listener, err = s.Node.StartForward(locParts[0], target, uri)
	case "web":
		fallthrough
	case "ws":
		fallthrough
	case "wss":
		target.Host = target.Hostname()
		err = s.Forward.AddForward(target.String(), uri)
	default:
		err = fmt.Errorf("not supported scheme %v", target.Scheme)
	}
	if err == nil && rdp && len(s.Config.RDPDir) > 0 {
		s.configLock.Lock()
		fileData := fmt.Sprintf(RDPTemplate, listener.Addr(), target.User.Username())
		os.MkdirAll(s.Config.RDPDir, os.ModePerm)
		savepath := filepath.Join(s.Config.RDPDir, locParts[0]+".rdp")
		err := ioutil.WriteFile(savepath, []byte(fileData), os.ModePerm)
		s.configLock.Unlock()
		if err != nil {
			WarnLog("Server(%v) save rdp info to %v failed with %v", s.Name, savepath, err)
		} else {
			InfoLog("Server(%v) save rdp info to %v success", s.Name, savepath)
		}
	}
	if err == nil && vnc && len(s.Config.VNCDir) > 0 {
		s.configLock.Lock()
		password, _ := target.User.Password()
		fileData := fmt.Sprintf(VNCTemplate, locParts[0], listener.Addr(), password)
		os.MkdirAll(s.Config.VNCDir, os.ModePerm)
		savepath := filepath.Join(s.Config.VNCDir, locParts[0]+".vnc")
		err := ioutil.WriteFile(savepath, []byte(fileData), os.ModePerm)
		s.configLock.Unlock()
		if err != nil {
			WarnLog("Server(%v) save vnc info to %v failed with %v", s.Name, savepath, err)
		} else {
			InfoLog("Server(%v) save vnc info to %v success", s.Name, savepath)
		}
	}
	return
}

// RemoveForward will remove forward
func (s *Service) RemoveForward(loc string) (err error) {
	locParts := strings.SplitN(loc, "~", 2)
	if len(locParts) < 2 {
		s.aliasLock.Lock()
		_, having := s.alias[loc]
		if having {
			delete(s.alias, loc)
		}
		s.aliasLock.Unlock()
		if !having {
			err = fmt.Errorf("local uri is not exists %v", loc)
		}
		// err = fmt.Errorf("local uri must be alias~*://*, but %v", loc)
		return
	}
	target, err := url.Parse(locParts[1])
	if err != nil {
		return
	}
	var rdp, vnc bool
	switch target.Scheme {
	case "socks":
		err = s.Node.StopForward(locParts[0])
	case "tcp":
		err = s.Node.StopForward(locParts[0])
	case "rdp":
		rdp = true
		err = s.Node.StopForward(locParts[0])
	case "vnc":
		vnc = true
		err = s.Node.StopForward(locParts[0])
	default:
		err = s.Forward.RemoveForward(locParts[0])
	}
	if rdp && len(s.Config.RDPDir) > 0 {
		s.configLock.Lock()
		savepath := filepath.Join(s.Config.RDPDir, locParts[0]+".rdp")
		err := os.Remove(savepath)
		s.configLock.Unlock()
		if err != nil {
			WarnLog("Server(%v) remove rdp file on %v failed with %v", s.Name, savepath, err)
		} else {
			InfoLog("Server(%v) remove rdp file on %v success", s.Name, savepath)
		}
	}
	if vnc && len(s.Config.VNCDir) > 0 {
		s.configLock.Lock()
		savepath := filepath.Join(s.Config.VNCDir, locParts[0]+".vnc")
		err := os.Remove(savepath)
		s.configLock.Unlock()
		if err != nil {
			WarnLog("Server(%v) remove vnc file on %v failed with %v", s.Name, savepath, err)
		} else {
			InfoLog("Server(%v) remove vnc file on %v success", s.Name, savepath)
		}
	}
	return
}

// SyncDialAll will sync dial uri by raw
func (s *Service) SyncDialAll(uris string, raw io.ReadWriteCloser) (sid uint16, err error) {
	sid, err = s.DialAll(uris, raw, true)
	return
}

// DialAll will dial uri by raw
func (s *Service) DialAll(uris string, raw io.ReadWriteCloser, sync bool) (sid uint16, err error) {
	sid, err = s.dialAll(uris, raw, sync)
	if err == nil || err != ErrForwardNotExist || s.Finder == nil {
		return
	}
	uris, err = s.Finder.FindForward(uris)
	if err == nil {
		sid, err = s.dialAll(uris, raw, sync)
	}
	return
}

func (s *Service) dialAll(uris string, raw io.ReadWriteCloser, sync bool) (sid uint16, err error) {
	DebugLog("Server(%v) try dial all to %v", s.Name, uris)
	for _, uri := range strings.Split(uris, ",") {
		sid, err = s.dialOne(uri, raw, sync)
		if err == nil {
			break
		}
	}
	return
}

func (s *Service) dialOne(uri string, raw io.ReadWriteCloser, sync bool) (sid uint16, err error) {
	DebugLog("Server(%v) try dial to %v", s.Name, uri)
	dialURI := uri
	if !strings.Contains(uri, "->") && !regexp.MustCompile("^[A-Za-z0-9]*://.*$").MatchString(uri) {
		parts := strings.SplitN(uri, "?", 2)
		s.aliasLock.Lock()
		target, ok := s.alias[parts[0]]
		s.aliasLock.Unlock()
		if !ok {
			router := s.Forward.FindForward(parts[0])
			if len(router) < 2 {
				err = ErrForwardNotExist
				return
			}
			target = router[1]
		}
		dialURI = target
		if len(parts) > 1 {
			if strings.Contains(dialURI, "?") {
				dialURI += "&" + parts[1]
			} else {
				dialURI += "?" + parts[1]
			}
		}
	}
	if sync {
		_, sid, err = s.Node.SyncDial(raw, dialURI)
	} else {
		_, sid, err = s.Node.Dial(raw, dialURI)
	}
	return
}

// DialRaw is router dial implemnet
func (s *Service) DialRawConn(channel Conn, sid uint16, uri string) (conn Conn, err error) {
	raw, err := s.Dialer.Dial(channel, sid, uri)
	if err == nil {
		conn = s.Node.NewConn(raw, sid, ConnTypeRaw)
	}
	return
}

// DialNet is net dialer to router
func (s *Service) DialNet(network, addr string) (conn net.Conn, err error) {
	addr = strings.TrimSuffix(addr, ":80")
	addr = strings.TrimSuffix(addr, ":443")
	if strings.HasPrefix(addr, "base64-") {
		var realAddr []byte
		realAddr, err = base64.RawURLEncoding.DecodeString(strings.TrimPrefix(addr, "base64-"))
		if err != nil {
			return
		}
		addr = string(realAddr)
	}
	conn, raw := dialer.CreatePipedConn()
	if err == nil {
		_, err = s.DialAll(addr, raw, true)
		if err != nil {
			conn.Close()
			raw.Close()
		}
	}
	return
}

// DialSSH is ssh dialer to ssh server
func (s *Service) DialSSH(uri string, config *ssh.ClientConfig) (client *ssh.Client, err error) {
	conn, raw := dialer.CreatePipedConn()
	if err == nil {
		_, err = s.DialAll(uri, raw, true)
		if err != nil {
			conn.Close()
			raw.Close()
		}
	}
	if err != nil {
		return
	}
	c, channel, request, err := ssh.NewClientConn(conn, uri, config)
	if err != nil {
		return nil, err
	}
	client = ssh.NewClient(c, channel, request)
	return
}

// DialPiper will dial uri on router and return piper
func (s *Service) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	piper := NewRouterPiper()
	_, err = s.DialAll(uri, piper, true)
	raw = piper
	return
}

// Start will start service
func (s *Service) Start() (err error) {
	if len(s.ConfigPath) > 0 {
		s.Config, s.configLast, err = ReadConfig(s.ConfigPath)
		if err != nil {
			ErrorLog("Server(%v) read config %v fail with %v", s.Name, s.ConfigPath, err)
			return
		}
	} else if s.Config == nil {
		err = fmt.Errorf("config is nil and configure path is empty")
		return
	}
	s.Name = s.Config.Name
	proxy.SetLogLevel(s.Config.Log)
	SetLogLevel(s.Config.Log)
	InfoLog("Server(%v) will start by config %v", s.Name, s.ConfigPath)
	s.Console.SOCKS = sproxy.NewServer()
	s.Console.SOCKS.Dialer = s
	s.Console.SOCKS.BufferSize = s.BufferSize
	s.Console.WS = wproxy.NewServer()
	s.Console.WS.Dialer = s
	s.Console.WS.BufferSize = s.BufferSize
	s.Forward = NewForward()
	if s.Handler == nil {
		handler := NewNormalAcessHandler(s.Config.Name)
		if len(s.Config.ACL) > 0 {
			handler.LoginAccess = s.Config.ACL
		}
		if len(s.Config.Access) > 0 {
			handler.DialAccess = s.Config.Access
		}
		handler.ConnDialer = DialRawConnF(s.DialRawConn)
		s.Handler = handler
	}
	s.Node = NewProxy(s.Config.Name, s.Handler)
	s.Node.Dir = s.Config.Dir
	s.Node.Router.BufferSize = s.BufferSize
	s.Node.Forward.BufferSize = s.BufferSize
	if len(s.Config.Cert) > 0 && !filepath.IsAbs(s.Config.Cert) {
		s.Config.Cert = filepath.Join(filepath.Dir(s.ConfigPath), s.Config.Cert)
	}
	if len(s.Config.Key) > 0 && !filepath.IsAbs(s.Config.Key) {
		s.Config.Key = filepath.Join(filepath.Dir(s.ConfigPath), s.Config.Key)
	}
	s.Node.Cert, s.Node.Key = s.Config.Cert, s.Config.Key
	if s.Config.Reconnect > 0 {
		s.Node.ReconnectDelay = time.Duration(s.Config.Reconnect) * time.Millisecond
	}
	s.Webs["state"] = http.HandlerFunc(s.Node.Router.StateH)
	s.Dialer = dialer.NewPool(s.Config.Name)
	s.Dialer.Webs = s.Webs
	err = s.Dialer.Bootstrap(s.Config.Dialer)
	if err != nil {
		return
	}
	// s.Socks.Dialer = s.SocksDialer
	s.Forward.Dialer = s.SyncDialAll
	if len(s.Config.Listen) > 0 {
		err = s.Node.Listen(s.Config.Listen)
		if err != nil {
			ErrorLog("Server(%v) node listen on %v fail with %v", s.Name, s.Config.Listen, err)
			return
		}
		InfoLog("Server(%v) node listen on %v success", s.Name, s.Config.Listen)
	}
	for loc, uri := range s.Config.Forwards {
		err = s.AddForward(loc, uri)
		if err != nil {
			ErrorLog("Server(%v) add forward by %v->%v fail with %v", s.Name, loc, uri, err)
		}
	}
	if len(s.Config.Console.SOCKS) > 0 {
		_, err = s.Console.SOCKS.Start(s.Config.Console.SOCKS)
		if err != nil {
			ErrorLog("Server(%v) start socks console on %v fail with %v\n", s.Name, s.Config.Console.SOCKS, err)
			s.Node.Stop()
			return
		}
		InfoLog("Server(%v) socks console listen on %v success", s.Name, s.Config.Console.SOCKS)
	}
	if len(s.Config.Console.WS) > 0 {
		_, err = s.Console.WS.Start(s.Config.Console.WS)
		if err != nil {
			ErrorLog("Server(%v) start ws console on %v fail with %v\n", s.Name, s.Config.Console.WS, err)
			s.Node.Stop()
			return
		}
		InfoLog("Server(%v) ws console listen on %v success", s.Name, s.Config.Console.WS)
	}
	s.Node.Start()
	mux := http.NewServeMux()
	mux.HandleFunc("/dav/", s.Forward.ProcWebSubsH)
	mux.HandleFunc("/web/", s.Forward.ProcWebSubsH)
	mux.HandleFunc("/ws/", s.Forward.ProcWebSubsH)
	mux.HandleFunc("/", s.Forward.HostForwardF)
	s.Forward.WebAuth = s.Config.Web.Auth
	s.Forward.WebSuffix = s.Config.Web.Suffix
	server := &http.Server{Addr: s.Config.Web.Listen, Handler: mux}
	if len(s.Config.Web.Listen) > 0 {
		s.Web, err = net.Listen("tcp", s.Config.Web.Listen)
		if err != nil {
			ErrorLog("Server(%v) start web on %v fail with %v\n", s.Name, s.Config.Console, err)
			return err
		}
		go func() {
			server.Serve(s.Web)
		}()
	}
	if s.OnReady != nil {
		s.OnReady()
	}
	if len(s.Config.Channels) > 0 {
		go s.Node.LoginChannel(true, s.Config.Channels...)
	}
	return
}

// Stop will stop service
func (s *Service) Stop() (err error) {
	InfoLog("Server(%v) is stopping", s.Name)
	if s.Node != nil {
		s.Node.Stop()
		s.Node = nil
	}
	if s.Console.SOCKS != nil {
		s.Console.SOCKS.Stop()
		s.Console.SOCKS = nil
	}
	if s.Console.WS != nil {
		s.Console.WS.Stop()
		s.Console.WS = nil
	}
	if s.Web != nil {
		s.Web.Close()
		s.Web = nil
	}
	return
}
