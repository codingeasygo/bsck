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

func TestForward(t *testing.T) {
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
			go func() {
				io.Copy(conn, raw)
				conn.Close()
				raw.Close()
			}()
			go func() {
				io.Copy(raw, conn)
				conn.Close()
				raw.Close()
			}()
			// time.Sleep(100 * time.Millisecond)
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

}

func TestForwadError(t *testing.T) {
	// test error
	forward := NewForward()
	//
	err := forward.AddForward("https://ech%EX%BU%XD%E6%96%87?xx=1", "https://echo?xx=%EX%BU%AD%E6%96%87")
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Printf("Err:%v\n", err)
	//
	err = forward.AddForward("https://echo?xx=%EX%BU%AD%E6%96%87", "https://echo?xx=%EX%BU%AD%E6%96%87")
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Printf("Err:%v\n", err)
	//
	err = forward.RemoveForward("https://ech%EX%BU%XD%E6%96%87?xx=1")
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Printf("Err:%v\n", err)
	//
	err = forward.RemoveForward("https://xxx?xx=1")
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Printf("Err:%v\n", err)
}
