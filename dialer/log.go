package dialer

import (
	"fmt"
	"log"
	"os"
)

//LogLevel is log leveo config
var LogLevel int = 3

//Log is the bsck package default log
var Log = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)

//DebugLog is log by debug level
func DebugLog(format string, args ...interface{}) {
	if LogLevel >= 3 {
		Log.Output(2, fmt.Sprintf("D "+format, args...))
	}
}

//InfoLog is log by info level
func InfoLog(format string, args ...interface{}) {
	if LogLevel >= 2 {
		Log.Output(2, fmt.Sprintf("I "+format, args...))
	}
}

//WarnLog is log by warn level
func WarnLog(format string, args ...interface{}) {
	if LogLevel >= 1 {
		Log.Output(2, fmt.Sprintf("W "+format, args...))
	}
}

//ErrorLog is log by error level
func ErrorLog(format string, args ...interface{}) {
	if LogLevel >= 0 {
		Log.Output(2, fmt.Sprintf("E "+format, args...))
	}
}
