package dialer

import (
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/Centny/gwf/util"
)

type OnceDialer struct {
	ID     string
	dialed int
	conf   util.Map
}

func (o *OnceDialer) Name() string {
	return o.ID
}

//initial dialer
func (o *OnceDialer) Bootstrap(options util.Map) error {
	o.ID = options.StrVal("id")
	if len(o.ID) < 1 {
		return fmt.Errorf("id is required")
	}
	o.conf = options
	return nil
}

//
func (o *OnceDialer) Options() util.Map {
	return o.conf
}

//match uri
func (o *OnceDialer) Matched(uri string) bool {
	return uri == "once"
}

//dial raw connection
func (o *OnceDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (r Conn, err error) {
	r = o
	o.dialed++
	if o.dialed > 1 {
		err = fmt.Errorf("dialed")
	}
	return
}

func (o *OnceDialer) Read(p []byte) (n int, err error) {
	return
}

func (o *OnceDialer) Write(p []byte) (n int, err error) {
	return
}

func (o *OnceDialer) Close() error {
	return nil
}

func (o *OnceDialer) Pipe(r io.ReadWriteCloser) (err error) {
	return
}

func (o *OnceDialer) String() string {
	return fmt.Sprintf("OnceDialer-%v", o.ID)
}

func TestBalancedDialerDefaul(t *testing.T) {
	NewDialer = func(t string) Dialer {
		return &OnceDialer{}
	}
	dialer := NewBalancedDialer()
	err := dialer.Bootstrap(util.Map{
		"id":      "t1",
		"matcher": ".*",
		"timeout": 500,
		"delay":   100,
		"dialers": []util.Map{
			{
				"id":          "i0",
				"type":        "once",
				"fail_remove": 2,
			},
			{
				"id":          "i1",
				"type":        "once",
				"fail_remove": 3,
			},
			{
				"id":          "i2",
				"type":        "once",
				"fail_remove": 4,
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	_, err = dialer.Dial(uint64(4), "not", nil)
	if err == nil {
		t.Error(err)
		return
	}
	for i := 0; i < 3; i++ {
		_, err = dialer.Dial(uint64(i), "once", nil)
		if err != nil {
			t.Error(err)
			return
		}
	}
	_, err = dialer.Dial(uint64(4), "once", nil)
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Println("--->", err)
	NewDialer = DefaultDialerCreator
	//
	//
	dialer = NewBalancedDialer()
	dialer.Name()
	dialer.Options()
	dialer.AddDialer(NewTCPDialer())
	//
	//test error

	//id not found
	err = dialer.Bootstrap(util.Map{})
	if err == nil {
		t.Error(err)
		return
	}
	//policy error
	err = dialer.Bootstrap(util.Map{
		"id": "t0",
		"policy": []util.Map{
			{
				"matcher": "[",
				"limit":   []int64{},
			},
		},
	})
	if err == nil {
		t.Error(err)
		return
	}
	//dialer type error
	err = dialer.Bootstrap(util.Map{
		"id": "t0",
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
	err = dialer.Bootstrap(util.Map{
		"id": "t0",
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
	//uri error
	_, err = dialer.Dial(10, "%S", nil)
	if err == nil {
		t.Error(err)
		return
	}

	//
	err = dialer.AddPolicy(".*", []int64{})
	if err == nil {
		t.Error(err)
		return
	}
	err = dialer.AddPolicy("[.*", []int64{})
	if err == nil {
		t.Error(err)
		return
	}
}

type TimeDialer struct {
	ID     string
	dialed int
	conf   util.Map
	last   int64
}

func (t *TimeDialer) Name() string {
	return t.ID
}

//initial dialer
func (t *TimeDialer) Bootstrap(options util.Map) error {
	t.ID = options.StrVal("id")
	if len(t.ID) < 1 {
		return fmt.Errorf("id is required")
	}
	t.conf = options
	return nil
}

//
func (t *TimeDialer) Options() util.Map {
	return t.conf
}

//match uri
func (t *TimeDialer) Matched(uri string) bool {
	return true
}

//dial raw connection
func (t *TimeDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (r Conn, err error) {
	if util.Now()-t.last < 100 {
		panic("too fast")
	}
	r = t
	t.last = util.Now()
	time.Sleep(10 * time.Millisecond)
	return
}

func (t *TimeDialer) Read(p []byte) (n int, err error) {
	return
}

func (t *TimeDialer) Write(p []byte) (n int, err error) {
	return
}

func (t *TimeDialer) Close() error {
	return nil
}

func (t *TimeDialer) Pipe(r io.ReadWriteCloser) (err error) {
	return
}

func (t *TimeDialer) String() string {
	return fmt.Sprintf("OnceDialer-%v", t.ID)
}

func TestBalancedDialerPolicy(t *testing.T) {
	NewDialer = func(t string) Dialer {
		return &TimeDialer{}
	}
	defer func() {
		NewDialer = DefaultDialerCreator
	}()
	dialer := NewBalancedDialer()
	err := dialer.Bootstrap(util.Map{
		"id":      "t1",
		"matcher": ".*",
		"timeout": 10000,
		"delay":   1,
		"dialers": []util.Map{
			{
				"id":          "i0",
				"type":        "time",
				"fail_remove": 2,
			},
			{
				"id":          "i1",
				"type":        "time",
				"fail_remove": 3,
			},
			{
				"id":          "i2",
				"type":        "time",
				"fail_remove": 4,
			},
		},
		"policy": []util.Map{
			{
				"matcher": ".*",
				"limit":   []int{110, 1},
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	//
	//test loop muti dial
	for i := 0; i < 10; i++ {
		_, err = dialer.Dial(uint64(i), "time", nil)
		if err != nil {
			t.Errorf("%v->%v", i, err)
			return
		}
	}
	//
	//test concurrency multi dial
	wg := sync.WaitGroup{}
	total := 100
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func(v int) {
			defer wg.Done()
			_, err = dialer.Dial(uint64(v), fmt.Sprintf("time-%v", v/10), nil)
			if err != nil {
				t.Errorf("%v->%v", v, err)
				return
			}
		}(i)
	}
	wg.Wait()
}

func TestBalancedDialerLimit(t *testing.T) {
	NewDialer = func(t string) Dialer {
		return &TimeDialer{}
	}
	defer func() {
		NewDialer = DefaultDialerCreator
	}()
	dialer := NewBalancedDialer()
	err := dialer.Bootstrap(util.Map{
		"id":      "t1",
		"matcher": ".*",
		"timeout": 10000,
		"delay":   1,
		"dialers": []util.Map{
			{
				"id":          "i0",
				"type":        "time",
				"fail_remove": 2,
				"limit":       []int{110, 1},
			},
			{
				"id":          "i1",
				"type":        "time",
				"fail_remove": 3,
				"limit":       []int{110, 1},
			},
			{
				"id":          "i2",
				"type":        "time",
				"fail_remove": 4,
				"limit":       []int{110, 1},
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	//
	//test loop muti dial
	for i := 0; i < 10; i++ {
		_, err = dialer.Dial(uint64(i), "time", nil)
		if err != nil {
			t.Errorf("%v->%v", i, err)
			return
		}
	}
	//
	//test concurrency multi dial
	wg := sync.WaitGroup{}
	total := 100
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func(v int) {
			defer wg.Done()
			_, err = dialer.Dial(uint64(v), fmt.Sprintf("time-%v", v/10), nil)
			if err != nil {
				t.Errorf("%v->%v", v, err)
				return
			}
		}(i)
	}
	wg.Wait()
}

func TestBalancedDialerFilter(t *testing.T) {
	NewDialer = func(t string) Dialer {
		return &TimeDialer{}
	}
	defer func() {
		NewDialer = DefaultDialerCreator
	}()
	dialer := NewBalancedDialer()
	err := dialer.Bootstrap(util.Map{
		"id":      "t1",
		"matcher": ".*",
		"timeout": 10000,
		"delay":   1,
		"filter": []util.Map{
			{
				"matcher": "time-[0-2]",
				"access":  0,
			},
			{
				"matcher": "time-[3-5]",
				"access":  1,
			},
		},
		"dialers": []util.Map{
			{
				"id":    "i0",
				"type":  "time",
				"limit": []int{110, 1},
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	for i := 0; i < 5; i++ {
		_, err = dialer.Dial(uint64(i), fmt.Sprintf("time-%v", i), nil)
		if i < 3 && err == nil {
			t.Errorf("%v->%v", i, err)
			return
		}
		if i > 2 && err != nil {
			t.Errorf("%v->%v", i, err)
			return
		}
	}
	//
	//test bootstrap error
	dialer = NewBalancedDialer()
	err = dialer.Bootstrap(util.Map{
		"id":      "t1",
		"matcher": ".*",
		"timeout": 10000,
		"delay":   1,
		"filter": []util.Map{
			{
				"matcher": "tim[e-[0-2]",
				"access":  0,
			},
		},
		"dialers": []util.Map{
			{
				"id":    "i0",
				"type":  "time",
				"limit": []int{110, 1},
			},
		},
	})
	if err == nil {
		t.Error(err)
		return
	}
}
