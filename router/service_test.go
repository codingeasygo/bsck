package router

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/bsck/router/native"
	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xhash"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/web"
	"github.com/codingeasygo/web/httptest"
	sshsrv "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

func assertNotUnix(c *Config) {
	unixFile, _ := c.ConsoleUnix()
	_, err := os.Stat(unixFile)
	if err == nil {
		panic(fmt.Sprintf("%v exists", unixFile))
	}
}

func TestConsoleUnixFile(t *testing.T) {
	c := &Config{}
	c.Dir = "."

	c.ConsoleUnix()

	c.Console.Unix = "xx"
	c.ConsoleUnix()

	c.Console.Unix = "/xx"
	c.ConsoleUnix()
}

var configTest1 = `
{
    "name": "r0",
    "listen": ":15023",
    "web": {
        "listen": ":15024",
        "suffix": ".test.loc:15024",
        "auth": ""
    },
    "console": {
        "socks": ":5081"
    },
    "proxy": {
        "web": "127.0.0.1:0",
        "addr": "127.0.0.1:0",
        "skip": [".*skip.loc"],
        "channel": ".*"
    },
    "forwards": {
        "tx~tcp://:2332": "http://web?dir=/tmp"
    },
    "channels": {},
    "dialer": {
        "standard": 1
    }
}
`
var configTest2 = `
{
    "name": "r0",
    "listen": ":15023",
    "web": {
        "listen": ":15024",
        "suffix": ".test.loc:15024",
        "auth": ""
    },
    "console": {
        "socks": ":5081"
    },
    "forwards": {
        "t0~web://t0": "http://127.0.0.1:80",
        "t1~web://t1": "http://web?dir=.",
        "t2~tcp://:2332": "http://dav?dir=.",
        "t3~socks://localhost:10322": "tcp://echo",
		"t4~proxy://localhost:10332": "tcp://echo",
        "t5~rdp://localhost:0": "tcp://127.0.0.1:22",
        "t6~vnc://localhost:0": "tcp://dev.loc:22",
        "w1~web://w1": "tcp://echo?abc=1",
        "w2": "tcp://echo",
        "w3": "http://dav?dir=.",
        "w40": "http://test1",
        "w41": "http://test1",
        "w5": "tcp://dev.loc:22",
        "w6": "http://state"
    },
    "channels": {},
    "dialer": {
        "standard": 1
    }
}
`

var configTestErr = `
{
    "name": "r0",
    "listen": ":15023",
    "web": {
        "listen": ":15024",
        "suffix": ".test.loc:15024",
        "auth": ""
    },
    "console": {
        "socks": ":5081"
    },
    "forwards": {
        "tx~tcp://127.0.0.1:xx": "http://web?dir=/tmp"
    },
    "channels": {},
    "dialer": {
        "standard": 1
    }
}
`

