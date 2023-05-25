package router

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
	"github.com/codingeasygo/util/xmap"
)

func runEchoServer(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}
		go io.Copy(conn, conn)
	}
}

func init() {
	ShowLog = 2
	SetLogLevel(LogLevelDebug)
	go runEchoServer("127.0.0.1:13200")
	go http.ListenAndServe(":6063", nil)
}

func TestRouterError(t *testing.T) {
	err := NewRouterError(ErrorChannelNotFound, "test")
	if err.IsRouterErrorType() != ErrorChannelNotFound {
		t.Error("error")
		return
	}
	fmt.Printf("err-->%v\n", err.Error())
	fmt.Printf("err-->%v\n", err.String())
}

func TestCmd(t *testing.T) {
	fmt.Printf("%v\n", CmdLoginChannel)
	fmt.Printf("%v\n", CmdLoginBack)
	fmt.Printf("%v\n", CmdPingConn)
	fmt.Printf("%v\n", CmdPingBack)
	fmt.Printf("%v\n", CmdDialConn)
	fmt.Printf("%v\n", CmdDialBack)
	fmt.Printf("%v\n", CmdConnData)
	fmt.Printf("%v\n", CmdConnClosed)
	fmt.Printf("%v\n", RouterCmd(0))
}

func TestRouterFrame(t *testing.T) {
	ParseLocalRouterFrame(frame.NewDefaultHeader(), make([]byte, 1024))
}

func TestConnType(t *testing.T) {
	fmt.Printf("%v\n", ConnTypeRaw)
	fmt.Printf("%v\n", ConnTypeChannel)
	fmt.Printf("%v\n", ConnType(0))
}

func TestConnID(t *testing.T) {
	connID := ZeroConnID
	fmt.Printf("%v\n", connID.LocalID())
	fmt.Printf("%v\n", connID.RemoteID())
	fmt.Printf("%v\n", connID)
	connID.Split()
}

func TestBondConn(t *testing.T) {
	conn1 := NewRouterConn(frame.NewRawReadWriteCloser(nil, nil, frame.DefaultBufferSize), 1, ConnTypeChannel)
	conn2 := NewRouterConn(frame.NewRawReadWriteCloser(nil, nil, frame.DefaultBufferSize), 2, ConnTypeChannel)
	bond := NewBondConn("Test", ConnTypeChannel)
	bond.AddConn(conn1)
	bond.AddConn(conn2)
	if bond.FindConn(1) != conn1 || bond.FindConn(2) != conn2 {
		t.Error("error")
		return
	}
	if bond.SelectConn() == nil {
		t.Error("error")
		return
	}
	fmt.Printf("%v\n", bond.Name())
	fmt.Printf("%v\n", bond.Type())
	fmt.Printf("%v\n", bond.String())
	fmt.Printf("%v\n", converter.JSON(bond.DislayConn()))
	bond.RemoveConn(1)
	bond.Close()
}

func TestRouterConn(t *testing.T) {
	raw := NewRouterConn(frame.NewRawReadWriteCloser(nil, nil, 102), 0, ConnTypeChannel)
	conn := NewRouterConn(raw, 1, ConnTypeChannel)
	conn.RawValue()
}

func TestRawDialer(t *testing.T) {
	dialer := DialRawF(func(channel Conn, sid uint16, uri string) (raw Conn, err error) {
		return nil, nil
	})
	dialer.DialRaw(nil, 0, "")
}

type ErrReadWriteCloser struct {
	Err     error
	ErrType int
	Data    []byte
}

func NewErrReadWriteCloser(data []byte, errType int) *ErrReadWriteCloser {
	return &ErrReadWriteCloser{
		Data:    data,
		ErrType: errType,
		Err:     fmt.Errorf("mock error"),
	}
}

func (e *ErrReadWriteCloser) Write(p []byte) (n int, err error) {
	if e.ErrType == 10 {
		err = e.Err
	}
	n = len(p)
	fmt.Println("RECV:", string(p))
	return
}

func (e *ErrReadWriteCloser) Read(b []byte) (n int, err error) {
	if e.ErrType == 20 {
		err = e.Err
	}
	n = len(e.Data)
	copy(b, e.Data)
	return
}

func (e *ErrReadWriteCloser) Close() (err error) {
	if e.ErrType == 30 {
		err = e.Err
	}
	return
}

func (e *ErrReadWriteCloser) Wait() error {
	return nil
}

