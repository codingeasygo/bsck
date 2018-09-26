package bsck

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/url"
	"testing"
	"time"
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
	fmt.Printf("RECV:%v\n", string(p))
	e.Recv++
	e.R <- 1
	return
}

func (e *Echo) Read(b []byte) (n int, err error) {
	fmt.Println("Echo.Read-->started", e)
	if e.Err != nil {
		err = e.Err
		return
	}
	<-e.W
	copy(b, []byte(e.Data))
	n = len(e.Data)
	e.Send++
	fmt.Println("Echo.Read-->done", e)
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

func TestProxy(t *testing.T) {
	master := NewProxy("master")
	master.Router.ACL["ms"] = "abc"
	master.Router.ACL["slaver"] = "abc"
	master.Router.ACL["slaver2"] = "abc"
	master.Router.ACL["slaver3"] = "abc"
	master.Router.ACL["err[slaver3"] = "abc"
	master.Heartbeat = 10 * time.Millisecond
	master.StartHeartbeat()
	var masterEcho *Echo
	master.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	})
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
	slaver := NewProxy("slaver")
	slaver.Heartbeat = 10 * time.Millisecond
	slaver.StartHeartbeat()
	var slaverEcho *Echo
	slaver.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, sid, uri)
		// err = fmt.Errorf("error")
		return
	})
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	slaver2 := NewProxy("slaver2")
	slaver2.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})

	{ //test slaver2->master->server
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
	}
	{ //test slaver2->master->slaver->server
		fmt.Printf("\n\n\ntest slaver2->master->slaver->server\n")
		slaverEcho = NewEcho("slaver")
		slaver2Echo := NewEcho("client")
		_, err = slaver2.Dial("master->slaver->xx", slaver2Echo)
		if err != nil {
			t.Error(err)
			return
		}
		//client->server
		slaver2Echo.W <- 1
		<-slaverEcho.R
		slaver2Echo.W <- 1
		<-slaverEcho.R
		// server->client
		slaverEcho.W <- 1
		<-slaver2Echo.R
		slaverEcho.W <- 1
		<-slaver2Echo.R
		//close
		slaver2Echo.Close()
		<-slaverEcho.R
	}
	{ //multi channel
		fmt.Printf("\n\n\ntest multi channel\n")
		var msEcho *Echo
		ms0 := NewProxy("ms")
		ms0.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
			fmt.Println("ms test dail to ", uri)
			conn = NewRawConn(msEcho, sid, uri)
			return
		})
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
	}
	{ //channel close
		fmt.Printf("\n\n\ntest channel close\n")
		slaver3Echo := NewEcho("slaver3")
		slaver3 := NewProxy("slaver3")
		slaver3.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
			fmt.Println("slaver3 test dail to ", uri)
			conn = NewRawConn(slaver3Echo, sid, uri)
			return
		})
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
		slaver3.SelectChannel("master").Close()
		<-slaver3Echo.R
		<-slaver2Echo.R
	}
	{ //dial remote fail
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
	}
}

