package bsrouter

import (
	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn"
)

var globalGateway *tun2conn.Gateway

func SwitchProxyMode(mode string) (result string) {
	if globalGateway == nil {
		result = "gateway is not running"
		return
	}
	globalGateway.Mode = tun2conn.ProxyMode(mode)
	return
}

func StartGateway(netAddr, gwAddr, gwDNS string) (result string) {
	if len(globalWorkDir) < 1 {
		result = "not boot"
		return
	}
	if globalDialer == nil {
		result = "node is not setup"
		return
	}
	if prepareDevice != nil {
		err := prepareDevice(netAddr, 24, gwAddr, globalWhiltelist)
		if err != nil {
			router.ErrorLog("Gateway prepare device error %v", err)
			lastError = err
			result = err.Error()
			return
		}
	}
	gw := tun2conn.NewGateway(globalDevice, gwAddr+"/24", gwDNS)
	gw.Cache = globalWorkDir
	gw.GFW = globalGFW
	gw.Dialer = globalDialer
	err := gw.Start()
	if err != nil {
		router.ErrorLog("Gateway start error %v", err)
		lastError = err
		result = err.Error()
		return
	}
	globalGateway = gw
	result = "OK"
	return
}

func StopGateway() (message string) {
	if globalGateway != nil {
		globalGateway.Stop()
	}
	if resetDevice != nil {
		resetDevice()
	}
	globalGateway = nil
	message = "OK"
	return
}
