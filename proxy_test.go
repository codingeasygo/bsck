package bsck

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ShowLog = 2
}

type Echo struct {
	Data string
	Recv int
	Send int
	Err  error
	W    chan int
	R    chan int
}

func NewEcho(data string) *Echo {
	return &Echo{
		Data: data,
		W:    make(chan int),
		R:    make(chan int, 100),
	}
}

func (e *Echo) Write(p []byte) (n int, err error) {
	if e.Err != nil {
		err = e.Err
		return
	}
	n = len(p)
	fmt.Printf("%v RECV:%v\n", e.Data, string(p))
	e.Recv++
	e.R <- 1
	return
}

func (e *Echo) Read(b []byte) (n int, err error) {
	fmt.Println("Echo.Read-->started", e.Data)
	if e.Err != nil {
		err = e.Err
		return
	}
	<-e.W
	copy(b, []byte(e.Data))
	n = len(e.Data)
	e.Send++
	fmt.Println("Echo.Read-->done", e.Data)
	return
}

func (e *Echo) Close() error {
	if e.Err == nil {
		e.Err = fmt.Errorf("closed")
		fmt.Printf("%v echo is closed\n", e.Data)
		close(e.W)
		e.R <- 1
	}
	return nil
}

func (e *Echo) String() string {
	return fmt.Sprintf("Echo(%v)", e.Data)
}

