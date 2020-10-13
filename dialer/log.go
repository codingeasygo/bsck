package dialer

import (
	"fmt"
	"log"
	"os"
)

const (
	//LogLevelDebug is debug log level
	LogLevelDebug = 40
	//LogLevelInfo is info log level
	LogLevelInfo = 30
	//LogLevelWarn is warn log level
	LogLevelWarn = 20
	//LogLevelError is error log level
	LogLevelError = 10
)

//LogLevel is log leveo config
var LogLevel = LogLevelInfo

//Logger is the bsck package default log
var Logger = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)

//SetLogLevel is set log level to l
func SetLogLevel(l int) {
	if l > 0 {
		LogLevel = l
	}
}

//DebugLog is the debug level log
func DebugLog(format string, args ...interface{}) {
	if LogLevel < LogLevelDebug {
		return
	}
	Logger.Output(2, fmt.Sprintf("D "+format, args...))
}

//InfoLog is the info level log
func InfoLog(format string, args ...interface{}) {
	if LogLevel < LogLevelInfo {
		return
	}
	Logger.Output(2, fmt.Sprintf("I "+format, args...))
}

//WarnLog is the warn level log
func WarnLog(format string, args ...interface{}) {
	if LogLevel < LogLevelWarn {
		return
	}
	Logger.Output(2, fmt.Sprintf("W "+format, args...))
}

//ErrorLog is the error level log
func ErrorLog(format string, args ...interface{}) {
	if LogLevel < LogLevelError {
		return
	}
	Logger.Output(2, fmt.Sprintf("E "+format, args...))
}
