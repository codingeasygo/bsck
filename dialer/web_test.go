package dialer

import (
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xmap"
)

func TestWebDialer(t *testing.T) {
	//test web dialer
	l, err := net.Listen("tcp", ":2422")
	if err != nil {
		t.Error(err)
		return
	}
	dialer := NewWebDialer("dav", NewWebdavHandler(xmap.M{
		"t0": "/tmp",
	}))
	dialer.Bootstrap(nil)
	if dialer.Matched("tcp://dav?dir=t0") {
		t.Error("error")
		return
	}
	if !dialer.Matched("http://dav?dir=t0") {
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
			raw, err := dialer.Dial(nil, cid, "http://dav?dir=t0", nil)
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
	cona, conb := CreatePipedConn()
	_, err = dialer.Dial(nil, 100, "http://dav?dir=t0", conb)
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
	conn, _, err := PipeWebDialerConn(nil, 100, "http://dav?dir=t0")
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
	ts := httptest.NewServer(dialer.Handler)
	data, err := xhttp.GetText("%v", ts.URL)
	if err == nil {
		t.Errorf("%v-%v", data, err)
		return
	}
	//
	_, err = dialer.Dial(nil, 100, "://", nil)
	if err == nil {
		t.Error(err)
		return
	}
	dialer.Name()
	dialer.Options()
}

func TestPipedConne(t *testing.T) {
	a, b := CreatePipedConn()
	a.RemoteAddr()
	a.LocalAddr()
	fmt.Printf("-->%v\n", a)
	b.Close()
}
