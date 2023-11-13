package dialer

import (
	"io"

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
		io.WriteString(s, "No PTY requested.\n")
		s.Exit(1)
		return
	}
	f, err := conpty.Start("powershell")
	if err != nil {
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