func TestService(t *testing.T) {
	socks.SetLogLevel(socks.LogLevelDebug)
	SetLogLevel(LogLevelDebug)
	tester := xdebug.CaseTester{
		0: 1,
		1: 1,
	}
	os.WriteFile("/tmp/test.json", []byte(configTest1), os.ModePerm)
	defer os.Remove("/tmp/test.json")
	service := NewService()
	service.OnReady = func() {}
	service.ConfigPath = "/tmp/test.json"
	service.Webs = map[string]http.Handler{
		"test1": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "abc")
		}),
	}
	err := service.Start()
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(10 * time.Millisecond)
	if tester.Run("ReloadConfig") {
		os.WriteFile("/tmp/test.json", []byte(configTest2), os.ModePerm)
		err = service.ReloadConfig()
		if err != nil {
			t.Error(err)
			return
		}
		os.WriteFile("/tmp/test.json", []byte(configTest2), os.ModePerm)
		err = service.ReloadConfig()
		if err != nil {
			t.Error(err)
			return
		}
		time.Sleep(100 * time.Millisecond)

		//
		os.WriteFile("/tmp/test.json", []byte(configTest1), os.ModePerm)
		err = service.ReloadConfig()
		if err != nil {
			t.Error(err)
			return
		}
		err = service.ReloadConfig()
		if err != nil {
			t.Error(err)
			return
		}
		time.Sleep(100 * time.Millisecond)

		//
		os.WriteFile("/tmp/test.json", []byte(configTestErr), os.ModePerm)
		err = service.ReloadConfig()
		if err != nil {
			t.Error(err)
			return
		}
		time.Sleep(100 * time.Millisecond)

		//
		service.Config.Forwards["none"] = "none"
		os.WriteFile("/tmp/test.json", []byte(configTest1), os.ModePerm)
		err = service.ReloadConfig()
		if err != nil {
			t.Error(err)
			return
		}
		time.Sleep(100 * time.Millisecond)

		//
		service.ConfigPath = "none"
		err = service.ReloadConfig()
		if err == nil {
			t.Error(err)
			return
		}
	}
	if tester.Run("ForwardName") {
		service.AddForward("a0", "http://dav")
		service.AddForward("a1", "http://dav?dir=.")
		service.WebForward.AddForward("web://a2", "http://dav?dir=.")
		conn0 := xio.NewQueryConn()
		_, err = service.DialAll("a0?x=1", conn0, false)
		if err != nil {
			t.Error(err)
			return
		}
		conn0.Close()
		conn1 := xio.NewQueryConn()
		_, err = service.DialAll("a1?x=1", conn1, false)
		if err != nil {
			t.Error(err)
			return
		}
		conn1.Close()
		conn2 := xio.NewQueryConn()
		_, err = service.DialAll("a2", conn2, false)
		if err != nil {
			t.Error(err)
			return
		}
		conn2.Close()
	}
	if tester.Run("ForwardFinder") {
		service.Finder = ForwardFinderF(func(uri string) (target string, err error) {
			switch uri {
			case "find0":
				target = "http://dav?dir=."
			default:
				err = fmt.Errorf("not supported %v", uri)
			}
			return
		})
		conn := xio.NewQueryConn()
		_, err = service.DialAll("find0", conn, false)
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}
	if tester.Run("ForwardError") { //Forward Error
		err = service.AddForward("xx~"+string([]byte{1, 1}), "http://web?dir=/tmp")
		if err == nil {
			t.Error(err)
			return
		}
		err = service.AddForward("xa", "http://web?dir=/tmp")
		if err != nil {
			t.Error(err)
			return
		}
		err = service.AddForward("xa", "http://web?dir=/tmp")
		if err == nil {
			t.Error(err)
			return
		}
		err = service.AddForward("xb~xxx://xxkk", "http://web?dir=/tmp")
		if err == nil {
			t.Error(err)
			return
		}
		err = service.RemoveForward("xx~" + string([]byte{1, 1}))
		if err == nil {
			t.Error(err)
			return
		}
		err = service.RemoveForward("xa")
		if err != nil {
			t.Error(err)
			return
		}
		err = service.RemoveForward("xa")
		if err == nil {
			t.Error(err)
			return
		}

		//
		service.Config.VNCDir = "xxxx"
		err = service.AddForward("va~vnc://", "http://web?dir=/tmp")
		if err != nil {
			t.Error(err)
			return
		}
		err = service.RemoveForward("va~vnc://")
		if err != nil {
			t.Error(err)
			return
		}

		//
		service.Config.RDPDir = "xxxx"
		err = service.AddForward("vb~rdp://", "http://web?dir=/tmp")
		if err != nil {
			t.Error(err)
			return
		}
		err = service.RemoveForward("vb~rdp://")
		if err != nil {
			t.Error(err)
			return
		}
	}
	if tester.Run("DialError") {
		_, err = service.DialNet("tcp", "base64-xx"+string([]byte{1, 1}))
		if err == nil {
			t.Error(err)
			return
		}
		_, err = service.DialNet("tcp", "127.0.0.1:10")
		if err == nil {
			t.Error(err)
			return
		}
	}
	service.Stop()
	assertNotUnix(service.Config)
	if tester.Run("PrepareSocksConsole") {
		srv := NewService()
		srv.Config = &Config{}
		srv.Start()
		port, err := srv.PrepareSocksConsole("127.0.0.1:0")
		if err != nil || port < 1 {
			t.Error(err)
			return
		}
		port2, err := srv.PrepareSocksConsole("127.0.0.1:0")
		if err != nil || port != port2 {
			t.Error(err)
			return
		}
		srv.Console.SOCKS.Start("127.0.0.1:0")
		srv.Stop()
	}
	if tester.Run("StartError") {
		srv := NewService()
		err := srv.Start()
		if err == nil {
			t.Error(err)
			return
		}

		//config
		srv.ConfigPath = "xxx.json"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		srv.ConfigPath = ""

		srv.ConfigPath = "log.go"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		srv.ConfigPath = ""
		assertNotUnix(srv.Config)

		//Node Listen
		srv.Config = &Config{
			Listen: "127.0.0.1:x",
		}
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)

		//Forward
		srv.Config = &Config{
			Forwards: map[string]string{
				"xx~tcp://127.0.0.1:x": "xxx",
			},
		}
		err = srv.Start()
		if err != nil {
			t.Error(err)
			return
		}
		srv.Stop()
		assertNotUnix(srv.Config)

		//Console
		srv.Config = &Config{}
		srv.Config.Console.SOCKS = "127.0.0.1:x"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)
		srv.Config = &Config{}
		srv.Config.Console.WS = "127.0.0.1:x"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)

		//Proxy
		srv.Config = &Config{}
		srv.Config.Proxy.WEB = "127.0.0.1:x"
		srv.Config.Proxy.Addr = "127.0.0.1:x"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)
		srv.Config = &Config{}
		srv.Config.Proxy.WEB = "127.0.0.1:x"
		srv.Config.Proxy.Addr = "127.0.0.1:0"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)

		os.RemoveAll("/tmp/xxx__")
		os.MkdirAll("/tmp/xxx__", os.ModePerm)
		os.Chmod("/tmp/xxx__", 0)
		srv.Config = &Config{}
		srv.Config.Console.Unix = "/tmp/xxx__/xx"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)

		//Web
		srv.Config = &Config{}
		srv.Config.Web.Listen = "127.0.0.1:x"
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)

		//Dialer
		srv.Config = &Config{}
		srv.Config.Dialer = xmap.M{
			"dialers": []xmap.M{
				{
					"type": "xxx",
				},
			},
		}
		err = srv.Start()
		if err == nil {
			t.Error(err)
			return
		}
		assertNotUnix(srv.Config)

	}
}

