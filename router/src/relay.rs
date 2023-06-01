extern crate smoltcp;
extern crate tun;

use std::{
    cell::RefCell,
    collections::HashMap,
    hash::Hash,
    io::{ErrorKind, Write},
    net::{IpAddr, Ipv4Addr, SocketAddr, SocketAddrV4},
    pin::Pin,
    rc::Rc,
    sync::Arc,
    task::{Context, Poll},
};

use async_trait::async_trait;
use log::{error, trace, warn};
use smoltcp::{
    iface::{Config, Interface, SocketHandle, SocketSet},
    phy::{Checksum, ChecksumCapabilities, Device, DeviceCapabilities, Medium},
    socket::tcp::Socket as TcpSocket,
    socket::tcp::SocketBuffer as TcpSocketBuffer,
    socket::udp::PacketBuffer as UdpPacketBuffer,
    socket::udp::PacketMetadata as UdpPacketMetadata,
    time::{Duration, Instant},
    wire::{EthernetAddress, Icmpv4Packet, Icmpv4Repr, IpAddress, IpProtocol, IpRepr, IpVersion, Ipv4Packet, Ipv4Repr, PrettyPrinter, TcpControl, TcpPacket, TcpRepr, UdpPacket, UdpRepr},
};
use tokio::{
    io::{AsyncRead, AsyncWrite, AsyncWriteExt},
    sync::{
        mpsc::{self, Receiver, Sender},
        Mutex,
    },
};

use crate::frame;

pub fn new_message_err<E>(err: E) -> std::io::Error
where
    E: Into<Box<dyn std::error::Error + Send + Sync>>,
{
    std::io::Error::new(ErrorKind::Other, err)
}

pub fn wrap_err<T, E>(result: Result<T, E>) -> tokio::io::Result<T>
where
    E: Into<Box<dyn std::error::Error + Send + Sync>>,
{
    match result {
        Ok(v) => Ok(v),
        Err(e) => Err(new_message_err(e)),
    }
}

fn new_udp_buffer(packets: usize) -> UdpPacketBuffer<'static> {
    UdpPacketBuffer::new(vec![UdpPacketMetadata::EMPTY; packets], vec![0; 16 * packets])
}

#[cfg(target_os = "linux")]
const IFF_PI_PREFIX_LEN: usize = 0;
#[cfg(any(target_os = "macos", target_os = "freebsd"))]
const IFF_PI_PREFIX_LEN: usize = 4;
#[cfg(target_os = "macos")]
const IPV4_PACKET_SIGNATURE: [u8; 4] = [000, 000, 000, 002];

const DEFAULT_TUN_MTU: usize = 4096;

pub struct Reader {
    conn_type: Proto,
    local_addr: SocketAddr,
    remote_addr: SocketAddr,
}

pub struct Writer {
    relay: Arc<Mutex<Relay_>>,
    conn_type: Proto,
    local_addr: SocketAddr,
    remote_addr: SocketAddr,
}

impl Writer {
    pub fn xxx(&self) {
        // self.relay.try_lock()
    }
}

#[derive(Clone, Debug, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum Proto {
    TCP,
    UDP,
}

#[derive(Clone, PartialEq, Eq, PartialOrd, Ord, Hash, Debug)]
pub struct ConnMeta {
    pub proto: Proto,
    pub local_addr: IpAddress,
    pub local_port: u16,
    pub remote_addr: IpAddress,
    pub remote_port: u16,
}

impl ConnMeta {
    pub fn new(proto: Proto, local_addr: IpAddress, local_port: u16, remote_addr: IpAddress, remote_port: u16) -> Self {
        Self {
            proto,
            local_addr,
            local_port,
            remote_addr,
            remote_port,
        }
    }

    pub fn as_ipv4(&self) -> tokio::io::Result<(Ipv4Addr, Ipv4Addr)> {
        if let IpAddress::Ipv4(local) = self.local_addr {
            if let IpAddress::Ipv4(remote) = self.remote_addr {
                Ok((local.into(), remote.into()))
            } else {
                Err(new_message_err("remote is not ipv4"))
            }
        } else {
            Err(new_message_err("local is not ipv4"))
        }
    }

    pub fn local_endpoint(&self) -> SocketAddr {
        SocketAddr::new(self.local_addr.into(), self.local_port)
    }