func TestProxy(t *testing.T) {
	var masterEcho *Echo
	handler := NewNormalAcessHandler("master", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		conn = NewRawConn(masterEcho, 1024, sid, uri)
		return
	}))
	handler.LoginAccess["slaver1"] = "abc"
	handler.LoginAccess["slaver2"] = "abc"
	handler.LoginAccess["slaver3"] = "abc"
	handler.LoginAccess["ms"] = "abc"
	handler.DialAccess = [][]string{{".*", ".*"}}
	master := NewProxy("master", handler)
	master.Heartbeat = 1000 * time.Millisecond
	master.StartHeartbeat()
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	defer master.Close()
	var slaver1Echo *Echo
	slaver1Handler := NewNormalAcessHandler("master", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaver1Echo, 1024, sid, uri)
		return
	}))
	slaver1Handler.DialAccess = [][]string{{".*", ".*"}}
	slaver1 := NewProxy("slaver1", slaver1Handler)
	// slaver1.Heartbeat = 50 * time.Millisecond
	// slaver1.StartHeartbeat()
	_, err = slaver1.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer slaver1.Close()
	slaver2 := NewProxy("slaver2", nil)
	_, err = slaver2.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer slaver2.Close()
	{ //test slaver2->master->server
		begin := time.Now()
		fmt.Printf("\n\n\ntest slaver2->master->server\n")
		masterEcho = NewEcho("master")
		slaver2Echo := NewEcho("client")
		_, err = slaver2.Dial("master->xx", slaver2Echo)
		if err != nil {
			t.Error(err)
			return
		}
		//client->server
		slaver2Echo.W <- 1
		<-masterEcho.R
		slaver2Echo.W <- 1
		<-masterEcho.R
		//server->client
		masterEcho.W <- 1
		<-slaver2Echo.R
		masterEcho.W <- 1
		<-slaver2Echo.R
		//close
		slaver2Echo.Close()
		<-masterEcho.R
		fmt.Printf("test slaver2->master->server use %v\n", time.Now().Sub(begin))
	}
	{ //test slaver2->master->slaver->server
		begin := time.Now()
		fmt.Printf("\n\n\ntest slaver2->master->slaver1->server\n")
		slaver1Echo = NewEcho("slaver1")
		slaver2Echo := NewEcho("client")
		_, err = slaver2.Dial("master->slaver1->xx", slaver2Echo)
		if err != nil {
			t.Error(err)
			return
		}
		//client->server
		slaver2Echo.W <- 1
		<-slaver1Echo.R
		slaver2Echo.W <- 1
		<-slaver1Echo.R
		// server->client
		slaver1Echo.W <- 1
		<-slaver2Echo.R
		slaver1Echo.W <- 1
		<-slaver2Echo.R
		//close
		slaver2Echo.Close()
		<-slaver1Echo.R
		fmt.Printf("test slaver2->master->slaver1->server use %v\n", time.Now().Sub(begin))
	}
	{ //multi channel
		begin := time.Now()
		fmt.Printf("\n\n\ntest multi channel\n")
		var msEcho *Echo
		ms0Handler := NewNormalAcessHandler("ms", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
			if uri == "error" {
				err = fmt.Errorf("test error")
				return
			}
			fmt.Println("ms test dail to ", uri)
			conn = NewRawConn(msEcho, 1024, sid, uri)
			return
		}))
		ms0Handler.DialAccess = [][]string{{".*", ".*"}}
		ms0 := NewProxy("ms", ms0Handler)
		err = ms0.LoginChannel(false, &ChannelOption{
			Enable: true,
			Token:  "abc",
			Local:  "0.0.0.0:0",
			Remote: "localhost:9232",
			Index:  0,
		}, &ChannelOption{
			Enable: true,
			Token:  "abc",
			Local:  "0.0.0.0:0",
			Remote: "localhost:9232",
			Index:  1,
		})
		if err != nil {
			t.Error(err)
			return
		}
		for i := 0; i < 10; i++ {
			msEcho = NewEcho("ms")
			slaver2Echo := NewEcho("client")
			_, err = slaver2.Dial("master->ms->xx", slaver2Echo)
			if err != nil {
				t.Error(err)
				return
			}
			//client->server
			slaver2Echo.W <- 1
			<-msEcho.R
			slaver2Echo.W <- 1
			<-msEcho.R
			// server->client
			msEcho.W <- 1
			<-slaver2Echo.R
			msEcho.W <- 1
			<-slaver2Echo.R
			//close
			slaver2Echo.Close()
			<-msEcho.R
		}
		ms0.Close()
		fmt.Printf("test multi channel use %v\n", time.Now().Sub(begin))
	}
	{ //channel close
		begin := time.Now()
		fmt.Printf("\n\n\ntest channel close\n")
		slaver3Echo := NewEcho("slaver3")
		slaver3Handler := NewNormalAcessHandler("slaver3", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
			fmt.Println("slaver3 test dail to ", uri)
			conn = NewRawConn(slaver3Echo, 1024, sid, uri)
			return
		}))
		slaver3Handler.DialAccess = [][]string{{".*", ".*"}}
		slaver3 := NewProxy("slaver3", slaver3Handler)
		slaver3.Login(&ChannelOption{
			Enable: true,
			Remote: "localhost:9232",
			Token:  "abc",
			Index:  0,
		})
		slaver2Echo := NewEcho("client")
		_, err = slaver2.Dial("master->slaver3->xx", slaver2Echo)
		if err != nil {
			t.Error(err)
			return
		}
		//client->server
		slaver2Echo.W <- 1
		<-slaver3Echo.R
		slaver2Echo.W <- 1
		<-slaver3Echo.R
		// server->client
		slaver3Echo.W <- 1
		<-slaver2Echo.R
		slaver3Echo.W <- 1
		<-slaver2Echo.R
		//close
		c, _ := slaver3.SelectChannel("master")
		c.Close()
		<-slaver3Echo.R
		<-slaver2Echo.R
		slaver3.Close()
		fmt.Printf("test channel close use %v\n", time.Now().Sub(begin))
	}
	{ //dial remote fail
		begin := time.Now()
		fmt.Printf("\n\n\ntest dial remote fail\n")
		slaver2Echo := NewEcho("client")
		_, err = slaver2.Dial("master->error", slaver2Echo)
		if err != nil {
			t.Error(err)
			return
		}
		<-slaver2Echo.R
		if slaver2Echo.Recv != 0 {
			t.Error("error")
			return
		}
		fmt.Printf("test dial remote fail use %v\n", time.Now().Sub(begin))
	}
	{ //login channel fail
		time.Sleep(3 * time.Second)
		begin := time.Now()
		fmt.Printf("\n\n\ntest login channel fail\n")
		slaver4 := NewProxy("slaver4", nil)
		err := slaver4.LoginChannel(false, &ChannelOption{
			Enable: false,
			Token:  "abc",
			Local:  "0.0.0.0:0",
			Remote: "localhost:9232",
			Index:  0,
		}, &ChannelOption{
			Enable: true,
			Token:  "abc",
			Local:  "0.0.0.0:0",
			Remote: "localhost:0",
			Index:  0,
		})
		if err == nil {
			t.Error(err)
			return
		}
		time.Sleep(time.Millisecond)
		slaver4.Close()
		fmt.Printf("test login channel fail use %v\n", time.Now().Sub(begin))
	}
	fmt.Printf("\n\n\nall test done\n")
}

