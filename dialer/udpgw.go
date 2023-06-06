package dialer

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/codingeasygo/util/xio/frame"
	"github.com/codingeasygo/util/xmap"
)

const (
	UDPGW_CLIENT_FLAG_KEEPALIVE = (1 << 0)
	UDPGW_CLIENT_FLAG_REBIND    = (1 << 1)
	UDPGW_CLIENT_FLAG_DNS       = (1 << 2)
	UDPGW_CLIENT_FLAG_IPV6      = (1 << 3)
)

type UdpGwDialer struct {
	Header   frame.Header
	MTU      int
	DNS      *net.UDPAddr
	MaxConn  int
	MaxAlive time.Duration
	Delay    time.Duration
	connAll  map[uint16]*UdpGwConn
	connLock sync.RWMutex
	exiter   chan int
	waiter   sync.WaitGroup
}

func NewUdpGwDialer() (dialer *UdpGwDialer) {
	dialer = &UdpGwDialer{
		Header:   frame.NewDefaultHeader(),
		MTU:      1600,
		MaxConn:  256,
		Delay:    time.Second,
		connAll:  map[uint16]*UdpGwConn{},
		connLock: sync.RWMutex{},
		exiter:   make(chan int, 1),
		waiter:   sync.WaitGroup{},
	}
	dialer.Header.SetByteOrder(binary.LittleEndian)
	dialer.Header.SetLengthFieldMagic(0)
	dialer.Header.SetLengthFieldLength(2)
	dialer.Header.SetLengthAdjustment(-2)
	dialer.Header.SetDataOffset(2)
	return
}

func (u *UdpGwDialer) Name() string {
	return "UdpGw"
}

// initial dialer
func (u *UdpGwDialer) Bootstrap(options xmap.M) (err error) {
	var dns string
	var maxaLive time.Duration = 60000
	err = options.ValidFormat(`
		mtu,o|i,r:1500;
		dns,o|s,l:0;
		max_conn,o|i,r:0;
		max_alive,o|i,r:0;
	`, &u.MTU, &dns, &u.MaxConn, &maxaLive)
	if err != nil {
		return
	}
	u.MaxAlive = maxaLive * time.Millisecond
	u.DNS, err = net.ResolveUDPAddr("", dns)
	if err != nil {
		return
	}
	u.waiter.Add(1)
	go u.loopTimeout()
	return
}

// shutdown
func (u *UdpGwDialer) Shutdown() (err error) {
	u.exiter <- 1
	u.waiter.Wait()
	return
}

func (u *UdpGwDialer) Options() xmap.M {
	return xmap.M{}
}

// match uri
func (u *UdpGwDialer) Matched(uri string) bool {
	switch uri {
	case "tcp://udpgw", "tcp://udpgw.loc", "udpgw", "udpgw.loc", "tcp://127.0.0.3:10", "127.0.0.3:10":
		return true
	default:
		return false
	}
}

// dial raw connection
func (u *UdpGwDialer) Dial(channel Channel, sid uint16, uri string) (conn Conn, err error) {
	gw := NewUdpGwConn(sid)
	gw.Header = u.Header
	gw.MTU = u.MTU
	gw.DNS = u.DNS
	gw.MaxConn = u.MaxConn
	base := frame.NewPassReadWriteCloser(u.Header, gw, u.Header.GetDataOffset()+u.MTU)
	conn = &udpGwRWC{
		ReadWriteCloser: NewCopyPipable(base),
		Info:            gw,
	}
	u.connLock.Lock()
	defer u.connLock.Unlock()
	u.connAll[sid] = gw
	return
}

func (u *UdpGwDialer) closeConn(conn *UdpGwConn) {
	u.connLock.Lock()
	defer u.connLock.Unlock()
	delete(u.connAll, conn.ID)
}

func (u *UdpGwDialer) loopTimeout() {
	defer u.waiter.Done()
	ticker := time.NewTicker(u.Delay)
	running := true
	for running {
		select {
		case <-ticker.C:
			u.procTimeout()
		case <-u.exiter:
			running = false
		}
	}
}

