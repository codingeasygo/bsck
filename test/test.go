package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"

	"github.com/codingeasygo/bsck/dialer"
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
	router.HandleNormal("^.*$", handler)
	dialer := dialer.NewWebDialer("control", router)
	err := dialer.Bootstrap(xmap.M{})
	if err != nil {
		panic(err)
	}
	listener, err := net.Listen("tcp", ":8832")
	if err != nil {
		panic(err)
	}
	sid := uint64(0)
	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		go func(c net.Conn) {
			_, err = dialer.Dial(sid, "http://control", c)
			if err != nil {
				panic(err)
			}
		}(conn)
	}
}
