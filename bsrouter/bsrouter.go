package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/Centny/gwf/util"
	"github.com/sutils/bsck"
	"github.com/sutils/dialer"
)

type Web struct {
	Suffix string `json:"suffix"`
	Listen string `json:"listen"`
	Auth   string `json:"auth"`
}

type Config struct {
	Name      string                `json:"name"`
	Cert      string                `json:"cert"`
	Key       string                `json:"key"`
	Listen    string                `json:"listen"`
	ACL       map[string]string     `json:"acl"`
	Socks5    string                `json:"socks5"`
	Web       Web                   `json:"web"`
	ShowLog   int                   `json:"showlog"`
	LogFlags  int                   `json:"logflags"`
	Forwards  map[string]string     `json:"forwards"`
	Channels  []*bsck.ChannelOption `json:"channels"`
	Dialer    util.Map              `json:"dialer"`
	Reconnect int64                 `json:"reconnect"`
}

const Version = "1.1.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-h" {
		fmt.Fprintf(os.Stderr, "Bond Socket Router Version %v\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:  %v configure\n", "bsrouter")
		fmt.Fprintf(os.Stderr, "        %v /etc/bsrouter.json'\n", "bsrouter")
		fmt.Fprintf(os.Stderr, "bsrouter options:\n")
		fmt.Fprintf(os.Stderr, "        name\n")
		fmt.Fprintf(os.Stderr, "             the router name\n")
		fmt.Fprintf(os.Stderr, "        listen\n")
		fmt.Fprintf(os.Stderr, "             the master listen address\n")
		fmt.Fprintf(os.Stderr, "        forwards\n")
		fmt.Fprintf(os.Stderr, "             the forward uri by 'listen address':'uri'\n")
		fmt.Fprintf(os.Stderr, "        showlog\n")
		fmt.Fprintf(os.Stderr, "             the log level\n")
		fmt.Fprintf(os.Stderr, "        channels\n")
		fmt.Fprintf(os.Stderr, "             the channel configure\n")
		fmt.Fprintf(os.Stderr, "        channels.token\n")
		fmt.Fprintf(os.Stderr, "             the auth token to master\n")
		fmt.Fprintf(os.Stderr, "        channels.local\n")
		fmt.Fprintf(os.Stderr, "             the binded local address connect to master\n")
		fmt.Fprintf(os.Stderr, "        channels.remote\n")
		fmt.Fprintf(os.Stderr, "             the master address\n")
		os.Exit(1)
	}
	var config Config
	var err error
	var data []byte
	if len(os.Args) > 1 {
		data, err = ioutil.ReadFile(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config from %v fail with %v\n", os.Args[1], err)
			os.Exit(1)
		}
	} else {
		u, _ := user.Current()
		for _, path := range []string{"./.bsrouter.json", "./bsrouter.json", u.HomeDir + "/.bsrouter/bsrouter.json", u.HomeDir + "/.bsrouter.json", "/etc/bsrouter/bsrouter.json", "/etc/bsrouer.json"} {
			data, err = ioutil.ReadFile(path)
			if err == nil {
				fmt.Printf("bsrouter using config from %v\n", path)
				break
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config from .bsrouter.json or ~/.bsrouter.json or /etc/bsrouter/bsrouter.json or /etc/bsrouter.json fail with %v\n", err)
			os.Exit(1)
		}
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse config fail with %v\n", err)
		os.Exit(1)
	}
	if config.LogFlags > 0 {
		bsck.Log.SetFlags(config.LogFlags)
	}
	bsck.ShowLog = config.ShowLog
	dialerPool := dialer.NewPool()
	dialerPool.AddDialer(config.Dialer,
		dialer.NewCmdDialer(), dialer.NewEchoDialer(),
		dialer.NewWebDialer(), dialer.NewTCPDialer())
	socks5 := bsck.NewSocksProxy()
	forward := bsck.NewForward()
	proxy := bsck.NewProxy(config.Name)
	proxy.Cert, proxy.Key = config.Cert, config.Key
	if config.Reconnect > 0 {
		proxy.ReconnectDelay = time.Duration(config.Reconnect) * time.Millisecond
	}
	if len(config.ACL) > 0 {
		proxy.ACL = config.ACL
	}
	var dailer = func(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
		if strings.Contains(uri, "->") {
			sid, err = proxy.Dial(uri, raw)
		} else if regexp.MustCompile("^[A-Za-z0-9]*://.*$").MatchString(uri) {
			var conn io.ReadWriteCloser
			sid = proxy.UniqueSid()
			conn, err = dialerPool.Dial(sid, uri)
			if err == nil {
				go func() {
					io.Copy(raw, conn)
					raw.Close()
					conn.Close()
				}()
				go func() {
					io.Copy(conn, raw)
					raw.Close()
					conn.Close()
				}()
			}
		} else {
			parts := strings.SplitN(uri, "?", 2)
			router := forward.FindForward(parts[0])
			if len(router) < 1 {
				err = fmt.Errorf("forward not found by %v", uri)
				return
			}
			dialURI := router[1]
			if len(parts) > 1 {
				if strings.Contains(dialURI, "?") {
					dialURI += "&" + parts[1]
				} else {
					dialURI += "?" + parts[1]
				}
			}
			sid, err = proxy.Dial(dialURI, raw)
		}
		return
	}
	socks5.Dailer = dailer
	forward.Dailer = dailer
	proxy.Handler = bsck.DialRawF(func(sid uint64, uri string) (conn bsck.Conn, err error) {
		raw, err := dialerPool.Dial(sid, uri)
		if err == nil {
			conn = bsck.NewRawConn(raw, sid, uri)
		}
		return
	})
	if len(config.Listen) > 0 {
		err := proxy.ListenMaster(config.Listen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start master on %v fail with %v\n", config.Listen, err)
			os.Exit(1)
		}
	}
	if len(config.Channels) > 0 {
		proxy.LoginChannel(true, config.Channels...)
	}
	for loc, uri := range config.Forwards {
		if strings.HasPrefix(loc, "tcp://") {
			err := proxy.StartForward(loc, uri)
			if err != nil {
				fmt.Fprintf(os.Stderr, "start forward by %v fail with %v\n", loc+"->"+uri, err)
				os.Exit(1)
			}
		} else {
			err := forward.AddForward(loc, uri)
			if err != nil {
				fmt.Fprintf(os.Stderr, "add forward by %v fail with %v\n", loc+"->"+uri, err)
				os.Exit(1)
			}
		}
	}
	proxy.StartHeartbeat()
	if len(config.Socks5) > 0 {
		err := socks5.Start(config.Socks5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start socks5 by %v fail with %v\n", config.Socks5, err)
			os.Exit(1)
		}
	}
	http.HandleFunc("/dav/", forward.ProcWebSubsH)
	http.HandleFunc("/ws/", forward.ProcWebSubsH)
	http.HandleFunc("/", forward.HostForwardF)
	forward.WebAuth = config.Web.Auth
	forward.WebSuffix = config.Web.Suffix
	server := &http.Server{Addr: config.Web.Listen}
	if len(config.Web.Listen) > 0 {
		go func() {
			bsck.Log.Printf("I bsrouter listen web on %v\n", config.Web.Listen)
			fmt.Println(server.ListenAndServe())
		}()
	}
	wc := make(chan os.Signal, 1)
	signal.Notify(wc, os.Interrupt, os.Kill)
	<-wc
	fmt.Println("clear bsrouter...")
	proxy.Close()
	server.Close()
}
