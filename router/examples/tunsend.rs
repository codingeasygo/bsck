use std::{
    io::{Read, Write},
    net::{SocketAddr, UdpSocket},
    sync::Arc,
    thread::spawn,
};

extern crate tun;

fn main() {
    let mut config = tun::Configuration::default();
    config.address((10, 1, 0, 1)).netmask((255, 255, 255, 0)).up();

    let dev = tun::create(&config).unwrap();
    let (mut dev_reader, mut dev_writer) = dev.split();

    let gw = Arc::new(UdpSocket::bind("127.0.0.1:6235").unwrap());
    let gw_addr: SocketAddr = "127.0.0.1:6238".parse().unwrap();
    let gw_read = gw.clone();
    spawn(move || {
        let mut buf = [0; 4096];
        buf[3] = 2;
        loop {
            let (n, _) = gw_read.recv_from(&mut buf[4..]).unwrap();
            println!("R==>{}", n);
            _ = dev_writer.write_all(&buf[0..n + 4]);
        }
    });
    let mut buf = [0; 4096];
    loop {
        let n = dev_reader.read(&mut buf).unwrap();
        println!("S==>{:?}", &buf[..n]);
        _ = gw.send_to(&buf[4..n], gw_addr);
    }
}
