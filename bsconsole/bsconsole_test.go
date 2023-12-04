package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xio"
)

func init() {
	go http.ListenAndServe(":6060", nil)
}

var configTestMaster = `
{
    "name": "master",
    "listen": ":15023",
    "web": {},
    "console": "",
    "forwards": {},
    "channels": [],
    "dialer": {
        "standard": 1
    },
    "acl": {
        "slaver": "abc",
        "caller": "abc"
    },
    "access": [
        [
            ".*",
            ".*"
        ]
    ]
}
`

var configTestSlaver = `
{
    "name": "slaver",
    "listen": "",
    "web": {},
    "console": "",
    "forwards": {},
    "channels": [
        {
            "enable":1,
            "remote": "localhost:15023",
            "token":  "abc",
            "index":  0
        }
    ],
    "dialer":{
        "standard": 1
	},
	"access": [
        [
            ".*",
            ".*"
        ]
    ]
}
`

var configTestCaller = `
{
    "name": "caller",
    "listen": "",
    "web": {},
    "console": ":1701",
    "forwards": {},
    "channels": [
        {
            "enable":1,
            "remote": "localhost:15023",
            "token":  "abc",
            "index":  0
        }
    ],
    "dialer":{
        "standard": 1
    }
}
`

func TestConsole(t *testing.T) {
	usr, _ := user.Current()
	env = append(env, "HOME="+usr.HomeDir)
	socks.SetLogLevel(socks.LogLevelDebug)
	var err error

	master := router.NewService()
	json.Unmarshal([]byte(configTestMaster), &master.Config)
	err = master.Start()
	if err != nil {
		t.Error(err)
		return
	}
	slaver := router.NewService()
	json.Unmarshal([]byte(configTestSlaver), &slaver.Config)
	err = slaver.Start()
	if err != nil {
		t.Error(err)
		return
	}
	caller := router.NewService()
	json.Unmarshal([]byte(configTestCaller), &caller.Config)
	err = caller.Start()
	if err != nil {
		t.Error(err)
		return
	}
	sshKey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile(filepath.Join(usr.HomeDir, ".ssh", "id_rsa"))
		w.Write(data)
	}))
	caller.AddForward("ssh-key", sshKey.URL)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "abc")
	}))
	echo, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		for {
			conn, err := echo.Accept()
			if err != nil {
				break
			}
			go io.Copy(conn, conn)
		}
	}()
	//
	var stdinWriter, stdoutReader *os.File
	//
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(".bsrouter.json", []byte(configTestCaller), os.ModePerm)
	{ //conn
		stdin, stdinWriter, _ = os.Pipe()
		stdoutReader, stdout, _ = os.Pipe()
		waiter := sync.WaitGroup{}
		exit = func(int) {
			t.Error("exit")
			stdinWriter.Close()
			stdoutReader.Close()
		}
		//
		waiter.Add(1)
		go func() {
			runall("bsconsole", "conn", "tcp://"+echo.Addr().String())
			waiter.Done()
		}()
		go fmt.Fprintf(stdinWriter, "abc")
		buffer := make([]byte, 1024)
		xio.FullBuffer(stdoutReader, buffer, 3, nil)
		if string(buffer[0:3]) != "abc" {
			t.Error("error")
			return
		}
		sig <- syscall.SIGTERM
		waiter.Wait()
		//
		//error
		exit = func(int) {}
		runall("bsconsole", "conn")
		runall("bsconsole", "conn", "tcp://127.0.0.1:0")
	}
	{ //ping
		stdin, stdout, stderr = os.Stdin, os.Stdout, os.Stderr
		exit = func(int) {
			t.Error("exit")
		}
		runall("bsconsole", "ping", "tcp://echo", "1")
		//
		//error
		exit = func(int) {}
		runall("bsconsole", "ping", "tcp://127.0.0.1:0", "1")
	}
	{ //state
		stdin, stdout, stderr = os.Stdin, os.Stdout, os.Stderr
		exit = func(int) {
			t.Error("exit")
		}
		runall("bsconsole", "state")
		runall("bsconsole", "state", "master")
		runall("bsconsole", "state", "http://state")
		runall("bsconsole", "state", "http://state", "*=*")
		//
		//error
		exit = func(int) {}
		runall("bsconsole", "state", "tcp://127.0.0.1:0", "3")
	}
	{ //shell
		stdin, stdout, stderr = os.Stdin, os.Stdout, os.Stderr
		stdoutReader, stdout, _ = os.Pipe()
		waiter := sync.WaitGroup{}
		buffer := make([]byte, 1024)
		exit = func(int) {
			t.Error("exit")
			stdoutReader.Close()
		}
		waiter.Add(1)
		go func() {
			runall("bsconsole", "shell", "master", "http_proxy", "curl", ts.URL)
			waiter.Done()
		}()
		xio.FullBuffer(stdoutReader, buffer, 3, nil)
		if string(buffer[0:3]) != "abc" {
			t.Error("error")
			return
		}
		waiter.Add(1)
		go func() {
			runall("bsconsole", "shell", "master", "socks_proxy", "curl", ts.URL)
			waiter.Done()
		}()
		xio.FullBuffer(stdoutReader, buffer, 3, nil)
		if string(buffer[0:3]) != "abc" {
			t.Error("error")
			return
		}
		//
		//error
		exit = func(int) {}
		runall("bsconsole", "shell")
		runall("bsconsole", "shell", "tcp://127.0.0.1:0", "socks_proxy", "xx")
	}
	// { //ssh,scp,sftp
	// 	exec.Command("go", "build", ".")
	// 	stdin, stdinWriter, _ = os.Pipe()
	// 	stdoutReader, stdout, _ = os.Pipe()
	// 	waiter := sync.WaitGroup{}
	// 	buffer := make([]byte, 1024)
	// 	exit = func(int) {
	// 		t.Error("exit")
	// 		stdoutReader.Close()
	// 	}
	// 	waiter.Add(1)
	// 	dir, _ := filepath.Abs(".")
	// 	command := filepath.Join(dir, "bsconsole")
	// 	go func() {
	// 		runall(command, "ssh", "master->tcp://dev.loc:22", "-lroot", "echo", "abc")
	// 		waiter.Done()
	// 	}()
	// 	xio.FullBuffer(stdoutReader, buffer, 3, nil)
	// 	if string(buffer[0:3]) != "abc" {
	// 		t.Error(string(buffer[0:3]))
	// 		return
	// 	}
	// 	//
	// 	//error
	// 	exit = func(int) {}
	// 	runall("bsconsole", "ssh", "tcp://127.0.0.1:0")
	// }
	{ //install
		exec.Command("go", "build", ".")
		exit = func(int) { t.Error("exit") }
		runall("bsconsole", "uninstall")
		runall("bsconsole", "install")
		//
		//error
		exit = func(int) {}
		runall("bsconsole", "install")
	}
	{ //--slaver
		stdin, stdout, stderr = os.Stdin, os.Stdout, os.Stderr
		exit = func(int) {
			t.Error("exit")
		}
		runall("bsconsole", "ping", "--slaver=127.0.0.1:1701", "tcp://echo", "1")
	}
	{ //bs-
		stdin, stdout, stderr = os.Stdin, os.Stdout, os.Stderr
		exit = func(int) {
			t.Error("exit")
		}
		runall("bs-ping", "--slaver=127.0.0.1:1701", "tcp://echo", "1")
	}
	{ //other
		exit = func(int) {}
		runall("bsconsole", "version")
		runall("bsconsole", "help")
		runall("bsconsole")
	}
}
