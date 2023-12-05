package dialer

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestFullBuf(t *testing.T) {
	r, w, _ := os.Pipe()
	w.Close()
	var last int64
	buf := make([]byte, 1024)
	err := fullBuf(r, buf, 1, &last)
	if err == nil {
		t.Error(err)
		return
	}
	r, w, _ = os.Pipe()
	go func() {
		err = fullBuf(r, buf, 10, &last)
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Println("read done")
		w.Close()
	}()
	w.WriteString("12345")
	time.Sleep(time.Millisecond)
	w.WriteString("12345")
	time.Sleep(time.Millisecond)
}

func TestDuplexPiped(t *testing.T) {

}
