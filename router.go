package bsck

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Centny/gwf/util"
)

const (
	//CmdLogin is the command of login to master
	CmdLogin = 10
	//CmdLoginBack is the command of login return from master
	CmdLoginBack = 11
	//CmdDial is the command of tcp dial by router
	CmdDial = 100
	//CmdDialBack is the command of tcp dial back from master/slaver
	CmdDialBack = 101
	//CmdData is the command of transfter tcp data
	CmdData = 110
	//CmdClosed is the command of tcp closed.
	CmdClosed = 120
	//CmdHeartbeat is the command of heartbeat on slaver/master
	CmdHeartbeat = 130
)

const (
	//ConnTypeRaw is the type of raw connection
	ConnTypeRaw = 100
	//ConnTypeChannel is the type of channel connection
	ConnTypeChannel = 200
)

func cmdString(cmd byte) string {
	switch cmd {
	case CmdLogin:
		return "Login"
	case CmdLoginBack:
		return "LoginBack"
	case CmdDial:
		return "Dial"
	case CmdDialBack:
		return "DialBack"
	case CmdData:
		return "Data"
	case CmdClosed:
		return "Closed"
	case CmdHeartbeat:
		return "Heartbeat"
	default:
		return fmt.Sprintf("unknow(%v)", cmd)
	}
}

//Conn is the interface that wraps the connection will be running on Router.
//
//ID is the unique of connection
//
//Name is the channel name, it will be used when join current connection to channel
//
//Index is the channel index, it will be used when join current connection to channel.
//
//Type is the connection type by ConnTypeRaw/ConnTypeChannel
type Conn interface {
	//the basic ReadWriteCloser
	io.ReadWriteCloser
	//the connection id
	ID() uint64
	//the channel name
	Name() string
	//the channel index.
	Index() int
	//the connection type
	Type() int
	//conn context getter
	Context() interface{}
	//conn context setter
	SetContext(v interface{})
}

type ConnectedWaiter interface {
	Wait() bool
	Ready()
}

//CmdReader is the interface that wraps the basic ReadCmd method.
//
//ReadCmd will read one command to b buffer, return the command length
//
//the command mode is length(4 byte)|data
type CmdReader interface {
	ReadCmd(b []byte) (n uint32, err error)
}

//RawConn is an implementation of the Conn interface for raw network connections.
type RawConn struct {
	//the raw connection
	Raw       io.ReadWriteCloser
	sid       uint64
	uri       string
	connected chan int
	closed    uint32
	lck       chan int
	context   interface{}
}

// NewRawConn returns a new RawConn by raw connection/session id/uri
func NewRawConn(raw io.ReadWriteCloser, sid uint64, uri string) (conn *RawConn) {
	conn = &RawConn{
		Raw:       raw,
		sid:       sid,
		uri:       uri,
		connected: make(chan int, 1),
		lck:       make(chan int, 1),
	}
	conn.lck <- 1
	return
}

func (r *RawConn) Write(p []byte) (n int, err error) {
	n = len(p)
	_, err = r.Raw.Write(p[13:])
	return
}

func (r *RawConn) Read(b []byte) (n int, err error) {
	panic("not supported")
}

//ReadCmd is an implementation of CmdReader
func (r *RawConn) ReadCmd(b []byte) (n uint32, err error) {
	var readed int
	readed, err = r.Raw.Read(b[13:])
	if err == nil {
		binary.BigEndian.PutUint32(b, uint32(readed+9))
		b[4] = CmdData
		binary.BigEndian.PutUint64(b[5:], r.sid)
		n = uint32(readed) + 13
	}
	return
}

//Close will close the raw connection
func (r *RawConn) Close() (err error) {
	<-r.lck
	if r.closed < 1 {
		close(r.connected)
		r.closed = 1
	}
	r.lck <- 1
	err = r.Raw.Close()
	return
}

func (r *RawConn) Wait() bool {
	v := <-r.connected
	return v > 0
}

func (r *RawConn) Ready() {
	<-r.lck
	if r.closed < 1 {
		r.connected <- 1
	}
	r.lck <- 1
}

//ID is an implementation of Conn
func (r *RawConn) ID() uint64 {
	return r.sid
}

//Name is an implementation of Conn
func (r *RawConn) Name() string {
	return ""
}

