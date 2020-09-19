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
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/codingeasygo/util/xmap"
	"golang.org/x/net/websocket"
)

//Version is bsrouter version
const Version = "1.4.2"

//CharTerm is console stop command
var CharTerm = []byte{3}

//Web is pojo for web configure
type Web struct {
	Listen string `json:"listen"`
}

//Web is pojo for configure
type Config struct {
	Name   string `json:"name"`
	Listen string `json:"listen"`
	Socks5 string `json:"socks5"`
	Web    Web    `json:"web"`
}

var server string
var win32 bool
var proxy bool
var ping bool
var state string
var help bool

func usage() {
	fmt.Fprintf(os.Stderr, "Bond Socket Console Version %v\n", Version)
	fmt.Fprintf(os.Stderr, "Usage:  %v [option] <forward uri>\n", "bsconsole")
	fmt.Fprintf(os.Stderr, "        %v 'x->y->tcp://127.0.0.1:80'\n", "bsconsole")
	fmt.Fprintf(os.Stderr, "bsrouter options:\n")
	fmt.Fprintf(os.Stderr, "        srv\n")
	fmt.Fprintf(os.Stderr, "             the remote bsrouter listen address, eg: ws://127.0.0.1:1082, tcp://127.0.0.1:2023\n")
}

func main() {
	var showVersion bool
	flag.StringVar(&server, "srv", "", "")
	flag.BoolVar(&win32, "win32", false, "win32 command")
	flag.BoolVar(&proxy, "proxy", false, "proxy mode")
	flag.BoolVar(&ping, "ping", false, "ping mode")
	flag.BoolVar(&help, "help", false, "show help")
	flag.BoolVar(&help, "h", false, "show help")
	flag.StringVar(&state, "state", "", "state mode")
	flag.BoolVar(&showVersion, "version", false, "show version")
	flag.BoolVar(&showVersion, "v", false, "show version")
	flag.Parse()
	if showVersion {
		fmt.Println(Version)
		os.Exit(1)
		return
	}
	_, fn := filepath.Split(os.Args[0])
	switch fn {
	case "bs-ping":
		ping = true
	case "bs-state":
		state = "router"
	}
	if help {
		usage()
		os.Exit(1)
		return
	}
	var fullURI, remote string
	if regexp.MustCompile("^(ws|wss|socks5)://.*$").MatchString(flag.Arg(0)) {
		server = flag.Arg(0)
		fullURI = ""
		remote = flag.Arg(0)
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
		if len(flag.Args()) > 0 {
			fullURI = flag.Args()[0]
		}
		if ping && !strings.Contains(fullURI, "tcp://echo") {
			fullURI += "->tcp://echo"
		}
		if len(state) > 0 {
			if len(fullURI) > 0 {
				fullURI += "->state://" + state
			} else {
				fullURI += "state://" + state
			}
		}
		remote = fullURI
	}
	//
	dialBeg := time.Now()
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
		targetURI := server
		conn, err = websocket.Dial(targetURI, "", rurl.Scheme+"://"+rurl.Host)
	case "socks5":
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
			buf[4] = byte(len(fullURI))
			copy(buf[5:], []byte(fullURI))
			binary.BigEndian.PutUint16(buf[5+len(fullURI):], 0)
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
	if ping {
		runPing(conn, remote, dialBeg)
	} else if proxy {
		runProxy(conn)
	} else if len(state) > 0 {
		runState(conn)
	} else {
		usage()
		os.Exit(1)
	}
}

func runPing(conn io.ReadWriteCloser, remote string, dialBeg time.Time) {
	var line []byte
	var err error
	var c uint64
	buf := make([]byte, 65)
	reader := bufio.NewReader(conn)
	for {
		pingBeg := time.Now()
		c++
		fmt.Fprintf(bytes.NewBuffer(buf), "%v", c)
		buf[64] = '\n'
		_, err = conn.Write(buf)
		if err != nil {
			break
		}
		line, _, err = reader.ReadLine()
		if err != nil {
			break
		}
		pingUsed := time.Now().Sub(pingBeg)
		fmt.Printf("%v Bytes from %v time=%v\n", len(line), remote, pingUsed)
		time.Sleep(time.Second)
	}
	if err != nil {
		fmt.Printf("Ping to %v fail with %v", remote, err)
	}
	// pingUsed := time.Now().Sub(pingBeg)
	// totalUsed := time.Now().Sub(dialBeg)
	// fmt.Printf("Ping to %v %v\n   Avg:\t\t%v\n   Count:\t\t%v\n   Used:\t%v\n\n", remote, status, pingUsed/time.Duration(i), i, totalUsed)
}

func runProxy(conn io.ReadWriteCloser) {
	go io.Copy(os.Stdout, conn)
	io.Copy(conn, os.Stdin)
	conn.Close()
}

func runState(conn io.ReadWriteCloser) {
	defer conn.Close()
	if state != "router" {
		fmt.Println("state is not supported by " + flag.Args()[0])
		return
	}
	data, _ := ioutil.ReadAll(conn)
	vals := xmap.M{}
	err := json.Unmarshal(data, &vals)
	if err != nil {
		fmt.Println(err)
		return
	}
	switch state {
	case "router":
		fmt.Printf("[Channels]\n")
		channels := vals.Map("channels")
		for name := range channels {
			fmt.Printf(" ->%v\n", name)
			bond := channels.Map(name)
			for idx := range bond {
				val := bond.Map(idx)
				idxVal, _ := strconv.ParseInt(strings.Replace(idx, "_", "", -1), 10, 64)
				heartbeat := val.Int64Def(0, "heartbeat")
				hs := time.Unix(0, heartbeat*1e6).Format("2006-01-02 15:04:05")
				fmt.Printf("   %d % 4d   %v   %v\n", idxVal, int(val["used"].(float64)), hs, val["connect"])
			}
		}
		fmt.Printf("\n\n[Table]\n")
		table := vals.ArrayStrDef(nil, "table")
		for _, t := range table {
			fmt.Printf(" %v\n", t)
		}
		fmt.Printf("\n")
	}
}
