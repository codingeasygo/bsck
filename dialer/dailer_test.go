package dialer

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/codingeasygo/util/xmap"
)

func init() {
	SetLogLevel(LogLevelDebug)
}

func TestPool(t *testing.T) {
	pool := NewPool("test")
	err := pool.Bootstrap(xmap.M{
		"dialers": []xmap.M{
			{
				"id":      "t0",
				"type":    "balance",
				"matcher": "^proxy://.*$",
			},
			{
				"id":      "t1",
				"type":    "socks",
				"matcher": "^socks://.*$",
			},
		},
		"std": 1,
	})
	if err != nil {
		t.Error(err)
		return
	}
	_, err = pool.Dial(nil, 10, "http://dav?dir=/tmp", nil)
	if err != nil {
		t.Error(err)
		return
	}

	//test not dialer
	pool = NewPool("test")
	_, err = pool.Dial(nil, 10, "http://dav?dir=/tmp", nil)
	if err == nil {
		t.Error(err)
		return
	}
	pool.AddDialer(NewTCPDialer())
	pool.Bootstrap(xmap.M{
		"standard": 1,
	})
	//
	//test error
	//dialer type error
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

func TestCopyPipable(t *testing.T) {
	cona, conb := CreatePipedConn()
	reader := NewClosableBuffer(bytes.NewBufferString("1234567890"))
	piped := NewCopyPipable(reader)
	piped.Pipe(conb)
	buf := make([]byte, 10)
	cona.Read(buf)
	fmt.Println(string(buf))
	//
	//test pipe error
	err := piped.Pipe(conb)
	if err == nil {
		t.Error(err)
	}
	//
	piped.Close()
}
