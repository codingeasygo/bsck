package gfw

import (
	"fmt"
	"testing"
)

func TestGFW(t *testing.T) {
	rules, err := ReadAllRules("gfwlist.txt", "user_rules.txt")
	if err != nil {
		t.Error(err)
		return
	}
	gfw := NewProxyGFW(rules...)
	gfw.Set(`
testproxy
192.168.1.18
	`, GfwProxy)
	if !gfw.IsProxy("youtube-ui.l.google.com") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("google.com") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("google.com.") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy(".google.com. ") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("www.google.com.hk") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("www.google.cn") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("google.cn") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy(".youtube.com.") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("testproxy") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("d3c33hcgiwev3.cloudfront.net") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("xx.wwwhost.biz") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("www.ftchinese.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("xxddsf.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("www.baidu.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("baidu.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("notexistxxx.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("qq.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("x.qq.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("x1.x2.qq.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("192.168.1.1") {
		t.Error("hav proxy")
		return
	}
	if !gfw.IsProxy("192.168.1.18") {
		t.Error("not proxy")
		return
	}
	fmt.Printf("info:%v\n", gfw)

	ReadGfwlist("abp.go")
	ReadGfwlist("none.txt")
	ReadUserRules("none.txt")

	CreateAbpJS(rules, "127.0.0.1:1000")
}