func (u *UdpGwDialer) procTimeout() {
	defer func() {
		if perr := recover(); perr != nil {
			WarnLog("UdpGwDialer process timeout is panic by %v", perr)
		}
	}()
	u.connLock.RLock()
	connAll := []*UdpGwConn{}
	for _, conn := range u.connAll {
		connAll = append(connAll, conn)
	}
	u.connLock.RUnlock()
	for _, conn := range connAll {
		conn.Timeout()
	}
}

type udpGwRead struct {
	Buffer []byte
	Readed int
}

type udpGwRawConn struct {
	raw   *net.UDPConn
	addr  *net.UDPAddr
	orig  *net.UDPAddr
	conid uint16
	flags uint8
	last  time.Time
}

type udpGwRWC struct {
	io.ReadWriteCloser
	Info interface{}
}

func (u *udpGwRWC) String() string {
	return fmt.Sprintf("%v", u.Info)
}

type UdpGwConn struct {
	frame.Header
	ID         uint16
	MTU        int
	DNS        *net.UDPAddr
	MaxConn    int
	MaxAlive   time.Duration
	Dialer     *UdpGwDialer
	connAll    map[uint16]*udpGwRawConn
	connLock   sync.RWMutex
	readBuffer chan *udpGwRead
	readWaiter sync.WaitGroup
	readClosed int
	exiter     chan int
	waiter     sync.WaitGroup
}

func NewUdpGwConn(id uint16) (conn *UdpGwConn) {
	conn = &UdpGwConn{
		ID:         id,
		Header:     frame.NewDefaultHeader(),
		MTU:        1600,
		MaxConn:    256,
		MaxAlive:   time.Minute,
		connAll:    map[uint16]*udpGwRawConn{},
		connLock:   sync.RWMutex{},
		readBuffer: make(chan *udpGwRead, 1024),
		readWaiter: sync.WaitGroup{},
		readClosed: 0,
		exiter:     make(chan int, 8),
		waiter:     sync.WaitGroup{},
	}
	return
}

func (u *UdpGwConn) Read(p []byte) (n int, err error) {
	if len(p) < u.MTU {
		err = fmt.Errorf("p must greater mtu %v>=%v", len(p), u.MTU)
		return
	}
	if u.readClosed > 0 {
		err = io.EOF
		return
	}
	read := &udpGwRead{
		Buffer: p,
		Readed: 0,
	}
	u.readWaiter.Add(1)
	u.readBuffer <- read
	u.readWaiter.Wait()
	n = read.Readed
	if n < 1 {
		err = io.EOF
	}
	return
}

func (u *UdpGwConn) Write(p []byte) (n int, err error) { //recv
	if len(p) < 3 {
		err = fmt.Errorf("data error")
		return
	}
	flags := uint8(p[0])
	conid := binary.BigEndian.Uint16(p[1:])
	if flags&UDPGW_CLIENT_FLAG_KEEPALIVE == UDPGW_CLIENT_FLAG_KEEPALIVE {
		n = len(p)
		return
	}
	var addrIP net.IP
	var addrPort uint16
	var data []byte
	if flags&UDPGW_CLIENT_FLAG_IPV6 == UDPGW_CLIENT_FLAG_IPV6 {
		addrIP = net.IP(p[3:19])
		addrPort = binary.BigEndian.Uint16(p[19:21])
		data = p[21:]
	} else {
		addrIP = net.IP(p[3:7])
		addrPort = binary.BigEndian.Uint16(p[7:9])
		data = p[9:]
	}
	u.connLock.RLock()
	conn := u.connAll[conid]
	u.connLock.RUnlock()
	if conn == nil {
		u.limitConn()
		orig := &net.UDPAddr{IP: addrIP, Port: int(addrPort)}
		addr := orig
		if flags&UDPGW_CLIENT_FLAG_DNS == UDPGW_CLIENT_FLAG_DNS && u.DNS != nil {
			addr = u.DNS
		}
		conn = &udpGwRawConn{conid: conid, flags: flags, addr: addr, orig: orig, last: time.Now()}
		conn.raw, err = net.DialUDP("udp", nil, addr)
		if err != nil {
			WarnLog("UdpGwConn(%v) udp conn(%v) dial to %v fail with %v", u.ID, conn.conid, addr, err)
			return
		}
		u.connLock.Lock()
		u.connAll[conid] = conn
		opened := len(u.connAll)
		u.connLock.Unlock()
		DebugLog("UdpGwConn(%v) %v/%v udp conn(%v) dial to %v success", u.ID, opened, u.MaxConn, conn.conid, addr)
		u.waiter.Add(1)
		go u.procRead(conn)
	}
	conn.last = time.Now()
	n, err = conn.raw.Write(data)
	n += len(addrIP) + 5
	return
}

