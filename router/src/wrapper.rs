use std::{io::ErrorKind, net::SocketAddr, sync::Arc};

use async_trait::async_trait;
use tokio::{
    io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt, ReadHalf, WriteHalf},
    net::{TcpStream, UdpSocket},
    sync::mpsc::{self, Receiver, Sender},
};
use tokio_rustls::client::TlsStream;

use crate::{frame, util::wrap_err};

pub struct WrapHalfReader<T> {
    inner: ReadHalf<T>,
}

#[async_trait]
impl<T> frame::RawReader for WrapHalfReader<T>
where
    T: AsyncRead + Send + Sync,
{
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        let n = self.inner.read(buf).await?;
        Ok(n)
    }
}

pub struct WrapHalfWriter<T> {
    inner: WriteHalf<T>,
}

#[async_trait]
impl<T> frame::RawWriter for WrapHalfWriter<T>
where
    T: AsyncWrite + Send + Sync,
{
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        self.inner.write_all(buf).await?;
        self.inner.flush().await?;
        Ok(buf.len())
    }
    async fn shutdown(&mut self) {
        _ = self.inner.shutdown().await;
    }
}

// unsafe impl Send for WrapTcpReader {}
// unsafe impl Sync for WrapTcpReader {}
// unsafe impl Send for WrapTcpWriter {}
// unsafe impl Sync for WrapTcpWriter {}

pub fn wrap_split<T>(stream: T) -> (WrapHalfReader<T>, WrapHalfWriter<T>)
where
    T: AsyncRead + AsyncWrite + Send + Sync + 'static,
{
    let (rx, tx) = tokio::io::split(stream);
    let rxa = WrapHalfReader { inner: rx };
    let txa = WrapHalfWriter { inner: tx };
    (rxa, txa)
}

pub fn wrap_split_w<T>(stream: T) -> (Box<dyn frame::RawReader + Send + Sync + 'static>, Box<dyn frame::RawWriter + Send + Sync + 'static>)
where
    T: AsyncRead + AsyncWrite + Send + Sync + 'static,
{
    let (rx, tx) = tokio::io::split(stream);
    let rxa = Box::new(WrapHalfReader { inner: rx });
    let txa = Box::new(WrapHalfWriter { inner: tx });
    (rxa, txa)
}

pub fn wrap_split_tcp_w(stream: TcpStream) -> (Box<dyn frame::RawReader + Send + Sync>, Box<dyn frame::RawWriter + Send + Sync>) {
    let (rx, tx) = tokio::io::split(stream);
    let rxa = Box::new(WrapHalfReader { inner: rx });
    let txa = Box::new(WrapHalfWriter { inner: tx });
    (rxa, txa)
}

pub fn wrap_split_tls_w(stream: TlsStream<TcpStream>) -> (Box<dyn frame::RawReader + Send + Sync>, Box<dyn frame::RawWriter + Send + Sync>) {
    let (rx, tx) = tokio::io::split(stream);
    let rxa = Box::new(WrapHalfReader { inner: rx });
    let txa = Box::new(WrapHalfWriter { inner: tx });
    (rxa, txa)
}

#[derive(Clone)]
pub struct WrapUdpConn {
    pub inner: Arc<UdpSocket>,
    pub remote: SocketAddr,
}

impl WrapUdpConn {
    pub async fn bind(adrr: String, remote: String) -> tokio::io::Result<WrapUdpConn> {
        let remote = wrap_err(remote.parse())?;
        let inner = UdpSocket::bind(adrr).await?;
        let conn = WrapUdpConn { inner: Arc::new(inner), remote };
        Ok(conn)
    }

    pub fn local_addr(&self) -> tokio::io::Result<SocketAddr> {
        self.inner.local_addr()
    }
}

#[async_trait]
impl frame::RawReader for WrapUdpConn {
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        let (n, _) = self.inner.recv_from(buf).await?;
        // println!("R==>{},{:?}", frome, &buf[0..n]);
        Ok(n)
    }
}

#[async_trait]
impl frame::RawWriter for WrapUdpConn {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        // println!("W==>{},{:?}", self.remote, &buf);
        let n = self.inner.send_to(buf, self.remote).await?;
        Ok(n)
    }
    async fn shutdown(&mut self) {}
}

pub struct WrapQuinnReader {
    inner: quinn::RecvStream,
}

#[async_trait]
impl frame::RawReader for WrapQuinnReader {
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        match self.inner.read(buf).await? {
            Some(v) => Ok(v),
            None => Ok(0),
        }
    }
}

pub struct WrapQuinnWriter {
    inner: quinn::SendStream,
    closed: bool,
}

#[async_trait]
impl frame::RawWriter for WrapQuinnWriter {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        self.inner.write_all(buf).await?;
        self.inner.flush().await?;
        Ok(buf.len())
    }
    async fn shutdown(&mut self) {
        if self.closed {
            return;
        }
        self.closed = true;
        _ = self.inner.shutdown().await;
    }
}

pub fn wrap_quinn_w(send: quinn::SendStream, recv: quinn::RecvStream) -> (Box<dyn frame::RawReader + Send + Sync>, Box<dyn frame::RawWriter + Send + Sync>) {
    let rxa = Box::new(WrapQuinnReader { inner: recv });
    let txa = Box::new(WrapQuinnWriter { inner: send, closed: false });
    (rxa, txa)
}

