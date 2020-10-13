package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/codingeasygo/bsck"
	"github.com/codingeasygo/util/xio"
)

//Version is bsrouter version
const Version = "2.0.0"

//Config is pojo for configure
type Config struct {
	Name    string `json:"name"`
	Console string `json:"socks5"`
}

var slaver string
var quiet bool
var conn bool
var proxy bool
var ping bool
var state bool
var shell bool
var help bool
var closer = func() {}

const proxyChainsConf = `
strict_chain
proxy_dns
remote_dns_subnet 224
tcp_read_time_out 15000
tcp_connect_time_out 8000
[ProxyList]
socks5 	127.0.0.1 %v
`

func usage() {
	fmt.Fprintf(os.Stderr, "Bond Socket Console Version %v\n", Version)
	fmt.Fprintf(os.Stderr, "Usage:  %v [option] <forward uri>\n", "bsconsole")
	fmt.Fprintf(os.Stderr, "        %v 'x->y->tcp://127.0.0.1:80'\n", "bsconsole")
	fmt.Fprintf(os.Stderr, "bsrouter options:\n")
	flag.PrintDefaults()
}

func main() {
	var showVersion bool
	flag.StringVar(&slaver, "slaver", "", "the slaver console address")
	flag.BoolVar(&quiet, "quiet", false, "quiet mode")
	flag.BoolVar(&conn, "conn", false, "redirect connection to stdio")
	flag.BoolVar(&proxy, "proxy", false, "redirect connection to std proxy")
	flag.BoolVar(&ping, "ping", false, "send ping to uri")
	flag.BoolVar(&help, "help", false, "show help")
	flag.BoolVar(&help, "h", false, "show help")
	flag.BoolVar(&state, "state", false, "show node state")
	flag.BoolVar(&shell, "shell", false, "start shell which forwaring conn to uri")
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
	case "bs-conn":
		conn = true
	case "bs-proxy":
		proxy = true
	case "bs-ping":
		ping = true
	case "bs-state":
		state = true
	case "bs-shell":
		shell = true
	}
	if help {
		usage()
		os.Exit(1)
		return
	}
	var fullURI string
	args := flag.Args()
	if len(args) > 0 && regexp.MustCompile("^(socks5)://.*$").MatchString(args[0]) {
		slaver = flag.Arg(0)
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "forwarding uri is not setted\n")
			usage()
			os.Exit(1)
			return
		}
		fullURI = args[1]
		args = args[2:]
	} else if len(slaver) < 1 {
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
		if len(config.Console) > 0 {
			slaver = "socks5://" + config.Console
		} else {
			fmt.Fprintf(os.Stderr, "not client access listen on config %v\n", path)
			os.Exit(1)
		}
		if len(args) > 0 {
			fullURI = args[0]
			args = args[1:]
		}
	} else {
		if len(args) > 0 {
			fullURI = args[0]
			args = args[1:]
		}
	}
	var err error
	var closer func()
	console := bsck.NewConsole(slaver)
	defer console.Close()
	if conn {
		closer = func() {}
		err = console.Redirect(fullURI, os.Stdin, os.Stdout, xio.CloserF(func() (err error) {
			if !quiet {
				fmt.Printf("Conn done with remote closed\n")
			}
			return
		}))
		if err != nil {
			if !quiet {
				fmt.Printf("Conn done with %v\n", err)
			}
			os.Exit(1)
		}
	} else if proxy {
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "ProxyRunner is not setting\n")
			usage()
			os.Exit(1)
			return
		}
		err = console.Proxy(fullURI, os.Stdin, os.Stdout, os.Stderr, func(listener net.Listener) (env []string, runnerName string, runnerArgs []string, err error) {
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			confFile, err := ioutil.TempFile("", "proxychains-*.conf")
			if err != nil {
				return
			}
			confFile.WriteString(fmt.Sprintf(proxyChainsConf, port))
			confFile.Close()
			runnerName = "proxychains4"
			runnerArgs = append(runnerArgs, "-q", "-f", confFile.Name())
			runnerArgs = append(runnerArgs, args...)
			return
		})
		if err != nil {
			fmt.Printf("Proxy done with %v\n", err)
			os.Exit(1)
		}
	} else if ping {
		closer = console.StartPing(fullURI, time.Second)
	} else if state {
		var query string
		if len(args) > 0 {
			query = args[0]
		}
		err = console.PrintState(fullURI, query)
		if err != nil {
			fmt.Printf("Print state done with %v\n", err)
			os.Exit(1)
		}
	} else if shell {
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "ProxyKey/ProxyRunner is not setting\n")
			usage()
			os.Exit(1)
			return
		}
		err = console.Proxy(fullURI, os.Stdin, os.Stdout, os.Stderr, func(listener net.Listener) (env []string, runnerName string, runnerArgs []string, err error) {
			keys := args[0]
			keys = strings.ReplaceAll(keys, "${HOST}", listener.Addr().String())
			env = strings.Split(keys, ",")
			for i, e := range env {
				if e == "http_proxy" || e == "https_proxy" || e == "HTTP_PROXY" || e == "HTTPS_PROXY" {
					env[i] = fmt.Sprintf("%v=http://%v", e, listener.Addr())
				} else if e == "socks_proxy" || e == "SOCKS_PROXY" {
					env[i] = fmt.Sprintf("%v=socks5://%v", e, listener.Addr())
				}
			}
			runnerName = args[1]
			for _, arg := range args[2:] {
				runnerArgs = append(runnerArgs, strings.ReplaceAll(arg, "${PROXY_HOST}", listener.Addr().String()))
			}
			return
		})
		if err != nil {
			fmt.Printf("Shell done with %v\n", err)
			os.Exit(1)
		}
	} else {
		usage()
		os.Exit(1)
	}
	if closer != nil {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc,
			syscall.SIGHUP,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT)
		<-sigc
		closer()
	}
}
