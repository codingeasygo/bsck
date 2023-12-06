package native

import (
	"fmt"
	"time"
)

func ChangeProxyMode(mode, pacURL string, proxyAddr string, proxyPort int) (message string, err error) {
	switch mode {
	case "auto":
		pacURL := fmt.Sprintf("%v?timestamp=%v", pacURL, time.Now().Local().UnixNano()/1e6)
		message, err = ChangeProxyModeNative("auto", pacURL)
	case "global":
		message, err = ChangeProxyModeNative("global", proxyAddr, fmt.Sprintf("%v", proxyPort))
	default:
		message, err = ChangeProxyModeNative("manual")
	}
	return
}
