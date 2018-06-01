package bsck

// type DataHandler interface {
// 	OnData(channel *Channel, data []byte) (err error)
// }

// type Transport struct {
// 	SRC *Channel
// 	DST *Channel
// }

// type Master struct {
// 	session    map[uint64]*Transport
// 	sessionLck sync.RWMutex
// 	ACL        map[string]string
// 	aclLck     sync.RWMutex
// 	channel    map[string]*bondChannel
// 	channelLck sync.RWMutex
// }

// func (m *Master) Accept(raw net.Conn) (err error) {
// 	return
// }

// func (m *Master) loopReadRaw(channel *Channel, bufferSize uint64) {
// 	var err error
// 	var readed int
// 	var last int64
// 	buf := make([]byte, bufferSize)
// 	for {
// 		readed, err = channel.Read(buf[:4])
// 		if err != nil {
// 			break
// 		}
// 		if readed < 4 {
// 			err = fmt.Errorf("head too small")
// 			break
// 		}
// 		length := binary.BigEndian.Uint64(buf)
// 		if length+4 > bufferSize {
// 			err = fmt.Errorf("frame too large")
// 			break
// 		}
// 		err = fullBuf(channel, buf[4:], length, &last)
// 		if err != nil {
// 			break
// 		}
// 		data := buf[:4+length]
// 		switch buf[4] {
// 		case CmdLogin:
// 			err = m.procLogin(channel, data)
// 		case CmdDial:
// 			err = m.procDail(channel, data)
// 		case CmdData:
// 			err = m.procRawData(channel, data)
// 		}
// 		if err != nil {
// 			break
// 		}
// 	}
// }

// func (m *Master) procLogin(channel *Channel, data []byte) (err error) {
// 	var option LoginOption
// 	err = json.Unmarshal(data[4:], &option)
// 	if err != nil {
// 		errorLog("Master unmarshal login option fail with %v", err)
// 		return
// 	}
// 	m.aclLck.RLock()
// 	token := m.ACL[option.Name]
// 	m.aclLck.RUnlock()
// 	if token != option.Token {
// 		warnLog("Master login fail with auth fail")
// 		return
// 	}
// 	m.channelLck.Lock()
// 	bond := m.channel[option.Name]
// 	if bond == nil {
// 		bond = newBondChannel()
// 	}
// 	//
// 	bond.channelLck.Lock()
// 	old := bond.channels[option.Index]
// 	if old != nil {
// 		warnLog("Master the channel(%v,%v) is exitst, will close it", option.Name, option.Index)
// 		old.Close()
// 	}
// 	bond.channels[option.Index] = channel
// 	bond.channelLck.Unlock()
// 	//
// 	m.channel[option.Name] = bond
// 	m.channelLck.Unlock()
// 	infoLog("Master the channel(%v,%v) is login success", option.Name, option.Index)
// 	return
// }

// func (m *Master) procDail(channel *Channel, data []byte) (err error) {
// 	conn := string(data[5:])
// 	router := strings.SplitN(conn, "@", 2)
// 	if len(router) < 2 {
// 		err = fmt.Errorf("invalid uri(%v)", conn)
// 		return
// 	}
// 	parts := strings.SplitN(router[1], "->", 2)
// 	if len(parts) < 2 {
// 		err = fmt.Errorf("invalid uri(%v)", conn)
// 		return
// 	}
// 	dst := parts[0]
// 	m.channelLck.RLock()
// 	bond := m.channel[dst]
// 	m.channelLck.RUnlock()
// 	var berr error
// 	if bond != nil {
// 		nextConn := router[0] + "->" + dst + "@" + parts[1]
// 		berr = bond.procDail(channel, nextConn)
// 		if berr == nil {
// 			return
// 		}
// 	} else {
// 		berr = fmt.Errorf("channel not found by %v", dst)
// 	}
// 	msg := []byte(berr.Error())
// 	buf := make([]byte, 5+len(msg))
// 	binary.BigEndian.PutUint64(buf, uint64(len(msg)+1))
// 	buf[4] = CmdDialBack
// 	copy(buf[:5], msg)
// 	_, err = channel.Write(buf)
// 	return
// }

// func (m *Master) procRawData(channel *Channel, data []byte) (err error) {
// 	sid := binary.BigEndian.Uint64(data[5:])
// 	m.sessionLck.RLock()
// 	transport := m.session[sid]
// 	m.sessionLck.RUnlock()
// 	if transport != nil {
// 		if transport.SRC == channel {
// 			_, err = transport.DST.Write(data)
// 		} else {
// 			_, err = transport.SRC.Write(data)
// 		}
// 		if err == nil {
// 			return
// 		}
// 	}
// 	buf := make([]byte, 9)
// 	binary.BigEndian.PutUint64(buf, 5)
// 	buf[4] = CmdClosed
// 	binary.BigEndian.PutUint64(buf[5:], sid)
// 	_, err = channel.Write(buf)
// 	return
// }

// type Slaver struct {
// }

// func (s *Slaver) loopReadRaw(channel *Channel, bufferSize uint64) {
// 	var err error
// 	var readed int
// 	var last int64
// 	buf := make([]byte, bufferSize)
// 	for {
// 		readed, err = channel.Read(buf[:4])
// 		if err != nil {
// 			break
// 		}
// 		if readed < 4 {
// 			err = fmt.Errorf("head too small")
// 			break
// 		}
// 		length := binary.BigEndian.Uint64(buf)
// 		if length+4 > bufferSize {
// 			err = fmt.Errorf("frame too large")
// 			break
// 		}
// 		err = fullBuf(channel, buf[4:], length, &last)
// 		if err != nil {
// 			break
// 		}
// 		data := buf[:4+length]
// 		switch buf[4] {
// 		case CmdDial:
// 			err = s.procDail(channel, data)
// 		case CmdData:
// 			err = s.procRawData(channel, data)
// 		}
// 		if err != nil {
// 			break
// 		}
// 	}
// }

// func (s *Slaver) procDail(channel *Channel, data []byte) (err error) {
// 	conn := string(data[5:])
// 	router := strings.SplitN(conn, "@", 2)
// 	if len(router) < 2 {
// 		err = fmt.Errorf("invalid uri(%v)", conn)
// 		return
// 	}
// 	return
// }

// func (s *Slaver) procRawData(channel *Channel, data []byte) (err error) {
// 	return
// }

// func (s *Slaver) Dail(uri string) (raw io.ReadWriteCloser, err error) {

// 	return
// }
