package dialer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/codingeasygo/util/xio/frame"
	"github.com/codingeasygo/util/xmap"
)

func TestUDPGW(t *testing.T) {
	//ipv4
	lv4, _ := net.ListenUDP("udp4", nil)
	addrv4 := lv4.LocalAddr().(*net.UDPAddr)
	datav4 := make([]byte, 1024)
	lenv4 := 0
	binary.BigEndian.PutUint16(datav4[1:], 1)
	lenv4 += 3
	lenv4 += copy(datav4[lenv4:], addrv4.IP)
	binary.BigEndian.PutUint16(datav4[lenv4:], uint16(addrv4.Port))
	lenv4 += 2
	lenv4 += copy(datav4[lenv4:], []byte("abc"))
	go func() {
		buf := make([]byte, 1024)
		for {
			n, from, _ := lv4.ReadFromUDP(buf)
			lv4.WriteToUDP(buf[0:n], from)
		}
	}()
	//ipv6
	lv6, _ := net.ListenUDP("udp6", nil)
	addrv6 := lv6.LocalAddr().(*net.UDPAddr)
	datav6 := make([]byte, 1024)
	lenv6 := 0
	datav6[0] = UDPGW_CLIENT_FLAG_IPV6
	binary.BigEndian.PutUint16(datav6[1:], 2)
	lenv6 += 3
	lenv6 += copy(datav6[lenv6:], addrv6.IP)
	binary.BigEndian.PutUint16(datav6[lenv6:], uint16(addrv6.Port))
	lenv6 += 2
	lenv6 += copy(datav6[lenv6:], []byte("abc"))
	go func() {
		buf := make([]byte, 1024)
		for {
			n, from, _ := lv6.ReadFromUDP(buf)
			lv6.WriteToUDP(buf[0:n], from)
		}
	}()
	dialer := NewUdpGwDialer()
	dialer.Delay = 10 * time.Millisecond
	dialer.Bootstrap(xmap.M{})
	gw, err := dialer.Dial(nil, 1, "")
	if err != nil {
		t.Error(err)
		return
	}
	rwc := frame.NewReadWriteCloser(dialer.Header, gw, dialer.MTU+8)

	//ipv4
	for i := 0; i < 100; i++ {
		binary.BigEndian.PutUint16(datav4[1:], uint16(i))
		_, err := rwc.Write(datav4[0:lenv4])
		if err != nil {
			t.Error(err)
			return
		}
		buf := make([]byte, 2048)
		n, _ := rwc.Read(buf)
		if !bytes.Equal(buf[:n], datav4[:lenv4]) {
			fmt.Printf("back->%v,%v\n", buf[:n], datav4[:lenv4])
			t.Error("error")
			return
		}
	}

	//ipv6
	buf := make([]byte, 2048)
	rwc.Write(datav6[0:lenv6])
	n, _ := rwc.Read(buf)
	if !bytes.Equal(buf[:n], datav6[:lenv6]) {
		fmt.Printf("back->%v,%v\n", buf[:n], datav6[:lenv6])
		t.Error("error")
		return
	}

	rwc.Close()
	dialer.Shutdown()
}
