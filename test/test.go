package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/web"
	"golang.org/x/net/websocket"
)

func init() {
	go http.ListenAndServe(":6063", nil)
}

func run_bandwidth(addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	go func() {
		buf := make([]byte, 1024)
		for {
			conn.Write(buf)
			time.Sleep(time.Second)
		}
	}()
	buf := make([]byte, 8*1024)
	all := []int64{}
	last := time.Now()
	show := time.Now()
	readed := int64(0)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			panic(err)
		}
		readed += int64(n)
		if time.Since(last) >= time.Second {
			all = append(all, readed)
			readed = 0
			last = time.Now()
			if len(all) > 5 {
				all = all[1:]
			}
		}
		if time.Since(show) > 2*time.Second && len(all) > 0 {
			total := int64(0)
			for _, v := range all {
				total += v
			}
			total = total / int64(len(all))
			if total > 1024*1024*1024 {
				fmt.Printf("speed %v GB/s =>%v\n", float64(total)/1024/1024/1024, total)
			} else if total > 1024*1024 {
				fmt.Printf("speed %v MB/s =>%v\n", float64(total)/1024/1024, total)
			} else if total > 1024 {
				fmt.Printf("speed %v KB/s =>%v\n", float64(total)/1024, total)
			}
		}
	}
}

func main() {
	if true {
		run_bandwidth(os.Args[1])
		return
	}
	handler := websocket.Server{
		Handler: func(c *websocket.Conn) {
			fmt.Fprintf(c, "connected\n")
			io.Copy(c, c)
		},
	}
	router := web.NewSessionMux("")
	router.HandleFunc("^/test(\\?.*)?$", func(s *web.Session) web.Result {
		return s.Printf("OK")
	})
	router.HandleNormal("^/ws/.*$", handler)
	dialer := dialer.NewWebDialer("control", router)
	err := dialer.Bootstrap(xmap.M{})
	if err != nil {
		panic(err)
	}
	go func() {
		listener, err := net.Listen("tcp", ":8832")
		if err != nil {
			panic(err)
		}
		sid := uint16(0)
		for {
			conn, err := listener.Accept()
			if err != nil {
				panic(err)
			}
			go func(c net.Conn) {
				_, err = dialer.Dial(nil, sid, "http://control", c)
				if err != nil {
					panic(err)
				}
			}(conn)
		}
	}()
	{
		sid := uint16(0)
		client := &http.Client{
			Transport: &http.Transport{
				Dial: func(network, addr string) (conn net.Conn, err error) {
					sid++
					conn, raw := net.Pipe()
					_, err = dialer.Dial(nil, sid, "http://control", raw)
					return
				},
			},
		}
		xclient := xhttp.NewClient(client)
		web.HandleFunc("^/test(\\?.*)?$", func(s *web.Session) web.Result {
			data, err := xclient.GetText("http://control/test")
			if err == nil {
				return s.Printf("%v", data)
			} else {
				return s.Printf("%v", err)
			}
		})
		web.ListenAndServe(":8833")
	}
}
