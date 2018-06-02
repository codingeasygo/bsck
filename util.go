package bsck

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

func readCmd(r io.Reader, buf []byte, last *int64) (length uint32, err error) {
	err = fullBuf(r, buf, 4, last)
	if err != nil {
		return
	}
	length = binary.BigEndian.Uint32(buf)
	if length+4 > uint32(len(buf)) {
		err = fmt.Errorf("frame too large")
		return
	}
	err = fullBuf(r, buf[4:], length, last)
	length += 4
	return
}

func fullBuf(r io.Reader, p []byte, length uint32, last *int64) error {
	all := uint32(0)
	buf := p[:length]
	for {
		readed, err := r.Read(buf)
		if err != nil {
			return err
		}
		if last != nil {
			*last = time.Now().Local().UnixNano() / 1e6
		}
		all += uint32(readed)
		if all < length {
			buf = p[all:]
			continue
		} else {
			break
		}
	}
	return nil
}

func writeCmd(r io.Writer, b []byte, cmd byte, sid uint64, msg []byte) (err error) {
	length := uint32(len(msg) + 9)
	binary.BigEndian.PutUint32(b, length)
	b[4] = cmd
	binary.BigEndian.PutUint64(b[5:], sid)
	copy(b[13:], msg)
	_, err = r.Write(b[:length+4])
	return
}

//Log is the bsck package default log
var Log = log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)

func debugLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("D "+format, args...))
}
func infoLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("I "+format, args...))
}
func warnLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("W "+format, args...))
}
func errorLog(format string, args ...interface{}) {
	Log.Output(2, fmt.Sprintf("E "+format, args...))
}
