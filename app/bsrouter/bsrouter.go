package bsrouter

import (
	"log"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/bsck/router"
	t2clog "github.com/codingeasygo/tun2conn/log"
	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xmap"

	_ "golang.org/x/mobile/bind"
)

var logPrinter Logger
var logger = log.New(&logWriter{}, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)
var globalWorkDir string
var lastError error

func init() {
	dialer.Logger = logger
	router.Logger = logger
	t2clog.Log = logger
}

func Bootstrap(dir string, log Logger) (result string) {
	if len(dir) < 1 {
		result = "work dir is empty"
		return
	}
	globalWorkDir = dir
	logPrinter = log
	return
}

type Logger interface {
	PrintLog(line string)
}

type logWriter struct {
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	if logPrinter != nil {
		logPrinter.PrintLog(string(p))
	}
	n = len(p)
	return
}

func State() (state string) {
	nodeState := xmap.M{}
	nodeState["code"] = 0
	nodeState["name"] = "nodeName"
	if lastError != nil {
		nodeState["debug"] = lastError.Error()
	}
	state = converter.JSON(nodeState)
	return
}
