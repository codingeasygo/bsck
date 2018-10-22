package dialer

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Centny/gwf/log"
	"github.com/Centny/gwf/util"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

//TelnetCtrlC is the ctrl-c command on telnet
var TelnetCtrlC = []byte{255, 244, 255, 253, 6}

//CmdStdinWriter is writer to handler charset replace and close command.
type CmdStdinWriter struct {
	io.Writer
	Replace  []byte
	CloseTag []byte
}

func NewCmdStdinWriter(w io.Writer, replace, closeTag []byte) (writer *CmdStdinWriter) {
	writer = &CmdStdinWriter{
		Writer:   w,
		Replace:  replace,
		CloseTag: closeTag,
	}
	return
}

func (c *CmdStdinWriter) Write(p []byte) (n int, err error) {
	if len(c.CloseTag) > 0 {
		newp := bytes.Replace(p, c.CloseTag, []byte{}, -1)
		if len(newp) != len(p) {
			err = fmt.Errorf("CmdStdinWriter is closed")
			return 0, err
		}
	}
	n = len(p)
	if len(c.Replace) > 0 {
		p = bytes.Replace(p, c.Replace, []byte{}, -1)
	}
	_, err = c.Writer.Write(p)
	return
}

//CmdDialer is an implementation of the Dialer interface for dial command
type CmdDialer struct {
	Replace     []byte
	CloseTag    []byte
	PS1         string
	Dir         string
	LC          string
	Prefix      string
	Env         []string
	Reuse       int64
	ReuseDelay  time.Duration
	running     map[string]*ReusableRWC
	runningLck  sync.RWMutex
	loopRunning bool
	conf        util.Map
}

//NewCmdDialer will return new CmdDialer
func NewCmdDialer() *CmdDialer {
	cmd := &CmdDialer{
		CloseTag:    nil,
		running:     map[string]*ReusableRWC{},
		runningLck:  sync.RWMutex{},
		Reuse:       3600000,
		ReuseDelay:  30 * time.Second,
		loopRunning: true,
		conf:        util.Map{},
	}
	if runtime.GOOS == "windows" {
		// cmd.Replace = []byte("\r")
	}
	return cmd
}

//Name will return dialer name
func (c *CmdDialer) Name() string {
	return "cmd"
}

func (c *CmdDialer) loopReuse() {
	log.D("CmdDailer the reuse time loop is starting")
	for c.loopRunning {
		c.runningLck.Lock()
		now := util.Now()
		for name, reused := range c.running {
			if now-reused.Last >= c.Reuse {
				reused.Destory()
				delete(c.running, name)
			}
		}
		c.runningLck.Unlock()
		time.Sleep(c.ReuseDelay)
	}
	log.D("CmdDailer the reuse time loop is stopped")
}

//Bootstrap the dilaer
func (c *CmdDialer) Bootstrap(options util.Map) error {
	if options != nil {
		c.PS1 = options.StrVal("PS1")
		c.Dir = options.StrVal("Dir")
		c.LC = options.StrVal("LC")
		c.Prefix = options.StrVal("Prefix")
		for k, v := range options.MapVal("Env") {
			c.Env = append(c.Env, fmt.Sprintf("%v=%v", k, v))
		}
		c.Reuse = options.IntValV("reuse", 3600000)
		c.ReuseDelay = time.Duration(options.IntValV("reuse_delay", 30000)) * time.Millisecond
	}
	if c.Reuse > 0 {
		go c.loopReuse()
	}
	return nil
}

func (c *CmdDialer) Options() util.Map {
	return c.conf
}

//Matched will return wheter uri is invalid uril.
func (c *CmdDialer) Matched(uri string) bool {
	target, err := url.Parse(uri)
	return err == nil && target.Scheme == "tcp" && target.Host == "cmd"
}

func (c *CmdDialer) onCmdPaused(r *ReusableRWC) {
	c.runningLck.Lock()
	defer c.runningLck.Unlock()
	c.running[r.Name] = r
	r.Last = util.Now()
	log.D("CmdDialer add session to reuse by %v->%p", r.Name, r)
}

