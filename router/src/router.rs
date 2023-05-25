use async_trait::async_trait;
use futures::lock::Mutex;
use log::{debug, info, warn};
use std::{
    collections::HashMap,
    io::ErrorKind,
    ptr::eq,
    sync::Arc,
    time::{SystemTime, UNIX_EPOCH},
};
use tokio::{sync::Notify, task::JoinHandle};

use crate::frame::{FrameReader, FrameWriter, Header, Reader, Writer};

const CONN_OK: &str = "OK";
const CONN_CLOSED: &str = "CLOSED";

fn new_message_err<E>(err: E) -> std::io::Error
where
    E: Into<Box<dyn std::error::Error + Send + Sync>>,
{
    std::io::Error::new(ErrorKind::Other, err)
}

fn json_must_i32(value: &json::JsonValue, key: &str) -> tokio::io::Result<i32> {
    match value[key].as_i32() {
        Some(v) => Ok(v),
        None => Err(new_message_err(format!("read {} fail", key))),
    }
}

fn json_must_str<'a>(value: &'a json::JsonValue, key: &'a str) -> tokio::io::Result<&'a str> {
    match value[key].as_str() {
        Some(v) => Ok(v),
        None => Err(new_message_err(format!("read {} fail", key))),
    }
}

#[derive(Clone, Debug)]
pub enum RouterCmd {
    None = 0,
    LoginChannel = 10,
    LoginBack = 11,
    PingConn = 20,
    PingBack = 21,
    DialConn = 100,
    DialBack = 101,
    ConnData = 110,
    ConnClosed = 120,
}

impl RouterCmd {
    pub fn as_cmd(cmd: &u8) -> RouterCmd {
        match cmd {
            10 => RouterCmd::LoginChannel,
            11 => RouterCmd::LoginBack,
            20 => RouterCmd::PingConn,
            21 => RouterCmd::PingBack,
            100 => RouterCmd::DialConn,
            101 => RouterCmd::DialBack,
            110 => RouterCmd::ConnData,
            120 => RouterCmd::ConnClosed,
            _ => RouterCmd::None,
        }
    }
    pub fn as_u8(&self) -> u8 {
        match self {
            RouterCmd::None => 0,
            RouterCmd::LoginChannel => 10,
            RouterCmd::LoginBack => 11,
            RouterCmd::PingConn => 20,
            RouterCmd::PingBack => 21,
            RouterCmd::DialConn => 100,
            RouterCmd::DialBack => 101,
            RouterCmd::ConnData => 110,
            RouterCmd::ConnClosed => 120,
        }
    }
}

#[derive(Clone)]
pub struct ConnID {
    pub lid: u8,
    pub rid: u8,
}

impl ConnID {
    pub fn new(lid: u8, rid: u8) -> Self {
        Self { lid, rid }
    }

    pub fn zero() -> Self {
        Self { lid: 0, rid: 0 }
    }

    pub fn to_string(&self) -> String {
        hex::encode([self.lid, self.rid])
    }
}

pub struct RouterFrame<'a> {
    pub buf: &'a mut [u8],
    pub sid: ConnID,
    pub cmd: RouterCmd,
    pub data_offset: usize,
}

impl<'a> RouterFrame<'a> {
    //parse remote frame, lid is in 0, rid is in 1
    pub fn prase_local(header: &Arc<Header>, buf: &'a mut [u8]) -> Self {
        let data_offset = header.data_offset;
        let sid = ConnID::new(buf[data_offset], buf[data_offset + 1]);
        let cmd = RouterCmd::as_cmd(&buf[data_offset + 2]);
        RouterFrame { buf, sid, cmd, data_offset }
    }

    //parse remote frame, lid is in 1, rid is in 0
    pub fn prase_remote(header: &Arc<Header>, buf: &'a mut [u8]) -> Self {
        let data_offset = header.data_offset;
        let sid = ConnID::new(buf[data_offset + 1], buf[data_offset]);
        let cmd = RouterCmd::as_cmd(&buf[data_offset + 2]);
        RouterFrame { buf, sid, cmd, data_offset }
    }

    pub fn new(header: &Arc<Header>, buf: &'a mut [u8], sid: &ConnID, cmd: &RouterCmd) -> Self {
        let data_offset = header.data_offset;
        header.write_head(buf);
        buf[data_offset] = sid.lid.clone();
        buf[data_offset + 1] = sid.rid.clone();
        buf[data_offset + 2] = cmd.as_u8();
        RouterFrame { buf, sid: sid.clone(), cmd: cmd.clone(), data_offset }
    }

    pub fn make_data_prefix(sid: &ConnID, cmd: &RouterCmd) -> Vec<u8> {
        let mut prefix = vec![0; 3];
        prefix[0] = sid.lid;
        prefix[1] = sid.lid;
        prefix[2] = cmd.as_u8();
        prefix
    }

    pub fn data(&self) -> &[u8] {
        &self.buf[self.data_offset + 3..]
    }

    pub fn str(&self) -> String {
        let data = self.data();
        match String::from_utf8(data.to_vec()) {
            Ok(v) => v,
            Err(_) => hex::encode(data),
        }
    }

