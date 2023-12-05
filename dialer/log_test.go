package dialer

import "testing"

func TestLog(t *testing.T) {
	LogLevel = 9
	DebugLog("log")
	InfoLog("log")
	WarnLog("warn")
	ErrorLog("error")
	LogLevel = LogLevelDebug
	DebugLog("log")
	InfoLog("log")
	WarnLog("warn")
	ErrorLog("error")
}
