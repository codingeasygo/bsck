package router

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/util/xnet"
	"github.com/codingeasygo/util/xsort"
	"github.com/codingeasygo/util/xtime"
)

const (
	ConnOK = "OK"
)

type RouterErrorType int

const (
	ErrorChannelNotFound RouterErrorType = 100
)

type RouterError struct {
	message string
	Type    RouterErrorType
}

func NewRouterError(errType RouterErrorType, format string, args ...interface{}) (err *RouterError) {
	err = &RouterError{
		message: fmt.Sprintf(format, args...),
		Type:    errType,
	}
	return
}

func (r *RouterError) IsRouterErrorType() (errorType RouterErrorType) { return r.Type }

func (r *RouterError) Error() string { return r.message }

func (r *RouterError) String() string { return r.message }

const (
	//CmdLoginChannel is the command of login to master
	CmdLoginChannel RouterCmd = 10
	//CmdLoginBack is the command of login return from master
	CmdLoginBack RouterCmd = 11
	//CmdPingConn is the command of ping to node
	CmdPingConn RouterCmd = 20
	//CmdPingBack is the command of ping return from node
	CmdPingBack RouterCmd = 21
	//CmdDial is the command of tcp dial by router
	CmdDialConn RouterCmd = 100
	//CmdDialBack is the command of tcp dial back from master/slaver
	CmdDialBack RouterCmd = 101
	//CmdConnData is the command of transfter tcp data
	CmdConnData RouterCmd = 110
	//CmdClosed is the command of tcp closed.
	CmdConnClosed RouterCmd = 120
)

type RouterCmd byte

func (c RouterCmd) String() string {
	switch c {
	case CmdLoginChannel:
		return "LoginChannel"
	case CmdLoginBack:
		return "LoginBack"
	case CmdPingConn:
		return "PingConn"
	case CmdPingBack:
		return "PingBack"
	case CmdDialConn:
		return "DialConn"
	case CmdDialBack:
		return "DialBack"
	case CmdConnData:
		return "ConnData"
	case CmdConnClosed:
		return "ConnClosed"
	default:
		return fmt.Sprintf("unknown cmd(%v)", byte(c))
	}
}

type RouterFrame struct {
	Buffer []byte
	SID    ConnID
	Cmd    RouterCmd
	Data   []byte
}

func ParseLocalRouterFrame(header frame.Header, buffer []byte) (frame *RouterFrame) {
	offset := header.GetDataOffset()
	frame = &RouterFrame{
		Buffer: buffer,
		SID:    [2]byte{buffer[offset], buffer[offset+1]},
		Cmd:    RouterCmd(buffer[offset+2]),
		Data:   buffer[offset+3:],
	}
	return
}

func ParseRemoteRouterFrame(header frame.Header, buffer []byte) (frame *RouterFrame) {
	offset := header.GetDataOffset()
	frame = &RouterFrame{
		Buffer: buffer,
		SID:    [2]byte{buffer[offset+1], buffer[offset]},
		Cmd:    RouterCmd(buffer[offset+2]),
		Data:   buffer[offset+3:],
	}
	return
}

func NewRouterFrame(header frame.Header, buffer []byte, sid ConnID, cmd RouterCmd) (frame *RouterFrame) {
	header.WriteHead(buffer)
	offset := header.GetDataOffset()
	localID, remoteID := sid.Split()
	buffer[offset], buffer[offset+1], buffer[offset+2] = localID, remoteID, byte(cmd)
	frame = &RouterFrame{
		Buffer: buffer,
		SID:    sid,
		Cmd:    RouterCmd(buffer[offset+2]),
		Data:   buffer[offset+3:],
	}
	return
}

func NewRouterFrameByMessage(header frame.Header, buffer []byte, sid ConnID, cmd RouterCmd, message []byte) (frame *RouterFrame) {
	offset := header.GetDataOffset() + 3
	if len(buffer) < 1 {
		buffer = make([]byte, offset+len(message))
	}
	copy(buffer[offset:], message)
	frame = NewRouterFrame(header, buffer, sid, cmd)
	return
}

func (f *RouterFrame) String() string {
	var show string
	if len(f.Buffer) > 32 {
		show = fmt.Sprintf("%v...]", strings.TrimSuffix(fmt.Sprintf("%v", f.Buffer[:32]), "]"))
	} else {
		show = fmt.Sprintf("%v", f.Buffer)
	}
	return fmt.Sprintf("Frame(%v,%v,%v)", len(f.Buffer), f.SID, show)
}

func writeMessage(w Conn, buffer []byte, sid ConnID, cmd RouterCmd, message []byte) (err error) {
	frame := NewRouterFrameByMessage(w, buffer, sid, cmd, message)
	err = w.WriteRouterFrame(frame)
	return
}

const (
	//ConnTypeRaw is the type of raw connection
	ConnTypeRaw ConnType = 100
	//ConnTypeChannel is the type of channel connection
	ConnTypeChannel ConnType = 200
)

type ConnType int

func (c ConnType) String() string {
	switch c {
	case ConnTypeRaw:
		return "Raw"
	case ConnTypeChannel:
		return "Channel"
	default:
		return fmt.Sprintf("unknown cmd(%v)", byte(c))
	}
}

type ConnID [2]byte

var ZeroConnID = ConnID{}

func (c ConnID) Split() (localID, remoteID byte) {
	localID, remoteID = c[0], c[1]
	return
}

func (c ConnID) LocalID() (localID byte) {
	localID = c[0]
	return
}

func (c ConnID) RemoteID() (remoteID byte) {
	remoteID = c[1]
	return
}

func (c ConnID) String() string {
	return "0x" + hex.EncodeToString([]byte{c[0], c[1]})
}

func MakeRawConnPrefix(sid ConnID, cmd RouterCmd) (prefix []byte) {
	prefix = []byte{sid[1], sid[0], byte(cmd)}
	return
}

type RawValuable interface {
	RawValue() interface{}
}

// Conn is the interface that wraps the connection will be running on Router.
//
// # ID is the unique of connection
//
// # Name is the channel name, it will be used when join current connection to channel
//
// Index is the channel index, it will be used when join current connection to channel.
//
// Type is the connection type by ConnTypeRaw/ConnTypeChannel
type Conn interface {
	//the basic ReadWriteCloser
	frame.ReadWriteCloser
	//ready
	ReadyWaiter
	//the connection id
	ID() uint16
	//the channel name
	Name() string
	//update change name
	SetName(name string)
	// the conn ping info
	Ping() (speed time.Duration, last time.Time)
	// update the conn ping info
	SetPing(speed time.Duration, last time.Time)
	//the conn recv last
	RecvLast() (last time.Time)
	// update the conn recv last info
	SetRecvLast(last time.Time)
	//the connection type
	Type() ConnType
	//conn context getter
	Context() xmap.M
	//alloc conn id, if can't alloc more id returing zero
	AllocConnID() byte
	//free conn id
	FreeConnID(connID byte)
	//used conn id
	UsedConnID() (used int)
	//read router frame
	ReadRouterFrame() (frame *RouterFrame, err error)
	//write router frame
	WriteRouterFrame(frame *RouterFrame) (err error)
}