    pub fn update_sid(&mut self, sid: ConnID) {
        self.buf[self.data_offset] = sid.lid.clone();
        self.buf[self.data_offset + 1] = sid.rid.clone();
        self.sid = sid;
    }

    pub fn update_cmd(&mut self, cmd: RouterCmd) {
        self.buf[self.data_offset + 2] = cmd.as_u8();
        self.cmd = cmd;
    }

    pub fn to_string(&self) -> String {
        if self.buf.len() > 32 {
            format!("RouterFrame({},{},{:?},{})", self.buf.len(), self.sid.to_string(), self.cmd, hex::encode(&self.buf[..32]))
        } else {
            format!("RouterFrame({},{},{:?},{})", self.buf.len(), self.sid.to_string(), self.cmd, hex::encode(&self.buf))
        }
    }
}

pub struct Ping {
    pub speed: i64,
    pub last: i64,
}

impl Ping {
    pub fn new() -> Self {
        Self { speed: 0, last: 0 }
    }
}

#[derive(Clone, PartialEq, Debug)]
pub enum ConnType {
    Channel,
    Raw,
}

#[async_trait]
pub trait RouterReader {
    async fn read(&mut self) -> tokio::io::Result<RouterFrame>;
    async fn set_data_prefix(&mut self, prefix: Vec<u8>);
    //wait ready
    async fn wait_ready(&self) -> tokio::io::Result<()>;
}

#[async_trait]
pub trait RouterWriter {
    async fn write(&mut self, frame: &RouterFrame<'_>) -> tokio::io::Result<usize>;
    async fn shutdown(&mut self);
    //wait ready
    async fn wait_ready(&self) -> tokio::io::Result<()>;
}

pub struct Conn {
    pub id: u16,
    pub name: String,
    pub ping: Ping,
    pub recv_last: i64,
    pub conn_type: ConnType,
    pub context: HashMap<String, String>,
    pub conn_seq: u8,
    pub conn_id: HashMap<u8, bool>,
    pub state: String,
    pub used: u64,
    pub ready: Arc<Notify>,
    pub loc: String,
    pub uri: String,
}

impl Conn {
    pub fn new(id: u16, conn_type: ConnType) -> Self {
        Self {
            id,
            name: String::from(""),
            ping: Ping::new(),
            recv_last: 0,
            conn_type,
            context: HashMap::new(),
            conn_seq: 0,
            conn_id: HashMap::new(),
            state: String::from(""),
            used: 0,
            ready: Arc::new(Notify::new()),
            loc: String::from(""),
            uri: String::from(""),
        }
    }

    pub fn alloc_conn_id(&mut self) -> u8 {
        for _ in 0..257 {
            self.conn_seq += 1;
            if self.conn_seq > 0 && !self.conn_id.contains_key(&self.conn_seq) {
                self.conn_id.insert(self.conn_seq, true);
                return self.conn_seq;
            }
        }
        0
    }

    pub fn free_conn_id(&mut self, conn_id: &u8) {
        if self.conn_id.contains_key(&conn_id) {
            self.conn_id.remove(conn_id);
        }
    }

    pub fn used_conn_id(&self) -> usize {
        self.conn_id.len()
    }

    //make ready
    pub fn make_ready(&mut self, err: String) {
        self.state = err;
        self.ready.notify_waiters();
    }

    pub fn ready_waiter(&self) -> Arc<Notify> {
        self.ready.clone()
    }
}

pub struct RouterConnReader {
    pub header: Arc<Header>,
    pub inner: FrameReader,
    pub conn: Arc<Mutex<Conn>>,
    ready: Arc<Notify>,
}

impl RouterConnReader {
    pub async fn new(header: Arc<Header>, inner: FrameReader, conn: Arc<Mutex<Conn>>) -> Self {
        let ready = conn.lock().await.ready_waiter();
        Self { header, inner, conn, ready }
    }
}

#[async_trait]
impl RouterReader for RouterConnReader {
    async fn read(&mut self) -> tokio::io::Result<RouterFrame> {
        let buf = self.inner.read().await?;
        Ok(RouterFrame::prase_remote(&self.header, buf))
    }

    async fn set_data_prefix(&mut self, _: Vec<u8>) {}

    async fn wait_ready(&self) -> tokio::io::Result<()> {
        self.ready.notified().await;
        Ok(())
    }
}

pub struct RouterConnWriter {
    pub header: Arc<Header>,
    pub inner: FrameWriter,
    pub conn: Arc<Mutex<Conn>>,
    ready: Arc<Notify>,
}

impl RouterConnWriter {
    pub async fn new(header: Arc<Header>, inner: FrameWriter, conn: Arc<Mutex<Conn>>) -> Self {
        let ready = conn.lock().await.ready_waiter();
        Self { header, inner, conn, ready }
    }
}

#[async_trait]
impl RouterWriter for RouterConnWriter {
    async fn write(&mut self, frame: &RouterFrame<'_>) -> tokio::io::Result<usize> {
        let n = self.inner.write(frame.buf).await?;
        Ok(n)
    }