func (u *UdpGwConn) procRead(conn *udpGwRawConn) {
	var err error
	defer func() {
		if perr := recover(); perr != nil {
			WarnLog("UdpGwConn(%v) process raw read is panic by %v", u.ID, perr)
		}
		u.waiter.Done()
		u.connLock.Lock()
		delete(u.connAll, conn.conid)
		opened := len(u.connAll)
		u.connLock.Unlock()
		conn.raw.Close()
		DebugLog("UdpGwConn(%v) %v/%v udp conn(%v) to %v is closed by %v", u.ID, opened, u.MaxConn, conn.conid, conn.addr, err)
	}()
	buffer := make([]byte, u.MTU)
	offset := 0
	if conn.flags&UDPGW_CLIENT_FLAG_IPV6 == UDPGW_CLIENT_FLAG_IPV6 {
		buffer[offset] = UDPGW_CLIENT_FLAG_IPV6
	} else {
		buffer[offset] = 0
	}
	if conn.flags&UDPGW_CLIENT_FLAG_DNS == UDPGW_CLIENT_FLAG_DNS {
		buffer[offset] |= UDPGW_CLIENT_FLAG_DNS
	}
	offset += 1
	binary.BigEndian.PutUint16(buffer[offset:], conn.conid)
	offset += 2
	offset += copy(buffer[offset:], conn.orig.IP)
	binary.BigEndian.PutUint16(buffer[offset:], uint16(conn.orig.Port))
	offset += 2
	running := true
	for running {
		n, xerr := conn.raw.Read(buffer[offset:])
		if xerr != nil {
			err = xerr
			break
		}
		conn.last = time.Now()
		select {
		case p := <-u.readBuffer:
			if p != nil {
				p.Readed = copy(p.Buffer, buffer[:offset+n])
				u.readWaiter.Done()
			} else {
				running = false
			}
		case <-u.exiter:
			running = false
		}
	}
}

func (u *UdpGwConn) limitConn() {
	u.connLock.Lock()
	defer u.connLock.Unlock()
	if len(u.connAll) < u.MaxConn {
		return
	}
	var oldest *udpGwRawConn
	for _, conn := range u.connAll {
		if oldest == nil || oldest.last.After(conn.last) {
			oldest = conn
		}
	}
	if oldest != nil {
		DebugLog("UdpGwConn(%v) closing connection %v by limit %v/%v", u.ID, oldest.addr, len(u.connAll), u.MaxConn)
		oldest.raw.Close()
		delete(u.connAll, oldest.conid)
	}
}

func (u *UdpGwConn) cloaseAllConn() (closed int) {
	u.connLock.Lock()
	defer u.connLock.Unlock()
	closed = len(u.connAll)
	for connid, conn := range u.connAll {
		conn.raw.Close()
		delete(u.connAll, connid)
	}
	return
}

func (u *UdpGwConn) Close() (err error) {
	closed := u.cloaseAllConn()
	for i := 0; i < closed; i++ {
		select {
		case u.readBuffer <- nil:
		default:
		}
	}
	u.waiter.Wait()
	u.readClosed = 1
	for i := 0; i < 10; i++ {
		select {
		case p := <-u.readBuffer:
			if p != nil {
				u.readWaiter.Done()
			}
		default:
		}
	}
	if u.Dialer != nil {
		u.Dialer.closeConn(u)
	}
	return
}

func (u *UdpGwConn) Timeout() {
	u.connLock.Lock()
	defer u.connLock.Unlock()
	now := time.Now()
	for _, conn := range u.connAll {
		if now.Sub(conn.last) > u.MaxAlive {
			conn.raw.Close()
		}
	}
}

func (u *UdpGwConn) String() string {
	u.connLock.RLock()
	defer u.connLock.RUnlock()
	return fmt.Sprintf("UdpGwConn:%v/%v", len(u.connAll), u.MaxConn)
}