func TestProxyError(t *testing.T) {
	var masterEcho *Echo
	handler := NewNormalAcessHandler("master", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, 1024, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	}))
	handler.LoginAccess["ms"] = "abc"
	handler.DialAccess = [][]string{{".*", ".*"}}
	master := NewProxy("master", handler)
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		master.Close()
		time.Sleep(time.Second)
	}()
	//
	// slaver := NewRouter("slaver")
	// var slaverEcho *Echo
	// slaver.DialRaw = func(sid uint64, uri string) (conn Conn, err error) {
	// 	fmt.Println("slaver test dail to ", uri)
	// 	conn = NewRawConn(slaverEcho, sid, uri, 4096)
	// 	// err = fmt.Errorf("error")
	// 	return
	// }
	// slaver.Login("", "localhost:9232", "abc", 0)
	//
	{ //test login error
		fmt.Printf("\n\n\ntest login error\n")
		slaverErr := NewProxy("error", nil)
		_, err = slaverErr.Login(&ChannelOption{
			Enable: true,
			Remote: "localhost:9232",
			Token:  "abc",
			Index:  0,
		})
		if err == nil {
			t.Error("error")
			return
		}
		slaverErr = NewProxy("error", nil)
		_, err = slaverErr.Login(&ChannelOption{
			Enable: true,
			Remote: "localhost:9232",
			Token:  "",
			Index:  0,
		})
		if err == nil {
			t.Error("error")
			return
		}
	}
	{ //login/dial error
		fmt.Printf("\n\n\ntest login/dial error\n")
		//configer error
		masterEcho = NewEcho("master")
		testc := &Channel{
			ReadWriteCloser: frame.NewReadWriteCloser(NewEcho("testing"), 1024),
			name:            "xx",
		}
		testc.Close()
		master.Router.addChannel(testc)
		//
		//test proc login fail
		err = master.Router.procLogin(NewRawConn(NewEcho("data"), 1024, 0, "ur"), make([]byte, 1024))
		if err == nil {
			t.Error("error")
			return
		}
		//
		echo := NewEcho("data")
		err = master.Router.procLogin(&Channel{ReadWriteCloser: frame.NewReadWriteCloser(echo, 1024)}, make([]byte, 1024))
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		//test proc dail fail
		buf := make([]byte, 1024)
		data := []byte{}
		//
		data = []byte("url")
		copy(buf[13:], data)
		echo = NewEcho("data")
		err = master.Router.procDial(&Channel{ReadWriteCloser: frame.NewReadWriteCloser(echo, 16)}, buf)
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		data = []byte("x@error")
		copy(buf[13:], data)
		echo = NewEcho("data")
		err = master.Router.procDial(&Channel{ReadWriteCloser: frame.NewReadWriteCloser(echo, len(data)+13)}, buf)
		time.Sleep(time.Second) //wait for go
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		data = []byte("x@not->error")
		copy(buf[13:], data)
		echo = NewEcho("data")
		err = master.Router.procDial(&Channel{ReadWriteCloser: frame.NewReadWriteCloser(echo, len(data)+13)}, buf)
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		data = []byte("x@xx->error")
		copy(buf[13:], data)
		echo = NewEcho("data")
		err = master.Router.procDial(&Channel{ReadWriteCloser: frame.NewReadWriteCloser(echo, len(data)+13)}, buf)
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		//test dial error
		_, err = master.Router.Dial("uri", NewEcho("testing"))
		if err == nil {
			t.Error(err)
			return
		}
		_, err = master.Router.Dial("not->abc", NewEcho("testing"))
		if err == nil {
			t.Error(err)
			return
		}
		_, err = master.Router.Dial("xx->abc", NewEcho("testing"))
		if err == nil {
			t.Error(err)
			return
		}
		//
		//test login error
		slaver := NewProxy("slaver", nil)
		err = slaver.LoginChannel(false, &ChannelOption{
			Enable: true,
			Token:  "abc",
			Remote: "loclahost:12",
		})
		if err == nil {
			t.Error(err)
			return
		}
		_, err = slaver.Login(&ChannelOption{
			Enable: true,
			Local:  "xxx",
			Remote: "localhost:9232",
			Token:  "abc",
			Index:  0,
		})
		if err == nil {
			t.Error(err)
			return
		}
		_, err = slaver.Login(&ChannelOption{
			Enable: true,
			Remote: "localhost:12",
			Token:  "abc",
			Index:  0,
		})
		if err == nil {
			t.Error(err)
			return
		}
		merr := NewErrReadWriteCloser([]byte("abc"), 10)
		merr.ErrType = 10
		_, err = slaver.JoinConn(frame.NewReadWriteCloser(merr, 1024), 0, &ChannelOption{
			Enable: true,
			Remote: "",
			Token:  "abc",
			Index:  0,
		})
		if err == nil {
			t.Error(err)
			return
		}
		merr.ErrType = 20
		_, err = slaver.JoinConn(frame.NewReadWriteCloser(merr, 1024), 0, &ChannelOption{
			Enable: true,
			Remote: "",
			Token:  "abc",
			Index:  0,
		})
		if err == nil {
			t.Error(err)
			return
		}
	}
	{ //test proc dial back error
		fmt.Printf("\n\n\ntest proc dail back error\n")
		//
		//test dial fail
		srcRaw := NewErrReadWriteCloser([]byte("error"), 10)
		src := &Channel{ReadWriteCloser: frame.NewReadWriteCloser(srcRaw, 1024)}
		dstRaw := NewErrReadWriteCloser([]byte("error"), 10)
		dst := &Channel{ReadWriteCloser: frame.NewReadWriteCloser(dstRaw, 1024)}
		master.Router.addTable(src, 1000, dst, 1001)
		buf := make([]byte, 1024)
		copy(buf[13:], []byte("error"))
		//
		binary.BigEndian.PutUint64(buf[5:], 1000)
		err = master.Router.procDialBack(src, buf)
		if err != srcRaw.Err {
			t.Error(err)
			return
		}
		//
		binary.BigEndian.PutUint64(buf[5:], 2000)
		err = master.Router.procDialBack(src, buf)
		if err != srcRaw.Err {
			t.Error(err)
			return
		}
	}
	{ //test loop read raw fail
		buf := make([]byte, 1024)
		//length error
		binary.BigEndian.PutUint32(buf, 6)
		srcRaw := NewErrReadWriteCloser(buf[0:10], 0)
		src := &Channel{ReadWriteCloser: frame.NewReadWriteCloser(srcRaw, 1024)}
		master.Router.loopReadRaw(src)
		//read cmd error
		binary.BigEndian.PutUint32(buf, 100)
		srcRaw = NewErrReadWriteCloser(buf[0:104], 0)
		src = &Channel{ReadWriteCloser: frame.NewReadWriteCloser(srcRaw, 50)}
		master.Router.loopReadRaw(src)
		//cmd error
		binary.BigEndian.PutUint32(buf, 100)
		srcRaw = NewErrReadWriteCloser(buf[0:104], 0)
		src = &Channel{ReadWriteCloser: frame.NewReadWriteCloser(srcRaw, 1024)}
		master.Router.loopReadRaw(src)
	}
	{ //test for cover
		rawConn := NewRawConn(NewEcho("data"), 1024, 0, "")
		rawConn.Index()
		func() {
			defer func() {
				recover()
			}()
			rawConn.Read(nil)
		}()
		cmdString(CmdLoginBack)
		echo := NewErrReadWriteCloser([]byte("data"), 0)
		xio.FullBuffer(echo, make([]byte, 1024), 8, nil)
		// master.DialRaw(0, "122:11")
	}
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