    async fn shutdown(&mut self) {
        _ = self.inner.shutdown().await
    }

    async fn wait_ready(&self) -> tokio::io::Result<()> {
        self.ready.notified().await;
        Ok(())
    }
}

pub struct RouterRawReader {
    pub header: Arc<Header>,
    pub inner: Box<dyn Reader + Send + Sync>,
    pub conn: Arc<Mutex<Conn>>,
    ready: Arc<Notify>,
    prefix: Vec<u8>,
    buf: Box<Vec<u8>>,
}

impl RouterRawReader {
    pub async fn new(header: Arc<Header>, inner: Box<dyn Reader + Send + Sync>, conn: Arc<Mutex<Conn>>, buffer_size: usize) -> Self {
        let ready = conn.lock().await.ready_waiter();
        Self { header, inner, conn, ready, buf: Box::new(vec![0; buffer_size]), prefix: Vec::new() }
    }

    // pub fn to_string(&self) -> String {
    //     let lid = self.id;
    //     let rid = self.get_rid();
    //     let loc = &self.loc;
    //     let uri = &self.uri;
    //     String::from(format!("RawReader({}<->{},{}<->{})", lid, rid, loc, uri))
    // }
}

#[async_trait]
impl RouterReader for RouterRawReader {
    async fn read(&mut self) -> tokio::io::Result<RouterFrame> {
        let s = &mut *self;
        let header = &s.header;
        let o = s.header.data_offset + s.prefix.len();
        let sbuf = s.buf.as_mut();
        let mut n = s.inner.read(&mut sbuf[o..]).await?;
        if n < 1 {
            return Err(std::io::Error::new(ErrorKind::UnexpectedEof, "EOF"));
        }
        n += o;
        let data = &mut sbuf[..n];
        let frame = RouterFrame::prase_remote(header, data);
        Ok(frame)
    }

    async fn set_data_prefix(&mut self, prefix: Vec<u8>) {
        if prefix.len() > 0 {
            self.buf[self.header.data_offset..self.header.data_offset + prefix.len()].copy_from_slice(&prefix);
        }
        self.prefix = prefix;
    }

    async fn wait_ready(&self) -> tokio::io::Result<()> {
        self.ready.notified().await;
        Ok(())
    }
}

pub struct RouterRawWriter {
    pub header: Arc<Header>,
    pub inner: Box<dyn Writer + Send + Sync>,
    pub conn: Arc<Mutex<Conn>>,
    ready: Arc<Notify>,
}

impl RouterRawWriter {
    pub async fn new(header: Arc<Header>, inner: Box<dyn Writer + Send + Sync>, conn: Arc<Mutex<Conn>>) -> Self {
        let ready = conn.lock().await.ready_waiter();
        Self { header, inner, conn, ready }
    }
}

#[async_trait]
impl RouterWriter for RouterRawWriter {
    async fn write(&mut self, frame: &RouterFrame<'_>) -> tokio::io::Result<usize> {
        self.inner.write(frame.data()).await
    }

    async fn shutdown(&mut self) {
        _ = self.inner.shutdown();
    }

    async fn wait_ready(&self) -> tokio::io::Result<()> {
        self.ready.notified().await;
        Ok(())
    }
}

pub struct RouterConn {
    pub name: String,
    pub header: Arc<Header>,
    pub id: u16,
    pub conn_type: ConnType,
    pub conn: Arc<Mutex<Conn>>,
    pub reader: Arc<Mutex<dyn RouterReader + Send + Sync>>,
    pub writer: Arc<Mutex<dyn RouterWriter + Send + Sync>>,
    pub forward: Arc<Mutex<RouterForward>>,
}

impl RouterConn {
    pub fn new(name: String, header: Arc<Header>, id: u16, conn_type: ConnType, conn: Arc<Mutex<Conn>>, reader: Arc<Mutex<dyn RouterReader + Send + Sync>>, writer: Arc<Mutex<dyn RouterWriter + Send + Sync>>, forward: Arc<Mutex<RouterForward>>) -> Self {
        Self { name, header, id, conn_type, conn, reader, writer, forward }
    }
    pub async fn name(&self) -> String {
        self.conn.lock().await.name.clone()
    }
    pub async fn set_name(&self, name: String) {
        self.conn.lock().await.name = name;
    }

    pub async fn alloc_conn_id(&self) -> u8 {
        self.conn.lock().await.alloc_conn_id()
    }

    pub async fn free_conn_id(&self, conn_id: &u8) {
        self.conn.lock().await.free_conn_id(conn_id);
    }

    pub async fn set_data_prefix(&self, prefix: Vec<u8>) {
        self.reader.lock().await.set_data_prefix(prefix).await;
    }

    pub async fn add_job(&self) {
        self.forward.lock().await.add_job();
    }

    pub async fn done_job(&self) {
        self.forward.lock().await.done_job();
    }

    pub async fn write(&self, frame: &RouterFrame<'_>) -> tokio::io::Result<usize> {
        let mut writer = self.writer.lock().await;
        let result = writer.write(frame).await;
        if result.is_err() {
            writer.shutdown().await
        }
        result
    }