// ReadyWaiter interface for ready waiter
type ReadyWaiter interface {
	Wait() error
	Ready(failed error, next func(err error))
}

type ReadyReadWriteCloser struct {
	ReadyWaiter
	frame.ReadWriteCloser
}

func WrapReadyReadWriteCloser(base frame.ReadWriteCloser, raw interface{}) (target frame.ReadWriteCloser) {
	target = base
	if waiter, ok := raw.(ReadyWaiter); waiter != nil && ok {
		target = &ReadyReadWriteCloser{
			ReadyWaiter:     waiter,
			ReadWriteCloser: base,
		}
	} else if waiter, ok := base.(ReadyWaiter); waiter != nil && ok {
		target = &ReadyReadWriteCloser{
			ReadyWaiter:     waiter,
			ReadWriteCloser: base,
		}
	}
	return
}

type BondConn struct {
	connName string
	connType ConnType
	connAll  map[uint16]Conn
	connUsed map[uint16]uint64
	connLock sync.RWMutex
}

func NewBondConn(name string, connType ConnType) *BondConn {
	return &BondConn{
		connName: name,
		connType: connType,
		connAll:  map[uint16]Conn{},
		connUsed: map[uint16]uint64{},
		connLock: sync.RWMutex{},
	}
}

func (b *BondConn) Name() string { return b.connName }

func (b *BondConn) Type() ConnType { return b.connType }

func (b *BondConn) AddConn(conn Conn) (remain int) {
	b.connLock.Lock()
	defer b.connLock.Unlock()
	id := conn.ID()
	b.connAll[id] = conn
	b.connUsed[id] = 0
	remain = len(b.connAll)
	return
}

func (b *BondConn) RemoveConn(id uint16) (remain int) {
	b.connLock.Lock()
	defer b.connLock.Unlock()
	delete(b.connAll, id)
	delete(b.connUsed, id)
	remain = len(b.connAll)
	return
}

func (b *BondConn) FindConn(id uint16) (conn Conn) {
	b.connLock.RLock()
	defer b.connLock.RUnlock()
	conn = b.connAll[id]
	return
}

func (b *BondConn) SelectConn() (conn Conn) {
	b.connLock.RLock()
	defer b.connLock.RUnlock()
	minUsed := uint64(0)
	for id, c := range b.connAll {
		used := b.connUsed[id]
		if conn == nil || used < minUsed {
			conn = c
			minUsed = used
		}
	}
	return
}

func (b *BondConn) ListConn() (connList []Conn) {
	b.connLock.RLock()
	defer b.connLock.RUnlock()
	for _, c := range b.connAll {
		connList = append(connList, c)
	}
	return
}

func (b *BondConn) Close() (err error) {
	b.connLock.RLock()
	defer b.connLock.RUnlock()
	for _, c := range b.connAll {
		c.Close()
	}
	return
}

func (b *BondConn) DislayConn() (connList []xmap.M) {
	b.connLock.RLock()
	defer b.connLock.RUnlock()
	idList := []uint16{}
	for id, conn := range b.connAll {
		speed, last := conn.Ping()
		info := xmap.M{
			"connect": fmt.Sprintf("%v", conn),
			"used":    b.connUsed[id],
			"ping": xmap.M{
				"speed": speed.Milliseconds(),
				"last":  xtime.Timestamp(last),
			},
			"id": id,
		}
		connList = append(connList, info)
		idList = append(idList, id)
	}
	xsort.SortFunc(connList, func(x, y int) bool {
		return idList[x] < idList[y]
	})
	return
}

func (b *BondConn) String() string {
	return fmt.Sprintf("BondConn{name:%v,type:%v}", b.Name(), b.connType)
}

type RouterConn struct {
	frame.ReadWriteCloser //the raw connection
	id                    uint16
	name                  string
	connType              ConnType
	context               xmap.M
	pingSpeed             time.Duration
	pingLast              time.Time
	recvLast              time.Time
	connSeq               uint8
	connAll               map[byte]bool
	connLck               sync.RWMutex
	failed                error
	waiter                *sync.Cond
}

func NewRouterConn(base frame.ReadWriteCloser, id uint16, connType ConnType) (conn *RouterConn) {
	conn = &RouterConn{
		ReadWriteCloser: base,
		id:              id,
		connType:        connType,
		context:         xmap.M{},
		connAll:         map[uint8]bool{},
		connLck:         sync.RWMutex{},
		waiter:          sync.NewCond(&sync.Mutex{}),
	}
	return
}

func (r *RouterConn) ID() uint16 {
	return r.id
}

func (r *RouterConn) Name() string {
	return r.name
}

func (r *RouterConn) SetName(name string) {
	r.name = name
}

func (r *RouterConn) Ping() (speed time.Duration, last time.Time) {
	speed, last = r.pingSpeed, r.pingLast
	return
}

func (r *RouterConn) SetPing(speed time.Duration, last time.Time) {
	r.pingSpeed, r.pingLast = speed, last
}

func (r *RouterConn) RecvLast() (last time.Time) {
	last = r.recvLast
	return
}

func (r *RouterConn) SetRecvLast(last time.Time) {
	r.recvLast = last
}

func (r *RouterConn) Type() ConnType {
	return r.connType
}

// Context is conn context getter
func (r *RouterConn) Context() xmap.M {
	return r.context
}

// alloc conn id, if can't alloc more id returing zero
func (r *RouterConn) AllocConnID() (connID byte) {
	r.connLck.Lock()
	defer r.connLck.Unlock()
	for i := 0; i < 257; i++ {
		r.connSeq++
		nextID := r.connSeq
		if nextID > 0 && !r.connAll[nextID] {
			r.connAll[nextID] = true
			connID = nextID
			break
		}
	}
	return
}

// free conn id
func (r *RouterConn) FreeConnID(connID byte) {
	r.connLck.Lock()
	defer r.connLck.Unlock()
	delete(r.connAll, connID)
}

func (r *RouterConn) UsedConnID() (used int) {
	r.connLck.RLock()
	defer r.connLck.RUnlock()
	used = len(r.connAll)
	return
}

// read router frame
func (r *RouterConn) ReadRouterFrame() (frame *RouterFrame, err error) {
	buffer, err := r.ReadWriteCloser.ReadFrame()
	if err == nil {
		frame = ParseRemoteRouterFrame(r.ReadWriteCloser, buffer)
	}
	return
}

