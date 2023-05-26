use std::{collections::HashMap, net::SocketAddr, sync::Arc};

use log::{info, warn};
use tokio::{
    net::{TcpListener, TcpStream},
    sync::Mutex,
};
use tokio_rustls::TlsConnector;

use crate::{
    router::{new_message_err, Handler, Router},
    wrapper::{wrap_split_tcp_w, wrap_split_tls_w},
};

pub struct ServerState {
    pub name: String,
    pub stopper: String,
    pub stopping: bool,
}

impl ServerState {
    pub fn new(name: String, stopper: String) -> Self {
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

pub struct Proxy {
    pub name: String,
    pub router: Arc<Mutex<Router>>,
    pub config: Option<Arc<rustls::ClientConfig>>,
    listener: HashMap<String, Arc<Mutex<ServerState>>>,
    waiter: Arc<wg::AsyncWaitGroup>,
}

impl Proxy {
    pub fn new(name: String, handler: Arc<dyn Handler + Send + Sync>) -> Self {
        let router = Arc::new(Mutex::new(Router::new(name.clone(), handler)));
        let waiter = Arc::new(wg::AsyncWaitGroup::new());
        waiter.add(1);
        Self { name, router, config: None, listener: HashMap::new(), waiter }
    }

    pub async fn login(&mut self, remote: Arc<String>, options: &String) -> tokio::io::Result<()> {
        if remote.starts_with("tcp://") {
            let stream = TcpStream::connect(remote.trim_start_matches("tcp://")).await?;
            let (rx, tx) = wrap_split_tcp_w(stream);
            self.router.lock().await.join_base(rx, tx, remote, options).await?;
            Ok(())
        } else if remote.starts_with("tls://") {
            if let Some(tls) = &self.config {
                let domain = remote.trim_start_matches("tcp://");
                let connector = TlsConnector::from(tls.clone());
                let stream = TcpStream::connect(domain).await?;
                let domain = rustls::ServerName::try_from(domain).map_err(|_| new_message_err("invalid domain"))?;
                let stream = connector.connect(domain, stream).await?;
                let (rx, tx) = wrap_split_tls_w(stream);
                self.router.lock().await.join_base(rx, tx, remote, options).await?;
                Ok(())
            } else {
                Err(new_message_err("not tls client config"))
            }
        } else {
            Err(new_message_err("not tls client config"))
        }
    }

    pub async fn start_forward(&mut self, name: String, loc: &String, remote: Arc<String>) -> tokio::io::Result<()> {
        if loc.starts_with("socks://") {
            let domain: &str = loc.trim_start_matches("socks://");
            let ln = TcpListener::bind(&domain).await?;
            let state = Arc::new(Mutex::new(ServerState::new(name.clone(), domain.to_string())));
            info!("Proxy({}) listen socks {} is success", self.name, ln.local_addr().unwrap());
            self.listener.insert(name.clone(), state.clone());
            let name = self.name.clone();
            let waiter = self.waiter.clone();
            let router = self.router.clone();
            waiter.add(1);
            tokio::spawn(async move { Self::loop_socks_accpet(name, waiter, ln, state, router, remote).await });
        } else {
            let domain: &str = loc.trim_start_matches("tcp://");
            let ln = TcpListener::bind(&domain).await?;
            let state = Arc::new(Mutex::new(ServerState::new(name.clone(), domain.to_string())));
            info!("Proxy({}) listen tcp {} is success", self.name, ln.local_addr().unwrap());
            self.listener.insert(name.clone(), state.clone());
            let name = self.name.clone();
            let waiter = self.waiter.clone();
            let router = self.router.clone();
            waiter.add(1);
            tokio::spawn(async move { Self::loop_tcp_accpet(name, waiter, ln, state, router, remote).await });
        }
        Ok(())
    }

    async fn loop_tcp_accpet(name: String, waiter: Arc<wg::AsyncWaitGroup>, ln: TcpListener, state: Arc<Mutex<ServerState>>, router: Arc<Mutex<Router>>, remote: Arc<String>) -> tokio::io::Result<()> {
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

    async fn proc_tcp_conn(name: &String, router: Arc<Mutex<Router>>, stream: TcpStream, from: SocketAddr, remote: Arc<String>) {
        info!("Proxy({}) start forward tcp conn {:?} to {:?}", name, from, &remote);
        let (reader, writer) = wrap_split_tcp_w(stream);
        match router.lock().await.dial_base(reader, writer, remote.clone()).await {
            Ok(_) => (),
            Err(e) => {
                info!("Proxy({}) forward tcp conn {:?} to {:?} fail with {:?}", name, from, &remote, e);
            }
        }
    }

    async fn loop_socks_accpet(name: String, waiter: Arc<wg::AsyncWaitGroup>, ln: TcpListener, state: Arc<Mutex<ServerState>>, router: Arc<Mutex<Router>>, remote: Arc<String>) -> tokio::io::Result<()> {
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
                    })
                    .await;
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

    async fn proc_socks_conn(name: String, router: Arc<Mutex<Router>>, stream: TcpStream, from: SocketAddr, remote: Arc<String>) {
        info!("Proxy({}) start forward socks conn {:?} to {:?}", name, from, &remote);
        let (reader, writer) = wrap_split_tcp_w(stream);
        match Router::dial_socks(router, reader, writer, remote.clone()).await {
            Ok(_) => (),
            Err(e) => {
                info!("Proxy({}) forward socks conn {:?} to {:?} fail with {:?}", name, from, &remote, e);
            }
        }
    }

    pub async fn wait(&self) {
        let waiter = self.router.lock().await.waiter().await;
        waiter.wait().await;
        self.waiter.clone().wait().await;
    }

    pub async fn shutdown(&mut self) -> Arc<wg::AsyncWaitGroup> {
        info!("Proxy({}) is stopping", self.name);
        for server in self.listener.values() {
            let mut server = server.lock().await;
            info!("Proxy({}) listener {} is stopping", self.name, server.to_string());
            server.stop().await;
        }
        self.router.lock().await.shutdown().await;
        self.waiter.done();
        self.waiter.clone()
    }
}

// #[async_trait]
// impl Handler for Proxy {
//     //on connection dial uri
//     async fn on_conn_dial_uri(&self, channel: &RouterConn, conn: &String, parts: &Vec<String>) -> tokio::io::Result<()> {
//         self.handler.on_conn_dial_uri(channel, conn, parts).await
//     }
//     //on connection login
//     async fn on_conn_login(&self, channel: &RouterConn, args: &String) -> tokio::io::Result<(String, String)> {
//         self.handler.on_conn_login(channel, args).await
//     }
//     //on connection close
//     async fn on_conn_close(&self, conn: &RouterConn) {
//         self.handler.on_conn_close(conn).await
//     }
//     //OnConnJoin is event on channel join
//     async fn on_conn_join(&self, conn: &RouterConn, option: &String, result: &String) {
//         self.handler.on_conn_join(conn, option, result).await
//     }
// }
