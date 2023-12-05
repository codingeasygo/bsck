package router

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xhash"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
)

func newBaseProxy(proxy bool) (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	err = proxy0.Listen("127.0.0.1:15023", "", "")
	if err != nil {
		return
	}
	proxyServer := ""
	if proxy {
		proxyServer = "socks5://test:123@127.0.0.1:13210"
	}
	_, _, err = proxy1.Login(xmap.M{
		"proxy":  proxyServer,
		"local":  "127.0.0.1:0",
		"remote": "127.0.0.1:15023",
		"token":  "123",
	})
	return
}

func newKeepProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N3"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	proxy1.KeepDelay = 30 * time.Millisecond
	err = proxy0.Listen("127.0.0.1:15023", "", "")
	if err != nil {
		return
	}
	proxy1.Channels["N0"] = xmap.M{
		"local":  "127.0.0.1:0",
		"remote": "127.0.0.1:15023",
		"token":  "123",
	}
	proxy1.Start()
	return
}

func newTlsProxy(proxy bool) (proxy0, proxy1 *Proxy, err error) {
	if _, xerr := os.Stat("../certs/rootCA.crt"); os.IsNotExist(xerr) {
		cmd := exec.Command("bash", "-c", "./gen.sh")
		cmd.Dir = "../certs"
		err = cmd.Run()
		if err != nil {
			return
		}
	}
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	err = proxy0.Listen("tls://127.0.0.1:15023", "../certs/server.crt", "../certs/server.key")
	if err != nil {
		fmt.Printf("listen error %v\n", err)
		return
	}
	proxyServer := ""
	if proxy {
		proxyServer = "socks5://test:123@127.0.0.1:13210"
	}
	_, _, err = proxy1.Login(xmap.M{
		"proxy":  proxyServer,
		"local":  "127.0.0.1:0",
		"remote": "tls://127.0.0.1:15023",
		"token":  "123",
		"tls_ca": "../certs/rootCA.crt",
	})
	return
}

func newTlsVerifyProxy(proxy bool) (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	err = proxy0.Listen("tls://127.0.0.1:15023", "../certs/server.crt", "../certs/server.key")
	if err != nil {
		return
	}
	proxyServer := ""
	if proxy {
		proxyServer = "socks5://test:123@127.0.0.1:13210"
	}
	_, _, err = proxy1.Login(xmap.M{
		"proxy":      proxyServer,
		"local":      "127.0.0.1:0",
		"remote":     "tls://127.0.0.1:15023",
		"token":      "123",
		"tls_cert":   "../certs/server.crt",
		"tls_key":    "../certs/server.key",
		"tls_verify": "0",
	})
	return
}

func newWsProxy(proxy bool) (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	err = proxy0.Listen("ws://127.0.0.1:15023", "", "")
	if err != nil {
		return
	}
	proxyServer := ""
	if proxy {
		proxyServer = "socks5://test:123@127.0.0.1:13210"
	}
	_, _, err = proxy1.Login(xmap.M{
		"proxy":  proxyServer,
		"remote": "ws://127.0.0.1:15023",
		"token":  "123",
	})
	return
}

func newWssProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	err = proxy0.Listen("wss://127.0.0.1:15023", "../certs/server.crt", "../certs/server.key")
	if err != nil {
		return
	}
	_, _, err = proxy1.Login(xmap.M{
		"remote": "wss://127.0.0.1:15023",
		"token":  "123",
		"tls_ca": "../certs/rootCA.crt",
	})
	return
}

func newWssVerifyProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	err = proxy0.Listen("wss://127.0.0.1:15023", "../certs/server.crt", "../certs/server.key")
	if err != nil {
		return
	}
	_, _, err = proxy1.Login(xmap.M{
		"remote":     "wss://127.0.0.1:15023",
		"token":      "123",
		"tls_verify": 0,
	})
	return
}

func newQuicProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = xhash.SHA1([]byte("123"))
	access0.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", DefaultBufferSize, access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = xhash.SHA1([]byte("123"))
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", DefaultBufferSize, access1)
	err = proxy0.Listen("quic://127.0.0.1:15023", "../certs/server.crt", "../certs/server.key")
	if err != nil {
		return
	}
	_, _, err = proxy1.Login(xmap.M{
		"remote": "quic://127.0.0.1:15023",
		"token":  "123",
		"tls_ca": "../certs/rootCA.crt",
	})
	return
}