//Index is an implementation of Conn
func (r *RawConn) Index() int {
	return 0
}

//Type is an implementation of Conn
func (r *RawConn) Type() int {
	return ConnTypeRaw
}

//Context is conn context getter
func (r *RawConn) Context() interface{} {
	return r.context
}

//SetContext is conn context setter
func (r *RawConn) SetContext(v interface{}) {
	r.context = v
}

func (r *RawConn) String() string {
	ts := strings.Split(r.uri, "->")
	uri, err := url.Parse(ts[len(ts)-1])
	if err != nil {
		return fmt.Sprintf("raw{error:%v,info:%v}", err, r.Raw)
	}
	router := uri.Query().Get("router")
	router = strings.SplitN(router, "?", 2)[0]
	args := uri.Query()
	args.Del("router")
	args.Del("cols")
	args.Del("rows")
	uri.RawQuery = args.Encode()
	return fmt.Sprintf("raw{uri:%v,router:%v,info:%v}", uri, router, r.Raw)
}

//Channel is an implementation of the Conn interface for channel network connections.
type Channel struct {
	io.ReadWriteCloser //the raw connection
	cid                uint64
	name               string
	index              int
	context            interface{}
	LoginArgs          interface{}
	Heartbeat          int64
}

//ID is an implementation of Conn
func (c *Channel) ID() uint64 {
	return c.cid
}

//Name is an implementation of Conn
func (c *Channel) Name() string {
	return c.name
}

//Index is an implementation of Conn
func (c *Channel) Index() int {
	return c.index
}

//Type is an implementation of Conn
func (c *Channel) Type() int {
	return ConnTypeChannel
}

//Context is conn context getter
func (c *Channel) Context() interface{} {
	return c.context
}

//SetContext is conn context setter
func (c *Channel) SetContext(v interface{}) {
	c.context = v
}

func (c *Channel) String() string {
	return fmt.Sprintf("channel{name:%v,index:%v,cid:%v,info:%v}", c.name, c.index, c.cid, c.ReadWriteCloser)
}

type bondChannel struct {
	channels   map[int]Conn
	used       map[int]uint64
	channelLck sync.RWMutex
}

func newBondChannel() *bondChannel {
	return &bondChannel{
		channels:   map[int]Conn{},
		used:       map[int]uint64{},
		channelLck: sync.RWMutex{},
	}
}

//TableRouter is the router table item
type TableRouter []interface{}

//Next will return next connection and session id
func (t TableRouter) Next(conn Conn) (target Conn, rsid uint64) {
	if t[0] == conn {
		target = t[2].(Conn)
		rsid = t[3].(uint64)
	} else if t[2] == conn {
		target = t[0].(Conn)
		rsid = t[1].(uint64)
	}
	return
}

func (t TableRouter) String() string {
	return fmt.Sprintf("%v %v <-> %v %v", t[0], t[1], t[2], t[3])
}

//Handler is the interface that wraps the handler of Router.
type Handler interface {
	//dial raw connection
	DialRaw(sid uint64, uri string) (raw Conn, err error)
	//check connection is login
	WhetherConnLogin(channel Conn) bool
	//on connection dial uri
	OnConnDialURI(channel Conn, conn string, parts []string) (err error)
	//on connection login
	OnConnLogin(channel Conn, args string) (name string, index int, err error)
	//on connection close
	OnConnClose(raw Conn) error
}

//Router is an implementation of the router control
type Router struct {
	Name            string        //current router name
	BufferSize      uint32        //buffer size of connection runner
	Heartbeat       time.Duration //the delay of heartbeat
	Handler         Handler       //the router handler
	connectSequence uint64
	channel         map[string]*bondChannel
	channelLck      sync.RWMutex
	table           map[string]TableRouter
	tableLck        sync.RWMutex
}

//NewRouter will return new Router by name
func NewRouter(name string) (router *Router) {
	router = &Router{
		Name:       name,
		channel:    map[string]*bondChannel{},
		channelLck: sync.RWMutex{},
		table:      map[string]TableRouter{},
		tableLck:   sync.RWMutex{},
		BufferSize: 1024 * 1024,
		Heartbeat:  5 * time.Second,
		Handler:    nil,
	}
	return
}

