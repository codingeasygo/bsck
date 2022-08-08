package bsck

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
	"github.com/codingeasygo/util/xmap"
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
		return fmt.Sprintf("unknown(%v)", cmd)
	}
}

type RawValuable interface {
	RawValue() interface{}
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
	frame.ReadWriteCloser
	//the connection id
	ID() uint64
	//the channel name
	Name() string
	//the channel index.
	Index() int
	//the connection type
	Type() int
	//conn context getter
	Context() xmap.M
}

//ReadyWaiter interface for ready waiter
type ReadyWaiter interface {
	Wait() error
	Ready(failed error, next func(err error))
}

type readDeadlinable interface {
	SetReadDeadline(t time.Time) error
}

type writeDeadlinable interface {
	SetWriteDeadline(t time.Time) error
}

//RawConn is an implementation of the Conn interface for raw network connections.
type RawConn struct {
	//the raw connection
	*frame.BaseReadWriteCloser
	Raw          io.ReadWriteCloser
	name         string
	sid          uint64
	uri          string
	context      xmap.M
	buffer       []byte
	readTimeout  time.Duration
	writeTimeout time.Duration
	ready        int
	closed       int
	failed       error
	readyLocker  sync.RWMutex
	closeLocker  sync.RWMutex
}

// NewRawConn returns a new RawConn by raw connection/session id/uri
func NewRawConn(name string, raw io.ReadWriteCloser, bufferSize int, sid uint64, uri string) (conn *RawConn) {
	conn = &RawConn{
		BaseReadWriteCloser: frame.NewReadWriteCloser(raw, bufferSize),
		Raw:                 raw,
		name:                name,
		sid:                 sid,
		uri:                 uri,
		readyLocker:         sync.RWMutex{},
		closeLocker:         sync.RWMutex{},
		buffer:              make([]byte, bufferSize),
		context:             xmap.M{},
	}
	conn.readyLocker.Lock()
	return
}

func (r *RawConn) Write(p []byte) (n int, err error) {
	// panic("not supported")
	n, err = r.Raw.Write(p)
	return
}

func (r *RawConn) Read(b []byte) (n int, err error) {
	// panic("not supported")
	n, err = r.Raw.Read(b)
	return
}

func (r *RawConn) ReadFrom(reader io.Reader) (w int64, err error) {
	w, err = io.CopyBuffer(r.Raw, reader, make([]byte, r.BufferSize))
	return
}

func (r *RawConn) WriteTo(w io.Writer) (n int64, err error) {
	n, err = io.CopyBuffer(w, r.Raw, make([]byte, r.BufferSize))
	return
}

//ReadFrame will read frame from raw
func (r *RawConn) ReadFrame() (frame []byte, err error) {
	if timeout, ok := r.Raw.(readDeadlinable); r.readTimeout > 0 && ok {
		timeout.SetReadDeadline(time.Now().Add(r.readTimeout))
	}
	n, err := r.Raw.Read(r.buffer[13:])
	if err != nil {
		return
	}
	binary.BigEndian.PutUint32(r.buffer, uint32(n+13))
	r.buffer[4] = CmdData
	binary.BigEndian.PutUint64(r.buffer[5:], r.sid)
	frame = r.buffer[:n+13]
	return
}

//WriteFrame will write frame to raw
func (r *RawConn) WriteFrame(buffer []byte) (n int, err error) {
	if len(buffer) < 14 {
		err = fmt.Errorf("error frame")
		return
	}
	if timeout, ok := r.Raw.(writeDeadlinable); r.writeTimeout > 0 && ok {
		timeout.SetWriteDeadline(time.Now().Add(r.readTimeout))
	}
	n, err = r.Raw.Write(buffer[13:])
	n += 13
	return
}

//SetReadTimeout is read timeout setter
func (r *RawConn) SetReadTimeout(timeout time.Duration) {
	r.readTimeout = timeout
}

//SetWriteTimeout is write timeout setter
func (r *RawConn) SetWriteTimeout(timeout time.Duration) {
	r.writeTimeout = timeout
}

//SetTimeout is read/write timeout setter
func (r *RawConn) SetTimeout(timeout time.Duration) {
	r.readTimeout = timeout
	r.writeTimeout = timeout
}

