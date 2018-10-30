package dialer

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/Centny/gwf/util"
)

type StateData struct {
}

func (s *StateData) State() util.Map {
	return util.Map{
		"a": 1,
		"b": 2,
	}
}

func TestState(t *testing.T) {
	dialer := NewStateDialer("test", &StateData{})
	cona, conb, _ := CreatePipedConn()
	_, err := dialer.Dial(0, "test", conb)
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
