package bsck

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
)

type Console struct {
	Client        *xhttp.Client
	SlaverAddress string
}

func NewConsole() (console *Console) {
	console = &Console{}
	client := &http.Client{
		Transport: &http.Transport{
			Dial: console.dialNet,
		},
	}
	console.Client = xhttp.NewClient(client)
	return
}

func (c *Console) dialAll(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	conn, err := net.Dial("tcp", c.SlaverAddress)
	if err == nil {
		buf := make([]byte, 1024*64)
		_, err = conn.Write([]byte{0x05, 0x01, 0x00})
		if err != nil {
			return
		}
		_, err = conn.Read(buf)
		buf[0], buf[1], buf[2], buf[3] = 0x05, 0x01, 0x00, 0x13
		buf[4] = byte(len(uri))
		copy(buf[5:], []byte(uri))
		binary.BigEndian.PutUint16(buf[5+len(uri):], 0)
		_, err = conn.Write(buf[:buf[4]+7])
		if err != nil {
			return
		}
		_, err = conn.Read(buf)
		if err != nil {
			return
		}
		if buf[1] != 0x00 {
			err = fmt.Errorf("connection fail")
		}
	}
	return
}

//dialNet is net dialer to router
func (c *Console) dialNet(network, addr string) (conn net.Conn, err error) {
	conn, raw, err := dialer.CreatePipedConn()
	if err == nil {
		addr = strings.TrimSuffix(addr, ":80")
		addr = strings.TrimSuffix(addr, ":443")
		_, err = c.dialAll(addr, raw)
	}
	return
}

func (c *Console) RedirectStd(uri string) (err error) {
	raw := xio.NewCombinedReadWriteCloser(os.Stdin, os.Stdout, nil)
	_, err = c.dialAll(uri, raw)
	return
}

func (c *Console) Dial(uri string) (conn io.ReadWriteCloser, err error) {
	conn, raw, err := dialer.CreatePipedConn()
	if err == nil {
		_, err = c.dialAll(uri, raw)
	}
	return
}