func (e *ErrReadWriteCloser) Ready(failed error, next func(err error)) {
	// if next != nil {
	next(e.Err)
	// }
}

func newBaseNode() (node0, node1 *Router, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	node0 = NewRouter("N0", access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	node1 = NewRouter("N1", access1)
	channelA10, channelB10, _ := xio.CreatePipedConn()
	node0.Accept(channelA10, false)
	_, _, err = node1.JoinConn(channelB10, xmap.M{
		"name":  "N1",
		"token": "123",
	})
	return
}

func newLinkNode() (nodeList []*Router, nameList []string, err error) {
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("N%02d", i)
		access := NewNormalAcessHandler(name)
		access.LoginAccess[fmt.Sprintf("N%02d", i+1)] = "123"
		access.DialAccess = append(access.DialAccess, []string{".*", ".*"})
		node := NewRouter(name, access)
		nodeList = append(nodeList, node)
		nameList = append(nameList, name)
		if len(nodeList) < 2 {
			continue
		}
		channelA, channelB, _ := xio.CreatePipedConn()
		if i%2 == 0 {
			go nodeList[i-1].Accept(channelA, true)
		} else {
			nodeList[i-1].Accept(channelA, false)
		}
		_, _, err = node.JoinConn(channelB, xmap.M{
			"name":  name,
			"token": "123",
		})
		if err != nil {
			break
		}
	}
	return
}

func newMultiNode() (nodeList []*Router, nameList []string, err error) {
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("N%02d", i)
		access := NewNormalAcessHandler(name)
		access.DialAccess = append(access.DialAccess, []string{".*", ".*"})
		node := NewRouter(name, access)
		nodeList = append(nodeList, node)
		nameList = append(nameList, name)
		if len(nodeList) < 2 {
			continue
		}
		node0 := nodeList[0]
		node0.Handler.(*NormalAcessHandler).LoginAccess[fmt.Sprintf("N%02d", i)] = "123"
		channelA, channelB, _ := xio.CreatePipedConn()
		node0.Accept(channelA, false)
		_, _, err = node.JoinConn(channelB, xmap.M{
			"name":  name,
			"token": "123",
		})
		if err != nil {
			break
		}
	}
	return
}

