package bsck

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	Raw io.ReadWriteCloser
	sid uint64
	uri string
}

// NewRawConn returns a new RawConn by raw connection/session id/uri
func NewRawConn(raw io.ReadWriteCloser, sid uint64, uri string) (conn *RawConn) {
	conn = &RawConn{
		Raw: raw,
		sid: sid,
		uri: uri,
	}
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
	err = r.Raw.Close()
	return
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

func (r *RawConn) String() string {
	return fmt.Sprintf("raw:%v", r.uri)
}

//Channel is an implementation of the Conn interface for channel network connections.
type Channel struct {
	//the raw connection
	io.ReadWriteCloser
	cid   uint64
	name  string
	index int
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

func (c *Channel) String() string {
	return fmt.Sprintf("channel:%v,%v,%v", c.name, c.index, c.cid)
}

//DialRawF is a function type to dial raw connection.
type DialRawF func(sid uint64, uri string) (raw Conn, err error)

//DialTCP is an implementation of DialRawF for tcp raw connection.
func DialTCP(sid uint64, uri string) (raw Conn, err error) {
	targetURI, err := url.Parse(uri)
	if err != nil {
		return
	}
	conn, err := net.Dial("tcp", targetURI.Host)
	if err == nil {
		raw = NewRawConn(conn, sid, uri)
	}
	return
}

//AuthOption is a pojo struct to login auth.
type AuthOption struct {
	//the channel index
	Index int `json:"index"`
	//the chnnale name
	Name string `json:"name"`
	//the auth token
	Token string `json:"token"`
}

//ChannelOption is a pojo struct for adding channel to Router
type ChannelOption struct {
	//the auth token
	Token string `json:"token"`
	//local tcp address to connection master
	Local string `json:"local"`
	//the remote address to login
	Remote string `json:"remote"`
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

//Router is an implementation of the router control
type Router struct {
	Name            string            //current router name
	BufferSize      uint32            //buffer size of connection runner
	DialRaw         DialRawF          //the function to dial raw connection
	ACL             map[string]string //the access control
	Heartbeat       time.Duration     //the delay of heartbeat
	aclLck          sync.RWMutex
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
		ACL:        map[string]string{},
		aclLck:     sync.RWMutex{},
		BufferSize: 1024 * 1024,
		Heartbeat:  5 * time.Second,
	}
	return
}

//StartHeartbeat will start the hearbeat on slaver/master
func (r *Router) StartHeartbeat() {
	if r.Heartbeat > 0 {
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
func (r *Router) Register(name string, index int, raw io.ReadWriteCloser) {
	channel := &Channel{
		ReadWriteCloser: raw,
		cid:             atomic.AddUint64(&r.connectSequence, 1),
		name:            name,
		index:           index,
	}
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
	bond.channels[channel.Index()] = channel
	bond.channelLck.Unlock()
	r.channel[channel.Name()] = bond
	r.channelLck.Unlock()
}

//SelectChannel will pick one channel by name.
func (r *Router) SelectChannel(name string) (dst Conn) {
	r.channelLck.RLock()
	bond := r.channel[name]
	r.channelLck.RUnlock()
	if bond == nil {
		return
	}
	var index int
	var min uint64 = math.MaxUint64
	bond.channelLck.Lock()
	for i, c := range bond.channels {
		used := bond.used[i]
		if used > min {
			continue
		}
		dst = c
		min = used
		index = i
	}
	if dst != nil {
		bond.used[index]++
	}
	bond.channelLck.Unlock()
	return dst
}

func (r *Router) addTable(src Conn, srcSid uint64, dst Conn, dstSid uint64) {
	r.tableLck.Lock()
	router := []interface{}{src, srcSid, dst, dstSid}
	r.table[fmt.Sprintf("%v-%v", src.ID(), srcSid)] = router
	r.table[fmt.Sprintf("%v-%v", dst.ID(), dstSid)] = router
	r.tableLck.Unlock()
	return
}

func (r *Router) removeTable(conn Conn, sid uint64) {
	r.tableLck.Lock()
	router := r.table[fmt.Sprintf("%v-%v", conn.ID(), sid)]
	if router != nil {
		delete(r.table, fmt.Sprintf("%v-%v", router[0].(Conn).ID(), router[1]))
		delete(r.table, fmt.Sprintf("%v-%v", router[2].(Conn).ID(), router[3]))
	}
	r.tableLck.Unlock()
}

func (r *Router) loopReadRaw(channel Conn, bufferSize uint32) {
	var err error
	var last int64
	var length uint32
	buf := make([]byte, bufferSize)
	infoLog("Router(%v) the reader(%v) is starting", r.Name, channel)
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
			errorLog("Router(%v) receive invalid frame(length:%v) from %v", length, channel)
			break
		}
		if ShowLog > 1 {
			debugLog("Router(%v) read one command(%v,%v) from %v", r.Name, cmdString(buf[4]), length, channel)
		}
		switch buf[4] {
		case CmdLogin:
			err = r.procLogin(channel, buf, length)
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
	infoLog("Router(%v) the reader(%v) is stopped by %v", r.Name, channel, err)
	r.channelLck.Lock()
	bond := r.channel[channel.Name()]
	if bond != nil {
		bond.channelLck.Lock()
		bond.channels[channel.Index()] = channel
		delete(bond.channels, channel.Index())
		bond.channelLck.Unlock()
	}
	r.channelLck.Unlock()
	//
	r.tableLck.Lock()
	if channel.Type() == ConnTypeRaw {
		router := r.table[fmt.Sprintf("%v-%v", channel.ID(), channel.ID())]
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
		}
	}
	r.tableLck.Unlock()
}

func (r *Router) loopHeartbeat() {
	data := []byte("ping...")
	length := uint32(len(data) + 13)
	buf := make([]byte, length)
	binary.BigEndian.PutUint32(buf, length-4)
	buf[4] = CmdHeartbeat
	binary.BigEndian.PutUint32(buf[5:], 0)
	copy(buf[13:], data)
	for {
		r.channelLck.RLock()
		for _, bond := range r.channel {
			bond.channelLck.RLock()
			for _, channel := range bond.channels {
				channel.Write(buf)
			}
			bond.channelLck.RUnlock()
		}
		r.channelLck.RUnlock()
		time.Sleep(r.Heartbeat)
	}
}

func (r *Router) procLogin(conn Conn, buf []byte, length uint32) (err error) {
	channel, ok := conn.(*Channel)
	if !ok {
		errorLog("Router(%v) proc login error with connection is not channel", r.Name)
		err = fmt.Errorf("connection is not channel")
		return
	}
	var option AuthOption
	err = json.Unmarshal(buf[13:length], &option)
	if err != nil {
		errorLog("Router(%v) unmarshal login option fail with %v", r.Name, err)
		err = writeCmd(conn, buf, CmdLoginBack, 0, []byte("parse login opiton fail with "+err.Error()))
		return
	}
	if len(option.Name) < 1 || len(option.Token) < 1 {
		errorLog("Router(%v) login option fail with name/token is required", r.Name)
		err = writeCmd(conn, buf, CmdLoginBack, 0, []byte("name/token is requried"))
		return
	}
	r.aclLck.RLock()
	token := r.ACL[option.Name]
	r.aclLck.RUnlock()
	if token != option.Token {
		warnLog("Router(%v) login fail with auth fail", r.Name)
		err = writeCmd(conn, buf, CmdLoginBack, 0, []byte("access denied "))
		return
	}
	channel.name = option.Name
	channel.index = option.Index
	r.addChannel(channel)
	writeCmd(channel, buf, CmdLoginBack, 0, []byte("OK:"+r.Name))
	infoLog("Router(%v) the channel(%v,%v) is login success", r.Name, option.Name, option.Index)
	return
}

func (r *Router) procDial(channel Conn, buf []byte, length uint32) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	conn := string(buf[13:length])
	debugLog("Router(%v) proc dail to %v on channel(%v)", r.Name, conn, channel)
	path := strings.SplitN(conn, "@", 2)
	if len(path) < 2 {
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte(fmt.Sprintf("invalid uri(%v)", conn)))
		return
	}
	parts := strings.SplitN(path[1], "->", 2)
	if len(parts) < 2 {
		dstSid := atomic.AddUint64(&r.connectSequence, 1)
		raw, cerr := r.DialRaw(dstSid, parts[0])
		if cerr != nil {
			err = writeCmd(channel, buf, CmdDialBack, sid, []byte(fmt.Sprintf("dial to uri(%v) fail with %v", parts[0], cerr)))
			return
		}
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte("OK"))
		if err == nil {
			r.Bind(channel, sid, raw, dstSid)
		}
		return
	}
	next := parts[0]
	dst := r.SelectChannel(next)
	if dst == nil {
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte("not connected by "+next))
		return
	}
	dstSid := atomic.AddUint64(&r.connectSequence, 1)
	r.addTable(channel, sid, dst, dstSid)
	werr := writeCmd(dst, buf, CmdDial, dstSid, []byte(path[0]+"->"+next+"@"+parts[1]))
	if werr != nil {
		warnLog("Router(%v) send dial to channel(%v) fail with %v", r.Name, dst, werr)
		err = writeCmd(channel, buf, CmdDialBack, sid, []byte(werr.Error()))
		r.removeTable(channel, sid)
	}
	return
}

