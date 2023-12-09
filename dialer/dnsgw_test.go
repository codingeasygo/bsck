package dialer

import (
	"fmt"
	"testing"

	"github.com/codingeasygo/util/xmap"
)

func TestDnsGW(t *testing.T) {
	dialer := NewDnsGwDialer()
	err := dialer.Bootstrap(xmap.M{})
	if err != nil {
		t.Error(err)
		return
	}
	dialer.Matched("tcp://dnsgw")
	dialer.Matched("tcp://none")
	dialer.Dial(nil, 0, "test")
	dialer.Options()
	dialer.Name()
	dialer.Shutdown()

	fmt.Printf("-->%v\n", dialer)
}