    pub fn remote_endpoint(&self) -> SocketAddr {
        SocketAddr::new(self.remote_addr.into(), self.remote_port)
    }
}

impl std::fmt::Display for ConnMeta {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{:?}({}:{}->{}:{})", self.proto, self.local_addr, self.local_port, self.remote_addr, self.remote_port)
    }
}

pub trait DeviceWriter {
    fn write(&mut self, buf: &[u8]) -> std::io::Result<usize>;
}

pub struct UdpGatewayReader {
    recv: Receiver<Vec<u8>>,
}

#[async_trait]
impl frame::RawReader for UdpGatewayReader {
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        match self.recv.recv().await {
            Some(v) => {
                buf[0..v.len()].copy_from_slice(&v);
                Ok(v.len())
            }
            None => Err(new_message_err("closed")),
        }
    }
}

pub struct UdpGatewayWriter {
    relay: Arc<Mutex<Relay_>>,
}

impl UdpGatewayWriter {
    pub fn new(relay: Arc<Mutex<Relay_>>) -> Self {
        Self { relay }
    }
}

#[async_trait]
impl frame::RawWriter for UdpGatewayWriter {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        let mut relay = self.relay.lock().await;
        relay.outgress_udp(&buf).await
    }
    async fn shutdown(&mut self) {}
}

struct TcpConn {
    pub id: u16,
    pub stream: TcpSocket<'static>,
    pub send: Sender<Vec<u8>>,
}

impl TcpConn {
    pub fn new(id: u16, stream: TcpSocket<'static>, send: Sender<Vec<u8>>) -> Self {
        Self { id, stream, send }
    }
}

pub struct ConnReader {
    pub addr: ConnMeta,
    recv: Receiver<Vec<u8>>,
}

impl ConnReader {
    pub fn new(addr: ConnMeta, recv: Receiver<Vec<u8>>) -> Self {
        Self { addr, recv }
    }
}

#[async_trait]
impl frame::RawReader for ConnReader {
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        match self.recv.recv().await {
            Some(v) => {
                buf[0..v.len()].copy_from_slice(&v);
                Ok(v.len())
            }
            None => Err(new_message_err("closed")),
        }
    }
}

pub struct ConnWriter {
    pub addr: ConnMeta,
    relay: Arc<Mutex<Relay_>>,
}

impl ConnWriter {
    pub fn new(addr: ConnMeta, relay: Arc<Mutex<Relay_>>) -> Self {
        Self { addr, relay }
    }
}

#[async_trait]
impl frame::RawWriter for ConnWriter {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        println!("WW ---->{:?}", buf);
        Ok(buf.len())
    }
    async fn shutdown(&mut self) {}
}

struct Conn_ {
    pub addr: ConnMeta,
    pub recv: Receiver<Vec<u8>>,
}

impl Conn_ {
    pub fn new(addr: ConnMeta, recv: Receiver<Vec<u8>>) -> Self {
        Self { addr, recv }
    }
}

pub struct Relay {
    inner: Arc<Mutex<Relay_>>,
}

impl Relay {
    pub fn new(name: Arc<String>, writer: Box<dyn std::io::Write + Send + Sync>) -> Self {
        Self {
            inner: Arc::new(Mutex::new(Relay_::new(name, writer))),
        }
    }

    pub async fn start_udp_gw(&self, buffer: usize) -> (UdpGatewayReader, UdpGatewayWriter) {
        let reader = self.inner.lock().await.start_udp_gw(buffer);
        (reader, UdpGatewayWriter { relay: self.inner.clone() })
    }

    pub async fn ingress(&self, packet: &[u8]) -> tokio::io::Result<Option<(ConnReader, ConnWriter)>> {
        let mut relay = self.inner.lock().await;
        let conn = relay.ingress(packet).await?;
        if let Some(conn) = conn {
            let writer = ConnWriter::new(conn.addr.clone(), self.inner.clone());
            let reader = ConnReader::new(conn.addr, conn.recv);
            Ok(Some((reader, writer)))
        } else {
            Ok(None)
        }
    }
}

struct TcpData {
    pub cid: u16,
    pub data: Vec<u8>,
}

