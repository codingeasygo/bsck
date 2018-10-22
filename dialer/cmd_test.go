package dialer

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Centny/gwf/util"
)

func TestCmdDialer(t *testing.T) {
	cmd := NewCmdDialer()
	cmd.PS1 = "CmdDialer"
	cmd.Prefix = `echo testing`
	cmd.Bootstrap(util.Map{
		"reuse":       100,
		"reuse_delay": 50,
		"Env": util.Map{
			"a": "val",
		},
	})
	if !cmd.Matched("tcp://cmd?exec=/bin/bash") {
		t.Error("error")
		return
	}
	fmt.Println("---->0")
	raw, err := cmd.Dial(10, "tcp://cmd?exec=/bin/bash&PS1=testing&Dir=/tmp&e1=1&reuse=xx", nil)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println("---->1")
	go io.Copy(os.Stdout, raw)
	fmt.Fprintf(raw, "ls\n")
	fmt.Fprintf(raw, "ls /tmp/\n")
	fmt.Fprintf(raw, "echo abc\n")
	fmt.Println("---->1-0")
	time.Sleep(200 * time.Millisecond)
	raw.Write(TelnetCtrlC)
	fmt.Println("---->1-1")
	time.Sleep(200 * time.Millisecond)
	raw.Close()
	time.Sleep(200 * time.Millisecond)
	fmt.Println("---->2")
	{ //test reuse
		raw2, err := cmd.Dial(10, "tcp://cmd?exec=/bin/bash&PS1=testing&Dir=/tmp&e1=1&reuse=xx", nil)
		if err != nil || raw == raw2 {
			fmt.Printf("-->%p--%p\n", raw, raw2)
			t.Error(err)
			return
		}
		raw3, err := cmd.Dial(10, "tcp://cmd?exec=/bin/bash&PS1=testing&Dir=/tmp&e1=1&reuse=xx", nil)
		if err != nil || raw == raw3 || raw2 == raw3 {
			fmt.Printf("-->%p--%p\n", raw, raw2)
			t.Error(err)
			return
		}
		raw2.Close()
		raw4, err := cmd.Dial(10, "tcp://cmd?exec=/bin/bash&PS1=testing&Dir=/tmp&e1=1&reuse=xx", nil)
		if err != nil || raw4 != raw2 {
			fmt.Printf("-->%p--%p\n", raw, raw2)
			t.Error(err)
			return
		}
		raw4.Close()
		raw3.Close()
		raw5, err := cmd.Dial(10, "tcp://cmd?exec=/bin/bash&PS1=testing&Dir=/tmp&e1=1&reuse=xx", nil)
		if err != nil || raw5 != raw3 {
			fmt.Printf("-->%p--%p\n", raw, raw2)
			t.Error(err)
			return
		}
		raw5.(*ReusableRWC).Resume()
		raw5.(*ReusableRWC).Resume()
		raw5.Close()
		raw5.Close()
		time.Sleep(500 * time.Millisecond)
		if raw5.(*ReusableRWC).Reused {
			t.Error("error")
			return
		}
		//
		//for cover
		raw4.Read(make([]byte, 1024))
		raw4.Write(make([]byte, 1024))
		raw5.(*ReusableRWC).Resume()
		//
		//
	}
	{ //test ctrl-c
		fmt.Printf("\n\n\ntest ctrl-c\n\n")
		raw2, err := cmd.Dial(10, "tcp://cmd?exec=/bin/bash", nil)
		if err != nil {
			t.Error(err)
			return
		}
		for i := 0; i < 5; i++ {
			raw2.Write(CtrlC)
		}
		if raw2.(*ReusableRWC).Reused {
			t.Error("errors")
			return
		}
		fmt.Printf("----->\n")
	}
	//for cover
	fmt.Printf("%v\n", cmd)
	//
	//test encoding
	fmt.Printf("\n\n\ntest encoding\n\n")
	raw, err = cmd.Dial(10, "tcp://cmd?exec=/bin/bash&LC=zh_CN.GBK", nil)
	if err != nil {
		t.Error(err)
		return
	}
	go io.Copy(os.Stdout, raw)
	fmt.Fprintf(raw, "ls\n")
	time.Sleep(200 * time.Millisecond)
	raw.Close()
	//
	raw, err = cmd.Dial(10, "tcp://cmd?exec=/bin/bash&LC=zh_CN.GB18030", nil)
	if err != nil {
		t.Error(err)
		return
	}
	go io.Copy(os.Stdout, raw)
	fmt.Fprintf(raw, "ls\n")
	time.Sleep(200 * time.Millisecond)
	raw.Close()
	//
	//
	cmd.Shutdown()
	time.Sleep(500 * time.Millisecond)
	cmd.Name()
	cmd.Options()
}

func TestCmdDialer2(t *testing.T) {
	cmd := NewCmdDialer()
	cmd.PS1 = "CmdDialer"
	cmd.Prefix = `echo testing`
	cmd.Bootstrap(nil)
	if !cmd.Matched("tcp://cmd?exec=bash") {
		t.Error("error")
		return
	}
	raw, err := cmd.Dial(10, "tcp://cmd?exec=bash", nil)
	if err != nil {
		t.Error(err)
		return
	}
	go io.Copy(os.Stdout, raw)
	fmt.Fprintf(raw, "ls\n")
	fmt.Fprintf(raw, "ls /tmp/\n")
	fmt.Fprintf(raw, "echo abc\n")
	time.Sleep(200 * time.Millisecond)
	raw.Write(TelnetCtrlC)
	time.Sleep(200 * time.Millisecond)
	raw.Close()
	time.Sleep(200 * time.Millisecond)
	//for cover
	fmt.Printf("%v\n", cmd)
	//
	//test encoding
	raw, err = cmd.Dial(10, "tcp://cmd?exec=bash&LC=zh_CN.GBK", nil)
	if err != nil {
		t.Error(err)
		return
	}
	go io.Copy(os.Stdout, raw)
	fmt.Fprintf(raw, "ls\n")
	time.Sleep(200 * time.Millisecond)
	raw.Close()
	//
	raw, err = cmd.Dial(10, "tcp://cmd?exec=bash&LC=zh_CN.GB18030", nil)
	if err != nil {
		t.Error(err)
		return
	}
	go io.Copy(os.Stdout, raw)
	fmt.Fprintf(raw, "ls\n")
	time.Sleep(200 * time.Millisecond)
	raw.Close()
	//
	//test error
	_, err = cmd.Dial(100, "://cmd", nil)
	if err == nil {
		t.Error("error")
		return
	}
}
