package bsck

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

	"github.com/codingeasygo/util/proxy/socks"
)

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
	caller := NewService()
	json.Unmarshal([]byte(configTestCaller), &caller.Config)
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
	json.Unmarshal([]byte(configTestCaller), conf)
	console := NewConsole(conf.Console)
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
		if console.parseProxyURI("${URI}", "tcp://tcp-a.b.c") != "tcp://a.b.c" {
			t.Error("error")
			return
		}
		if console.parseProxyURI("${HOST}", "tcp://tcp-a.b.c") != "a.b.c" {
			t.Error("error")
			return
		}
		if console.parseProxyURI("${URI}", "tcp://a.b.c") != "tcp://a.b.c" {
			t.Error("error")
			return
		}
		if console.parseProxyURI("${HOST}", "tcp://a.b.c") != "a.b.c" {
			t.Error("error")
			return
		}
		if console.parseProxyURI("${URI}", "tcp://a") != "http://a" {
			t.Error("error")
			return
		}
		if console.parseProxyURI("${HOST}", "tcp://a") != "a" {
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
			fmt.Printf("out is \n%v\n", string(log.Bytes()))
			t.Error(err)
			return
		}
		fmt.Printf("out is \n%v\n", string(log.Bytes()))
		ts.Close()
		if string(res.Bytes()) != "OK" {
			t.Error(string(res.Bytes()))
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
			fmt.Printf("out is \n%v\n", string(log.Bytes()))
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
			fmt.Printf("out is \n%v\n", string(log.Bytes()))
			t.Error(err)
			return
		}
		fmt.Printf("out is \n%v\n", string(log.Bytes()))
		if string(res.Bytes()) == "OK" {
			t.Error(string(res.Bytes()))
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
			fmt.Printf("log is \n%v\n", string(log.Bytes()))
			t.Error(err)
			return
		}
		fmt.Printf("res is \n%v\n", string(res.Bytes()))
		if string(res.Bytes()) != "OK" {
			t.Error(string(res.Bytes()))
			return
		}
	}
	caller.Stop()
	slaver.Stop()
	master.Stop()
}
