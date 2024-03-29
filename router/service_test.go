package router

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/web"
	"github.com/codingeasygo/web/httptest"
	"golang.org/x/crypto/ssh"
)

var configTest1 = `
{
    "name": "r0",
    "listen": ":15023",
    "web": {
        "listen": ":15024",
        "suffix": ".test.loc:15024",
        "auth": ""
    },
    "socks5": ":5081",
    "forwards": {
		"tx~tcp://:2332": "http://web?dir=/tmp"
	},
    "channels": [],
    "dialer":{
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
    "socks5": ":5081",
    "forwards": {
        "t0~web://t0": "http://127.0.0.1:80",
        "t1~web://t1": "http://web?dir=.",
        "t2~tcp://:2332": "http://dav?dir=.",
        "t3~socks://localhost:10322": "tcp://echo",
        "t4~rdp://localhost:0": "tcp://127.0.0.1:22",
		"t5~vnc://localhost:0": "tcp://dev.loc:22",
		"w1~web://w1": "tcp://echo?abc=1",
		"w2": "tcp://echo",
		"w3": "http://dav?dir=.",
		"w40": "http://test1",
		"w41": "http://test1",
		"w5": "tcp://dev.loc:22",
		"w6": "http://state"
    },
    "channels": [],
    "dialer":{
        "standard": 1
    }
}
`

