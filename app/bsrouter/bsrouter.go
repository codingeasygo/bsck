package bsrouter

import (
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/codingeasygo/bsck/dialer"
	"github.com/codingeasygo/bsck/router"
	t2clog "github.com/codingeasygo/tun2conn/log"
	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xmap"

	_ "golang.org/x/mobile/bind"
)

func init() {
	go http.ListenAndServe(":6065", nil)
}

var logPrinter Logger
var logger = log.New(&logWriter{}, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)
var globalWorkDir string
var lastError error

func init() {
	log.SetOutput(&logWriter{})
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)
	dialer.Logger = logger
	router.Logger = logger
	t2clog.Log = logger
	t2clog.LogLevel = 3
}

func Bootstrap(dir string, log Logger) (res Result) {
	if len(dir) < 1 {
		res = newCodeResult(-1, "work dir is empty")
		return
	}
	globalWorkDir = dir
	logPrinter = log
	res = newCodeResult(0, "OK")
	return
}

func HandleMessage(message []byte) (response []byte) {
	response = message
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

type Result interface {
	Code() int
	Message() string
	IntValue() int
	StringValue() string
}

type valueResult struct {
	code        int
	message     string
	intValue    int
	stringValue string
}

func newCodeResult(code int, message string) (result *valueResult) {
	result = &valueResult{
		code:    code,
		message: message,
	}
	return
}

func newIntResult(v int) (result *valueResult) {
	result = &valueResult{
		intValue: v,
	}
	return
}

func newStringResult(v string) (result *valueResult) {
	result = &valueResult{
		stringValue: v,
	}
	return
}

func (v *valueResult) Code() int {
	return v.code
}

func (v *valueResult) Message() string {
	return v.message
}

func (v *valueResult) IntValue() int {
	return v.intValue
}

func (v *valueResult) StringValue() string {
	return v.stringValue
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
