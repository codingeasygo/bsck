package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/util/xcrypto"
)

// Version is bsrouter version
const Version = "2.0.0"

var exitf = func(code int) {
	os.Exit(code)
}

func main() {
	go http.ListenAndServe(":6062", nil)
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
	if len(os.Args) > 1 && os.Args[1] == "create" {
		if len(os.Args) > 3 && os.Args[2] == "cert" {
			name := "bsrouter"
			if len(os.Args) > 4 {
				name = os.Args[4]
			}
			rootCert, rootKey, rootCertPEM, rootKeyPEM, xerr := xcrypto.GenerateRootCA([]string{name}, name, 2048)
			if xerr != nil {
				fmt.Printf("%v\n", xerr)
				os.Exit(1)
				return
			}
			_, _, certPEM, keyPEM, xerr := xcrypto.GenerateCert(rootCert, rootKey, nil, []string{name}, name, []string{name}, nil, 2048)
			if xerr != nil {
				fmt.Printf("%v\n", xerr)
				os.Exit(1)
				return
			}
			xerr = os.WriteFile(filepath.Join(os.Args[3], name+"CA.key"), rootKeyPEM, os.ModePerm)
			if xerr != nil {
				fmt.Printf("%v\n", xerr)
				os.Exit(1)
				return
			}
			xerr = os.WriteFile(filepath.Join(os.Args[3], name+"CA.pem"), rootCertPEM, os.ModePerm)
			if xerr != nil {
				fmt.Printf("%v\n", xerr)
				os.Exit(1)
				return
			}
			xerr = os.WriteFile(filepath.Join(os.Args[3], name+".key"), keyPEM, os.ModePerm)
			if xerr != nil {
				fmt.Printf("%v\n", xerr)
				os.Exit(1)
				return
			}
			xerr = os.WriteFile(filepath.Join(os.Args[3], name+".pem"), certPEM, os.ModePerm)
			if xerr != nil {
				fmt.Printf("%v\n", xerr)
				os.Exit(1)
				return
			}
			os.Exit(0)
		} else {
			fmt.Printf("not supported %v\n", os.Args[2:])
			os.Exit(1)
		}
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
			router.ErrorLog("bsrouter read config from .bsrouter.json or ~/.bsrouter.json or /etc/bsrouter/bsrouter.json or /etc/bsrouter.json fail with %v\n", err)
			exitf(1)
		}
	}
	service := router.NewService()
	service.ConfigPath = configPath
	service.NewBackend = func(name string) (cmd *exec.Cmd, err error) {
		switch name {
		case "HostForward":
			exeFile, _ := os.Executable()
			exeDir, _ := filepath.Abs(filepath.Dir(exeFile))
			runFile := filepath.Join(exeDir, "bsconsole")
			consoleURI := service.Config.ConsoleURI()
			cmd = exec.Command("bash", "-c", fmt.Sprintf("sudo %v --slaver=%v host.service", runFile, consoleURI))
			if runtime.GOOS == "darwin" {
				// cmd = exec.Command("osascript", "-e", fmt.Sprintf(`do shell script "%v --slaver %v host.service" with administrator privileges`, runFile, consoleURI))
				cmd = exec.Command("bash", "-c", fmt.Sprintf("sudo %v --slaver=%v host.service", runFile, consoleURI))
			}
		default:
			err = fmt.Errorf("%v is not supported", name)
		}
		return
	}
	err = service.Start()
	if err != nil {
		panic(err)
	}
	wc := make(chan os.Signal, 1)
	signal.Notify(wc, os.Interrupt, syscall.SIGTERM)
	<-wc
	service.Stop()
}
