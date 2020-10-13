package bsck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
		closer := console.StartPing("master", 10*time.Millisecond)
		if err != nil {
			t.Error(err)
			return
		}
		time.Sleep(100 * time.Millisecond)
		closer()
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
		err := console.Proxy("master->tcp://${HOST}", nil, res, log, func(l net.Listener) (env []string, runner string, args []string, err error) {
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
		res.Reset()
		log.Reset()
		err = console.Proxy("master->tcp://${HOSTxx}", nil, res, log, func(l net.Listener) (env []string, runner string, args []string, err error) {
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
	}
	caller.Stop()
	slaver.Stop()
	master.Stop()
}