    pub async fn read_str(&self) -> tokio::io::Result<String> {
        let mut reader = self.reader.lock().await;
        let frame = reader.read().await?;
        Ok(frame.str())
    }

    pub async fn shutdown(&self) {
        self.writer.lock().await.shutdown().await;
    }

    pub async fn make_ready(&self, err: String) {
        self.conn.lock().await.make_ready(err);
    }

    pub async fn wait_ready(&self) -> tokio::io::Result<()> {
        self.reader.lock().await.wait_ready().await
    }

    pub fn to_string(&self) -> String {
        format!("RouterConn(id:{})", self.id)
    }

    pub fn start_read(conn: Arc<RouterConn>) -> JoinHandle<()> {
        tokio::spawn(async move {
            conn.add_job().await;
            _ = Self::loop_read(conn).await;
        })
    }

    async fn loop_read(conn: Arc<RouterConn>) -> tokio::io::Result<()> {
        let mut reader = conn.reader.lock().await;
        info!("Router({}) forward {:?} read loop is starting on {}", conn.name, conn.conn_type, conn.to_string());
        loop {
            let err = match reader.read().await {
                Ok(mut frame) => {
                    let mut forward = conn.forward.lock().await;
                    match forward.proc_conn_frame(&conn, &mut frame).await {
                        Ok(v) => Ok(v),
                        Err(e) => Err(e),
                    }
                }
                Err(e) => Err(e),
            };
            if err.is_err() {
                conn.forward.lock().await.close_channel(&conn).await;
                let mut writer = conn.writer.lock().await;
                writer.shutdown().await;
                info!("Router({}) forward {:?} loop read is done by {:?} on {}", conn.name, conn.conn_type, err, conn.to_string());
                break;
            }
        }
        info!("Router({}) forward {:?} read loop is stopped on {}", conn.name, conn.conn_type, conn.to_string());
        conn.done_job().await;
        Ok(())
    }
}

pub struct RouterChannel {
    pub name: String,
    pub conn_type: ConnType,
    pub conn_all: HashMap<u16, Arc<RouterConn>>,
}

impl RouterChannel {
    pub fn new(name: String, conn_type: ConnType) -> Self {
        Self { name, conn_type, conn_all: HashMap::new() }
    }
    pub fn add_conn(&mut self, conn: Arc<RouterConn>) {
        self.conn_all.insert(conn.id, conn);
    }

    pub fn remove_conn(&mut self, id: &u16) -> Option<Arc<RouterConn>> {
        self.conn_all.remove(id)
    }

    pub fn find_conn(&self, id: &u16) -> Option<&Arc<RouterConn>> {
        self.conn_all.get(id)
    }

    pub async fn select_conn(&self) -> Option<&Arc<RouterConn>> {
        let mut conn = None;
        let mut min_used = 0;
        for c in self.conn_all.values() {
            let used = c.conn.lock().await.used;
            if conn.is_none() || min_used > used {
                conn = Some(c);
                min_used = used;
            }
        }
        conn
    }
    pub async fn shutdown(&mut self) {
        for conn in self.conn_all.values() {
            conn.shutdown().await;
        }
    }
}

pub struct RouterTableItem {
    pub from_conn: Arc<RouterConn>,
    pub from_sid: ConnID,
    pub next_conn: Arc<RouterConn>,
    pub next_sid: ConnID,
    pub uri: Arc<String>,
}

impl RouterTableItem {
    pub fn next(&self, conn: &Arc<RouterConn>) -> Option<(&Arc<RouterConn>, &ConnID)> {
        if eq(&*self.from_conn, &**conn) {
            Some((&self.next_conn, &self.next_sid))
        } else if eq(&*self.next_conn, &**conn) {
            Some((&self.next_conn, &self.next_sid))
        } else {
            None
        }
    }

    pub fn update(&mut self, conn: &Arc<RouterConn>, sid: ConnID) {
        if eq(&*self.from_conn, &**conn) {
            self.from_sid = sid;
        } else if eq(&*self.next_conn, &**conn) {
            self.next_sid = sid
        }
    }

    pub fn exists(&self, conn: &Arc<RouterConn>) -> bool {
        eq(&*self.from_conn, &**conn) || eq(&*self.next_conn, &**conn)
    }

    pub fn all_key(&self) -> Vec<String> {
        let mut from_key = RouterTableItem::router_key(&self.from_conn, &self.from_sid);
        let mut next_key = RouterTableItem::router_key(&self.next_conn, &self.next_sid);
        from_key.append(&mut next_key);
        from_key
    }

    pub fn to_string(&self) -> String {
        format!("{} {}<->{} {:?}", self.from_conn.to_string(), self.from_sid.to_string(), self.next_sid.to_string(), self.next_conn.to_string())
    }