func TestReconnect(t *testing.T) {
	// ShowLog = 0
	handler := NewNormalAcessHandler("master", nil)
	handler.LoginAccess["slaver"] = "abc"
	handler.DialAccess = [][]string{{".*", ".*"}}
	master := NewProxy("master", handler)
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	defer master.Close()
	slaver := NewProxy("slaver", nil)
	slaver.ReconnectDelay = 100 * time.Millisecond
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	c, _ := slaver.SelectChannel("master")
	c.Close()
	time.Sleep(200 * time.Millisecond)
	if _, err = slaver.SelectChannel("master"); err != nil {
		t.Error(err)
		return
	}
	master.Close()
	time.Sleep(200 * time.Millisecond)
	slaver.Close()
	time.Sleep(200 * time.Millisecond)
}

func TestProxyForward(t *testing.T) {
	var masterEcho *Echo
	handler := NewNormalAcessHandler("master", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, 1024, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	}))
	handler.LoginAccess["slaver"] = "abc"
	handler.LoginAccess["client"] = "abc"
	handler.DialAccess = [][]string{{".*", ".*"}}
	master := NewProxy("master", handler)
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	var slaverEcho = NewEcho("slaver")
	slaverHandler := NewNormalAcessHandler("slaver", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, 1024, sid, uri)
		// err = fmt.Errorf("error")
		return
	}))
	slaverHandler.DialAccess = [][]string{{".*", ".*"}}
	slaver := NewProxy("slaver", slaverHandler)
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	client := NewProxy("client", nil)
	_, err = client.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		slaver.Close()
		client.Close()
		master.Close()
		time.Sleep(time.Millisecond)
	}()
	//
	//test forward
	//
	listen, _ := url.Parse("tcp://:2335")
	_, err = client.StartForward("x0", listen, "master->slaver->xx")
	if err != nil {
		t.Error(err)
		return
	}
	conn, err := net.Dial("tcp", "localhost:2335")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Fprintf(conn, "forward data")
	<-slaverEcho.R
	//
	slaverEcho.W <- 1
	buf := make([]byte, 1024)
	readed, err := conn.Read(buf)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println("--->", string(buf[:readed]))
	//
	conn.Close()
	<-slaverEcho.R
	//
	//test forward error
	fmt.Printf("\n\n\ntest forward error\n")
	listen, _ = url.Parse("tcp://:2336")
	_, err = client.StartForward("x1", listen, "not->xx->xx")
	if err != nil {
		t.Error(err)
		return
	}
	conn, err = net.Dial("tcp", "localhost:2336")
	if err != nil {
		t.Error(err)
		return
	}
	_, err = conn.Read(buf)
	if err == nil {
		t.Error(err)
		return
	}
	//
	_, err = client.StartForward("", listen, "not->xx->xx")
	if err == nil {
		t.Error(err)
		return
	}
	//
	//test stop forward
	err = client.StopForward("x0")
	if err != nil {
		t.Error(err)
		return
	}
}

