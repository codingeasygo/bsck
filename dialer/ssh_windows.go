package dialer

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/gliderlabs/ssh"
)

func sshHandler(s ssh.Session) {
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
		s.Exit(1)
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
	s.Exit(1)
}
