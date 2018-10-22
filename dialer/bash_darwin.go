package dialer

import (
	"os/exec"
	"syscall"
	"unsafe"
)

func setCmdAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr.Setctty = true
	cmd.SysProcAttr.Setsid = true
}

func GetWindowRect(ws *WinSize, fd uintptr) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

func SetWindowRect(ws *WinSize, fd uintptr) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}