// pub fn wrap_conn_tcp(header: Arc<Header>, id: u16, conn_type: ConnType, forward: Arc<Mutex<RouterForward>>,buffer_size:usize, stream: TcpStream) -> Arc<RouterConn> {
//     let (rx, tx) = wrap_split_tcp_w(stream);
//     let conn=Arc::new(Mutex::new(Conn::new(id.clone(),conn_type.clone())));
//     let frame_reader=FrameReader::new(header.clone(),rx,buffer_size);
//     let reader=Arc::new(Mutex::new(RouterConnReader::new(header.clone(), FrameReader::new(), conn)))
//     let (reader, writer) = (Arc::new(Mutex::new(rxx)), Arc::new(Mutex::new(txx)));
//     Arc::new(RouterConn::new(header, id, conn_type, reader, writer, forward))
// }

pub struct WrapChannelReader {
    inner: Receiver<Vec<u8>>,
}

#[async_trait]
impl frame::RawReader for WrapChannelReader {
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

pub struct WrapChannelWriter {
    inner: Sender<Vec<u8>>,
}

#[async_trait]
impl frame::RawWriter for WrapChannelWriter {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        match self.inner.send(Vec::from(buf)).await {
            Ok(_) => Ok(buf.len()),
            Err(e) => Err(std::io::Error::new(ErrorKind::Other, e)),
        }
    }
    async fn shutdown(&mut self) {
        _ = self.inner.send(Vec::from([])).await;
    }
}

unsafe impl Send for WrapChannelReader {}
unsafe impl Sync for WrapChannelReader {}
unsafe impl Send for WrapChannelWriter {}
unsafe impl Sync for WrapChannelWriter {}

pub fn wrap_channel(buffer: usize) -> (WrapChannelReader, WrapChannelWriter) {
    let (tx, rx) = mpsc::channel::<Vec<u8>>(buffer);
    let rxa = WrapChannelReader { inner: rx };
    let txa = WrapChannelWriter { inner: tx };
    (rxa, txa)
}

pub fn wrap_channel_w(buffer: usize) -> (Box<dyn frame::RawReader + Send + Sync>, Box<dyn frame::RawWriter + Send + Sync>) {
    let (tx, rx) = mpsc::channel::<Vec<u8>>(buffer);
    let rxa = Box::new(WrapChannelReader { inner: rx });
    let txa = Box::new(WrapChannelWriter { inner: tx });
    (rxa, txa)
}

pub struct TunReader<T>
where
    T: AsyncRead + Send + Sync,
{
    inner: ReadHalf<T>,
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    buffer: Vec<u8>,
}

impl<T> TunReader<T>
where
    T: AsyncRead + Send + Sync,
{
    pub fn new(inner: ReadHalf<T>) -> Self {
        Self {
            inner,
            #[cfg(any(target_os = "macos", target_os = "freebsd"))]
            buffer: vec![0u8; 2038],
        }
    }
}

#[async_trait]
impl<T> frame::RawReader for TunReader<T>
where
    T: AsyncRead + Send + Sync,
{
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        #[cfg(any(target_os = "linux", target_os = "android"))]
        {
            let n = self.inner.read(buf).await?;
            Ok(n)
        }
        #[cfg(any(target_os = "macos", target_os = "freebsd"))]
        {
            let n = self.inner.read(&mut self.buffer).await?;
            buf[0..n - 4].copy_from_slice(&self.buffer[4..n]);
            Ok(n)
        }
    }
}

pub struct TunWriter<T>
where
    T: AsyncWrite + Send + Sync,
{
    inner: WriteHalf<T>,
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    buffer: Vec<u8>,
}

impl<T> TunWriter<T>
where
    T: AsyncWrite + Send + Sync,
{
    pub fn new(inner: WriteHalf<T>) -> Self {
        Self {
            inner,
            #[cfg(any(target_os = "macos", target_os = "freebsd"))]
            buffer: vec![0u8; 2038],
        }
    }
}

#[async_trait]
impl<T> frame::RawWriter for TunWriter<T>
where
    T: AsyncWrite + Send + Sync,
{
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        #[cfg(any(target_os = "linux", target_os = "android"))]
        {
            self.inner.write_all(&buf).await?;
            Ok(buf.len())
        }
        #[cfg(any(target_os = "macos", target_os = "freebsd"))]
        {
            let n = buf.len();
            self.buffer[3] = 2;
            self.buffer[4..4 + n].copy_from_slice(buf);
            self.inner.write_all(&self.buffer[0..4 + n]).await?;
            Ok(n)
        }
    }
    async fn shutdown(&mut self) {
        _ = self.inner.shutdown().await;
    }
}

pub fn wrap_tun<T>(tun: T) -> (TunReader<T>, TunWriter<T>)
where
    T: AsyncRead + AsyncWrite + Sync + Send,
{
    let (tun_reader, tun_writer) = tokio::io::split(tun);
    let tun_reader = TunReader::new(tun_reader);
    let tun_writer = TunWriter::new(tun_writer);
    (tun_reader, tun_writer)
}
