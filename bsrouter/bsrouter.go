package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"

	"github.com/codingeasygo/bsck"
)

//Version is bsrouter version
const Version = "2.0.0"

var exitf = func(code int) {
	os.Exit(code)
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Println(Version)
		os.Exit(0)
	}
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
		fmt.Fprintf(os.Stderr, "        channels.local\n")
		fmt.Fprintf(os.Stderr, "             the binded local address connect to master\n")
		fmt.Fprintf(os.Stderr, "        channels.remote\n")
		fmt.Fprintf(os.Stderr, "             the master address\n")
		os.Exit(1)
	}
	var configPath string
	var err error
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	} else {
		u, _ := user.Current()
		for _, path := range []string{"./.bsrouter.json", "./bsrouter.json", u.HomeDir + "/.bsrouter/bsrouter.json", u.HomeDir + "/.bsrouter.json", "/etc/bsrouter/bsrouter.json", "/etc/bsrouer.json"} {
			_, err = os.Stat(path)
			if err == nil {
				configPath = path
				break
			}
		}
		if err != nil {
			bsck.ErrorLog("bsrouter read config from .bsrouter.json or ~/.bsrouter.json or /etc/bsrouter/bsrouter.json or /etc/bsrouter.json fail with %v\n", err)
			exitf(1)
		}
	}
	service := bsck.NewService()
	service.ConfigPath = configPath
	err = service.Start()
	wc := make(chan os.Signal, 1)
	signal.Notify(wc, os.Interrupt, os.Kill)
	<-wc
	service.Stop()
}
