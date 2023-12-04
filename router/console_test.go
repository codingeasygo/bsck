package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/proxy/socks"
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

var configTestConsole2 = `
{
    "name": "caller",
    "listen": "",
    "web": {},
    "console": {
        "ws":":1701"
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

func TestConsole(t *testing.T) {
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
		json.Unmarshal([]byte(configData), &caller.Config)
		err = caller.Start()
		if err != nil {
			t.Error(err)
			return
		}
		usr, _ := user.Current()
		sshKey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := ioutil.ReadFile(filepath.Join(usr.HomeDir, ".ssh", "id_rsa"))
			w.Write(data)
		}))
		caller.AddForward("ssh-key", sshKey.URL)
		time.Sleep(100 * time.Millisecond)
		conf := &Config{}
		json.Unmarshal([]byte(configData), conf)
		console, err := NewConsoleByConfig(conf)
		if err != nil {
			t.Error(err)
			return
		}
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
			//
			res.Reset()
			log.Reset()
			conn, err := exec.LookPath("conn")
			if err != nil {
				t.Errorf("find conn command is fail with %v", err)
				return
			}
			err = console.ProxySSH("dev.loc", bytes.NewBuffer(nil), res, log, conn+" tcp://dev.loc:22", "ssh", "-ltest", "echo", "-n", "OK")
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
		caller.Stop()
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
}
