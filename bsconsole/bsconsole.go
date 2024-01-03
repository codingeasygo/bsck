package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn"
	"github.com/codingeasygo/tun2conn/util"
	"github.com/codingeasygo/util/proxy"
	"github.com/codingeasygo/util/xhash"
	"github.com/codingeasygo/util/xio"
	"github.com/songgao/water"
)

// Version is bsrouter version
const Version = "2.0.0"

var stdin = os.Stdin
var stdout = os.Stdout
var stderr = os.Stderr
var exit = os.Exit
var sig = make(chan os.Signal, 100)

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
	_, fn := filepath.Split(os.Args[0])
	fn = strings.TrimSuffix(fn, ".exe")
	fmt.Fprintf(stderr, "Bond Socket Console Version %v\n", Version)
	fmt.Fprintf(stderr, "Usage:  %v command <forward uri> [options]\n", fn)
	fmt.Fprintf(stderr, "        %v conn 'x->y->tcp://127.0.0.1:80'\n", fn)
	fmt.Fprintf(stderr, "%v command list:\n", fn)
	fmt.Fprintf(stderr, "    conn        redirect uri connection to stdio\n")
	fmt.Fprintf(stderr, "        %v conn 'x->y->tcp://127.0.0.1:80'\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    proxy       redirect local connection to uri\n")
	fmt.Fprintf(stderr, "        %v conn 'x->y' :100322\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    proxychains start shell by proxychains\n")
	fmt.Fprintf(stderr, "        %v proxychains 'x->y' bash\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    ping        send ping to uri\n")
	fmt.Fprintf(stderr, "        %v ping 'x->y'\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    state       show node state\n")
	fmt.Fprintf(stderr, "        %v state 'x->y'\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    shell       start shell which forwaring conn to uri\n")
	fmt.Fprintf(stderr, "        %v shell 'x->y' http_proxy,https_proxy bash\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    ssh         start ssh to uri\n")
	fmt.Fprintf(stderr, "        %v ssh 'x->y' -l root\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    scp         start scp to uri\n")
	fmt.Fprintf(stderr, "        %v scp 'x->y' root@bshost:/tmp/xx /tmp/\n", fn)
	fmt.Fprintf(stderr, "\n")
	fmt.Fprintf(stderr, "    sftp        start sftp to uri\n")
	fmt.Fprintf(stderr, "        %v sftp 'x->y' root@bshost:/tmp/xx\n", fn)
	fmt.Fprintf(stderr, "\n")
}

func main() {
	runall(os.Args...)
}

var env []string

