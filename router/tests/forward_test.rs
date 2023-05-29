#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use router::{
        log::init_simple_log,
        proxy::Proxy,
        router::{NormalAcessHandler, Router},
        util::JSON,
        wrapper::{wrap_channel, wrap_split_tcp_w},
    };
    use tokio::net::TcpStream;

    #[tokio::test]
    async fn base_router() {
        init_simple_log().unwrap();
        let mut options = JSON::new();
        options.insert(String::from("name"), serde_json::Value::String(String::from("NX")));
        options.insert(String::from("token"), serde_json::Value::String(String::from("123")));
        // let join_uri = Arc::new(String::from("piped"));
        let dial_uri = Arc::new(String::from("N0->tcp://127.0.0.1:13200"));

        let handler = Arc::new(NormalAcessHandler::new());
        let router = Router::new(Arc::new(String::from("NX")), handler);
        let stream = TcpStream::connect("127.0.0.1:13100").await.unwrap();
        let (rx, tx) = wrap_split_tcp_w(stream);
        let res = router.join_base(rx, tx, Arc::new(options)).await;
        assert!(res.is_ok(), "{:?}", res);
        // tokio::time::sleep(Duration::from_millis(1000000)).await;
        for i in 0..10 {
            let (rxb, mut txa) = wrap_channel();
            let (mut rxa, txb) = wrap_channel();
            let conn = router.dial_base(rxb, txb, dial_uri.clone()).await.unwrap();
            conn.wait().await.unwrap();
            println!("forward is started");
            let data = format!("123-{}", i);
            match txa.write(data.as_bytes()).await {
                Ok(n) => assert_eq!(n, data.len()),
                Err(e) => assert!(false, "{}", e),
            }
            println!("forward is writed");
            let mut buf = [0; 1024];
            match rxa.read(&mut buf).await {
                Ok(n) => {
                    assert_eq!(n, data.len());
                    assert_eq!(buf[0..n].to_vec(), data.as_bytes())
                }
                Err(e) => assert!(false, "{}", e),
            }
            txa.shutdown().await;
        }
        router.shutdown().await;
        // tokio::time::sleep(tokio::time::Duration::from_millis(1000000)).await;
    }

    #[tokio::test]
    async fn proxy_tcp() {
        init_simple_log().unwrap();
        let mut options = JSON::new();
        options.insert(String::from("name"), serde_json::Value::String(String::from("NX")));
        options.insert(String::from("token"), serde_json::Value::String(String::from("123")));
        options.insert(String::from("remote"), serde_json::Value::String(String::from("tcp://127.0.0.1:13100")));
        let dial_uri = Arc::new(String::from("N0->tcp://127.0.0.1:13200"));
        let addr = String::from("tcp://127.0.0.1:1107");
        let handler = Arc::new(NormalAcessHandler::new());
        let mut proxy = Proxy::new(Arc::new(String::from("NX")), handler);
        proxy.login(Arc::new(options)).await.unwrap();
        proxy.start_forward(Arc::new(String::from("test")), &addr, dial_uri).await.unwrap();
        tokio::time::sleep(tokio::time::Duration::from_millis(1000000)).await;
        // proxy.start_forward(loc, remote)
    }

    #[tokio::test]
    async fn proxy_socks() {
        init_simple_log().unwrap();
        let mut options = JSON::new();
        options.insert(String::from("name"), serde_json::Value::String(String::from("NX")));
        options.insert(String::from("token"), serde_json::Value::String(String::from("123")));
        options.insert(String::from("remote"), serde_json::Value::String(String::from("tcp://127.0.0.1:13100")));
        let dial_uri = Arc::new(String::from("N0->${HOST}"));
        let forward_addr = String::from("socks://127.0.0.1:1107");
        let web_addr = String::from("127.0.0.1:1100");
        let handler = Arc::new(NormalAcessHandler::new());
        let mut proxy = Proxy::new(Arc::new(String::from("NX")), handler);
        proxy.login(Arc::new(options)).await.unwrap();
        proxy.start_forward(Arc::new(String::from("f1")), &forward_addr, dial_uri).await.unwrap();
        proxy.start_web(Arc::new(String::from("w1")), &web_addr).await.unwrap();
        tokio::time::sleep(tokio::time::Duration::from_millis(8000)).await;
        proxy.shutdown().await;
        proxy.wait().await;
        println!("shutdown is done...");
    }
}