//StartHeartbeat will start the hearbeat on slaver/master
func (r *Router) StartHeartbeat() {
	if r.Heartbeat > 0 {
		InfoLog("Router(%v) start heartbeat by %v delay", r.Name, r.Heartbeat)
		go r.loopHeartbeat()
	}
}

//Accept one raw connecton as channel,
//it will auth the raw connecton by ACL.
func (r *Router) Accept(raw io.ReadWriteCloser) {
	channel := &Channel{
		ReadWriteCloser: raw,
		cid:             atomic.AddUint64(&r.connectSequence, 1),
	}
	go r.loopReadRaw(channel, r.BufferSize)
}

//Register one logined raw connecton to channel,
func (r *Router) Register(channel Conn) {
	r.addChannel(channel)
	go r.loopReadRaw(channel, r.BufferSize)
}

//Bind one raw connection to channel by session.
func (r *Router) Bind(src Conn, srcSid uint64, dst Conn, dstSid uint64) {
	r.addTable(src, srcSid, dst, dstSid)
	go r.loopReadRaw(dst, r.BufferSize)
}

func (r *Router) addChannel(channel Conn) {
	r.channelLck.Lock()
	bond := r.channel[channel.Name()]
	if bond == nil {
		bond = newBondChannel()
	}
	bond.channelLck.Lock()
	old, _ := bond.channels[channel.Index()]
	bond.channels[channel.Index()] = channel
	bond.channelLck.Unlock()
	r.channel[channel.Name()] = bond
	r.channelLck.Unlock()
	if old != nil {
		old.Close()
	}
	InfoLog("Router(%v) add channel(%v) success", r.Name, channel)
}

//UniqueSid will return new session id
func (r *Router) UniqueSid() (sid uint64) {
	sid = atomic.AddUint64(&r.connectSequence, 1)
	return
}

//SelectChannel will pick one channel by name.
func (r *Router) SelectChannel(name string) (dst Conn, err error) {
	r.channelLck.RLock()
	defer r.channelLck.RUnlock()
	bond := r.channel[name]
	if bond == nil || len(bond.channels) < 1 {
		err = fmt.Errorf("channel not exist by name(%v)", name)
		return
	}
	var index int
	var min uint64
	for i, c := range bond.channels {
		used := bond.used[i]
		if dst != nil && used > min {
			continue
		}
		dst = c
		min = used
		index = i
	}
	bond.used[index]++
	return
}

func (r *Router) addTable(src Conn, srcSid uint64, dst Conn, dstSid uint64) {
	r.tableLck.Lock()
	router := []interface{}{src, srcSid, dst, dstSid}
	r.table[fmt.Sprintf("%v-%v", src.ID(), srcSid)] = router
	r.table[fmt.Sprintf("%v-%v", dst.ID(), dstSid)] = router
	r.tableLck.Unlock()
	return
}

func (r *Router) removeTable(conn Conn, sid uint64) TableRouter {
	r.tableLck.Lock()
	defer r.tableLck.Unlock()
	return r.removeTableNoLock(conn, sid)
}

func (r *Router) removeTableNoLock(conn Conn, sid uint64) TableRouter {
	router := r.table[fmt.Sprintf("%v-%v", conn.ID(), sid)]
	if router != nil {
		delete(r.table, fmt.Sprintf("%v-%v", router[0].(Conn).ID(), router[1]))
		delete(r.table, fmt.Sprintf("%v-%v", router[2].(Conn).ID(), router[3]))
	}
	return router
}

