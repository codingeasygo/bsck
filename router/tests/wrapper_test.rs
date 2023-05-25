#[cfg(test)]
mod tests {

    use log::info;
    use router::{log::init_simple_log, wrapper::wrap_channel};

    #[tokio::test]
    async fn test_wrap_channel() {
        init_simple_log().unwrap();
        info!("{}", "test_wrap_channel");
        let mut handles = vec![];
        let (mut rx, mut tx) = wrap_channel();
        handles.push(tokio::spawn(async move {
            for _ in 0..10 {
                _ = tx.write(String::from("abc").as_bytes()).await;
            }
        }));
        let mut buf = [0; 1024];
        for _ in 0..10 {
            let n = rx.read(&mut buf).await.unwrap();
            assert_eq!(n, 3);
            assert_eq!(&buf[0..3], String::from("abc").as_bytes());
        }
        futures::future::join_all(handles).await;
    }

    // #[tokio::test]
    // async fn test_arc_lock() {
    //     let mut handles = vec![];
    //     let forward = Arc::new(RefCell::new(String::from("message")));
    //     let read_runner = Arc::clone(&forward);
    //     handles.push(tokio::spawn(async move {
    //         let runner = read_runner.read().unwrap();
    //         println!("read runner is starting {}", runner);
    //     }));
    //     let proxy_runner = Arc::clone(&forward);
    //     handles.push(tokio::spawn(async move {
    //         let runner = proxy_runner.read().unwrap();
    //         println!("proxy runner is starting {}", runner);
    //     }));
    //     futures::future::join_all(handles).await;
    // }
}
