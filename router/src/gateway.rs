use std::{collections::HashMap, io::ErrorKind, sync::Arc, usize};

use async_trait::async_trait;
use smoltcp::{
    iface::{Interface, SocketHandle, SocketSet},
    phy::{Device, DeviceCapabilities, Medium, RxToken, TxToken},
    socket::Socket,
    time::{Duration, Instant},
    wire::{IpAddress, IpEndpoint, IpListenEndpoint, Ipv4Address},
};
use tokio::sync::{
    broadcast,
    mpsc::{self, Receiver, Sender},
    Mutex,
};

use crate::{frame, router::Router, util::ConnSeq};

struct UdpGwFlag {
    v: u8,
}

impl UdpGwFlag {
    pub fn new(v: u8) -> Self {
        Self { v }
    }

    // pub fn is_keep_alive(&self) -> bool {
    //     self.v & (1 << 0) == 1 << 0
    // }

    // pub fn is_dns(&self) -> bool {
    //     self.v & (1 << 2) == 1 << 2
    // }

    pub fn is_ipv6(&self) -> bool {
        self.v & (1 << 3) == 1 << 3
    }

    // pub fn mark_keep_alive(&mut self) {
    //     self.v = self.v | 1 << 0
    // }

    pub fn mark_dns(&mut self) {
        self.v = self.v | 1 << 2
    }

    pub fn mark_ipv6(&mut self) {
        self.v = self.v | 1 << 3
    }
}

impl Into<u8> for UdpGwFlag {
    fn into(self) -> u8 {
        self.v
    }
}

#[derive(Clone, PartialEq, Eq, Hash)]
struct UdpGwEndpoint {
    local: IpListenEndpoint,
    remote: IpEndpoint,
}

impl UdpGwEndpoint {
    pub fn new(local: IpListenEndpoint, remote: IpEndpoint) -> Self {
        Self { local, remote }
    }
}

struct UdpGw {
    ep_all: HashMap<u16, UdpGwEndpoint>,
    id_all: HashMap<UdpGwEndpoint, u16>,
    handle_all: HashMap<u16, SocketHandle>,
    id_seq: u16,
}

impl UdpGw {
    pub fn new() -> Self {
        Self { ep_all: HashMap::new(), id_all: HashMap::new(), handle_all: HashMap::new(), id_seq: 0 }
    }

    pub fn new_cid(&mut self) -> u16 {
        self.id_seq = (self.id_seq as u32 + 1) as u16;
        self.id_seq
    }

    pub fn parse_frame<'a>(&mut self, buf: &'a [u8]) -> Option<(UdpGwFlag, &SocketHandle, &UdpGwEndpoint, &'a [u8])> {
        let flag = UdpGwFlag::new(buf[2]);
        let id = u16::from_be_bytes([buf[3], buf[4]]);
        let handle = self.handle_all.get(&id)?;
        let ep = self.ep_all.get(&id)?;
        if flag.is_ipv6() {
            Some((flag, handle, ep, &buf[23..]))
        } else {
            Some((flag, handle, ep, &buf[11..]))
        }
    }

    pub fn create_frame(&mut self, handle: &SocketHandle, local: &IpListenEndpoint, remote: &IpEndpoint, data: &[u8]) -> Vec<u8> {
        let ep = UdpGwEndpoint::new(local.clone(), remote.clone());
        let id = match self.id_all.get(&ep) {
            Some(v) => *v,
            None => {
                let new_id = self.new_cid();
                self.ep_all.insert(new_id, ep.clone());
                self.id_all.insert(ep, new_id);
                self.handle_all.insert(new_id, handle.clone());
                new_id
            }
        };
        let mut flag = UdpGwFlag::new(0);
        if local.port == 53 {
            flag.mark_dns();
        }
        let addr = match &local.addr {
            Some(addr) => match addr {
                IpAddress::Ipv4(addr) => addr.as_bytes(),
                IpAddress::Ipv6(addr) => {
                    flag.mark_ipv6();
                    addr.as_bytes()
                }
            },
            None => &[0, 0, 0, 0],
        };
        let addr_len = addr.len();
        let frame_len = (5 + addr_len + data.len()) as u16;
        let mut buf = vec![0u8; (frame_len + 2) as usize];
        buf[0..2].copy_from_slice(&frame_len.to_le_bytes() as &[u8]);
        buf[2..3].copy_from_slice(&[flag.into()]);
        buf[3..5].copy_from_slice(&id.to_be_bytes() as &[u8]);
        buf[5..5 + addr_len].copy_from_slice(addr);
        buf[5 + addr_len..7 + addr_len].copy_from_slice(&local.port.to_be_bytes() as &[u8]);
        buf[7 + addr_len..].copy_from_slice(&data);
        buf
    }
}

