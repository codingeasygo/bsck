package dialer

import (
	"bytes"
	"fmt"
	"io"
	"net/url"

	"github.com/Centny/gwf/util"
)

func createCmd(c *CmdDialer, runnable string, remote *url.URL) (cmdReader io.Reader,
	cmd := exec.Command("cmd", "/C", runnable)
	stdin, _ := cmd.StdinPipe()
	stdout, piped, _ := os.Pipe()
	cmd.Stdout = piped
	cmd.Stderr = piped
	cmdReader = stdout
	cmdWriter = stdin
	cmdCloser = func() error {
		stdin.Close()
		piped.Close()
		cmd.Process.Kill()
		err := cmd.Wait()
		return err
	}
	cmdStart = cmd.Start
	return
}
