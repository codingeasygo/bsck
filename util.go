package bsck

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"

	"github.com/codingeasygo/util/xio/frame"
)

func writeCmd(w frame.Writer, buffer []byte, cmd byte, sid uint64, msg []byte) (err error) {
	buffer[4] = cmd
	binary.BigEndian.PutUint64(buffer[5:], sid)
	copy(buffer[13:], msg)
	_, err = w.WriteFrame(buffer[:len(msg)+13])
	return
}

//Log is the bsck package default log
var Log = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)

func DebugLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("D "+format, args...))
}
func InfoLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("I "+format, args...))
}
func WarnLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("W "+format, args...))
}
func ErrorLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("E "+format, args...))
}