pub struct Relay_ {
    pub name: Arc<String>,
    iface: Interface,
    conn_all: SocketSet<'static>,
    cid_seq: u16,
    connid_all: HashMap<SocketHandle, u16>,
    handle_all: HashMap<u16, SocketHandle>,
    meta_all: HashMap<u16, ConnMeta>,
    tcp_all: HashMap<ConnMeta, TcpConn>,
    udp_all: HashMap<ConnMeta, u16>,
    udp_gw: Sender<Vec<u8>>,
    tcp_gw: Receiver<TcpData>,
    buffer: Vec<u8>,
}

impl Relay_ {
    pub fn new(name: Arc<String>, writer: Box<dyn std::io::Write + Send + Sync>) -> Self {
        let config = Config::new(EthernetAddress([0x02, 0x00, 0x00, 0x00, 0x00, 0x01]).into());
        // let mut dev = RawDevice::new(1900,  Medium::Ip, reader, writer)
        let mut dev = StmPhy::new();
        let iface = Interface::new(config, &mut dev);
        Self {
            name,
            iface,
            cid_seq: 0,
            writer,
            meta_all: HashMap::new(),
            tcp_all: HashMap::new(),
            udp_all: HashMap::new(),
            udp_gw: None,
            buffer: vec![0u8; 4096],
        }
    }

    fn new_cid(&mut self) -> u16 {
        self.cid_seq += 1;
        self.cid_seq
    }

    fn start_udp_gw(&mut self, buffer: usize) -> UdpGatewayReader {
        let (send, recv) = mpsc::channel(buffer);
        self.udp_gw = Some(send);
        UdpGatewayReader { recv }
    }

    async fn ingress(&mut self, packet: &[u8]) -> tokio::io::Result<Option<Conn_>> {
        match IpVersion::of_packet(&packet) {
            Ok(IpVersion::Ipv4) => self.ingress_ipv4(packet).await,
            Ok(IpVersion::Ipv6) => {
                trace!("droped");
                Ok(None)
            }
            Err(e) => {
                warn!("ingress error {:?}", e);
                Ok(None)
            }
        }
    }

    async fn ingress_ipv4(&mut self, packet: &[u8]) -> tokio::io::Result<Option<Conn_>> {
        let checksum_caps = ChecksumCapabilities::ignored();
        let ipv4_packet = Ipv4Packet::new_unchecked(&packet);
        let ipv4_repr = wrap_err(Ipv4Repr::parse(&ipv4_packet, &checksum_caps))?;
        let src_addr = ipv4_repr.src_addr;
        let dst_addr = ipv4_repr.dst_addr;
        match ipv4_repr.next_header {
            IpProtocol::Icmp => {
                let icmp_packet = Icmpv4Packet::new_unchecked(ipv4_packet.payload());
                let icmp_repr = wrap_err(Icmpv4Repr::parse(&icmp_packet, &checksum_caps))?;
                self.ingress_ipv4_icmp(ipv4_repr, icmp_packet, icmp_repr)
            }
            IpProtocol::Udp => {
                // println!("packet-->{:?}", packet);
                let udp_packet = UdpPacket::new_unchecked(ipv4_packet.payload());
                let udp_repr = wrap_err(UdpRepr::parse(&udp_packet, &IpAddress::Ipv4(src_addr), &IpAddress::Ipv4(dst_addr), &checksum_caps))?;

                // println!(
                //     "\x1b[32m{}\x1b[0m",
                //     PrettyPrinter::<Ipv4Packet<&[u8]>>::new("", &ipv4_packet)
                // );
                // println!("\x1b[32m     \\ {:?}\x1b[0m", &udp_repr.payload);

                self.ingress_ipv4_udp(ipv4_repr, udp_repr, udp_packet).await?;
                Ok(None)
            }
            IpProtocol::Tcp => {
                let tcp_packet = TcpPacket::new_unchecked(ipv4_packet.payload());
                let tcp_repr = wrap_err(TcpRepr::parse(&tcp_packet, &IpAddress::Ipv4(src_addr), &IpAddress::Ipv4(dst_addr), &checksum_caps))?;

                println!("\x1b[32m{}\x1b[0m", PrettyPrinter::<Ipv4Packet<&[u8]>>::new("", &ipv4_packet));
                // println!("\x1b[32m     \\ {:?}\x1b[0m", &tcp_repr.payload);

                self.ingress_ipv4_tcp(ipv4_repr, tcp_repr, tcp_packet).await
            }
            _ => Ok(None),
        }
    }

