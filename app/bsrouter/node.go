package bsrouter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/util/xcrypto"
	"github.com/codingeasygo/util/xio"
)

var globalDialer xio.PiperDialer
var globalConsole *router.Console
var globalService *router.Service
var globalWhiltelist []string

func StartConsole(config string) (whiltelist []string, result string) {
	if len(globalWorkDir) < 1 {
		result = "not boot"
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
		result = err.Error()
		lastError = err
		return
	}
	conf.Dir = globalWorkDir
	for _, ip := range conf.ResolveWhitelist() {
		whiltelist = append(whiltelist, ip.String())
	}
	cli := router.NewConsoleByConfig(conf)
	err = cli.Ping("tcp://echo", time.Second, 1)
	if err != nil {
		router.ErrorLog("Gateway ping to router error %v", err)
		result = err.Error()
		lastError = err
		return
	}
	globalDialer = cli
	globalConsole = cli
	globalWhiltelist = whiltelist
	result = "OK"
	return
}

func StopConsole() {
	if globalConsole != nil {
		globalConsole = nil
		globalDialer = nil
		globalWhiltelist = nil
	}
}

func StartNode(config string) (whiltelist []string, result string) {
	if len(globalWorkDir) < 1 {
		result = "not boot"
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
		result = err.Error()
		return
	}
	conf.Dir = globalWorkDir
	for _, ip := range conf.ResolveWhitelist() {
		whiltelist = append(whiltelist, ip.String())
	}
	if _, xerr := os.Stat(filepath.Join(conf.Dir, "bsrouter.key")); os.IsNotExist(xerr) {
		rootCert, rootKey, rootCertPEM, rootKeyPEM, xerr := xcrypto.GenerateRootCA([]string{"bsrouter"}, "bsrouter", 2048)
		if xerr != nil {
			lastError = xerr
			result = xerr.Error()
			return
		}
		_, _, certPEM, keyPEM, xerr := xcrypto.GenerateCert(rootCert, rootKey, nil, []string{"bsrouter"}, "bsrouter", []string{"bsrouter"}, nil, 2048)
		if xerr != nil {
			lastError = xerr
			result = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "rootCA.key"), rootKeyPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			result = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "rootCA.pem"), rootCertPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			result = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "bsrouter.key"), keyPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			result = xerr.Error()
			return
		}
		xerr = os.WriteFile(filepath.Join(conf.Dir, "bsrouter.pem"), certPEM, os.ModePerm)
		if xerr != nil {
			lastError = xerr
			result = xerr.Error()
			return
		}
		router.InfoLog("Node create cert on %v by bsrouter.key/bsrouter.pem", conf.Dir)
	}
	service := router.NewService()
	service.Config = conf
	xerr := service.Start()
	if xerr != nil {
		lastError = xerr
		result = xerr.Error()
		return
	}
	globalDialer = service
	globalService = service
	globalWhiltelist = whiltelist
	result = "OK"
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
