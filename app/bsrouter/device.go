package bsrouter

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn"
	"github.com/codingeasygo/tun2conn/util"
	"github.com/songgao/water"
)

var globalDevice io.ReadWriteCloser
var prepareDevice func(netAddr string, netMask int, gwAddr string, whitelist []string) (err error)
var resetDevice func()

func SetupPipeDevice(out Sender) (in Sender) {
	conn := newTunConn(out)
	globalDevice = conn
	in = conn
	router.InfoLog("setup pipe device success")
	return
}

func SetupFileDevice(fd uintptr) {
	globalDevice = tun2conn.NewFileDevice(fd, "Gateway")
	router.InfoLog("setup file device %v success", fd)
}

func SetupUdpDeivce(addr string) (
	res struct {
		Result string
		Port   int
	},
) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		lastError = err
		res.Result = err.Error()
		router.ErrorLog("setup udp device %v fail with %v", addr, err)
		return
	}
	res.Port = conn.LocalAddr().(*net.UDPAddr).Port
	globalDevice = tun2conn.NewPacketConnDevice(conn)
	res.Result = "OK"
	router.InfoLog("create udp device %v success", conn.LocalAddr())
	return
}

func SetupTunDevice() (
	res struct {
		Name   string
		Result string
	},
) {
	device, err := water.New(water.Config{
		DeviceType: water.TUN,
	})
	if err != nil {
		lastError = err
		res.Result = err.Error()
		router.ErrorLog("setup tun device fail with %v", err)
		return
	}
	res.Name = device.Name()
	res.Result = "OK"
	globalDevice = device
	prepareDevice = prepareTunDevice
	resetDevice = resetTunDevice
	router.InfoLog("setup device %v success", device.Name())
	return
}

var tunNetwork *util.Network
var tunWhitelist []string

func prepareTunDevice(netAddr string, netMask int, gwAddr string, whitelist []string) (err error) {
	device, ok := globalDevice.(*water.Interface)
	if !ok {
		err = fmt.Errorf("device %v is not tun", reflect.TypeOf(device))
		return
	}
	tunNetwork, err = util.LoadNetwork(device.Name(), netAddr, netMask, gwAddr)
	if err != nil {
		err = fmt.Errorf("load network error %v", err)
		router.InfoLog("prepare device %v fail with %v", device.Name(), err)
		return
	}
	tunNetwork.Gateway = true
	router.InfoLog("prepare load network %v success", tunNetwork)
	err = tunNetwork.Setup()
	if err != nil {
		err = fmt.Errorf("setup network error %v", err)
		router.InfoLog("prepare device %v fail with %v", device.Name(), err)
		return
	}
	tunWhitelist = whitelist
	for _, w := range whitelist {
		if strings.Contains(w, ":") { //ipv6
			continue
		}
		err = tunNetwork.AddRouter(w, 32)
		if err != nil {
			router.ErrorLog("prepare add router by %v fail with %v", w, err)
			break
		}
	}
	return
}

func resetTunDevice() {
	if tunNetwork == nil {
		return
	}
	for _, w := range tunWhitelist {
		if strings.Contains(w, ":") { //ipv6
			continue
		}
		err := tunNetwork.RemoveRouter(w, 32)
		if err != nil {
			router.WarnLog("reset remove router by %v fail with %v", w, err)
		}
	}
	err := tunNetwork.Reset()
	if err != nil {
		router.WarnLog("reset tun network by %v fail with %v", tunNetwork, err)
	}
	tunNetwork = nil
	tunWhitelist = nil
}

func ReleaseDevice() (result string) {
	if globalDevice == nil {
		result = "not setup"
		return
	}
	err := globalDevice.Close()
	if err == nil {
		result = "OK"
	} else {
		result = err.Error()
	}
	return
}

type Sender interface {
	Send([]byte)
	Done()
}

type tunConn struct {
	out        Sender
	readBuffer chan []byte
}

func newTunConn(out Sender) (conn *tunConn) {
	conn = &tunConn{
		out:        out,
		readBuffer: make(chan []byte, 64),
	}
	return
}

func (w *tunConn) Send(p []byte) {
	select {
	case w.readBuffer <- p:
	default:
	}
}

func (w *tunConn) Done() {
	w.Close()
}

func (w *tunConn) Read(p []byte) (n int, err error) {
	data := <-w.readBuffer
	if len(data) < 1 {
		err = fmt.Errorf("closed")
		return
	}
	n = copy(p, data)
	return
}

func (w *tunConn) Write(p []byte) (n int, err error) {
	w.out.Send(p)
	n = len(p)
	return
}

func (w *tunConn) Close() (err error) {
	select {
	case w.readBuffer <- nil:
	default:
	}
	w.out.Done()
	return
}
