pub mod frame;
pub mod log;
pub mod proxy;
pub mod router;
pub mod util;
pub mod wrapper;
// use quinn::Endpoint;
// use std::{error::Error, fs, net::SocketAddr, sync::Arc};
// use walkdir::WalkDir;

// use crate::frame::CombineStream;

// pub static mut EASY_CONNN: ConnPool = ConnPool::new();

// pub struct ConnAddr {
//     pub addr: String,
//     pub delay: i64,
// }

// pub struct ConnPool {
//     addrs: Vec<ConnAddr>,
// }

// impl ConnPool {
//     const fn new() -> ConnPool {
//         ConnPool { addrs: Vec::new() }
//     }

//     pub fn best_addr(&self) -> &ConnAddr {
//         &self.addrs[0]
//     }

//     // pub async fn connect<T, B: ToSocketAddrs>(addr: B) -> tokio::io::Result<FrameStream<TcpStream>>
//     // where
//     //     T: From<TcpStream>,
//     // {
//     //     let target = TcpStream::connect(addr).await?;
//     //     Ok(FrameStream::new(target))
//     // }

//     // pub fn proxy<T>(&self, loc: &T, target: String) -> Result<(), Box<dyn Error>>
//     // where
//     //     T: AsyncRead + AsyncWrite + Unpin + ?Sized,
//     // {
//     //     Ok(())
//     // }
// }

// struct SkipServerVerification;

// impl SkipServerVerification {
//     pub fn new() -> Arc<Self> {
//         Arc::new(Self)
//     }
// }

// impl rustls::client::ServerCertVerifier for SkipServerVerification {
//     fn verify_server_cert(&self, _end_entity: &rustls::Certificate, _intermediates: &[rustls::Certificate], _server_name: &rustls::ServerName, _scts: &mut dyn Iterator<Item = &[u8]>, _ocsp_response: &[u8], _now: std::time::SystemTime) -> Result<rustls::client::ServerCertVerified, rustls::Error> {
//         Ok(rustls::client::ServerCertVerified::assertion())
//     }
// }

// pub fn make_client_endpoint(addr: SocketAddr, config: rustls::ClientConfig) -> Result<Endpoint, Box<dyn Error>> {
//     let mut endpoint = Endpoint::client(addr)?;
//     endpoint.set_default_client_config(quinn::ClientConfig::new(Arc::new(config)));
//     Ok(endpoint)
// }

// pub async fn run_client() {
//     let config = load_client_config(String::from("cert/ca"), &[]).unwrap();
//     let endpoint = make_client_endpoint("127.0.0.1:0".parse().unwrap(), config).unwrap();
//     let connect = endpoint.connect("127.0.0.1:4242".parse().unwrap(), "test.loc").unwrap();
//     let connection = connect.await.unwrap();
//     println!("[client] connected: addr={}", connection.remote_address());
//     // let xx = connection.open_uni();
//     let (sx, rx) = connection.open_bi().await.unwrap();
//     let mut conn = CombineStream::new(sx, rx);
//     // let (mut send, mut recv) = connection.open_bi().await.unwrap();
//     // send.write_all(b"xxx").await.unwrap();
//     // sleep(Duration::from_millis(1000));
//     // send.finish().await.unwrap();
//     let stdin = tokio::io::stdin();
//     let stdout = tokio::io::stdout();
//     let mut std = CombineStream::new(stdout, stdin);
//     // tokio::io::copy(&mut recv, &mut send).await.unwrap();
//     tokio::io::copy_bidirectional(&mut conn, &mut std).await.unwrap();
//     // connection.open_bi().await.
// }

// pub fn load_client_config(dir: String, certs: &[&[u8]]) -> Result<rustls::ClientConfig, Box<dyn Error>> {
//     let mut roots = rustls::RootCertStore::empty();
//     for cert in rustls_native_certs::load_native_certs()? {
//         roots.add(&rustls::Certificate(cert.0))?;
//     }
//     if !dir.is_empty() {
//         for entry in WalkDir::new(dir) {
//             let entry = entry?;
//             if !entry.metadata()?.is_file() {
//                 continue;
//             }
//             let cert_str = fs::read(entry.path())?;
//             let cert = pem::parse(cert_str)?;
//             roots.add(&rustls::Certificate((&cert.contents()).to_vec()))?;
//         }
//     }
//     for cert in certs {
//         roots.add(&rustls::Certificate(cert.to_vec()))?;
//     }
//     let config = rustls::ClientConfig::builder()
//         .with_safe_defaults()
//         // .with_custom_certificate_verifier(SkipServerVerification::new())
//         .with_root_certificates(roots)
//         .with_no_client_auth();
//     Ok(config)
// }

// pub fn add(left: usize, right: usize) -> usize {
//     left + right
// }

// pub async fn xxx() {
//     // let mut s = TcpStream::connect("127.0.0.1:13100").await.unwrap();
//     // let mut stream = Arc::new(Mutex::new(s));
//     // let mut sx = stream.lock().unwrap();
//     // let (rx, tx) = sx.split();
//     // let forward = Arc::new(Mutex::new(ForwardConn::new(rx, tx)));
//     // let runner = forward.clone();
//     // let xx = tokio::spawn(async move {
//     //     let mut f = runner.lock().unwrap();
//     //     // f.loop_forward_read().await;
//     //     // ()
//     // });
//     // tokio::join!(xx);
// }

// #[cfg(test)]
// mod tests {
//     use super::*;

//     #[test]
//     fn it_works() {
//         // let cc: FrameStream<TcpStream> = ConnPool::connect(("s", 10)).await.unwrap();
//         let result = add(2, 2);
//         assert_eq!(result, 4);
//         // ConnPool::SHARED;
//     }
// }