func (r *Router) procDialBack(channel Conn, buf []byte, length uint32) (err error) {
	sid := binary.BigEndian.Uint64(buf[5:])
	debugLog("Router(%v) proc dail back by %v on channel(%v)", r.Name, sid, channel)
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
			infoLog("Router(%v) dail to %v success", r.Name, target)
			r.Bind(channel, sid, target, target.ID())
		} else {
			infoLog("Router(%v) dail to %v fail with %v", r.Name, target, msg)
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
	if router == nil {
		err = writeCmd(channel, buf, CmdClosed, sid, []byte("closed"))
		return
	}
	target, rsid := router.Next(channel)
	binary.BigEndian.PutUint64(buf[5:], rsid)
	_, werr := target.Write(buf[:length])
	if werr != nil {
		err = writeCmd(channel, buf, CmdClosed, sid, []byte("closed"))
	}
	return
}

func (r *Router) procClosed(channel Conn, buf []byte, length uint32) (err error) {
	message := string(buf[13:length])
	sid := binary.BigEndian.Uint64(buf[5:])
	debugLog("Router(%v) the session(%v) is closed by %v", r.Name, sid, message)
	r.tableLck.RLock()
	router := r.table[fmt.Sprintf("%v-%v", channel.ID(), sid)]
	r.tableLck.RUnlock()
	if router == nil {
		return
	}
	r.removeTable(channel, sid)
	target, rsid := router.Next(channel)
	if target.Type() == ConnTypeRaw {
		target.Close()
	} else {
		writeCmd(target, buf, CmdClosed, rsid, []byte(message))
	}
	return
}