func TestProxy(t *testing.T) {
	testProxy := func(proxy0, proxy1 *Proxy, err error) error {
		if err != nil {
			t.Errorf("%v\n%v", err, xdebug.CallStack())
			return err
		}
		connA, connB, _ := xio.CreatePipedConn()
		_, _, err = proxy1.DialConn(connB, "N0->tcp://127.0.0.1:13200")
		if err != nil {
			t.Error(err)
			return err
		}
		fmt.Fprintf(connA, "abc")
		buf := make([]byte, 1024)
		n, err := connA.Read(buf)
		if err != nil || string(buf[0:n]) != "abc" {
			err = fmt.Errorf("%v,%v,%v", err, n, buf[0:n])
			t.Error(err)
			return err
		}
		proxy0.Stop()
		proxy1.Stop()
		return err
	}
	tester := xdebug.CaseTester{
		0:  1,
		13: 1,
	}
	if tester.Run("newBaseProxy") && testProxy(newBaseProxy(false)) != nil {
		return
	}
	if tester.Run("newBaseProxy-2") && testProxy(newBaseProxy(true)) != nil {
		return
	}
	if tester.Run("newTlsProxy") && testProxy(newTlsProxy(false)) != nil {
		return
	}
	if tester.Run("newTlsProxy-2") && testProxy(newTlsProxy(true)) != nil {
		return
	}
	if tester.Run("newTlsVerifyProxy") && testProxy(newTlsVerifyProxy(false)) != nil {
		return
	}
	if tester.Run("newTlsVerifyProxy") && testProxy(newTlsVerifyProxy(true)) != nil {
		return
	}
	if tester.Run("newWsProxy") && testProxy(newWsProxy(false)) != nil {
		return
	}
	if tester.Run("newWssProxy") && testProxy(newWssProxy()) != nil {
		return
	}
	if tester.Run("newWssVerifyProxy") && testProxy(newWssVerifyProxy()) != nil {
		return
	}
	if tester.Run("newWsProxy-P") && testProxy(newWsProxy(true)) != nil {
		return
	}
	if tester.Run("newQuicProxy") && testProxy(newQuicProxy()) != nil {
		return
	}
	if tester.Run("speed") {
		proxy0, proxy1, _ := newBaseProxy(false)
		proxy2 := NewProxy("NX", DefaultBufferSize, NewNormalAcessHandler("NX"))
		speed, err := proxy2.Ping(xmap.M{
			"remote": "127.0.0.1:15023",
		})
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Printf("speed-->%v\n", converter.JSON(speed))
		_, err = proxy2.Ping(xmap.M{
			"remote": "127.0.0.1:15xx",
		})
		if err != nil {
			t.Error(err)
			return
		}
		_, err = proxy2.Ping(xmap.M{})
		if err == nil {
			t.Error(err)
			return
		}
		proxy0.Stop()
		proxy1.Stop()
	}
	if tester.Run("notify") {
		proxy0, proxy1, _ := newBaseProxy(false)
		proxy1.Notify("N0", []byte("abc"))
		proxy0.Stop()
		proxy1.Stop()
	}
	if tester.Run("keep") { //keep
		proxy0, proxy1, err := newKeepProxy()
		if err != nil {
			t.Error(err)
			return
		}
		time.Sleep(10 * time.Millisecond)

		proxy2 := NewProxy("N2", DefaultBufferSize, NewNormalAcessHandler("N2"))
		proxy2.KeepDelay = 30 * time.Millisecond
		proxy2.Channels["NX"] = xmap.M{
			"remote": "127.0.0.1:15023",
			"token":  "123",
		}
		err = proxy2.Keep()
		if err == nil {
			t.Error(err)
			return
		}

		channel, err := proxy1.SelectChannel("N0")
		if err != nil {
			t.Error(err)
			return
		}
		channel.Close()
		time.Sleep(50 * time.Millisecond)
		err = testProxy(proxy0, proxy1, nil)
		if err != nil {
			t.Error(err)
			return
		}
		//
		proxy0.Stop()
		time.Sleep(50 * time.Millisecond)
		proxy1.Stop()
		proxy1.Router = nil
		proxy1.procKeep()

		//login error
		proxy2 = NewProxy("N2", DefaultBufferSize, NewNormalAcessHandler("N2"))
		proxy2.KeepDelay = 30 * time.Millisecond
		proxy2.Channels["N2"] = xmap.M{
			"remote": "127.0.0.1:15020",
			"token":  "123",
		}
		err = proxy2.Keep()
		if err == nil {
			t.Error(err)
			return
		}
	}
	if tester.Run("error") { //error
		access0 := NewNormalAcessHandler("N0")
		access0.LoginAccess["N1"] = "123"
		access0.LoginAccess["N2"] = "123"
		access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
		proxy0 := NewProxy("N0", DefaultBufferSize, access0)
		err := proxy0.Listen("tls://:0", "../certs/server.crt", "xxx.key")
		if err == nil {
			t.Error(err)
			return
		}
		err = proxy0.Listen("tls://:0", "../certs/server.crt", "")
		if err == nil {
			t.Error(err)
			return
		}
		err = proxy0.Listen("xxx://:0", "", "")
		if err == nil {
			t.Error(err)
			return
		}

		access1 := NewNormalAcessHandler("N1")
		access1.LoginAccess["N2"] = "123"
		access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
		proxy1 := NewProxy("N1", DefaultBufferSize, access1)
		_, _, err = proxy1.Login(xmap.M{
			"local":    "127.0.0.1:0",
			"remote":   "127.0.0.1:15023",
			"token":    "123",
			"tls_cert": "xxx.crt",
			"tls_key":  "../certs/server.key",
		})
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = proxy1.Login(xmap.M{
			"local":    "127.0.0.1:0",
			"remote":   "127.0.0.1:15023",
			"token":    "123",
			"tls_cert": "../certs/server.crt",
			"tls_key":  "../certs/server.key",
			"tls_ca":   "xxx.crt",
		})
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = proxy1.Login(xmap.M{
			"local":    "127.0.0.1:0",
			"remote":   "127.0.0.1:15023",
			"token":    "123",
			"tls_cert": "../certs/server.crt",
			"tls_key":  "../certs/server.key",
			"tls_ca":   "log.go",
		})
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = proxy1.Login(xmap.M{
			"local":    "127.0.0.1:0",
			"remote":   "wss://127.0.0.1:15023",
			"token":    "123",
			"tls_cert": "../certs/server.crt",
			"tls_key":  "../certs/server.key",
			"tls_ca":   "xxx.crt",
		})
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = proxy1.Login(xmap.M{
			"local":  "127.0.0.1:xx",
			"remote": "127.0.0.1:15023",
			"token":  "123",
		})
		if err == nil {
			t.Error(err)
			return
		}
		_, _, err = proxy1.Login(xmap.M{})
		if err == nil {
			t.Error(err)
			return
		}
		_, err = proxy1.loadClientConfig("", "", "", "")
		if err == nil {
			t.Error(err)
			return
		}
		_, err = proxy1.loadClientConfig("log.go", "log.go", "", "")
		if err == nil {
			t.Error(err)
			return
		}
		_, err = proxy1.loadClientConfig("../certs/server.crt", "../certs/server.key", "log.go", "")
		if err == nil {
			t.Error(err)
			return
		}
	}
	if tester.Run("DialError") {
		proxy := NewProxy("N0", DefaultBufferSize, NewNormalAcessHandler("N0"))

		//proxy error
		_, err := proxy.dialConn("tcp://127.0.0.1:100", string([]byte{1, 1}), "", "", "", "", "")
		if err == nil {
			t.Error(err)
			return
		}

		//cert error
		_, err = proxy.dialConn("tls://127.0.0.1:100", "", "log.go", "log.go", "log.go", "", "")
		if err == nil {
			t.Error(err)
			return
		}
		//cert error
		_, err = proxy.dialConn("quic://127.0.0.1:100", "", "log.go", "log.go", "log.go", "", "")
		if err == nil {
			t.Error(err)
			return
		}

		//proxy dial error
		_, err = proxy.dialConn("tls://127.0.0.1:10", "socks5://127.0.0.1:13210", "../certs/server.crt", "../certs/server.key", "", "", "")
		if err == nil {
			t.Error(err)
			return
		}
		_, err = proxy.dialConn("tls://127.0.0.1:13200", "socks5://127.0.0.1:13210", "../certs/server.crt", "../certs/server.key", "", "", "")
		if err == nil {
			t.Error(err)
			return
		}
	}
	if tester.Run() {
		proxy := NewProxy("N1", DefaultBufferSize, nil)
		proxy.DialRawConn(nil, 0, "")
		proxy.OnConnDialURI(nil, "", nil)
		proxy.OnConnLogin(nil, "")
	}
}

func TestInfoRWC(t *testing.T) {
	rwc := NewInfoRWC(nil, "abc")
	rwc.RawValue()
	fmt.Printf("-->%v\n", rwc)
}

func TestEncodeWebURI(t *testing.T) {
	EncodeWebURI("%v", "(xxxkssf)")
}

func TestTlsConfigShow(t *testing.T) {
	TlsConfigShow("-----BEGIN xkksdfsdf")
	TlsConfigShow("0x00")
	TlsConfigShow("0x00" + hex.EncodeToString(make([]byte, 1024)))
	TlsConfigShow("xxx")
}
