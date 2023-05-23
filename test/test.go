package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/web"
	"golang.org/x/net/websocket"
)

func init() {
	go http.ListenAndServe(":6063", nil)
}

func main() {
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
