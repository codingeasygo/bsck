use async_trait::async_trait;
use bytes::Bytes;
use futures::Future;
use http_body_util::Full;
use hyper::body::Incoming;
use hyper::server::conn::http1;
use hyper::service::Service;
use hyper::{Request, Response, StatusCode};
use log::{info, warn};
use rustls::OwnedTrustAnchor;
use std::fs::File;
use std::io::BufReader;
use std::os::fd::{AsRawFd, RawFd};
use std::pin::Pin;
use std::{collections::HashMap, net::SocketAddr, sync::Arc};
use tokio::net::TcpSocket;
use tokio::{
    net::{TcpListener, TcpStream},
    sync::Mutex,
};
use tokio_rustls::TlsConnector;

use crate::util::{json_must_str, json_option_i64, json_option_str, json_option_str_tuple, read_certs, wrap_err, SkipServerVerification, JSON};
use crate::wrapper::wrap_quinn_w;
use crate::{
    router::{Handler, Router},
    util::new_message_err,
    wrapper::{wrap_split_tcp_w, wrap_split_tls_w},
};

pub struct ServerState {
    pub name: Arc<String>,
    pub stopper: String,
    pub stopping: bool,
}

impl ServerState {
    pub fn new(name: Arc<String>, stopper: String) -> Self {
        Self { name, stopper, stopping: false }
    }

    pub async fn stop(&mut self) {
        self.stopping = true;
        _ = TcpStream::connect(&self.stopper).await;
    }

    pub fn to_string(&self) -> String {
        format!("Server(name:{},stopper:{})", self.name, self.stopper)
    }
}

#[async_trait]
pub trait Preparer {
    async fn prepare_fd(&self, fd: RawFd) -> tokio::io::Result<()>;
}

pub struct SkipPreparer {}

impl SkipPreparer {
    pub fn new() -> Self {
        Self {}
    }
}

#[async_trait]
impl Preparer for SkipPreparer {
    async fn prepare_fd(&self, _: RawFd) -> tokio::io::Result<()> {
        Ok(())
    }
}

pub struct Proxy {
    pub name: Arc<String>,
    pub router: Arc<Router>,
    pub handler: Arc<dyn Handler + Send + Sync>,
    pub preparer: Arc<dyn Preparer + Send + Sync>,
    listener: HashMap<String, Arc<Mutex<ServerState>>>,
    waiter: Arc<wg::AsyncWaitGroup>,
}

impl Proxy {
    pub fn new(name: Arc<String>, handler: Arc<dyn Handler + Send + Sync>) -> Self {
        let router = Arc::new(Router::new(name.clone(), handler.clone()));
        let waiter = Arc::new(wg::AsyncWaitGroup::new());
        waiter.add(1);
        let preparer = Arc::new(SkipPreparer::new());
        Self { name, router, handler, preparer, listener: HashMap::new(), waiter }
    }

    fn load_tls_config(option: &Arc<JSON>) -> tokio::io::Result<Arc<rustls::ClientConfig>> {
        let mut root_store = rustls::RootCertStore::empty();
        match json_option_str(&option, "tls_ca") {
            Some(ca) => {
                let mut pem = BufReader::new(File::open(ca.as_str())?);
                let certs = rustls_pemfile::certs(&mut pem)?;
                let trust_anchors = certs.iter().map(|cert| {
                    let ta = webpki::TrustAnchor::try_from_cert_der(&cert[..]).unwrap();
                    OwnedTrustAnchor::from_subject_spki_name_constraints(ta.subject, ta.spki, ta.name_constraints)
                });
                root_store.add_server_trust_anchors(trust_anchors);
            }
            None => {
                root_store.add_server_trust_anchors(webpki_roots::TLS_SERVER_ROOTS.0.iter().map(|ta| OwnedTrustAnchor::from_subject_spki_name_constraints(ta.subject, ta.spki, ta.name_constraints)));
            }
        }

        let verify = match json_option_i64(&option, "tls_verify") {
            Some(v) => v > 0,
            None => true,
        };
        if verify {
            let builder = rustls::ClientConfig::builder().with_safe_defaults().with_root_certificates(root_store);
            let config = match json_option_str_tuple(&option, "tls_cert", "tls_key") {
                Some((cert, key)) => {
                    let (cert, key) = read_certs(cert, key)?;
                    match builder.with_single_cert(cert, key) {
                        Ok(v) => Ok(v),
                        Err(e) => Err(new_message_err(e)),
                    }?
                }
                None => builder.with_no_client_auth(),
            };
            Ok(Arc::new(config))
        } else {
            let builder = rustls::ClientConfig::builder().with_safe_defaults().with_custom_certificate_verifier(SkipServerVerification::new());
            let config = builder.with_no_client_auth();
            Ok(Arc::new(config))
        }
    }