    pub fn router_key(conn: &Arc<RouterConn>, sid: &ConnID) -> Vec<String> {
        let id = conn.id;
        let mut keys = Vec::new();
        if sid.lid > 0 {
            keys.push(format!("l-{}-{}", id, sid.lid))
        }
        if sid.rid > 0 {
            keys.push(format!("r-{}-{}", id, sid.rid))
        }
        keys
    }
}

pub struct RouterTable {
    table: HashMap<String, Arc<Mutex<RouterTableItem>>>,
}

impl RouterTable {
    pub fn new() -> Self {
        Self { table: HashMap::new() }
    }
    pub fn add_table(&mut self, item: RouterTableItem) {
        let keys = item.all_key();
        let val = Arc::new(Mutex::new(item));
        for key in keys {
            self.table.insert(key, val.clone());
        }
    }

    pub fn find_table(&self, conn: &Arc<RouterConn>, sid: &ConnID) -> Option<&Arc<Mutex<RouterTableItem>>> {
        for key in RouterTableItem::router_key(conn, sid) {
            match self.table.get(&key) {
                Some(v) => return Some(v),
                None => (),
            }
        }
        None
    }

    pub async fn update_table(&self, conn: &Arc<RouterConn>, sid: &ConnID) -> Option<&Arc<Mutex<RouterTableItem>>> {
        let item = self.find_table(conn, sid)?;
        item.lock().await.update(&conn, sid.clone());
        Some(item)
    }

    pub async fn remove_table(&mut self, conn: &Arc<RouterConn>, sid: &ConnID) -> Option<Arc<Mutex<RouterTableItem>>> {
        let item = match self.find_table(conn, sid) {
            Some(r) => Some(r.clone()),
            None => None,
        }?;
        for key in item.lock().await.all_key() {
            self.table.remove(&key);
        }
        None
    }

    pub async fn list_all_next(&self, conn: &Arc<RouterConn>) -> Vec<Arc<Mutex<RouterTableItem>>> {
        let mut conn_all = Vec::new();
        for v in self.table.values() {
            let item = v.lock().await;
            if item.exists(conn) {
                conn_all.push(v.clone());
            }
        }
        conn_all
    }

    pub async fn list_all_raw(&self) -> HashMap<u16, Arc<RouterConn>> {
        let mut conn_all = HashMap::new();
        for v in self.table.values() {
            let item = v.lock().await;
            if item.from_conn.conn_type == ConnType::Raw {
                conn_all.insert(item.from_conn.id, item.from_conn.clone());
            }
            if item.from_conn.conn_type == ConnType::Raw {
                conn_all.insert(item.next_conn.id, item.next_conn.clone());
            }
        }
        conn_all
    }
}

#[async_trait]
pub trait Handler {
    //on connection dial uri
    async fn on_conn_dial_uri(&self, channel: &RouterConn, conn: &String, parts: &Vec<String>) -> tokio::io::Result<()>;
    //on connection login
    async fn on_conn_login(&self, channel: &RouterConn, args: &String) -> tokio::io::Result<(String, String)>;
    //on connection close
    async fn on_conn_close(&self, conn: &RouterConn);
    //OnConnJoin is event on channel join
    async fn on_conn_join(&self, conn: &RouterConn, option: &String, result: &String);
}

pub struct RouterForward {
    pub name: String,
    pub table: RouterTable,
    pub channels: HashMap<String, RouterChannel>,
    pub handler: Arc<dyn Handler + Send + Sync>,
    waiter: wg::AsyncWaitGroup,
}

impl RouterForward {
    pub fn new(name: String, handler: Arc<dyn Handler + Send + Sync>) -> Self {
        Self { name, table: RouterTable::new(), channels: HashMap::new(), handler, waiter: wg::AsyncWaitGroup::new() }
    }

    async fn write_message(conn: &Arc<RouterConn>, sid: &ConnID, cmd: &RouterCmd, message: &[u8]) -> tokio::io::Result<usize> {
        let n = conn.header.data_offset + 3 + message.len();
        let mut buffer = vec![0; n];
        buffer[conn.header.data_offset + 3..].copy_from_slice(message);
        let frame = RouterFrame::new(&conn.header, &mut buffer, sid, cmd);
        conn.writer.lock().await.write(&frame).await
    }

    pub fn add_job(&mut self) {
        self.waiter.add(1);
    }

    pub fn done_job(&mut self) {
        self.waiter.done();
    }

    pub async fn proc_conn_frame(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        debug!("Router({}) receive conn frame {} on channel {}", self.name, frame.to_string(), channel.to_string());
        match frame.cmd {
            RouterCmd::None => self.proc_invalid_cmd(channel, frame).await,
            RouterCmd::LoginBack => self.proc_invalid_cmd(channel, frame).await,
            RouterCmd::LoginChannel => self.proc_login_channel(channel, frame).await,
            RouterCmd::PingConn => self.proc_ping_conn(channel, frame).await,
            RouterCmd::PingBack => self.proc_ping_back(channel, frame).await,
            RouterCmd::DialConn => self.proc_dial_conn(channel, frame).await,
            RouterCmd::DialBack => self.proc_dial_back(channel, frame).await,
            RouterCmd::ConnData => self.proc_conn_data(channel, frame).await,
            RouterCmd::ConnClosed => self.proc_conn_closed(channel, frame).await,
        }
    }

