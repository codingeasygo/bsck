use log::debug;
use router::frame::{RawReader, RawWriter};
use router::log::init_simple_log;
use router::relay::{self, RawDevice};
use smoltcp::iface::{Config, Interface, SocketSet};
use smoltcp::socket::{tcp, udp, Socket};
use smoltcp::time::Instant;
use smoltcp::wire::{IpAddress, IpCidr};
use std::cell::RefCell;
use std::io::Read;
use std::os::fd::AsRawFd;
use std::rc::Rc;
use std::sync::Arc;

// #[tokio::main(flavor = "multi_thread", worker_threads = 3)]
// async fn main() {
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

fn main() {
    let mut tun_config = tun::Configuration::default();
    tun_config.address((10, 0, 0, 3)).netmask((255, 255, 255, 0)).up();
    let dev = tun::create(&tun_config).unwrap();
    dev.set_nonblock().unwrap();
    let fd = dev.as_raw_fd();
    println!("---->{}", fd);
    // dev.set_nonblock().unwrap();
    let (reader, writer) = dev.split();
    let reader = Rc::new(RefCell::new(reader));
    let writer = Arc::new(std::sync::Mutex::new(writer));
    let mut device = RawDevice::new(1900, smoltcp::phy::Medium::Ip, reader, writer);
    // Create interface
    let mut config = Config::new(smoltcp::wire::HardwareAddress::Ip);
    config.random_seed = rand::random();

    let mut iface = Interface::new(config, &mut device);
    // iface.update_ip_addrs(|ip_addrs| {
    //     ip_addrs.push(IpCidr::new(IpAddress::v4(10, 0, 0, 1), 24)).unwrap();
    // });
    iface.set_any_ip(true);

    let mut sockets = SocketSet::new(vec![]);
    // let udp_rx_buffer = udp::PacketBuffer::new(vec![udp::PacketMetadata::EMPTY, udp::PacketMetadata::EMPTY], vec![0; 65535]);
    // let udp_tx_buffer = udp::PacketBuffer::new(vec![udp::PacketMetadata::EMPTY, udp::PacketMetadata::EMPTY], vec![0; 65535]);
    // let udp_socket = udp::Socket::new(udp_rx_buffer, udp_tx_buffer);

    let tcp2_rx_buffer = tcp::SocketBuffer::new(vec![0; 64]);
    let tcp2_tx_buffer = tcp::SocketBuffer::new(vec![0; 128]);
    let tcp2_socket = tcp::Socket::new(tcp2_rx_buffer, tcp2_tx_buffer);
    let tcp2_handle = sockets.add(tcp2_socket);

    let mut tcp_6970_active = false;

    loop {
        let timestamp = Instant::now();
        iface.poll(timestamp, &mut device, &mut sockets);
        for (_, v) in sockets.iter_mut() {
            match v {
                Socket::Udp(v) => {
                    if v.can_recv() {
                        let (data, ep) = v.recv().unwrap();
                        let data = data.to_owned();
                        println!("can_recv--->{:?}", data);
                        if v.can_send() && !data.is_empty() {
                            debug!("tcp:6970 send data: {:?}", data);
                            v.send_slice(&data[..], ep).unwrap();
                        }
                    }
                }
                Socket::Tcp(v) => {
                    if v.may_recv() {
                        let data = v
                            .recv(|buffer| {
                                let recvd_len = buffer.len();
                                let data = buffer.to_owned();
                                (recvd_len, data)
                            })
                            .unwrap();
                        if v.can_send() && !data.is_empty() {
                            debug!("tcp:6970 send data: {:?}", data);
                            v.send_slice(&data[..]).unwrap();
                        }
                    }
                }
                _ => (),
            }
        }
        smoltcp::phy::wait(fd, iface.poll_delay(timestamp, &sockets)).expect("wait error");
        // std::thread::sleep(std::time::Duration::from_millis(10));
    }
}