#[derive(Clone, PartialEq, Eq, Hash)]
pub enum ConnProto {
    TCP,
    UDPGW,
}

#[derive(Clone, PartialEq, Eq, Hash)]
pub struct ConnHandle {
    pub proto: ConnProto,
    pub handle: SocketHandle,
    pub local: IpEndpoint,
    pub remote: IpEndpoint,
}

impl ConnHandle {
    pub fn new(proto: ConnProto, handle: SocketHandle, local: IpEndpoint, remote: IpEndpoint) -> Self {
        Self { proto, handle, local, remote }
    }

    pub fn udpgw() -> Self {
        let local = IpEndpoint::new(IpAddress::Ipv4(Ipv4Address::new(0, 0, 0, 0)), 0);
        let remote = IpEndpoint::new(IpAddress::Ipv4(Ipv4Address::new(0, 0, 0, 0)), 0);
        Self { proto: ConnProto::UDPGW, handle: SocketHandle::default(), local: local, remote: remote }
    }
}

#[derive(Clone)]
pub struct ConnMeta {
    pub id: u16,
    pub handle: SocketHandle,
    pub proto: ConnProto,
    pub local: IpEndpoint,
    pub remote: IpEndpoint,
}

impl ConnMeta {
    pub fn new(id: u16, handle: SocketHandle, proto: ConnProto, local: IpEndpoint, remote: IpEndpoint) -> Self {
        Self { id, handle, proto, local, remote }
    }

    pub fn udpgw() -> Self {
        let local = IpEndpoint::new(IpAddress::v4(0, 0, 0, 0), 0);
        let remote = IpEndpoint::new(IpAddress::v4(0, 0, 0, 0), 0);
        ConnMeta { id: 0, handle: SocketHandle::default(), proto: ConnProto::UDPGW, local, remote }
    }

    pub fn get_handle(&self) -> ConnHandle {
        ConnHandle::new(self.proto.clone(), self.handle.clone(), self.local.clone(), self.remote.clone())
    }

    pub fn dial_uri(&self) -> String {
        match self.proto {
            ConnProto::TCP => format!("tcp://{}", self.local),
            ConnProto::UDPGW => format!("tcp://udpgw"),
        }
    }

    pub fn to_string(&self) -> String {
        match self.proto {
            ConnProto::TCP => format!("TCP({},{}<=>{})", self.id, self.local, self.remote),
            ConnProto::UDPGW => format!("UDPGW({})", self.id),
        }
    }
}

pub struct ConnReader {
    pub meta: Arc<ConnMeta>,
    pub inner: Receiver<Vec<u8>>,
}

impl ConnReader {
    pub fn new(meta: Arc<ConnMeta>, inner: Receiver<Vec<u8>>) -> Self {
        Self { meta, inner }
    }

    pub fn get_handle(&self) -> ConnHandle {
        self.meta.get_handle()
    }
}

#[async_trait]
impl frame::RawReader for ConnReader {
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        match self.inner.recv().await {
            Some(data) => {
                let n = data.len();
                if n < 1 {
                    Err(std::io::Error::new(ErrorKind::UnexpectedEof, "EOF"))
                } else {
                    buf[..n].copy_from_slice(&data);
                    Ok(n)
                }
            }
            None => Err(std::io::Error::new(ErrorKind::Other, "closed")),
        }
    }
}

