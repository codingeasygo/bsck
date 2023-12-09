package dialer

import (
	"testing"

	"github.com/codingeasygo/util/xmap"
)

func TestUdpGW(t *testing.T) {
	dialer := NewUdpGwDialer()
	err := dialer.Bootstrap(xmap.M{
		"dns": "127.0.0.1:53",
	})
	if err != nil {
		t.Error(err)
		return
	}
	dialer.Matched("tcp://udpgw")
	dialer.Matched("tcp://none")
	dialer.Dial(nil, 0, "test")
	dialer.Options()
	dialer.Name()
	dialer.Shutdown()

	err = dialer.Bootstrap(xmap.M{
		"dns": []byte{1, 2, 3},
	})
	if err == nil {
		t.Error(err)
		return
	}
	err = dialer.Bootstrap(xmap.M{
		"max_alive": -1,
	})
	if err == nil {
		t.Error(err)
		return
	}
}
