package bsrouter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn/gfw"
	"github.com/codingeasygo/util/xcrypto"
	"github.com/codingeasygo/util/xio"
)

var globalDialer xio.PiperDialer
var globalConsole *router.Console
var globalService *router.Service
var globalWhiltelist []string

var schemeMatcher = regexp.MustCompile(`^[a-zA-z0-9]+://`)

func StartConsole(config string) (res Result) {
	if globalConsole != nil {
		res = newCodeResult(0, "running")
		return
	}
	if len(globalWorkDir) < 1 {
		res = newCodeResult(-1, "not bootstrap")
		return
	}
	var err error
	var console *router.Console
	var whiltelist []string
	if schemeMatcher.MatchString(config) {
		console = router.NewConsole(config)
		whiltelist, err = console.ListWhitelist()
		if err != nil {
			router.ErrorLog("Gateway list whitelist error %v by %v", err, config)
			lastError = err
			res = newCodeResult(-1, err.Error())
			return
		}
	} else {
		conf := &router.Config{}
		if strings.HasPrefix(config, "{") {
			err = json.Unmarshal([]byte(config), conf)
		} else {
			var data []byte
			data, err = os.ReadFile(config)
			if err == nil {
				err = json.Unmarshal(data, conf)
			}
		}
		if err != nil {
			router.ErrorLog("Gateway prase config error %v by \n %v", err, conf)
			lastError = err
			res = newCodeResult(-1, err.Error())
			return
		}
		conf.Dir = globalWorkDir
		for _, ip := range conf.ResolveWhitelist() {
			whiltelist = append(whiltelist, ip.String())
		}
		console = router.NewConsoleByConfig(conf)
	}
	err = console.Ping("tcp://echo", time.Second, 1)
	if err != nil {
		router.ErrorLog("Gateway ping to router error %v", err)
		lastError = err
		res = newCodeResult(-1, err.Error())
		return
	}
	globalDialer = console
	globalConsole = console
	globalWhiltelist = whiltelist
	res = newStringResult(strings.Join(whiltelist, ","))
	return
}

func StopConsole() {
	if globalConsole != nil {
		globalConsole = nil
		globalDialer = nil
		globalWhiltelist = nil
	}
}

func StartNode(config string) (res Result) {
	if globalService != nil {
		res = newCodeResult(0, "running")
		return
	}
	if len(globalWorkDir) < 1 {
		res = newCodeResult(-1, "not bootstrap")
		return
	}
	var err error
	conf := &router.Config{}
	if strings.HasPrefix(config, "{") {
		err = json.Unmarshal([]byte(config), conf)
	} else {
		var data []byte
		data, err = os.ReadFile(config)
		if err == nil {
			err = json.Unmarshal(data, conf)
		}
	}
	if err != nil {
		router.ErrorLog("Gateway prase config error %v by \n %v", err, conf)
		lastError = err
		res = newCodeResult(-1, err.Error())
		return
	}
	conf.Dir = globalWorkDir
	unixFile, _ := conf.ConsoleUnix()
	if len(unixFile) > 0 {
		os.RemoveAll(unixFile)
	}
	whiltelist := []string{}
	for _, ip := range conf.ResolveWhitelist() {
		whiltelist = append(whiltelist, ip.String())
	}
	if _, xerr := os.Stat(filepath.Join(conf.Dir, "bsrouter.key")); os.IsNotExist(xerr) {
		rootCert, rootKey, rootCertPEM, rootKeyPEM, xerr := xcrypto.GenerateRootCA([]string{"bsrouter"}, "bsrouter", 2048)
		if xerr != nil {
			lastError = xerr
			res = newCodeResult(-1, err.Error())
			return
		}
		_, _, certPEM, keyPEM, xerr := xcrypto.GenerateCert(rootCert, rootKey, nil, []string{"bsrouter"}, "bsrouter", []string{"bsrouter"}, nil, 2048)
		if xerr != nil {
			lastError = xerr
			res = newCodeResult(-1, err.Error())
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "rootCA.key"), rootKeyPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			res = newCodeResult(-1, err.Error())
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "rootCA.pem"), rootCertPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			res = newCodeResult(-1, err.Error())
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "bsrouter.key"), keyPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			res = newCodeResult(-1, err.Error())
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "bsrouter.pem"), certPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			res = newCodeResult(-1, err.Error())
			return
		}
		router.InfoLog("Node create cert on %v by bsrouter.key/bsrouter.pem", conf.Dir)
	}
	service := router.NewService()
	service.Config = conf
	service.GFW = gfw.Shared
	xerr := service.Start()
	if xerr != nil {
		lastError = xerr
		res = newCodeResult(-1, err.Error())
		return
	}
	globalDialer = service
	globalService = service
	globalWhiltelist = whiltelist
	res = newStringResult(strings.Join(whiltelist, ","))
	return
}

func StopNode() {
	if globalService != nil {
		globalService.Stop()
		globalService = nil
		globalDialer = nil
		globalWhiltelist = nil
	}
}