var configTestMaster = `
{
    "name": "master",
    "listen": ":15023",
    "web": {},
    "console": {"unix":"none"},
    "forwards": {},
    "channels": {},
    "dialer": {
        "standard": 1
    },
    "acl": {
        "slaver": "a9993e364706816aba3e25717850c26c9cd0d89d",
        "caller": "a9993e364706816aba3e25717850c26c9cd0d89d"
    },
    "access": [
        [
            ".*",
            ".*"
        ]
    ]
}
`

var configTestSlaver = `
{
    "name": "slaver",
    "listen": "",
    "web": {},
    "console": {"unix":"none"},
    "forwards": {},
    "channels": {
        "master": {
            "enable": 1,
            "remote": "localhost:15023",
            "token": "abc",
            "index": 0
        }
    },
    "dialer": {
        "standard": 1,
		"ssh" : 1
    },
    "access": [
        [
            ".*",
            ".*"
        ]
    ]
}
`

var configTestCaller = `
{
    "name": "caller",
    "listen": "",
    "web": {},
    "console": {
        "socks": ":1701"
    },
    "forwards": {},
    "channels": {
        "master": {
            "enable": 1,
            "remote": "localhost:15023",
            "token": "abc",
            "index": 0
        }
    },
    "dialer": {
        "standard": 1
    }
}
`