pub struct ConnWriter {
    pub meta: Arc<ConnMeta>,
    pub signal: Sender<u8>,
    pub inner: Sender<Vec<u8>>,
}

impl ConnWriter {
    pub fn new(meta: Arc<ConnMeta>, signal: Sender<u8>, inner: Sender<Vec<u8>>) -> Self {
        Self { meta, signal, inner }
    }

    pub fn get_handle(&self) -> ConnHandle {
        self.meta.get_handle()
    }
}

#[async_trait]
impl frame::RawWriter for ConnWriter {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        match self.inner.send(Vec::from(buf)).await {
            Ok(_) => {
                _ = self.signal.try_send(1);
                Ok(buf.len())
            }
            Err(e) => Err(std::io::Error::new(ErrorKind::Other, e)),
        }
    }
    async fn shutdown(&mut self) {
        _ = self.inner.send(Vec::from([])).await;
    }
}

pub struct Conn {
    pub meta: Arc<ConnMeta>,
    pub reader: ConnReader,
    pub writer: ConnWriter,
}

impl Conn {
    pub fn new(meta: Arc<ConnMeta>, reader: ConnReader, writer: ConnWriter) -> Self {
        Self { meta, reader, writer }
    }

    pub fn pipe(meta: Arc<ConnMeta>, signal: Sender<u8>, buffer: usize) -> (Conn, Conn) {
        let (txa, rxb) = mpsc::channel::<Vec<u8>>(buffer);
        let (txb, rxa) = mpsc::channel::<Vec<u8>>(buffer);
        let ra = ConnReader::new(meta.clone(), rxa);
        let rb = ConnReader::new(meta.clone(), rxb);
        let wa = ConnWriter::new(meta.clone(), signal.clone(), txa);
        let wb = ConnWriter::new(meta.clone(), signal, txb);
        let a = Self::new(meta.clone(), ra, wa);
        let b = Self::new(meta.clone(), rb, wb);
        (a, b)
    }

    pub fn get_handle(&self) -> ConnHandle {
        self.meta.get_handle()
    }

    pub fn dial_uri(&self) -> String {
        self.meta.dial_uri()
    }

    pub fn split(self) -> (ConnReader, ConnWriter) {
        (self.reader, self.writer)
    }

    pub fn to_string(&self) -> String {
        self.meta.to_string()
    }
}

struct GatewayInner {
    mtu: usize,
    conn_set: SocketSet<'static>,
    conn_all: HashMap<SocketHandle, Conn>,
    conn_seq: ConnSeq,
    udpgw_all: UdpGw,
    udpgw_conn: Option<Conn>,
    pub name: Arc<String>,
    pub iface: Interface,
    pub signal: Sender<u8>,
}

impl GatewayInner {
    pub fn new(name: Arc<String>, iface: Interface, signal: Sender<u8>) -> Self {
        let conn_set = SocketSet::new(vec![]);
        let conn_all = HashMap::new();
        let udpgw_all = UdpGw::new();
        Self { mtu: 2000, conn_set, conn_all, conn_seq: ConnSeq::new(), udpgw_all, udpgw_conn: None, name, iface, signal }
    }

    pub fn delay(&mut self) -> Option<Duration> {
        self.iface.poll_delay(Instant::now(), &self.conn_set)
    }

