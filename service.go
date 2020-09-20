//Package bsck provider tcp socket proxy router
//
//the supported router is client->(slaver->master->slaver)*-server,
//
//the channel of slaver to master can be multi physical tcp connect by different router
package bsck

import (
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
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xmap"
	"golang.org/x/crypto/ssh"
)

//ShowLog will show more log.
var ShowLog = 0

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

//Web is pojo for web configure
type Web struct {
	Suffix string `json:"suffix"`
	Listen string `json:"listen"`
	Auth   string `json:"auth"`
}

//Config is pojo for all configure
type Config struct {
	Name      string            `json:"name"`
	Cert      string            `json:"cert"`
	Key       string            `json:"key"`
	Listen    string            `json:"listen"`
	ACL       map[string]string `json:"acl"`
	Access    [][]string        `json:"access"`
	Socks5    string            `json:"socks5"`
	Web       Web               `json:"web"`
	ShowLog   int               `json:"showlog"`
	Forwards  map[string]string `json:"forwards"`
	Channels  []xmap.M          `json:"channels"`
	Dialer    xmap.M            `json:"dialer"`
	Proxy     xmap.M            `json:"proxy"`
	Reconnect int64             `json:"reconnect"`
	RDPDir    string            `json:"rdp_dir"`
	VNCDir    string            `json:"vnc_dir"`
}

