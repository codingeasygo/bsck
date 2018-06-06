package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"regexp"
	"strings"
	"sync"

	"github.com/sutils/readkey"
	"golang.org/x/net/websocket"
)

const Version = "1.1.0"

var CharTerm = []byte{3}

type Web struct {
	Listen string `json:"listen"`
}

type Config struct {
	Name   string `json:"name"`
	Listen string `json:"listen"`
	Socks5 string `json:"socks5"`
	Web    Web    `json:"web"`
}

func appendSize(uri string) string {
	cols, rows := readkey.GetSize()
	if strings.Contains(uri, "?") {
		uri += fmt.Sprintf("&cols=%v&rows=%v", cols, rows)
	} else {
		uri += fmt.Sprintf("?cols=%v&rows=%v", cols, rows)
	}
	return uri
}

func main() {
	// tm.Clear()
	var server string
	var win32 bool
	flag.StringVar(&server, "s", "", "")
	flag.BoolVar(&win32, "win32", false, "win32 command")
	flag.Parse()
	if len(flag.Args()) < 1 {
		fmt.Fprintf(os.Stderr, "Bond Socket Console Version %v\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:  %v [option] <forward uri>\n", "bsrouter")
		fmt.Fprintf(os.Stderr, "        %v 'x->y->tcp://127.0.0.1:80'\n", "bsrouter")
		fmt.Fprintf(os.Stderr, "bsrouter options:\n")
		fmt.Fprintf(os.Stderr, "        s\n")
		fmt.Fprintf(os.Stderr, "             the remote bsrouter listen address, eg: ws://127.0.0.1:1082, tcp://127.0.0.1:2023\n")
		os.Exit(1)
		return
	}
	var uri string
	if regexp.MustCompile("^[A-Za-z0-9]*://.*$").MatchString(flag.Arg(0)) {
		server = flag.Arg(0)
		uri = ""
	} else if len(server) < 1 {
		var err error
		var data []byte
		var path string
		u, _ := user.Current()
		for _, path = range []string{"./.bsrouter.json", "./bsrouter.json", u.HomeDir + "/.bsrouter/bsrouter.json", u.HomeDir + "/.bsrouter.json", "/etc/bsrouter/bsrouter.json", "/etc/bsrouer.json"} {
			data, err = ioutil.ReadFile(path)
			if err == nil {
				fmt.Printf("bsconsole using config %v\n", path)
				break
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config from .bsrouter.json or ~/.bsrouter/bsrouter.json  or ~/.bsrouter.json or /etc/bsrouter/bsrouter.json or /etc/bsrouter.json fail with %v\n", err)
			os.Exit(1)
		}
		var config Config
		err = json.Unmarshal(data, &config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse config fail with %v\n", err)
			os.Exit(1)
		}
		if len(config.Web.Listen) > 0 {
			server = "ws://" + config.Web.Listen + "/ws"
		} else if len(config.Socks5) > 0 {
			server = "socks5://" + config.Socks5
		} else {
			fmt.Fprintf(os.Stderr, "not client access listen on config %v\n", path)
			os.Exit(1)
		}
		uri = flag.Args()[0]
	}
	//
	var err error
	var conn io.ReadWriteCloser
	rurl, err := url.Parse(server)
	if err != nil {
		fmt.Printf("connect to %v fail with %v\n", server, err)
		os.Exit(1)
	}
	switch rurl.Scheme {
	case "ws":
		fallthrough
	case "wss":
		fullURI := server
		if len(uri) > 0 {
			if strings.Contains(uri, "->") {
				uri = appendSize(uri)
				fullURI += "/?router=" + url.QueryEscape(uri)
			} else {
				fullURI += "/" + uri
				fullURI = appendSize(fullURI)
			}
		} else {
			fullURI = appendSize(fullURI)
		}
		conn, err = websocket.Dial(fullURI, "", "https://"+rurl.Host)
	case "socks5":
		uri = appendSize(uri)
		conn, err = net.Dial("tcp", rurl.Host)
		if err == nil {
			buf := make([]byte, 1024*64)
			proxyReader := bufio.NewReader(conn)
			_, err = conn.Write([]byte{0x05, 0x01, 0x00})
			if err != nil {
				return
			}
			_, err = conn.Read(buf)
			buf[0], buf[1], buf[2], buf[3] = 0x05, 0x01, 0x00, 0x13
			buf[4] = byte(len(uri))
			copy(buf[5:], []byte(uri))
			binary.BigEndian.PutUint16(buf[5+len(uri):], 0)
			_, err = conn.Write(buf[:buf[4]+7])
			if err != nil {
				return
			}
			_, err = proxyReader.Read(buf)
			if err != nil {
				return
			}
			if buf[1] != 0x00 {
				err = fmt.Errorf("connection fail")
			}
		}
	default:
		conn, err = net.Dial(rurl.Scheme, rurl.Host)
	}
	if err != nil {
		fmt.Printf("connect to %v fail with %v\n", server, err)
		os.Exit(1)
	}
	if win32 {
		runWinConsole(conn)
	} else {
		runUnixConsole(conn)
	}
}

func runWinConsole(conn io.ReadWriteCloser) {
	last := 0
	lastLck := sync.RWMutex{}
	stopc := 0
	go func() {
		buf := make([]byte, 1024)
		for {
			readed, err := os.Stdin.Read(buf)
			if err != nil {
				break
			}
			stopc = 0
			lastLck.Lock()
			fmt.Fprintf(os.Stdout, "\033[%dA", 1)
			fmt.Fprintf(os.Stdout, "\033[%dC", last)
			_, err = conn.Write(buf[:readed])
			lastLck.Unlock()
			if err != nil {
				break
			}
		}
	}()
	go func() {
		buf := make([]byte, 1024)
		for {
			readed, err := conn.Read(buf)
			if err != nil {
				break
			}
			lastLck.Lock()
			parts := bytes.Split(buf[:readed], []byte("\n"))
			last = len(parts[len(parts)-1])
			_, err = os.Stdout.Write(buf[:readed])
			lastLck.Unlock()
			if err != nil {
				break
			}
		}
	}()
	wc := make(chan os.Signal)
	signal.Notify(wc, os.Interrupt, os.Kill)
	for {
		<-wc
		stopc++
		if stopc >= 5 {
			break
		}
	}
	conn.Close()
	return
}

func runUnixConsole(conn io.ReadWriteCloser) {
	readkey.Open()
	defer func() {
		conn.Close()
		readkey.Close()
		os.Exit(1)
	}()
	go func() {
		io.Copy(os.Stdout, conn)
		fmt.Printf("connection is closed\n")
		readkey.Close()
		os.Exit(1)
	}()
	stopc := 0
	for {
		key, err := readkey.Read()
		if err != nil {
			break
		}
		if bytes.Equal(key, CharTerm) {
			stopc++
			if stopc > 5 {
				break
			}
		} else {
			stopc = 0
		}
		_, err = conn.Write(key)
		if err != nil {
			break
		}
	}
}
