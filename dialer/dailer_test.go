package dialer

import (
	"testing"

	"github.com/Centny/gwf/util"
)

func TestPool(t *testing.T) {
	pool := NewPool()
	err := pool.Bootstrap(util.Map{
		"dialers": []util.Map{
			{
				"id":      "t0",
				"type":    "balance",
				"matcher": "^proxy://.*$",
			},
			{
				"type": "cmd",
			},
			{
				"type": "echo",
			},
			{
				"id":      "t1",
				"type":    "socks",
				"matcher": "^socks://.*$",
			},
			{
				"type": "web",
			},
			{
				"type": "tcp",
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	_, err = pool.Dial(10, "http://web?dir=/tmp", nil)
	if err != nil {
		t.Error(err)
		return
	}

	//test not dialer
	pool = NewPool()
	_, err = pool.Dial(10, "http://web?dir=/tmp", nil)
	if err == nil {
		t.Error(err)
		return
	}
	pool.AddDialer(NewTCPDialer())
	pool.Bootstrap(util.Map{
		"standard": 1,
	})
	//
	//test error
	//dialer type error
	err = pool.Bootstrap(util.Map{
		"dialers": []util.Map{
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
	err = pool.Bootstrap(util.Map{
		"dialers": []util.Map{
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
