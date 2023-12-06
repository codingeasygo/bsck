package syscall

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
)

func HandlerSSH(s ssh.Session) {
	defer func() {
		if perr := recover(); perr != nil {
			s.Exit(1)
		}
	}()
	_, winCh, isPty := s.Pty()
	if !isPty {
		cmd := exec.Command("powershell")
		cmd.Stdin = s
		cmd.Stdout = s
		cmd.Stderr = s
		err := cmd.Run()
		if err != nil {
			fmt.Fprintf(s, "powershell exit with %v", err)
		}
		s.Exit(cmd.ProcessState.ExitCode())
		return
	}
	f, err := conpty.Start("powershell")
	if err != nil {
		fmt.Fprintf(s, "start pty error %v", err)
		s.Exit(1)
		return
	}
	go func() {
		for win := range winCh {
			f.Resize(win.Width, win.Height)
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