// write router frame
func (r *RouterConn) WriteRouterFrame(frame *RouterFrame) (err error) {
	_, err = r.ReadWriteCloser.WriteFrame(frame.Buffer)
	return
}

func (r *RouterConn) RawValue() (raw interface{}) {
	valuable, ok := r.ReadWriteCloser.(RawValuable)
	if ok {
		raw = valuable.RawValue()
	}
	return
}

// Close will close the raw connection
func (r *RouterConn) Close() (err error) {
	r.ReadWriteCloser.Close()
	r.waiter.L.Lock()
	if r.failed == nil {
		r.failed = fmt.Errorf("closed")
	}
	r.waiter.Broadcast()
	r.waiter.L.Unlock()
	return
}

// Wait is ConnectedWaiter impl
func (r *RouterConn) Wait() error {
	if waiter, ok := r.ReadWriteCloser.(ReadyWaiter); ok {
		return waiter.Wait()
	}
	r.waiter.L.Lock()
	r.waiter.Wait()
	r.waiter.L.Unlock()
	return r.failed
}

// Ready is ConnectedWaiter impl
func (r *RouterConn) Ready(failed error, next func(err error)) {
	if waiter, ok := r.ReadWriteCloser.(ReadyWaiter); ok {
		waiter.Ready(failed, next)
		return
	}
	//
	r.waiter.L.Lock()
	if r.failed == nil {
		r.failed = failed
	}
	r.waiter.Broadcast()
	r.waiter.L.Unlock()
	if r.failed == nil && next != nil {
		go next(r.failed)
	}
}

func (r *RouterConn) String() string {
	return fmt.Sprintf("RouterConn(%v,%v,%v,%v)", r.name, r.connType, r.id, r.ReadWriteCloser)
}

// RouterItem is the router table item
type RouterItem struct {
	FromConn Conn
	FromSID  ConnID
	NextConn Conn
	NexSID   ConnID
	URI      string
}

func (r *RouterItem) Update(conn Conn, sid ConnID) {
	if r.FromConn == conn {
		r.FromSID = sid
	} else if r.NextConn == conn {
		r.NexSID = sid
	}
}

// Next will return next connection and session id
func (r *RouterItem) Next(conn Conn) (next Conn, sid ConnID) {
	if r.FromConn == conn {
		next = r.NextConn
		sid = r.NexSID
	} else if r.NextConn == conn {
		next = r.FromConn
		sid = r.FromSID
	}
	return
}

func (r *RouterItem) AllKey() (keyList []string) {
	{
		local, remote := routerKey(r.FromConn, r.FromSID)
		if len(local) > 0 {
			keyList = append(keyList, local)
		}
		if len(remote) > 0 {
			keyList = append(keyList, remote)
		}
	}
	{
		local, remote := routerKey(r.NextConn, r.NexSID)
		if len(local) > 0 {
			keyList = append(keyList, local)
		}
		if len(remote) > 0 {
			keyList = append(keyList, remote)
		}
	}
	return
}

func (r RouterItem) String() string {
	return fmt.Sprintf("%v %v<->%v %v", r.FromConn, r.FromSID, r.NexSID, r.NextConn)
}

func routerKey(conn Conn, sid ConnID) (local, remote string) {
	if sid[0] > 0 {
		local = fmt.Sprintf("l-%d-%v", conn.ID(), sid[0])
	}
	// if sid[1] > 0 {
	// 	remote = fmt.Sprintf("r-%d-%v", conn.ID(), sid[1])
	// }
	return
}

// DialRawStdF is a function type to dial raw connection.
type DialRawStdF func(channel Conn, id uint16, uri string) (raw io.ReadWriteCloser, err error)

// DialRaw will dial raw connection
func (d DialRawStdF) DialRawStd(channel Conn, id uint16, uri string) (raw io.ReadWriteCloser, err error) {
	raw, err = d(channel, id, uri)
	return
}

// RawDialer is dialer to dial raw by uri
type RawDialer interface {
	DialRawStd(channel Conn, id uint16, uri string) (raw io.ReadWriteCloser, err error)
}

// DialRawConnF is a function type to dial raw connection.
type DialRawConnF func(channel Conn, id uint16, uri string) (conn Conn, err error)

// DialRaw will dial raw connection
func (d DialRawConnF) DialRawConn(channel Conn, id uint16, uri string) (conn Conn, err error) {
	conn, err = d(channel, id, uri)
	return
}

type ConnDialer interface {
	DialRawConn(channel Conn, id uint16, uri string) (conn Conn, err error)
}

// Handler is the interface that wraps the handler of Router.
type Handler interface {
	//conn dialer
	ConnDialer
	//on connection dial uri
	OnConnDialURI(channel Conn, conn string, parts []string) (err error)
	//on connection login
	OnConnLogin(channel Conn, args string) (name string, result xmap.M, err error)
	//on connection close
	OnConnClose(raw Conn) error
	//OnConnJoin is event on channel join
	OnConnJoin(channel Conn, option interface{}, result xmap.M)
}

// Router is an implementation of the router control
type Router struct {
	Name          string //current router name
	Header        frame.Header
	BufferSize    int           //buffer size of connection runner
	MaxConnection int           //max connection count
	Heartbeat     time.Duration //the delay of heartbeat
	Timeout       time.Duration //the delay of timeout
	Handler       Handler       //the router handler
	sidSequence   uint32
	channelAll    map[string]*BondConn
	channelLck    sync.RWMutex
	tableAll      map[string]*RouterItem
	tableLck      sync.RWMutex
	exiter        chan int
	waiter        sync.WaitGroup
}

// NewRouter will return new Router by name
func NewRouter(name string, handler Handler) (router *Router) {
	router = &Router{
		Name:          name,
		Header:        frame.NewDefaultHeader(),
		Handler:       handler,
		channelAll:    map[string]*BondConn{},
		channelLck:    sync.RWMutex{},
		tableAll:      map[string]*RouterItem{},
		tableLck:      sync.RWMutex{},
		BufferSize:    4 * 1024,
		MaxConnection: 4096,
		Heartbeat:     5 * time.Second,
		Timeout:       15 * time.Second,
		exiter:        make(chan int, 1),
		waiter:        sync.WaitGroup{},
	}
	router.Header.SetLengthFieldMagic(0)
	router.Header.SetDataOffset(2)
	router.Header.SetLengthFieldLength(2)
	return
}

