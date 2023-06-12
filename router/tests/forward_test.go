package main

import (
	"fmt"
	"net"
	"testing"
)

func TestDns(t *testing.T) {
	conn, err := net.Dial("udp", "192.168.1.1:53")
	if err != nil {
		panic(err)
	}
	data := []byte{220, 195, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 119, 119, 119, 5, 98, 97, 105, 100, 117, 3, 99, 111, 109, 0, 0, 1, 0, 1}
	conn.Write(data)
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		panic(err)
	}
	fmt.Printf("buf-->%v\n", buf[0:n])
}
