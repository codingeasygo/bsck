use std::{
    collections::HashMap,
    pin::Pin,
    sync::Arc,
    task::{Context, Poll, Waker},
    usize,
};

use futures::Future;
use smoltcp::{
    iface::{Interface, SocketHandle, SocketSet},
    phy::{Device, DeviceCapabilities, Medium, RxToken, TxToken},
    socket::Socket,
    time::{Duration, Instant},
    wire::{IpAddress, IpEndpoint, IpListenEndpoint, Ipv4Address},
};
use tokio::{
    io::{AsyncRead, AsyncWrite},
    sync::{
        broadcast,
        mpsc::{self, Receiver, Sender},
        Mutex,
    },
};

use crate::{frame, router::Router, util::new_message_err, wrapper::wrap_split};

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

struct UdpGwConn {
    ep_all: HashMap<u16, IpEndpoint>,
    id_all: HashMap<IpListenEndpoint, u16>,
    handle_all: HashMap<u16, SocketHandle>,
    id_seq: u16,
    waker: ConnWaker,
}

impl UdpGwConn {
    pub fn new() -> Self {
        Self { ep_all: HashMap::new(), id_all: HashMap::new(), handle_all: HashMap::new(), id_seq: 0, waker: ConnWaker::new() }
    }

    pub fn new_cid(&mut self) -> u16 {
        self.id_seq = (self.id_seq as u32 + 1) as u16;
        self.id_seq
    }