func (r *Router) NewConn(v interface{}, connID uint16, connType ConnType) (conn Conn) {
	if c, ok := v.(Conn); ok {
		conn = c
	} else if f, ok := v.(frame.ReadWriteCloser); ok {
		if connID < 1 {
			connID = r.NewConnID()
		}
		conn = NewRouterConn(WrapReadyReadWriteCloser(f, nil), connID, connType)
	} else if raw, ok := v.(io.ReadWriteCloser); ok {
		if connID < 1 {
			connID = r.NewConnID()
		}
		if connType == ConnTypeChannel {
			conn = NewRouterConn(WrapReadyReadWriteCloser(frame.NewReadWriteCloser(r.Header, raw, r.BufferSize), raw), connID, connType)
		} else {
			conn = NewRouterConn(WrapReadyReadWriteCloser(frame.NewRawReadWriteCloser(r.Header, raw, r.BufferSize), raw), connID, connType)
		}
	} else {
		panic(fmt.Sprintf("channel type is not supported by %v=>router.Conn", reflect.TypeOf(v)))
	}
	return
}

// Accept one raw connection as channel,
// it will auth the raw connection by ACL.
func (r *Router) Accept(channel interface{}, sync bool) {
	conn := r.NewConn(channel, 0, ConnTypeChannel)
	r.waiter.Add(1)
	if sync {
		r.procConnRead(conn)
	} else {
		go r.procConnRead(conn)
	}
}

// Register one login raw connection to channel,
// it will auth the raw connection by ACL.
func (r *Router) Register(channel interface{}) {
	conn := r.NewConn(channel, 0, ConnTypeChannel)
	r.addChannel(conn)
	r.waiter.Add(1)
	go r.procConnRead(conn)
}

func (r *Router) findChannel(name string, new bool) (channel *BondConn) {
	r.channelLck.RLock()
	defer r.channelLck.RUnlock()
	channel = r.channelAll[name]
	if channel == nil && new {
		channel = NewBondConn(name, ConnTypeChannel)
		r.channelAll[name] = channel
	}
	return
}

func (r *Router) addChannel(channel Conn) {
	name := channel.Name()
	if len(name) < 1 {
		panic(fmt.Sprintf("channel %v name is empty", channel))
	}
	bound := r.findChannel(name, true)
	connected := bound.AddConn(channel)
	InfoLog("Router(%v) add channel(%v) success with %v connected", r.Name, channel, connected)
}

func (r *Router) removeChannel(name string) {
	r.channelLck.RLock()
	defer r.channelLck.RUnlock()
	delete(r.channelAll, name)
}

func (r *Router) removeChannelConn(channel Conn) {
	name := channel.Name()
	bound := r.findChannel(name, true)
	connected := bound.RemoveConn(channel.ID())
	InfoLog("Router(%v) remove channel(%v) success with %v connected", r.Name, channel, connected)
	if connected < 1 {
		r.removeChannel(name)
	}
}

func (r *Router) listChannel() (channels []*BondConn) {
	r.channelLck.RLock()
	defer r.channelLck.RUnlock()
	for _, channel := range r.channelAll {
		channels = append(channels, channel)
	}
	return
}

func (r *Router) DisplayChannel(query xmap.M) (channels xmap.M) {
	r.channelLck.RLock()
	defer r.channelLck.RUnlock()
	channels = xmap.M{}
	for name, bond := range r.channelAll {
		channels[name] = bond.DislayConn()
	}
	return
}

// NewSid will return new session id
func (r *Router) NewConnID() (connID uint16) {
	connID = uint16(atomic.AddUint32(&r.sidSequence, 1) % uint32(r.MaxConnection))
	return
}

// SelectChannel will pick one channel by name.
func (r *Router) SelectChannel(name string) (target Conn, err error) {
	channel := r.findChannel(name, false)
	if channel != nil {
		target = channel.SelectConn()
	}
	if target == nil {
		err = NewRouterError(ErrorChannelNotFound, "channel %v is not found", name)
	}
	return
}

// CloseChannel will call close on all bond channle by name.
func (r *Router) CloseChannel(name string) (err error) {
	channel := r.findChannel(name, false)
	if channel != nil {
		err = channel.Close()
	}
	return
}

func (r *Router) addTable(fromConn Conn, fromSID ConnID, nextConn Conn, nextSID ConnID, conn string) *RouterItem {
	r.tableLck.Lock()
	defer r.tableLck.Unlock()
	router := &RouterItem{FromConn: fromConn, FromSID: fromSID, NextConn: nextConn, NexSID: nextSID, URI: conn}
	for _, key := range router.AllKey() {
		r.tableAll[key] = router
	}
	return router
}

func (r *Router) updateTable(router *RouterItem) {
	r.tableLck.Lock()
	defer r.tableLck.Unlock()
	for _, key := range router.AllKey() {
		r.tableAll[key] = router
	}
}

func (r *Router) findTable(conn Conn, sid ConnID) *RouterItem {
	r.tableLck.Lock()
	defer r.tableLck.Unlock()
	return r.findTableNoLock(conn, sid)
}

func (r *Router) findTableNoLock(conn Conn, sid ConnID) *RouterItem {
	local, remote := routerKey(conn, sid)
	router := r.tableAll[local]
	if router == nil {
		router = r.tableAll[remote]
	}
	return router
}

func (r *Router) removeTable(conn Conn, sid ConnID) *RouterItem {
	r.tableLck.Lock()
	defer r.tableLck.Unlock()
	return r.removeTableNoLock(conn, sid)
}

func (r *Router) removeTableNoLock(conn Conn, sid ConnID) *RouterItem {
	router := r.findTableNoLock(conn, sid)
	if router != nil {
		for _, key := range router.AllKey() {
			delete(r.tableAll, key)
		}
	}
	return router
}

func (r *Router) removeCloseTable(channel Conn) (notify []*RouterItem, close []io.Closer) {
	r.tableLck.Lock()
	defer r.tableLck.Unlock()
	addedAll := map[string]bool{}
	checkAdded := func(v interface{}) bool {
		key := fmt.Sprintf("%p", v)
		added := addedAll[key]
		addedAll[key] = true
		return !added
	}
	for _, router := range r.tableAll {
		target, sid := router.Next(channel)
		if target == nil {
			continue
		}
		if target.Type() == ConnTypeRaw {
			if checkAdded(target) {
				close = append(close, target)
			}
		} else {
			if checkAdded(router) {
				notify = append(notify, router)
			}
		}
		r.removeTableNoLock(target, sid)
	}
	return
}

func (r *Router) DisplayTable(query xmap.M) (tableList []string) {
	r.tableLck.RLock()
	defer r.tableLck.RUnlock()
	added := map[string]bool{}
	for _, router := range r.tableAll {
		key := fmt.Sprintf("%p", router)
		if added[key] {
			continue
		}
		added[key] = true
		tableList = append(tableList, router.String())
	}
	return
}