func TestService(t *testing.T) {
	socks.SetLogLevel(socks.LogLevelDebug)
	SetLogLevel(LogLevelDebug)
	ioutil.WriteFile("/tmp/test.json", []byte(configTest1), os.ModePerm)
	defer os.Remove("/tmp/test.json")
	service := NewService()
	service.ConfigPath = "/tmp/test.json"
	service.Webs = map[string]http.Handler{
		"test1": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "abc")
		}),
	}
	service.Finder = ForwardFinderF(func(uri string) (target string, err error) {
		switch uri {
		case "find0":
			target = "http://dav?dir=."
		default:
			err = fmt.Errorf("not supported %v", uri)
		}
		return
	})
	err := service.Start()
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(10 * time.Millisecond)
	ioutil.WriteFile("/tmp/test.json", []byte(configTest2), os.ModePerm)
	err = service.ReloadConfig()
	time.Sleep(100 * time.Millisecond)
	runTestEcho := func(name string, echoa, echob *xio.PipeReadWriteCloser) {
		wc := make(chan int, 1)
		go func() {
			buf := make([]byte, 128)
			for i := 0; i < 10; i++ {
				n, _ := echoa.Read(buf)
				fmt.Printf("%v received %v", name, string(buf[0:n]))
				wc <- 1
			}
			echoa.Close()
			wc <- 1
		}()
		for i := 0; i < 10; i++ {
			fmt.Fprintf(echoa, "abc\n")
			<-wc
		}
		<-wc
		_, err = fmt.Fprintf(echoa, "abc\n")
		if err == nil {
			t.Error(err)
			return
		}
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		dialer := dialer.NewSocksProxyDialer()
		dialer.Bootstrap(xmap.M{
			"id":      "testing",
			"address": "localhost:10322",
		})
		_, err = dialer.Dial(nil, 1000, "echo", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks0", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SyncDialAll("tcp://echo", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks1", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SyncDialAll("tcp://echo?abc=1", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks2", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SyncDialAll("w1?abc=1", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks3", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SyncDialAll("w2?abc=1", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks4", echoa, echob)
	}
	{ //web test
		data, err := xhttp.GetText("http://:2332?abc=123")
		if err != nil || !strings.Contains(data, "router.go") {
			t.Error(err)
			return
		}
		fmt.Printf("%v\n", data)
	}
	{ //web test
		data, err := service.Client.GetText("http://w3?abc=123")
		if err != nil || !strings.Contains(data, "router.go") {
			t.Error(err)
			return
		}
		fmt.Printf("%v\n", data)
	}
	{ //web test
		data, err := service.Client.GetText("http://w40,w41")
		if err != nil || data != "abc" {
			t.Error(err)
			return
		}
		fmt.Printf("%v\n", data)
	}
	{ //finder test
		data, err := service.Client.GetText("http://find0")
		if err != nil || !strings.Contains(data, "router.go") {
			t.Error(err)
			return
		}
		fmt.Printf("%v\n", data)
	}
	{ //ssh test
		client, err := service.DialSSH("w5", &ssh.ClientConfig{
			User:            "test",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth:            []ssh.AuthMethod{ssh.Password("123")},
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
		ss.Stdin = bytes.NewBufferString("echo -n abc")
		data, err := ss.Output("bash")
		if err != nil || string(data) != "abc" {
			t.Errorf("err:%v,%v", err, string(data))
			return
		}
	}
	{ //state test
		data, err := service.Client.GetMap("http://w6?*=*")
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Printf("%v\n", converter.JSON(data))
	}
	//
	ioutil.WriteFile("/tmp/test.json", []byte(configTest1), os.ModePerm)
	err = service.ReloadConfig()
	time.Sleep(10 * time.Millisecond)
	service.Stop()
	time.Sleep(10 * time.Millisecond)
}

var configTestMaster = `
{
    "name": "master",
    "listen": ":15023",
    "web": {},
    "console": {},
    "forwards": {},
    "channels": [],
    "dialer": {
        "standard": 1
    },
    "acl": {
        "slaver": "abc",
        "caller": "abc"
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
    "console": {},
    "forwards": {},
    "channels": [
        {
            "enable": 1,
            "remote": "localhost:15023",
            "token": "abc",
            "index": 0
        }
    ],
    "dialer": {
        "standard": 1
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
        "socks":":1701"
    },
    "forwards": {},
    "channels": [
        {
            "enable": 1,
            "remote": "localhost:15023",
            "token": "abc",
            "index": 0
        }
    ],
    "dialer": {
        "standard": 1
    }
}
`

func TestSSH(t *testing.T) {
	// LogLevel
	ioutil.WriteFile("/tmp/test_master.json", []byte(configTestMaster), os.ModePerm)
	ioutil.WriteFile("/tmp/test_slaver.json", []byte(configTestSlaver), os.ModePerm)
	ioutil.WriteFile("/tmp/test_caller.json", []byte(configTestCaller), os.ModePerm)
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
		client, err := caller.DialSSH("master->slaver->tcp://dev.loc:22", &ssh.ClientConfig{
			User:            "test",
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth:            []ssh.AuthMethod{ssh.Password("123")},
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
	caller.Stop()
	slaver.Stop()
	master.Stop()
	time.Sleep(100 * time.Millisecond)
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
		state, err := caller.Client.GetMap(EncodeWebURI("http://(master->http://state)/?*=*"))
		if err != nil || len(state.Map("channels")) < 1 || len(state.ArrayDef(nil, "table")) < 1 {
			t.Error(converter.JSON(state))
			return
		}
		state, err = caller.Client.GetMap(EncodeWebURI("http://(master->http://state)/?a=111"))
		if err != nil || len(state.Map("channels")) > 0 || len(state.ArrayDef(nil, "table")) > 0 {
			t.Error(state)
			return
		}
		state, err = caller.Client.GetMap(EncodeWebURI("http://(master->http://state)"))
		if err != nil || len(state.Map("channels")) > 0 || len(state.ArrayDef(nil, "table")) > 0 {
			t.Error(state)
			return
		}
		conna.Close()
	}
	caller.Stop()
	slaver.Stop()
	master.Stop()
	time.Sleep(100 * time.Millisecond)
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
func (t *TestReverseDialer) Dial(channel dialer.Channel, sid uint64, uri string, raw io.ReadWriteCloser) (conn dialer.Conn, err error) {
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
		conn = dialer.NewCopyPipable(rawConn)
	}
	return
}

func TestReverseWeb(t *testing.T) {
	master := NewService()
	master.Config = &Config{
		Name:   "master",
		Listen: ":12663",
		ACL: map[string]string{
			"slaver": "123",
		},
		Access: [][]string{{".*", ".*"}},
	}
	err := master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewService()
	slaver.Config = &Config{
		Name: "slaver",
		Channels: []xmap.M{
			{
				"enable": 1,
				"index":  1,
				"remote": ":12663",
				"token":  "123",
			},
		},
		Access: [][]string{{".*", ".*"}},
	}
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
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
}

func TestSlaverHandler(t *testing.T) {
	master := NewService()
	master.Config = &Config{
		Name:   "master",
		Listen: ":12663",
		ACL: map[string]string{
			"slaver": "123",
		},
		Access: [][]string{{".*", ".*"}},
	}
	err := master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewService()
	slaver.Config = &Config{
		Name: "slaver",
		Channels: []xmap.M{
			{
				"enable": 1,
				"index":  1,
				"remote": ":12663",
				"token":  "123",
			},
		},
		Access: [][]string{{".*", ".*"}},
	}
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
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
}
