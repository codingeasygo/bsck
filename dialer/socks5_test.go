package dialer

import (
	"fmt"
	"net"
	"testing"

	"github.com/Centny/gwf/util"
)

type ErroPool string

func (e ErroPool) Get(uri string) (address string, err error) {
	err = fmt.Errorf("test error")
	return
}

func (e ErroPool) Done(address, uri string, err error) {
	return
}

func TestSocksProxy(t *testing.T) {
	dailer := NewSocksProxyDialer()
	dailer.Bootstrap(util.Map{
		"id":      "testing",
		"address": "127.0.0.1:1080",
	})
	remote := "tcp://www.google.com:80"
	if !dailer.Matched(remote) {
		t.Error("error")
		return
	}
	raw, err := dailer.Dial(100, remote, nil)
	if err != nil {
		t.Error(err)
		return
	}
	raw.Close()
	cona, conb, _ := CreatePipedConn()
	_, err = dailer.Dial(100, remote, conb)
	if err != nil {
		t.Error(err)
		return
	}
	cona.Close()
	//
	//for cover
	dailer.Name()
	dailer.Options()
	//
	//test error
	_, err = dailer.Dial(10, "%AX", nil)
	if err == nil {
		t.Error(err)
		return
	}
	_, err = dailer.Dial(10, "tcp://abc", nil)
	if err == nil {
		t.Error(err)
		return
	}
	_, err = dailer.Dial(10, "tcp://abc:abc", nil)
	if err == nil {
		t.Error(err)
		return
	}
	//bootstrap error
	dailer2 := NewSocksProxyDialer()
	err = dailer2.Bootstrap(util.Map{})
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Printf("-->%v\n", dailer)
	//server error
	dailer2 = NewSocksProxyDialer()
	err = dailer2.Bootstrap(util.Map{
		"id":      "testing",
		"address": "127.0.0.1:12210",
	})
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
	//read/write error
	l, err := net.Listen("tcp", ":12210")
	mode := 0
	assert(err == nil)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				break
			}
			switch mode {
			case 0:
				conn.Close()
			case 1:
				conn.Write([]byte{0x05, 0x09})
				conn.Close()
			case 2:
				conn.Write([]byte{0x05, 0x00})
				conn.Close()
			case 3:
				conn.Write([]byte{0x05, 0x00})
				conn.Write([]byte{0x05, 0x10, 0x00, 0x03, 0x03, 'a', 'b', 'c', 0x00, 0x20})
				conn.Close()
			case 4:
				conn.Write([]byte{0x05, 0x00})
				conn.Write([]byte{0x05, 0x01, 0x00, 0x04,
					0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
					0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
					0x00, 0x20})
				conn.Close()
			case 5:
				conn.Write([]byte{0x05, 0x00})
				conn.Write([]byte{0x05, 0x01, 0x00, 0x05, 0x03, 'a', 'b', 'c', 0x00, 0x20})
				conn.Close()
			}
		}
	}()
	mode = 0
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
	mode = 1
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
	mode = 2
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
	mode = 3
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
	mode = 4
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
	mode = 5
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
	//
	//test pool error
	dailer2 = NewSocksProxyDialer()
	dailer2.Pooler = ErroPool("")
	_, err = dailer2.Dial(10, "tcp://127.0.0.1:100", nil)
	if err == nil {
		t.Error(err)
		return
	}
}

func TestCodeError(t *testing.T) {
	err := &CodeError{Inner: fmt.Errorf("error"), ByteCode: 0x01}
	if err.Error() != "error" || err.Code() != 0x01 {
		t.Error("error")
	}
}