    pub async fn login(&mut self, option: Arc<JSON>) -> tokio::io::Result<()> {
        let remote = json_must_str(&option, "remote")?;
        if remote.starts_with("tcp://") {
            let conn = TcpSocket::new_v4()?;
            conn.bind("0.0.0.0:0".parse().unwrap())?;
            let fd = conn.as_raw_fd();
            self.preparer.prepare_fd(fd).await?;

            let domain = remote.trim_start_matches("tcp://");
            let addr = wrap_err(domain.parse())?;
            let stream = conn.connect(addr).await?;
            let (rx, tx) = wrap_split_tcp_w(stream);
            self.router.join_base(rx, tx, option).await?;
            Ok(())
        } else if remote.starts_with("tls://") {
            let conn = TcpSocket::new_v4()?;
            conn.bind(":0".parse().unwrap())?;
            let fd = conn.as_raw_fd();
            self.preparer.prepare_fd(fd).await?;

            let domain = remote.trim_start_matches("tcp://");
            let addr = wrap_err(domain.parse())?;
            let tls = Self::load_tls_config(&option)?;
            let connector = TlsConnector::from(tls);
            let stream = conn.connect(addr).await?;
            let server_name = rustls::ServerName::try_from(domain).map_err(|_| new_message_err("invalid domain"))?;
            let stream = connector.connect(server_name, stream).await?;
            let (rx, tx) = wrap_split_tls_w(stream);
            self.router.join_base(rx, tx, option).await?;
            Ok(())
        } else if remote.starts_with("quic://") {
            let conn = std::net::UdpSocket::bind(":0")?;
            let fd = conn.as_raw_fd();
            self.preparer.prepare_fd(fd).await?;

            let domain = remote.trim_start_matches("quic://");
            let addr = wrap_err(domain.parse())?;
            let runtime = quinn::default_runtime().ok_or_else(|| new_message_err("no async runtime found"))?;
            let mut endpoint = quinn::Endpoint::new(quinn::EndpointConfig::default(), None, conn, runtime)?;
            let tls = Self::load_tls_config(&option)?;
            endpoint.set_default_client_config(quinn::ClientConfig::new(tls));
            let conn = wrap_err(endpoint.connect(addr, domain))?.await?;
            let (send, recv) = conn.open_bi().await?;
            let (rx, tx) = wrap_quinn_w(send, recv);
            self.router.join_base(rx, tx, option).await?;
            Ok(())
        } else {
            Err(new_message_err("not tls client config"))
        }
    }

    pub async fn start_forward(&mut self, name: Arc<String>, loc: &String, remote: Arc<String>) -> tokio::io::Result<()> {
        let router = self.router.clone();
        if loc.starts_with("socks://") {
            let domain: &str = loc.trim_start_matches("socks://");
            let ln = TcpListener::bind(&domain).await?;
            let state = Arc::new(Mutex::new(ServerState::new(name.clone(), domain.to_string())));
            info!("Proxy({}) listen socks {} is success", self.name, ln.local_addr().unwrap());
            self.listener.insert(name.to_string(), state.clone());
            let waiter = self.waiter.clone();
            waiter.add(1);
            tokio::spawn(async move { Self::loop_socks_accpet(name, waiter, ln, state, router, remote).await });
        } else {
            let domain: &str = loc.trim_start_matches("tcp://");
            let ln = TcpListener::bind(&domain).await?;
            let state = Arc::new(Mutex::new(ServerState::new(name.clone(), domain.to_string())));
            info!("Proxy({}) listen tcp {} is success", self.name, ln.local_addr().unwrap());
            self.listener.insert(name.to_string(), state.clone());
            let waiter = self.waiter.clone();
            waiter.add(1);
            tokio::spawn(async move { Self::loop_tcp_accpet(name, waiter, ln, state, router, remote).await });
        }
        Ok(())
    }

