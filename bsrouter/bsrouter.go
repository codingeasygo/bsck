package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/sutils/bsck"
)

type Config struct {
	Name     string                `json:"name"`
	Listen   string                `json:"listen"`
	ShowLog  int                   `json:"showlog"`
	Forwards map[string]string     `json:"forwads"`
	Channels []*bsck.ChannelOption `json:"channels"`
}

const Version = "1.0.0"

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Bond Socket Router Version %v\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:  %v -c configure\n", "bsrouter")
		fmt.Fprintf(os.Stderr, "        %v -c /etc/bsrouter.json'\n", "bsrouter")
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
	data, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config from %v fail with %v", os.Args[2], err)
		os.Exit(1)
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse config from %v fail with %v", os.Args[2], err)
		os.Exit(1)
	}
	bsck.ShowLog = config.ShowLog
	proxy := bsck.NewProxy(config.Name)
	if len(config.Listen) > 0 {
		err := proxy.ListenMaster(config.Listen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start master on %v fail with %v", config.Listen, err)
			os.Exit(1)
		}
	}
	if len(config.Channels) > 0 {
		err := proxy.LoginChannel(config.Channels...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start master on %v fail with %v", config.Listen, err)
			os.Exit(1)
		}
	}
	for loc, uri := range config.Forwards {
		err := proxy.StartForward(loc, uri)
		if err != nil {
			fmt.Fprintf(os.Stderr, "start forward by %v fail with %v", loc+"->"+uri, err)
			os.Exit(1)
		}
	}
	proxy.StartHeartbeat()
	wc := make(chan int)
	wc <- 1
}
