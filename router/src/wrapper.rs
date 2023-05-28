use std::io::ErrorKind;

use async_trait::async_trait;
use tokio::{
    io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt, ReadHalf, WriteHalf},
    net::TcpStream,
    sync::mpsc::{self, Receiver, Sender},
};
use tokio_rustls::client::TlsStream;

use crate::frame;

pub struct WrapTcpReader<T> {
    inner: ReadHalf<T>,
}

#[async_trait]
impl<T> frame::RawReader for WrapTcpReader<T>
where
    T: AsyncRead + Send + Sync,
{
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        self.inner.read(buf).await
    }
}

pub struct WrapTcpWriter<T> {
    inner: WriteHalf<T>,
}

#[async_trait]
impl<T> frame::RawWriter for WrapTcpWriter<T>
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

pub fn wrap_split<T>(stream: T) -> (WrapTcpReader<T>, WrapTcpWriter<T>)
where
    T: AsyncRead + AsyncWrite + Send + Sync,
{
    let (rx, tx) = tokio::io::split(stream);
    (WrapTcpReader { inner: rx }, WrapTcpWriter { inner: tx })
}

pub fn wrap_split_tcp_w(stream: TcpStream) -> (Box<dyn frame::RawReader + Send + Sync>, Box<dyn frame::RawWriter + Send + Sync>) {
    let (rx, tx) = tokio::io::split(stream);
    let rxa = Box::new(WrapTcpReader { inner: rx });
    let txa = Box::new(WrapTcpWriter { inner: tx });
    (rxa, txa)
}

pub fn wrap_split_tls_w(stream: TlsStream<TcpStream>) -> (Box<dyn frame::RawReader + Send + Sync>, Box<dyn frame::RawWriter + Send + Sync>) {
    let (rx, tx) = tokio::io::split(stream);
    let rxa = Box::new(WrapTcpReader { inner: rx });
    let txa = Box::new(WrapTcpWriter { inner: tx });
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

pub fn wrap_channel() -> (Box<dyn frame::RawReader + Send + Sync>, Box<dyn frame::RawWriter + Send + Sync>) {
    let (tx, rx) = mpsc::channel::<Vec<u8>>(16);
    let rxa = Box::new(WrapChannelReader { inner: rx });
    let txa = Box::new(WrapChannelWriter { inner: tx });
    (rxa, txa)
}
