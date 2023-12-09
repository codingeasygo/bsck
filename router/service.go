// Package bsck provider tcp socket proxy router
//
// the supported router is client->(slaver->master->slaver)*-server,
//
// the channel of slaver to master can be multi physical tcp connect by different router
package router

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/bsck/router/native"
	"github.com/codingeasygo/tun2conn/gfw"
	"github.com/codingeasygo/util/proxy"
	sproxy "github.com/codingeasygo/util/proxy/socks"
	wproxy "github.com/codingeasygo/util/proxy/ws"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/util/xtime"
	"github.com/codingeasygo/web"
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
	Name     string            `json:"name"`
	Dir      string            `json:"dir"`
	Cert     string            `json:"cert"`
	Key      string            `json:"key"`
	Insecure int               `json:"insecure"`
	Listen   string            `json:"listen"`
	ACL      map[string]string `json:"acl"`
	Access   [][]string        `json:"access"`
	Console  struct {
		SOCKS string `json:"socks"`
		WS    string `json:"ws"`
		Unix  string `json:"unix"`
	} `json:"console"`
	Proxy struct {
		WEB     string   `json:"web"`
		Addr    string   `json:"addr"`
		Skip    []string `json:"skip"`
		Channel string   `json:"channel"`
	} `json:"proxy"`
	Web      Web               `json:"web"`
	Log      int               `json:"log"`
	Forwards map[string]string `json:"forwards"`
	Channels map[string]xmap.M `json:"channels"`
	Dialer   xmap.M            `json:"dialer"`
	RDPDir   string            `json:"rdp_dir"`
	VNCDir   string            `json:"vnc_dir"`
}

// ReadConfig will read configure from file
func ReadConfig(filename string) (config *Config, last int64, err error) {
	var configData []byte
	stat, err := os.Stat(filename)
	if err == nil {
		configData, err = os.ReadFile(filename)
	}
	if err != nil {
		return
	}
	last = stat.ModTime().Local().UnixNano() / 1e6
	user, _ := user.Current()
	config = &Config{}
	err = json.Unmarshal(configData, config)
	if err != nil {
		return
	}
	if len(config.Cert) < 1 && config.Insecure != 1 {
		config.Cert = "bsrouter.pem"
	}
	if len(config.Key) < 1 && config.Insecure != 1 {
		config.Key = "bsrouter.key"
	}
	if len(config.Dir) < 1 {
		config.Dir, err = filepath.Abs(filepath.Dir(filename))
	}
	if len(config.RDPDir) < 1 {
		config.RDPDir = filepath.Join(user.HomeDir, "Desktop")
	}
	if len(config.VNCDir) < 1 {
		config.VNCDir = filepath.Join(user.HomeDir, "Desktop")
	}
	return
}

var defaultUnixFile = filepath.Join(".bsrouter", "run", "bsrouter.sock")
var userHomeDir = func() string {
	home, _ := os.UserHomeDir()
	return home
}

func (c *Config) ConsoleUnix() (unixFile, unixDir string) {
	unixFile = c.Console.Unix
	if len(unixFile) < 1 {
		unixFile = filepath.Join(userHomeDir(), defaultUnixFile)
	}
	if !filepath.IsAbs(unixFile) {
		unixFile = filepath.Join(c.Dir, unixFile)
	}
	unixFile, _ = filepath.Abs(unixFile)
	unixDir = filepath.Dir(unixFile)
	return
}