func (r *Router) procPingLoop() {
	defer func() {
		r.waiter.Done()
		InfoLog("Router(%v) the ping runner is stopped", r.Name)
	}()
	InfoLog("Router(%v) the ping runner is starting by %v", r.Name, r.Heartbeat)
	timer := time.NewTicker(r.Heartbeat)
	running := true
	for running {
		select {
		case <-timer.C:
			r.procPingAll()
		case <-r.exiter:
			running = false
		}
	}
}

func (r *Router) procTimeoutLoop() {
	defer func() {
		r.waiter.Done()
		InfoLog("Router(%v) the timeout runner is stopped", r.Name)
	}()
	InfoLog("Router(%v) the timeout runner is starting by %v", r.Name, r.Timeout)
	timer := time.NewTicker(r.Timeout)
	running := true
	for running {
		select {
		case <-timer.C:
			r.procTimeoutAll()
		case <-r.exiter:
			running = false
		}
	}
}

func (r *Router) procConnRead(conn Conn) {
	defer func() {
		if perr := recover(); perr != nil {
			ErrorLog("Router(%v) proc conn read is panic with %v,%v, callstack is \n%v", r.Name, perr, conn, xdebug.CallStack())
		}
		conn.Close()
		r.waiter.Done()
	}()
	InfoLog("Router(%v) the reader %v is starting", r.Name, conn)
	var frame *RouterFrame
	var err error
	// offset := conn.GetDataOffset()
	conn.SetRecvLast(time.Now())
	for {
		frame, err = conn.ReadRouterFrame()
		if err != nil {
			break
		}
		conn.SetRecvLast(time.Now())
		if ShowLog > 1 {
			DebugLog("Router(%v) read one command(%v) from %v", r.Name, frame, conn)
		}
		switch frame.Cmd {
		case CmdLoginChannel:
			err = r.procLoginChannel(conn, frame)
		case CmdPingConn:
			err = r.procPingConn(conn, frame)
		case CmdPingBack:
			err = r.procPingBack(conn, frame)
		case CmdDialConn:
			err = r.procDialConn(conn, frame)
		case CmdDialBack:
			err = r.procDialBack(conn, frame)
		case CmdConnData:
			err = r.procConnData(conn, frame)
		case CmdConnClosed:
			err = r.procConnClosed(conn, frame)
		default:
			err = fmt.Errorf("not supported cmd(%x)", byte(frame.Cmd))
		}
		if err != nil {
			break
		}
	}
	notified, closed := r.closeConn(conn, err)
	InfoLog("Router(%v) the reader %v is stopped by %v, notify %v close %v connection", r.Name, conn, err, notified, closed)
	r.Handler.OnConnClose(conn)
}

func (r *Router) closeConn(conn Conn, reason error) (notified, closed int) {
	toNotify, toClose := r.removeCloseTable(conn)
	for _, closer := range toClose {
		closer.Close()
	}
	for _, router := range toNotify {
		target, sid := router.Next(conn)
		if target != nil {
			writeMessage(target, nil, sid, CmdConnClosed, []byte("channel closed"))
			target.FreeConnID(sid.LocalID())
		}
	}
	if conn.Type() == ConnTypeChannel {
		r.removeChannelConn(conn)
	}
	notified, closed = len(toNotify), len(toClose)
	return
}

func (r *Router) procLoginChannel(channel Conn, frame *RouterFrame) (err error) {
	name, result, err := r.Handler.OnConnLogin(channel, string(frame.Data))
	if err != nil {
		ErrorLog("Router(%v) proc login fail with %v", r.Name, err)
		message := converter.JSON(xmap.M{"code": 10, "message": err.Error()})
		writeMessage(channel, nil, frame.SID, CmdLoginBack, []byte(message))
		return
	}
	if result == nil {
		result = xmap.M{}
	}
	channel.SetName(name)
	result["name"] = r.Name
	result["code"] = 0
	r.addChannel(channel)
	message := converter.JSON(result)
	writeMessage(channel, nil, frame.SID, CmdLoginBack, []byte(message))
	InfoLog("Router(%v) the channel %v is login success on %v", r.Name, name, channel)
	return
}

func (r *Router) procPingAll() {
	defer func() {
		if perr := recover(); perr != nil {
			ErrorLog("Router(%v) proc ping all is panic with %v, callstack is \n%v", r.Name, xdebug.CallStack())
		}
	}()
	channels := r.listChannel()
	order := r.Header.GetByteOrder()
	offset := r.Header.GetDataOffset()
	buffer := make([]byte, offset+11)
	for _, channel := range channels {
		for _, conn := range channel.ListConn() {
			startTime := xtime.TimeNow()
			order.PutUint64(buffer[offset+3:], uint64(startTime))
			writeMessage(conn, buffer, ConnID{}, CmdPingConn, buffer[offset+3:])
		}
	}
}

func (r *Router) procPingConn(channel Conn, frame *RouterFrame) (err error) {
	offset := channel.GetDataOffset()
	frame.Buffer[offset+2] = byte(CmdPingBack)
	err = channel.WriteRouterFrame(frame)
	return
}

func (r *Router) procPingBack(channel Conn, frame *RouterFrame) (err error) {
	order := channel.GetByteOrder()
	startTime := xtime.TimeUnix(int64(order.Uint64(frame.Data)))
	channel.SetPing(time.Since(startTime), time.Now())
	return
}

func (r *Router) procTimeoutAll() {
	defer func() {
		if perr := recover(); perr != nil {
			ErrorLog("Router(%v) proc timeout all is panic with %v, callstack is \n%v", r.Name, xdebug.CallStack())
		}
	}()
	channels := r.listChannel()
	for _, channel := range channels {
		for _, conn := range channel.ListConn() {
			if time.Since(conn.RecvLast()) > r.Timeout {
				conn.Close()
			}
		}
	}
}

func (r *Router) procDialRaw(channel Conn, sid ConnID, conn, uri string) (err error) {
	if sid[1] < 1 {
		xerr := fmt.Errorf("remote id is zero")
		WarnLog("Router(%v) dial %v to %v fail on channel(%v) by %v", r.Name, sid, conn, channel, xerr)
		err = writeMessage(channel, nil, sid, CmdDialBack, []byte(xerr.Error()))
		return
	}
	connID := r.NewConnID()
	next, nextError := r.Handler.DialRawConn(channel, connID, uri)
	if nextError != nil {
		InfoLog("Router(%v) dial %v to %v fail on channel(%v) by %v", r.Name, sid, conn, channel, nextError)
		message := []byte(fmt.Sprintf("dial to uri(%v) fail with %v", uri, nextError))
		err = writeMessage(channel, nil, sid, CmdDialBack, message)
		return
	}
	localID := channel.AllocConnID()
	sid[0] = localID
	nextSID := ConnID{localID, localID}
	next.SetDataPrefix(MakeRawConnPrefix(nextSID, CmdConnData))
	DebugLog("Router(%v) dial(%v-%v->%v-%v) to %v success on channel(%v)", r.Name, channel.ID(), sid, next.ID(), nextSID, conn, channel)
	r.addTable(channel, sid, next, nextSID, conn)
	err = writeMessage(channel, nil, sid, CmdDialBack, []byte(ConnOK))
	if err != nil {
		next.Close()
		r.removeTable(channel, sid)
		channel.FreeConnID(localID)
	} else {
		r.waiter.Add(1)
		go r.procConnRead(next)
	}
	return
}

