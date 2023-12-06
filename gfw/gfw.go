package gfw

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

const (
	//GfwProxy is GFW target for proxy
	GfwProxy = "proxy"
	//GfwLocal is GFW target for local
	GfwLocal = "local"
)

// GFW impl check if domain in gfw list
type GFW struct {
	list map[string]string
	lck  sync.RWMutex
}

// NewGFW will create new GFWList
func NewGFW() (gfw *GFW) {
	gfw = &GFW{
		list: map[string]string{
			"*": GfwLocal,
		},
		lck: sync.RWMutex{},
	}
	return
}

func NewProxyGFW(rules ...string) (gfw *GFW) {
	gfw = NewGFW()
	gfw.Set(strings.Join(rules, "\n"), GfwProxy)
	return
}

// Set list
func (g *GFW) Set(list, target string) {
	g.lck.Lock()
	defer g.lck.Unlock()
	g.list[list] = target
}

// IsProxy return true, if domain target is dns://proxy
func (g *GFW) IsProxy(domain string) bool {
	return g.Find(domain) == GfwProxy
}

// Find domain target
func (g *GFW) Find(domain string) (target string) {
	g.lck.RLock()
	defer g.lck.RUnlock()
	domain = strings.Trim(domain, " \t.")
	if len(domain) < 1 {
		target = g.list["*"]
		return
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		target = g.check(parts...)
	} else {
		n := len(parts) - 1
		for i := 0; i < n; i++ {
			target = g.check(parts[i:]...)
			if len(target) > 0 {
				break
			}
		}
	}
	if len(target) < 1 {
		target = g.list["*"]
	}
	return
}

func (g *GFW) check(parts ...string) (target string) {
	ptxt := fmt.Sprintf("(?m)^[^\\@]*[\\|\\.]*(http://)?(https://)?%v$", strings.Join(parts, "\\."))
	pattern, err := regexp.Compile(ptxt)
	if err == nil {
		for key, val := range g.list {
			if len(pattern.FindString(key)) > 0 {
				target = val
				break
			}
		}
	}
	return
}

func (g *GFW) String() string {
	return "GFW"
}

// ReadGfwlist will read and decode gfwlist file
func ReadGfwlist(gfwFile string) (rules []string, err error) {
	gfwRaw, err := os.ReadFile(gfwFile)
	if err != nil {
		return
	}
	gfwData, err := base64.StdEncoding.DecodeString(string(gfwRaw))
	if err != nil {
		err = fmt.Errorf("decode gfwlist.txt fail with %v", err)
		return
	}
	gfwRulesAll := strings.Split(string(gfwData), "\n")
	for _, rule := range gfwRulesAll {
		if strings.HasPrefix(rule, "[") || strings.HasPrefix(rule, "!") || len(strings.TrimSpace(rule)) < 1 {
			continue
		}
		rules = append(rules, rule)
	}
	return
}

// ReadUserRules will read and decode user rules
func ReadUserRules(gfwFile string) (rules []string, err error) {
	gfwData, err := os.ReadFile(gfwFile)
	if err != nil {
		return
	}
	gfwRulesAll := strings.Split(string(gfwData), "\n")
	for _, rule := range gfwRulesAll {
		rule = strings.TrimSpace(rule)
		if strings.HasPrefix(rule, "--") || strings.HasPrefix(rule, "!") || len(strings.TrimSpace(rule)) < 1 {
			continue
		}
		rules = append(rules, rule)
	}
	return
}

func ReadAllRules(gfwFile, userFile string) (rules []string, err error) {
	rules, err = ReadGfwlist(gfwFile)
	if err == nil {
		userRules, _ := ReadUserRules(userFile)
		rules = append(rules, userRules...)
	}
	return
}

func CreateAbpJS(gfwRules []string, proxyAddr string) (js string) {
	gfwRulesJS, _ := json.Marshal(gfwRules)
	parts := strings.Split(proxyAddr, ":")
	js = strings.Replace(ABP_JS, "__RULES__", string(gfwRulesJS), 1)
	js = strings.Replace(js, "__SOCKS5ADDR__", strings.Join(parts[:len(parts)-1], ":"), -1)
	js = strings.Replace(js, "__SOCKS5PORT__", parts[len(parts)-1], -1)
	return
}