//Dial will start command and pipe to stdin/stdout
func (c *CmdDialer) Dial(sid uint64, uri string, pipe io.ReadWriteCloser) (raw Conn, err error) {
	remote, err := url.Parse(uri)
	if err != nil {
		return
	}
	reuse := remote.Query().Get("reuse")
	var reusable *ReusableRWC
	if len(reuse) > 0 {
		c.runningLck.Lock()
		old, ok := c.running[reuse]
		delete(c.running, reuse)
		c.runningLck.Unlock()
		if ok {
			log.D("CmdDialer reusing session by %v->%p", reuse, old)
			old.Resume()
			fmt.Fprintf(old, "\n")
			raw = old
			return
		}
	}
	runnable := remote.Query().Get("exec")
	log.D("CmdDialer dial to cmd:%v", runnable)
	var cmdReader io.Reader
	var cmdWriter io.Writer
	var cmdCloser func() error
	var cmdStart func() error
	switch runtime.GOOS {
	case "windows":
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
	default:
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
	}
	//
	lc := remote.Query().Get("LC")
	if len(lc) < 1 {
		lc = c.LC
	}
	var combined *CombinedRWC
	switch lc {
	case "zh_CN.GBK":
		combined = &CombinedRWC{
			Reader: transform.NewReader(cmdReader, simplifiedchinese.GBK.NewDecoder()),
			Writer: NewCmdStdinWriter(transform.NewWriter(cmdWriter, simplifiedchinese.GBK.NewEncoder()), c.Replace, c.CloseTag),
			Closer: cmdCloser,
		}
	case "zh_CN.GB18030":
		combined = &CombinedRWC{
			Reader: transform.NewReader(cmdReader, simplifiedchinese.GB18030.NewDecoder()),
			Writer: NewCmdStdinWriter(transform.NewWriter(cmdWriter, simplifiedchinese.GB18030.NewEncoder()), c.Replace, c.CloseTag),
			Closer: cmdCloser,
		}
	default:
		combined = &CombinedRWC{
			Reader: cmdReader,
			Writer: NewCmdStdinWriter(cmdWriter, c.Replace, c.CloseTag),
			Closer: cmdCloser,
		}
	}
	err = cmdStart()
	if err == nil {
		reusable = NewReusableRWC(combined)
		reusable.Name = reuse
		reusable.Reused = len(reuse) > 0 && c.Reuse > 0
		reusable.OnPaused = c.onCmdPaused
		raw = reusable
		if pipe != nil {
			err = reusable.Pipe(pipe)
		}
	}
	return
}

//Shutdown the dialer
func (c *CmdDialer) Shutdown() (err error) {
	c.loopRunning = false
	return
}

func (c *CmdDialer) String() string {
	return "Cmd"
}

//CombinedRWC is an implementation of io.ReadWriteClose to combined reader/writer/closer
type CombinedRWC struct {
	io.Reader
	io.Writer
	Closer func() error
	closed uint32
}

//Close will call closer only once
func (c *CombinedRWC) Close() (err error) {
	if !atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		return fmt.Errorf("ReusableRWC is closed")
	}
	if c.Closer != nil {
		err = c.Closer()
	}
	return
}

//ReusableRWC
type ReusableRWC struct {
	Raw      io.ReadWriteCloser
	paused   uint32
	piped    uint32
	Name     string
	Reused   bool
	Last     int64
	OnPaused func(r *ReusableRWC)
}

func NewReusableRWC(raw io.ReadWriteCloser) (reusable *ReusableRWC) {
	reusable = &ReusableRWC{
		Raw:  raw,
		Last: util.Now(),
	}
	return
}

func (r *ReusableRWC) Write(p []byte) (n int, err error) {
	if r.Reused && atomic.LoadUint32(&r.paused) == 1 {
		err = fmt.Errorf("ReusableRWC is paused")
		return
	}
	n, err = r.Raw.Write(p)
	if err != nil {
		r.Reused = false
	}
	return
}

func (r *ReusableRWC) Read(b []byte) (n int, err error) {
	if r.Reused && atomic.LoadUint32(&r.paused) == 1 {
		err = fmt.Errorf("ReusableRWC is paused")
		return
	}
	n, err = r.Raw.Read(b)
	if err != nil {
		r.Reused = false
	}
	return
}

func (r *ReusableRWC) Close() (err error) {
	if r.Reused {
		if atomic.CompareAndSwapUint32(&r.paused, 0, 1) {
			r.Raw.Write([]byte("\n"))
			r.OnPaused(r)
			return
		}
		err = fmt.Errorf("ReusableRWC is paused")
	} else {
		err = r.Raw.Close()
	}
	return
}

func (r *ReusableRWC) Resume() (err error) {
	if r.Reused {
		if atomic.CompareAndSwapUint32(&r.paused, 1, 0) {
			r.Last = util.Now()
			return
		}
		err = fmt.Errorf("ReusableRWC is running")
	}
	return
}

func (r *ReusableRWC) Destory() (err error) {
	r.Reused = false
	err = r.Raw.Close()
	return
}

func (r *ReusableRWC) Pipe(raw io.ReadWriteCloser) (err error) {
	if atomic.CompareAndSwapUint32(&r.piped, 0, 1) {
		go r.copyAndClose(r, raw)
		go r.copyAndClose(raw, r)
	} else {
		err = fmt.Errorf("piped")
	}
	return
}

func (r *ReusableRWC) copyAndClose(src io.ReadWriteCloser, dst io.ReadWriteCloser) {

	io.Copy(dst, src)
	dst.Close()
	src.Close()
}