func TestRouter(t *testing.T) {
	tester := xdebug.CaseTester{
		0:  1,
		10: 1,
	}
	if tester.Run() { //base dial
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		for i := 0; i < 5; i++ { //node1->node0
			connA, connB, _ := xio.CreatePipedConn()
			_, _, err := node1.DialConn(connB, "N0->tcp://127.0.0.1:13200")
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(connA, "abc")
			buf := make([]byte, 1024)
			n, err := connA.Read(buf)
			if err != nil || string(buf[0:n]) != "abc" {
				t.Errorf("%v,%v,%v", err, n, buf[0:n])
				return
			}
			if i%2 == 0 { //for some conn is not closed
				connA.Close()
			}
		}
		for i := 0; i < 5; i++ { //node0->node1
			connA, connB, _ := xio.CreatePipedConn()
			_, _, err := node0.DialConn(connB, "N1->tcp://127.0.0.1:13200")
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(connA, "abc")
			buf := make([]byte, 1024)
			n, err := connA.Read(buf)
			if err != nil || string(buf[0:n]) != "abc" {
				t.Errorf("%v,%v,%v", err, n, buf[0:n])
				return
			}
			if i%2 == 0 { //for some conn is not closed
				connA.Close()
			}
		}
		node1.Stop()
		node0.Stop()
	}
	if tester.Run() { //parallel dial
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		waiter := sync.WaitGroup{}
		for i := 0; i < 5; i++ { //node1->node0
			waiter.Add(1)
			go func(x int) {
				defer waiter.Done()
				connA, connB, _ := xio.CreatePipedConn()
				_, _, err := node1.DialConn(connB, "N0->tcp://127.0.0.1:13200")
				if err != nil {
					t.Error(err)
					return
				}
				fmt.Fprintf(connA, "abc")
				buf := make([]byte, 1024)
				n, err := connA.Read(buf)
				if err != nil || string(buf[0:n]) != "abc" {
					t.Errorf("%v,%v,%v", err, n, buf[0:n])
					return
				}
				if x%2 == 0 { //for some conn is not closed
					connA.Close()
				}
			}(i)
		}
		for i := 0; i < 5; i++ { //node0->node1
			waiter.Add(1)
			go func(x int) {
				defer waiter.Done()
				connA, connB, _ := xio.CreatePipedConn()
				_, _, err := node0.DialConn(connB, "N1->tcp://127.0.0.1:13200")
				if err != nil {
					t.Error(err)
					return
				}
				fmt.Fprintf(connA, "abc")
				buf := make([]byte, 1024)
				n, err := connA.Read(buf)
				if err != nil || string(buf[0:n]) != "abc" {
					t.Errorf("%v,%v,%v", err, n, buf[0:n])
					return
				}
				if x%2 == 0 { //for some conn is not closed
					connA.Close()
				}
			}(i)
		}
		waiter.Wait()
		node1.Stop()
		node0.Stop()
	}
	if tester.Run() { //parallel dial closed
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		waiter := sync.WaitGroup{}
		for i := 0; i < 5; i++ { //node1->node0
			waiter.Add(1)
			go func(x int) {
				defer waiter.Done()
				connA, connB, _ := xio.CreatePipedConn()
				_, _, err := node1.DialConn(connB, "N0->tcp://127.0.0.1:13200")
				if err != nil {
					t.Error(err)
					return
				}
				fmt.Fprintf(connA, "abc")
				buf := make([]byte, 1024)
				n, err := connA.Read(buf)
				if err != nil || string(buf[0:n]) != "abc" {
					t.Errorf("%v,%v,%v", err, n, buf[0:n])
					return
				}
				connA.Close()
			}(i)
		}
		for i := 0; i < 5; i++ { //node0->node1
			waiter.Add(1)
			go func(x int) {
				defer waiter.Done()
				connA, connB, _ := xio.CreatePipedConn()
				_, _, err := node0.DialConn(connB, "N1->tcp://127.0.0.1:13200")
				if err != nil {
					t.Error(err)
					return
				}
				fmt.Fprintf(connA, "abc")
				buf := make([]byte, 1024)
				n, err := connA.Read(buf)
				if err != nil || string(buf[0:n]) != "abc" {
					t.Errorf("%v,%v,%v", err, n, buf[0:n])
					return
				}
				connA.Close()
			}(i)
		}
		waiter.Wait()
		time.Sleep(50 * time.Millisecond)
		channel0, err := node1.SelectChannel("N0")
		if err != nil || channel0.UsedConnID() > 0 {
			t.Errorf("%v,%v", err, channel0.UsedConnID())
			return
		}
		channel1, err := node0.SelectChannel("N1")
		if err != nil || channel1.UsedConnID() > 0 {
			t.Errorf("%v,%v", err, channel1.UsedConnID())
			return
		}
		node1.Stop()
		node0.Stop()
	}
	if tester.Run() { //channel close
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		connA, connB, _ := xio.CreatePipedConn()
		_, _, err = node1.DialConn(connB, "N0->tcp://127.0.0.1:13200")
		if err != nil {
			t.Error(err)
			return
		}
		go func() {
			node0.CloseChannel("N1")
		}()
		buf := make([]byte, 1024)
		_, err = connA.Read(buf)
		if err == nil {
			t.Errorf("%v", err)
			return
		}
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //LoginAccess fail
		access0 := NewNormalAcessHandler("N0")
		access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
		node0 := NewRouter("N0", access0)

		access1 := NewNormalAcessHandler("N1")
		access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
		node1 := NewRouter("N1", access1)
		channelA10, channelB10, _ := xio.CreatePipedConn()
		node0.Accept(channelA10, false)
		_, _, err := node1.JoinConn(channelB10, xmap.M{
			"name":  "N1",
			"token": "123",
		})
		if err == nil {
			t.Error(err)
			return
		}
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //DialAccess fail
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}

		node0.Handler.(*NormalAcessHandler).DialAccess = [][]string{}
		_, _, err = node1.SyncDial(xio.NewDiscardReadWriteCloser(), "N0->tcp://127.0.0.1:13200")
		if err == nil {
			t.Error(err)
			return
		}

		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //dial fail
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}

		connA, connB, _ := xio.CreatePipedConn()
		_, _, err = node1.Dial(connB, "N0->tcp://127.0.0.1:10")
		if err != nil {
			t.Error(err)
			return
		}
		buf := make([]byte, 1024)
		_, err = connA.Read(buf)
		if err == nil {
			t.Errorf("%v", err)
			return
		}

		_, _, err = node1.SyncDial(xio.NewDiscardReadWriteCloser(), "N0->tcp://127.0.0.1:10")
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = node1.SyncDial(xio.NewDiscardReadWriteCloser(), "N0->N1->tcp://127.0.0.1:10")
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = node1.SyncDial(xio.NewDiscardReadWriteCloser(), "N0->X0->tcp://127.0.0.1:10")
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = node1.SyncDial(xio.NewDiscardReadWriteCloser(), "NX->tcp://127.0.0.1:10")
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = node1.SyncDial(xio.NewDiscardReadWriteCloser(), "N0")
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = node1.SyncDial(NewErrReadWriteCloser([]byte("123"), 40), "N0->tcp://127.0.0.1:13200")
		if err != nil {
			t.Error(err)
			return
		}

		time.Sleep(50 * time.Millisecond)
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //link dial
		nodeList, nameListA, err := newLinkNode()
		if err != nil {
			t.Error(err)
			return
		}
		nameListB := []string{}
		for i := len(nameListA) - 1; i >= 0; i-- {
			nameListB = append(nameListB, nameListA[i])
		}

		nodeA := nodeList[0]
		connURIA := fmt.Sprintf("%v->tcp://127.0.0.1:13200", strings.Join(nameListA[1:], "->"))
		for i := 0; i < 10; i++ {
			connA, connB, _ := xio.CreatePipedConn()
			_, _, err := nodeA.DialConn(connB, connURIA)
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(connA, "abc")
			buf := make([]byte, 1024)
			n, err := connA.Read(buf)
			if err != nil || string(buf[0:n]) != "abc" {
				t.Errorf("%v,%v,%v", err, n, buf[0:n])
				return
			}
			if i%2 == 0 { //for some conn is not closed
				connA.Close()
			}
		}
		nodeB := nodeList[len(nodeList)-1]
		connURIB := fmt.Sprintf("%v->tcp://127.0.0.1:13200", strings.Join(nameListB[1:], "->"))
		for i := 0; i < 10; i++ {
			connA, connB, _ := xio.CreatePipedConn()
			_, _, err := nodeB.DialConn(connB, connURIB)
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(connA, "abc")
			buf := make([]byte, 1024)
			n, err := connA.Read(buf)
			if err != nil || string(buf[0:n]) != "abc" {
				t.Errorf("%v,%v,%v", err, n, buf[0:n])
				return
			}
			if i%2 == 0 { //for some conn is not closed
				connA.Close()
			}
		}
		for _, node := range nodeList {
			node.Stop()
		}
	}
	if tester.Run() { //multi dial
		nodeList, nameList, err := newMultiNode()
		if err != nil {
			t.Error(err)
			return
		}
		for i, name := range nameList[1:] { //node0->*
			connURI := fmt.Sprintf("%v->tcp://127.0.0.1:13200", name)
			connA, connB, _ := xio.CreatePipedConn()
			_, _, err := nodeList[0].DialConn(connB, connURI)
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(connA, "abc")
			buf := make([]byte, 1024)
			n, err := connA.Read(buf)
			if err != nil || string(buf[0:n]) != "abc" {
				t.Errorf("%v,%v,%v", err, n, buf[0:n])
				return
			}
			if i%2 == 0 { //for some conn is not closed
				connA.Close()
			}
		}
		for i := range nameList[1:] { //*->node0
			connURI := fmt.Sprintf("%v->tcp://127.0.0.1:13200", "N00")
			connA, connB, _ := xio.CreatePipedConn()
			_, _, err := nodeList[i+1].DialConn(connB, connURI)
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(connA, "abc")
			buf := make([]byte, 1024)
			n, err := connA.Read(buf)
			if err != nil || string(buf[0:n]) != "abc" {
				t.Errorf("%v,%v,%v", err, n, buf[0:n])
				return
			}
			if i%2 == 0 { //for some conn is not closed
				connA.Close()
			}
		}
		for _, node := range nodeList[1:] {
			node.Stop()
		}
		nodeList[0].Stop()
	}
	if tester.Run() { //loc dial
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		connA, connB, _ := xio.CreatePipedConn()
		_, _, err = node1.DialConn(connB, "tcp://127.0.0.1:13200")
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Fprintf(connA, "abc")
		buf := make([]byte, 1024)
		n, err := connA.Read(buf)
		if err != nil || string(buf[0:n]) != "abc" {
			t.Errorf("%v,%v,%v", err, n, buf[0:n])
			return
		}
		connA.Close()
		node1.Stop()
		node0.Stop()
	}
	if tester.Run() { //piper
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}

		piper, err := node1.DialPiper("N0->tcp://127.0.0.1:13200", node1.BufferSize)
		if err != nil {
			t.Error(err)
			return
		}
		connA, connB, _ := xio.CreatePipedConn()
		go func() {
			xerr := piper.PipeConn(connB, "")
			fmt.Println("--->", xerr)
		}()
		fmt.Fprintf(connA, "abc")
		buf := make([]byte, 1024)
		n, err := connA.Read(buf)
		if err != nil || string(buf[0:n]) != "abc" {
			t.Errorf("%v,%v,%v", err, n, buf[0:n])
			return
		}
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //ping
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		node0.Heartbeat = time.Millisecond
		node0.Start()
		node1.Start()
		node0.procPingAll()
		time.Sleep(2 * time.Millisecond)
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //timeout
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		node0.Timeout = time.Millisecond
		node0.Start()
		node1.Start()
		node0.procTimeoutAll()
		time.Sleep(2 * time.Millisecond)
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //state
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		connA, connB, _ := xio.CreatePipedConn()
		_, _, err = node1.DialConn(connB, "N0->tcp://127.0.0.1:13200")
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Fprintf(connA, "abc")
		buf := make([]byte, 1024)
		n, err := connA.Read(buf)
		if err != nil || string(buf[0:n]) != "abc" {
			t.Errorf("%v,%v,%v", err, n, buf[0:n])
			return
		}
		mux := http.NewServeMux()
		ts := httptest.NewServer(mux)
		mux.HandleFunc("/node0", node0.StateH)
		mux.HandleFunc("/node1", node1.StateH)
		xhttp.GetMap("%v/node0?a=b", ts.URL)
		xhttp.GetMap("%v/node1?a=b", ts.URL)
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //as
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		node0.NewConn(xio.NewDiscardReadWriteCloser(), 0, ConnTypeChannel)
		node0.NewConn(frame.NewReadWriteCloser(nil, nil, 1024), 0, ConnTypeChannel)
		node0.NewConn(NewRouterConn(nil, 1, ConnTypeChannel), 0, ConnTypeChannel)
		func() {
			defer func() {
				if rerr := recover(); rerr == nil {
					t.Error("error")
				}
			}()
			node0.NewConn("123", 0, ConnTypeChannel)
		}()
		node0.Stop()
		node1.Stop()
	}
	if tester.Run() { //error
		node0, node1, err := newBaseNode()
		if err != nil {
			t.Error(err)
			return
		}
		_, err = node0.SelectChannel("XX")
		if err == nil {
			t.Error(err)
			return
		}
		func() {
			defer func() {
				if perr := recover(); perr == nil {
					t.Error("error")
				}
			}()
			node0.procConnRead(nil)
		}()
		func() {
			defer func() {
				if perr := recover(); perr == nil {
					t.Error("error")
				}
			}()
			node0.addChannel(NewRouterConn(nil, 0, ConnTypeRaw))
		}()
		func() {
			defer func() {
				if perr := recover(); perr == nil {
					t.Error("error")
				}
			}()
			var node2 *Router
			node2.procPingAll()
		}()
		func() {
			defer func() {
				if perr := recover(); perr == nil {
					t.Error("error")
				}
			}()
			var node2 *Router
			node2.procTimeoutAll()
		}()
		{
			piper := NewRouterPiper()
			piper.Close()
			piper.PipeConn(xio.NewDiscardReadWriteCloser(), "")
		}

		node0.Stop()
		node1.Stop()
		node1.Stop()
	}
	if tester.Run() { //mock error
		access0 := NewNormalAcessHandler("N0")
		access0.LoginAccess["N1"] = "123"
		access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
		node0 := NewRouter("N0", access0)
		discard := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, xio.NewDiscardReadWriteCloser(), 1024), 0, ConnTypeRaw)
		discard.SetName("DISCARD")
		{ //cmd error
			node0.Accept(frame.NewRawReadWriter(node0.Header, bytes.NewBufferString("abc"), node0.BufferSize), true)
		}
		{ //dial conn error
			node0.procDialConn(discard, &RouterFrame{Data: []byte("")}) //uri error

			conn := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, NewErrReadWriteCloser([]byte("abc"), 10), node0.BufferSize), 1, ConnTypeRaw)
			conn.SetName("ERR")
			node0.addChannel(conn)
			err := node0.procDialConn(discard, &RouterFrame{Data: []byte("XX|ERR->tcp://127.0.0.1:80")}) //net writer error
			if err != nil {
				t.Error(err)
				return
			}
		}
		{ //dial raw error
			node0.procDialRaw(discard, ZeroConnID, "", "") //id error
		}
		{ //dial back error
			node0.procDialBack(discard, &RouterFrame{Data: []byte("")}) //router error

			conn := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, NewErrReadWriteCloser([]byte("abc"), 10), node0.BufferSize), 1, ConnTypeChannel)
			conn.SetName("ERR")
			node0.addTable(discard, ConnID{1, 1}, conn, ConnID{1, 1}, "test")
			node0.procDialBack(discard, NewRouterFrameByMessage(node0.Header, nil, ConnID{1, 1}, CmdDialBack, []byte("abc"))) //router error //forward error
		}
		{ //conn data error
			node0.procConnData(discard, &RouterFrame{SID: ConnID{100, 100}, Data: []byte("")}) //router error

			conn := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, NewErrReadWriteCloser([]byte("abc"), 10), node0.BufferSize), 1, ConnTypeChannel)
			conn.SetName("ERR")
			node0.addTable(discard, ConnID{1, 1}, conn, ConnID{1, 1}, "test")
			node0.procConnData(discard, NewRouterFrameByMessage(node0.Header, nil, ConnID{1, 1}, CmdDialBack, []byte("abc"))) //router error //forward error
		}
		{ //join conn error
			writeErr := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, NewErrReadWriteCloser([]byte("abc"), 10), node0.BufferSize), 1, ConnTypeChannel)
			_, _, err := node0.JoinConn(writeErr, xmap.M{})
			if err == nil {
				t.Error("error")
				return
			}
			readErr := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, NewErrReadWriteCloser([]byte("abc"), 20), node0.BufferSize), 1, ConnTypeChannel)
			_, _, err = node0.JoinConn(readErr, xmap.M{})
			if err == nil {
				t.Error("error")
				return
			}
		}
		{ //dial conn
			conn := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, NewErrReadWriteCloser([]byte("abc"), 10), node0.BufferSize), 1, ConnTypeRaw)
			conn.SetName("ERR")
			node0.addChannel(conn)

			_, _, err := node0.Dial(discard, "ERR->tcp://127.0.0.1:10")
			if err == nil {
				t.Error(err)
				return
			}
		}
	}
}

