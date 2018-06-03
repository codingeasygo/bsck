package bsck

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sutils/dialer"

	"golang.org/x/net/websocket"
)

func hget(format string, args ...interface{}) (data string, err error) {
	url := fmt.Sprintf(format, args...)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	if res.StatusCode != 200 {
		err = fmt.Errorf("code:%v", res.StatusCode)
		return
	}
	defer res.Body.Close()
	bys, err := ioutil.ReadAll(res.Body)
	data = string(bys)
	return
}

func TestForwad(t *testing.T) {
	dialerPool := dialer.NewPool()
	dialerPool.AddDialer(dialer.NewCmdDialer())
	dialerPool.AddDialer(dialer.NewEchoDialer())
	dialerPool.AddDialer(dialer.NewWebDialer())
	dialerPool.AddDialer(dialer.NewTCPDialer())
	//

	forward := NewForward()
	forward.WebSuffix = ".loc"
	forward.Dailer = func(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
		conn, err := dialerPool.Dial(0, uri)
		if err == nil {
			go io.Copy(conn, raw)
			go io.Copy(raw, conn)
			time.Sleep(100 * time.Millisecond)
		}
		fmt.Println("dial to ", uri, err)
		return
	}
	{ //test web forward
		err := forward.AddForward("web://loctest0", "http://127.0.0.1:80")
		if err != nil {
			t.Error(err)
			return
		}
		err = forward.AddForward("web://loctest1?auth=1", "http://127.0.0.1:80")
		if err != nil {
			t.Error(err)
			return
		}
		err = forward.AddForward("web://loctest2", "https://www.baidu.com:443")
		if err != nil {
			t.Error(err)
			return
		}
		err = forward.AddForward("web://loctest3", "https://127.0.0.1:80")
		if err != nil {
			t.Error(err)
			return
		}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			name := strings.TrimPrefix(req.URL.Path, "/web/")
			switch name {
			case "loctest0":
				req.Host = "loctest0.loc"
			case "loctest1":
				req.Host = "loctest1.loc"
			case "loctest2":
				req.Host = "loctest2.loc"
			case "loctest3":
				req.Host = "loctest3.loc"
			case "loctest4":
				req.Host = "loctest4.loc"
			}
			req.URL.Path = "/"
			forward.HostForwardF(w, req)
		}))
		//
		data, err := hget("%v/web/loctest0", ts.URL)
		if err != nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("data->:\n%v\n\n\n\n", data)
		//
		forward.WebAuth = "test:123"
		data, err = hget("%v/web/loctest1", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("data->:\n%v\n\n\n\n", data)
		forward.WebAuth = ""
		//
		data, err = hget("%v/web/loctest2", ts.URL)
		if err != nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("data->:\n%v\n\n\n\n", data)
		//
		forward.RemoveForward("web://loctest0")
		forward.RemoveForward("web://loctest1")
		//
	}
	{ //test sub forward
		err := forward.AddForward("ws://t0", "tcp://echo")
		if err != nil {
			t.Error(err)
			return
		}
		err = forward.AddForward("web://t1", "http://web?dir=./")
		if err != nil {
			t.Error(err)
			return
		}
		forward.webMapping["terr"] = []string{"web://terr", "http://we%EX%B8%AG%E6b?dir=dds%EX%B8%AG%E6%96%87"}
		ts := httptest.NewServer(http.HandlerFunc(forward.ProcWebSubsH))
		tsURL, _ := url.Parse(ts.URL)
		wsconn, err := websocket.Dial("ws://"+tsURL.Host+"/ws/t0", "", ts.URL)
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Fprintf(wsconn, "data->%v", 0)
		buf := make([]byte, 1024)
		readed, err := wsconn.Read(buf)
		fmt.Println("->", readed, string(buf[:readed]), err)
		wsconn.Close()
		//
		data, err := hget("%v/dav/t1/build.sh", ts.URL)
		if err != nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("res->:\n%v\n\n\n\n", data)
		//
		data, err = hget("%v/dav/terr", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("res->:\n%v\n\n\n\n", err)
		//
		data, err = hget("%v/dav/xx", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("res->:\n%v\n\n\n\n", err)
		//
		data, err = hget("%v/da", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("res->:\n%v\n\n\n\n", err)
		//
		forward.RemoveForward("ws://t0")
	}
	// { //test tcp forward
	// 	//test spicial port
	// 	err = testmap("x1", "tcp://:7234<master>tcp://localhost:9392")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	//test auto create port
	// 	err = testmap("x2", "tcp://<master>tcp://localhost:9392")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	//test remote not found
	// 	err = testmap("x3", "tcp://:7235<not>tcp://localhost:9392")
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// }
	// { //test forward limit
	// 	_, err = forward.AddUriForward("xv-0", "tcp://:23221?limit=1<master>tcp://localhost:9392")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	con, err := net.Dial("tcp", "localhost:23221")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	fmt.Fprintf(con, "value-%v", 1)
	// 	buf := make([]byte, 100)
	// 	readed, err := con.Read(buf)
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	if string(buf[0:readed]) != "value-1" {
	// 		t.Error("error")
	// 		return
	// 	}
	// 	_, err = net.Dial("tcp", "localhost:23221")
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	time.Sleep(200 * time.Millisecond)
	// 	err = forward.RemoveForward("tcp://:23221")
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// }
	// { //add/remove forward
	// 	_, err = forward.AddUriForward("xy-0", "tcp://:24221<master>tcp://localhost:2422")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	_, err = forward.AddUriForward("xy-1", "tcp://:24221<master>tcp://localhost:2422") //repeat
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	//
	// 	_, err = forward.AddUriForward("xy-2", "tcp://:2322?limit=xxx<master>tcp://localhost:2422")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	//
	// 	_, err = forward.AddUriForward("xy-3", "web://loc1<master>http://localhost:2422")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	_, err = forward.AddUriForward("xy-4", "web://loc1<master>http://localhost:2422") //repeat
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	//
	// 	_, err = forward.AddUriForward("xy-5", "xxxx://:24221<master>tcp://localhost:2422") //not suppored
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	//
	// 	// err = forward.RemoveForward("tcp://:24221")
	// 	// if err != nil {
	// 	// 	t.Error(err)
	// 	// 	return
	// 	// }
	// 	err = forward.RemoveForward("tcp://:2322")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	err = forward.RemoveForward("web://loc1")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	//test error
	// 	err = forward.RemoveForward("tcp://:283x")
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	err = forward.RemoveForward("web://loctestxxx")
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	err = forward.RemoveForward("://loctestxxx")
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// }
	// { //test forward name not found
	// 	_, err = forward.AddUriForward("xc-0", "tcp://:23221<xxxx>tcp://localhost:9392")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	con, err := net.Dial("tcp", "localhost:23221")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	buf := make([]byte, 100)
	// 	_, err = con.Read(buf)
	// 	if err == nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	err = forward.RemoveForward("tcp://:23221")
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// }
	// //test error
	// {
	// 	//name repeat
	// 	_, err = forward.AddUriForward("x1", "tcp://:7234<x>tcp://localhost:9392")
	// 	if err == nil {
	// 		t.Error("nil")
	// 		return
	// 	}
	// 	//local repeat
	// 	_, err = forward.AddUriForward("xx", "tcp://:7234<x>tcp://localhost:9392")
	// 	if err == nil {
	// 		t.Error("nil")
	// 		return
	// 	}
	// 	//listen error
	// 	_, err = forward.AddUriForward("xx", "tcp://:7<x>tcp://localhost:9392")
	// 	if err == nil {
	// 		t.Error("nil")
	// 		return
	// 	}
	// }
	// ms := forward.List()
	// if len(ms) < 2 {
	// 	t.Error("mapping error")
	// 	return
	// }
	// forward.Stop("x1", true)
	// forward.Stop("x2", true)
	// // forward.Stop("x3", true)
	// //test error
	// {
	// 	err = forward.Stop("not", false)
	// 	if err == nil {
	// 		t.Error("nil")
	// 		return
	// 	}
	// }
	// forward.Close()
	// time.Sleep(time.Second)
	// client.Close()
	// slaver.Close()
	// slaver2.Close()
	// master.Close()
	// time.Sleep(time.Second)

}
