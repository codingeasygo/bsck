package bsck

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
)

var configTest1 = `
{
    "name": "r0",
    "listen": ":5023",
    "web": {
        "listen": ":5024",
        "suffix": ".test.loc:5024",
        "auth": ""
    },
    "socks5": ":5081",
    "forwards": {
		"tx~tcp://:2332": "http://web?dir=/tmp"
	},
    "channels": [],
    "dialer":{
        "standard": 1
    }
}
`
var configTest2 = `
{
    "name": "r0",
    "listen": ":5023",
    "web": {
        "listen": ":5024",
        "suffix": ".test.loc:5024",
        "auth": ""
    },
    "socks5": ":5081",
    "forwards": {
        "t0~web://": "http://127.0.0.1:80",
        "t1~web://": "http://web?dir=/tmp",
        "t2~tcp://:2332": "http://web?dir=/tmp",
        "t3~socks://localhost:10322": "tcp://echo",
        "t4~rdp://localhost:0": "tcp://127.0.0.1:22",
		"t5~vnc://localhost:0": "tcp://127.0.0.1:22",
		"w1~web://": "tcp://echo?abc=1",
		"w2~web://": "tcp://echo"
    },
    "channels": [],
    "dialer":{
        "standard": 1
    }
}
`

func TestService(t *testing.T) {
	ioutil.WriteFile("/tmp/test.json", []byte(configTest1), os.ModePerm)
	defer os.Remove("/tmp/test.json")
	service := NewService()
	service.ConfigPath = "/tmp/test.json"
	err := service.Start()
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(10 * time.Millisecond)
	ioutil.WriteFile("/tmp/test.json", []byte(configTest2), os.ModePerm)
	err = service.ReloadConfig()
	time.Sleep(10 * time.Millisecond)
	runTestEcho := func(name string, echoa, echob *xio.PipeConn) {
		wc := make(chan int, 1)
		go func() {
			buf := make([]byte, 128)
			for i := 0; i < 10; i++ {
				n, _ := echoa.Read(buf)
				fmt.Printf("%v received %v", name, string(buf[0:n]))
				wc <- 1
			}
			echoa.Close()
			wc <- 1
		}()
		for i := 0; i < 10; i++ {
			fmt.Fprintf(echoa, "abc\n")
			<-wc
		}
		<-wc
		_, err = fmt.Fprintf(echoa, "abc\n")
		if err == nil {
			t.Error(err)
			return
		}
	}
	//
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		dialer := dialer.NewSocksProxyDialer()
		dialer.Bootstrap(xmap.M{
			"id":      "testing",
			"address": "localhost:10322",
		})
		_, err = dialer.Dial(1000, "echo", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks0", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SocksDialer(SocksUriTypeBS, "tcp://echo", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks1", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SocksDialer(SocksUriTypeBS, "tcp://echo?abc=1", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks2", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SocksDialer(SocksUriTypeBS, "w1?abc=1", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks3", echoa, echob)
	}
	{ //socks test
		echoa, echob, _ := xio.Pipe()
		_, err = service.SocksDialer(SocksUriTypeBS, "w2?abc=1", echob)
		if err != nil {
			t.Error(err)
			return
		}
		runTestEcho("socks4", echoa, echob)
	}
	// time.Sleep(1000 * time.Millisecond)
	//
	ioutil.WriteFile("/tmp/test.json", []byte(configTest1), os.ModePerm)
	err = service.ReloadConfig()
	time.Sleep(10 * time.Millisecond)
	service.Stop()
}