    fn ingress_ipv4_icmp(&mut self, ipv4_repr: Ipv4Repr, icmp_packet: Icmpv4Packet<&[u8]>, icmp_repr: Icmpv4Repr) -> tokio::io::Result<Option<Conn_>> {
        Ok(None)
    }

    async fn ingress_ipv4_udp(&mut self, ipv4_repr: Ipv4Repr, udp_repr: UdpRepr, udp_packet: UdpPacket<&[u8]>) -> tokio::io::Result<()> {
        if udp_packet.payload().len() == 0 || self.udp_gw.is_none() {
            return Ok(());
        }
        let src_addr = ipv4_repr.src_addr;
        let dst_addr = ipv4_repr.dst_addr;
        let meta = ConnMeta::new(Proto::UDP, dst_addr.into_address(), udp_repr.dst_port, src_addr.into_address(), udp_repr.src_port);
        let cid = match self.udp_all.get(&meta) {
            Some(v) => v.clone(),
            None => {
                let v = self.new_cid();
                self.meta_all.insert(v.clone(), meta.clone());
                v
            }
        };
        let playload = udp_packet.payload();
        let mut data = vec![0u8; playload.len() + 9];
        data[0..2].copy_from_slice(&cid.to_be_bytes() as &[u8]);
        data[2] = 10;
        data[3..7].copy_from_slice(dst_addr.as_bytes());
        data[7..9].copy_from_slice(&udp_repr.dst_port.to_be_bytes() as &[u8]);
        data[9..].copy_from_slice(playload);
        let udp_gw = self.udp_gw.as_ref().unwrap();
        let result = udp_gw.try_send(data);
        if let Err(e) = result {
            warn!("Relay({}) udp gw send fail with {}", self.name, e);
        }
        Ok(())
    }

    async fn outgress_udp(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        let sid = u16::from_be_bytes([buf[0], buf[1]]);
        let meta = match self.meta_all.get(&sid) {
            Some(m) => m,
            None => return Ok(buf.len()),
        };
        match buf[2] {
            10 => {
                let (local_addr, remote_addr) = meta.as_ipv4()?;
                let playload = &buf[9..];
                let mut checksum_caps = ChecksumCapabilities::ignored();
                checksum_caps.ipv4 = Checksum::Tx;
                checksum_caps.udp = Checksum::Tx;

                let udp_repr = UdpRepr {
                    src_port: meta.local_port,
                    dst_port: meta.remote_port,
                };

                let ip_repr = Ipv4Repr {
                    src_addr: local_addr.into(),
                    dst_addr: remote_addr.into(),
                    next_header: IpProtocol::Udp,
                    payload_len: udp_repr.header_len() + playload.len(),
                    hop_limit: 0,
                };

                let total_len = IFF_PI_PREFIX_LEN + ip_repr.buffer_len() + udp_repr.header_len() + playload.len();
                let mut buffer = vec![0u8; total_len];

                let mut ipv4_packet = Ipv4Packet::new_unchecked(&mut buffer[IFF_PI_PREFIX_LEN..]);
                ip_repr.emit(&mut ipv4_packet, &checksum_caps);

                let mut udp_packet = UdpPacket::new_unchecked(ipv4_packet.payload_mut());
                udp_repr.emit(&mut udp_packet, &local_addr.into(), &remote_addr.into(), playload.len(), |b| b.copy_from_slice(playload), &checksum_caps);
                // println!("payload_mut {:?}", ipv4_packet.payload_mut());

                // let pkt = &buffer[IFF_PI_PREFIX_LEN..IFF_PI_PREFIX_LEN + total_len];
                // // DEBUG
                // let ipv4_packet = Ipv4Packet::new_unchecked(&pkt);
                // println!(
                //     "\x1b[31mDispatch: {}\x1b[0m",
                //     PrettyPrinter::<Ipv4Packet<&[u8]>>::new("", &ipv4_packet)
                // );

                // Write to TUN.
                #[cfg(target_os = "macos")]
                buffer[..IFF_PI_PREFIX_LEN].copy_from_slice(&IPV4_PACKET_SIGNATURE);

                println!("W  {},{:?}", buffer.len(), buffer);

                wrap_err(self.writer.write(&buffer))
            }
            v => Err(new_message_err(format!("not supported address type {}", v))),
        }
    }

