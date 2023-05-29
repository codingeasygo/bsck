use std::sync::Arc;

use router::{log::init_simple_log, proxy::Proxy, router::NormalAcessHandler, util::JSON};

#[tokio::main(flavor = "multi_thread", worker_threads = 3)]
async fn main() {
    init_simple_log().unwrap();
    let name = Arc::new(String::from("NX"));
    let mut options = JSON::new();
    options.insert(String::from("name"), serde_json::Value::String(String::from("NX")));
    options.insert(String::from("token"), serde_json::Value::String(String::from("123")));
    options.insert(String::from("remote"), serde_json::Value::String(String::from("tcp://127.0.0.1:13100")));
    let options = Arc::new(options);
    let socks_dial_uri = Arc::new(String::from("N0->${HOST}"));
    let tcp_dial_uri = Arc::new(String::from("N0->tcp://127.0.0.1:13200"));
    let socks_addr = String::from("socks://127.0.0.1:1107");
    let tcp_addr = String::from("tcp://127.0.0.1:13300");
    let web_addr = String::from("tcp://127.0.0.1:1100");
    let handler = Arc::new(NormalAcessHandler::new());
    let mut proxy = Proxy::new(name, handler);
    for _ in 0..32 {
        proxy.login(options.clone()).await.unwrap();
    }
    proxy.start_forward(Arc::new(String::from("s01")), &socks_addr, socks_dial_uri).await.unwrap();
    proxy.start_forward(Arc::new(String::from("t01")), &tcp_addr, tcp_dial_uri).await.unwrap();
    proxy.start_web(Arc::new(String::from("web")), &web_addr).await.unwrap();
    proxy.wait().await;
}