    pub async fn proc_invalid_cmd(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        warn!("Router({}) the channel receive invalid cmd {:?} on {}", self.name, frame.cmd, channel.to_string());
        Err(new_message_err(format!("invalid cmd {:?}", frame.cmd)))
    }

    pub async fn proc_login_channel(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        match match String::from_utf8(frame.data().to_vec()) {
            Ok(args) => {
                let (name, _) = self.handler.on_conn_login(channel, &args).await?;
                channel.set_name(name.clone()).await;
                self.add_channel(&name, channel);
                Self::write_message(channel, &frame.sid, &RouterCmd::LoginBack, CONN_OK.as_bytes()).await?;
                info!("Router({}) the channel({}) is login success on {}", self.name, &name, channel.to_string());
                Ok(())
            }
            Err(e) => Err(new_message_err(e)),
        } {
            Ok(_) => Ok(()),
            Err(e) => {
                Self::write_message(channel, &frame.sid, &RouterCmd::LoginBack, e.to_string().as_bytes()).await?;
                Err(e)
            }
        }
    }

    pub async fn proc_ping_conn(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        frame.update_cmd(RouterCmd::PingBack);
        channel.write(frame).await?;
        Ok(())
    }

    pub async fn proc_ping_back(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        let start_time: i64 = match channel.header.byte_order {
            crate::frame::ByteOrder::BE => i64::from_be_bytes(frame.data().try_into().unwrap()),
            crate::frame::ByteOrder::LE => i64::from_le_bytes(frame.data().try_into().unwrap()),
        };
        let now_time = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_millis() as i64;
        let mut conn = channel.conn.lock().await;
        conn.ping.speed = now_time - start_time;
        conn.ping.last = now_time;
        Ok(())
    }

    pub async fn proc_dial_conn(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        RouterForward::write_message(channel, &frame.sid, &RouterCmd::DialBack, "not supported".as_bytes()).await?;
        Ok(())
    }

    pub async fn proc_dial_back(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        match frame.str().as_str() {
            CONN_OK => match self.table.update_table(&channel, &frame.sid).await {
                Some(rv) => {
                    let router = rv.lock().await;
                    match router.next(channel) {
                        Some((next, next_sid)) => match next.conn_type {
                            ConnType::Channel => {
                                //forward to next
                                frame.update_sid(next_sid.clone());
                                _ = next.write(frame).await;
                                Ok(())
                            }
                            ConnType::Raw => {
                                //ready to read
                                info!("Router({}) dial {}->{} success on {}", self.name, next.to_string(), router.uri, channel.to_string());
                                next.make_ready(String::new()).await;
                                RouterConn::start_read(next.clone());
                                Ok(())
                            }
                        },
                        None => {
                            info!("Router({}) proc dial back fail with not router by {},{} on {}", self.name, channel.id, frame.sid.to_string(), channel.to_string());
                            Self::write_message(channel, &frame.sid, &RouterCmd::ConnClosed, CONN_CLOSED.as_bytes()).await?;
                            Ok(())
                        }
                    }
                }
                None => {
                    info!("Router({}) proc dial back fail with not router by {},{} on {}", self.name, channel.id, frame.sid.to_string(), channel.to_string());
                    Self::write_message(channel, &frame.sid, &RouterCmd::ConnClosed, CONN_CLOSED.as_bytes()).await?;
                    Ok(())
                }
            },
            m => match self.table.remove_table(channel, &frame.sid).await {
                Some(rv) => {
                    let router = rv.lock().await;
                    info!("Router({}) dial {}->{} fail with {}", self.name, router.to_string(), router.uri, m);
                    channel.free_conn_id(&frame.sid.lid).await;
                    if let Some((next, _)) = router.next(channel) {
                        next.shutdown().await;
                        next.make_ready(m.to_string()).await;
                    }
                    Ok(())
                }
                None => {
                    info!("Router({}) proc dial back fail with not router by {},{} on {}", self.name, channel.id, frame.sid.to_string(), channel.to_string());
                    Ok(())
                }
            },
        }
    }

    pub async fn proc_conn_data(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        match self.table.find_table(channel, &frame.sid) {
            Some(rv) => {
                let router = rv.lock().await;
                match router.next(channel) {
                    Some((next, next_sid)) => {
                        frame.update_sid(next_sid.clone());
                        _ = next.write(frame).await;
                        Ok(())
                    }
                    None => {
                        Self::write_message(channel, &frame.sid, &RouterCmd::ConnClosed, CONN_CLOSED.as_bytes()).await?;
                        Ok(())
                    }
                }
            }
            None => match channel.conn_type {
                ConnType::Channel => {
                    Self::write_message(channel, &frame.sid, &RouterCmd::ConnClosed, CONN_CLOSED.as_bytes()).await?;
                    Ok(())
                }
                ConnType::Raw => {
                    channel.shutdown().await;
                    Err(new_message_err(CONN_CLOSED))
                }
            },
        }
    }