func TestNormalAcessHandlerErr(t *testing.T) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["x["] = "123"
	access0.LoginAccess["a"] = "x[]"
	access0.DialAccess = [][]string{
		{"x[", ".*"},
		{".*", "x["},
		{".*"},
		{"a", "b"},
	}
	node0 := NewRouter("N0", access0)
	discard := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, xio.NewDiscardReadWriteCloser(), 1024), 0, ConnTypeRaw)
	discard.SetName("TEST")
	{ //dial raw
		errDial := NewNormalAcessHandler("N0")
		errDial.Dialer = DialRawF(func(channel Conn, sid uint16, uri string) (raw Conn, err error) {
			return
		})
		errDial.DialRaw(discard, 0, "")

		errDial.Dialer = nil
		errDial.NetDialer = nil
		errDial.DialRaw(discard, 0, "")
	}
	{ //conn login error
		_, _, err := access0.OnConnLogin(discard, "xxx")
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = access0.OnConnLogin(discard, "{}")
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = access0.OnConnLogin(discard, `{"name":"abc","token":"123"}`)
		if err == nil {
			t.Error(err)
			return
		}
	}
	{ //dail url error
		notLogin := NewRouterConn(frame.NewRawReadWriteCloser(node0.Header, xio.NewDiscardReadWriteCloser(), 1024), 0, ConnTypeRaw)
		err := access0.OnConnDialURI(notLogin, "A|B->tcp://", []string{"B", "tcp://"})
		if err == nil {
			t.Error(err)
			return
		}
		err = access0.OnConnDialURI(discard, "A|B->tcp://", []string{"B", "tcp://"})
		if err == nil {
			t.Error(err)
			return
		}
	}
	{
		access0.OnConnJoin(discard, xmap.M{}, xmap.M{})
	}
}
