package dialer

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Centny/gwf/routing"
	"github.com/Centny/gwf/routing/httptest"
	"github.com/Centny/gwf/util"
)

func TestWebDialer(t *testing.T) {
	//test web dialer
	l, err := net.Listen("tcp", ":2422")
	if err != nil {
		t.Error(err)
		return
	}
	dialer := NewWebDialer()
	dialer.Bootstrap(nil)
	if dialer.Matched("tcp://web?dir=/tmp") {
		t.Error("error")
		return
	}
	if !dialer.Matched("http://web?dir=/tmp") {
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
			raw, err := dialer.Dial(cid, "http://web?dir=/tmp", nil)
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
	fmt.Println(util.HGet("http://localhost:2422/"))
	fmt.Println(util.HPost("http://localhost:2422/", nil))
	//
	//test pipe
	cona, conb, _ := CreatePipedConn()
	_, err = dialer.Dial(100, "http://web?dir=/tmp", conb)
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
	conn, _, err := PipeWebDialerConn(100, "http://web?dir=/tmp")
	if err != nil {
		t.Error(err)
		return
	}
	conn.SetDeadline(time.Now())
	conn.SetReadDeadline(time.Now())
	conn.SetWriteDeadline(time.Now())
	fmt.Printf("%v,%v,%v\n", conn.LocalAddr(), conn.RemoteAddr(), conn.Network())
	//test error
	_, _, err = PipeWebDialerConn(100, "://")
	if err == nil {
		t.Error(err)
		return
	}
	_, _, err = PipeWebDialerConn(100, "http://web")
	if err == nil {
		t.Error(err)
		return
	}
	//
	ts := httptest.NewServer(func(hs *routing.HTTPSession) routing.HResult {
		dialer.ServeHTTP(hs.W, hs.R)
		return routing.HRES_RETURN
	})
	data, err := ts.G("/")
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