    async fn loop_tcp_accpet(name: Arc<String>, waiter: Arc<wg::AsyncWaitGroup>, ln: TcpListener, state: Arc<Mutex<ServerState>>, router: Arc<Router>, remote: Arc<String>) -> tokio::io::Result<()> {
        waiter.add(1);
        info!("Proxy({}) forward tcp {:?}->{} loop is starting", name, ln.local_addr().unwrap(), &remote);
        let err = loop {
            match ln.accept().await {
                Ok((stream, from)) => {
                    if state.lock().await.stopping {
                        break new_message_err("stopped");
                    }
                    Self::proc_tcp_conn(&name, router.clone(), stream, from, remote.clone()).await;
                }
                Err(e) => {
                    warn!("Proxy({}) accept tcp on {:?} is fail by {:?}", name, ln.local_addr().unwrap(), e);
                }
            }
        };
        info!("Proxy({}) forward tcp {:?}->{} loop is stopped by {:?}", name, ln.local_addr().unwrap(), &remote, err);
        waiter.done();
        Ok(())
    }

    async fn proc_tcp_conn(name: &Arc<String>, router: Arc<Router>, stream: TcpStream, from: SocketAddr, remote: Arc<String>) {
        info!("Proxy({}) start forward tcp conn {:?} to {:?}", name, from, &remote);
        let (reader, writer) = wrap_split_tcp_w(stream);
        match router.dial_base(reader, writer, remote.clone()).await {
            Ok(_) => (),
            Err(e) => {
                info!("Proxy({}) forward tcp conn {:?} to {:?} fail with {:?}", name, from, &remote, e);
            }
        }
    }

    async fn loop_socks_accpet(name: Arc<String>, waiter: Arc<wg::AsyncWaitGroup>, ln: TcpListener, state: Arc<Mutex<ServerState>>, router: Arc<Router>, remote: Arc<String>) -> tokio::io::Result<()> {
        info!("Proxy({}) forward socks5 {:?}->{} loop is starting", name, ln.local_addr().unwrap(), remote);
        let err = loop {
            match ln.accept().await {
                Ok((stream, from)) => {
                    info!("accept sockes proxy from {:?}", from);
                    if state.lock().await.stopping {
                        break new_message_err("stopped");
                    }
                    let name = name.clone();
                    let router = router.clone();
                    let remote = remote.clone();
                    let waiter = waiter.clone();
                    _ = tokio::spawn(async move {
                        waiter.add(1);
                        Self::proc_socks_conn(name, router, stream, from, remote).await;
                        waiter.done();
                    });
                }
                Err(e) => {
                    warn!("Proxy({}) accept socks5 on {:?} is fail by {:?}", name, ln.local_addr().unwrap(), e);
                }
            }
        };
        info!("Proxy({}) forward socks5 {:?}->{} loop is stopped by {:?}", name, ln.local_addr().unwrap(), &remote, err);
        waiter.done();
        Ok(())
    }

    async fn proc_socks_conn(name: Arc<String>, router: Arc<Router>, stream: TcpStream, from: SocketAddr, remote: Arc<String>) {
        info!("Proxy({}) start forward socks conn {:?} to {:?}", name, from, &remote);
        let (reader, writer) = wrap_split_tcp_w(stream);
        match router.dial_socks(reader, writer, remote.clone()).await {
            Ok(_) => (),
            Err(e) => {
                info!("Proxy({}) forward socks conn {:?} to {:?} fail with {:?}", name, from, &remote, e);
            }
        }
    }