func (r *Router) loopReadRaw(channel Conn, bufferSize uint32) {
	var err error
	var last int64
	var length uint32
	buf := make([]byte, bufferSize)
	InfoLog("Router(%v) the reader(%v) is starting", r.Name, channel)
	var procReadCmd = func(b []byte) (n uint32, err error) {
		n, err = readCmd(channel, buf, &last)
		return
	}
	if cmdReader, ok := channel.(CmdReader); ok {
		procReadCmd = cmdReader.ReadCmd
	}
	for {
		length, err = procReadCmd(buf)
		if err != nil {
			break
		}
		if length < 13 {
			ErrorLog("Router(%v) receive invalid frame(length:%v) from %v", length, channel)
			break
		}
		if ShowLog > 1 {
			DebugLog("Router(%v) read one command(%v,%v) from %v", r.Name, cmdString(buf[4]), length, channel)
		}
		switch buf[4] {
		case CmdLogin:
			err = r.procLogin(channel, buf, length)
		}
		if err != nil {
			break
		}
		if !r.Handler.WhetherConnLogin(channel) {
			err = fmt.Errorf("not access")
			break
		}
		switch buf[4] {
		case CmdDial:
			err = r.procDial(channel, buf, length)
		case CmdDialBack:
			err = r.procDialBack(channel, buf, length)
		case CmdData:
			err = r.procChannelData(channel, buf, length)
		case CmdClosed:
			err = r.procClosed(channel, buf, length)
		case CmdHeartbeat:
			err = r.procHeartbeat(channel, buf, length)
		default:
			err = fmt.Errorf("not supported cmd(%v)", buf[4])
		}
		if err != nil {
			break
		}
	}
	channel.Close()
	InfoLog("Router(%v) the reader(%v) is stopped by %v", r.Name, channel, err)
	r.channelLck.Lock()
	bond := r.channel[channel.Name()]
	if bond != nil {
		bond.channelLck.Lock()
		bond.channels[channel.Index()] = channel
		delete(bond.channels, channel.Index())
		bond.channelLck.Unlock()
		InfoLog("Router(%v) remove channel(%v) success", r.Name, channel)
	}
	r.channelLck.Unlock()
	//
	r.tableLck.Lock()
	if channel.Type() == ConnTypeRaw {
		// router := r.table[fmt.Sprintf("%v-%v", channel.ID(), channel.ID())]
		router := r.removeTableNoLock(channel, channel.ID())
		if router != nil {
			target, rsid := router.Next(channel)
			writeCmd(target, buf, CmdClosed, rsid, []byte(err.Error()))
		}
	} else {
		for _, router := range r.table {
			target, rsid := router.Next(channel)
			if target == nil {
				continue
			}
			if target.Type() == ConnTypeRaw {
				target.Close()
			} else {
				writeCmd(target, buf, CmdClosed, rsid, []byte(err.Error()))
			}
			r.removeTableNoLock(target, rsid)
		}
	}
	r.tableLck.Unlock()
	r.Handler.OnConnClose(channel)
}

func (r *Router) loopHeartbeat() {
	data := []byte("ping...")
	length := uint32(len(data) + 13)
	buf := make([]byte, length)
	binary.BigEndian.PutUint32(buf, length-4)
	buf[4] = CmdHeartbeat
	binary.BigEndian.PutUint32(buf[5:], 0)
	copy(buf[13:], data)
	last := time.Now().Local().UnixNano() / 1e6
	showed := false
	for {
		now := time.Now().Local().UnixNano() / 1e6
		r.channelLck.RLock()
		for name, bond := range r.channel {
			if now-last > 30000 {
				InfoLog("Router(%v) send heartbeat to %v", r.Name, name)
				showed = true
			}
			bond.channelLck.RLock()
			for _, channel := range bond.channels {
				channel.Write(buf)
			}
			bond.channelLck.RUnlock()
		}
		r.channelLck.RUnlock()
		if showed {
			last = time.Now().Local().UnixNano() / 1e6
			showed = false
		}
		time.Sleep(r.Heartbeat)
	}
}

func (r *Router) procLogin(conn Conn, buf []byte, length uint32) (err error) {
	channel, ok := conn.(*Channel)
	if !ok {
		ErrorLog("Router(%v) proc login error with connection is not channel", r.Name)
		err = fmt.Errorf("connection is not channel")
		conn.Close()
		return
	}
	name, index, err := r.Handler.OnConnLogin(channel, string(buf[13:length]))
	if err != nil {
		ErrorLog("Router(%v) proc login fail with %v", r.Name, err)
		err = writeCmd(conn, buf, CmdLoginBack, 0, []byte(err.Error()))
		conn.Close()
		return
	}
	channel.name = name
	channel.index = index
	r.addChannel(channel)
	writeCmd(channel, buf, CmdLoginBack, 0, []byte("OK:"+r.Name))
	InfoLog("Router(%v) the channel(%v,%v) is login success on %v", r.Name, name, index, channel)
	return
}

