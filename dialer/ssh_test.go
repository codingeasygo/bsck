package dialer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"

	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xhash"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
	"golang.org/x/crypto/ssh"
)

func TestSSH(t *testing.T) {
	if _, xerr := os.Stat("../certs/rootCA.crt"); os.IsNotExist(xerr) {
		cmd := exec.Command("bash", "-c", "./gen.sh")
		cmd.Dir = "../certs"
		err := cmd.Run()
		if err != nil {
			t.Error(err)
			return
		}
	}
	dialer := NewSshDialer()
	err := dialer.Bootstrap(xmap.M{
		"dir":  "../certs",
		"cert": "server.crt",
		"key":  "server.key",
		"auth": xhash.SHA1([]byte("123")),
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer dialer.Shutdown()
	tester := xdebug.CaseTester{
		0: 1,
	}
	if tester.Run("match") {
		if !dialer.Matched("ssh://server") {
			t.Error("errr")
			return
		}
		if dialer.Matched("ssh://xxx") {
			t.Error("errr")
			return
		}
	}
	if tester.Run("normal") {
		conn, err := dialer.Dial(&testChannel{}, 1, "ssh://server")
		if err != nil {
			t.Error(err)
			return
		}
		a, b := net.Pipe()
		go io.Copy(conn, a)
		go io.Copy(a, conn)
		c, channel, request, err := ssh.NewClientConn(b, "127.0.0.1", &ssh.ClientConfig{
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth:            []ssh.AuthMethod{ssh.Password("123")},
		})
		if err != nil {
			t.Error(err)
			return
		}
		client := ssh.NewClient(c, channel, request)
		ss, err := client.NewSession()
		if err != nil {
			t.Error(err)
			return
		}
		stdout := bytes.NewBuffer(nil)
		allout := bytes.NewBuffer(nil)
		ss.Stdin = bytes.NewBufferString("echo -n abc")
		ss.Stdout, ss.Stderr = xio.NewMultiWriter(stdout, allout), allout
		err = ss.Run("bash")
		data := stdout.Bytes()
		if err != nil || string(data) != "abc" {
			t.Errorf("err:%v,%v", err, string(data))
			return
		}
		ss.Close()
		client.Close()
	}
	if tester.Run("not auth") {
		conn, err := dialer.Dial(&testChannel{}, 1, "ssh://server")
		if err != nil {
			t.Error(err)
			return
		}
		a, b := net.Pipe()
		go io.Copy(conn, a)
		go io.Copy(a, conn)
		_, _, _, err = ssh.NewClientConn(b, "127.0.0.1", &ssh.ClientConfig{
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Auth:            []ssh.AuthMethod{ssh.Password("xxxxxx")},
		})
		if err == nil {
			t.Error(err)
			return
		}
	}
	if tester.Run("error") {
		errDialer := NewSshDialer()
		err := errDialer.Bootstrap(xmap.M{
			"dir":  "../certs",
			"cert": "server.xxx",
			"key":  "server.key",
			"auth": xhash.SHA1([]byte("123")),
		})
		if err == nil {
			t.Error(err)
			return
		}
	}
	if tester.Run("cover") {
		dialer.Name()
		dialer.Options()
		dialer.Network()
		dialer.Addr()
		fmt.Printf("-->%v\n", dialer)
	}
}
