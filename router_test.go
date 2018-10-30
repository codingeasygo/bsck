package bsck

import (
	"strings"
	"testing"
)

func TestRawConnError(t *testing.T) {
	raw := NewRawConn(nil, 0, "xx->xxx->%AX")
	if !strings.Contains(raw.String(), "error") {
		t.Error("error")
	}
}
