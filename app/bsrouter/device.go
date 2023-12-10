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
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
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

func SetupFileDevice(fd int) {
	globalDevice = tun2conn.NewFileDevice(uintptr(fd), "Gateway")
	router.InfoLog("setup file device %v success", fd)
}

func SetupUdpDeivce(addr string) (res Result) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		lastError = err
		res = newCodeResult(-1, err.Error())
		router.ErrorLog("setup udp device %v fail with %v", addr, err)
		return
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	globalDevice = tun2conn.NewPacketConnDevice(conn)
	res = newIntResult(port)
	router.InfoLog("create udp device %v success", conn.LocalAddr())
	return
}

func SetupTunDevice() (res Result) {
	device, err := water.New(water.Config{
		DeviceType: water.TUN,
	})
	if err != nil {
		lastError = err
		res = newCodeResult(-1, err.Error())
		router.ErrorLog("setup tun device fail with %v", err)
		return
	}
	globalDevice = device
	prepareDevice = prepareTunDevice
	resetDevice = resetTunDevice
	res = newStringResult(device.Name())
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

func ReleaseDevice() (res Result) {
	if globalDevice == nil {
		res = newCodeResult(-1, "not setup")
		return
	}
	err := globalDevice.Close()
	if err == nil {
		res = newCodeResult(0, "OK")
	} else {
		res = newCodeResult(-1, err.Error())
	}
	return
}

type Sender interface {
	Send([]byte)
	Done()
}

type tunConn struct {
	out    Sender
	buffer chan *stack.PacketBuffer
}

func newTunConn(out Sender) (conn *tunConn) {
	conn = &tunConn{
		out:    out,
		buffer: make(chan *stack.PacketBuffer, 64),
	}
	return
}

func (w *tunConn) Send(p []byte) {
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(p),
	})
	select {
	case w.buffer <- pkt:
	default:
	}
}

func (w *tunConn) Done() {
	w.Close()
}

func (w *tunConn) ReadPacket() (pkt *stack.PacketBuffer, err error) {
	pkt = <-w.buffer
	if pkt == nil {
		err = fmt.Errorf("closed")
	}
	return
}

func (w *tunConn) Read(p []byte) (n int, err error) {
	panic("not supported")
}

func (w *tunConn) Write(p []byte) (n int, err error) {
	w.out.Send(p)
	n = len(p)
	return
}

func (w *tunConn) Close() (err error) {
	select {
	case w.buffer <- nil:
	default:
	}
	w.out.Done()
	return
}
