package dialer

import (
	"os"
	"testing"

	"github.com/creack/pty"
)

// func TestBash(t *testing.T) {
// 	cmd := NewCmd("n1", "", "bash")
// 	cback := make(chan []byte)
// 	cmd.EnableCallback([]byte("-sctrl: "), cback)
// 	cmd.SetOut(os.Stdout)
// 	err := cmd.Start()
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}
// 	fmt.Fprintf(cmd, "echo kkkss && echo '-sctrl: %v'\n", "t00")
// 	msg := <-cback
// 	if string(msg) == "t00'" {
// 		msg = <-cback
// 	}
// 	if string(msg) != "t00" {
// 		t.Error(string(msg))
// 		return
// 	}
// }

func TestGetSize(t *testing.T) {
	pty, vty, err := pty.Open()
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		pty.Close()
		vty.Close()
	}()
	_, _, err = GetFileWinSize(pty)
	if err != nil {
		t.Error(err)
		return
	}
	err = SetFileWinSize(pty, 1024, 768)
	if err != nil {
		t.Error(err)
		return
	}
	_, _, err = GetFileWinSize(os.Stdout)
	if err == nil {
		t.Error(err)
		return
	}
	err = SetFileWinSize(os.Stdout, 1024, 768)
	if err == nil {
		t.Error(err)
		return
	}
}