var configTestProxy = `
{
    "name": "caller",
    "listen": "",
    "web": {},
    "console": {
        "socks": ":1701"
    },
    "proxy": {
        "web": "127.0.0.1:0",
        "addr": "127.0.0.1:0",
        "channel": ".*"
    },
    "gfwlist": {
        "channel": ".*"
    },
    "forwards": {},
    "channels": {
        "master": {
            "enable": 1,
            "remote": "localhost:15023",
            "token": "abc",
            "index": 0
        }
    },
    "dialer": {
        "standard": 1
    }
}
`

func TestProxy(t *testing.T) {
	// LogLevel
	os.WriteFile("/tmp/test_master.json", []byte(configTestMaster), os.ModePerm)
	os.WriteFile("/tmp/test_slaver.json", []byte(configTestSlaver), os.ModePerm)
	os.WriteFile("/tmp/test_caller.json", []byte(configTestProxy), os.ModePerm)
	defer func() {
		os.Remove("/tmp/test_master.json")
		os.Remove("/tmp/test_slaver.json")
		os.Remove("/tmp/test_caller.json")
	}()
	var err error
	//
	master := NewService()
	master.ConfigPath = "/tmp/test_master.json"
	err = master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewService()
	slaver.ConfigPath = "/tmp/test_slaver.json"
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
	caller := NewService()
	caller.ConfigPath = "/tmp/test_caller.json"
	err = caller.Start()
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(1000 * time.Millisecond)

	ts := httptest.NewServer(caller.Proxy.Mux)

	_, err = ts.GetText("/pac.js")
	if err != nil {
		t.Error(err)
		return
	}

	_, err = ts.GetText("/proxy?mode=global")
	if err != nil {
		t.Error(err)
		return
	}

	old := native.ChangeProxyScript
	native.ChangeProxyScript = "#!/bin/bash\nexit 1"
	_, err = ts.GetText("/proxy?mode=global")
	if err == nil {
		t.Error(err)
		return
	}
	native.ChangeProxyScript = old

	//proxy
	raw, err := caller.dialProxyPiper("tcp://127.0.0.1:13200", 4096)
	if err != nil {
		t.Error(err)
		return
	}
	raw.Close()
	//skip
	caller.Config.Proxy.Skip = []string{".*"}
	raw, err = caller.dialProxyPiper("tcp://127.0.0.1:13200", 4096)
	if err != nil {
		t.Error(err)
		return
	}
	raw.Close()
	_, err = caller.dialProxyPiper("tcp://127.0.0.1:10", 4096)
	if err == nil {
		t.Error(err)
		return
	}
	//gfwlist
	os.Remove("/tmp/gfwlist.txt.update")
	caller.UpdateGfwlist()
	caller.UpdateGfwlist()

	os.Remove("/tmp/gfwlist.txt.update")
	caller.Config.Gfwlist.Channel = "none"
	caller.UpdateGfwlist()

	caller.Node.Dir = "none"
	caller.Config.Gfwlist.Channel = ".*"
	caller.UpdateGfwlist()

	caller.Config.Gfwlist.Channel = ".*"
	caller.Config.Gfwlist.Source = "none"
	caller.UpdateGfwlist()

	caller.Node.Dir = caller.Config.Dir
	os.Remove("/tmp/gfwlist.txt.update")
	os.WriteFile("/tmp/gfwlist.txt.update", []byte(""), 01000)
	caller.UpdateGfwlist()
	os.Remove("/tmp/gfwlist.txt.update")

	//not channel
	caller.Config.Proxy.Channel = "none"
	caller.Config.Proxy.Skip = []string{}
	_, err = caller.dialProxyPiper("tcp://127.0.0.1:13200", 4096)
	if err == nil {
		t.Error(err)
		return
	}
	//err chnnale
	caller.Config.Proxy.Channel = "[xx"
	caller.Config.Proxy.Skip = []string{}
	_, err = caller.dialProxyPiper("tcp://127.0.0.1:13200", 4096)
	if err == nil {
		t.Error(err)
		return
	}

	caller.Stop()
	slaver.Stop()
	master.Stop()
	time.Sleep(100 * time.Millisecond)
	assertNotUnix(caller.Config)
}