func dialProxyConn(proxy, remote string, port uint16) (conn net.Conn, err error) {
	conn, err = net.Dial("tcp", proxy)
	if err != nil {
		return
	}
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		conn.Close()
		return
	}
	buf := make([]byte, 1024*64)
	err = xio.FullBuffer(conn, buf, 2, nil)
	if err != nil {
		conn.Close()
		return
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		err = fmt.Errorf("unsupported %x", buf)
		conn.Close()
		return
	}
	blen := len(remote) + 7
	buf[0], buf[1], buf[2] = 0x05, 0x01, 0x00
	buf[3], buf[4] = 0x03, byte(len(remote))
	copy(buf[5:], []byte(remote))
	buf[blen-2] = byte(port / 256)
	buf[blen-1] = byte(port % 256)
	_, err = conn.Write(buf[:blen])
	if err != nil {
		conn.Close()
		return
	}
	err = xio.FullBuffer(conn, buf, 5, nil)
	if err != nil {
		conn.Close()
		return
	}
	switch buf[3] {
	case 0x01:
		err = xio.FullBuffer(conn, buf[5:], 5, nil)
	case 0x03:
		err = xio.FullBuffer(conn, buf[5:], uint32(buf[4])+2, nil)
	case 0x04:
		err = xio.FullBuffer(conn, buf[5:], 17, nil)
	default:
		err = fmt.Errorf("reply address type is not supported:%v", buf[3])
	}
	if err != nil {
		conn.Close()
		return
	}
	if buf[1] != 0x00 {
		err = fmt.Errorf("response code(%x)", buf[1])
	}
	return
}

func TestSocketProxyForward(t *testing.T) {
	handler := NewNormalAcessHandler("master", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			err = fmt.Errorf("it must be not reached")
		}
		// err = fmt.Errorf("error")
		return
	}))
	handler.LoginAccess["slaver"] = "abc"
	handler.LoginAccess["client"] = "abc"
	handler.DialAccess = [][]string{{".*", ".*"}}
	master := NewProxy("master", handler)
	// var masterEcho = NewEcho("master")
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	var slaverEcho = NewEcho("slaver")
	slaverHandler := NewNormalAcessHandler("slaver", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, 1024, sid, uri)
		// err = fmt.Errorf("error")
		return
	}))
	slaverHandler.DialAccess = [][]string{{".*", ".*"}}
	slaver := NewProxy("slaver", slaverHandler)
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	client := NewProxy("client", nil)
	_, err = client.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		slaver.Close()
		client.Close()
		master.Close()
		time.Sleep(time.Millisecond)
	}()
	//
	//test forward
	//
	listen, _ := url.Parse("socks://:2336")
	_, err = client.StartForward("x0", listen, "master->slaver->tcp://${HOST}")
	if err != nil {
		t.Error(err)
		return
	}
	conn, err := dialProxyConn("localhost:2336", "xx", 100)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println("start send data---->")
	fmt.Fprintf(conn, "forward data")
	<-slaverEcho.R
	fmt.Println("salver readed")
	//
	slaverEcho.W <- 1
	buf := make([]byte, 1024)
	readed, err := conn.Read(buf)
	if err != nil {
		t.Error(err)
		return
	}
	if readed < 1 {
		t.Error("not data")
		return
	}
	fmt.Println("readed--->", readed, string(buf[:readed]))
	//
	conn.Close()
	<-slaverEcho.R
	//
	//test stop forward
	err = client.StopForward("x0")
	if err != nil {
		t.Error(err)
		return
	}
}

