extern crate smoltcp;
extern crate tun;

use std::{cell::RefCell, collections::HashMap, rc::Rc, sync::Arc};

use log::{info, warn};
use smoltcp::{
    iface::{Interface, SocketHandle, SocketSet},
    phy::{Device, DeviceCapabilities, Medium},
    socket::udp::{self},
    socket::{tcp, Socket},
    time::{Duration, Instant},
    wire::IpEndpoint,
};
use tokio::{
    io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt},
    sync::mpsc::{self, Receiver, Sender},
};

use crate::{
    frame,
    util::{new_message_err, now, Sequence},
};

#[cfg(target_os = "linux")]
const IFF_PI_PREFIX_LEN: usize = 0;
#[cfg(any(target_os = "macos", target_os = "freebsd"))]
const IFF_PI_PREFIX_LEN: usize = 4;
#[cfg(target_os = "macos")]
const IPV4_PACKET_SIGNATURE: [u8; 4] = [000, 000, 000, 002];
pub const DEFAULT_MTU: usize = 1500;

pub struct Conn {
    pub cid: u16,
    pub source: Arc<String>,
    pub target: Arc<String>,
    pub reader: frame::RecvReader,
    pub writer: frame::SendWriter,
}

impl Conn {
    pub fn new(cid: u16, source: Arc<String>, target: Arc<String>, recv: Receiver<Vec<u8>>, send: Sender<Vec<u8>>, signal: Sender<u16>) -> Self {
        let reader = frame::RecvReader::new(recv);
        let writer = frame::SendWriter::new(send, signal);
        Self { cid, source, target, reader, writer }
    }
}

type UdpGateway = Conn;

pub struct TcpGateway {
    recv: Receiver<Conn>,
}

impl TcpGateway {
    pub fn new(recv: Receiver<Conn>) -> Self {
        Self { recv }
    }

    pub async fn accept(&mut self) -> Option<Conn> {
        self.recv.recv().await
    }
}

struct TcpInfo {
    pub sh: SocketHandle,
    pub send: Sender<Vec<u8>>,
    pub recv: Receiver<Vec<u8>>,
}

pub struct RelayInner {
    pub name: Arc<String>,
    mtu: usize,
    conn_all: SocketSet<'static>,
    conn_seq: Sequence,
    tcp_id: HashMap<SocketHandle, u16>,
    tcp_all: HashMap<u16, TcpInfo>,
    udp_ep: HashMap<u16, IpEndpoint>,
    udp_id: HashMap<IpEndpoint, u16>,
    udp_sh: HashMap<u16, SocketHandle>,
    udp_send: Sender<Vec<u8>>,
    udp_recv: Receiver<Vec<u8>>,
    sig_send: Sender<u16>,
    last_all: HashMap<u16, i64>,
}

impl RelayInner {
    pub fn new(name: Arc<String>, mtu: usize) -> (Self, Receiver<u16>, UdpGateway) {
        let (txa, rxb) = mpsc::channel(64);
        let (txb, rxa) = mpsc::channel(64);
        let mut conn_seq = Sequence::new();
        let target = Arc::new(format!("tcp://udpgw"));
        let (sig_send, sig_recv) = mpsc::channel(4096);
        let udp_gw = UdpGateway::new(conn_seq.next(), Arc::new(String::new()), target, rxa, txa, sig_send.clone());
        let s = Self {
            name,
            mtu,
            conn_all: SocketSet::new(vec![]),
            conn_seq: Sequence::new(),
            tcp_id: HashMap::new(),
            tcp_all: HashMap::new(),
            udp_ep: HashMap::new(),
            udp_id: HashMap::new(),
            udp_sh: HashMap::new(),
            udp_send: txb,
            udp_recv: rxb,
            sig_send,
            last_all: HashMap::new(),
        };
        (s, sig_recv, udp_gw)
    }

    fn list_conn_all(&mut self) -> (Vec<SocketHandle>, Vec<SocketHandle>) {
        let mut tcp_all = Vec::new();
        let mut udp_all = Vec::new();
        for (h, v) in self.conn_all.iter_mut() {
            match v {
                Socket::Udp(_) => udp_all.push(h),
                Socket::Tcp(_) => tcp_all.push(h),
                _ => (),
            }
        }
        (tcp_all, udp_all)
    }

