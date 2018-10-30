package dialer

import (
	"os/exec"
)

func setCmdAttr(cmd *exec.Cmd) {

}

func GetWindowRect(ws *WinSize, fd uintptr) error {
	ws.Col = 100
	ws.Row = 80
	return nil
}

func SetWindowRect(ws *WinSize, fd uintptr) error {
	return nil
}
