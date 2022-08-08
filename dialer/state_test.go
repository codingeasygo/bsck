package dialer

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/codingeasygo/util/xmap"
)

type StateData struct {
}

func (s *StateData) State(args ...interface{}) xmap.M {
	return xmap.M{
		"a": 1,
		"b": 2,
	}
}

func TestState(t *testing.T) {
	dialer := NewStateDialer("test", &StateData{})
	cona, conb := CreatePipedConn()
	_, err := dialer.Dial(nil, 0, "test", conb)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Fprintf(cona, "test")
	bys, err := ioutil.ReadAll(cona)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println(len(bys))
	fmt.Println(string(bys))
	//
	if !dialer.Matched("state://test") {
		t.Error("error")
		return
	}
	if dialer.Matched("state://tesxt") {
		t.Error("error")
		return
	}
	//
	dialer.Name()
	dialer.Bootstrap(nil)
	dialer.Options()
}