    async fn ingress_ipv4_tcp(&mut self, ipv4_repr: Ipv4Repr, tcp_repr: TcpRepr<'_>, _: TcpPacket<&[u8]>) -> tokio::io::Result<Option<Conn_>> {
        let src_addr = ipv4_repr.src_addr;
        let dst_addr = ipv4_repr.dst_addr;
        let meta = ConnMeta::new(Proto::TCP, dst_addr.into_address(), tcp_repr.dst_port, src_addr.into_address(), tcp_repr.src_port);
        if self.tcp_all.contains_key(&meta) {
            self.process_ipv4_tcp(ipv4_repr, tcp_repr, meta).await?;
            return Ok(None);
        }
        match (tcp_repr.control, tcp_repr.ack_number) {
            (TcpControl::Syn, None) => {
                let mut sock: TcpSocket = TcpSocket::new(TcpSocketBuffer::new(vec![0u8; 65535]), TcpSocketBuffer::new(vec![0u8; 65535]));
                let local_endpoint = meta.local_endpoint();
                match sock.listen(local_endpoint) {
                    Ok(_) => (),
                    Err(e) => {
                        warn!("Relay({}) listen tcp on {} fail with {:?}", self.name, local_endpoint, e);
                        return Ok(None);
                    }
                }
                sock.set_timeout(Some(smoltcp::time::Duration::from_secs(15)));
                sock.set_ack_delay(Some(Duration::from_millis(1)));

                sock.accepts(self.iface.context(), &ipv4_repr.into(), &tcp_repr);

                let cid = self.new_cid();
                let (send, recv) = mpsc::channel(4);
                let tcp_conn = TcpConn::new(cid, sock, send);
                let conn = Conn_::new(meta.clone(), recv);
                self.meta_all.insert(cid.clone(), meta.clone());
                self.tcp_all.insert(meta.clone(), tcp_conn);

                self.process_ipv4_tcp(ipv4_repr, tcp_repr, meta).await?;

                Ok(Some(conn))
            }
            _ => Ok(None),
        }
    }

    async fn process_ipv4_tcp(&mut self, ipv4_repr: Ipv4Repr, tcp_repr: TcpRepr<'_>, tcp_meta: ConnMeta) -> tokio::io::Result<()> {
        let tcp_conn = match self.tcp_all.get_mut(&tcp_meta) {
            Some(v) => v,
            None => return Ok(()),
        };

        tcp_conn.stream.accepts(self.iface.context(), &ipv4_repr.into(), &tcp_repr);

        let writer = &mut self.writer;
        let buffer = &mut self.buffer;

        let mut checksum_caps = ChecksumCapabilities::ignored();
        checksum_caps.ipv4 = Checksum::Tx;
        checksum_caps.tcp = Checksum::Tx;
        checksum_caps.udp = Checksum::Tx;

        let mut device_caps = smoltcp::phy::DeviceCapabilities::default();
        device_caps.max_transmission_unit = DEFAULT_TUN_MTU - IFF_PI_PREFIX_LEN;
        device_caps.checksum = checksum_caps;

        match tcp_conn.stream.process(self.iface.context(), &ipv4_repr.into(), &tcp_repr) {
            Some((reply_ip_repr, reply_tcp_repr)) => {
                Self::emit_ip_tcp(writer, reply_ip_repr, reply_tcp_repr, buffer)?;
                ()
            }
            None => (),
        };

        tcp_conn.stream.dispatch(self.iface.context(), |_, (reply_ip_repr, reply_tcp_repr)| -> tokio::io::Result<()> {
            Self::emit_ip_tcp(writer, reply_ip_repr, reply_tcp_repr, buffer)
        })?;

        if tcp_conn.stream.may_recv() {
            _ = tcp_conn.stream.recv(|buf: &mut [u8]| {
                // let mut data = vec![0u8; buf.len()];
                // data.copy_from_slice(buf);
                // println!("data--->{:?}", data);
                // // match tcp_conn.send.try_send(data) {
                // //     Ok(_) => (buf.len(), None),
                // //     Err(e) => match e {
                // //         mpsc::error::TrySendError::Full(_) => (0, None),
                // //         mpsc::error::TrySendError::Closed(_) => (0, Some(new_message_err("closed"))),
                // //     },
                // // }
                println!("recv--->{:?}", buf);
                // (buf.len(), None)
                let recvd_len = buf.len();
                (recvd_len, ())
            });
        }
        if tcp_conn.stream.may_send() {
            _ = tcp_conn.stream.send(|buf: &mut [u8]| {
                // let mut data = vec![0u8; buf.len()];
                // data.copy_from_slice(buf);
                // println!("data--->{:?}", data);
                // // match tcp_conn.send.try_send(data) {
                // //     Ok(_) => (buf.len(), None),
                // //     Err(e) => match e {
                // //         mpsc::error::TrySendError::Full(_) => (0, None),
                // //         mpsc::error::TrySendError::Closed(_) => (0, Some(new_message_err("closed"))),
                // //     },
                // // }
                // buf[0..3].copy_from_slice("abc".as_bytes());
                // println!("send--->{:?}", &buf[0..3]);
                (0, ())
            });
        }
        match tcp_conn.stream.process(self.iface.context(), &ipv4_repr.into(), &tcp_repr) {
            Some((reply_ip_repr, reply_tcp_repr)) => {
                Self::emit_ip_tcp(writer, reply_ip_repr, reply_tcp_repr, buffer)?;
                ()
            }
            None => (),
        };

        tcp_conn.stream.dispatch(self.iface.context(), |_, (reply_ip_repr, reply_tcp_repr)| -> tokio::io::Result<()> {
            println!("xx------");
            Self::emit_ip_tcp(writer, reply_ip_repr, reply_tcp_repr, buffer)
        })?;
        Ok(())
    }

