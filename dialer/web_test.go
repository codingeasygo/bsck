package dialer

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/web"
	"github.com/codingeasygo/web/httptest"
)

func TestWebDialer(t *testing.T) {
	//test web dialer
	l, err := net.Listen("tcp", ":2422")
	if err != nil {
		t.Error(err)
		return
	}
	dialer := NewWebDialer("dav", NewWebdavHandler())
	dialer.Bootstrap(nil)
	if dialer.Matched("tcp://dav?dir=/tmp") {
		t.Error("error")
		return
	}
	if !dialer.Matched("http://dav?dir=/tmp") {
		t.Error("error")
		return
	}
	//
	go func() {
		var cid uint64
		for {
			con, err := l.Accept()
			if err != nil {
				break
			}
			raw, err := dialer.Dial(cid, "http://dav?dir=/tmp", nil)
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
	fmt.Println(xhttp.GetText("http://localhost:2422/"))
	fmt.Println(xhttp.GetText("http://localhost:2422/"))
	//
	//test pipe
	cona, conb, _ := CreatePipedConn()
	_, err = dialer.Dial(100, "http://dav?dir=/tmp", conb)
	if err != nil {
		t.Error(err)
		return
	}
	cona.Close()
	//
	dialer.Shutdown()
	time.Sleep(100 * time.Millisecond)
	//for cover
	fmt.Printf("%v,%v\n", dialer.Addr(), dialer.Network())
	//test web conn
	conn, _, err := PipeWebDialerConn(100, "http://dav?dir=/tmp")
	if err != nil {
		t.Error(err)
		return
	}
	conn.SetDeadline(time.Now())
	conn.SetReadDeadline(time.Now())
	conn.SetWriteDeadline(time.Now())
	fmt.Printf("%v,%v,%v\n", conn.LocalAddr(), conn.RemoteAddr(), conn.Network())
	//test error
	//
	ts := httptest.NewHandlerFuncServer(func(hs *web.Session) web.Result {
		dialer.Handler.ServeHTTP(hs.W, hs.R)
		return web.Return
	})
	data, err := ts.GetText("/")
	if err == nil {
		t.Errorf("%v-%v", data, err)
		return
	}
	//
	_, err = dialer.Dial(100, "://", nil)
	if err == nil {
		t.Error(err)
		return
	}
	dialer.Name()
	dialer.Options()
}

func TestPipedConne(t *testing.T) {
	a, b, err := CreatePipedConn()
	if err != nil {
		t.Error(err)
		return
	}
	a.RemoteAddr()
	a.LocalAddr()
	a.Network()
	fmt.Printf("-->%v\n", a)
	b.Close()
}