func (r *Router) procRawDial(channel Conn, sid uint64, conn, uri string) (err error) {
	buf := make([]byte, 10240)
	dstSid := atomic.AddUint64(&r.connectSequence, 1)
	raw, cerr := r.Handler.DialRaw(dstSid, uri)
	if cerr != nil {
		DebugLog("Router(%v) dail to %v fail on channel(%v) by %v", r.Name, conn, channel, cerr)
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte(fmt.Sprintf("dial to uri(%v) fail with %v", uri, cerr)))
		return
	}
	DebugLog("Router(%v) dail to %v success on channel(%v)", r.Name, conn, channel)
	err = writeCmd(channel, buf, CmdDialBack, sid, []byte("OK"))
	if err == nil {
		r.Bind(channel, sid, raw, dstSid)
	}
	return
}

func (r *Router) procDial(channel Conn, buf []byte, length uint32) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	conn := string(buf[13:length])
	DebugLog("Router(%v) proc dail to %v on channel(%v)", r.Name, conn, channel)
	path := strings.SplitN(conn, "@", 2)
	if len(path) < 2 {
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte(fmt.Sprintf("invalid uri(%v)", conn)))
		return
	}
	parts := strings.SplitN(path[1], "->", 2)
	err = r.Handler.OnConnDialURI(channel, conn, parts)
	if err != nil {
		WarnLog("Router(%v) process dial uri event to %v on channel(%v) fail with %v", r.Name, conn, channel, err)
		return
	}
	if len(parts) < 2 {
		go r.procRawDial(channel, sid, conn, parts[0])
		return
	}
	next := parts[0]
	dst, err := r.SelectChannel(next)
	if err != nil {
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte(err.Error()))
		return
	}
	dstSid := atomic.AddUint64(&r.connectSequence, 1)
	r.addTable(channel, sid, dst, dstSid)
	werr := writeCmd(dst, buf, CmdDial, dstSid, []byte(path[0]+"->"+next+"@"+parts[1]))
	if werr != nil {
		WarnLog("Router(%v) send dial to channel(%v) fail with %v", r.Name, dst, werr)
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte(werr.Error()))
		r.removeTable(channel, sid)
	}
	return
}

func (r *Router) procDialBack(channel Conn, buf []byte, length uint32) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	DebugLog("Router(%v) proc dail back by %v on channel(%v)", r.Name, sid, channel)
	r.tableLck.RLock()
	router := r.table[fmt.Sprintf("%v-%v", channel.ID(), sid)]
	r.tableLck.RUnlock()
	if router == nil {
		err = writeCmd(channel, buf, CmdClosed, sid, []byte("closed"))
		return
	}
	target, rsid := router.Next(channel)
	if target.Type() == ConnTypeRaw {
		msg := string(buf[13:length])
		if msg == "OK" {
			InfoLog("Router(%v) dail to %v success", r.Name, target)
			r.Bind(channel, sid, target, target.ID())
			if waiter, ok := target.(ConnectedWaiter); ok {
				waiter.Ready()
			}
		} else {
			InfoLog("Router(%v) dail to %v fail with %v", r.Name, target, msg)
			r.removeTable(channel, sid)
			target.Close()
		}
	} else {
		binary.BigEndian.PutUint64(buf[5:], rsid)
		_, werr := target.Write(buf[:length])
		if werr != nil {
			err = writeCmd(channel, buf, CmdClosed, sid, []byte("closed"))
		}
	}
	return
}

func (r *Router) procChannelData(channel Conn, buf []byte, length uint32) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	r.tableLck.RLock()
	router := r.table[fmt.Sprintf("%v-%v", channel.ID(), sid)]
	r.tableLck.RUnlock()
	if router != nil {
		target, rsid := router.Next(channel)
		binary.BigEndian.PutUint64(buf[5:], rsid)
		_, err = target.Write(buf[:length])
	}
	if router == nil || err != nil {
		err = writeCmd(channel, buf, CmdClosed, sid, []byte("closed"))
	}
	return
}

func (r *Router) procClosed(channel Conn, buf []byte, length uint32) (err error) {
	message := string(buf[13:length])
	sid := binary.BigEndian.Uint64(buf[5:])
	DebugLog("Router(%v) the session(%v) is closed by %v", r.Name, sid, message)
	router := r.removeTable(channel, sid)
	if router != nil {
		target, rsid := router.Next(channel)
		if target.Type() == ConnTypeRaw {
			target.Close()
		} else {
			writeCmd(target, buf, CmdClosed, rsid, []byte(message))
		}
	}
	return
}

