package dialer

import (
	"bytes"
	"fmt"
	"io"
	"net/url"

	"github.com/codingeasygo/util"
)

func createCmd(c *CmdDialer, runnable string, remote *url.URL) (cmdReader io.Reader,
	cmdWriter io.Writer,
	cmdCloser func() error,
	cmdStart func() error) {
	cmd := NewCmd("Cmd", c.PS1, "bash", "-c", runnable)
	if len(c.Prefix) > 0 {
		cmd.Prefix = bytes.NewBuffer([]byte(c.Prefix))
	}
	cmd.Dir = c.Dir
	cmd.Raw.Env = append(cmd.Raw.Env, c.Env...)
	ps1 := remote.Query().Get("PS1")
	if len(ps1) > 0 {
		cmd.PS1 = ps1
	}
	dir := remote.Query().Get("Dir")
	if len(dir) > 0 {
		cmd.Dir = dir
	}
	for key, vals := range remote.Query() {
		switch key {
		case "PS1":
		case "Dir":
		case "LC":
		case "exec":
		default:
			cmd.Raw.Env = append(cmd.Raw.Env, fmt.Sprintf("%v=%v", key, vals[0]))
		}
	}
	cmd.Cols, cmd.Rows = 80, 60
	util.ValidAttrF(`cols,O|I,R:0;rows,O|I,R:0;`, remote.Query().Get, true, &cmd.Cols, &cmd.Rows)
	cmdReader = cmd
	cmdWriter = cmd
	cmdCloser = cmd.Close
	cmdStart = cmd.Start
	return
}