//ReadConfig will read configure from file
func ReadConfig(path string) (config *Config, last int64, err error) {
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

//ErrForwardNotExist is foward is not exist error
var ErrForwardNotExist = fmt.Errorf("%v", "forward not exist")

//ForwardFinder is forward finder
type ForwardFinder interface {
	FindForward(uri string) (target string, err error)
}

//ForwardFinderF is func implement ForwardFinder
type ForwardFinderF func(uri string) (target string, err error)

//FindForward is implement ForwardFinder
func (f ForwardFinderF) FindForward(uri string) (target string, err error) {
	target, err = f(uri)
	return
}

//Service is bound socket service
type Service struct {
	Node       *Proxy
	Socks      *SocksProxy
	Web        net.Listener
	Forward    *Forward
	Dialer     *dialer.Pool
	Handler    ProxyHandler
	Finder     ForwardFinder
	Config     *Config
	ConfigPath string
	Client     *xhttp.Client
	Webs       map[string]http.Handler
	configLock sync.RWMutex
	configLast int64
	alias      map[string]string
	aliasLock  sync.RWMutex
}

//NewService will return new Service
func NewService() (s *Service) {
	s = &Service{
		configLock: sync.RWMutex{},
		alias:      map[string]string{},
		aliasLock:  sync.RWMutex{},
	}
	client := &http.Client{
		Transport: &http.Transport{
			Dial: s.DialNet,
		},
	}
	s.Client = xhttp.NewClient(client)
	return
}

//ReloadConfig will check configure modify time and reload
func (s *Service) ReloadConfig() (err error) {
	fileInfo, err := os.Stat(s.ConfigPath)
	if err != nil {
		DebugLog("ReloadConfig reload configure fail with %v", err)
		return
	}
	newLast := fileInfo.ModTime().Local().UnixNano() / 1e6
	if newLast == s.configLast {
		return
	}
	config := s.Config
	newConfig, newLast, err := ReadConfig(s.ConfigPath)
	if err != nil {
		DebugLog("ReloadConfig read configure %v fail with %v", s.ConfigPath, err)
		return
	}
	DebugLog("bsrouter will reload modified configure %v by old(%v),new(%v)", s.ConfigPath, s.configLast, newLast)
	//remove missing
	for loc := range config.Forwards {
		if _, ok := newConfig.Forwards[loc]; ok {
			continue
		}
		err = s.RemoveFoward(loc)
		if err != nil {
			ErrorLog("bsrouter remove forward by %v fail with %v", loc, err)
		}
	}
	for loc, uri := range newConfig.Forwards {
		if config.Forwards[loc] == uri {
			continue
		}
		err = s.AddFoward(loc, uri)
		if err != nil {
			ErrorLog("bsrouter add forward by %v->%v fail with %v", loc, uri, err)
		}
	}
	config.Forwards = newConfig.Forwards
	s.configLast = newLast
	return
}

//AddFoward will add forward by local and remote
func (s *Service) AddFoward(loc, uri string) (err error) {
	locParts := strings.SplitN(loc, "~", 2)
	if len(locParts) < 2 {
		s.aliasLock.Lock()
		_, having := s.alias[loc]
		if !having {
			s.alias[loc] = uri
		}
		s.aliasLock.Unlock()
		if having {
			err = fmt.Errorf("local uri is exeists %v", loc)
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
		target.Host = locParts[0]
		err = s.Forward.AddForward(target.String(), uri)
	default:
		err = fmt.Errorf("not supported scheme %v", target.Scheme)
	}
	if err == nil && rdp && len(s.Config.RDPDir) > 0 {
		s.configLock.Lock()
		fileData := fmt.Sprintf(RDPTmpl, listener.Addr(), target.User.Username())
		os.MkdirAll(s.Config.RDPDir, os.ModePerm)
		savepath := filepath.Join(s.Config.RDPDir, locParts[0]+".rdp")
		err := ioutil.WriteFile(savepath, []byte(fileData), os.ModePerm)
		s.configLock.Unlock()
		if err != nil {
			WarnLog("Service save rdp info to %v faile with %v", savepath, err)
		} else {
			InfoLog("Service save rdp info to %v success", savepath)
		}
	}
	if err == nil && vnc && len(s.Config.VNCDir) > 0 {
		s.configLock.Lock()
		password, _ := target.User.Password()
		fileData := fmt.Sprintf(VNCTmpl, locParts[0], listener.Addr(), password)
		os.MkdirAll(s.Config.VNCDir, os.ModePerm)
		savepath := filepath.Join(s.Config.VNCDir, locParts[0]+".vnc")
		err := ioutil.WriteFile(savepath, []byte(fileData), os.ModePerm)
		s.configLock.Unlock()
		if err != nil {
			WarnLog("Service save vnc info to %v faile with %v", savepath, err)
		} else {
			InfoLog("Service save vnc info to %v success", savepath)
		}
	}
	return
}

//RemoveFoward will remove forward
func (s *Service) RemoveFoward(loc string) (err error) {
	locParts := strings.SplitN(loc, "~", 2)
	if len(locParts) < 2 {
		s.aliasLock.Lock()
		_, having := s.alias[loc]
		if having {
			delete(s.alias, loc)
		}
		s.aliasLock.Unlock()
		if !having {
			err = fmt.Errorf("local uri is not exeists %v", loc)
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
			WarnLog("Service remove rdp file on %v faile with %v", savepath, err)
		} else {
			InfoLog("Service remove rdp file on %v success", savepath)
		}
	}
	if vnc && len(s.Config.VNCDir) > 0 {
		s.configLock.Lock()
		savepath := filepath.Join(s.Config.VNCDir, locParts[0]+".vnc")
		err := os.Remove(savepath)
		s.configLock.Unlock()
		if err != nil {
			WarnLog("Service remove vnc file on %v faile with %v", savepath, err)
		} else {
			InfoLog("Service remove vnc file on %v success", savepath)
		}
	}
	return
}

//DialerAll will dial uri by raw
func (s *Service) DialerAll(uris string, raw io.ReadWriteCloser) (sid uint64, err error) {
	sid, err = s.dialerAll(uris, raw)
	if err == nil || err != ErrForwardNotExist || s.Finder == nil {
		return
	}
	uris, err = s.Finder.FindForward(uris)
	if err == nil {
		sid, err = s.dialerAll(uris, raw)
	}
	return
}

func (s *Service) dialerAll(uris string, raw io.ReadWriteCloser) (sid uint64, err error) {
	DebugLog("Service try dial all to %v", uris)
	for _, uri := range strings.Split(uris, ",") {
		sid, err = s.dialerOne(uri, raw)
		if err == nil {
			break
		}
	}
	return
}

func (s *Service) dialerOne(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	DebugLog("Service try dial to %v", uri)
	if strings.Contains(uri, "->") {
		sid, err = s.Node.Dial(uri, raw)
		return
	}
	if regexp.MustCompile("^[A-Za-z0-9]*://.*$").MatchString(uri) {
		sid = s.Node.UniqueSid()
		_, err = s.Dialer.Dial(sid, uri, raw)
		return
	}
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
	dialURI := target
	if len(parts) > 1 {
		if strings.Contains(dialURI, "?") {
			dialURI += "&" + parts[1]
		} else {
			dialURI += "?" + parts[1]
		}
	}
	sid, err = s.Node.Dial(dialURI, raw)
	return
}

//SocksDialer is socks proxy dialer
func (s *Service) SocksDialer(utype int, uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	switch utype {
	case SocksUriTypeBS:
		sid, err = s.DialerAll(uri, raw)
	default:
		err = fmt.Errorf("not supported for type %v", utype)
	}
	return
}

//DialRaw is router dial implemnet
func (s *Service) DialRaw(sid uint64, uri string) (conn Conn, err error) {
	raw, err := s.Dialer.Dial(sid, uri, nil)
	if err == nil {
		conn = NewRawConn(fmt.Sprintf("%v", sid), raw, s.Node.BufferSize, sid, uri)
	}
	return
}

//DialNet is net dialer to router
func (s *Service) DialNet(network, addr string) (conn net.Conn, err error) {
	conn, raw, err := dialer.CreatePipedConn()
	if err == nil {
		addr = strings.TrimSuffix(addr, ":80")
		addr = strings.TrimSuffix(addr, ":443")
		_, err = s.DialerAll(addr, raw)
	}
	return
}

//DialSSH is ssh dialer to ssh server
func (s *Service) DialSSH(uri string, config *ssh.ClientConfig) (client *ssh.Client, err error) {
	conn, raw, err := dialer.CreatePipedConn()
	if err == nil {
		_, err = s.DialerAll(uri, raw)
	}
	if err != nil {
		return
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, uri, config)
	if err != nil {
		return nil, err
	}
	client = ssh.NewClient(c, chans, reqs)
	return
}

//Start will start service
func (s *Service) Start() (err error) {
	if len(s.ConfigPath) > 0 {
		s.Config, s.configLast, err = ReadConfig(s.ConfigPath)
		if err != nil {
			ErrorLog("Service read config %v fail with %v", s.ConfigPath, err)
			return
		}
	} else if s.Config == nil {
		err = fmt.Errorf("config is nil and configure path is empty")
		return
	}
	InfoLog("Service will start by config %v", s.ConfigPath)
	s.Socks = NewSocksProxy()
	s.Forward = NewForward()
	if s.Handler == nil {
		handler := NewNormalAcessHandler(s.Config.Name, nil)
		if len(s.Config.ACL) > 0 {
			handler.LoginAccess = s.Config.ACL
		}
		if len(s.Config.Access) > 0 {
			handler.DialAccess = s.Config.Access
		}
		handler.Dialer = DialRawF(s.DialRaw)
		s.Handler = handler
	}
	s.Node = NewProxy(s.Config.Name, s.Handler)
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
	s.Dialer = dialer.NewPool()
	s.Dialer.AddDialer(dialer.NewStateDialer("router", s.Node.Router))
	s.Dialer.Webs = s.Webs
	err = s.Dialer.Bootstrap(s.Config.Dialer)
	if err != nil {
		return
	}
	// s.Socks.Dialer = s.SocksDialer
	s.Forward.Dialer = s.DialerAll
	if len(s.Config.Listen) > 0 {
		err = s.Node.ListenMaster(s.Config.Listen)
		if err != nil {
			ErrorLog("Service node listen on %v fail with %v", s.Config.Listen, err)
			return
		}
		s.Node.StartHeartbeat()
		InfoLog("node listen on %v success", s.Config.Listen)
	}
	if len(s.Config.Channels) > 0 {
		go s.Node.LoginChannel(true, s.Config.Channels...)
	}
	for loc, uri := range s.Config.Forwards {
		err = s.AddFoward(loc, uri)
		if err != nil {
			ErrorLog("Service add forward by %v->%v fail with %v", loc, uri, err)
		}
	}
	if len(s.Config.Socks5) > 0 {
		err = s.Socks.Start(s.Config.Socks5)
		if err != nil {
			ErrorLog("Service start socks5 on %v fail with %v\n", s.Config.Socks5, err)
			s.Node.Close()
			return
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/dav/", s.Forward.ProcWebSubsH)
	mux.HandleFunc("/ws/", s.Forward.ProcWebSubsH)
	mux.HandleFunc("/", s.Forward.HostForwardF)
	s.Forward.WebAuth = s.Config.Web.Auth
	s.Forward.WebSuffix = s.Config.Web.Suffix
	server := &http.Server{Addr: s.Config.Web.Listen, Handler: mux}
	if len(s.Config.Web.Listen) > 0 {
		s.Web, err = net.Listen("tcp", s.Config.Web.Listen)
		if err != nil {
			ErrorLog("Service start web on %v fail with %v\n", s.Config.Socks5, err)
			return err
		}
		go func() {
			server.Serve(s.Web)
		}()
	}
	return
}

//Stop will stop service
func (s *Service) Stop() (err error) {
	if s.Node != nil {
		s.Node.Close()
		s.Node = nil
	}
	if s.Socks != nil {
		s.Socks.Close()
		s.Socks = nil
	}
	if s.Web != nil {
		s.Web.Close()
		s.Web = nil
	}
	return
}
