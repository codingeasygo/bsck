use std::sync::Arc;

use json::object;
use router::{log::init_simple_log, proxy::Proxy, router::NormalAcessHandler};

#[tokio::main(flavor = "multi_thread", worker_threads = 3)]
async fn main() {
    init_simple_log().unwrap();
    let login_optionslet = object! {
        name: "NX",
        token:"123",
    };
    let join_uri = Arc::new(String::from("tcp://127.0.0.1:13100"));
    let socks_dial_uri = Arc::new(String::from("N0->${HOST}"));
    let tcp_dial_uri = Arc::new(String::from("N0->tcp://127.0.0.1:13200"));
    let socks_addr = String::from("socks://127.0.0.1:1107");
    let tcp_addr = String::from("tcp://127.0.0.1:13300");
    let web_addr = String::from("tcp://127.0.0.1:1100");
    let handler = Arc::new(NormalAcessHandler::new());
    let mut proxy = Proxy::new(String::from("NX"), handler);
    proxy.login(join_uri, &login_optionslet.dump()).await.unwrap();
    proxy.start_forward(String::from("s01"), &socks_addr, socks_dial_uri).await.unwrap();
    proxy.start_forward(String::from("t01"), &tcp_addr, tcp_dial_uri).await.unwrap();
    proxy.start_web(String::from("web"), &web_addr).await.unwrap();
    proxy.wait().await;
}
