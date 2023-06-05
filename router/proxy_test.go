package router

import (
	"fmt"
	"testing"
	"time"

	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
)

func newBaseProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", access1)
	err = proxy0.Listen("127.0.0.1:15023")
	if err != nil {
		return
	}
	_, _, err = proxy1.Login(xmap.M{
		"local":  "127.0.0.1:0",
		"remote": "127.0.0.1:15023",
		"token":  "123",
	})
	return
}

func newTlsProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", access0)
	proxy0.CA = "../certs/rootCA.crt"
	proxy0.Cert = "../certs/server.crt"
	proxy0.Key = "../certs/server.key"

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", access1)
	err = proxy0.Listen("tls://127.0.0.1:15023")
	if err != nil {
		return
	}
	_, _, err = proxy1.Login(xmap.M{
		"local":  "127.0.0.1:0",
		"remote": "tls://127.0.0.1:15023",
		"token":  "123",
		"tls_ca": "../certs/rootCA.crt",
	})
	return
}

func newTlsVerifyProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", access0)
	proxy0.CA = "../certs/rootCA.crt"
	proxy0.Cert = "../certs/server.crt"
	proxy0.Key = "../certs/server.key"

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", access1)
	err = proxy0.Listen("tls://127.0.0.1:15023")
	if err != nil {
		return
	}
	_, _, err = proxy1.Login(xmap.M{
		"local":      "127.0.0.1:0",
		"remote":     "tls://127.0.0.1:15023",
		"token":      "123",
		"tls_cert":   "../certs/server.crt",
		"tls_key":    "../certs/server.key",
		"tls_verify": "0",
	})
	return
}

func newWsProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", access0)

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", access1)
	err = proxy0.Listen("ws://127.0.0.1:15023")
	if err != nil {
		return
	}
	_, _, err = proxy1.Login(xmap.M{
		"remote": "ws://127.0.0.1:15023",
		"token":  "123",
	})
	return
}

func newWssProxy() (proxy0, proxy1 *Proxy, err error) {
	access0 := NewNormalAcessHandler("N0")
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", access0)
	proxy0.CA = "../certs/rootCA.crt"
	proxy0.Cert = "../certs/server.crt"
	proxy0.Key = "../certs/server.key"

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", access1)
	err = proxy0.Listen("wss://127.0.0.1:15023")
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
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", access0)
	proxy0.CA = "../certs/rootCA.crt"
	proxy0.Cert = "../certs/server.crt"
	proxy0.Key = "../certs/server.key"

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", access1)
	err = proxy0.Listen("wss://127.0.0.1:15023")
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
	access0.LoginAccess["N1"] = "123"
	access0.LoginAccess["N2"] = "123"
	access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
	proxy0 = NewProxy("N0", access0)
	proxy0.CA = "../certs/rootCA.crt"
	proxy0.Cert = "../certs/server.crt"
	proxy0.Key = "../certs/server.key"

	access1 := NewNormalAcessHandler("N1")
	access1.LoginAccess["N2"] = "123"
	access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
	proxy1 = NewProxy("N1", access1)
	err = proxy0.Listen("quic://127.0.0.1:15023")
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
			t.Error(err)
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
		0: 1,
		7: 1,
	}
	if tester.Run() && testProxy(newBaseProxy()) != nil {
		return
	}
	if tester.Run() && testProxy(newTlsProxy()) != nil {
		return
	}
	if tester.Run() && testProxy(newTlsVerifyProxy()) != nil {
		return
	}
	if tester.Run() && testProxy(newWsProxy()) != nil {
		return
	}
	if tester.Run() && testProxy(newWssProxy()) != nil {
		return
	}
	if tester.Run() && testProxy(newWssVerifyProxy()) != nil {
		return
	}
	if tester.Run() && testProxy(newQuicProxy()) != nil {
		return
	}
	if tester.Run() { //reconnect
		proxy0, proxy1, err := newBaseProxy()
		if err != nil {
			t.Error(err)
			return
		}
		proxy1.ReconnectDelay = 10 * time.Millisecond
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

		proxy0, proxy1, err = newBaseProxy()
		if err != nil {
			t.Error(err)
			return
		}
		proxy1.ReconnectDelay = 10 * time.Millisecond
		proxy0.Stop()
		time.Sleep(50 * time.Millisecond)
		proxy1.Stop()
	}
	if tester.Run() { //error
		access0 := NewNormalAcessHandler("N0")
		access0.LoginAccess["N1"] = "123"
		access0.LoginAccess["N2"] = "123"
		access0.DialAccess = append(access0.DialAccess, []string{".*", ".*"})
		proxy0 := NewProxy("N0", access0)
		proxy0.Cert = "../certs/server.crt"
		proxy0.Key = "xxx.key"
		err := proxy0.Listen("tls://:0")
		if err == nil {
			t.Error(err)
			return
		}
		proxy0.CA = "../certs/rootCA.crt"
		proxy0.Cert = "../certs/server.crt"
		proxy0.Key = "../certs/server.key"
		err = proxy0.Listen("127.0.0.1:x")
		if err == nil {
			t.Error(err)
			return
		}

		access1 := NewNormalAcessHandler("N1")
		access1.LoginAccess["N2"] = "123"
		access1.DialAccess = append(access1.DialAccess, []string{".*", ".*"})
		proxy1 := NewProxy("N1", access1)
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
	}
	if tester.Run() {
		proxy := NewProxy("N1", nil)
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
