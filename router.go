package bsck

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	CmdLogin     = 10
	CmdLoginBack = 11
	CmdDial      = 100
	CmdDialBack  = 101
	CmdData      = 110
	CmdClosed    = 120
)

const (
	ConnTypeRaw     = 100
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

type Conn interface {
	io.ReadWriteCloser
	ID() uint64
	Name() string
	Index() int
	Type() int
}

type CmdReader interface {
	ReadCmd(b []byte) (n uint32, err error)
}

type RawConn struct {
	Raw io.ReadWriteCloser
	sid uint64
	uri string
}

func NewRawConn(raw io.ReadWriteCloser, sid uint64, uri string, bufferSize int) (conn *RawConn) {
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

func (r *RawConn) Close() (err error) {
	err = r.Raw.Close()
	return
}

func (r *RawConn) ID() uint64 {
	return r.sid
}
func (r *RawConn) Name() string {
	return ""
}
func (r *RawConn) Index() int {
	return 0
}
func (r *RawConn) String() string {
	return fmt.Sprintf("raw:%v", r.uri)
}
func (r *RawConn) Type() int {
	return ConnTypeRaw
}

type Channel struct {
	io.ReadWriteCloser
	cid   uint64
	name  string
	index int
}

func (c *Channel) ID() uint64 {
	return c.cid
}

func (c *Channel) Name() string {
	return c.name
}

func (c *Channel) Index() int {
	return c.index
}

func (c *Channel) String() string {
	return fmt.Sprintf("channel:%v,%v,%v", c.name, c.index, c.cid)
}

func (c *Channel) Type() int {
	return ConnTypeChannel
}

type DailRawF func(sid uint64, uri string) (raw Conn, err error)

type AuthOption struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

type ChannelOption struct {
	Token  string `json:"token"`
	Local  string `json:"local"`
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

type TableRouter []interface{}

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

type Router struct {
	Name            string
	BufferSize      uint32
	connectSequence uint64
	channel         map[string]*bondChannel
	channelLck      sync.RWMutex
	table           map[string]TableRouter
	tableLck        sync.RWMutex
	//
	ACL    map[string]string
	aclLck sync.RWMutex
	//
	DailRaw DailRawF
}

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
	}
	return
}

func (r *Router) Accept(raw io.ReadWriteCloser) {
	channel := &Channel{
		ReadWriteCloser: raw,
		cid:             atomic.AddUint64(&r.connectSequence, 1),
	}
	go r.loopReadRaw(channel, r.BufferSize)
}

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
		debugLog("Router(%v) read one command(%v,%v) from %v", r.Name, cmdString(buf[4]), length, channel)
		switch buf[4] {
		case CmdLogin:
			err = r.procLogin(channel, buf, length)
		case CmdDial:
			err = r.procDail(channel, buf, length)
		case CmdDialBack:
			err = r.procDailBack(channel, buf, length)
		case CmdData:
			err = r.procChannelData(channel, buf, length)
		case CmdClosed:
			err = r.procClosed(channel, buf, length)
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

func (r *Router) procDail(channel Conn, buf []byte, length uint32) (err error) {
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
		raw, cerr := r.DailRaw(dstSid, parts[0])
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

func (r *Router) procDailBack(channel Conn, buf []byte, length uint32) (err error) {
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

func (r *Router) Dail(uri string, raw io.ReadWriteCloser) (sid uint64, err error) {
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
	dst := NewRawConn(raw, sid, uri, 4096)
	r.addTable(channel, sid, dst, sid)
	err = writeCmd(channel, make([]byte, 4096), CmdDial, sid, []byte(fmt.Sprintf("%v@%v", parts[0], parts[1])))
	if err != nil {
		r.removeTable(channel, sid)
	}
	return
}

func (r *Router) LoginChannel(channels ...*ChannelOption) (err error) {
	for index, channel := range channels {
		err = r.Login(channel.Local, channel.Remote, channel.Token, index)
		if err != nil {
			return
		}
	}
	return
}

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