    pub fn poll<D>(&mut self, device: &mut D) -> Vec<Conn>
    where
        D: Device + ?Sized,
    {
        let s = &mut *self;
        let iface = &mut s.iface;
        let conn_set = &mut s.conn_set;
        let conn_all = &mut s.conn_all;
        let conn_seq = &mut s.conn_seq;
        let udpgw = &mut s.udpgw_all;
        let signal = &mut s.signal;
        let mut new_conn_h = Vec::new();
        let mut new_udpgw = false;
        let mut close_conn_h = Vec::new();
        iface.poll(Instant::now(), device, conn_set);
        for (h, v) in conn_set.iter_mut() {
            match v {
                Socket::Udp(v) => match &mut s.udpgw_conn {
                    Some(c) => {
                        let local = v.endpoint();
                        if v.can_recv() {
                            match v.recv() {
                                Ok((data, ep)) => {
                                    let buf = udpgw.create_frame(&h, &local, &ep, &data);
                                    _ = c.writer.inner.try_send(buf);
                                }
                                Err(_) => {
                                    v.close();
                                    close_conn_h.push(h)
                                }
                            }
                        }
                        if v.can_send() {
                            match c.reader.inner.try_recv() {
                                Ok(buf) => match udpgw.parse_frame(&buf) {
                                    Some((_, _, ep, data)) => {
                                        _ = v.send_slice(&data, ep.remote);
                                    }
                                    None => (), //drop
                                },
                                Err(e) => match e {
                                    mpsc::error::TryRecvError::Empty => (),
                                    mpsc::error::TryRecvError::Disconnected => {
                                        v.close();
                                        close_conn_h.push(h);
                                    }
                                },
                            }
                        }
                    }
                    None => {
                        new_udpgw = true;
                    }
                },
                Socket::Tcp(v) => match conn_all.get_mut(&h) {
                    Some(c) => {
                        if v.can_recv() {
                            match v.recv(|data| {
                                let mut n = data.len();
                                if n > s.mtu {
                                    n = s.mtu;
                                }
                                let buf = Vec::from(&data[0..n]);
                                match c.writer.inner.try_send(buf) {
                                    Ok(_) => (n, 0),
                                    Err(e) => match e {
                                        mpsc::error::TrySendError::Full(_) => (0, 0),
                                        mpsc::error::TrySendError::Closed(_) => (0, -1),
                                    },
                                }
                            }) {
                                Ok(code) => {
                                    if code == 0 {
                                        _ = signal.try_send(1);
                                    } else {
                                        v.close();
                                    }
                                }
                                Err(_) => {
                                    v.close();
                                }
                            }
                        }
                        if v.can_send() {
                            match v.send(|data| {
                                if data.len() < s.mtu {
                                    return (0, 0);
                                }
                                match c.reader.inner.try_recv() {
                                    Ok(buf) => {
                                        let n = buf.len();
                                        data[0..n].copy_from_slice(&buf);
                                        (n, 0)
                                    }
                                    Err(e) => match e {
                                        mpsc::error::TryRecvError::Empty => (0, 0),
                                        mpsc::error::TryRecvError::Disconnected => (0, -1),
                                    },
                                }
                            }) {
                                Ok(code) => {
                                    if code == 0 {
                                        _ = signal.try_send(1);
                                    } else {
                                        v.close();
                                    }
                                }
                                Err(_) => {
                                    v.close();
                                }
                            }
                        }
                        if !v.is_open() {
                            _ = c.writer.inner.try_send(Vec::new());
                            close_conn_h.push(h);
                        }
                    }
                    None => {
                        if v.is_open() {
                            new_conn_h.push(h);
                        } else {
                            close_conn_h.push(h);
                        }
                    }
                },
                _ => (),
            }
        }
        for ch in close_conn_h {
            match conn_all.remove(&ch) {
                Some(conn) => log::info!("Gateway({}) {} conn is closed", s.name, conn.to_string()),
                None => (),
            };
            conn_set.remove(ch);
        }
        let mut new_conn = Vec::new();
        if new_udpgw {
            let meta = ConnMeta::udpgw();
            let (a, b) = Conn::pipe(Arc::new(meta), signal.clone(), 256);
            log::info!("Gateway({}) {} conn is starting", s.name, a.to_string());
            s.udpgw_conn = Some(a);
            new_conn.push(b);
            _ = signal.try_send(1);
        }
        let mut add_tcp_conn = |ch| {
            let c = conn_set.find_mut(&ch)?;
            if let Socket::Tcp(c) = c {
                let remote = c.remote_endpoint()?;
                let local = c.local_endpoint()?;
                let meta = ConnMeta::new(conn_seq.new_cid(), ch.clone(), ConnProto::TCP, local, remote);
                let (a, b) = Conn::pipe(Arc::new(meta), signal.clone(), 8);
                log::info!("Gateway({}) {} conn is starting", s.name, a.to_string());
                conn_all.insert(ch, a);
                new_conn.push(b);
            }
            Some(1)
        };
        for ch in new_conn_h {
            add_tcp_conn(ch);
        }
        new_conn
    }
}