func TestProxyTLS(t *testing.T) {
	handler := NewNormalAcessHandler("master", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			err = fmt.Errorf("it must be not reached")
		}
		// err = fmt.Errorf("error")
		return
	}))
	handler.LoginAccess["slaver"] = "abc"
	handler.LoginAccess["client"] = "abc"
	handler.DialAccess = [][]string{{".*", ".*"}}
	master := NewProxy("master", handler)
	master.Cert = "bsrouter/bsrouter.pem"
	master.Key = "bsrouter/bsrouter.key"
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	var slaverEcho = NewEcho("slaver")
	slaverHandler := NewNormalAcessHandler("slaver", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, 1024, sid, uri)
		// err = fmt.Errorf("error")
		return
	}))
	slaverHandler.DialAccess = [][]string{{".*", ".*"}}
	slaver := NewProxy("slaver", slaverHandler)
	slaver.Cert = "bsrouter/bsrouter.pem"
	slaver.Key = "bsrouter/bsrouter.key"
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	client := NewProxy("client", nil)
	client.Cert = "bsrouter/bsrouter.pem"
	client.Key = "bsrouter/bsrouter.key"
	_, err = client.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err != nil {
		t.Error(err)
		return
	}
	slaver.Close()
	client.Close()
	master.Close()
	//
	//
	client = NewProxy("client", nil)
	client.Cert = "bsrouter/bsrouter.xxx"
	client.Key = "bsrouter/bsrouter.xxx"
	_, err = client.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err == nil {
		t.Error(err)
		return
	}
	//
	master = NewProxy("master", nil)
	master.Cert = "bsrouter/bsrouter.xxx"
	master.Key = "bsrouter/bsrouter.xxx"
	err = master.ListenMaster(":9232")
	if err == nil {
		t.Error(err)
		return
	}
	master.UniqueSid()
	master.Close()
}

func TestProxyClose(t *testing.T) {
	for i := 0; i < 10; i++ {
		master := NewProxy("master", nil)
		err := master.ListenMaster(":9232")
		if err != nil {
			t.Error(err)
			return
		}
		master.Close()
	}
}

func TestProxyDialSync(t *testing.T) {
	var masterEcho *Echo
	handler := NewNormalAcessHandler("master", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, 1024, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	}))
	handler.DialAccess = [][]string{{".*", ".*"}}
	handler.LoginAccess["slaver"] = "abc"
	handler.LoginAccess["client"] = "abc"
	master := NewProxy("master", handler)
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	var slaverEcho = NewEcho("slaver")
	slaverHandler := NewNormalAcessHandler("slaver", DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, 1024, sid, uri)
		// err = fmt.Errorf("error")
		return
	}))
	slaverHandler.DialAccess = [][]string{{".*", ".*"}}
	slaver := NewProxy("slaver", slaverHandler)
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	client := NewProxy("client", nil)
	_, err = client.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		master.Close()
		slaver.Close()
		client.Close()
		time.Sleep(time.Second)
	}()
	cona, connb, _ := dialer.CreatePipedConn()
	_, err = client.SyncDial("master->slaver->xxx", connb)
	if err != nil {
		t.Error("error")
		return
	}
	fmt.Fprintf(cona, "data->%v", 0)
	<-slaverEcho.R
	slaverEcho.W <- 1
	buf := make([]byte, 1024)
	readed, err := cona.Read(buf)
	if err != nil {
		t.Error("error")
		return
	}
	if readed < 1 {
		t.Error("error")
		return
	}
	fmt.Printf("master-->\n%v\n\n", converter.JSON(master.State()))
	fmt.Printf("slaver-->\n%v\n\n", converter.JSON(slaver.State()))
	fmt.Printf("client-->\n%v\n\n", converter.JSON(client.State()))
	_, err = client.SyncDial("master->error", connb)
	if err == nil {
		t.Error("error")
		return
	}
}
