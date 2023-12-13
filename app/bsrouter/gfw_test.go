package bsrouter

import (
	"testing"

	"github.com/codingeasygo/tun2conn/gfw"
)

func TestSetupGFW(t *testing.T) {
	SetupGFW("", "")

	gfw, _ := gfw.Shared.LoadGFW()
	if !gfw.IsProxy("www.google.com.hk.") || !gfw.IsProxy("www.google.com.hk") {
		t.Error("error")
		return
	}
}
