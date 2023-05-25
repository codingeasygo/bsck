package main

import (
	"io"
	"net"
	"time"

	"github.com/codingeasygo/bsck/router"
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

// func runProxyServer(addr string) {
// 	ln, err := net.Listen("tcp", addr)
// 	if err != nil {
// 		panic(err)
// 	}
// 	for {
// 		conn, err := ln.Accept()
// 		if err != nil {
// 			panic(err)
// 		}
// 		remote, err := net.Dial("tcp", "127.0.0.1:13101")
// 		if err != nil {
// 			panic(err)
// 		}
// 		target := xio.NewPrintConn("X", remote)
// 		go io.Copy(conn, target)
// 		go io.Copy(target, conn)
// 	}
// }

func main() {
	// runDumServer(":13100")
	go runEchoServer(":13200")
	router.ShowLog = 3
	router.SetLogLevel(router.LogLevelDebug)

	access0 := router.NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["NX"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 := router.NewProxy("N0", access0)
	proxy0.Heartbeat = time.Second

	access1 := router.NewNormalAcessHandler("N1")
	access1.LoginAccess["NX"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 := router.NewProxy("N1", access1)
	proxy1.Heartbeat = time.Second
	err := proxy0.Listen(":13100")
	if err != nil {
		panic(err)
	}
	// err = proxy1.Listen(":13101")
	// if err != nil {
	// 	panic(err)
	// }
	// _, _, err = proxy1.Login(xmap.M{
	// 	"remote": "127.0.0.1:13100",
	// 	"token":  "123",
	// })
	// if err != nil {
	// 	panic(err)
	// }
	// proxy0.Start()
	// proxy1.Start()
	// runProxyServer(":13100")
	waiter := make(chan int, 1)
	<-waiter
}
