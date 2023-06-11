use std::collections::HashMap;
use std::ffi::CStr;
use std::os::raw::{c_char, c_int};
use std::sync::Arc;

use router::proxy::Proxy;
use router::router::NormalAcessHandler;
use serde_json::Value;
use tokio::sync::mpsc;

static mut G_ROUTER: Option<Proxy> = None;

#[no_mangle]
pub extern "C" fn router_start(config: *const c_char) -> c_int {
    let config_str = unsafe { CStr::from_ptr(config) };
    let config_str = match config_str.to_str() {
        Ok(v) => v.to_string(),
        Err(e) => {
            println!("Router start fail with {:?}", e);
            return -1;
        }
    };
    let config: HashMap<String, Value> = match serde_json::from_str(&config_str) {
        Ok(v) => v,
        Err(e) => {
            println!("Router start fail with {:?}", e);
            return -1;
        }
    };
    tokio::spawn(async move {
        let handler = Arc::new(NormalAcessHandler::new());
        match Proxy::bootstrap(handler, Arc::new(config)).await {
            Ok(proxy) => unsafe {
                G_ROUTER = Some(proxy);
                let (stopper, receive) = mpsc::channel(8);
                G_ROUTER.as_mut().unwrap().run(receive).await;
                _ = stopper.send(1).await;
            },
            Err(e) => {
                println!("Router is stopped by {:?}", e);
                return;
            }
        }
    });
    0
}

#[no_mangle]
pub extern "C" fn router_stop() {
    tokio::spawn(async move {
        unsafe {
            if let Some(proxy) = &mut G_ROUTER {
                proxy.shutdown().await;
            }
            G_ROUTER = None;
        }
    });
}