    pub async fn start_web(&mut self, name: Arc<String>, domain: &String) -> tokio::io::Result<()> {
        let router = self.router.clone();
        let domain: &str = domain.trim_start_matches("tcp://");
        let state = Arc::new(Mutex::new(ServerState::new(name.clone(), domain.to_string())));
        info!("Proxy({}) listen web server {} is success", self.name, domain);
        let ln = TcpListener::bind(&domain).await?;
        self.listener.insert(name.to_string(), state.clone());
        let name = self.name.clone();
        let waiter = self.waiter.clone();
        waiter.add(1);
        tokio::spawn(async move { Self::loop_web_accpet(name, waiter, ln, state, router).await });
        Ok(())
    }

    async fn loop_web_accpet(name: Arc<String>, waiter: Arc<wg::AsyncWaitGroup>, ln: TcpListener, state: Arc<Mutex<ServerState>>, router: Arc<Router>) -> tokio::io::Result<()> {
        let name = Arc::new(name);
        info!("Proxy({}) web server {} loop is starting", name, ln.local_addr().unwrap());
        let err = loop {
            let (stream, _) = ln.accept().await?;
            if state.lock().await.stopping {
                break new_message_err("stopped");
            }
            let name = name.clone();
            let router = router.clone();
            tokio::task::spawn(async move {
                let handler = ProxyWebHandler { router };
                if let Err(e) = http1::Builder::new().keep_alive(true).serve_connection(stream, handler).await {
                    warn!("Proxy({}) web server proc http fail with {:?}", name, e);
                }
            });
        };
        info!("Proxy({}) web server {} loop is stopped by {:?}", name, ln.local_addr().unwrap(), err);
        waiter.done();
        Ok(())
    }

    pub async fn wait(&self) {
        self.router.wait().await;
        self.waiter.clone().wait().await;
    }

    pub async fn shutdown(&mut self) {
        info!("Proxy({}) is stopping", self.name);
        for server in self.listener.values() {
            let mut server = server.lock().await;
            info!("Proxy({}) listener {} is stopping", self.name, server.to_string());
            server.stop().await;
        }
        self.router.shutdown().await;
        self.waiter.done();
        self.waiter.wait().await;
    }

    pub async fn display(&self) -> json::JsonValue {
        // self.router.lock().await.display().await
        json::object! {}
    }
}

struct ProxyWebHandler {
    pub router: Arc<Router>,
}

impl ProxyWebHandler {
    async fn display(router: Arc<Router>) -> Result<Response<Full<Bytes>>, hyper::Error> {
        let display = router.display().await;
        let data = json::stringify(display);
        Self::make_response(StatusCode::OK, data)
    }

    async fn backtrace(_: Arc<Router>) -> Result<Response<Full<Bytes>>, hyper::Error> {
        Self::make_response(StatusCode::OK, format!("{:?}", std::backtrace::Backtrace::capture()))
    }

    fn make_response(code: StatusCode, s: String) -> Result<Response<Full<Bytes>>, hyper::Error> {
        Ok(Response::builder().status(code).body(Full::new(Bytes::from(s))).unwrap())
    }
}

impl Service<Request<Incoming>> for ProxyWebHandler {
    type Response = Response<Full<Bytes>>;
    type Error = hyper::Error;
    type Future = Pin<Box<dyn Future<Output = Result<Self::Response, Self::Error>> + Send>>;

    fn call(&mut self, req: Request<Incoming>) -> Self::Future {
        let uri = req.uri().path().to_string();
        let router = self.router.clone();
        Box::pin(async move {
            match uri.as_str() {
                "/display" => Self::display(router).await,
                "/backtrace" => Self::backtrace(router).await,
                _ => Self::make_response(StatusCode::NOT_FOUND, format!("{} NOT FOUND", uri)),
            }
        })
    }
}
