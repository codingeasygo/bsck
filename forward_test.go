package bsck

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Centny/gwf/util"
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
	dialerPool.Bootstrap(util.Map{
		"standard": 1,
	})
	//
	var sidSequence uint64
	forward := NewForward()
	forward.WebSuffix = ".loc"
	forward.Dialer = func(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
		if strings.Contains(uri, "error") {
			err = fmt.Errorf("test error")
		} else {
			sid = atomic.AddUint64(&sidSequence, 1)
			_, err = dialerPool.Dial(sid, uri, raw)
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
		err = forward.AddForward("web://loctest4", "https://error")
		if err != nil {
			t.Error(err)
			return
		}
		if len(forward.FindForward("loctest0")) < 1 {
			t.Error("error")
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
		fmt.Printf("loctest0->:\n%v\n\n\n\n", data)
		//
		forward.WebAuth = "test:123"
		data, err = hget("%v/web/loctest1", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("loctest1->:\n%v\n\n\n\n", data)
		forward.WebAuth = ""
		//
		data, err = hget("%v/web/loctest2", ts.URL)
		if err != nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("loctest2->:\n%v\n\n\n\n", len(data))
		//
		data, err = hget("%v/web/loctest3", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("loctest3->:\n%v\n\n\n\n", data)
		//
		data, err = hget("%v/web/loctest4", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("loctest4->:\n%v\n\n\n\n", data)
		//
		data, err = hget("%v/web/xxxx", ts.URL)
		if err == nil {
			t.Errorf("%v-%v", err, data)
			return
		}
		fmt.Printf("xxxx->:\n%v\n\n\n\n", data)
		//
		forward.RemoveForward("loctest0")
		forward.RemoveForward("loctest1")
		forward.RemoveForward("loctest2")
		forward.RemoveForward("loctest3")
		//
	}
	{ //test sub forward
		err := forward.AddForward("ws://t0", "tcp://echo")
		if err != nil {
			t.Error(err)
			return
		}
		if len(forward.FindForward("t0")) < 1 {
			t.Error("error")
			return
		}
		err = forward.AddForward("ws://t1", "tcp://error")
		if err != nil {
			t.Error(err)
			return
		}
		err = forward.AddForward("web://t2", "http://web?dir=./")
		if err != nil {
			t.Error(err)
			return
		}
		forward.webMapping["terr"] = []string{"web://terr", "http://we%EX%B8%AG%E6b?dir=dds%EX%B8%AG%E6%96%87"}
		ts := httptest.NewServer(http.HandlerFunc(forward.ProcWebSubsH))
		tsURL, _ := url.Parse(ts.URL)
		//
		//
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
		//for run error test.
		websocket.Dial("ws://"+tsURL.Host+"/ws/t1", "", ts.URL)
		//
		//
		wsconn2, err := websocket.Dial("ws://"+tsURL.Host+"/ws/?router="+url.QueryEscape("tcp://echo?x=a"), "", ts.URL)
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Fprintf(wsconn2, "data->%v", 0)
		readed, err = wsconn2.Read(buf)
		fmt.Println("->", readed, string(buf[:readed]), err)
		wsconn2.Close()
		//
		data, err := hget("%v/dav/t2/build.sh", ts.URL)
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
		forward.RemoveForward("t0")
		forward.RemoveForward("t1")
		forward.RemoveForward("t2")
	}

}

func TestForwadError(t *testing.T) {
	// test error
	forward := NewForward()
	forward.AddForward("ws://t0", "tcp://xx")
	err := forward.AddForward("ws://t0", "tcp://xx")
	if err == nil {
		t.Error(err)
		return
	}
	//
	err = forward.AddForward("https://ech%EX%BU%XD%E6%96%87?xx=1", "https://echo?xx=%EX%BU%AD%E6%96%87")
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
}

func TestForwardUri(t *testing.T) {
	_, _, err := ForwardUri([]string{"tcp://loc", "xx://loc"}).URL()
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = ForwardUri([]string{"tcp://loc", "a->xx://loc"}).URL()
	if err != nil {
		t.Error(err)
		return
	}
}

func TestWaitReadWriteCloser(t *testing.T) {
	cona, conb, _ := dialer.CreatePipedConn()
	wrwc := NewWaitReadWriteCloser(conb)
	fmt.Printf("%v\n", wrwc)
	wrwc.ReadWriteCloser = nil
	fmt.Printf("%v\n", wrwc)
	cona.Close()

}