func (r *Router) procDialConn(channel Conn, frame *RouterFrame) (err error) {
	conn := string(frame.Data)
	DebugLog("Router(%v) proc dial(%v) to %v on channel(%v)", r.Name, frame.SID, conn, channel)
	path := strings.SplitN(conn, "|", 2)
	if len(path) < 2 {
		WarnLog("Router(%v) proc dial to %v on channel(%v) fail with invalid uri", r.Name, conn, channel)
		err = writeMessage(channel, nil, frame.SID, CmdDialBack, []byte(fmt.Sprintf("invalid uri(%v)", conn)))
		return
	}
	parts := strings.SplitN(path[1], "->", 2)
	err = r.Handler.OnConnDialURI(channel, conn, parts)
	if err != nil {
		WarnLog("Router(%v) process dial uri event to %v on channel(%v) fail with %v", r.Name, conn, channel, err)
		err = writeMessage(channel, nil, frame.SID, CmdDialBack, []byte(fmt.Sprintf("%v", err)))
		return
	}
	if len(parts) < 2 {
		go r.procDialRaw(channel, frame.SID, conn, parts[0])
		return
	}
	nextName := parts[0]
	if channel.Name() == nextName {
		err = fmt.Errorf("self dial error")
		DebugLog("Router(%v) proc dial to %v on channel(%v) fail with select channel error %v", r.Name, conn, channel, err)
		message := err.Error()
		err = writeMessage(channel, nil, frame.SID, CmdDialBack, []byte(message))
		return
	}
	next, err := r.SelectChannel(nextName)
	if err != nil {
		DebugLog("Router(%v) proc dial to %v on channel(%v) fail with select channel error %v", r.Name, conn, channel, err)
		message := err.Error()
		err = writeMessage(channel, nil, frame.SID, CmdDialBack, []byte(message))
		return
	}
	localID := next.AllocConnID()
	nextSID := ConnID{localID, 0}
	if ShowLog > 1 {
		DebugLog("Router(%v) forwarding dial(%v-%v->%v-%v) %v to channel(%v)", r.Name, channel.ID(), frame.SID, next.ID(), nextSID, conn, next)
	}
	r.addTable(channel, frame.SID, next, nextSID, conn)
	writeError := writeMessage(next, nil, nextSID, CmdDialConn, []byte(path[0]+"->"+nextName+"|"+parts[1]))
	if writeError != nil {
		WarnLog("Router(%v) send dial to channel(%v) fail with %v", r.Name, next, writeError)
		message := writeError.Error()
		err = writeMessage(channel, nil, frame.SID, CmdDialBack, []byte(message))
		r.removeTable(channel, frame.SID)
		next.FreeConnID(localID)
	}
	return
}

func (r *Router) procDialBack(channel Conn, frame *RouterFrame) (err error) {
	DebugLog("Router(%v) proc dial back by %v on channel(%v)", r.Name, frame.SID, channel)
	router := r.findTable(channel, frame.SID)
	if router == nil {
		err = writeMessage(channel, nil, frame.SID, CmdConnClosed, []byte("closed"))
		return
	}
	router.Update(channel, frame.SID)
	next, nextSID := router.Next(channel)
	if next.Type() == ConnTypeRaw {
		msg := string(frame.Data)
		if msg == ConnOK {
			InfoLog("Router(%v) dial %v->%v success on %v", r.Name, next, router.URI, channel)
			r.updateTable(router)
			r.startConnRead(next, nextSID)
		} else {
			InfoLog("Router(%v) dial %v->%v fail with %v", r.Name, next, router.URI, msg)
			channel.FreeConnID(frame.SID.LocalID())
			r.removeTable(channel, frame.SID)
			if waiter, ok := next.(ReadyWaiter); ok {
				waiter.Ready(fmt.Errorf("%v", msg), nil)
			}
			next.Close()
		}
	} else {
		nextSID[0] = next.AllocConnID()
		router.Update(next, nextSID)
		r.updateTable(router)
		writeError := r.procNextForward(channel, frame.SID, next, nextSID, frame)
		if writeError != nil {
			r.removeTable(channel, frame.SID)
			err = writeMessage(channel, nil, frame.SID, CmdConnClosed, []byte("closed"))
		}
	}
	return
}

func (r *Router) startConnRead(conn Conn, sid ConnID) {
	conn.Ready(nil, func(err error) {
		if err == nil {
			r.waiter.Add(1)
			r.procConnRead(conn)
		} else {
			r.closeConn(conn, err)
		}
	})
}

func (r *Router) procConnData(conn Conn, frame *RouterFrame) (err error) {
	router := r.findTable(conn, frame.SID)
	if router == nil {
		if conn.Type() == ConnTypeRaw { //if raw should close the raw, else ignore
			err = fmt.Errorf("not router")
		} else {
			err = writeMessage(conn, nil, frame.SID, CmdConnClosed, []byte("closed"))
		}
		return
	}
	next, nextSID := router.Next(conn)
	if ShowLog > 1 {
		DebugLog("Router(%v) forwaring %v bytes by %v-%v->%v-%v, source:%v, next:%v, uri:%v", r.Name, len(frame.Data), conn.ID(), frame.SID, next.ID(), nextSID, conn, next, router.URI)
	}
	writeError := r.procNextForward(conn, frame.SID, next, nextSID, frame)
	if writeError != nil && conn.Type() == ConnTypeRaw {
		err = writeError
	}
	return
}

func (r *Router) procConnClosed(channel Conn, frame *RouterFrame) (err error) {
	message := string(frame.Data)
	DebugLog("Router(%v) the session(%v) is closed by %v", r.Name, frame.SID, message)
	router := r.removeTable(channel, frame.SID)
	if router != nil {
		next, nextID := router.Next(channel)
		if next.Type() == ConnTypeRaw {
			next.Close()
		} else {
			r.procNextForward(channel, frame.SID, next, nextID, frame)
			next.FreeConnID(nextID.LocalID())
		}
		channel.FreeConnID(frame.SID.LocalID())
	}
	return
}