func (r *Router) procHeartbeat(conn Conn, buf []byte, length uint32) (err error) {
	if channel, ok := conn.(*Channel); ok {
		channel.Heartbeat = time.Now().Local().UnixNano() / 1e6
	}
	return
}

//Dial to remote by uri and bind channel to raw connection.
//
//return the session id
func (r *Router) Dial(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	sid, _, err = r.DialConn(uri, raw)
	return
}

func (r *Router) DialConn(uri string, raw io.ReadWriteCloser) (sid uint64, conn *RawConn, err error) {
	parts := strings.SplitN(uri, "->", 2)
	if len(parts) < 2 {
		err = fmt.Errorf("invalid uri(%v), it must like x->y", uri)
		return
	}
	channel, err := r.SelectChannel(parts[0])
	if err != nil {
		return
	}
	DebugLog("Router(%v) start dail to %v on channel(%v)", r.Name, uri, channel)
	sid = atomic.AddUint64(&r.connectSequence, 1)
	conn = NewRawConn(raw, sid, uri)
	r.addTable(channel, sid, conn, sid)
	err = writeCmd(channel, make([]byte, 4096), CmdDial, sid, []byte(fmt.Sprintf("%v@%v", parts[0], parts[1])))
	if err != nil {
		r.removeTable(channel, sid)
	}
	return
}

func (r *Router) SyncDial(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	sid, conn, err := r.DialConn(uri, raw)
	if err == nil && !conn.Wait() {
		err = fmt.Errorf("connect reseted")
	}
	return
}

//JoinConn will add channel by the connected connection
func (r *Router) JoinConn(conn io.ReadWriteCloser, index int, args interface{}) (err error) {
	data, _ := json.Marshal(args)
	buf := make([]byte, 1024)
	err = writeCmd(conn, buf, CmdLogin, 0, data)
	if err != nil {
		WarnLog("Router(%v) send login to %v fail with %v", r.Name, conn, err)
		return
	}
	length, err := readCmd(conn, buf, nil)
	if err != nil {
		WarnLog("Router(%v) read login back from %v fail with %v", r.Name, conn, err)
		return
	}
	msg := string(buf[13:length])
	if !strings.HasPrefix(msg, "OK:") {
		err = fmt.Errorf("%v", msg)
		WarnLog("Router(%v) login to %v fail with %v", r.Name, conn, err)
		return
	}
	remoteName := strings.TrimPrefix(msg, "OK:")
	channel := &Channel{
		ReadWriteCloser: NewBufferConn(conn, 4096),
		cid:             atomic.AddUint64(&r.connectSequence, 1),
		name:            remoteName,
		index:           index,
		LoginArgs:       args,
	}
	r.Register(channel)
	InfoLog("Router(%v) login to %v success, bind to %v,%v", r.Name, conn, remoteName, index)
	return
}

//Close all channel
func (r *Router) Close() (err error) {
	r.channelLck.Lock()
	for _, bond := range r.channel {
		bond.channelLck.Lock()
		for _, channel := range bond.channels {
			channel.Close()
		}
		bond.channelLck.Unlock()
	}
	r.channelLck.Unlock()
	return
}

//State return the current state of router
func (r *Router) State() (state util.Map) {
	state = util.Map{}
	//
	channels := util.Map{}
	r.channelLck.RLock()
	for name, bond := range r.channel {
		channel := util.Map{}
		for idx, con := range bond.channels {
			info := util.Map{
				"connect": fmt.Sprintf("%v", con),
				"used":    bond.used[idx],
			}
			if c, ok := con.(*Channel); ok {
				info["heartbeat"] = c.Heartbeat
			}
			channel[fmt.Sprintf("_%v", idx)] = info
		}
		channels[name] = channel
	}
	r.channelLck.RUnlock()
	state["channels"] = channels
	//
	table := []string{}
	r.tableLck.RLock()
	added := map[string]bool{}
	for _, t := range r.table {
		key := fmt.Sprintf("%p", t)
		if added[key] {
			continue
		}
		added[key] = true
		table = append(table, t.String())
	}
	r.tableLck.RUnlock()
	state["table"] = table
	return
}