func TestSSH(t *testing.T) {
	sshServer := &sshsrv.Server{
		Addr: "127.0.0.1:13322",
		ServerConfigCallback: func(ctx sshsrv.Context) *ssh.ServerConfig {
			return &ssh.ServerConfig{
				NoClientAuth: true,
			}
		},
		Handler: func(s sshsrv.Session) {
			cmd := exec.Command("bash")
			cmd.Stdin = s
			cmd.Stdout = s
			cmd.Stderr = s
			cmd.Run()
		},
	}
	defer sshServer.Close()
	go sshServer.ListenAndServe()
	// LogLevel
	os.WriteFile("/tmp/test_master.json", []byte(configTestMaster), os.ModePerm)
	os.WriteFile("/tmp/test_slaver.json", []byte(configTestSlaver), os.ModePerm)
	os.WriteFile("/tmp/test_caller.json", []byte(configTestCaller), os.ModePerm)
	defer func() {
		os.Remove("/tmp/test_master.json")
		os.Remove("/tmp/test_slaver.json")
		os.Remove("/tmp/test_caller.json")
	}()
	var err error
	//
	master := NewService()
	master.ConfigPath = "/tmp/test_master.json"
	err = master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewService()
	slaver.ConfigPath = "/tmp/test_slaver.json"
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
	caller := NewService()
	caller.ConfigPath = "/tmp/test_caller.json"
	err = caller.Start()
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 10; i++ { //ssh test
		info := fmt.Sprintf("data-%v", i)
		client, err := caller.DialSSH("master->slaver->tcp://127.0.0.1:13322", &ssh.ClientConfig{
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		})
		if err != nil {
			t.Error(err)
			return
		}
		ss, err := client.NewSession()
		if err != nil {
			t.Error(err)
			return
		}
		stdout := bytes.NewBuffer(nil)
		allout := bytes.NewBuffer(nil)
		ss.Stdin = bytes.NewBufferString("echo -n " + info)
		ss.Stdout, ss.Stderr = xio.NewMultiWriter(stdout, allout), allout
		err = ss.Run("bash")
		data := stdout.Bytes()
		if err != nil || string(data) != info {
			t.Errorf("err:%v,%v", err, string(data))
			return
		}
		ss.Close()
		client.Close()
	}

	_, err = caller.DialSSH("master->slaver->tcp://127.0.0.1:10", nil)
	if err == nil {
		t.Error(err)
		return
	}

	caller.Stop()
	slaver.Stop()
	master.Stop()
	time.Sleep(100 * time.Millisecond)
	assertNotUnix(caller.Config)
}

func TestState(t *testing.T) {
	var err error
	//
	master := NewService()
	json.Unmarshal([]byte(configTestMaster), &master.Config)
	err = master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewService()
	json.Unmarshal([]byte(configTestSlaver), &slaver.Config)
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
	caller := NewService()
	json.Unmarshal([]byte(configTestCaller), &caller.Config)
	err = caller.Start()
	if err != nil {
		t.Error(err)
		time.Sleep(1000 * time.Second)
		return
	}
	time.Sleep(100 * time.Millisecond)
	{ //state
		conna, connb, _ := xio.CreatePipedConn()
		_, err = caller.SyncDialAll("master->slaver->tcp://echo", connb)
		if err != nil {
			t.Error(err)
			return
		}
		state, err := caller.Client.GetMap(EncodeWebURI("http://(master->http://state)/"))
		if err != nil || len(state.Map("channels")) < 1 || len(state.ArrayDef(nil, "table")) < 1 {
			t.Error(converter.JSON(state))
			return
		}
		conna.Close()
	}
	caller.Stop()
	slaver.Stop()
	master.Stop()
	time.Sleep(100 * time.Millisecond)
	assertNotUnix(caller.Config)
}

type TestReverseDialer struct {
	Web0 *httptest.Server
	Web1 *httptest.Server
}

func (t *TestReverseDialer) Name() string {
	return "TestReverseDialer"
}