func (r *Router) procNextForward(fromConn Conn, fromSid ConnID, next Conn, nextSID ConnID, frame *RouterFrame) (err error) {
	offset := next.GetDataOffset()
	copy(frame.Buffer[offset:offset+2], nextSID[:])
	err = next.WriteRouterFrame(frame)
	return
}

// JoinConn will add channel by the connected connection
func (r *Router) JoinConn(conn, args interface{}) (channel Conn, result xmap.M, err error) {
	channel = r.NewConn(conn, 0, ConnTypeChannel)
	data, _ := json.Marshal(args)
	DebugLog("Router(%v) login join connection %v by options %v", r.Name, channel, string(data))
	err = writeMessage(channel, nil, ZeroConnID, CmdLoginChannel, data)
	if err != nil {
		WarnLog("Router(%v) send login to %v fail with %v", r.Name, channel, err)
		return
	}
	frame, err := channel.ReadRouterFrame()
	if err != nil {
		WarnLog("Router(%v) read login back from %v fail with %v", r.Name, channel, err)
		return
	}
	result = xmap.M{}
	err = json.Unmarshal(frame.Data, &result)
	if err != nil || result.Int("code") != 0 || len(result.Str("name")) < 1 {
		err = fmt.Errorf("%v", string(frame.Data))
		WarnLog("Router(%v) login to %v fail with %v", r.Name, channel, err)
		return
	}
	remoteName := result.Str("name")
	channel.SetName(remoteName)
	r.Register(channel)
	InfoLog("Router(%v) login to %v success, bind to %v", r.Name, channel, remoteName)
	r.Handler.OnConnJoin(channel, args, result)
	return
}

// Dial to remote by uri and bind channel to raw connection. return the session id
func (r *Router) Dial(raw io.ReadWriteCloser, uri string) (sid ConnID, connID uint16, err error) {
	sid, conn, err := r.DialConn(raw, uri)
	if err == nil {
		connID = conn.ID()
	}
	return
}

// SyncDial will dial to remote by uri and wait dial successes
func (r *Router) SyncDial(raw io.ReadWriteCloser, uri string) (sid ConnID, connID uint16, err error) {
	sid, conn, err := r.DialConn(raw, uri)
	if err == nil {
		connID = conn.ID()
		if waiter, ok := conn.(ReadyWaiter); ok {
			err = waiter.Wait()
		}
	}
	return
}

func (r *Router) dialConnLoc(raw io.ReadWriteCloser, uri string) (sid ConnID, conn Conn, err error) {
	DebugLog("Router(%v) start raw(%v) dial to %v", r.Name, reflect.TypeOf(raw), uri)
	fromID := r.NewConnID()
	from := r.NewConn(raw, fromID, ConnTypeRaw)
	nextID := r.NewConnID()
	next, nextErr := r.Handler.DialRawConn(from, nextID, uri)
	if nextErr != nil {
		err = nextErr
		return
	}
	fromSID := ConnID{1, 1}
	nextSID := ConnID{1, 1}
	from.SetDataPrefix(MakeRawConnPrefix(fromSID, CmdConnData))
	next.SetDataPrefix(MakeRawConnPrefix(nextSID, CmdConnData))
	r.addTable(from, fromSID, next, nextSID, uri)
	r.startConnRead(from, sid)
	r.startConnRead(next, sid)
	sid = fromSID
	conn = from
	return
}

// DialConn will dial to remote by uri and bind channel to raw connection and return raw connection
func (r *Router) DialConn(raw io.ReadWriteCloser, uri string) (sid ConnID, conn Conn, err error) {
	defer func() {
		if err != nil {
			raw.Close()
		}
	}()
	parts := strings.SplitN(uri, "->", 2)
	if len(parts) < 2 {
		sid, conn, err = r.dialConnLoc(raw, uri)
		return
	}
	channel, err := r.SelectChannel(parts[0])
	if err != nil {
		return
	}
	localID := channel.AllocConnID()
	channelSID := ConnID{localID, 0}
	conn = r.NewConn(raw, 0, ConnTypeRaw)
	connSID := ConnID{localID, localID}
	conn.Context()["URI"] = uri
	conn.SetDataPrefix(MakeRawConnPrefix(connSID, CmdConnData))
	DebugLog("Router(%v) start dial(%v-%v->%v-%v) to %v on channel(%v)", r.Name, conn.ID(), connSID, channel.ID(), channelSID, uri, channel)
	r.addTable(channel, channelSID, conn, connSID, uri)
	err = writeMessage(channel, nil, channelSID, CmdDialConn, []byte(fmt.Sprintf("%v|%v", parts[0], parts[1])))
	if err != nil {
		r.removeTable(channel, sid)
		channel.FreeConnID(localID)
	}
	sid = connSID
	return
}

func (r *Router) Start() (err error) {
	r.waiter.Add(1)
	go r.procPingLoop()
	r.waiter.Add(1)
	go r.procTimeoutLoop()
	return
}

// Close all channel
func (r *Router) Stop() (err error) {
	for i := 0; i < 10; i++ {
		select {
		case r.exiter <- 1:
		default:
		}
	}
	all := []io.Closer{}
	r.channelLck.Lock()
	for _, bond := range r.channelAll {
		all = append(all, bond)
	}
	r.channelLck.Unlock()
	r.tableLck.Lock()
	for _, table := range r.tableAll {
		all = append(all, table.FromConn, table.NextConn)
	}
	r.tableLck.Unlock()
	InfoLog("Router(%v) router will close %v connection", r.Name, len(all))
	for _, closer := range all {
		InfoLog("Router(%v) %v is closing", r.Name, closer)
		closer.Close()
	}
	r.waiter.Wait()
	return
}

// State return the current state of router
func (r *Router) State(args ...interface{}) (state xmap.M) {
	state = xmap.M{}
	state["channels"] = r.DisplayChannel(xmap.M{})
	state["table"] = r.DisplayTable(xmap.M{})
	return
}

// StateH return the current state of router
func (r *Router) StateH(res http.ResponseWriter, req *http.Request) {
	var query = xmap.M{}
	for key := range req.URL.Query() {
		query[key] = req.URL.Query().Get(key)
	}
	state := r.State(query)
	fmt.Fprintf(res, "%v", converter.JSON(state))

}

// DialPiper will dial uri on router and return piper
func (r *Router) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	piper := NewRouterPiper()
	_, _, err = r.SyncDial(piper, uri)
	raw = piper
	return
}

// RouterPiper is Waiter/Piper implement
type RouterPiper struct {
	raw    io.ReadWriteCloser
	next   func(err error)
	ready  int
	failed error
	waiter *sync.Cond
}

// NewRouterPiper will return new RouterPiper
func NewRouterPiper() (piper *RouterPiper) {
	piper = &RouterPiper{
		waiter: sync.NewCond(&sync.Mutex{}),
	}
	return
}

