package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
)

func init() {
	go http.ListenAndServe(":6063", nil)
}

func main() {
	switch os.Args[1] {
	case "server":
		runServer()
	case "echo":
		runEcho()
	case "get":
		runGet(os.Args[2])
	case "bench":
		switch os.Args[2] {
		case "get":
			benchGet(os.Args[3])
		}

	}
}

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

// func runDumServer(addr string) {
// 	ln, err := net.Listen("tcp", addr)
// 	if err != nil {
// 		panic(err)
// 	}
// 	for {
// 		conn, err := ln.Accept()
// 		if err != nil {
// 			panic(err)
// 		}
// 		go func(c net.Conn) {
// 			buf := make([]byte, 1024)
// 			for {
// 				n, xerr := c.Read(buf)
// 				if xerr != nil {
// 					break
// 				}
// 				fmt.Printf("R==>%v\n", buf[:n])
// 			}
// 		}(conn)
// 	}
// }

func runProxyServer(addr, target string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}
		remote, err := net.Dial("tcp", target)
		if err != nil {
			panic(err)
		}
		target := xio.NewPrintConn("X", remote)
		// target.Mode = 0x10
		go io.Copy(conn, target)
		go io.Copy(target, conn)
	}
}

func runServer() {
	// runDumServer(":13100")
	go runProxyServer(":1108", "127.0.0.1:1107")
	go runProxyServer(":13103", "127.0.0.1:13100")
	go runEchoServer(":13200")
	// router.ShowLog = 3
	// router.SetLogLevel(router.LogLevelDebug)
	// dialer.SetLogLevel(router.LogLevelDebug)

	dialer0 := dialer.NewPool("N0")
	dialer0.Bootstrap(xmap.M{
		"std":   1,
		"udpgw": xmap.M{},
	})
	access0 := router.NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["NX"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	access0.RawDialer = router.DialRawStdF(func(channel router.Conn, id uint16, uri string) (raw io.ReadWriteCloser, err error) {
		raw, err = dialer0.Dial(channel, id, uri)
		return
	})
	proxy0 := router.NewProxy("N0", access0)
	proxy0.Router.BufferSize = 2 * 1024
	proxy0.Forward.BufferSize = 2 * 1024
	proxy0.Cert = "../../certs/server.crt"
	proxy0.Key = "../../certs/server.key"
	proxy0.Heartbeat = time.Second

	access1 := router.NewNormalAcessHandler("N1")
	access1.LoginAccess["NX"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 := router.NewProxy("N1", access1)
	proxy1.Router.BufferSize = 2 * 1024
	proxy1.Forward.BufferSize = 2 * 1024
	proxy1.Heartbeat = time.Second
	err := proxy0.Listen("tcp://:13100")
	if err != nil {
		panic(err)
	}
	err = proxy0.Listen("quic://:13100")
	if err != nil {
		panic(err)
	}
	u, _ := url.Parse("socks://127.0.0.1:1106")
	proxy0.StartForward("xx", u, "${HOST}")
	// err = proxy1.Listen(":13101")
	// if err != nil {
	// 	panic(err)
	// }
	_, _, err = proxy1.Login(xmap.M{
		"remote": "quic://127.0.0.1:13100",
		"token":  "123",
		"tls_ca": "../../certs/rootCA.crt",
	})
	if err != nil {
		panic(err)
	}
	// proxy0.Start()
	// proxy1.Start()
	// runProxyServer(":13100")
	waiter := make(chan int, 1)
	<-waiter
}

func runEcho() {
	conn, err := socks.Dial("127.0.0.1:1107", "127.0.0.1:13200")
	if err != nil {
		panic(err)
	}
	go io.Copy(os.Stdout, conn)
	io.Copy(conn, os.Stdin)
}

func runGet(uri string) {
	client := xhttp.NewClient(&http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return socks.Dial("127.0.0.1:1107", addr)
			},
		},
	})
	for i := 0; i < 100; i++ {
		data, err := client.GetText(uri)
		if err != nil {
			panic(err)
		}
		fmt.Println(data)
	}
}

func benchGet(uri string) {
	client := xhttp.NewClient(&http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return socks.Dial("127.0.0.1:1107", addr)
			},
		},
	})
	for x := 0; x < 100; x++ {
		waiter := sync.WaitGroup{}
		for i := 0; i < 1000; i++ {
			waiter.Add(1)
			go func(v int) {
				data, err := client.GetText(uri)
				fmt.Printf("%03d get data with data:%v,err:%v\n", v, len(data), err)
				waiter.Done()
			}(i)
		}
		waiter.Wait()
		time.Sleep(time.Second)
	}
}
