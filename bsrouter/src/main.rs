use router::frame::{RawReader, RawWriter};
use router::log::init_simple_log;
use router::relay::{self, CacheDevice};
use serde_json::de;
use smoltcp::iface::{Config, Interface};
use std::cell::RefCell;
use std::fs::read;
use std::os::fd::AsRawFd;
use std::rc::Rc;
use std::sync::Arc;
use tokio::io::unix::AsyncFd;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::runtime::Runtime;

#[tokio::main(flavor = "multi_thread", worker_threads = 3)]
async fn main() {
    //     init_simple_log(0).unwrap();

    //     let mut config = tun::Configuration::default();
    //     config.address((10, 0, 0, 3)).netmask((255, 255, 255, 0)).up();
    //     #[cfg(target_os = "linux")]
    //     config.platform(|config| {
    //         config.packet_information(true);
    //     });
    //     let dev = tun::create(&config).unwrap();

    //     let name = Arc::new(String::from("test"));

    //     let mut buf = [0; 4096];

    //     let (mut reader, writer) = dev.split();
    //     // let writer = tokio_fd::AsyncFd::try_from(dev.as_raw_fd()).unwrap();
    //     // let relay = relay::Relay::new(name, Box::new(writer));
    //     // let (mut udg_gw_r, mut udg_gw_w) = relay.start_udp_gw(64).await;
    //     // tokio::spawn(async move {
    //     //     loop {
    //     //         let mut buf = [0; 4096];
    //     //         let readed = udg_gw_r.read(&mut buf).await;
    //     //         if let Ok(n) = readed {
    //     //             _ = udg_gw_w.write(&buf[0..n]).await;
    //     //         } else {
    //     //             break;
    //     //         }
    //     //     }
    //     // });
    //     loop {
    //         let amount = reader.read(&mut buf).unwrap();
    //         let packet = &buf[4..amount];
    //         println!("R  {},{:?}", amount, &buf[0..amount]);
    //         // let result = relay.ingress(packet).await.unwrap();
    //         // if let Some((mut r, mut w)) = result {
    //         //     println!("OPEN {}", r.addr);
    //         //     tokio::spawn(async move {
    //         //         loop {
    //         //             let mut buf = [0; 4096];
    //         //             let readed = r.read(&mut buf).await;
    //         //             if let Ok(n) = readed {
    //         //                 _ = w.write(&buf[0..n]).await;
    //         //             } else {
    //         //                 break;
    //         //             }
    //         //         }
    //         //     });
    //         // }
    //     }

    //     // let xx = router::tun::Relay::new();
    //     // xx.ingress(&vec![0; 100]);
    //     // let name = Arc::new(String::from("NX"));
    //     // let mut options = JSON::new();
    //     // options.insert(String::from("name"), serde_json::Value::String(String::from("NX")));
    //     // options.insert(String::from("token"), serde_json::Value::String(String::from("123")));
    //     // options.insert(String::from("remote"), serde_json::Value::String(String::from("quic://127.0.0.1:13100,tcp://127.0.0.1:13100")));
    //     // options.insert(String::from("domain"), serde_json::Value::String(String::from("test.loc")));
    //     // options.insert(String::from("tls_ca"), json!("certs/rootCA.crt"));
    //     // options.insert(String::from("keep"), json!(10));
    //     // let options = Arc::new(options);
    //     // let socks_dial_uri = Arc::new(String::from("N0->${HOST}"));
    //     // let tcp_dial_uri = Arc::new(String::from("N0->tcp://127.0.0.1:13200"));
    //     // let socks_addr = String::from("socks://127.0.0.1:1107");
    //     // let tcp_addr = String::from("tcp://127.0.0.1:13300");
    //     // let web_addr = String::from("tcp://127.0.0.1:1100");
    //     // let handler = Arc::new(NormalAcessHandler::new());
    //     // let mut proxy = Proxy::new(name, handler);
    //     // proxy.channels.insert(String::from("N0"), options);
    //     // proxy.start_forward(Arc::new(String::from("s01")), &socks_addr, socks_dial_uri).await.unwrap();
    //     // proxy.start_forward(Arc::new(String::from("t01")), &tcp_addr, tcp_dial_uri).await.unwrap();
    //     // proxy.start_web(Arc::new(String::from("web")), &web_addr).await.unwrap();
    //     // proxy.run().await;
    // }

    let mut tun_config = tun::Configuration::default();
    tun_config.address((10, 0, 0, 3)).netmask((255, 255, 255, 0)).up();
    let dev = tun::create(&tun_config).unwrap();
    dev.set_nonblock().unwrap();
    let fd = dev.as_raw_fd();
    println!("---->{}", fd);

    let dev = tokio_fd::AsyncFd::try_from(fd).unwrap();
    let mut dev = Box::new(dev);
    // let writer = tokio_fd::AsyncFd::try_from(fd).unwrap();
    // let reader = Rc::new(RefCell::new(reader));
    // let writer = Arc::new(std::sync::Mutex::new(writer));
    let mut device = CacheDevice::new(1500, smoltcp::phy::Medium::Ip);
    // // Create interface
    let mut config = Config::new(smoltcp::wire::HardwareAddress::Ip);
    config.random_seed = rand::random();

    let mut iface = Interface::new(config, &mut device);
    // iface.update_ip_addrs(|ip_addrs| {
    //     ip_addrs.push(IpCidr::new(IpAddress::v4(10, 0, 0, 1), 24)).unwrap();
    // });
    iface.set_any_ip(true);
    let name = Arc::new(String::from("test"));
    let (relay, tcp_gw, udp_gw) = relay::Relay::new(name, iface, device);
    tokio::spawn(async move { conn_echo(udp_gw).await });
    tokio::spawn(async move {
        let mut gw = tcp_gw;
        loop {
            match gw.accept().await {
                Some(c) => {
                    let target = c.target.clone();
                    println!("conn is start {}", target);
                    tokio::spawn(async move {
                        conn_echo(c).await;
                        println!("conn is done {}", target);
                    });
                }
                None => (),
            };
        }
    });
    let mut relay = Box::new(relay);
    loop {
        // match relay.as_mut().poll(&mut dev).await {
        //     Ok(_) => (),
        //     Err(e) => (),
        // };
        // smoltcp::phy::wait(fd, relay.poll_delay()).expect("wait error");
        let relay = relay.as_mut();
        relay.poll(&mut dev).await.unwrap();
    }
}

pub async fn conn_echo(c: relay::Conn) {
    let mut c = c;
    let mut buf = vec![0u8; 2048];
    loop {
        let n = match c.reader.read(&mut buf).await {
            Ok(v) => v,
            Err(e) => {
                break;
            }
        };
        // println!("ar-->{}", n);
        match c.writer.write(&buf[0..n]).await {
            Ok(_) => (),
            Err(e) => {
                break;
            }
        }
        // println!("aw-->{}", n);
    }
}
