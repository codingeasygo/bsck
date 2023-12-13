package bsrouter

import (
	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn/gfw"
)

func SetupGFW(gfwRules, userRules string) (res Result) {
	if len(globalWorkDir) < 1 {
		res = newCodeResult(-1, "not bootstrap")
		return
	}
	gfw.Shared.Dir = globalWorkDir
	if len(gfwRules) > 0 {
		gfw.Shared.Gfwlist = gfwRules
	}
	if len(userRules) > 0 {
		gfw.Shared.UserRules = userRules
	}
	_, err := gfw.Shared.LoadGFW()
	if err != nil {
		lastError = err
		res = newCodeResult(-1, err.Error())
		router.ErrorLog("setup gfw fail with %v", err)
		return
	}
	res = newCodeResult(0, "OK")
	return
}
