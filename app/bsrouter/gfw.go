package bsrouter

import (
	"strings"

	"github.com/codingeasygo/bsck/router"
	"github.com/codingeasygo/tun2conn/gfw"
)

var globalGFW = gfw.NewGFW()

func SetupGFW(gfwRules, userRules string) (result string) {
	var err error
	var rules []string
	if strings.HasSuffix(gfwRules, ".txt") {
		rules, err = gfw.ReadAllRules(gfwRules, userRules)
	} else {
		rules, err = gfw.DecodeAllRules(gfwRules, userRules)
	}
	if err != nil {
		router.ErrorLog("setup gfw error %v", err)
		lastError = err
		result = err.Error()
		return
	}
	globalGFW.Set(strings.Join(rules, "\n"), gfw.GfwProxy)
	return
}