func (c *Config) ConsoleURI() (uri string) {
	if len(c.Console.Unix) > 0 {
		uri, _ = c.ConsoleUnix()
		if !strings.HasPrefix(uri, "unix://") {
			uri = "unix://" + uri
		}
	} else if len(c.Console.SOCKS) > 0 {
		uri = c.Console.SOCKS
		if !strings.HasPrefix(uri, "socks5://") {
			uri = "socks5://" + uri
		}
	} else if len(c.Console.WS) > 0 {
		uri = c.Console.WS
		if !strings.HasPrefix(uri, "ws://") && !strings.HasPrefix(uri, "wss://") {
			uri = "ws://" + uri
		}
	} else {
		uri, _ = c.ConsoleUnix()
		if !strings.HasPrefix(uri, "unix://") {
			uri = "unix://" + uri
		}
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
	Node    *Node
	Console struct {
		SOCKS *sproxy.Server
		WS    *wproxy.Server
		Unix  *sproxy.Server
	}
	Proxy struct {
		Web    net.Listener
		Mux    *web.SessionMux
		Listen net.Listener
		Server *proxy.Server
	}
	Web         net.Listener
	WebForward  *WebForward
	Dialer      *dialer.Pool
	Handler     Handler
	Finder      ForwardFinder
	Config      *Config
	ConfigPath  string
	Client      *xhttp.Client
	Webs        map[string]http.Handler
	BufferSize  int
	OnReady     func()
	OnService   func(name, command string)
	NewBackend  func(name string) (*exec.Cmd, error)
	configLock  sync.RWMutex
	configLast  int64
	alias       map[string]string
	aliasLock   sync.RWMutex
	backendAll  map[string]*backendRunner
	backendLock sync.RWMutex
}

// NewService will return new Service
func NewService() (s *Service) {
	s = &Service{
		BufferSize:  32 * 1024,
		configLock:  sync.RWMutex{},
		alias:       map[string]string{},
		aliasLock:   sync.RWMutex{},
		backendAll:  map[string]*backendRunner{},
		backendLock: sync.RWMutex{},
		Webs:        map[string]http.Handler{},
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
	newConfig, newLast, err := ReadConfig(s.ConfigPath)
	if err != nil {
		DebugLog("Server(%v) read configure %v fail with %v", s.Name, s.ConfigPath, err)
		return
	}
	if newLast == s.configLast {
		return
	}
	config := s.Config
	DebugLog("Server(%v) will reload modified configure %v by old(%v),new(%v)", s.Name, s.ConfigPath, s.configLast, newLast)
	//remove missing
	for loc := range config.Forwards {
		if _, ok := newConfig.Forwards[loc]; ok {
			continue
		}
		xerr := s.RemoveForward(loc)
		if xerr != nil {
			ErrorLog("Server(%v) remove forward by %v fail with %v", s.Name, loc, xerr)
		}
	}
	for loc, uri := range newConfig.Forwards {
		if config.Forwards[loc] == uri {
			continue
		}
		xerr := s.AddForward(loc, uri)
		if xerr != nil {
			ErrorLog("Server(%v) add forward by %v->%v fail with %v", s.Name, loc, uri, xerr)
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
	case "host":
		err = s.sendBackendCommand("HostForward", "@add %v %v %v", locParts[0], locParts[1], uri)
	case "web":
		fallthrough
	case "ws":
		fallthrough
	case "wss":
		target.Host = target.Hostname()
		err = s.WebForward.AddForward(target.String(), uri)
	default:
		err = fmt.Errorf("not supported scheme %v", target.Scheme)
	}
	if err == nil && rdp && len(s.Config.RDPDir) > 0 {
		s.configLock.Lock()
		fileData := fmt.Sprintf(RDPTemplate, listener.Addr(), target.User.Username())
		savepath := filepath.Join(s.Config.RDPDir, locParts[0]+".rdp")
		err := os.WriteFile(savepath, []byte(fileData), os.ModePerm)
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
		savepath := filepath.Join(s.Config.VNCDir, locParts[0]+".vnc")
		err := os.WriteFile(savepath, []byte(fileData), os.ModePerm)
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
	case "proxy":
		err = s.Node.StopForward(locParts[0])
	case "tcp":
		err = s.Node.StopForward(locParts[0])
	case "rdp":
		rdp = true
		err = s.Node.StopForward(locParts[0])
	case "vnc":
		vnc = true
		err = s.Node.StopForward(locParts[0])
	case "host":
		err = s.sendBackendCommand("HostForward", "@remove %v", locParts[0])
	default:
		err = s.WebForward.RemoveForward(locParts[0])
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
			router := s.WebForward.FindForward(parts[0])
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
	if err == nil {
		client = ssh.NewClient(c, channel, request)
	}
	return
}

// DialPiper will dial uri on router and return piper
func (s *Service) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	piper := NewRouterPiper()
	_, err = s.DialAll(uri, piper, true)
	raw = piper
	return
}

func (s *Service) dialConsolePiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	if strings.HasPrefix(uri, "tcp://") {
		targetURI, xerr := url.Parse(uri)
		if xerr != nil {
			err = xerr
			return
		}
		hostname := targetURI.Hostname()
		if strings.HasSuffix(hostname, ".Service") {
			name := strings.TrimSuffix(hostname, ".Service")
			conn := NewBackendPiper(s, name)
			conn.OnCommand = func(command string) {
				if s.OnService != nil {
					s.OnService(name, command)
				}
			}
			s.addBackendPiper(name, conn)
			raw = conn
			return
		}
	}
	raw, err = s.DialPiper(uri, bufferSize)
	return
}

func (s *Service) addBackendPiper(name string, piper *BackendPiper) (err error) {
	s.backendLock.Lock()
	defer s.backendLock.Unlock()
	backend := s.backendAll[name]
	if backend != nil {
		backend.ReadyConn(piper)
	} else {
		err = fmt.Errorf("not started")
	}
	return
}

func (s *Service) sendBackendCommand(name string, format string, args ...interface{}) (err error) {
	backend, err := s.startBackend(name)
	if err == nil {
		_, err = fmt.Fprintf(backend.Conn, format+"\n", args...)
	}
	return
}

func (s *Service) startBackend(name string) (backend *backendRunner, err error) {
	s.backendLock.Lock()
	backend = s.backendAll[name]
	if backend != nil {
		s.backendLock.Unlock()
		return
	}
	backend = newBackendRunner(s, name)
	s.backendAll[name] = backend
	s.backendLock.Unlock()

	err = backend.Start()
	if err != nil {
		s.backendLock.Lock()
		delete(s.backendAll, name)
		s.backendLock.Unlock()
	}
	return
}

func (s *Service) stopBackend(name string) (err error) {
	s.backendLock.Lock()
	backend := s.backendAll[name]
	s.backendLock.Unlock()
	if backend != nil {
		err = backend.Stop()
	} else {
		err = fmt.Errorf("not started")
	}
	return
}

func (s *Service) stopServiceBackendAll() {
	nameAll := []string{}
	s.backendLock.Lock()
	for name := range s.backendAll {
		nameAll = append(nameAll, name)
	}
	s.backendLock.Unlock()
	for _, name := range nameAll {
		s.stopBackend(name)
	}
}

// PACH is http handler to get pac js
func (s *Service) proxyPACH(w *web.Session) web.Result {
	w.W.Header().Set("Content-Type", "application/x-javascript")
	rules, _ := gfw.ReadAllRules(filepath.Join(s.Node.Dir, "gfwlist.txt"), filepath.Join(s.Node.Dir, "user_rules.txt"))
	abp := gfw.CreateAbpJS(rules, fmt.Sprintf("127.0.1:%v", s.Proxy.Web.Addr().(*net.TCPAddr).Port))
	return w.Printf("%v", abp)
}

func (s *Service) proxyModeH(w *web.Session) web.Result {
	mode := w.Argument("mode")
	pacURL := fmt.Sprintf("http://127.0.1:%v/pac.js?timestamp=%v", s.Proxy.Web.Addr().(*net.TCPAddr).Port, xtime.Now())
	message, err := native.ChangeProxyMode(mode, pacURL, "127.0.0.1", s.Proxy.Listen.Addr().(*net.TCPAddr).Port)
	if err != nil {
		ErrorLog("Server(%v) change proxy mode to %v fail with %v, message is \n%v", s.Name, mode, err, message)
		w.W.WriteHeader(500)
		return w.Printf("%v", message)
	} else {
		InfoLog("Server(%v) change proxy mode to %v success", s.Name, mode)
		return w.Printf("%v", "OK")
	}
}

func (s *Service) dialDirectPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	conn, err := s.Dialer.Dial(dialer.NewChannelInfo(0, ""), 0, uri)
	if err != nil {
		return
	}
	raw = xio.NewCopyPiper(conn, bufferSize)
	return
}

func (s *Service) dialProxyPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	for _, skip := range s.Config.Proxy.Skip {
		matcher, xerr := regexp.Compile(skip)
		if xerr == nil && matcher.MatchString(uri) {
			raw, err = s.dialDirectPiper(uri, bufferSize)
			return
		}
	}
	channelAll := s.Node.ListChannelName()
	channelSelected := channelAll
	if len(s.Config.Proxy.Channel) > 0 {
		matcher, xerr := regexp.Compile(s.Config.Proxy.Channel)
		if xerr != nil {
			err = xerr
			return
		}
		channelSelected = []string{}
		for _, c := range channelAll {
			if matcher.MatchString(c) {
				channelSelected = append(channelSelected, c)
			}
		}
	}
	if len(channelSelected) < 1 {
		err = fmt.Errorf("not channel be used on proxy")
		return
	}
	channel := channelSelected[rand.Intn(len(channelSelected))]
	targetURI := fmt.Sprintf("%v->%v", channel, uri)
	raw, err = s.DialPiper(targetURI, bufferSize)
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
	s.Console.SOCKS.Dialer = xio.PiperDialerF(s.dialConsolePiper)
	s.Console.SOCKS.BufferSize = s.BufferSize
	s.Console.WS = wproxy.NewServer()
	s.Console.WS.Dialer = xio.PiperDialerF(s.dialConsolePiper)
	s.Console.WS.BufferSize = s.BufferSize
	s.Console.Unix = sproxy.NewServer()
	s.Console.Unix.Dialer = xio.PiperDialerF(s.dialConsolePiper)
	s.Console.Unix.BufferSize = s.BufferSize
	s.WebForward = NewWebForward()
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
	s.Node = NewNode(s.Config.Name, s.BufferSize, s.Handler)
	s.Node.Dir = s.Config.Dir
	s.Webs["state"] = http.HandlerFunc(s.Node.Router.StateH)
	s.Dialer = dialer.NewPool(s.Config.Name)
	s.Dialer.Webs = s.Webs
	if s.Config.Dialer == nil {
		s.Config.Dialer = xmap.M{}
	}
	dialerConfig := s.Config.Dialer
	dialerConfig["dir"] = s.Config.Dir
	err = s.Dialer.Bootstrap(s.Config.Dialer)
	if err != nil {
		return
	}
	// s.Socks.Dialer = s.SocksDialer
	s.WebForward.Dialer = s.SyncDialAll
	if len(s.Config.Listen) > 0 {
		err = s.Node.Listen(s.Config.Listen, s.Config.Cert, s.Config.Key)
		if err != nil {
			ErrorLog("Server(%v) node listen on %v fail with %v", s.Name, s.Config.Listen, err)
			return
		}
		InfoLog("Server(%v) node listen on %v success", s.Name, s.Config.Listen)
	}
	if len(s.Config.Console.SOCKS) > 0 {
		_, err = s.Console.SOCKS.Start("tcp", s.Config.Console.SOCKS)
		if err != nil {
			ErrorLog("Server(%v) start socks console on %v fail with %v\n", s.Name, s.Config.Console.SOCKS, err)
			s.Stop()
			return
		}
		InfoLog("Server(%v) socks console listen on %v success", s.Name, s.Config.Console.SOCKS)
	}
	if len(s.Config.Console.WS) > 0 {
		_, err = s.Console.WS.Start("tcp", s.Config.Console.WS)
		if err != nil {
			ErrorLog("Server(%v) start ws console on %v fail with %v\n", s.Name, s.Config.Console.WS, err)
			s.Stop()
			return
		}
		InfoLog("Server(%v) ws console listen on %v success", s.Name, s.Config.Console.WS)
	}
	if runtime.GOOS != "windows" && s.Config.Console.Unix != "off" && s.Config.Console.Unix != "none" && s.Config.Console.Unix != "0" {
		unixFile, unixDir := s.Config.ConsoleUnix()
		os.MkdirAll(unixDir, os.ModePerm)
		_, err = s.Console.Unix.Start("unix", unixFile)
		if err != nil {
			ErrorLog("Server(%v) start unix console on %v fail with %v\n", s.Name, s.Config.Console.Unix, err)
			s.Stop()
			return
		}
		InfoLog("Server(%v) unix console listen on %v success", s.Name, s.Config.Console.Unix)
	}
	if len(s.Config.Proxy.WEB) > 0 && len(s.Config.Proxy.Addr) > 0 {
		s.Proxy.Mux = web.NewSessionMux("")
		s.Proxy.Mux.HandleFunc("^/pac\\.js(\\?.*)?$", s.proxyPACH)
		s.Proxy.Mux.HandleFunc("^/proxy(\\?.*)?$", s.proxyModeH)
		s.Proxy.Server = proxy.NewServer(xio.PiperDialerF(s.dialProxyPiper))
		s.Proxy.Listen, err = s.Proxy.Server.Start("tcp", s.Config.Proxy.Addr)
		if err != nil {
			ErrorLog("Server(%v) start proxy server on %v fail with %v\n", s.Name, s.Config.Proxy.Addr, err)
			s.Stop()
			return
		}
		s.Proxy.Web, err = net.Listen("tcp", s.Config.Proxy.WEB)
		if err != nil {
			ErrorLog("Server(%v) start proxy web on %v fail with %v\n", s.Name, s.Config.Proxy.WEB, err)
			s.Stop()
			return
		}
		go (&http.Server{Handler: s.Proxy.Mux}).Serve(s.Proxy.Web)
		InfoLog("Server(%v) proxy server listen on %v/%v success", s.Name, s.Config.Proxy.WEB, s.Config.Proxy.Addr)
	}
	for loc, uri := range s.Config.Forwards {
		xerr := s.AddForward(loc, uri)
		if xerr != nil {
			ErrorLog("Server(%v) add forward by %v->%v fail with %v", s.Name, loc, uri, xerr)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/dav/", s.WebForward.ProcWebSubsH)
	mux.HandleFunc("/web/", s.WebForward.ProcWebSubsH)
	mux.HandleFunc("/ws/", s.WebForward.ProcWebSubsH)
	mux.HandleFunc("/", s.WebForward.HostForwardF)
	s.WebForward.WebAuth = s.Config.Web.Auth
	s.WebForward.WebSuffix = s.Config.Web.Suffix
	server := &http.Server{Addr: s.Config.Web.Listen, Handler: mux}
	if len(s.Config.Web.Listen) > 0 {
		s.Web, err = net.Listen("tcp", s.Config.Web.Listen)
		if err != nil {
			ErrorLog("Server(%v) start web on %v fail with %v\n", s.Name, s.Config.Console, err)
			s.Stop()
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
		s.Node.Channels = s.Config.Channels
	}
	s.Node.Start()
	return
}

// Stop will stop service
func (s *Service) Stop() (err error) {
	InfoLog("Server(%v) is stopping", s.Name)
	if s.Node != nil {
		s.Node.Stop()
		s.Node = nil
	}
	s.stopServiceBackendAll()
	if s.Console.SOCKS != nil {
		s.Console.SOCKS.Stop()
		s.Console.SOCKS = nil
	}
	if s.Console.WS != nil {
		s.Console.WS.Stop()
		s.Console.WS = nil
	}
	if s.Console.Unix != nil {
		s.Console.Unix.Stop()
		s.Console.Unix = nil
	}
	if s.Proxy.Web != nil {
		s.Proxy.Web.Close()
	}
	if s.Proxy.Server != nil {
		s.Proxy.Server.Close()
	}
	if s.Web != nil {
		s.Web.Close()
		s.Web = nil
	}
	s.Dialer.Shutdown()
	return
}

type BackendPiper struct {
	Service   *Service
	Name      string
	OnCommand func(string)
	conn      io.ReadWriteCloser
	closed    bool
	lock      sync.RWMutex
}

func NewBackendPiper(service *Service, name string) (piper *BackendPiper) {
	piper = &BackendPiper{
		Service:   service,
		Name:      name,
		OnCommand: func(s string) {},
		lock:      sync.RWMutex{},
	}
	return
}

func (s *BackendPiper) Write(p []byte) (n int, err error) {
	if s.conn == nil {
		err = fmt.Errorf("not ready")
		return
	}
	n, err = s.conn.Write(p)
	return
}

func (b *BackendPiper) PipeConn(conn io.ReadWriteCloser, target string) (err error) {
	defer func() {
		conn.Close()
		b.Close()
	}()
	b.lock.Lock()
	b.conn = conn
	closed := b.closed
	b.lock.Unlock()
	if closed {
		err = fmt.Errorf("closed")
		return
	}
	reader := bufio.NewReader(conn)
	commandRegex := regexp.MustCompile(`^@[^\s]+\s`)
	for {
		line, xerr := reader.ReadString('\n')
		if xerr != nil {
			err = xerr
			break
		}
		if commandRegex.MatchString(line) {
			b.OnCommand(line)
		} else {
			InfoLog("ServicePiper(%v/%v) %v", b.Service.Name, b.Name, line)
		}
	}
	return
}

func (b *BackendPiper) Close() (err error) {
	b.lock.Lock()
	b.closed = true
	conn := b.conn
	b.lock.Unlock()
	if conn != nil {
		conn.Close()
	}
	return
}

type backendRunner struct {
	Service *Service
	Name    string
	Cmd     *exec.Cmd
	Conn    *BackendPiper
	ready   chan int
	lock    sync.RWMutex
}

func newBackendRunner(service *Service, name string) (runner *backendRunner) {
	runner = &backendRunner{
		Service: service,
		Name:    name,
		ready:   make(chan int, 1),
		lock:    sync.RWMutex{},
	}
	return
}

func (b *backendRunner) Start() (err error) {
	b.Cmd, err = b.Service.NewBackend(b.Name)
	if err == nil {
		b.Cmd.Stdin = os.Stdin
		b.Cmd.Stdout = os.Stdout
		b.Cmd.Stderr = os.Stderr
		err = b.Cmd.Start()
	}
	if err != nil {
		WarnLog("Backend(%v) %v service backend start error with %v", b.Service.Name, b.Name, err)
		return
	}
	exiter := make(chan int, 1)
	go func() {
		err := b.Cmd.Wait()
		InfoLog("Backend(%v) %v service backend is stopped by %v", b.Service.Name, b.Name, err)
		exiter <- 1
	}()
	select {
	case <-b.ready:
		InfoLog("Backend(%v) %v service backend is ready", b.Service.Name, b.Name)
	case <-exiter:
		err = fmt.Errorf("stopped")
	}
	return
}

func (b *backendRunner) Stop() (err error) {
	if b.Conn != nil {
		b.Conn.Close()
	}
	if b.Cmd != nil && b.Cmd.Process != nil {
		b.Cmd.Process.Kill()
	}
	return
}

func (b *backendRunner) ReadyConn(conn *BackendPiper) {
	if b.Conn != nil {
		b.Conn.Close()
	}
	b.Conn = conn
	select {
	case b.ready <- 1:
	default:
	}
}