// initial dialer
func (t *TestReverseDialer) Bootstrap(options xmap.M) error {
	return nil
}

// shutdown
func (t *TestReverseDialer) Shutdown() error {
	return nil
}

func (t *TestReverseDialer) Options() xmap.M {
	return xmap.M{}
}

// match uri
func (t *TestReverseDialer) Matched(uri string) bool {
	return strings.HasPrefix(uri, "xx://")
}

// dial raw connection
func (t *TestReverseDialer) Dial(channel dialer.Channel, sid uint16, uri string) (conn dialer.Conn, err error) {
	targetURL, err := url.Parse(uri)
	if err != nil {
		return
	}
	var rawConn net.Conn
	switch targetURL.Host {
	case "web0":
		rawConn, err = net.Dial("tcp", strings.TrimPrefix(t.Web0.URL, "http://"))
	case "web1":
		rawConn, err = net.Dial("tcp", strings.TrimPrefix(t.Web1.URL, "http://"))
	default:
		err = fmt.Errorf("remote %v is not exists", targetURL.Host)
	}
	if err == nil {
		conn = xio.NewCopyPiper(rawConn, 4096)
	}
	return
}

func TestReverseWeb(t *testing.T) {
	master := NewService()
	master.Config = &Config{
		Name:   "master",
		Listen: ":12663",
		ACL: map[string]string{
			"slaver": xhash.SHA1([]byte("123")),
		},
		Access: [][]string{{".*", ".*"}},
	}
	master.Config.Console.Unix = "none"
	err := master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	defer master.Stop()
	slaver := NewService()
	slaver.Config = &Config{
		Name: "slaver",
		Channels: map[string]xmap.M{
			"master": {
				"enable": 1,
				"index":  1,
				"remote": ":12663",
				"token":  "123",
			},
		},
		Access: [][]string{{".*", ".*"}},
	}
	slaver.Config.Console.Unix = "none"
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
	defer slaver.Stop()
	time.Sleep(time.Millisecond * 100)

	//mock remote http server
	webSlaver0 := httptest.NewMuxServer()
	webSlaver0.Mux.HandleFunc("/test", func(s *web.Session) web.Result {
		return s.SendPlainText("web0")
	})
	webSlaver1 := httptest.NewMuxServer()
	webSlaver1.Mux.HandleFunc("/test", func(s *web.Session) web.Result {
		return s.SendPlainText("web1")
	})

	//register remote http server dialer
	dialerSlaver := &TestReverseDialer{
		Web0: webSlaver0,
		Web1: webSlaver1,
	}
	slaver.Dialer.AddDialer(dialerSlaver)

	//reverse remote handler
	handlerMaster := func(s *web.Session) web.Result {
		remoteID := strings.SplitN(strings.TrimPrefix(s.R.URL.Path, "/remote/"), "/", 2)[0]
		remoteHost := fmt.Sprintf("slaver->xx://%v", remoteID)
		reverseAddr := fmt.Sprintf("http://base64-%v/", base64.RawURLEncoding.EncodeToString([]byte(remoteHost)))
		reverseURL, _ := url.Parse(reverseAddr)
		proxy := httputil.NewSingleHostReverseProxy(reverseURL)
		proxy.Transport = &http.Transport{
			Dial: master.DialNet,
		}
		proxy.ServeHTTP(s.W, s.R)
		return web.Return
	}

	//mocker master http acceept
	webMaster := httptest.NewMuxServer()
	webMaster.Mux.HandleFunc("^/remote/.*$", handlerMaster)

	//request to web0
	web0Resp, err := webMaster.GetText("/remote/web0/test")
	if err != nil || web0Resp != "web0" {
		t.Errorf("%v,%v", err, web0Resp)
		return
	}
	//request to web1
	web1Resp, err := webMaster.GetText("/remote/web1/test")
	if err != nil || web1Resp != "web1" {
		t.Error(err)
		return
	}
	assertNotUnix(slaver.Config)
}