//Close will close the raw connection
func (r *RawConn) Close() (err error) {
	if _, ok := r.Raw.(ReadyWaiter); ok {
		return r.Raw.Close()
	}
	r.closeLocker.Lock()
	if r.closed > 0 {
		r.closeLocker.Unlock()
		return
	}
	r.closed = 1
	ready := r.ready
	r.closeLocker.Unlock()
	//
	if ready < 1 {
		r.readyLocker.Unlock()
	}
	err = r.Raw.Close()
	return
}

//Wait is ConnectedWaiter impl
func (r *RawConn) Wait() error {
	if waiter, ok := r.Raw.(ReadyWaiter); ok {
		return waiter.Wait()
	}
	r.readyLocker.Lock()
	_ = r //do nothing for warning
	r.readyLocker.Unlock()
	return r.failed
}

//Ready is ConnectedWaiter impl
func (r *RawConn) Ready(failed error, next func(err error)) {
	if waiter, ok := r.Raw.(ReadyWaiter); ok {
		waiter.Ready(failed, next)
		return
	}
	r.closeLocker.Lock()
	if r.closed > 0 {
		r.closeLocker.Unlock()
		return
	}
	r.ready = 1
	r.closeLocker.Unlock()
	//
	r.failed = failed
	r.readyLocker.Unlock()
	if failed == nil && next != nil {
		go next(nil)
	}
}

//ID is an implementation of Conn
func (r *RawConn) ID() uint64 {
	return r.sid
}

