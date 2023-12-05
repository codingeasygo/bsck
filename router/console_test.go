package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/proxy/socks"
	sshsrv "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

func TestHosts(t *testing.T) {
	hosts := NewRewrite()
	err := hosts.Read("/etc/hosts")
	if err != nil {
		t.Error(err)
		return
	}
	if _, ok := hosts.Match("localhost"); !ok {
		t.Error("error")
		return
	}
	os.WriteFile("/tmp/hosts", []byte(`127.0.0.1 a.test.loc *.xxx.loc`), os.ModePerm)
	err = hosts.Read("/tmp/hosts")
	if err != nil {
		t.Error(err)
		return
	}
	if _, ok := hosts.Match("a.test.loc"); !ok {
		t.Error("error")
		return
	}
	if _, ok := hosts.Match("x.xxx.loc"); !ok {
		t.Error("error")
		return
	}
	if _, ok := hosts.Match("none.loc"); ok {
		t.Error("error")
		return
	}
	fmt.Println(converter.JSON(hosts))
	//
	err = hosts.Read("/tmp/none")
	if err == nil {
		t.Error(err)
		return
	}
}

var configTestConsole1 = `
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

var configTestConsole2 = `
{
    "name": "caller",
    "listen": "",
    "web": {},
    "console": {
        "ws": ":1701"
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

func TestConsole(t *testing.T) {
	sshServer := &sshsrv.Server{
		Addr: "127.0.0.1:13322",
		ServerConfigCallback: func(ctx sshsrv.Context) *ssh.ServerConfig {
			return &ssh.ServerConfig{
				NoClientAuth: true,
			}
		},
		Handler: func(s sshsrv.Session) {
			fmt.Println("---->xxxx-->")
			cmd := exec.Command("bash")
			cmd.Stdin = s
			cmd.Stdout = s
			cmd.Stderr = s
			cmd.Run()
		},
	}
	defer sshServer.Close()
	go sshServer.ListenAndServe()
	//
	socks.SetLogLevel(socks.LogLevelDebug)
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
	testCaller := func(configData string) {
		caller := NewService()
		err = json.Unmarshal([]byte(configData), &caller.Config)
		if err != nil {
			t.Error(err)
			return
		}
		err = caller.Start()
		if err != nil {
			t.Error(err)
			return
		}
		defer caller.Stop()
		time.Sleep(100 * time.Millisecond)
		conf := &Config{}
		json.Unmarshal([]byte(configData), conf)
		console, err := NewConsoleByConfig(conf)
		if err != nil {
			t.Error(err)
			return
		}
		defer console.Close()
		{ //http
			state, err := console.Client.GetMap(EncodeWebURI("http://(http://state)?*=*"))
			if err != nil || len(state) < 1 {
				t.Error(err)
				return
			}
			_, err = console.Client.GetMap("http://base64-x++?*=*")
			if err == nil {
				t.Error(err)
				return
			}
		}
		{ //redirect
			in := bytes.NewBufferString("hello")
			out := bytes.NewBuffer(nil)
			err := console.Redirect("tcp://echo", in, out, nil)
			if err != nil {
				t.Error(err)
				return
			}
		}
		{ //dial
			conn, err := console.Dial("tcp://echo")
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			_, err = console.Dial("tcxp://xxecho")
			if err == nil {
				t.Error(err)
				return
			}
		}
		{ //Ping
			err := console.Ping("master", 10*time.Millisecond, 3)
			if err != nil {
				t.Error(err)
				return
			}
			err = console.Ping("master->tcp://127.0.0.1:10", 10*time.Millisecond, 3)
			if err == nil {
				t.Error(err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		{ //PrintState
			conn, err := console.Dial("master->slaver->tcp://echo")
			if err != nil {
				t.Error(err)
				return
			}
			console.PrintState("", "*")
			console.PrintState("master", "*")
			conn.Close()
			console.PrintState("masterx", "*")
		}
		{ //parseProxyURI
			if result, _ := console.parseProxyURI("${URI}", "tcp://tcp-a.b.c"); result != "tcp://a.b.c" {
				t.Error("error")
				return
			}
			if result, _ := console.parseProxyURI("${HOST}", "tcp://tcp-a.b.c"); result != "a.b.c" {
				t.Error("error")
				return
			}
			if result, _ := console.parseProxyURI("${URI}", "tcp://a.b.c"); result != "tcp://a.b.c" {
				t.Error("error")
				return
			}
			if result, _ := console.parseProxyURI("${HOST}", "tcp://a.b.c"); result != "a.b.c" {
				t.Error("error")
				return
			}
			if result, _ := console.parseProxyURI("${URI}", "tcp://a"); result != "http://a" {
				t.Error("error")
				return
			}
			if result, _ := console.parseProxyURI("${HOST}", "tcp://a"); result != "a" {
				t.Error("error")
				return
			}
			if _, err := console.parseProxyURI("${HOST}", "a.b."+string([]byte{1, 0x7f})); err == nil {
				t.Error("error")
				return
			}
			console.Rewrite = NewRewrite()
			console.Rewrite.Single["a.b.c"] = "x.y.z"
			if result, _ := console.parseProxyURI("${HOST}", "tcp://a.b.c:80"); result != "x.y.z:80" {
				t.Error(result)
				return
			}
		}
		{ //forward
			ln, err := console.StartForward("127.0.0.1:0", "master->tcp://echo")
			if err != nil {
				t.Error(err)
				return
			}
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(conn, "abc")
			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil || string(buffer[:n]) != "abc" {
				t.Error(err)
				return
			}
			conn.Close()
			ln.Close()
			//
			//dial error
			ln, err = console.StartForward("127.0.0.1:0", "xxx->tcp://echo")
			if err != nil {
				t.Error(err)
				return
			}
			conn, err = net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Error(err)
				return
			}
			_, err = conn.Read(buffer)
			if err == nil {
				t.Error(err)
				return
			}
			//
			//start error
			_, err = console.StartForward("127.0.0.1:x", "master->tcp://echo")
			if err == nil {
				t.Error(err)
				return
			}
		}
		{ //Proxy
			home, _ := exec.Command("bash", "-c", "echo $HOME").Output()
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "OK")
			}))
			res := bytes.NewBuffer(nil)
			log := bytes.NewBuffer(nil)
			//
			res.Reset()
			log.Reset()
			err := console.ProxyExec("master->tcp://${HOST}", nil, res, log, func(l net.Listener) (env []string, runner string, args []string, err error) {
				env = []string{"http_proxy=http://" + l.Addr().String(), "HOME=" + string(home)}
				runner = "curl"
				args = []string{"-v", ts.URL}
				return
			})
			if err != nil {
				fmt.Printf("out is \n%v\n", log.String())
				t.Error(err)
				return
			}
			fmt.Printf("out is \n%v\n", log.String())
			ts.Close()
			if res.String() != "OK" {
				t.Error(res.String())
				return
			}
			//
			err = console.ProxyProcess("master->tcp://${HOST}", nil, os.Stdout, os.Stderr, func(l net.Listener) (env []string, runner string, args []string, err error) {
				env = []string{"http_proxy=http://" + l.Addr().String(), "HOME=" + string(home)}
				runner, err = exec.LookPath("curl")
				args = []string{"-v", ts.URL}
				return
			})
			if err != nil {
				fmt.Printf("out is \n%v\n", log.String())
				t.Error(err)
				return
			}
			err = console.ProxyProcess("master->tcp://${HOST}", nil, os.Stdout, os.Stderr, func(l net.Listener) (env []string, runner string, args []string, err error) {
				err = fmt.Errorf("test error")
				return
			})
			if err == nil {
				t.Error(err)
				return
			}
			//
			res.Reset()
			log.Reset()
			err = console.ProxyExec("master->tcp://${HOSTxx}", nil, res, log, func(l net.Listener) (env []string, runner string, args []string, err error) {
				env = []string{"http_proxy=http://" + l.Addr().String(), "HOME=" + string(home)}
				runner = "curl"
				args = []string{"-v", ts.URL}
				return
			})
			if err != nil {
				fmt.Printf("out is \n%v\n", log.String())
				t.Error(err)
				return
			}
			fmt.Printf("out is \n%v\n", log.String())
			if res.String() == "OK" {
				t.Error(res.String())
				return
			}
			err = console.ProxyExec("master->tcp://${HOSTxx}", nil, res, log, func(l net.Listener) (env []string, runner string, args []string, err error) {
				err = fmt.Errorf("test error")
				return
			})
			if err == nil {
				t.Error(err)
				return
			}
			//
			res.Reset()
			log.Reset()
			err = console.ProxySSH("", bytes.NewBuffer([]byte("echo -n OK")), res, log, "nc 127.0.0.1 13322", "ssh", "-o", "StrictHostKeyChecking=no")
			if err != nil {
				fmt.Printf("log is \n%v\n", log.String())
				t.Error(err)
				return
			}
			fmt.Printf("res is \n%v\n", res.String())
			if res.String() != "OK" {
				t.Error(res.String())
				return
			}
			res.Reset()
			log.Reset()
			err = console.ProxySSH("dev", bytes.NewBuffer([]byte("echo -n OK")), res, log, "nc 127.0.0.1 13322", "ssh", "-o", "StrictHostKeyChecking=no")
			if err != nil {
				fmt.Printf("log is \n%v\n", log.String())
				t.Error(err)
				return
			}
			fmt.Printf("res is \n%v\n", res.String())
			if res.String() != "OK" {
				t.Error(res.String())
				return
			}
		}
	}
	testCaller(configTestConsole1)
	testCaller(configTestConsole2)
	slaver.Stop()
	master.Stop()
	_, err = NewConsoleByConfig(&Config{})
	if err == nil {
		t.Error(err)
		return
	}
	_, err = NewConsole("").Dial("tcp://echo")
	if err == nil {
		t.Error(err)
		return
	}
	console := NewConsole("")
	console.running["xxx"] = &ErrReadWriteCloser{}
	console.Close()
}
