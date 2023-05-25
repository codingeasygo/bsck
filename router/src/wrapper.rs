use std::io::ErrorKind;

use async_trait::async_trait;
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt, ReadHalf, WriteHalf},
    net::TcpStream,
    sync::mpsc::{self, Receiver, Sender},
};

use crate::frame::{Reader, Writer};

pub struct WrapTcpReader {
    inner: ReadHalf<TcpStream>,
}

#[async_trait]
impl Reader for WrapTcpReader {
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        self.inner.read(buf).await
    }
}

pub struct WrapTcpWriter {
    inner: WriteHalf<TcpStream>,
}

#[async_trait]
impl Writer for WrapTcpWriter {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        self.inner.write(buf).await
    }
    async fn shutdown(&mut self) {
        _ = self.inner.shutdown().await;
    }
}

unsafe impl Send for WrapTcpReader {}
unsafe impl Sync for WrapTcpReader {}
unsafe impl Send for WrapTcpWriter {}
unsafe impl Sync for WrapTcpWriter {}

pub fn wrap_split_tcp(stream: TcpStream) -> (WrapTcpReader, WrapTcpWriter) {
    let (rx, tx) = tokio::io::split(stream);
    (WrapTcpReader { inner: rx }, WrapTcpWriter { inner: tx })
}

pub fn wrap_split_tcp_w(stream: TcpStream) -> (Box<dyn Reader + Send + Sync>, Box<dyn Writer + Send + Sync>) {
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
impl Reader for WrapChannelReader {
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
impl Writer for WrapChannelWriter {
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

pub fn wrap_channel() -> (Box<dyn Reader + Send + Sync>, Box<dyn Writer + Send + Sync>) {
    let (tx, rx) = mpsc::channel::<Vec<u8>>(16);
    let rxa = Box::new(WrapChannelReader { inner: rx });
    let txa = Box::new(WrapChannelWriter { inner: tx });
    (rxa, txa)
}
