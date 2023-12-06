package dialer

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xmap"
)

func testWebServer(addr string, dialer *WebDialer) (ln net.Listener, err error) {
	ln, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}
	go func() {
		var cid uint16
		for {
			con, err := ln.Accept()
			if err != nil {
				break
			}
			cid++
			raw, err := dialer.Dial(&testChannel{id: cid}, cid, "http://dav?dir=t0")
			if err != nil {
				panic(err)
			}
			go func() {
				buf := make([]byte, 1024)
				for {
					n, err := raw.Read(buf)
					if err != nil {
						con.Close()
						break
					}
					con.Write(buf[0:n])
				}
			}()
			go io.Copy(raw, con)
		}
	}()
	return
}

func TestWebDialer(t *testing.T) {
	tester := xdebug.CaseTester{
		0: 1,
		4: 1,
	}
	//dav
	if tester.Run("dav") {
		dialer := NewWebDialer("dav", NewWebdavHandler(xmap.M{
			"t0": "/tmp",
		}))
		ln, err := testWebServer(":2422", dialer)
		if err != nil {
			t.Error(err)
			return
		}
		dialer.Bootstrap(nil)
		if dialer.Matched("tcp://dav?dir=t0") {
			t.Error("error")
			return
		}
		if !dialer.Matched("http://dav?dir=t0") {
			t.Error("error")
			return
		}
		fmt.Println(xhttp.GetText("http://localhost:2422/"))
		dialer.Shutdown()
		time.Sleep(100 * time.Millisecond)
		ln.Close()
	}
	if tester.Run("srv") { //srv
		var dialer *WebDialer
		dialer = NewWebDialer("srv", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := dialer.FindConnByRequest(r.RemoteAddr)
			fmt.Printf("-->%v,%v\n", conn, err)
			channel, err := dialer.FindChannelByRequest(r.RemoteAddr)
			fmt.Printf("-->%v,%v\n", channel, err)
			fmt.Fprintf(w, "OK")
		}))
		ln, err := testWebServer(":2423", dialer)
		if err != nil {
			t.Error(err)
			return
		}
		dialer.Bootstrap(nil)
		fmt.Println(xhttp.GetText("http://localhost:2423/"))
		dialer.Shutdown()
		time.Sleep(100 * time.Millisecond)
		ln.Close()
	}
	if tester.Run("cover") { //for cover
		dialer := NewWebDialer("xx", nil)
		fmt.Printf("%v,%v\n", dialer.Addr(), dialer.Network())
		dialer.Name()
		dialer.Options()
		dialer.stopped = true
		dialer.Dial(nil, 0, "")
		dialer.FindConnByID("xxx")
		dialer.FindChannelByRequest("xxxx")
		dialer.cons["11"] = &WebDialerConn{}
		dialer.FindChannelByRequest("session_id=11")

		//
		conn, _, err := PipeWebDialerConn(&testChannel{}, 100, "http://dav?dir=t0")
		if err != nil {
			t.Error(err)
			return
		}
		conn.SetDeadline(time.Now())
		conn.SetReadDeadline(time.Now())
		conn.SetWriteDeadline(time.Now())
		fmt.Printf("%v,%v,%v\n", conn.LocalAddr(), conn.RemoteAddr(), conn.Network())
		conn, _, _ = PipeWebDialerConn(nil, 100, "http://dav?dir=t0")
		fmt.Printf("%v,%v,%v\n", conn.LocalAddr(), conn.RemoteAddr(), conn.Network())

		//
		addr := NewWebDialerAddr("", "")
		addr.Network()
	}
	if tester.Run("dav-cover") {
		dav := NewWebdavHandler(xmap.M{"xx": "/tmp"})
		req := httptest.NewRequest("GET", "http://localhost", nil)
		dav.ServeHTTP(httptest.NewRecorder(), req)

		req = httptest.NewRequest("GET", "http://localhost?dir=xx", nil)
		dav.ServeHTTP(httptest.NewRecorder(), req)

		req = httptest.NewRequest("GET", "http://localhost?dir=x0", nil)
		dav.ServeHTTP(httptest.NewRecorder(), req)

		req.RemoteAddr = "xxxx;xx;"
		dav.ServeHTTP(httptest.NewRecorder(), req)

		req.RemoteAddr = fmt.Sprintf("uri=%v", url.QueryEscape(string([]byte{1, 2, 3})))
		dav.ServeHTTP(httptest.NewRecorder(), req)

		f := NewWebdavFileHandler(".")
		req = httptest.NewRequest("POST", "http://localhost", nil)
		f.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func TestPipedConne(t *testing.T) {
	a, b := CreatePipedConn("a", "b")
	a.RemoteAddr()
	a.LocalAddr()
	fmt.Printf("-->%v\n", a)
	b.Close()
}