func TestError(t *testing.T) {
	master := NewProxy("master")
	master.Router.ACL["ms"] = "abc"
	var masterEcho *Echo
	master.Router.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	})
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
		slaverErr := NewProxy("error")
		err = slaverErr.Login(&ChannelOption{
			Enable: true,
			Remote: "localhost:9232",
			Token:  "abc",
			Index:  0,
		})
		if err == nil {
			t.Error("error")
			return
		}
		slaverErr = NewProxy("error")
		err = slaverErr.Login(&ChannelOption{
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
			ReadWriteCloser: NewEcho("testing"),
			name:            "xx",
		}
		testc.Close()
		master.Router.addChannel(testc)
		//
		//test proc login fail
		err = master.Router.procLogin(NewRawConn(NewEcho("data"), 0, "ur"), make([]byte, 1024), 1024)
		if err == nil {
			t.Error("error")
			return
		}
		//
		echo := NewEcho("data")
		err = master.Router.procLogin(&Channel{ReadWriteCloser: echo}, make([]byte, 1024), 1024)
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
		err = master.Router.procDial(&Channel{ReadWriteCloser: echo}, buf, 16)
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		data = []byte("x@error")
		copy(buf[13:], data)
		echo = NewEcho("data")
		err = master.Router.procDial(&Channel{ReadWriteCloser: echo}, buf, uint32(len(data)+13))
		time.Sleep(time.Second) //wait for go
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		data = []byte("x@not->error")
		copy(buf[13:], data)
		echo = NewEcho("data")
		err = master.Router.procDial(&Channel{ReadWriteCloser: echo}, buf, uint32(len(data)+13))
		if err != nil || echo.Recv != 1 {
			t.Error(err)
			return
		}
		//
		data = []byte("x@xx->error")
		copy(buf[13:], data)
		echo = NewEcho("data")
		err = master.Router.procDial(&Channel{ReadWriteCloser: echo}, buf, uint32(len(data)+13))
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
		slaver := NewProxy("slaver")
		err = slaver.LoginChannel(false, &ChannelOption{
			Enable: true,
			Token:  "abc",
			Remote: "loclahost:12",
		})
		if err == nil {
			t.Error(err)
			return
		}
		err = slaver.Login(&ChannelOption{
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
		err = slaver.Login(&ChannelOption{
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
		err = slaver.JoinConn(merr, &ChannelOption{
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
		err = slaver.JoinConn(merr, &ChannelOption{
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
		src := &Channel{ReadWriteCloser: srcRaw}
		dstRaw := NewErrReadWriteCloser([]byte("error"), 10)
		dst := &Channel{ReadWriteCloser: dstRaw}
		master.Router.addTable(src, 1000, dst, 1001)
		buf := make([]byte, 1024)
		copy(buf[13:], []byte("error"))
		//
		binary.BigEndian.PutUint64(buf[5:], 1000)
		err = master.Router.procDialBack(src, buf, 18)
		if err != srcRaw.Err {
			t.Error(err)
			return
		}
		//
		binary.BigEndian.PutUint64(buf[5:], 2000)
		err = master.Router.procDialBack(src, buf, 18)
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
		src := &Channel{ReadWriteCloser: srcRaw}
		master.Router.loopReadRaw(src, 10240)
		//read cmd error
		binary.BigEndian.PutUint32(buf, 100)
		srcRaw = NewErrReadWriteCloser(buf[0:104], 0)
		src = &Channel{ReadWriteCloser: srcRaw}
		master.Router.loopReadRaw(src, 50)
		//cmd error
		binary.BigEndian.PutUint32(buf, 100)
		srcRaw = NewErrReadWriteCloser(buf[0:104], 0)
		src = &Channel{ReadWriteCloser: srcRaw}
		master.Router.loopReadRaw(src, 10240)
	}
	{ //test for cover
		rawConn := NewRawConn(NewEcho("data"), 0, "")
		rawConn.Index()
		func() {
			defer func() {
				recover()
			}()
			rawConn.Read(nil)
		}()
		cmdString(CmdLoginBack)
		echo := NewErrReadWriteCloser([]byte("data"), 0)
		fullBuf(echo, make([]byte, 1024), 8, nil)
		master.DialRaw(0, "122:11")
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
	master := NewProxy("master")
	master.Router.ACL["slaver"] = "abc"
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewProxy("slaver")
	slaver.ReconnectDelay = 100 * time.Millisecond
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	slaver.SelectChannel("master").Close()
	time.Sleep(100 * time.Millisecond)
	if slaver.SelectChannel("master") == nil {
		t.Error("error")
		return
	}
	master.Close()
	time.Sleep(200 * time.Millisecond)
	slaver.Close()
	time.Sleep(200 * time.Millisecond)
}

func TestDialTCP(t *testing.T) {
	handler := NewTCPDialer()
	_, err := handler.DialRaw(10, "tcp://localhost:80")
	if err != nil {
		t.Error("error")
		return
	}
	_, err = handler.DialRaw(10, "tcp:localhost:80")
	if err == nil {
		t.Error("error")
		return
	}
	_, err = handler.DialRaw(10, "tcp://localhost:80%EX%B8%AD%E6%96%87")
	if err == nil {
		t.Error("error")
		return
	}
	handler.OnConnClose(nil)
}

func TestProxyForward(t *testing.T) {
	master := NewProxy("master")
	master.Router.ACL["slaver"] = "abc"
	master.Router.ACL["client"] = "abc"
	var masterEcho *Echo
	master.Router.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	})
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewProxy("slaver")
	var slaverEcho = NewEcho("slaver")
	slaver.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, sid, uri)
		// err = fmt.Errorf("error")
		return
	})
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	client := NewProxy("client")
	err = client.Login(&ChannelOption{
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
		client.Close()
		time.Sleep(time.Second)
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
	err = fullBuf(conn, buf, 2, nil)
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
	err = fullBuf(conn, buf, 5, nil)
	if err != nil {
		conn.Close()
		return
	}
	switch buf[3] {
	case 0x01:
		err = fullBuf(conn, buf[5:], 5, nil)
	case 0x03:
		err = fullBuf(conn, buf[5:], uint32(buf[4])+2, nil)
	case 0x04:
		err = fullBuf(conn, buf[5:], 17, nil)
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
	master := NewProxy("master")
	master.Router.ACL["slaver"] = "abc"
	master.Router.ACL["client"] = "abc"
	var masterEcho *Echo
	master.Router.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	})
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewProxy("slaver")
	var slaverEcho = NewEcho("slaver")
	slaver.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, sid, uri)
		// err = fmt.Errorf("error")
		return
	})
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	client := NewProxy("client")
	err = client.Login(&ChannelOption{
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
		client.Close()
		time.Sleep(time.Second)
	}()
	//
	//test forward
	//
	listen, _ := url.Parse("socks://:2336")
	_, err = client.StartForward("x0", listen, "master->slaver")
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
	master := NewProxy("master")
	master.Cert = "bsrouter/bsrouter.pem"
	master.Key = "bsrouter/bsrouter.key"
	master.Router.ACL["slaver"] = "abc"
	master.Router.ACL["client"] = "abc"
	var masterEcho *Echo
	master.Router.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		if uri == "error" {
			err = fmt.Errorf("error")
		} else {
			conn = NewRawConn(masterEcho, sid, uri)
		}
		// err = fmt.Errorf("error")
		return
	})
	err := master.ListenMaster(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	slaver := NewProxy("slaver")
	slaver.Cert = "bsrouter/bsrouter.pem"
	slaver.Key = "bsrouter/bsrouter.key"
	var slaverEcho = NewEcho("slaver")
	slaver.Handler = DialRawF(func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, sid, uri)
		// err = fmt.Errorf("error")
		return
	})
	slaver.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	//
	client := NewProxy("client")
	client.Cert = "bsrouter/bsrouter.pem"
	client.Key = "bsrouter/bsrouter.key"
	err = client.Login(&ChannelOption{
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
		client.Close()
		time.Sleep(time.Second)
	}()
	//
	//
	client = NewProxy("client")
	client.Cert = "bsrouter/bsrouter.xxx"
	client.Key = "bsrouter/bsrouter.xxx"
	err = client.Login(&ChannelOption{
		Remote: "localhost:9232",
		Token:  "abc",
		Index:  0,
	})
	if err == nil {
		t.Error(err)
		return
	}
	//
	master = NewProxy("master")
	master.Cert = "bsrouter/bsrouter.xxx"
	master.Key = "bsrouter/bsrouter.xxx"
	err = master.ListenMaster(":9232")
	if err == nil {
		t.Error(err)
		return
	}
	master.UniqueSid()
}
