package syscall

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
)

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

func HandlerSSH(s ssh.Session) {
	defer func() {
		if perr := recover(); perr != nil {
			s.Exit(1)
		}
	}()

	shell, err := exec.LookPath("sh")
	if err != nil {
		shell, err = exec.LookPath("bash")
	}
	if err != nil {
		shell, err = exec.LookPath("/usr/bin/sh")
	}
	if err != nil {
		shell, err = exec.LookPath("/usr/bin/bash")
	}
	if err != nil {
		shell, err = exec.LookPath("/system/bin/sh")
	}
	if err != nil {
		fmt.Fprintf(s, "look shell fail")
		return
	}

	ptyReq, winCh, isPty := s.Pty()
	if !isPty {
		args := []string{}
		if cmdArg := s.Command(); len(cmdArg) > 0 {
			shell = cmdArg[0]
			args = cmdArg[1:]
		}
		cmd := exec.Command(shell, args...)
		cmd.Stdin = s
		cmd.Stdout = s
		cmd.Stderr = s
		err := cmd.Run()
		if err != nil {
			fmt.Fprintf(s, "sh exit with %v", err)
		}
		s.Exit(cmd.ProcessState.ExitCode())
		return
	}
	cmd := exec.Command(shell)
	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
	f, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(s, "start pty error %v", err)
		s.Exit(1)
		return
	}
	go func() {
		for win := range winCh {
			setWinsize(f, win.Width, win.Height)
		}
	}()
	go func() {
		io.Copy(f, s) // stdin
		f.Close()
	}()
	io.Copy(s, f) // stdout
	s.Exit(cmd.ProcessState.ExitCode())
}

func SubsystemSSH() map[string]ssh.SubsystemHandler {
	return map[string]ssh.SubsystemHandler{
		"sftp": func(s ssh.Session) {
			server, err := sftp.NewServer(s)
			if err == nil {
				server.Serve()
				server.Close()
			}
		},
	}
}