func TestSlaverHandler(t *testing.T) {
	master := NewService()
	master.Config = &Config{
		Name:   "master",
		Listen: ":12663",
		ACL: map[string]string{
			"slaver": xhash.SHA1([]byte("123")),
		},
		Access: [][]string{{".*", ".*"}},
	}
	master.Config.Console.Unix = "none"
	err := master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	defer master.Stop()
	slaver := NewService()
	slaver.Config = &Config{
		Name: "slaver",
		Channels: map[string]xmap.M{
			"master": {
				"enable": 1,
				"index":  1,
				"remote": ":12663",
				"token":  "123",
			},
		},
		Access: [][]string{{".*", ".*"}},
	}
	slaver.Config.Console.Unix = "none"
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
	defer slaver.Stop()
	time.Sleep(time.Millisecond * 100)

	//mock remote http server
	webMux := web.NewSessionMux("")
	webMux.HandleFunc("/query", func(s *web.Session) web.Result {
		return s.SendPlainText("handler")
	})
	webMux.HandleFunc("/body/err", func(s *web.Session) web.Result {
		n, err := io.Copy(s.W, s.R.Body)
		fmt.Printf("copy %v bytes data, err is %v\n", n, err)
		return web.Return
	})
	webMux.HandleFunc("/body/ok", func(s *web.Session) web.Result {
		buffer := bytes.NewBuffer(nil)
		n, err := io.Copy(buffer, s.R.Body)
		io.Copy(s.W, buffer)
		fmt.Printf("copy %v bytes data, err is %v\n", n, err)
		return web.Return
	})

	//register remote http server dialer
	dialerSlaver := dialer.NewWebDialer("handler", webMux)
	dialerSlaver.Bootstrap(xmap.M{})
	slaver.Dialer.AddDialer(dialerSlaver)

	//request to web1
	remoteHost := "slaver->http://handler"
	reverseAddr := fmt.Sprintf("http://base64-%v/", base64.RawURLEncoding.EncodeToString([]byte(remoteHost)))
	for i := 0; i < 5; i++ {
		handlerResp, err := master.Client.GetText("%v/query", reverseAddr)
		if err != nil || handlerResp != "handler" {
			t.Error(err)
			return
		}
	}
	data := make([]byte, 10240)
	for i := 0; i < 5; i++ {
		handlerResp, err := master.Client.PostJSONMap(xmap.M{"abc": 1, "data": data}, "%v/body/ok", reverseAddr)
		if err != nil || handlerResp.IntDef(0, "abc") != 1 {
			t.Error(err)
			return
		}
	}
	_, err = master.Client.PostJSONMap(xmap.M{"abc": 1, "data": data}, "%v/body/err", reverseAddr)
	if err == nil {
		t.Error(err)
		return
	}
	assertNotUnix(slaver.Config)
}

func TestResolveWhitelist(t *testing.T) {
	config := &Config{}
	if wl := config.ResolveWhitelist(); len(wl) > 0 {
		t.Error("error")
		return
	}
	config.Channels = map[string]xmap.M{
		"A": {
			"remote": "tcp://192.168.1.1:100",
		},
		"B": {
			"remote": "tcp://example.com:100",
		},
		"C": {
			"remote": "tcp://none.loc:100",
		},
		"ERR": {
			"remote": string([]byte{1, 2, 3}),
		},
	}
	config.Whitelist = []string{"192.1.1.1", "whitelist.txt", "none.txt"}
	wl := config.ResolveWhitelist()
	if len(wl) < 1 {
		t.Errorf("%v", wl)
		return
	}
	fmt.Printf("whitelist is \n%v\n", wl)
}

func TestHandlerMonitor(t *testing.T) {
	handler := &HandlerMonitor{
		Handler:         NewNormalAcessHandler("test"),
		AfterConnClose:  func(raw Conn) {},
		AfterConnJoin:   func(channel Conn, option interface{}, result xmap.M) {},
		AfterConnNotify: func(channel Conn, message []byte) {},
	}
	handler.OnConnClose(nil)
	handler.OnConnJoin(nil, nil, nil)
	handler.OnConnNotify(nil, nil)
}