func runall(osArgs ...string) {
	for _, arg := range osArgs {
		if arg == "-v" {
			fmt.Printf("%v\n", Version)
			return
		}
		if arg == "-h" || arg == "--help" {
			usage()
			return
		}
	}
	var command string
	var args []string
	var slaverURI string
	runner, err := os.Executable()
	if err != nil {
		fmt.Printf("%v\n", err)
		exit(1)
		return
	}
	dir := filepath.Dir(runner)
	_, fn := filepath.Split(osArgs[0])
	fn = strings.TrimSuffix(fn, ".exe")
	args = osArgs[1:]

	if strings.HasPrefix(fn, "bs-") {
		command = strings.TrimPrefix(fn, "bs-")
		args = args[:]
	} else {
		if len(args) < 1 {
			usage()
			exit(1)
			return
		}
		command = args[0]
		args = args[1:]
	}

	if len(args) > 0 && (strings.HasPrefix(args[0], "-slaver=") || strings.HasPrefix(args[0], "--slaver=")) {
		slaverURI = args[0]
		slaverURI = strings.TrimPrefix(slaverURI, "-slaver=")
		slaverURI = strings.TrimPrefix(slaverURI, "--slaver=")
		args = args[1:]
	}

	switch command {
	case "install":
		fmt.Printf("start install command\n")
		var err error
		filename, _ := filepath.Abs(osArgs[0])
		filedir, _ := filepath.Split(filename)
		err = mklink(filepath.Join(filedir, "bs-conn"), filename)
		if err != nil {
			exit(1)
		}
		fmt.Printf("link bs-conn success\n")
		err = mklink(filepath.Join(filedir, "bs-forward"), filename)
		if err != nil {
			exit(1)
		}
		fmt.Printf("link bs-forward success\n")
		err = mklink(filepath.Join(filedir, "bs-proxy"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-proxychains"), filename)
		if err != nil {
			exit(1)
		}
		fmt.Printf("link bs-proxychains success\n")
		err = mklink(filepath.Join(filedir, "bs-ping"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-state"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-shell"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-chrome"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-scp"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-sftp"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-ssh"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-ssh-copy-id"), filename)
		if err != nil {
			exit(1)
		}
		err = mklink(filepath.Join(filedir, "bs-host"), filename)
		if err != nil {
			exit(1)
		}
		fmt.Printf("Install is done\n")
		return
	case "uninstall":
		fmt.Printf("start uninstall command\n")
		filename, _ := filepath.Abs(osArgs[0])
		filedir, _ := filepath.Split(filename)
		removeFile(filepath.Join(filedir, "bs-conn"))
		removeFile(filepath.Join(filedir, "bs-forward"))
		removeFile(filepath.Join(filedir, "bs-proxy"))
		removeFile(filepath.Join(filedir, "bs-proxychains"))
		removeFile(filepath.Join(filedir, "bs-ping"))
		removeFile(filepath.Join(filedir, "bs-state"))
		removeFile(filepath.Join(filedir, "bs-shell"))
		removeFile(filepath.Join(filedir, "bs-chrome"))
		removeFile(filepath.Join(filedir, "bs-scp"))
		removeFile(filepath.Join(filedir, "bs-sftp"))
		removeFile(filepath.Join(filedir, "bs-ssh"))
		removeFile(filepath.Join(filedir, "bs-ssh-copy-id"))
		removeFile(filepath.Join(filedir, "bs-host"))
		fmt.Printf("Uninstall is done\n")
		return
	case "version":
		fmt.Println(Version)
		return
	case "help":
		usage()
		exit(1)
		return
	}
	//load slaver address
	if len(slaverURI) < 1 {
		slaverURI = os.Getenv("BS_CONSOLE_URI")
	}
	var console *router.Console
	if len(slaverURI) < 1 {
		var err error
		var data []byte
		var path string
		u, _ := user.Current()
		for _, path = range []string{"./.bsrouter.json", "./bsrouter.json", u.HomeDir + "/.bsrouter/bsrouter.json", u.HomeDir + "/.bsrouter.json", "/etc/bsrouter/bsrouter.json", "/etc/bsrouter.json"} {
			data, err = os.ReadFile(path)
			if err == nil {
				fmt.Printf("bsconsole using config %v\n", path)
				break
			}
		}
		if err != nil {
			fmt.Fprintf(stderr, "read config from .bsrouter.json or ~/.bsrouter/bsrouter.json  or ~/.bsrouter.json or /etc/bsrouter/bsrouter.json or /etc/bsrouter.json fail with %v\n", err)
			exit(1)
		}
		var config router.Config
		err = json.Unmarshal(data, &config)
		if err != nil {
			fmt.Fprintf(stderr, "parse config fail with %v\n", err)
			exit(1)
		}
		slaverURI = config.ConsoleURI()
		console = router.NewConsoleByConfig(&config)
	} else {
		if !strings.Contains(slaverURI, "://") {
			slaverURI = "socks5://" + slaverURI
		}
		console = router.NewConsole(slaverURI)
	}
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	console.Env = env
	if hostFile := os.Getenv("BS_REWRITE_HOSTS"); len(hostFile) > 0 {
		err = console.Rewrite.Read(hostFile)
		if err != nil {
			fmt.Fprintf(stderr, "read hosts file %v fail with %v\n", hostFile, err)
			exit(1)
		}
		fmt.Printf("Console using hosts rewrite from %v\n", hostFile)
	}
	defer console.Close()
	switch command {
	case "conn":
		if len(args) < 1 {
			fmt.Fprintf(stderr, "uri is not setted\n")
			usage()
			exit(1)
			return
		}
		fullURI := args[0]
		fullURI = strings.Trim(fullURI, "'\"")
		err = console.Redirect(fullURI, stdin, stdout, xio.CloserF(func() (err error) {
			sig <- syscall.SIGABRT
			return
		}))
		if err != nil {
			exit(1)
		}
		<-sig
		console.Close()
	case "forward":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "local/uri is not setting\n")
			usage()
			exit(1)
			return
		}
		proxy.SetLogLevel(40)
		router.SetLogLevel(40)
		n := len(args) / 2
		for i := 0; i < n; i++ {
			locAddr := args[2*i]
			fullURI := args[2*i+1]
			_, err = console.StartForward(locAddr, fullURI)
			if err != nil {
				fmt.Printf("start forward fail with %v\n", err)
				exit(1)
			}
		}
		<-sig
		console.Close()
	case "proxy":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "uri/local is not setting\n")
			usage()
			exit(1)
			return
		}
		proxy.SetLogLevel(40)
		router.SetLogLevel(40)
		fullURI := args[0]
		fullURI = strings.Trim(fullURI, "'\"")
		if !strings.HasSuffix(fullURI, "tcp://${HOST}") && !strings.HasSuffix(fullURI, "${URI}") {
			fullURI += "->tcp://${HOST}"
		}
		locAddr := args[1]
		server, listener, err := console.StartProxy(locAddr, fullURI)
		if err != nil {
			fmt.Printf("Proxy done with %v\n", err)
			exit(1)
		}
		<-sig
		server.Close()
		listener.Close()
		console.Close()
	case "proxychains":
		if len(args) < 2 {
			fmt.Fprintf(stderr, "uri/runner is not setting\n")
			usage()
			exit(1)
			return
		}
		fullURI := args[0]
		fullURI = strings.Trim(fullURI, "'\"")
		if !strings.HasSuffix(fullURI, "tcp://${HOST}") && !strings.HasSuffix(fullURI, "${URI}") {
			fullURI += "->tcp://${HOST}"
		}
		var tempFile *os.File
		err = console.ProxyExec(fullURI, stdin, stdout, stderr, func(listener net.Listener) (env []string, runnerName string, runnerArgs []string, err error) {
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			tempFile, err = ioutil.TempFile("", "proxychains-*.conf")
			if err != nil {
				return
			}
			tempFile.WriteString(fmt.Sprintf(proxyChainsConf, port))
			tempFile.Close()
			runnerName = "proxychains4"
			runnerArgs = append(runnerArgs, "-q", "-f", tempFile.Name())
			runnerArgs = append(runnerArgs, args[1:]...)
			return
		})
		if tempFile != nil {
			os.Remove(tempFile.Name())
		}
		fmt.Printf("Proxychains done with %v\n", err)
		if err != nil {
			exit(1)
		}
	case "ping":
		fullURI := ""
		if len(args) > 0 {
			fullURI = args[0]
		}
		fullURI = strings.Trim(fullURI, "'\"")
		max := uint64(0)
		if len(args) > 1 {
			max, _ = strconv.ParseUint(args[1], 10, 64)
		}
		go func() {
			<-sig
			console.Close()
		}()
		err = console.Ping(fullURI, time.Second, max)
		if err != nil {
			exit(1)
		}
	case "state":
		var fullURI, query string
		if len(args) > 1 {
			fullURI = args[0]
			query = args[1]
		} else if len(args) > 0 {
			fullURI = args[0]
		}
		fullURI = strings.Trim(fullURI, "'\"")
		err = console.PrintState(fullURI, query)
		if err != nil {
			fmt.Printf("Print state done with %v\n", err)
			exit(1)
		}
	case "shell":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "key/runner is not setting\n")
			usage()
			exit(1)
			return
		}
		fullURI := args[0]
		fullURI = strings.Trim(fullURI, "'\"")
		if !strings.HasSuffix(fullURI, "tcp://${HOST}") && !strings.HasSuffix(fullURI, "${URI}") {
			fullURI += "->tcp://${HOST}"
		}
		err = console.ProxyExec(fullURI, stdin, stdout, stderr, func(listener net.Listener) (env []string, runnerName string, runnerArgs []string, err error) {
			keys := args[1]
			keys = strings.ReplaceAll(keys, "${HOST}", listener.Addr().String())
			env = strings.Split(keys, ",")
			for i, e := range env {
				if e == "http_proxy" || e == "https_proxy" || e == "HTTP_PROXY" || e == "HTTPS_PROXY" {
					env[i] = fmt.Sprintf("%v=http://%v", e, listener.Addr())
				} else if e == "socks_proxy" || e == "SOCKS_PROXY" {
					env[i] = fmt.Sprintf("%v=socks5://%v", e, listener.Addr())
				}
			}
			runnerName = args[2]
			for _, arg := range args[3:] {
				runnerArgs = append(runnerArgs, strings.ReplaceAll(arg, "${PROXY_HOST}", listener.Addr().String()))
			}
			return
		})
		fmt.Printf("Shell done with %v\n", err)
		if err != nil {
			exit(1)
		}
	case "ssh", "scp", "sftp", "ssh-copy-id":
		fullURI := ""
		fullArgs := args
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			fullURI = args[0]
			fullArgs = args[1:]
		}
		fullURI = strings.Trim(fullURI, "'\"")
		proxyCommand := filepath.Join(dir, "bsconsole")
		if runtime.GOOS == "windows" {
			proxyCommand += ".exe"
		}
		proxyCommand += fmt.Sprintf(" conn --slaver=%v '${URI}'", slaverURI)
		go func() {
			<-sig
			console.Close()
		}()
		err = console.ProxySSH(fullURI, stdin, stdout, stderr, proxyCommand, command, fullArgs...)
		if err != nil {
			exit(1)
		}
	case "chrome":
		if len(args) < 1 {
			fmt.Fprintf(stderr, "uri is not setting\n")
			usage()
			exit(1)
			return
		}
		proxy.SetLogLevel(40)
		router.SetLogLevel(40)
		fullURI := args[0]
		fullSHA := xhash.SHA1([]byte(os.Getenv("BS_REWRITE_HOSTS") + "-" + fullURI))
		fullURI = strings.Trim(fullURI, "'\"")
		if !strings.HasSuffix(fullURI, "tcp://${HOST}") && !strings.HasSuffix(fullURI, "${URI}") {
			fullURI += "->tcp://${HOST}"
		}
		runnerPath := ""
		runnerPath, err = exec.LookPath("./chrome")
		if err != nil {
			runnerPath, err = exec.LookPath("chrome")
		}
		if err != nil {
			runnerPath, err = exec.LookPath("./google-chrome")
		}
		if err != nil {
			runnerPath, err = exec.LookPath("google-chrome")
		}
		if err != nil {
			runnerPath, err = exec.LookPath("./Google Chrome")
		}
		if err != nil {
			runnerPath, err = exec.LookPath("Google-Chrome")
		}
		if err != nil && runtime.GOOS == "windows" {
			runnerPath, err = exec.LookPath(`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`)
		}
		if err != nil && runtime.GOOS == "windows" {
			runnerPath, err = exec.LookPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`)
		}
		if err != nil && runtime.GOOS == "darwin" {
			runnerPath, err = exec.LookPath(`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`)
		}
		if err != nil {
			fmt.Printf("Chrome search google chrome fail, add it to path\n")
			exit(1)
			return
		}
		dataDir, _ := os.UserHomeDir()
		dataDir = filepath.Join(dataDir, ".bsrouter", "cache", fullSHA)
		err = os.MkdirAll(dataDir, os.ModePerm)
		if err != nil {
			fmt.Printf("create datadir on %v fail with %v\n", dataDir, err)
			exit(1)
			return
		}
		fmt.Printf("Chrome using google chrome on %v, datadir on %v\n", runnerPath, dataDir)
		go func() {
			<-sig
			console.Close()
		}()
		err = console.ProxyProcess(fullURI, stdin, stdout, stderr, func(listener net.Listener) (env []string, runnerName string, runnerArgs []string, err error) {
			fmt.Printf("Chrome proxy all to %v\n\n\n", listener.Addr())
			runnerName = runnerPath
			runnerArgs = append(runnerArgs, fmt.Sprintf("--proxy-server=socks5://%v", listener.Addr()))
			runnerArgs = append(runnerArgs, fmt.Sprintf("--proxy-bypass-list=\"%v\"", "<-loopback>"))
			runnerArgs = append(runnerArgs, fmt.Sprintf("--user-data-dir=%v", dataDir))
			runnerArgs = append(runnerArgs, args[1:]...)
			return
		})
		fmt.Printf("Chrome done with %v\n", err)
		if err != nil {
			exit(1)
		}
	case "host", "host.service":
		if len(args) > 0 && len(args) < 2 {
			fmt.Printf("[name] <net address> <uri> is required\n")
			return
		}

		device, err := water.New(water.Config{
			DeviceType: water.TUN,
		})
		if err != nil {
			fmt.Printf("create tun device error %v\n", err)
			exit(1)
		}
		defer device.Close()
		fmt.Printf("Host create tun device %v success\n", device.Name())

		ifac := util.NewInterface(device.Name())
		ifac.Log = true
		gw := tun2conn.NewGateway(device, "10.0.0.1/24", "")
		gw.Dialer = console

		hostMap := router.NewHostMap()

		addForward := func(args ...string) {
			var item *router.HostItem
			if len(args) == 2 {
				item, err = hostMap.AddForward(args[0], args[0], args[1])
			} else {
				item, err = hostMap.AddForward(args[0], args[1], args[2])
			}
			if err != nil {
				fmt.Printf("Host add forward fail with %v by %v\n", err, args)
			} else {
				fmt.Printf("Host add forward susscess by %v\n", args)
				ifac.AddAddress(item.LocalAddr.String(), item.LocalMask, item.LocalGW.String())
			}
		}
		removeForward := func(args ...string) {
			item := hostMap.RemoveForward(args[0])
			if item == nil {
				fmt.Printf("Host remove forward fail by %v\n", args)
			} else {
				fmt.Printf("Host remove forward susscess by %v\n", args)
				ifac.RemoveAddress(item.LocalAddr.String(), item.LocalMask, item.LocalGW.String())
			}
		}
		onCommand := func(cmd string, args ...string) {
			switch cmd {
			case "@add":
				if len(args) < 2 {
					fmt.Printf("Host add forward fail with invalid cmd @add by %v\n", args)
					return
				}
				addForward(args...)
			case "@rm", "@remove":
				if len(args) < 1 {
					fmt.Printf("Host remove fail with invalid cmd %v by %v\n", cmd, args)
					return
				}
				removeForward(args...)
			}
		}
		if command == "host.service" {
			go loopCommand(console, "HostForward.Service", onCommand)
		} else {
			go procCommand(os.Stdin, onCommand)
		}
		if len(args) > 1 {
			addForward(args...)
		}
		gw.Policy = func(on string, ip net.IP, port uint16, domain, cname string, questions []string) (uri string, newIP net.IP, newPort uint16) {
			uri, remote := hostMap.Match(ip)
			if on == "udp" {
				newIP = remote
				newPort = port
			} else {
				if len(uri) > 0 {
					uri += "->"
				}
				uri += fmt.Sprintf("tcp://%v:%v", remote, port)
			}
			return
		}
		err = gw.Start()
		if err != nil {
			fmt.Printf("start gateway error %v\n", err)
			exit(1)
		}
		defer gw.Stop()
		<-sig
	default:
		fmt.Fprintf(stderr, "%v is not supported\n", command)
		usage()
		exit(1)
	}
}

func mklink(link, target string) (err error) {
	var runner string
	var args []string
	if runtime.GOOS == "windows" {
		runner = "cmd"
		args = []string{"/c", "mklink", link + ".exe", target}
	} else {
		runner = "ln"
		args = []string{"-s", target, link}
	}
	cmd := exec.Command(runner, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		fmt.Printf("link %v %v fail with %v\n", link, target, err)
	}
	return
}

func removeFile(target string) (err error) {
	fmt.Printf("remove %v\n", target)
	os.Remove(target)
	os.Remove(target + ".exe")
	return
}

func loopCommand(console *router.Console, name string, on func(cmd string, args ...string)) {
	for {
		conn, err := console.Dial("tcp://" + name)
		if err != nil {
			fmt.Printf("dial to cmd channel fail with %v\n", err)
			time.Sleep(3 * time.Second)
			continue
		}
		fmt.Printf("dial to cmd channel fail with %v\n", err)
		err = procCommand(conn, on)
		fmt.Printf("read cmd channel fail with %v\n", err)
	}
}

func procCommand(input io.Reader, on func(cmd string, args ...string)) (err error) {
	reader := bufio.NewReader(input)
	cmdRegex := regexp.MustCompile(`@[^\s]+\s+`)
	argRegex := regexp.MustCompile(`\s+`)
	for {
		line, xerr := reader.ReadString('\n')
		if xerr != nil {
			err = xerr
			break
		}
		line = strings.TrimSpace(line)
		if !cmdRegex.MatchString(line) {
			continue
		}
		args := argRegex.Split(line, -1)
		on(args[0], args[1:]...)
	}
	return
}
