package bsck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestConsole(t *testing.T) {
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
	// { //http
	// 	state, err := console.Client.GetMap(EncodeWebURI("http://(http://state)?*=*"))
	// 	if err != nil || len(state) < 1 {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	_, err = console.Client.GetMap("http://base64-xxx?*=*")
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}

	// }
	// { //redirect
	// 	in := bytes.NewBufferString("hello")
	// 	out := bytes.NewBuffer(nil)
	// 	err := console.Redirect("tcp://echo", in, out, nil)
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// }
	// { //dial
	// 	conn, err := console.Dial("tcp://echo")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	conn.Close()
	// }
	{ //Proxy
		out := bytes.NewBuffer(nil)
		err := console.Proxy("master->tcp://${HOST}", "curl", "http_proxy", nil, out, out, "-v", "http://github.com")
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Printf("out is %v\n", string(out.Bytes()))
	}
	caller.Stop()
	slaver.Stop()
	master.Stop()
	time.Sleep(100 * time.Millisecond)
}