//Name is an implementation of Conn
func (r *RawConn) Name() string {
	return r.name
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
func (r *RawConn) Context() xmap.M {
	return r.context
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
	return fmt.Sprintf("raw{sid:%v,uri:%v,router:%v,info:%v}", r.sid, uri, router, r.Raw)
}

//Channel is an implementation of the Conn interface for channel network connections.
type Channel struct {
	frame.ReadWriteCloser //the raw connection
	cid                   uint64
	name                  string
	index                 int
	context               xmap.M
	Heartbeat             time.Time
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
func (c *Channel) Context() xmap.M {
	return c.context
}

// func (c *Channel) Close() (err error) {
// 	err = c.ReadWriteCloser.Close()
// 	DebugLog("Channel %v is closed by \n%v", c, debug.CallStack())
// 	return
// }

func (c *Channel) RawValue() (raw interface{}) {
	valuable, ok := c.ReadWriteCloser.(RawValuable)
	if ok {
		raw = valuable.RawValue()
	}
	return
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
func (t TableRouter) Next(conn Conn) (target Conn, sid uint64) {
	if t[0] == conn {
		target = t[2].(Conn)
		sid = t[3].(uint64)
	} else if t[2] == conn {
		target = t[0].(Conn)
		sid = t[1].(uint64)
	}
	return
}

func (t TableRouter) String() string {
	return fmt.Sprintf("%v %v <-> %v %v", t[0], t[1], t[2], t[3])
}

//Handler is the interface that wraps the handler of Router.
type Handler interface {
	//dial raw connection
	DialRaw(channel Conn, sid uint64, uri string) (raw Conn, err error)
	//on connection dial uri
	OnConnDialURI(channel Conn, conn string, parts []string) (err error)
	//on connection login
	OnConnLogin(channel Conn, args string) (name string, index int, result xmap.M, err error)
	//on connection close
	OnConnClose(raw Conn) error
}

//Router is an implementation of the router control
type Router struct {
	Name            string        //current router name
	BufferSize      int           //buffer size of connection runner
	Heartbeat       time.Duration //the delay of heartbeat
	Handler         Handler       //the router handler
	connectSequence uint64
	channel         map[string]*bondChannel
	channelLck      sync.RWMutex
	table           map[string]TableRouter
	tableLck        sync.RWMutex
	rawConn         map[string][]io.ReadWriteCloser
	rawLck          sync.RWMutex
}

//NewRouter will return new Router by name
func NewRouter(name string) (router *Router) {
	router = &Router{
		Name:       name,
		channel:    map[string]*bondChannel{},
		channelLck: sync.RWMutex{},
		table:      map[string]TableRouter{},
		tableLck:   sync.RWMutex{},
		rawConn:    map[string][]io.ReadWriteCloser{},
		rawLck:     sync.RWMutex{},
		BufferSize: 1024,
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

//Accept one raw connection as channel,
//it will auth the raw connection by ACL.
func (r *Router) Accept(raw frame.ReadWriteCloser) {
	channel := &Channel{
		ReadWriteCloser: raw,
		cid:             atomic.AddUint64(&r.connectSequence, 1),
		context:         xmap.M{},
		Heartbeat:       time.Now(),
	}
	go r.loopReadRaw(channel)
}

//Accept one raw connection as channel, and loop read it
//it will auth the raw connection by ACL.
func (r *Router) AcceptSync(raw frame.ReadWriteCloser) {
	channel := &Channel{
		ReadWriteCloser: raw,
		cid:             atomic.AddUint64(&r.connectSequence, 1),
		context:         xmap.M{},
		Heartbeat:       time.Now(),
	}
	r.loopReadRaw(channel)
}

//Register one login raw connection to channel,
func (r *Router) Register(channel Conn) {
	r.addChannel(channel)
	go r.loopReadRaw(channel)
}

// //Bind one raw connection to channel by session.
// func (r *Router) Bind(src Conn, srcSid uint64, dst Conn, dstSid uint64, conn string) {
// 	r.addTable(src, srcSid, dst, dstSid, conn)
// 	go r.loopReadRaw(dst)
// }

func (r *Router) addChannel(channel Conn) {
	r.channelLck.Lock()
	bond := r.channel[channel.Name()]
	if bond == nil {
		bond = newBondChannel()
	}
	bond.channelLck.Lock()
	old := bond.channels[channel.Index()]
	bond.channels[channel.Index()] = channel
	bond.channelLck.Unlock()
	r.channel[channel.Name()] = bond
	r.channelLck.Unlock()
	if old != nil {
		old.Close()
		InfoLog("Router(%v) add channel(%v) found old connection, will close %v", r.Name, channel, old)
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
	bond := r.channel[name]
	if bond == nil || len(bond.channels) < 1 {
		err = fmt.Errorf("channel not exist by name(%v)", name)
		r.channelLck.RUnlock()
		return
	}
	r.channelLck.RUnlock()
	bond.channelLck.Lock()
	defer bond.channelLck.Unlock()
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

//CloseChannel will call close on all bond channle by name.
func (r *Router) CloseChannel(name string) (err error) {
	r.channelLck.RLock()
	bond := r.channel[name]
	if bond == nil || len(bond.channels) < 1 {
		r.channelLck.RUnlock()
		return
	}
	r.channelLck.RUnlock()
	bond.channelLck.Lock()
	defer bond.channelLck.Unlock()
	for _, c := range bond.channels {
		c.Close()
	}
	return
}

func (r *Router) addTable(src Conn, srcSid uint64, dst Conn, dstSid uint64, conn string) {
	r.tableLck.Lock()
	router := []interface{}{src, srcSid, dst, dstSid, conn}
	r.table[fmt.Sprintf("%v-%v", src.ID(), srcSid)] = router
	r.table[fmt.Sprintf("%v-%v", dst.ID(), dstSid)] = router
	r.tableLck.Unlock()
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

func (r *Router) loopReadRaw(channel Conn) {
	InfoLog("Router(%v) the reader(%v) is starting", r.Name, channel)
	var err error
	for {
		var buf []byte
		buf, err = channel.ReadFrame()
		if err != nil {
			break
		}
		if len(buf) < 13 {
			ErrorLog("Router(%v) receive invalid frame(length:%v) from %v", len(buf), channel)
			break
		}
		if ShowLog > 1 {
			DebugLog("Router(%v) read one command(%v,%v) from %v", r.Name, cmdString(buf[4]), len(buf), channel)
		}
		switch buf[4] {
		case CmdLogin:
			err = r.procLogin(channel, buf)
		case CmdDial:
			err = r.procDial(channel, buf)
		case CmdDialBack:
			err = r.procDialBack(channel, buf)
		case CmdData:
			err = r.procChannelData(channel, buf)
		case CmdClosed:
			err = r.procClosed(channel, buf)
		case CmdHeartbeat:
			err = r.procHeartbeat(channel, buf)
		default:
			err = fmt.Errorf("not supported cmd(%v)", buf[4])
		}
		if err != nil {
			break
		}
	}
	channel.Close()
	InfoLog("Router(%v) the reader(%v) is stopped by %v", r.Name, channel, err)
	if channel.Type() == ConnTypeChannel {
		r.channelLck.Lock()
		bond := r.channel[channel.Name()]
		if bond != nil {
			bond.channelLck.Lock()
			if bond.channels[channel.Index()] == channel {
				delete(bond.channels, channel.Index())
			}
			bond.channelLck.Unlock()
			if len(bond.channels) < 1 {
				delete(r.channel, channel.Name())
			}
			InfoLog("Router(%v) remove channel(%v) success", r.Name, channel)
		}
		r.channelLck.Unlock()
	}
	//
	running := []io.Closer{}
	r.tableLck.Lock()
	if channel.Type() == ConnTypeRaw {
		// router := r.table[fmt.Sprintf("%v-%v", channel.ID(), channel.ID())]
		router := r.removeTableNoLock(channel, channel.ID())
		if router != nil {
			target, sid := router.Next(channel)
			writeCmd(target, nil, CmdClosed, sid, []byte(err.Error()))
		}
	} else {
		for _, router := range r.table {
			target, sid := router.Next(channel)
			if target == nil {
				continue
			}
			if target.Type() == ConnTypeRaw {
				running = append(running, target)
			} else {
				writeCmd(target, nil, CmdClosed, sid, []byte(err.Error()))
			}
			r.removeTableNoLock(target, sid)
		}
	}
	r.tableLck.Unlock()
	for _, closer := range running {
		closer.Close()
	}
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
	last := time.Now()
	for {
		shouldKeep, shouldClose := []Conn{}, []Conn{}
		r.channelLck.RLock()
		for name, bond := range r.channel {
			if len(bond.channels) < 1 {
				continue
			}
			if time.Since(last) > 30*time.Second {
				InfoLog("Router(%v) send heartbeat to %v", r.Name, name)
			}
			bond.channelLck.RLock()
			for _, channel := range bond.channels {
				if c, ok := channel.(*Channel); ok && time.Since(c.Heartbeat) > 3*r.Heartbeat {
					shouldClose = append(shouldClose, channel)
					WarnLog("Router(%v) send heartbeat to %v channel %v is out of sync, it will be closed by %v", r.Name, name, c, c)
				} else {
					shouldKeep = append(shouldKeep, channel)
				}
			}
			bond.channelLck.RUnlock()
		}
		r.channelLck.RUnlock()
		for _, channel := range shouldKeep {
			channel.WriteFrame(buf)
		}
		for _, channel := range shouldClose {
			channel.Close()
		}
		if time.Since(last) > 30*time.Second {
			last = time.Now()
		}
		time.Sleep(r.Heartbeat)
	}
}

func (r *Router) procLogin(conn Conn, buf []byte) (err error) {
	channel, ok := conn.(*Channel)
	if !ok {
		ErrorLog("Router(%v) proc login error with connection is not channel", r.Name)
		err = fmt.Errorf("connection is not channel")
		conn.Close()
		return
	}
	name, index, result, err := r.Handler.OnConnLogin(channel, string(buf[13:]))
	if err != nil {
		ErrorLog("Router(%v) proc login fail with %v", r.Name, err)
		message := converter.JSON(xmap.M{"code": 10, "message": err.Error()})
		err = writeCmd(conn, nil, CmdLoginBack, 0, []byte(message))
		conn.Close()
		return
	}
	if result == nil {
		result = xmap.M{}
	}
	channel.name = name
	channel.index = index
	result["name"] = r.Name
	result["code"] = 0
	r.addChannel(channel)
	message := converter.JSON(result)
	writeCmd(channel, nil, CmdLoginBack, 0, []byte(message))
	InfoLog("Router(%v) the channel(%v,%v) is login success on %v", r.Name, name, index, channel)
	return
}

func (r *Router) procRawDial(channel Conn, sid uint64, conn, uri string) (err error) {
	dstSid := atomic.AddUint64(&r.connectSequence, 1)
	raw, rawError := r.Handler.DialRaw(channel, dstSid, uri)
	if rawError != nil {
		DebugLog("Router(%v) dial(%v) to %v fail on channel(%v) by %v", r.Name, sid, conn, channel, rawError)
		message := []byte(fmt.Sprintf("dial to uri(%v) fail with %v", uri, rawError))
		err = writeCmd(channel, nil, CmdDialBack, sid, message)
		return
	}
	DebugLog("Router(%v) dial(%v-%v->%v-%v) to %v success on channel(%v)", r.Name, channel.ID(), sid, raw.ID(), dstSid, conn, channel)
	r.addTable(channel, sid, raw, dstSid, conn)
	err = writeCmd(channel, nil, CmdDialBack, sid, []byte("OK"))
	if err != nil {
		raw.Close()
		r.removeTable(channel, sid)
	} else {
		go r.loopReadRaw(raw)
	}
	return
}

func (r *Router) procDial(channel Conn, buf []byte) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	conn := string(buf[13:])
	DebugLog("Router(%v) proc dial(%v) to %v on channel(%v)", r.Name, sid, conn, channel)
	path := strings.SplitN(conn, "@", 2)
	if len(path) < 2 {
		WarnLog("Router(%v) proc dial to %v on channel(%v) fail with invalid uri", r.Name, conn, channel)
		err = writeCmd(channel, nil, CmdDialBack, sid, []byte(fmt.Sprintf("invalid uri(%v)", conn)))
		return
	}
	parts := strings.SplitN(path[1], "->", 2)
	err = r.Handler.OnConnDialURI(channel, conn, parts)
	if err != nil {
		WarnLog("Router(%v) process dial uri event to %v on channel(%v) fail with %v", r.Name, conn, channel, err)
		err = writeCmd(channel, nil, CmdDialBack, sid, []byte(fmt.Sprintf("%v", err)))
		return
	}
	if len(parts) < 2 {
		go r.procRawDial(channel, sid, conn, parts[0])
		return
	}
	next := parts[0]
	if channel.Name() == next {
		err = fmt.Errorf("self dial error")
		DebugLog("Router(%v) proc dial to %v on channel(%v) fail with select channel error %v", r.Name, conn, channel, err)
		message := err.Error()
		err = writeCmd(channel, nil, CmdDialBack, sid, []byte(message))
		return
	}
	dst, err := r.SelectChannel(next)
	if err != nil {
		DebugLog("Router(%v) proc dial to %v on channel(%v) fail with select channel error %v", r.Name, conn, channel, err)
		message := err.Error()
		err = writeCmd(channel, nil, CmdDialBack, sid, []byte(message))
		return
	}
	dstSid := atomic.AddUint64(&r.connectSequence, 1)
	if ShowLog > 1 {
		DebugLog("Router(%v) forwarding dial(%v-%v->%v-%v) %v to channel(%v)", r.Name, channel.ID(), sid, dst.ID(), dstSid, conn, dst)
	}
	r.addTable(channel, sid, dst, dstSid, conn)
	writeError := writeCmd(dst, nil, CmdDial, dstSid, []byte(path[0]+"->"+next+"@"+parts[1]))
	if writeError != nil {
		WarnLog("Router(%v) send dial to channel(%v) fail with %v", r.Name, dst, writeError)
		message := writeError.Error()
		err = writeCmd(channel, nil, CmdDialBack, sid, []byte(message))
		r.removeTable(channel, sid)
	}
	return
}

func (r *Router) procDialBack(channel Conn, buf []byte) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	DebugLog("Router(%v) proc dial back by %v on channel(%v)", r.Name, sid, channel)
	r.tableLck.RLock()
	router := r.table[fmt.Sprintf("%v-%v", channel.ID(), sid)]
	r.tableLck.RUnlock()
	if router == nil {
		err = writeCmd(channel, nil, CmdClosed, sid, []byte("closed"))
		return
	}
	target, targetID := router.Next(channel)
	if target.Type() == ConnTypeRaw {
		msg := string(buf[13:])
		if msg == "OK" {
			InfoLog("Router(%v) dial to %v success", r.Name, target)
			r.addTable(channel, sid, target, target.ID(), router[4].(string))
			if waiter, ok := target.(ReadyWaiter); ok {
				waiter.Ready(nil, func(err error) {
					if err == nil {
						r.loopReadRaw(target)
					} else {
						router := r.removeTable(channel, sid)
						if router != nil {
							writeCmd(channel, nil, CmdClosed, sid, []byte(err.Error()))
						}
					}
				})
			} else {
				go r.loopReadRaw(target)
			}
		} else {
			InfoLog("Router(%v) dial to %v fail with %v", r.Name, target, msg)
			r.removeTable(channel, sid)
			if waiter, ok := target.(ReadyWaiter); ok {
				waiter.Ready(fmt.Errorf("%v", msg), nil)
			}
			target.Close()
		}
	} else {
		binary.BigEndian.PutUint64(buf[5:], targetID)
		_, writeError := target.WriteFrame(buf)
		if writeError != nil {
			err = writeCmd(channel, nil, CmdClosed, sid, []byte("closed"))
		}
	}
	return
}

func (r *Router) procChannelData(channel Conn, buf []byte) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	r.tableLck.RLock()
	router := r.table[fmt.Sprintf("%v-%v", channel.ID(), sid)]
	r.tableLck.RUnlock()
	if router == nil {
		if channel.Type() == ConnTypeRaw {
			err = fmt.Errorf("not router")
		}
		return
	}
	target, targetID := router.Next(channel)
	if ShowLog > 1 {
		DebugLog("Router(%v) forwaring %v bytes by %v-%v->%v-%v, source:%v, next:%v, uri:%v", r.Name, len(buf)-13, channel.ID(), sid, target.ID(), targetID, channel, target, router[4])
	}
	binary.BigEndian.PutUint64(buf[5:], targetID)
	_, writeError := target.WriteFrame(buf)
	if writeError != nil {
		if channel.Type() == ConnTypeRaw {
			err = writeError
		}
	}
	return
}

func (r *Router) procClosed(channel Conn, buf []byte) (err error) {
	message := string(buf[13:])
	sid := binary.BigEndian.Uint64(buf[5:])
	DebugLog("Router(%v) the session(%v) is closed by %v", r.Name, sid, message)
	router := r.removeTable(channel, sid)
	if router != nil {
		target, targetID := router.Next(channel)
		if target.Type() == ConnTypeRaw {
			target.Close()
		} else {
			binary.BigEndian.PutUint64(buf[5:], targetID)
			target.WriteFrame(buf)
		}
	}
	return
}

func (r *Router) procHeartbeat(conn Conn, buf []byte) (err error) {
	if channel, ok := conn.(*Channel); ok {
		channel.Heartbeat = time.Now()
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

//SyncDial will dial to remote by uri and wait dial successes
func (r *Router) SyncDial(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	sid, conn, err := r.DialConn(uri, raw)
	if err == nil {
		if waiter, ok := conn.(ReadyWaiter); ok {
			err = waiter.Wait()
		}
	}
	return
}

//DialConn will dial to remote by uri and bind channel to raw connection and return raw connection
func (r *Router) DialConn(uri string, raw io.ReadWriteCloser) (sid uint64, conn Conn, err error) {
	parts := strings.SplitN(uri, "->", 2)
	if len(parts) < 2 {
		DebugLog("Router(%v) start raw dial to %v", r.Name, uri)
		sid = r.UniqueSid()
		conn, err = r.Handler.DialRaw(nil, sid, uri)
		if err != nil {
			if waiter, ok := raw.(ReadyWaiter); ok {
				waiter.Ready(err, nil)
			}
			raw.Close()
			return
		}
		r.rawLck.Lock()
		r.rawConn[fmt.Sprintf("%p", raw)] = []io.ReadWriteCloser{raw, conn}
		r.rawConn[fmt.Sprintf("%p", conn)] = []io.ReadWriteCloser{conn, raw}
		r.rawLck.Unlock()
		proc := func(err error) {
			defer func() {
				r.rawLck.Lock()
				delete(r.rawConn, fmt.Sprintf("%p", raw))
				delete(r.rawConn, fmt.Sprintf("%p", conn))
				r.rawLck.Unlock()
			}()
			if err != nil {
				raw.Close()
				conn.Close()
				return
			}
			go func() {
				_, err = io.CopyBuffer(raw, conn, make([]byte, r.BufferSize))
				raw.Close()
			}()
			_, err = io.CopyBuffer(conn, raw, make([]byte, r.BufferSize))
			conn.Close()
		}
		if waiter, ok := conn.(ReadyWaiter); ok {
			waiter.Ready(nil, nil)
		}
		if waiter, ok := raw.(ReadyWaiter); ok {
			waiter.Ready(nil, proc)
		} else {
			go proc(nil)
		}
		return
	}
	channel, err := r.SelectChannel(parts[0])
	if err != nil {
		return
	}
	sid = atomic.AddUint64(&r.connectSequence, 1)
	conn = NewRawConn(fmt.Sprintf("%v", sid), raw, r.BufferSize, sid, uri)
	DebugLog("Router(%v) start dial(%v-%v->%v-%v) to %v on channel(%v)", r.Name, conn.ID(), sid, channel.ID(), sid, uri, channel)
	r.addTable(channel, sid, conn, sid, uri)
	err = writeCmd(channel, nil, CmdDial, sid, []byte(fmt.Sprintf("%v@%v", parts[0], parts[1])))
	if err != nil {
		r.removeTable(channel, sid)
	}
	return
}

//JoinConn will add channel by the connected connection
func (r *Router) JoinConn(conn frame.ReadWriteCloser, index int, args interface{}) (channel *Channel, result xmap.M, err error) {
	data, _ := json.Marshal(args)
	DebugLog("Router(%v) login join connection %v by options %v", r.Name, conn, string(data))
	err = writeCmd(conn, nil, CmdLogin, 0, data)
	if err != nil {
		WarnLog("Router(%v) send login to %v fail with %v", r.Name, conn, err)
		return
	}
	buf, err := conn.ReadFrame()
	if err != nil {
		WarnLog("Router(%v) read login back from %v fail with %v", r.Name, conn, err)
		return
	}
	result = xmap.M{}
	err = json.Unmarshal(buf[13:], &result)
	if err != nil || result.Int("code") != 0 || len(result.Str("name")) < 1 {
		err = fmt.Errorf("%v", string(buf[13:]))
		WarnLog("Router(%v) login to %v fail with %v", r.Name, conn, err)
		return
	}
	remoteName := result.Str("name")
	channel = &Channel{
		ReadWriteCloser: conn,
		cid:             atomic.AddUint64(&r.connectSequence, 1),
		name:            remoteName,
		index:           index,
		context:         xmap.M{},
		Heartbeat:       time.Now(),
	}
	r.Register(channel)
	InfoLog("Router(%v) login to %v success, bind to %v,%v", r.Name, conn, remoteName, index)
	return
}

//Close all channel
func (r *Router) Close() (err error) {
	all := []io.Closer{}
	r.channelLck.Lock()
	for _, bond := range r.channel {
		bond.channelLck.Lock()
		for _, channel := range bond.channels {
			all = append(all, channel)
		}
		bond.channelLck.Unlock()
	}
	r.channelLck.Unlock()
	r.tableLck.Lock()
	for _, table := range r.table {
		if closer, ok := table[0].(io.Closer); ok {
			all = append(all, closer)
		}
		if closer, ok := table[2].(io.Closer); ok {
			all = append(all, closer)
		}
	}
	r.tableLck.Unlock()
	r.rawLck.Lock()
	for _, entry := range r.rawConn {
		if entry[0] != nil {
			all = append(all, entry[0])
		}
	}
	r.rawLck.Unlock()
	InfoLog("Router(%v) router will close %v connection", r.Name, len(all))
	for _, closer := range all {
		InfoLog("Router(%v) %v is closing", r.Name, closer)
		closer.Close()
	}
	return
}

//State return the current state of router
func (r *Router) State(args ...interface{}) (state xmap.M) {
	state = xmap.M{}
	//
	var query = xmap.M{}
	if len(args) > 0 {
		query = xmap.Wrap(args[0])
	}
	if len(query) < 1 {
		return
	}
	channels := xmap.M{}
	r.channelLck.RLock()
	for name, bond := range r.channel {
		queryType := query.Str(name)
		if len(queryType) < 1 {
			queryType = query.Str("*")
		}
		if queryType != "info" && queryType != "*" {
			continue
		}
		channel := xmap.M{}
		for idx, con := range bond.channels {
			info := xmap.M{
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
		queryType := query.Str(t[0].(Conn).Name())
		if len(queryType) < 1 {
			queryType = query.Str(t[2].(Conn).Name())
		}
		if len(queryType) < 1 {
			queryType = query.Str("*")
		}
		if queryType != "table" && queryType != "*" {
			continue
		}
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

//StateH return the current state of router
func (r *Router) StateH(w http.ResponseWriter, req *http.Request) {
	var query = xmap.M{}
	for key := range req.URL.Query() {
		query[key] = req.URL.Query().Get(key)
	}
	state := r.State(query)
	w.Header().Add("Content-Type", "application/json;charset=utf-8")
	fmt.Fprintf(w, "%v", converter.JSON(state))
}

func writeCmd(w frame.Writer, buffer []byte, cmd byte, sid uint64, msg []byte) (err error) {
	if buffer == nil {
		buffer = make([]byte, len(msg)+13)
	}
	buffer[4] = cmd
	binary.BigEndian.PutUint64(buffer[5:], sid)
	copy(buffer[13:], msg)
	_, err = w.WriteFrame(buffer[:len(msg)+13])
	return
}

//DialPiper will dial uri on router and return piper
func (r *Router) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	piper := NewWaitedPiper()
	_, err = r.SyncDial(uri, piper)
	raw = piper
	return
}

//WaitedPiper is Waiter/Piper implement
type WaitedPiper struct {
	Base        io.ReadWriteCloser
	next        func(err error)
	ready       int
	failed      error
	closed      int
	readyLocker sync.RWMutex
	baseLocker  sync.RWMutex
	closeLocker sync.RWMutex
}

//NewWaitedPiper will return new WaitedPiper
func NewWaitedPiper() (piper *WaitedPiper) {
	piper = &WaitedPiper{
		readyLocker: sync.RWMutex{},
		baseLocker:  sync.RWMutex{},
		closeLocker: sync.RWMutex{},
	}
	piper.readyLocker.Lock()
	piper.baseLocker.Lock()
	return
}

//Wait will wait piper is ready
func (r *WaitedPiper) Wait() error {
	r.readyLocker.Lock()
	_ = r //do nothing for warning
	r.readyLocker.Unlock()
	return r.failed
}

//Ready will set piper is ready, failed/next at lasted is not nil
func (r *WaitedPiper) Ready(failed error, next func(err error)) {
	if failed == nil && next == nil {
		panic("failed/next is nil")
	}
	r.closeLocker.Lock()
	if r.closed > 0 {
		r.closeLocker.Unlock()
		return
	}
	r.next = next
	r.failed = failed
	r.closeLocker.Unlock()
	//
	r.readyLocker.Unlock()
}

//PipeConn will pipe connection, it must be called after Wait success, or panic
func (r *WaitedPiper) PipeConn(conn io.ReadWriteCloser, target string) (err error) {
	if r.next == nil || r.failed != nil {
		panic("not ready")
	}
	r.closeLocker.Lock()
	if r.closed > 0 {
		r.closeLocker.Unlock()
		err = fmt.Errorf("closed")
		return
	}
	r.Base = conn
	r.ready = 1
	r.closeLocker.Unlock()
	r.baseLocker.Unlock()
	r.next(nil)
	return
}

func (r *WaitedPiper) Read(p []byte) (n int, err error) {
	if r.ready == 0 {
		r.baseLocker.Lock()
		_ = r //do nothing for warning
		r.baseLocker.Unlock()
	}
	err = r.failed
	if r.failed == nil {
		n, err = r.Base.Read(p)
	}
	return
}

func (r *WaitedPiper) Write(p []byte) (n int, err error) {
	if r.ready == 0 {
		r.baseLocker.Lock()
		_ = r //do nothing for warning
		r.baseLocker.Unlock()
	}
	err = r.failed
	if r.failed == nil {
		n, err = r.Base.Write(p)
	}
	return
}

//Close will close ready piper, it will lock when it is not ready
func (r *WaitedPiper) Close() (err error) {
	r.closeLocker.Lock()
	if r.closed > 0 {
		r.closeLocker.Unlock()
		err = fmt.Errorf("already closed")
		return
	}
	r.closed = 1
	ready := r.ready
	r.closeLocker.Unlock()
	if ready < 1 {
		r.baseLocker.Unlock()
	}
	if r.next != nil {
		r.next(fmt.Errorf("closed"))
	}
	if r.Base != nil {
		err = r.Base.Close()
	}
	return
}

func (r *WaitedPiper) String() string {
	return fmt.Sprintf("WaitedPiper(%v)", xio.RemoteAddr(r.Base))
}
