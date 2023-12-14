package dialer

import (
	"bytes"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"testing"

	"github.com/codingeasygo/util/xmap"
)

func init() {
	SetLogLevel(LogLevelDebug)
	go http.ListenAndServe(":6063", nil)
}

type errDialer struct {
	Dialer
}

func (e *errDialer) Bootstrap(o xmap.M) (err error) {
	err = fmt.Errorf("test erro")
	return
}

type testChannel struct {
	id uint16
}

func (t *testChannel) ID() uint16 {
	return t.id
}
func (t *testChannel) Name() string {
	return "test"
}
func (t *testChannel) Context() xmap.M {
	return xmap.M{}
}

func TestChannelInfo(t *testing.T) {
	info := NewChannelInfo(1, "a")
	info.ID()
	info.Name()
	info.Context()
}

func TestPool(t *testing.T) {
	_ = DefaultDialerCreator("x")
	NewDialer = func(t string) (dialer Dialer) {
		return NewTCPDialer()
	}
	pool := NewPool("test")
	pool.Webs = map[string]http.Handler{
		"a": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}
	err := pool.Bootstrap(xmap.M{
		"std":   1,
		"ssh":   1,
		"udpgw": 1,
		"web":   1,
		"dialers": []xmap.M{
			{"type": "xxx"},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	_, err = pool.Dial(nil, 10, "tcp://127.0.0.1:10")
	if err == nil {
		t.Error(err)
		return
	}
	pool.Shutdown()
	pool.AddDialer(NewTCPDialer())

	//test not dialer
	pool = NewPool("test")
	_, err = pool.Dial(nil, 10, "http://dav?dir=/tmp")
	if err == nil {
		t.Error(err)
		return
	}

	//test dialer piper
	pool = NewPool("test")
	pool.AddDialer(NewUdpGwDialer(), NewEchoDialer())
	_, err = pool.DialPiper("tcp://udpgw", 1024)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = pool.DialPiper("tcp://echo", 1024)
	if err != nil {
		t.Error(err)
		return
	}

	//
	//test error
	//dialer type error
	NewDialer = DefaultDialerCreator
	err = pool.Bootstrap(xmap.M{
		"dialers": []xmap.M{
			{
				"type": "xx",
			},
		},
	})
	if err == nil {
		t.Error(err)
		return
	}
	//dialer bootstrap error
	err = pool.Bootstrap(xmap.M{
		"dialers": []xmap.M{
			{
				"type": "balance",
			},
		},
	})
	if err == nil {
		t.Error(err)
		return
	}

	NewDialer = func(t string) (dialer Dialer) {
		dialer = &errDialer{}
		return
	}
	pool = NewPool("test")
	pool.Bootstrap(xmap.M{
		"dialers": []xmap.M{{"type": "xx"}},
	})
	NewDialer = DefaultDialerCreator
}

type ClosableBuffer struct {
	*bytes.Buffer
}

func NewClosableBuffer(b *bytes.Buffer) *ClosableBuffer {
	return &ClosableBuffer{Buffer: b}
}

func (c *ClosableBuffer) Close() error {
	return nil
}

func TestCreator(t *testing.T) {
	RegisterDialerCreator("abc", func() Dialer { return NewTCPDialer() })
	DefaultDialerCreator("abc")
}
