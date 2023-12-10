package bsrouter

import (
	"strings"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn/gfw"
)

var globalGFW = gfw.NewGFW()

func SetupGFW(gfwRules, userRules string) (res Result) {
	var err error
	var rules []string
	if len(gfwRules) > 0 {
		if strings.HasSuffix(gfwRules, ".txt") {
			rules, err = gfw.ReadGfwlist(gfwRules)
		} else {
			rules, err = gfw.DecodeGfwlist(gfwRules)
		}
		if err != nil {
			router.ErrorLog("setup gfw error %v", err)
			lastError = err
			res = newCodeResult(-1, err.Error())
			return
		}
	}
	if len(userRules) > 0 {
		if strings.HasSuffix(userRules, ".txt") {
			rules, err = gfw.ReadUserRules(gfwRules)
		} else {
			rules = gfw.DecodeUserRules(gfwRules)
		}
		if err != nil {
			router.ErrorLog("setup gfw error %v", err)
			lastError = err
			res = newCodeResult(-1, err.Error())
			return
		}
	}
	globalGFW = gfw.NewGFW()
	if len(rules) > 0 {
		globalGFW.Set(strings.Join(rules, "\n"), gfw.GfwProxy)
	}
	res = newCodeResult(0, "OK")
	return
}