    pub fn poll<D>(&mut self, iface: &mut Interface, device: &mut D) -> Vec<Conn>
    where
        D: Device + ?Sized,
    {
        // self.poll_tcp_send();
        // self.poll_udp_send();
        iface.poll(Instant::now(), device, &mut self.conn_all);
        for (h, v) in self.conn_all.iter_mut() {
            match v {
                Socket::Udp(v) => {
                    if v.can_recv() {
                        let (data, ep) = v.recv().unwrap();
                        let data = data.to_owned();
                        if v.can_send() && !data.is_empty() {
                            v.send_slice(&data[..], ep).unwrap();
                        }
                    }
                }
                Socket::Tcp(v) => {
                    if v.may_recv() {
                        let data = v
                            .recv(|buffer| {
                                let recvd_len = buffer.len();
                                let data = buffer.to_owned();
                                (recvd_len, data)
                            })
                            .unwrap();
                        if v.can_send() && !data.is_empty() {
                            v.send_slice(&data[..]).unwrap();
                        }
                    }
                }
                _ => (),
            }
        }
        // let (tcp_all, udp_all) = self.list_conn_all();
        // let mut new_all = Vec::new();
        // for sh in tcp_all {
        //     match self.poll_tcp_recv(sh) {
        //         Some(c) => new_all.push(c),
        //         None => (),
        //     }
        // }
        // for sh in udp_all {
        //     self.poll_udp_recv(sh);
        // }
        // new_all
        Vec::new()
    }

    pub fn poll_delay(&mut self, iface: &mut Interface) -> Option<Duration> {
        iface.poll_delay(Instant::now(), &mut self.conn_all)
    }

    fn poll_udp_recv(&mut self, sh: SocketHandle) {
        let conn: &mut udp::Socket = match self.conn_all.find_mut(sh) {
            Some(v) => v,
            None => return,
        };
        if !conn.can_recv() {
            return;
        }
        let local = conn.endpoint();
        if local.addr.is_none() {
            conn.close();
            return;
        }
        let dst_addr = local.addr.unwrap();
        let (playload, ep) = conn.recv().unwrap();
        let cid = match self.udp_id.get(&ep) {
            Some(v) => v.clone(),
            None => {
                let cid = self.conn_seq.next();
                self.udp_id.insert(ep, cid);
                self.udp_ep.insert(cid, ep);
                self.udp_sh.insert(cid, sh.clone());
                info!("Relay({}) accept new udp {} connect {}=>{}", self.name, cid, ep, local);
                cid
            }
        };
        self.last_all.insert(cid, now());
        let dst_bytes = dst_addr.as_bytes();
        let n = dst_bytes.len();
        let mut data = vec![0u8; 5 + n + playload.len()];
        data[0..2].copy_from_slice(&cid.to_be_bytes() as &[u8]);
        data[2] = n as u8;
        data[3..3 + n].copy_from_slice(dst_addr.as_bytes());
        data[3 + n..5 + n].copy_from_slice(&local.port.to_be_bytes() as &[u8]);
        data[5 + n..].copy_from_slice(playload);
        match self.udp_send.try_send(data) {
            Ok(_) => (),
            Err(e) => match e {
                mpsc::error::TrySendError::Full(_) => {
                    warn!("Relay({}) udp gw recv is dropped by {}", self.name, e);
                }
                mpsc::error::TrySendError::Closed(_) => conn.close(),
            },
        }
    }

    fn poll_udp_send(&mut self) {
        let data = match self.udp_recv.try_recv() {
            Ok(v) => v,
            Err(_) => return,
        };
        println!("abcc-->{}", data.len());
        if data.len() < 1 {
            return;
        }
        let cid = u16::from_be_bytes([data[0], data[1]]);
        let n = data[2] as usize;
        println!("--->{},{:?}", n, data);
        let ep = match self.udp_ep.get(&cid) {
            Some(v) => v,
            None => {
                warn!("Relay({}) udp gw send is dropped by not connected", self.name);
                return;
            }
        };
        let sh = match self.udp_sh.get(&cid) {
            Some(v) => v,
            None => {
                warn!("Relay({}) udp gw send is dropped by not connected", self.name);
                return;
            }
        };
        let conn: &mut udp::Socket = match self.conn_all.find_mut(*sh) {
            Some(v) => v,
            None => {
                warn!("Relay({}) udp gw send is dropped by not connected", self.name);
                return;
            }
        };
        if !conn.can_send() {
            warn!("Relay({}) udp gw send is dropped by not sendable", self.name);
            return;
        }
        let buf = match conn.send(data.len() - n - 5, *ep) {
            Ok(v) => v,
            Err(e) => {
                warn!("Relay({}) udp gw send is dropped by {}", self.name, e);
                return;
            }
        };
        buf.copy_from_slice(&data[5 + n..]);
        self.last_all.insert(cid, now());
    }