    fn emit_ip_tcp(writer: &mut Box<dyn std::io::Write + Send + Sync>, reply_ip_repr: IpRepr, reply_tcp_repr: TcpRepr<'_>, buffer: &mut [u8]) -> tokio::io::Result<()> {
        let mut checksum_caps = ChecksumCapabilities::ignored();
        checksum_caps.ipv4 = Checksum::Tx;
        checksum_caps.tcp = Checksum::Tx;
        checksum_caps.udp = Checksum::Tx;

        let ip_hdr_len = reply_ip_repr.buffer_len();
        let tcp_len = reply_tcp_repr.buffer_len();
        assert!(ip_hdr_len + tcp_len + IFF_PI_PREFIX_LEN <= DEFAULT_TUN_MTU);
        assert_eq!(reply_ip_repr.payload_len(), reply_tcp_repr.buffer_len());

        let total_len = reply_ip_repr.buffer_len();

        let src_addr = reply_ip_repr.src_addr();
        let dst_addr = reply_ip_repr.dst_addr();

        reply_ip_repr.emit(&mut buffer[IFF_PI_PREFIX_LEN..], &checksum_caps);

        let mut ipv4_packet = Ipv4Packet::new_unchecked(&mut buffer[IFF_PI_PREFIX_LEN..]);
        let mut tcp_packet = TcpPacket::new_unchecked(ipv4_packet.payload_mut());
        reply_tcp_repr.emit(&mut tcp_packet, &src_addr, &dst_addr, &checksum_caps);

        let pkt = &buffer[IFF_PI_PREFIX_LEN..IFF_PI_PREFIX_LEN + total_len];

        // DEBUG
        let ipv4_packet = Ipv4Packet::new_unchecked(&pkt);
        // let tcp_packet = TcpPacket::new_unchecked(ipv4_packet.payload());
        println!("\x1b[31mDispatch: {}\x1b[0m", PrettyPrinter::<Ipv4Packet<&[u8]>>::new("", &ipv4_packet));
        // println!("\x1b[31m     \\ {:?}\x1b[0m", &tcp_packet.payload());

        // Write to TUN.
        #[cfg(target_os = "macos")]
        buffer[..IFF_PI_PREFIX_LEN].copy_from_slice(&IPV4_PACKET_SIGNATURE);

        writer.write_all(&buffer[0..IFF_PI_PREFIX_LEN + total_len])?;
        writer.flush()?;
        Ok(())
    }
}

pub struct UdpReader {}

pub struct RawDevice {
    reader: Rc<RefCell<dyn std::io::Read + Send + Sync>>,
    writer: Arc<std::sync::Mutex<dyn std::io::Write + Send + Sync>>,
    mtu: usize,
    medium: Medium,
    buffer: Vec<u8>,
}

