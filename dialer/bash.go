package dialer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/kr/pty"
)

var CtrlC = []byte{255, 244, 255, 253, 6}

type Cmd struct {
	Raw    *exec.Cmd
	Dir    string
	Name   string
	PS1    string
	pipe   *os.File
	tty    *os.File
	Prefix io.Reader
	OnExit func(err error)
	Rows   int
	Cols   int
	ctrlc  int
}

func NewCmd(name, ps1, shell string, args ...string) (cmd *Cmd) {
	cmd = &Cmd{
		Name: name,
		PS1:  ps1,
		Raw:  exec.Command(shell, args...),
	}
	cmd.Raw.Dir = cmd.Dir
	cmd.Raw.Env = os.Environ()
	return
}

func (c *Cmd) AddEnvf(format string, args ...interface{}) {
	c.Raw.Env = append(c.Raw.Env, fmt.Sprintf(format, args...))
}

// func (c *Cmd) String() string {
// 	return c.Name
// }

func (c *Cmd) prepare() (err error) {
	if len(c.PS1) > 0 {
		c.Raw.Env = append(c.Raw.Env, "PS1="+c.PS1)
	}
	//
	c.pipe, c.tty, err = pty.Open()
	if err != nil {
		return
	}
	if c.Rows > 0 && c.Cols > 0 {
		err = SetFileWinSize(c.pipe, c.Rows, c.Cols)
	}
	if err != nil {
		c.tty.Close()
		c.pipe.Close()
		return
	}
	c.Raw.Stdout = c.tty
	c.Raw.Stdin = c.tty
	c.Raw.Stderr = c.tty
	if c.Raw.SysProcAttr == nil {
		c.Raw.SysProcAttr = &syscall.SysProcAttr{}
	}
	setCmdAttr(c.Raw)
	return
}

func (c *Cmd) Start() (err error) {
	err = c.prepare()
	if err != nil {
		return
	}
	//
	err = c.Raw.Start()
	if err != nil {
		c.pipe.Close()
		c.tty.Close()
		return
	}
	c.tty.Close()
	if c.Prefix != nil {
		io.Copy(c, c.Prefix)
	}
	return
}

func (c *Cmd) Write(p []byte) (n int, err error) {
	ctrlc := bytes.Count(p, CtrlC)
	if ctrlc > 0 {
		c.ctrlc += ctrlc
		if c.ctrlc >= 5 {
			c.Close()
			err = fmt.Errorf("Cmd is closed")
			return
		}
	} else {
		c.ctrlc = 0
	}
	n, err = c.pipe.Write(p)
	return
}

func (c *Cmd) Read(p []byte) (n int, err error) {
	n, err = c.pipe.Read(p)
	return
}

//Close the command.
func (c *Cmd) Close() error {
	c.pipe.Close()
	c.tty.Close()
	c.Raw.Process.Kill()
	return c.Raw.Wait()
}

// type CallbackCmd struct {
// 	*Cmd
// 	*MultiWriter
// 	out *OutWriter
// }

// func NewCallbackCmd(name, ps1, shell string) (cmd *CallbackCmd) {
// 	cmd = &CallbackCmd{
// 		Cmd:         NewCmd(name, ps1, shell),
// 		out:         NewOutWriter(),
// 		MultiWriter: NewMultiWriter(),
// 	}
// 	cmd.MultiWriter.Add(cmd.out)
// 	return
// }

// func (c *CallbackCmd) Start() (err error) {
// 	err = c.prepare()
// 	if err != nil {
// 		return
// 	}
// 	err = c.Raw.Start()
// 	if err != nil {
// 		c.pipe.Close()
// 		c.tty.Close()
// 		return
// 	}
// 	c.tty.Close()
// 	go func() {
// 		_, err = io.Copy(c.MultiWriter, c.pipe)
// 		if c.OnExit != nil {
// 			c.OnExit(err)
// 		}
// 	}()
// 	//
// 	if c.Prefix != nil {
// 		io.Copy(c, c.Prefix)
// 	}
// 	time.Sleep(500 * time.Millisecond)
// 	return
// }

// func (c *CallbackCmd) EnableCallback(prefix []byte, back chan []byte) {
// 	c.out.EnableCallback(prefix, back)
// }

// func (c *CallbackCmd) DisableCallback() {
// 	c.out.DisableCallback()
// }

// func (c *CallbackCmd) Write(p []byte) (n int, err error) {
// 	n, err = c.Cmd.Write(p)
// 	return
// }

// func (c *CallbackCmd) Read(b []byte) (n int, err error) {
// 	n, err = c.Cmd.Read(b)
// 	return
// }

// func (c *CallbackCmd) Close() error {
// 	c.DisableCallback()
// 	return c.Cmd.Close()
// }

func GetFileWinSize(t *os.File) (rows, cols int, err error) {
	var ws WinSize
	err = GetWindowRect(&ws, t.Fd())
	rows, cols = int(ws.Row), int(ws.Col)
	return
}

func SetFileWinSize(t *os.File, rows, cols int) (err error) {
	var ws WinSize
	ws.Row, ws.Col = uint16(rows), uint16(cols)
	err = SetWindowRect(&ws, t.Fd())
	return
}

type WinSize struct {
	Row    uint16
	Col    uint16
	PixelX uint16
	PixelY uint16
}