    pub async fn proc_conn_closed(&mut self, channel: &Arc<RouterConn>, frame: &mut RouterFrame<'_>) -> tokio::io::Result<()> {
        let message = frame.str();
        debug!("Router({}) the session({}) is closed by {}", self.name, frame.sid.to_string(), message);
        match self.table.find_table(channel, &frame.sid) {
            Some(rv) => {
                let router = rv.lock().await;
                match router.next(channel) {
                    Some((next, next_sid)) => match channel.conn_type {
                        ConnType::Channel => {
                            frame.update_sid(next_sid.clone());
                            _ = next.write(frame).await;
                        }
                        ConnType::Raw => channel.shutdown().await,
                    },
                    None => match channel.conn_type {
                        ConnType::Channel => (),
                        ConnType::Raw => channel.shutdown().await,
                    },
                }
            }
            None => (),
        }
        channel.free_conn_id(&frame.sid.lid).await;
        Ok(())
    }

    pub fn add_channel(&mut self, name: &String, conn: &Arc<RouterConn>) {
        let channel = match self.channels.get_mut(name) {
            Some(c) => c,
            None => {
                self.channels.insert(name.clone(), RouterChannel::new(name.clone(), ConnType::Channel));
                self.channels.get_mut(name).unwrap()
            }
        };
        channel.add_conn(conn.clone());
    }

    pub async fn select_channel(&self, name: &String) -> tokio::io::Result<Arc<RouterConn>> {
        match async { self.channels.get(name)?.select_conn().await }.await {
            Some(c) => Ok(c.clone()),
            None => Err(new_message_err(format!("channel {} is not exists", name))),
        }
    }

    pub async fn close_channel(&mut self, conn: &Arc<RouterConn>) {
        for v in self.table.list_all_next(conn).await {
            let item = v.lock().await;
            let (next, next_sid) = item.next(conn).unwrap();
            match next.conn_type {
                ConnType::Channel => {
                    _ = Self::write_message(next, &next_sid, &RouterCmd::ConnClosed, CONN_CLOSED.as_bytes()).await;
                }
                ConnType::Raw => next.shutdown().await,
            }
        }
    }

    pub async fn shutdown(&mut self) -> wg::AsyncWaitGroup {
        info!("Router({}) all forward channel is stopping", self.name);
        for channel in self.channels.values_mut() {
            channel.shutdown().await;
        }
        info!("Router({}) all forward raw conn is stopping", self.name);
        for conn in self.table.list_all_raw().await.values() {
            conn.shutdown().await;
        }
        self.waiter.clone()
    }
}

pub struct Router {
    pub name: String,
    pub header: Arc<Header>,
    pub buffer_size: usize,
    pub forward: Arc<Mutex<RouterForward>>,
    cid_seq: u16,
    // cid_used: HashSet<u16>,
}

impl Router {
    pub fn new(name: String, handler: Arc<dyn Handler + Send + Sync>) -> Self {
        let mut h = Header::new();
        h.length_field_length = 2;
        h.data_offset = 2;
        let header = Arc::new(h);
        let forward = Arc::new(Mutex::new(RouterForward::new(name.clone(), handler)));
        Self { name, header, buffer_size: 8 * 1024, forward, cid_seq: 0 }
    }

    pub fn new_conn_id(&mut self) -> u16 {
        self.cid_seq += 1;
        self.cid_seq
    }

    pub async fn add_channel(&mut self, name: &String, conn: &Arc<RouterConn>) {
        self.forward.lock().await.add_channel(name, conn)
    }

    pub async fn select_channel(&self, name: &String) -> tokio::io::Result<Arc<RouterConn>> {
        self.forward.lock().await.select_channel(name).await
    }

    async fn add_table(&mut self, from_conn: Arc<RouterConn>, from_sid: ConnID, next_conn: Arc<RouterConn>, next_sid: ConnID, uri: Arc<String>) {
        let item = RouterTableItem { from_conn, from_sid, next_conn, next_sid, uri };
        self.forward.lock().await.table.add_table(item);
    }

    async fn remove_table(&mut self, conn: &Arc<RouterConn>, sid: &ConnID) -> Option<Arc<Mutex<RouterTableItem>>> {
        self.forward.lock().await.table.remove_table(conn, sid).await
    }

    pub async fn register(&mut self, conn: Arc<RouterConn>) {
        let name = conn.name().await;
        self.add_channel(&name, &conn).await;
        RouterConn::start_read(conn);
    }

    pub async fn join_base(&mut self, reader: Box<dyn Reader + Send + Sync>, writer: Box<dyn Writer + Send + Sync>, options: &String) -> tokio::io::Result<()> {
        let frame_reader = FrameReader::new(self.header.clone(), reader, self.buffer_size);
        let frame_writer = FrameWriter::new(self.header.clone(), writer);
        let id: u16 = self.new_conn_id();
        let info = Arc::new(Mutex::new(Conn::new(id.clone(), ConnType::Channel)));
        let conn_reader = Arc::new(Mutex::new(RouterConnReader::new(self.header.clone(), frame_reader, info.clone()).await));
        let conn_writer = Arc::new(Mutex::new(RouterConnWriter::new(self.header.clone(), frame_writer, info.clone()).await));
        let conn = Arc::new(RouterConn::new(self.name.clone(), self.header.clone(), id, ConnType::Channel, info, conn_reader, conn_writer, self.forward.clone()));
        self.join_conn(conn, options).await
    }

