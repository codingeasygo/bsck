package dialer

import (
	"fmt"
	"io"
	"os"
	"testing"
)

func TestEcho(t *testing.T) {
	dialer := NewEchoDialer()
	dialer.Bootstrap(nil)
	if dialer.Matched("tcp://echox") {
		t.Error("error")
		return
	}
	if !dialer.Matched("tcp://echo") {
		t.Error("error")
		return
	}
	conn, err := dialer.Dial(10, "tcp://echo", nil)
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
	dialer.Name()
	dialer.Options()
}
