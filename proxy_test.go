package bsck

import (
	"fmt"
	"log"
	"testing"
	"time"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
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
	if e.Err != nil {
		err = e.Err
		return
	}
	<-e.W
	copy(b, []byte(e.Data))
	n = len(e.Data)
	e.Send++
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
	var masterEcho *Echo
	master.Router.DailRaw = func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("master test dail to ", uri)
		conn = NewRawConn(masterEcho, sid, uri, 4096)
		// err = fmt.Errorf("error")
		return
	}
	err := master.Listen(":9232")
	if err != nil {
		t.Error(err)
		return
	}
	//
	slaver := NewRouter("slaver")
	var slaverEcho *Echo
	slaver.DailRaw = func(sid uint64, uri string) (conn Conn, err error) {
		fmt.Println("slaver test dail to ", uri)
		conn = NewRawConn(slaverEcho, sid, uri, 4096)
		// err = fmt.Errorf("error")
		return
	}
	slaver.Login("", "localhost:9232", "abc", 0)
	//
	slaver2 := NewRouter("slaver2")
	slaver2.Login("", "localhost:9232", "abc", 0)

	{ //test slaver2->master->server
		fmt.Printf("\n\n\ntest slaver2->master->server\n")
		masterEcho = NewEcho("master")
		slaver2Echo := NewEcho("client")
		_, err = slaver2.Dail("master->xx", slaver2Echo)
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
		_, err = slaver2.Dail("master->slaver->xx", slaver2Echo)
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
		ms0 := NewRouter("ms")
		ms0.DailRaw = func(sid uint64, uri string) (conn Conn, err error) {
			fmt.Println("ms test dail to ", uri)
			conn = NewRawConn(msEcho, sid, uri, 4096)
			return
		}
		ms0.LoginChannel(&ChannelOption{
			Token:  "abc",
			Local:  "",
			Remote: "localhost:9232",
		}, &ChannelOption{
			Token:  "abc",
			Local:  "",
			Remote: "localhost:9232",
		})
		for i := 0; i < 10; i++ {
			msEcho = NewEcho("ms")
			slaver2Echo := NewEcho("client")
			_, err = slaver2.Dail("master->ms->xx", slaver2Echo)
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
		slaver3 := NewRouter("slaver3")
		slaver3.DailRaw = func(sid uint64, uri string) (conn Conn, err error) {
			fmt.Println("slaver3 test dail to ", uri)
			conn = NewRawConn(slaver3Echo, sid, uri, 4096)
			return
		}
		slaver3.Login("", "localhost:9232", "abc", 0)
		slaver2Echo := NewEcho("client")
		_, err = slaver2.Dail("master->slaver3->xx", slaver2Echo)
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
	{ //clear
		master.Close()
		time.Sleep(time.Second)
	}
}