    fn poll_tcp_recv(&mut self, sh: SocketHandle) -> Option<Conn> {
        let conn: &mut tcp::Socket = match self.conn_all.find_mut(sh) {
            Some(v) => v,
            None => return None,
        };
        let source = match conn.remote_endpoint() {
            Some(v) => v,
            None => return None,
        };
        let target = match conn.local_endpoint() {
            Some(v) => v,
            None => return None,
        };
        let (cid, new_conn) = match self.tcp_id.get(&sh) {
            Some(v) => (v.clone(), None),
            None => {
                let cid = self.conn_seq.next();
                let (txa, rxb) = mpsc::channel(4);
                let (txb, rxa) = mpsc::channel(4);
                let info = TcpInfo { sh: sh.clone(), send: txa, recv: rxa };
                self.tcp_id.insert(sh.clone(), cid);
                self.tcp_all.insert(cid, info);
                info!("Relay({}) accept new tcp {} connect {}=>{}", self.name, cid, source, target);
                let src = format!("{}", source);
                let uri = format!("tcp://{}", target);
                let conn = Conn::new(cid.clone(), Arc::new(src), Arc::new(uri), rxb, txb, self.sig_send.clone());
                (cid, Some(conn))
            }
        };
        let info = match self.tcp_all.get_mut(&cid) {
            Some(v) => v,
            None => {
                conn.close();
                return None;
            }
        };
        if !conn.can_recv() {
            return new_conn;
        }
        match conn.recv(|buffer| {
            let mut n = buffer.len();
            if n > self.mtu {
                n = self.mtu;
            }
            let data = buffer[0..n].to_owned();
            match info.send.try_send(data) {
                Ok(_) => {
                    self.last_all.insert(cid, now());
                    (n, None)
                }
                Err(e) => match e {
                    mpsc::error::TrySendError::Full(_) => (0, None),
                    mpsc::error::TrySendError::Closed(_) => (0, Some(new_message_err("closed"))),
                },
            }
        }) {
            Ok(e) => match e {
                Some(_) => conn.close(),
                None => (),
            },
            Err(_) => {
                info.recv.close();
                _ = info.send.try_send(Vec::new());
            }
        }
        new_conn
    }

    fn poll_tcp_send(&mut self) {
        for (cid, info) in &mut self.tcp_all {
            let conn: &mut tcp::Socket = match self.conn_all.find_mut(info.sh) {
                Some(v) => v,
                None => continue,
            };
            if !conn.can_send() {
                return;
            }
            let free = conn.send_capacity() - conn.send_queue();
            if free < self.mtu {
                _ = self.sig_send.try_send(1);
                return;
            }
            let data = match info.recv.try_recv() {
                Ok(data) => {
                    if data.is_empty() {
                        info.recv.close();
                        conn.close();
                        continue;
                    }
                    data
                }
                Err(e) => match e {
                    mpsc::error::TryRecvError::Empty => continue,
                    mpsc::error::TryRecvError::Disconnected => {
                        conn.close();
                        continue;
                    }
                },
            };
            match conn.send_slice(&data) {
                Ok(n) => {
                    if n != data.len() {
                        println!("send_slice a---->{},{}", n, data.len());
                        info.recv.close();
                        conn.close();
                    } else {
                        self.last_all.insert(cid.clone(), now());
                    }
                }
                Err(e) => {
                    println!("send_slice b---->{}", e);
                    info.recv.close();
                    conn.close();
                }
            }
        }
    }
}

pub struct Relay {
    relay: RelayInner,
    iface: Interface,
    device: CacheDevice,
    sender: Sender<Conn>,
    signal: Receiver<u16>,
}

