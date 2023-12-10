package bsrouter

import (
	"net"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn"
)

var globalGateway *tun2conn.Gateway

func SwitchProxyMode(mode string) (res Result) {
	if globalGateway == nil {
		res = newCodeResult(-1, "gateway is not running")
		return
	}
	globalGateway.Mode = tun2conn.ProxyMode(mode)
	res = newCodeResult(0, "OK")
	return
}

func StartGateway(netAddr, gwAddr, gwDNS, channel, mode string) (res Result) {
	if globalGateway != nil {
		res = newCodeResult(0, "running")
		return
	}
	if len(globalWorkDir) < 1 {
		res = newCodeResult(-1, "not bootstrap")
		return
	}
	if globalDialer == nil {
		res = newCodeResult(-1, "node is not setup")
		return
	}
	if prepareDevice != nil {
		err := prepareDevice(netAddr, 24, gwAddr, globalWhiltelist)
		if err != nil {
			router.ErrorLog("Gateway prepare device error %v", err)
			lastError = err
			res = newCodeResult(-1, err.Error())
			return
		}
	}
	gw := tun2conn.NewGateway(globalDevice, gwAddr+"/24", gwDNS)
	gw.Channel = func(on string, ip net.IP, port uint16, domain, cname string, questions []string) string {
		return channel
	}
	gw.Mode = tun2conn.ProxyMode(mode)
	gw.Cache = globalWorkDir
	gw.GFW = globalGFW
	gw.Dialer = globalDialer
	err := gw.Start()
	if err != nil {
		router.ErrorLog("Gateway start error %v", err)
		lastError = err
		res = newCodeResult(-1, err.Error())
		return
	}
	globalGateway = gw
	res = newCodeResult(0, "OK")
	return
}

func StopGateway() (res Result) {
	if globalGateway != nil {
		globalGateway.Stop()
	}
	if resetDevice != nil {
		resetDevice()
	}
	globalGateway = nil
	res = newCodeResult(0, "OK")
	return
}