func (r *Router) procHeartbeat(channel Conn, buf []byte, length uint32) (err error) {
	return
}

//Dial to remote by uri and bind channel to raw connection.
//
//return the session id
func (r *Router) Dial(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
	parts := strings.SplitN(uri, "->", 2)
	if len(parts) < 2 {
		err = fmt.Errorf("invalid uri(%v), it must like x->y", uri)
		return
	}
	channel := r.SelectChannel(parts[0])
	if channel == nil {
		err = fmt.Errorf("channel is not exist by %v", parts[0])
		return
	}
	debugLog("Router(%v) start dail to %v on channel(%v)", r.Name, uri, channel)
	sid = atomic.AddUint64(&r.connectSequence, 1)
	dst := NewRawConn(raw, sid, uri)
	r.addTable(channel, sid, dst, sid)
	err = writeCmd(channel, make([]byte, 4096), CmdDial, sid, []byte(fmt.Sprintf("%v@%v", parts[0], parts[1])))
	if err != nil {
		r.removeTable(channel, sid)
	}
	return
}

//LoginChannel will login all channel by options.
func (r *Router) LoginChannel(channels ...*ChannelOption) (err error) {
	for index, channel := range channels {
		err = r.Login(channel.Local, channel.Remote, channel.Token, index)
		if err != nil {
			return
		}
	}
	return
}

//Login will add channel by local address, master address, auth token, channel index.
func (r *Router) Login(local, address, token string, index int) (err error) {
	infoLog("Router(%v) start dial to %v", r.Name, address)
	var dialer net.Dialer
	if len(local) > 0 {
		dialer.LocalAddr, err = net.ResolveTCPAddr("tcp", local)
		if err != nil {
			return
		}
	}
	conn, err := dialer.Dial("tcp", address)
	if err != nil {
		warnLog("Router dial to %v fail with %v", address, err)
		return
	}
	err = r.JoinConn(conn, token, index)
	return
}

//JoinConn will add channel by the connected connection, auth token, channel index
func (r *Router) JoinConn(conn io.ReadWriteCloser, token string, index int) (err error) {
	data, _ := json.Marshal(&AuthOption{
		Name:  r.Name,
		Token: token,
		Index: index,
	})
	buf := make([]byte, 1024)
	err = writeCmd(conn, buf, CmdLogin, 0, data)
	if err != nil {
		warnLog("Router(%v) send login to %v fail with %v", r.Name, conn, err)
		return
	}
	length, err := readCmd(conn, buf, nil)
	if err != nil {
		warnLog("Router(%v) read login back from %v fail with %v", r.Name, conn, err)
		return
	}
	msg := string(buf[13:length])
	if !strings.HasPrefix(msg, "OK:") {
		err = fmt.Errorf("%v", msg)
		warnLog("Router(%v) login to %v fail with %v", r.Name, conn, err)
		return
	}
	remote := strings.TrimPrefix(msg, "OK:")
	r.Register(remote, index, NewBufferConn(conn, 4096))
	infoLog("Router(%v) login to %v success, bind to %v,%v", r.Name, conn, remote, index)
	return
}