pub struct CachePacket {
    pub rx_all: Vec<Vec<u8>>,
    pub tx_all: Vec<Vec<u8>>,
}

impl CachePacket {
    pub fn new() -> Self {
        Self { rx_all: Vec::new(), tx_all: Vec::new() }
    }
}

pub struct CacheDevice {
    cache: CachePacket,
    mtu: usize,
}

impl CacheDevice {
    pub fn new(mtu: usize) -> CacheDevice {
        CacheDevice { cache: CachePacket::new(), mtu }
    }

    pub fn rx_size(&self) -> usize {
        self.cache.rx_all.len()
    }

    pub fn tx_size(&self) -> usize {
        self.cache.tx_all.len()
    }

    pub async fn read<R>(&mut self, reader: &mut R) -> tokio::io::Result<usize>
    where
        R: frame::RawReader + Send + Sync,
    {
        let mut packet = vec![0u8; self.mtu];
        let n = reader.read(&mut packet).await?;
        self.cache.rx_all.push(packet[0..n].to_vec());
        Ok(1)
    }

    pub async fn write<W>(&mut self, writer: &mut W) -> tokio::io::Result<()>
    where
        W: frame::RawWriter + Send + Sync,
    {
        let cache = &mut self.cache;
        while cache.tx_all.len() > 0 {
            let packet = cache.tx_all.remove(0);
            writer.write(&packet.to_vec()).await?;
        }
        Ok(())
    }
}

impl Device for CacheDevice {
    type RxToken<'a> = CacheRxToken where Self: 'a;
    type TxToken<'a> = CacheTxToken<'a> where Self: 'a;

    fn receive(&mut self, _timestamp: Instant) -> Option<(Self::RxToken<'_>, Self::TxToken<'_>)> {
        if self.cache.rx_all.is_empty() {
            None
        } else {
            let packet = self.cache.rx_all.remove(0);
            Some((CacheRxToken::new(packet), CacheTxToken::new(&mut self.cache)))
        }
    }

    fn transmit(&mut self, _timestamp: Instant) -> Option<Self::TxToken<'_>> {
        Some(CacheTxToken::new(&mut self.cache))
    }

    fn capabilities(&self) -> DeviceCapabilities {
        let mut caps = DeviceCapabilities::default();
        caps.max_transmission_unit = 1536;
        caps.max_burst_size = Some(1);
        caps.medium = Medium::Ip;
        caps
    }
}

pub struct CacheRxToken {
    cache: Vec<u8>,
}

impl CacheRxToken {
    pub fn new(cache: Vec<u8>) -> Self {
        Self { cache }
    }
}

impl RxToken for CacheRxToken {
    fn consume<R, F>(self, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        f(&mut self.cache.to_vec())
    }
}

pub struct CacheTxToken<'a> {
    cache: &'a mut CachePacket,
}

impl<'a> CacheTxToken<'a> {
    fn new(cache: &'a mut CachePacket) -> Self {
        Self { cache }
    }
}

impl<'a> TxToken for CacheTxToken<'a> {
    fn consume<R, F>(self, len: usize, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        let mut packet = vec![0u8; len];
        let result = f(&mut packet);
        self.cache.tx_all.push(packet);
        result
    }
}