    pub fn load<'a>(&mut self, buf: &'a [u8]) -> Option<(UdpGwFlag, &SocketHandle, &IpEndpoint, &'a [u8])> {
        let flag = UdpGwFlag::new(buf[2]);
        let id = u16::from_be_bytes([buf[3], buf[4]]);
        let handle = self.handle_all.get(&id)?;
        let remote = self.ep_all.get(&id)?;
        if flag.is_ipv6() {
            Some((flag, handle, remote, &buf[11..]))
        } else {
            Some((flag, handle, remote, &buf[23..]))
        }
    }

    pub fn put(&mut self, buf: &mut tokio::io::ReadBuf<'_>, handle: &SocketHandle, local: &IpListenEndpoint, remote: &IpEndpoint, data: usize) {
        let id = match self.id_all.get(local) {
            Some(v) => *v,
            None => {
                let new_id = self.new_cid();
                self.ep_all.insert(new_id, remote.clone());
                self.id_all.insert(local.clone(), new_id);
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
        let frame_len = (5 + addr.len() + data) as u16;
        buf.put_slice(&frame_len.to_le_bytes() as &[u8]);
        buf.put_slice(&[flag.into()]);
        buf.put_slice(&id.to_be_bytes() as &[u8]);
        buf.put_slice(addr);
        buf.put_slice(&local.port.to_be_bytes() as &[u8]);
    }

    pub fn recv_wake(&mut self) {
        self.waker.recv_wake();
    }
    pub fn send_wake(&mut self) {
        self.waker.send_wake();
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
pub struct Conn {
    gw: Arc<Mutex<GatewayInner>>,
    pub handle: ConnHandle,
}

impl Conn {
    fn new(gw: Arc<Mutex<GatewayInner>>, handle: ConnHandle) -> Self {
        Self { gw, handle }
    }
}

impl AsyncRead for Conn {
    fn poll_read(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &mut tokio::io::ReadBuf<'_>) -> Poll<std::io::Result<()>> {
        let gw = self.gw.lock();
        match Box::pin(gw).as_mut().poll(cx) {
            Poll::Ready(mut gw) => gw.poll_read(&self.handle, cx, buf),
            Poll::Pending => Poll::Pending,
        }
    }
}

impl AsyncWrite for Conn {
    fn poll_write(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &[u8]) -> Poll<std::io::Result<usize>> {
        let gw = self.gw.lock();
        match Box::pin(gw).as_mut().poll(cx) {
            Poll::Ready(mut gw) => gw.poll_write(&self.handle, cx, buf),
            Poll::Pending => Poll::Pending,
        }
    }

    fn poll_flush(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        Poll::Ready(Ok(()))
    }

    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Result<(), std::io::Error>> {
        let gw = self.gw.lock();
        match Box::pin(gw).as_mut().poll(cx) {
            Poll::Ready(mut gw) => gw.poll_shutdown(&self.handle, cx),
            Poll::Pending => Poll::Pending,
        }
    }
}

struct ConnWaker {
    recv: Option<Waker>,
    send: Option<Waker>,
}

impl ConnWaker {
    pub fn new() -> Self {
        Self { recv: None, send: None }
    }
    pub fn recv_wake(&mut self) {
        match self.recv.take() {
            Some(w) => w.wake(),
            None => (),
        }
    }
    pub fn send_wake(&mut self) {
        match self.recv.take() {
            Some(w) => w.wake(),
            None => (),
        }
    }
}

struct ConnInfo {
    pub waker: ConnWaker,
}

impl ConnInfo {
    pub fn new(_: ConnHandle) -> Self {
        Self { waker: ConnWaker::new() }
    }

    pub fn recv_wake(&mut self) {
        self.waker.recv_wake();
    }
    pub fn send_wake(&mut self) {
        self.waker.send_wake();
    }
}

struct GatewayInner {
    conn_set: SocketSet<'static>,
    conn_all: HashMap<SocketHandle, ConnInfo>,
    udpgw: UdpGwConn,
    pub name: Arc<String>,
    pub iface: Interface,
    pub signal: Sender<u8>,
}

impl GatewayInner {
    pub fn new(name: Arc<String>, iface: Interface, signal: Sender<u8>) -> Self {
        let conn_set = SocketSet::new(vec![]);
        let conn_all = HashMap::new();
        let udpgw = UdpGwConn::new();
        Self { conn_set, conn_all, udpgw, name, iface, signal }
    }

    fn send_signal(&self) {
        _ = self.signal.try_send(1);
    }

    pub fn delay(&mut self) -> Option<Duration> {
        self.iface.poll_delay(Instant::now(), &self.conn_set)
    }

    pub fn poll<D>(&mut self, device: &mut D) -> Vec<ConnHandle>
    where
        D: Device + ?Sized,
    {
        self.iface.poll(Instant::now(), device, &mut self.conn_set);
        let mut new_conn_h = Vec::new();
        let mut close_conn_h = Vec::new();
        for (h, v) in self.conn_set.iter_mut() {
            match v {
                Socket::Udp(v) => {
                    if v.can_recv() {
                        self.udpgw.recv_wake();
                    }
                    if v.can_send() {
                        self.udpgw.send_wake();
                    }
                    if !v.is_open() {
                        close_conn_h.push(h);
                    }
                }
                Socket::Tcp(v) => {
                    if v.is_open() && !self.conn_all.contains_key(&h) {
                        if let Some(remote) = v.remote_endpoint() {
                            if let Some(local) = v.local_endpoint() {
                                new_conn_h.push(ConnHandle::new(ConnProto::TCP, h.clone(), local, remote));
                            }
                        }
                    }
                    if v.can_recv() {
                        match self.conn_all.get_mut(&h) {
                            Some(c) => c.recv_wake(),
                            None => (),
                        }
                    }
                    if v.can_send() {
                        match self.conn_all.get_mut(&h) {
                            Some(c) => c.send_wake(),
                            None => (),
                        }
                    }
                    if !v.is_open() {
                        close_conn_h.push(h);
                    }
                }
                _ => (),
            }
        }
        for c in &new_conn_h {
            self.conn_all.insert(c.handle, ConnInfo::new(c.clone()));
        }
        for h in close_conn_h {
            let c = self.conn_all.remove(&h);
            let v = self.conn_set.remove(h);
            match v {
                Socket::Udp(v) => {
                    log::info!("Gateway({}) udp conn {:?} is closed", self.name, v.endpoint())
                }
                Socket::Tcp(v) => {
                    if let Some(mut c) = c {
                        c.recv_wake();
                        c.send_wake();
                    }
                    log::info!("Gateway({}) tcp conn {:?}<=>{:?} is closed", self.name, v.local_endpoint(), v.remote_endpoint());
                }
                _ => (),
            }
        }
        new_conn_h
    }

    pub fn poll_read(&mut self, handle: &ConnHandle, cx: &mut Context<'_>, buf: &mut tokio::io::ReadBuf<'_>) -> Poll<tokio::io::Result<()>> {
        match handle.proto {
            ConnProto::TCP => match self.conn_set.find_mut(&handle.handle) {
                Some(v) => match v {
                    Socket::Tcp(v) => {
                        if v.can_recv() {
                            match v.recv(|data| {
                                let max = buf.capacity();
                                let n = data.len();
                                if n > max {
                                    buf.put_slice(&data[0..max]);
                                    (max, ())
                                } else {
                                    buf.put_slice(&data);
                                    (n, ())
                                }
                            }) {
                                Ok(_) => {
                                    self.send_signal();
                                    log::info!("read---->{}", buf.filled().len());
                                    Poll::Ready(Ok(()))
                                }
                                Err(e) => Poll::Ready(Err(new_message_err(e))),
                            }
                        } else {
                            match self.conn_all.get_mut(&handle.handle) {
                                Some(h) => {
                                    h.waker.recv = Some(cx.waker().clone());
                                    Poll::Pending
                                }
                                None => Poll::Ready(Err(new_message_err("invalid conn"))),
                            }
                        }
                    }
                    _ => Poll::Ready(Err(new_message_err("not supporeted conn"))),
                },
                None => Poll::Ready(Err(new_message_err("conn is closed"))),
            },
            ConnProto::UDPGW => {
                for (h, v) in self.conn_set.iter_mut() {
                    if let Socket::Udp(v) = v {
                        if !v.can_recv() {
                            continue;
                        }
                        let local = v.endpoint();
                        match v.recv() {
                            Ok((data, ep)) => {
                                log::info!("xx-->{:?},{}", data, ep);
                                self.udpgw.put(buf, &h, &local, &ep, data.len());
                                buf.put_slice(data);
                                return Poll::Ready(Ok(()));
                            }
                            Err(_) => v.close(),
                        }
                    }
                }
                self.udpgw.waker.recv = Some(cx.waker().clone());
                Poll::Pending
            }
        }
    }

    pub fn poll_write(&mut self, handle: &ConnHandle, cx: &mut Context<'_>, buf: &[u8]) -> Poll<tokio::io::Result<usize>> {
        match handle.proto {
            ConnProto::TCP => match self.conn_set.find_mut(&handle.handle) {
                Some(v) => match v {
                    Socket::Tcp(v) => {
                        if v.can_send() {
                            match v.send_slice(buf) {
                                Ok(n) => {
                                    self.send_signal();
                                    Poll::Ready(Ok(n))
                                }
                                Err(e) => Poll::Ready(Err(new_message_err(e))),
                            }
                        } else {
                            match self.conn_all.get_mut(&handle.handle) {
                                Some(h) => {
                                    h.waker.send = Some(cx.waker().clone());
                                    Poll::Pending
                                }
                                None => Poll::Ready(Err(new_message_err("invalid conn"))),
                            }
                        }
                    }
                    _ => Poll::Ready(Err(new_message_err("not supporeted conn"))),
                },
                None => Poll::Ready(Err(new_message_err("conn is closed"))),
            },
            ConnProto::UDPGW => {
                match self.udpgw.load(&buf) {
                    Some((_, handle, remote, data)) => match self.conn_set.find_mut(handle) {
                        Some(v) => match v {
                            Socket::Udp(v) => {
                                if v.can_send() {
                                    _ = v.send_slice(data, remote.clone());
                                    self.send_signal();
                                    Poll::Ready(Ok(buf.len()))
                                } else {
                                    self.udpgw.waker.send = Some(cx.waker().clone());
                                    Poll::Pending
                                }
                            }
                            _ => Poll::Ready(Ok(buf.len())), //drop
                        },
                        None => Poll::Ready(Ok(buf.len())), //drop
                    },
                    None => Poll::Ready(Ok(buf.len())), //drop
                }
            }
        }
    }

    pub fn poll_shutdown(&mut self, handle: &ConnHandle, _: &mut Context<'_>) -> Poll<tokio::io::Result<()>> {
        match self.conn_set.find_mut(&handle.handle) {
            Some(v) => match v {
                Socket::Tcp(v) => {
                    v.close();
                    self.send_signal();
                    Poll::Ready(Ok(()))
                }
                _ => Poll::Ready(Err(new_message_err("not supporeted conn"))),
            },
            None => Poll::Ready(Err(new_message_err("conn is closed"))),
        }
    }

    pub fn shutdown(&mut self, handle: &ConnHandle) -> tokio::io::Result<()> {
        match self.conn_set.find_mut(&handle.handle) {
            Some(v) => match v {
                Socket::Tcp(v) => {
                    v.close();
                    self.send_signal();
                    Ok(())
                }
                _ => Err(new_message_err("not supporeted conn")),
            },
            None => Err(new_message_err("conn is closed")),
        }
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
        {
            let udpgw = Conn::new(self.inner.clone(), ConnHandle::udpgw());
            let (udpgw_reader, udpgw_writer) = wrap_split(udpgw);
            let dial_addr = "tcp://udpgw";
            let dial_uri = self.remote.replace("${HOST}", &dial_addr);
            self.router.dial_base(udpgw_reader, udpgw_writer, Arc::new(dial_uri)).await?;
        }
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
            let mut gw = self.inner.lock().await;
            while device.rx_size() > 0 {
                let ch_all = gw.poll(&mut device);
                for ch in ch_all {
                    log::info!("Gateway({}) start forward conn {}<=>{} to {}", self.name, ch.local, ch.remote, self.remote);
                    let dial_addr = format!("tcp://{}", ch.local);
                    let dial_uri = self.remote.replace("${HOST}", &dial_addr);
                    let raw_conn = Conn::new(self.inner.clone(), ch.clone());
                    let (raw_reader, raw_writer) = wrap_split(raw_conn);
                    let result = self.router.dial_base(raw_reader, raw_writer, Arc::new(dial_uri)).await;
                    if result.is_err() {
                        _ = gw.shutdown(&ch);
                    }
                }
                device.write(&mut self.writer).await?;
            }
            delay = gw.delay();
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