    pub async fn join_conn(&mut self, conn: Arc<RouterConn>, options: &String) -> tokio::io::Result<()> {
        info!("Router({}) start login join connection {} by options {}", self.name, conn.to_string(), options);
        match self.join_call(conn.clone(), options).await {
            Ok(_) => Ok(()),
            Err(e) => {
                warn!("Router({}) login to {} fail with {}", self.name, conn.to_string(), e);
                conn.writer.lock().await.shutdown().await;
                Err(e)
            }
        }
    }

    async fn join_call(&mut self, conn: Arc<RouterConn>, options: &String) -> tokio::io::Result<()> {
        RouterForward::write_message(&conn, &ConnID::zero(), &RouterCmd::LoginChannel, options.as_bytes()).await?;
        let message = conn.read_str().await?;
        let result = match json::parse(&message) {
            Ok(v) => Ok(v),
            Err(e) => Err(new_message_err(e)),
        }?;
        let code = json_must_i32(&result, &"code")?;
        if code != 0 {
            return Err(new_message_err(message));
        }
        let name = String::from(json_must_str(&result, &"name")?);
        conn.set_name(name.clone()).await;
        self.register(conn.clone()).await;
        info!("Router({}) login to {} success, bind to {}", self.name, conn.to_string(), name);
        self.forward.lock().await.handler.on_conn_join(&conn, options, &message).await;
        Ok(())
    }

    pub async fn dial_base(&mut self, reader: Box<dyn Reader + Send + Sync>, writer: Box<dyn Writer + Send + Sync>, uri: Arc<String>) -> tokio::io::Result<Arc<RouterConn>> {
        let id: u16 = self.new_conn_id();
        let info = Arc::new(Mutex::new(Conn::new(id.clone(), ConnType::Raw)));
        let conn_reader = Arc::new(Mutex::new(RouterRawReader::new(self.header.clone(), reader, info.clone(), self.buffer_size).await));
        let conn_writer = Arc::new(Mutex::new(RouterRawWriter::new(self.header.clone(), writer, info.clone()).await));
        let conn = Arc::new(RouterConn::new(self.name.clone(), self.header.clone(), id, ConnType::Raw, info, conn_reader, conn_writer, self.forward.clone()));
        conn.set_name(format!("Raw{}", id)).await;
        self.dial_conn(conn.clone(), uri).await?;
        Ok(conn)
    }

    pub async fn dial_conn(&mut self, conn: Arc<RouterConn>, uri: Arc<String>) -> tokio::io::Result<()> {
        let parts: Vec<&str> = uri.splitn(2, "->").collect();
        if parts.len() < 2 {
            return Err(new_message_err("invalid uri"));
        }
        let name = parts[0].to_string();
        let channel = self.select_channel(&name).await?.clone();
        let lid = channel.alloc_conn_id().await;
        let channel_sid = ConnID::new(lid, 0);
        let conn_sid = ConnID::new(lid, lid);
        conn.set_data_prefix(RouterFrame::make_data_prefix(&conn_sid, &RouterCmd::ConnData)).await;
        debug!("Router({}) start dial({}-{}->{}-{}) to {} on channel({})", self.name, conn.id, conn_sid.to_string(), channel.id, channel_sid.to_string(), uri, channel.to_string());
        self.add_table(channel.clone(), channel_sid.clone(), conn.clone(), conn_sid, uri.clone()).await;
        match RouterForward::write_message(&channel, &channel_sid, &RouterCmd::DialConn, format!("{}|{}", parts[0], parts[1]).as_bytes()).await {
            Ok(_) => Ok(()),
            Err(e) => {
                self.remove_table(&channel, &channel_sid).await;
                channel.free_conn_id(&lid).await;
                conn.shutdown().await;
                Err(e)
            }
        }
    }

    pub async fn shutdown(&mut self) -> wg::AsyncWaitGroup {
        info!("Router({}) is stopping", self.name);
        self.forward.lock().await.shutdown().await
    }
}

pub struct NormalAcessHandler {}

impl NormalAcessHandler {
    pub fn new() -> Self {
        Self {}
    }
}

#[async_trait]
impl Handler for NormalAcessHandler {
    //on connection dial uri
    async fn on_conn_dial_uri(&self, _: &RouterConn, _: &String, _: &Vec<String>) -> tokio::io::Result<()> {
        Ok(())
    }
    //on connection login
    async fn on_conn_login(&self, _: &RouterConn, _: &String) -> tokio::io::Result<(String, String)> {
        Ok((String::from("NX"), String::from("OK")))
    }
    //on connection close
    async fn on_conn_close(&self, _: &RouterConn) {}
    //OnConnJoin is event on channel join
    async fn on_conn_join(&self, _: &RouterConn, _: &String, _: &String) {}
}