struct GatewayRunner<R, W>
where
    R: frame::RawReader + Send + Sync,
    W: frame::RawWriter + Send + Sync,
{
    name: Arc<String>,
    inner: Arc<Mutex<GatewayInner>>,
    router: Arc<Router>,
    remote: Arc<String>,
    mtu: usize,
    signal: Receiver<u8>,
    stopper: broadcast::Receiver<u8>,
    reader: R,
    writer: W,
}

impl<R, W> GatewayRunner<R, W>
where
    R: frame::RawReader + Send + Sync,
    W: frame::RawWriter + Send + Sync,
{
    pub fn new(name: Arc<String>, iface: Interface, router: Arc<Router>, remote: Arc<String>, mtu: usize, stopper: broadcast::Receiver<u8>, reader: R, writer: W) -> Self {
        let (send, recv) = mpsc::channel(8);
        let inner = Arc::new(Mutex::new(GatewayInner::new(name.clone(), iface, send)));
        Self { name, inner, router, remote, mtu, signal: recv, stopper, reader, writer }
    }

    pub async fn run(&mut self) -> tokio::io::Result<()> {
        let mut delay: Option<Duration> = None;
        let mut device = CacheDevice::new(self.mtu);
        let mut gw = self.inner.lock().await;
        loop {
            match delay.take() {
                Some(delay) => tokio::select! {
                    v = device.read(&mut self.reader) => v,
                    _ = self.signal.recv() => Ok(0),
                    _ = tokio::time::sleep(tokio::time::Duration::from_micros(delay.micros())) => Ok(0),
                    _ = self.stopper.recv() => Ok(0),
                },
                None => tokio::select! {
                    v = device.read(&mut self.reader) => v,
                    _ = self.signal.recv() => Ok(0),
                    _ = self.stopper.recv() => Ok(0),
                },
            }?;
            loop {
                let conn_all = gw.poll(&mut device);
                for conn in conn_all {
                    log::info!("Gateway({}) start forward conn {} to {}", self.name, conn.to_string(), self.remote);
                    let dial_uri = self.remote.replace("${HOST}", &conn.dial_uri());
                    let (conn_reader, conn_writer) = conn.split();
                    _ = self.router.dial_base(Box::new(conn_reader), Box::new(conn_writer), Arc::new(dial_uri)).await;
                }
                device.write(&mut self.writer).await?;

                if device.rx_size() < 1 {
                    delay = gw.delay();
                    break;
                }
            }
        }
    }
}

#[derive(Clone)]
pub struct Gateway {
    pub router: Arc<Router>,
    pub mtu: usize,
    pub stopper: broadcast::Sender<u8>,
    pub waiter: wg::AsyncWaitGroup,
}

impl Gateway {
    pub fn new(router: Arc<Router>) -> Self {
        let (stopper, _) = broadcast::channel(8);
        let waiter = wg::AsyncWaitGroup::new();
        Self { router, mtu: 1600, stopper, waiter }
    }

    pub async fn start<R, W>(&mut self, name: Arc<String>, iface: Interface, reader: R, writer: W, remote: Arc<String>)
    where
        R: frame::RawReader + Send + Sync + 'static,
        W: frame::RawWriter + Send + Sync + 'static,
    {
        let router = self.router.clone();
        let mtu = self.mtu;
        let stopper = self.stopper.subscribe();
        let waiter = self.waiter.clone();
        waiter.add(1);
        tokio::spawn(async move {
            log::info!("Gateway({}) gateway is starting by remote {}", name, remote);
            let mut runner = GatewayRunner::new(name.clone(), iface, router, remote, mtu, stopper, reader, writer);
            let res = runner.run().await;
            waiter.done();
            log::info!("Gateway({}) gateway is stopped by {:?}", name, res);
        });
    }

    pub async fn stop(&self) {
        _ = self.stopper.send(1);
    }

    pub async fn wait(&self) {
        self.waiter.wait().await;
    }
}
