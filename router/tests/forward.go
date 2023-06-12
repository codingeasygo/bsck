package main

import (
	"bytes"
	"encoding/binary"
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
	case "dns":
		runDnsServer()
	case "speed":
		testSpeed(os.Args[2])
	case "tps":
		testTPS(os.Args[2])
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
		fmt.Printf("echo accept from %v\n", conn.RemoteAddr())
		go io.Copy(conn, conn)
	}
}

func runDnsServer() {
	laddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:53")
	ln, err := net.ListenUDP("udp", laddr)
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 2048)
	connAll := map[string]net.Conn{}
	for {
		n, fromAddr, err := ln.ReadFromUDP(buf)
		if err != nil {
			break
		}
		remote := connAll[fromAddr.String()]
		if remote == nil {
			remote, err = net.Dial("udp", "10.1.0.2:53")
			if err != nil {
				panic(err)
			}
			connAll[fromAddr.String()] = remote
			go func(r net.Conn, toAddr *net.UDPAddr) {
				buf := make([]byte, 2048)
				for {
					n, err := r.Read(buf)
					if err != nil {
						break
					}
					ln.WriteToUDP(buf[0:n], toAddr)
				}
			}(remote, fromAddr)
		}
		remote.Write(buf[0:n])
	}
}

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
		"std": 1,
		"udpgw": xmap.M{
			"dns": "192.168.1.1:53",
		},
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

func speedConn(conn net.Conn, action string) (err error) {
	buf := make([]byte, 8*1024)
	for i := uint16(0); i < 4*1024; i++ {
		binary.BigEndian.PutUint16(buf[2*i:], i)
	}
	n := 0
	seq := uint64(0)
	lastAll := []int64{}
	lastTime := time.Now()
	lastBytes := int64(0)
	for {
		if action == "R" {
			n, err = conn.Read(buf)
		} else {
			seq++
			n, err = conn.Write(buf)
		}
		if err != nil {
			break
		}
		lastBytes += int64(n)
		if time.Since(lastTime) > time.Second {
			lastAll = append(lastAll, lastBytes)
			if len(lastAll) > 5 {
				lastAll = lastAll[1:]
			}
			lastBytes = 0
			lastTime = time.Now()
			lastTotal := int64(0)
			for _, v := range lastAll {
				lastTotal += v
			}
			lastAvg := lastTotal / int64(len(lastAll))
			if lastAvg > 1024*1024*1024 {
				fmt.Printf("%v %v GB/s\n", action, float64(lastAvg)/1024/1024/1024)
			} else if lastAvg > 1024*1024 {
				fmt.Printf("%v %v MB/s\n", action, float64(lastAvg)/1024/1024)
			} else if lastAvg > 1024 {
				fmt.Printf("%v %v KB/s\n", action, float64(lastAvg)/1024)
			} else {
				fmt.Printf("%v %v B/s\n", action, float64(lastAvg))
			}
		}
	}
	return
}

func testSpeed(addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	go speedConn(conn, "W")
	speedConn(conn, "R")
}

func testTPS(addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	mtu := 8 * 1024
	out := make([]byte, mtu)
	in := make([]byte, mtu)
	n := 0
	seq := uint64(0)
	action := "T"
	lastAll := []int64{}
	lastTime := time.Now()
	lastBytes := int64(0)
	for {
		seq++
		binary.BigEndian.PutUint64(out, seq)
		n, err = conn.Write(out)
		if err != nil {
			break
		}
		err = xio.FullBuffer(conn, in, uint32(len(in)), nil)
		if err != nil {
			break
		}
		if !bytes.Equal(out, in) {
			err = fmt.Errorf("%v", "not equal")
			break
		}
		lastBytes += int64(n)
		if time.Since(lastTime) > time.Second {
			lastAll = append(lastAll, lastBytes)
			if len(lastAll) > 5 {
				lastAll = lastAll[1:]
			}
			lastBytes = 0
			lastTime = time.Now()
			lastTotal := int64(0)
			for _, v := range lastAll {
				lastTotal += v
			}
			lastAvg := lastTotal / int64(len(lastAll))
			if lastAvg > 1024*1024*1024 {
				fmt.Printf("%v %v GB/s\n", action, float64(lastAvg)/1024/1024/1024)
			} else if lastAvg > 1024*1024 {
				fmt.Printf("%v %v MB/s\n", action, float64(lastAvg)/1024/1024)
			} else if lastAvg > 1024 {
				fmt.Printf("%v %v KB/s\n", action, float64(lastAvg)/1024)
			} else {
				fmt.Printf("%v %v B/s\n", action, float64(lastAvg))
			}
		}
	}
	fmt.Println("err-->", err)
}