// Wait will wait piper is ready
func (r *RouterPiper) Wait() error {
	if r.ready < 1 {
		r.waiter.L.Lock()
		if r.ready < 1 {
			r.waiter.Wait()
		}
		r.waiter.L.Unlock()
	}
	return r.failed
}

// Ready will set piper is ready, failed/next at lasted is not nil
func (r *RouterPiper) Ready(failed error, next func(err error)) {
	r.waiter.L.Lock()
	r.failed = failed
	r.ready = 1
	r.next = next
	r.waiter.Broadcast()
	r.waiter.L.Unlock()
}

// PipeConn will pipe connection, it must be called after Wait success, or panic
func (r *RouterPiper) PipeConn(conn io.ReadWriteCloser, target string) (err error) {
	r.Wait()
	err = r.failed
	if err == nil {
		r.raw = conn
		r.next(err)
		err = fmt.Errorf("pipe done")
	} else {
		conn.Close()
	}
	return
}

// Close will close ready piper, it will lock when it is not ready
func (r *RouterPiper) Close() (err error) {
	r.waiter.L.Lock()
	r.ready = 1
	if r.failed == nil {
		r.failed = fmt.Errorf("closed")
	}
	r.waiter.Broadcast()
	r.waiter.L.Unlock()
	if r.raw != nil {
		r.raw.Close()
	}
	return r.failed
}

func (r *RouterPiper) Read(p []byte) (n int, err error) {
	r.Wait()
	err = r.failed
	if err == nil {
		n, err = r.raw.Read(p)
	}
	return
}

func (r *RouterPiper) Write(p []byte) (n int, err error) {
	r.Wait()
	err = r.failed
	if err == nil {
		n, err = r.raw.Write(p)
	}
	return
}

func (r *RouterPiper) String() string {
	return fmt.Sprintf("RouterPiper(%v)", xio.RemoteAddr(r.raw))
}

// NormalAcessHandler is normal access handler for proxy handler
type NormalAcessHandler struct {
	Name        string            //the access name
	LoginAccess map[string]string //the access control
	DialAccess  [][]string
	ConnDialer  ConnDialer
	RawDialer   RawDialer
	NetDialer   xnet.Dialer
	lock        sync.RWMutex
}

// NewNormalAcessHandler will return new handler
func NewNormalAcessHandler(name string) (handler *NormalAcessHandler) {
	handler = &NormalAcessHandler{
		Name:        name,
		LoginAccess: map[string]string{},
		NetDialer:   xnet.NewNetDailer(),
		lock:        sync.RWMutex{},
	}
	return
}

// DialRaw is proxy handler to dial remove
func (n *NormalAcessHandler) DialRawConn(channel Conn, id uint16, uri string) (conn Conn, err error) {
	var raw io.ReadWriteCloser
	if n.ConnDialer != nil {
		conn, err = n.ConnDialer.DialRawConn(channel, id, uri)
	} else if n.RawDialer != nil {
		raw, err = n.RawDialer.DialRawStd(channel, id, uri)
		if err == nil {
			conn = NewRouterConn(frame.NewRawReadWriteCloser(channel, raw, channel.BufferSize()), id, ConnTypeRaw)
		}
	} else if n.NetDialer != nil {
		raw, err = n.NetDialer.Dial(uri)
		if err == nil {
			conn = NewRouterConn(frame.NewRawReadWriteCloser(channel, raw, channel.BufferSize()), id, ConnTypeRaw)
		}
	} else {
		err = fmt.Errorf("not supported")
	}
	return
}

func (n *NormalAcessHandler) loginAccess(name, token string) (matched string) {
	n.lock.RLock()
	defer n.lock.RUnlock()
	for key, val := range n.LoginAccess {
		keyPattern, err := regexp.Compile(key)
		if err != nil {
			WarnLog("NormalAcessHandler(%v) compile acl key regexp(%v) fail with %v", n.Name, key, err)
			continue
		}
		valPattern, err := regexp.Compile(val)
		if err != nil {
			WarnLog("NormalAcessHandler(%v) compile acl token regexp(%v) fail with %v", n.Name, val, err)
			continue
		}
		if keyPattern.MatchString(name) && valPattern.MatchString(token) {
			matched = key
			break
		}
	}
	return
}

// OnConnLogin is proxy handler to handle login
func (n *NormalAcessHandler) OnConnLogin(channel Conn, args string) (name string, result xmap.M, err error) {
	var option = xmap.M{}
	err = json.Unmarshal([]byte(args), &option)
	if err != nil {
		ErrorLog("NormalAcessHandler(%v) unmarshal login option fail with %v", n.Name, err)
		err = fmt.Errorf("parse login option fail with " + err.Error())
		return
	}
	var token string
	err = option.ValidFormat(`
		name,R|S,L:0;
		token,R|S,L:0;
	`, &name, &token)
	if err != nil {
		ErrorLog("NormalAcessHandler(%v) login option fail with %v", n.Name, err)
		return
	}
	matched := n.loginAccess(name, token)
	if len(matched) < 1 {
		WarnLog("NormalAcessHandler(%v) login %v/%v fail with auth fail", n.Name, name, token)
		err = fmt.Errorf("access denied ")
		return
	}
	// channel.SetName(name)
	InfoLog("NormalAcessHandler(%v) channel %v login success on %v ", n.Name, name, channel)
	channel.Context()["option"] = option
	return
}

// OnConnDialURI is proxy handler to handle dial uri
func (n *NormalAcessHandler) OnConnDialURI(channel Conn, conn string, parts []string) (err error) {
	name := channel.Name()
	if len(name) < 1 {
		err = fmt.Errorf("not login")
		return
	}
	for _, entry := range n.DialAccess {
		if len(entry) != 2 {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with entry must be [<source>,<target>], but %v", n.Name, entry)
			continue
		}
		source, sourceError := regexp.Compile(entry[0])
		if sourceError != nil {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with %v by entry source %v", n.Name, sourceError, entry[0])
			continue
		}
		if !source.MatchString(name) {
			continue
		}
		target, targetError := regexp.Compile(entry[1])
		if targetError != nil {
			WarnLog("NormalAcessHandler(%v) compile dial access fail with %v by entry target %v", n.Name, targetError, entry[1])
			continue
		}
		if target.MatchString(name) {
			return nil
		}
	}
	err = fmt.Errorf("not access")
	return
}

// OnConnClose is proxy handler when connection is closed
func (n *NormalAcessHandler) OnConnClose(conn Conn) (err error) {
	return nil
}

// OnConnJoin is proxy handler when channel join
func (n *NormalAcessHandler) OnConnJoin(channel Conn, option interface{}, result xmap.M) {
}
