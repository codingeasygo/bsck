package dialer

import (
	"fmt"
	"testing"

	"github.com/codingeasygo/util/xmap"
)

func TestSchemaDialer(t *testing.T) {
	dialer := NewSchemaDialer()
	dialer.Bootstrap(xmap.M{
		"mapping": xmap.M{
			"test://loc1": "http://localhost",
			"test://loc2": "https://localhost",
			"test://err":  "https://xx/%EX%B8%AD%E8%AF%AD%E8%A8%80",
			"abc://*":     "http://localhost",
		},
	})
	if !dialer.Matched("test://loc1") {
		t.Error("error")
		return
	}
	if !dialer.Matched("abc://xxx") {
		t.Error("error")
		return
	}
	if dialer.Matched("https://xx/%EX%B8%AD%E8%AF%AD%E8%A8%80") {
		t.Error("error")
		return
	}
	con, err := dialer.Dial(nil, 10, "test://loc1", nil)
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	//
	con, err = dialer.Dial(nil, 10, "test://loc2", nil)
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	//
	con, err = dialer.Dial(nil, 10, "abc://xxx", nil)
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	//
	fmt.Printf("%v test done...\n", dialer)
	dialer.Name()
	dialer.Options()
	dialer.Shutdown()
	//
	//test pipe
	cona, conb := CreatePipedConn()
	con, err = dialer.Dial(nil, 10, "test://loc1", conb)
	if err != nil {
		t.Error(err)
		return
	}
	con.Close()
	cona.Close()
	//
	//test error
	_, err = dialer.Dial(nil, 10, "http://xxx", nil)
	if err == nil {
		t.Error(err)
		return
	}
	_, err = dialer.Dial(nil, 10, "https://xx/%EX%B8%AD%E8%AF%AD%E8%A8%80", nil)
	if err == nil {
		t.Error(err)
		return
	}
	_, err = dialer.Dial(nil, 10, "test://err", nil)
	if err == nil {
		t.Error(err)
		return
	}
}
