use std::sync::Arc;

use router::{log::init_simple_log, proxy::Proxy, router::NormalAcessHandler, util::JSON};
use serde_json::json;
use tokio::sync::mpsc;

#[tokio::main(flavor = "multi_thread", worker_threads = 3)]
async fn main() {
    init_simple_log(0).unwrap();
    let name = Arc::new(String::from("NX"));
    let mut options = JSON::new();
    options.insert(String::from("name"), serde_json::Value::String(String::from("NX")));
    options.insert(String::from("token"), serde_json::Value::String(String::from("123")));
    options.insert(String::from("remote"), serde_json::Value::String(String::from("quic://127.0.0.1:13100,tcp://127.0.0.1:13100")));
    options.insert(String::from("domain"), serde_json::Value::String(String::from("test.loc")));
    options.insert(String::from("tls_ca"), json!("certs/rootCA.crt"));
    options.insert(String::from("keep"), json!(10));
    let options = Arc::new(options);
    let socks_dial_uri = Arc::new(String::from("N0->${HOST}"));
    let gw_dial_uri = Arc::new(String::from("N0->${HOST}"));
    let tcp_dial_uri = Arc::new(String::from("N0->tcp://127.0.0.1:13200"));
    let socks_addr = String::from("socks://127.0.0.1:1107");
    let gw_addr = String::from("gw://0.0.0.0:6238>192.168.1.103:6238");
    let tcp_addr = String::from("tcp://127.0.0.1:13300");
    let web_addr = String::from("tcp://127.0.0.1:1100");
    let handler = Arc::new(NormalAcessHandler::new());
    let mut proxy = Proxy::new(name, handler);
    proxy.channels.insert(String::from("N0"), options);
    _ = proxy.keep().await;
    proxy.start_forward(Arc::new(String::from("s01")), &socks_addr, socks_dial_uri).await.unwrap();
    proxy.start_forward(Arc::new(String::from("g01")), &gw_addr, gw_dial_uri).await.unwrap();
    proxy.start_forward(Arc::new(String::from("t01")), &tcp_addr, tcp_dial_uri).await.unwrap();
    proxy.start_web(Arc::new(String::from("web")), &web_addr).await.unwrap();
    let (stopper, receive) = mpsc::channel(8);
    proxy.run(receive).await;
    _ = stopper.send(1).await;
}
