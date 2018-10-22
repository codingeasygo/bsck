package dialer

import (
	"fmt"
	"testing"

	"github.com/Centny/gwf/util"
)

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
	dailer.Name()
	dailer.Options()
	//
	//test error
	dailer = NewSocksProxyDialer()
	err = dailer.Bootstrap(util.Map{})
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Printf("-->%v\n", dailer)
}