impl RawDevice {
    pub fn new(mtu: usize, medium: Medium, reader: Rc<RefCell<dyn std::io::Read + Send + Sync>>, writer: Arc<std::sync::Mutex<dyn std::io::Write + Send + Sync>>) -> RawDevice {
        let buffer = vec![0u8; mtu];
        RawDevice { mtu, medium, reader, writer, buffer }
    }
}

impl Device for RawDevice {
    type RxToken<'a> = RxToken;
    type TxToken<'a> = TxToken;

    fn capabilities(&self) -> DeviceCapabilities {
        let mut caps = DeviceCapabilities::default();
        caps.max_transmission_unit = self.mtu;
        caps.medium = self.medium;
        caps
    }

    fn receive(&mut self, _timestamp: Instant) -> Option<(Self::RxToken<'_>, Self::TxToken<'_>)> {
        let mut reader = self.reader.borrow_mut();
        match reader.read(&mut self.buffer) {
            Ok(size) => {
                let rx = RxToken {
                    buffer: self.buffer[IFF_PI_PREFIX_LEN..size].to_owned(),
                };
                let tx = TxToken { lower: self.writer.clone() };
                Some((rx, tx))
            }
            Err(err) if err.kind() == std::io::ErrorKind::WouldBlock => None,
            Err(err) => panic!("{}", err),
        }
    }

    fn transmit(&mut self, _timestamp: Instant) -> Option<Self::TxToken<'_>> {
        Some(TxToken { lower: self.writer.clone() })
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
    lower: Arc<std::sync::Mutex<dyn std::io::Write + Send + Sync>>,
}

impl smoltcp::phy::TxToken for TxToken {
    fn consume<R, F>(self, len: usize, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        let mut lower = self.lower.lock().unwrap();
        let mut buffer = vec![0; IFF_PI_PREFIX_LEN + len];
        #[cfg(target_os = "macos")]
        buffer[..IFF_PI_PREFIX_LEN].copy_from_slice(&IPV4_PACKET_SIGNATURE);
        let result = f(&mut buffer[IFF_PI_PREFIX_LEN..]);
        match lower.write_all(&buffer[..]) {
            Ok(_) => {
                _ = lower.flush();
            }
            Err(err) => {
                error!("phy: tx failed due to {}", err)
            }
        }
        result
    }
}

struct StmPhy {
    rx_buffer: [u8; 1536],
    tx_buffer: [u8; 1536],
}

impl<'a> StmPhy {
    fn new() -> StmPhy {
        StmPhy { rx_buffer: [0; 1536], tx_buffer: [0; 1536] }
    }
}

impl smoltcp::phy::Device for StmPhy {
    type RxToken<'a> = StmPhyRxToken<'a> where Self: 'a;
    type TxToken<'a> = StmPhyTxToken<'a> where Self: 'a;

    fn receive(&mut self, _timestamp: Instant) -> Option<(Self::RxToken<'_>, Self::TxToken<'_>)> {
        Some((StmPhyRxToken(&mut self.rx_buffer[..]), StmPhyTxToken(&mut self.tx_buffer[..])))
    }

    fn transmit(&mut self, _timestamp: Instant) -> Option<Self::TxToken<'_>> {
        Some(StmPhyTxToken(&mut self.tx_buffer[..]))
    }

    fn capabilities(&self) -> DeviceCapabilities {
        let mut caps = DeviceCapabilities::default();
        caps.max_transmission_unit = 1536;
        caps.max_burst_size = Some(1);
        caps.medium = Medium::Ethernet;
        caps
    }
}

struct StmPhyRxToken<'a>(&'a mut [u8]);

impl<'a> smoltcp::phy::RxToken for StmPhyRxToken<'a> {
    fn consume<R, F>(mut self, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        // TODO: receive packet into buffer
        let result = f(&mut self.0);
        println!("rx called");
        result
    }
}

struct StmPhyTxToken<'a>(&'a mut [u8]);

impl<'a> smoltcp::phy::TxToken for StmPhyTxToken<'a> {
    fn consume<R, F>(self, len: usize, f: F) -> R
    where
        F: FnOnce(&mut [u8]) -> R,
    {
        let result = f(&mut self.0[..len]);
        println!("tx called {}", len);
        // TODO: send packet out
        result
    }
}
