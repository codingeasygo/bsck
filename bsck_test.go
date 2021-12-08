package bsck

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		http.ListenAndServe(":6063", nil)
	}()
}