impl Relay {
    pub fn new(name: Arc<String>, iface: Interface, device: CacheDevice) -> (Relay, TcpGateway, UdpGateway) {
        let (relay, signal, udg_gw) = RelayInner::new(name, 1500);
        let (sender, tcp_gw) = mpsc::channel(8);
        (Self { relay, iface, device, sender, signal }, TcpGateway::new(tcp_gw), udg_gw)
    }

    pub async fn poll<T>(&mut self, dev: &mut T) -> tokio::io::Result<()>
    where
        T: AsyncRead + AsyncWrite + Unpin,
    {
        let signal = &mut self.signal;
        let device = &mut self.device;
        tokio::select! {
            _= signal.recv()=>{
                // println!("recv--->")
            },
            _= device.read_from(dev)=>{
                // println!("read--->")
            },
        }
        let new_all = self.relay.poll(&mut self.iface, device);
        for c in new_all {
            match self.sender.send(c).await {
                Ok(_) => (),
                Err(e) => return Err(new_message_err(e.to_string())),
            }
        }
        device.write_to(dev).await.unwrap();
        Ok(())
    }

    pub fn poll_delay(&mut self) -> Option<Duration> {
        self.relay.poll_delay(&mut self.iface)
    }
}

pub struct CacheDevice {
    mtu: usize,
    medium: Medium,
    buffer: Vec<u8>,
    ingress: Vec<Vec<u8>>,
    egress: Rc<RefCell<Vec<Vec<u8>>>>,
}

impl CacheDevice {
    pub fn new(mtu: usize, medium: Medium) -> CacheDevice {
        CacheDevice {
            mtu,
            medium,
            buffer: vec![0u8; 2048],
            ingress: Vec::new(),
            egress: Rc::new(RefCell::new(Vec::new())),
        }
    }

    pub async fn read_from<T>(&mut self, r: &mut T) -> tokio::io::Result<()>
    where
        T: AsyncRead + Unpin,
    {
        let n = r.read(&mut self.buffer).await?;
        self.ingress.push(self.buffer[IFF_PI_PREFIX_LEN..n].to_vec());
        Ok(())
    }

    pub async fn write_to<T>(&mut self, w: &mut T) -> tokio::io::Result<()>
    where
        T: AsyncWrite + Unpin,
    {
        let mut egress = self.egress.borrow_mut();
        while egress.len() > 0 {
            let data = egress.remove(0);
            w.write_all(&data).await?;
        }
        Ok(())
    }
}

impl Device for CacheDevice {
    type RxToken<'a> = RxToken;
    type TxToken<'a> = TxToken;

    fn capabilities(&self) -> DeviceCapabilities {
        let mut caps = DeviceCapabilities::default();
        caps.max_transmission_unit = self.mtu;
        caps.medium = self.medium;
        caps
    }

    fn receive(&mut self, _timestamp: Instant) -> Option<(Self::RxToken<'_>, Self::TxToken<'_>)> {
        if self.ingress.len() > 0 {
            let data = self.ingress.remove(0);
            let rx = RxToken { buffer: data };
            let tx = TxToken { egress: self.egress.clone() };
            Some((rx, tx))
        } else {
            None
        }
    }

    fn transmit(&mut self, _timestamp: Instant) -> Option<Self::TxToken<'_>> {
        Some(TxToken { egress: self.egress.clone() })
    }
}

pub struct RxToken {
    buffer: Vec<u8>,
}

impl smoltcp::phy::RxToken for RxToken {
    fn consume<R, F>(mut self, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        f(&mut self.buffer[..])
    }
}

pub struct TxToken {
    egress: Rc<RefCell<Vec<Vec<u8>>>>,
}

impl smoltcp::phy::TxToken for TxToken {
    fn consume<R, F>(self, len: usize, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        let mut buffer = vec![0; IFF_PI_PREFIX_LEN + len];
        #[cfg(target_os = "macos")]
        buffer[..IFF_PI_PREFIX_LEN].copy_from_slice(&IPV4_PACKET_SIGNATURE);
        let result = f(&mut buffer[IFF_PI_PREFIX_LEN..]);
        self.egress.borrow_mut().push(buffer);
        result
    }
}
