package dialer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codingeasygo/util/xmap"
)

func TestTCPDialer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%v", "OK")
	}))
	tcp := NewTCPDialer()
	tcp.Bootstrap(nil)
	defer tcp.Shutdown()
	if !tcp.Matched("tcp://localhost:80") {
		t.Error("error")
		return
	}
	if tcp.Matched("https://xx/%EX%B8%AD%E8%AF%AD%E8%A8%80") {
		t.Error("error")
		return
	}
	con, err := tcp.Dial(nil, 10, ts.URL)
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	//
	con, err = tcp.Dial(nil, 10, "http://www.baidu.com")
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	con, err = tcp.Dial(nil, 10, "https://www.baidu.com")
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	//
	con, err = tcp.Dial(nil, 10, ts.URL+"?bind=0.0.0.0:0")
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	//
	fmt.Printf("%v test done...\n", tcp)
	tcp.Name()
	tcp.Options()
	//
	//test error
	_, err = tcp.Dial(nil, 10, ts.URL+"?bind=0.0.0.0")
	if err == nil {
		t.Error(err)
		return
	}
	//
	//test bind configure
	tcp = NewTCPDialer()
	tcp.Bootstrap(xmap.M{
		"bind": "0.0.0.0:0",
	})
	con, err = tcp.Dial(nil, 10, ts.URL)
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
}
