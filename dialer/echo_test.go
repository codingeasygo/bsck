package dialer

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/codingeasygo/util/xio"
)

func TestEcho(t *testing.T) {
	dialer := NewEchoDialer()
	dialer.Bootstrap(nil)
	defer dialer.Shutdown()
	if dialer.Matched("tcp://echox") {
		t.Error("error")
		return
	}
	if !dialer.Matched("tcp://echo") {
		t.Error("error")
		return
	}
	conn, err := dialer.Dial(nil, 10, "tcp://echo")
	if err != nil {
		t.Error(err)
		return
	}
	go func() {
		for {
			_, err := io.Copy(os.Stdout, conn)
			if err != nil {
				break
			}
		}
	}()
	fmt.Fprintf(conn, "data-%v\n", 0)
	conn.Close()

	//
	//for cover
	dialer.Name()
	dialer.Options()

	//
	//test closed EchoReadWriteCloser()
	rwc := NewEchoReadWriteCloser()
	qc := xio.NewQueryConn()
	go rwc.Pipe(qc)
	qc.Query(context.Background(), []byte("abc"))
	rwc.Close()
	_, err = rwc.Write(nil)
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Printf("-->%v\n", rwc)
}
