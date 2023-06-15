use async_trait::async_trait;
use router::{frame, log::init_simple_log, proxy::Proxy, router::NormalAcessHandler, util::JSON};
use serde_json::json;
use smoltcp::wire::{IpAddress, IpCidr};
use std::sync::Arc;
use tokio::{
    io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt, ReadHalf, WriteHalf},
    sync::mpsc,
};

#[tokio::main(flavor = "multi_thread", worker_threads = 2)]
async fn main() {
    init_simple_log(0).unwrap();
    let name = Arc::new(String::from("NX"));
    let mut options = JSON::new();
    options.insert(String::from("name"), serde_json::Value::String(String::from("NX")));
    options.insert(String::from("token"), serde_json::Value::String(String::from("123")));
    options.insert(String::from("remote"), serde_json::Value::String(String::from("tcp://192.168.1.7:13100")));
    options.insert(String::from("domain"), serde_json::Value::String(String::from("test.loc")));
    options.insert(String::from("tls_ca"), json!("certs/rootCA.crt"));
    options.insert(String::from("keep"), json!(10));
    let options = Arc::new(options);
    let socks_dial_uri = Arc::new(String::from("N0->${HOST}"));
    let gw_dial_uri = Arc::new(String::from("N0->${HOST}"));
    let tcp_dial_uri = Arc::new(String::from("N0->tcp://127.0.0.1:13200"));
    let socks_addr = String::from("socks://127.0.0.1:1107");
    let tcp_addr = String::from("tcp://127.0.0.1:13300");
    let web_addr = String::from("tcp://127.0.0.1:1100");
    let handler = Arc::new(NormalAcessHandler::new());
    let mut proxy = Proxy::new(name, handler);
    proxy.channels.insert(String::from("N0"), options);
    _ = proxy.keep().await;
    proxy.start_forward(Arc::new(String::from("s01")), &socks_addr, socks_dial_uri).await.unwrap();
    proxy.start_forward(Arc::new(String::from("t01")), &tcp_addr, tcp_dial_uri).await.unwrap();
    proxy.start_web(Arc::new(String::from("web")), &web_addr).await.unwrap();

    //
    let mut config = tun::Configuration::default();
    config.address((10, 1, 0, 3)).netmask((255, 255, 255, 0)).destination((10, 1, 0, 1)).up();
    let dev = tun::create_as_async(&config).unwrap();
    let (dev_reader, dev_writer) = tokio::io::split(dev);
    let dev_reader = TunReader::new(dev_reader);
    let dev_writer = TunWriter::new(dev_writer);
    proxy.start_gateway(IpCidr::new(IpAddress::v4(10, 1, 0, 2), 24), dev_reader, dev_writer, gw_dial_uri).await;
    //
    let (stopper, receive) = mpsc::channel(8);
    proxy.run(receive).await;
    _ = stopper.send(1).await;
}

pub struct TunReader<T>
where
    T: AsyncRead + Send + Sync,
{
    inner: ReadHalf<T>,
    buffer: Vec<u8>,
}

impl<T> TunReader<T>
where
    T: AsyncRead + Send + Sync,
{
    pub fn new(inner: ReadHalf<T>) -> Self {
        Self { inner, buffer: vec![0u8; 2038] }
    }
}

#[async_trait]
impl<T> frame::RawReader for TunReader<T>
where
    T: AsyncRead + Send + Sync,
{
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize> {
        let n = self.inner.read(&mut self.buffer).await?;
        // log::info!("R-->{:?}", &self.buffer[0..n]);
        buf[0..n - 4].copy_from_slice(&self.buffer[4..n]);
        Ok(n)
    }
}

pub struct TunWriter<T>
where
    T: AsyncWrite + Send + Sync,
{
    inner: WriteHalf<T>,
    buffer: Vec<u8>,
}

impl<T> TunWriter<T>
where
    T: AsyncWrite + Send + Sync,
{
    pub fn new(inner: WriteHalf<T>) -> Self {
        Self { inner, buffer: vec![0u8; 2038] }
    }
}

#[async_trait]
impl<T> frame::RawWriter for TunWriter<T>
where
    T: AsyncWrite + Send + Sync,
{
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize> {
        let n = buf.len();
        self.buffer[3] = 2;
        self.buffer[4..4 + n].copy_from_slice(buf);
        // log::info!("W-->{:?}", &self.buffer[0..4 + n]);
        self.inner.write_all(&self.buffer[0..4 + n]).await?;
        Ok(n)
    }
    async fn shutdown(&mut self) {
        _ = self.inner.shutdown().await;
    }
}
