use async_trait::async_trait;
use log::{debug, info, warn};
use std::{
    collections::{HashMap, HashSet},
    io::ErrorKind,
    sync::Arc,
    time::{Duration, SystemTime, UNIX_EPOCH},
};
use tokio::{
    sync::{
        mpsc::{Receiver, Sender},
        Mutex, Notify,
    },
    task::JoinHandle,
};

use crate::util::{json_must_i64, json_must_str, JSON};
use crate::{frame, util::new_message_err};

const CONN_OK: &str = "OK";
const CONN_CLOSED: &str = "CLOSED";

#[derive(Clone, Debug)]
pub enum Cmd {
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

impl Cmd {
    pub fn as_cmd(cmd: &u8) -> Cmd {
        match cmd {
            10 => Cmd::LoginChannel,
            11 => Cmd::LoginBack,
            20 => Cmd::PingConn,
            21 => Cmd::PingBack,
            100 => Cmd::DialConn,
            101 => Cmd::DialBack,
            110 => Cmd::ConnData,
            120 => Cmd::ConnClosed,
            _ => Cmd::None,
        }
    }
    pub fn as_u8(&self) -> u8 {
        match self {
            Cmd::None => 0,
            Cmd::LoginChannel => 10,
            Cmd::LoginBack => 11,
            Cmd::PingConn => 20,
            Cmd::PingBack => 21,
            Cmd::DialConn => 100,
            Cmd::DialBack => 101,
            Cmd::ConnData => 110,
            Cmd::ConnClosed => 120,
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
        hex::encode([self.lid, self.rid]).to_uppercase()
    }
}

pub struct Frame<'a> {
    pub buf: &'a mut [u8],
    pub sid: ConnID,
    pub cmd: Cmd,
    pub data_offset: usize,
}

impl<'a> Frame<'a> {
    //parse remote frame, lid is in 0, rid is in 1
    pub fn prase_local(header: &Arc<frame::Header>, buf: &'a mut [u8]) -> Self {
        let data_offset = header.data_offset;
        let sid = ConnID::new(buf[data_offset], buf[data_offset + 1]);
        let cmd = Cmd::as_cmd(&buf[data_offset + 2]);
        Frame { buf, sid, cmd, data_offset }
    }

    //parse remote frame, lid is in 1, rid is in 0
    pub fn prase_remote(header: &Arc<frame::Header>, buf: &'a mut [u8]) -> Self {
        let data_offset = header.data_offset;
        let sid = ConnID::new(buf[data_offset + 1], buf[data_offset]);
        let cmd = Cmd::as_cmd(&buf[data_offset + 2]);
        Frame { buf, sid, cmd, data_offset }
    }

    pub fn new(header: &Arc<frame::Header>, buf: &'a mut [u8], sid: &ConnID, cmd: &Cmd) -> Self {
        let data_offset = header.data_offset;
        header.write_head(buf);
        buf[data_offset] = sid.lid.clone();
        buf[data_offset + 1] = sid.rid.clone();
        buf[data_offset + 2] = cmd.as_u8();
        Frame { buf, sid: sid.clone(), cmd: cmd.clone(), data_offset }
    }

    pub fn copy(header: &Arc<frame::Header>, sid: &ConnID, cmd: &Cmd, message: &[u8]) -> Vec<u8> {
        let o = header.data_offset;
        let n = o + 3 + message.len();
        let mut buf = vec![0; n];
        header.write_head(&mut buf);
        buf[o] = sid.lid.clone();
        buf[o + 1] = sid.rid.clone();
        buf[o + 2] = cmd.as_u8();
        buf[o + 3..].copy_from_slice(message);
        buf
    }

    pub fn make_data_prefix(sid: &ConnID, cmd: &Cmd) -> Vec<u8> {
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

    pub fn update_cmd(&mut self, cmd: Cmd) {
        self.buf[self.data_offset + 2] = cmd.as_u8();
        self.cmd = cmd;
    }

    pub fn to_string(&self) -> String {
        if self.buf.len() > 32 {
            format!("Frame({},{},{:?},{})", self.buf.len(), self.sid.to_string(), self.cmd, hex::encode(&self.buf[..32]))
        } else {
            format!("Frame({},{},{:?},{})", self.buf.len(), self.sid.to_string(), self.cmd, hex::encode(&self.buf))
        }
    }
}

#[derive(Clone, PartialEq, Debug)]
pub enum ConnType {
    Channel,
    Raw,
}

struct StateWaiter_ {
    pub state: Option<String>,
}

impl StateWaiter_ {
    pub fn new() -> Self {
        Self { state: None }
    }
}

#[derive(Clone)]
pub struct StateWaiter {
    inner: Arc<Mutex<StateWaiter_>>,
    pub ready: Arc<Notify>,
}

impl StateWaiter {
    pub fn new() -> Self {
        Self { inner: Arc::new(Mutex::new(StateWaiter_::new())), ready: Arc::new(Notify::new()) }
    }

    pub async fn ready(&self, state: Option<String>) {
        self.inner.lock().await.state = state;
        self.ready.notify_waiters();
    }

    pub async fn wait(&self) -> Option<String> {
        self.ready.notified().await;
        self.inner.lock().await.state.clone()
    }
}

#[derive(Clone)]
pub struct JobWaiter {
    pub name: Arc<String>,
    pub waiter: Arc<wg::AsyncWaitGroup>,
}

impl JobWaiter {
    pub fn new(name: Arc<String>) -> Self {
        Self { name, waiter: Arc::new(wg::AsyncWaitGroup::new()) }
    }

    pub fn add(&self) {
        self.waiter.add(1);
    }

    pub fn done(&self) {
        self.waiter.done()
    }

    pub async fn wait(&self) {
        self.waiter.wait().await;
    }
}

#[async_trait]
pub trait Reader {
    async fn read(&mut self) -> tokio::io::Result<Frame>;
    async fn set_conn_id(&mut self, conn_id: ConnID);
    async fn wait_ready(&self) -> Option<String>;
}

#[async_trait]
pub trait Writer {
    async fn write(&mut self, frame: &Frame<'_>) -> tokio::io::Result<usize>;
    async fn shutdown(&mut self);
    async fn wait_ready(&self) -> Option<String>;
}

pub struct ChannelReader {
    pub header: Arc<frame::Header>,
    pub inner: frame::Reader,
    pub waiter: StateWaiter,
}

impl ChannelReader {
    pub async fn new(inner: frame::Reader, waiter: StateWaiter) -> Self {
        Self { header: inner.header.clone(), inner, waiter }
    }
}

#[async_trait]
impl Reader for ChannelReader {
    async fn read(&mut self) -> tokio::io::Result<Frame> {
        let buf = self.inner.read().await?;
        Ok(Frame::prase_remote(&self.header, buf))
    }

    async fn set_conn_id(&mut self, _: ConnID) {}

    async fn wait_ready(&self) -> Option<String> {
        self.waiter.wait().await
    }
}

pub struct ChannelWriter {
    pub header: Arc<frame::Header>,
    pub inner: frame::Writer,
    pub waiter: StateWaiter,
}

impl ChannelWriter {
    pub async fn new(inner: frame::Writer, waiter: StateWaiter) -> Self {
        Self { header: inner.header.clone(), inner, waiter }
    }
}

#[async_trait]
impl Writer for ChannelWriter {
    async fn write(&mut self, frame: &Frame<'_>) -> tokio::io::Result<usize> {
        self.inner.write(frame.buf).await
    }

    async fn shutdown(&mut self) {
        _ = self.inner.shutdown().await
    }

    async fn wait_ready(&self) -> Option<String> {
        self.waiter.wait().await
    }
}

pub struct RawReader {
    pub header: Arc<frame::Header>,
    pub inner: Box<dyn frame::RawReader + Send + Sync>,
    pub waiter: StateWaiter,
    conn_id: ConnID,
    buf: Box<Vec<u8>>,
}

impl RawReader {
    pub async fn new(header: Arc<frame::Header>, inner: Box<dyn frame::RawReader + Send + Sync>, waiter: StateWaiter, buffer_size: usize) -> Self {
        Self { header, inner, waiter, buf: Box::new(vec![0; buffer_size]), conn_id: ConnID::zero() }
    }
}

#[async_trait]
impl Reader for RawReader {
    async fn read(&mut self) -> tokio::io::Result<Frame> {
        let s = &mut *self;
        let header = &s.header;
        let o = s.header.data_offset;
        let sbuf = s.buf.as_mut();
        let mut n = s.inner.read(&mut sbuf[o + 3..]).await?;
        if n < 1 {
            return Err(std::io::Error::new(ErrorKind::UnexpectedEof, "R-EOF"));
        }
        n += o + 3;
        let data = &mut sbuf[..n];
        let frame = Frame::new(header, data, &s.conn_id, &Cmd::ConnData);
        Ok(frame)
    }

    async fn set_conn_id(&mut self, conn_id: ConnID) {
        self.conn_id = conn_id;
    }

    async fn wait_ready(&self) -> Option<String> {
        self.waiter.wait().await
    }
}

pub struct RawWriter {
    pub header: Arc<frame::Header>,
    pub inner: Box<dyn frame::RawWriter + Send + Sync>,
    pub waiter: StateWaiter,
}

impl RawWriter {
    pub async fn new(header: Arc<frame::Header>, inner: Box<dyn frame::RawWriter + Send + Sync>, waiter: StateWaiter) -> Self {
        Self { header, inner, waiter }
    }
}

#[async_trait]
impl Writer for RawWriter {
    async fn write(&mut self, frame: &Frame<'_>) -> tokio::io::Result<usize> {
        self.inner.write(frame.data()).await
    }

    async fn shutdown(&mut self) {
        _ = self.inner.shutdown().await;
    }

    async fn wait_ready(&self) -> Option<String> {
        self.waiter.wait().await
    }
}

pub struct ConnSeq {
    pub sid_seq: u8,
    pub sid_all: HashMap<u8, bool>,
}

impl ConnSeq {
    pub fn new() -> Self {
        Self { sid_seq: 0, sid_all: HashMap::new() }
    }

    pub fn alloc_conn_id(&mut self) -> tokio::io::Result<u8> {
        for _ in 0..257 {
            self.sid_seq = (self.sid_seq as u16 + 1) as u8;
            if self.sid_seq > 0 && !self.sid_all.contains_key(&self.sid_seq) {
                self.sid_all.insert(self.sid_seq, true);
                return Ok(self.sid_seq);
            }
        }
        Err(new_message_err("too many connect"))
    }

    pub fn free_conn_id(&mut self, conn_id: &u8) {
        if self.sid_all.contains_key(&conn_id) {
            self.sid_all.remove(conn_id);
        }
    }

    pub fn used_conn_id(&self) -> usize {
        self.sid_all.len()
    }
}

#[derive(Clone)]
pub struct Ping {
    pub speed: i64,
    pub last: i64,
}

impl Ping {
    pub fn new(speed: i64, last: i64) -> Self {
        Self { speed, last }
    }
}

#[derive(Clone)]
pub struct Conn {
    pub id: u16,
    pub name: Arc<String>,
    pub conn_type: ConnType,
    pub header: Arc<frame::Header>,
    pub waiter: StateWaiter,
    pub reader: Arc<Mutex<dyn Reader + Send + Sync>>,
    pub writer: Arc<Mutex<dyn Writer + Send + Sync>>,
    pub recv_last: i64,
    pub ping: Option<Ping>,
    pub used: u32,
}

impl Conn {
    pub fn new(id: u16, conn_type: ConnType, header: Arc<frame::Header>, waiter: StateWaiter, reader: Arc<Mutex<dyn Reader + Send + Sync>>, writer: Arc<Mutex<dyn Writer + Send + Sync>>) -> Self {
        Self { id, name: Arc::new(String::new()), conn_type, header, waiter, reader, writer, recv_last: 0, ping: None, used: 0 }
    }

    pub async fn set_conn_id(&mut self, conn_id: ConnID) {
        self.reader.lock().await.set_conn_id(conn_id).await;
    }

    pub async fn write(&self, frame: &Frame<'_>) -> tokio::io::Result<usize> {
        let mut writer = self.writer.lock().await;
        let result = writer.write(frame).await;
        if result.is_err() {
            writer.shutdown().await
        }
        result
    }

    pub async fn write_message(&self, sid: &ConnID, cmd: &Cmd, message: &[u8]) -> tokio::io::Result<usize> {
        let mut buf = Frame::copy(&self.header, sid, cmd, message);
        let frame = Frame::new(&self.header, &mut buf, sid, cmd);
        self.write(&frame).await
    }

    pub async fn read_str(&self) -> tokio::io::Result<String> {
        let mut reader = self.reader.lock().await;
        let frame = reader.read().await?;
        Ok(frame.str())
    }

    pub async fn shutdown(&self) {
        self.writer.lock().await.shutdown().await;
    }

    pub async fn ready(&self, state: Option<String>) {
        self.waiter.ready(state).await
    }

    pub async fn wait(&self) -> tokio::io::Result<()> {
        match self.waiter.wait().await {
            Some(m) => Err(new_message_err(m)),
            None => Ok(()),
        }
    }

    pub fn to_string(&self) -> String {
        format!("{:?}({})", self.conn_type, self.id)
    }

    pub fn display(&self) -> String {
        format!("{:?}({})", self.conn_type, self.id)
    }

    pub fn start_read(job: Job, conn: Arc<Conn>) -> JoinHandle<()> {
        tokio::spawn(async move {
            job.add();
            _ = Self::loop_read(&job, &conn).await;
            job.done()
        })
    }

    async fn loop_read(job: &Job, conn: &Arc<Conn>) -> tokio::io::Result<()> {
        let mut reader = conn.reader.lock().await;
        info!("Router({}) forward {:?} loop read is starting on {}", job.name, conn.conn_type, conn.to_string());
        loop {
            let task = match reader.read().await {
                Ok(mut frame) => {
                    // let start_time = now();
                    let mut router = job.router.lock().await;
                    let task = router.proc_conn_frame(&conn.id, &mut frame).await;
                    // let used_time = now() - start_time;
                    // info!("{} frame is used {}", conn.to_string(), used_time);
                    task
                }
                Err(e) => Err(e),
            };
            // let start_time = now();
            // info!("{} task is  start {}", conn.to_string(), start_time);
            let err = match task {
                Ok(mut t) => t.call(job).await,
                Err(e) => Err(e),
            };
            // let used_time = now() - start_time;
            // info!("{} task is  used {}", conn.to_string(), used_time);
            if err.is_err() {
                _ = job.router.lock().await.close_conn(&conn.id).await.call(job).await;
                conn.shutdown().await;
                info!("Router({}) forward {:?} loop read is done by {:?} on {}", job.name, conn.conn_type, err, conn.to_string());
                break;
            }
        }
        info!("Router({}) forward {:?} read loop is stopped on {}", job.name, conn.conn_type, conn.to_string());
        Ok(())
    }
}

#[derive(Clone)]
pub struct TableItem {
    pub id: u32,
    pub from_conn: u16,
    pub from_type: ConnType,
    pub from_sid: ConnID,
    pub next_conn: u16,
    pub next_type: ConnType,
    pub next_sid: ConnID,
}

impl TableItem {
    pub fn next(&self, conn: &u16) -> Option<(u16, ConnType, ConnID)> {
        if self.from_conn == *conn {
            Some((self.next_conn.clone(), self.next_type.clone(), self.next_sid.clone()))
        } else if self.next_conn == *conn {
            Some((self.from_conn.clone(), self.from_type.clone(), self.from_sid.clone()))
        } else {
            None
        }
    }

    pub fn update(&mut self, conn: &u16, sid: &ConnID) {
        if self.from_conn == *conn {
            self.from_sid = sid.clone();
        } else if self.next_conn == *conn {
            self.next_sid = sid.clone()
        }
    }

    pub fn exists(&self, conn: &u16) -> bool {
        self.from_conn == *conn || self.next_conn == *conn
    }

    pub fn all_key(&self) -> Vec<String> {
        let mut from_key = TableItem::router_key(&self.from_conn, &self.from_sid);
        let mut next_key = TableItem::router_key(&self.next_conn, &self.next_sid);
        from_key.append(&mut next_key);
        from_key
    }

    pub fn to_string(&self) -> String {
        format!("{} {}<->{} {}", self.from_conn.to_string(), self.from_sid.to_string(), self.next_sid.to_string(), self.next_conn.to_string())
    }

    pub fn router_key(conn: &u16, sid: &ConnID) -> Vec<String> {
        let mut keys = Vec::new();
        if sid.lid > 0 {
            keys.push(format!("l-{}-{}", conn, sid.lid))
        }
        keys
    }
}

pub struct Table {
    item_all: HashMap<u32, TableItem>,
    key_all: HashMap<String, u32>,
    id_seq: u32,
}

impl Table {
    pub fn new() -> Self {
        Self { item_all: HashMap::new(), key_all: HashMap::new(), id_seq: 0 }
    }

    fn add(&mut self, from_conn: &Arc<Conn>, from_sid: ConnID, next_conn: &Arc<Conn>, next_sid: ConnID) {
        self.id_seq = ((self.id_seq as u64) + 1) as u32;
        let item = TableItem { id: self.id_seq.clone(), from_conn: from_conn.id, from_type: from_conn.conn_type.clone(), from_sid, next_conn: next_conn.id, next_type: next_conn.conn_type.clone(), next_sid };
        for key in item.all_key() {
            self.key_all.insert(key, item.id);
        }
        self.item_all.insert(item.id, item);
    }

    pub fn find(&self, conn: &u16, sid: &ConnID) -> Option<TableItem> {
        for key in TableItem::router_key(conn, sid) {
            match self.key_all.get(&key) {
                Some(v) => match self.item_all.get(v) {
                    Some(v) => return Some(v.clone()),
                    None => (),
                },
                None => (),
            }
        }
        None
    }

    pub fn update(&mut self, conn: &u16, sid: &ConnID) -> Option<TableItem> {
        for key in TableItem::router_key(conn, sid) {
            match self.key_all.get(&key) {
                Some(v) => match self.item_all.get_mut(v) {
                    Some(v) => {
                        v.update(conn, sid);
                        return Some(v.clone());
                    }
                    None => (),
                },
                None => (),
            }
        }
        None
    }

    pub fn next(&mut self, conn: &u16, sid: &ConnID, update: bool) -> Option<(u16, ConnType, ConnID)> {
        if update {
            self.update(conn, sid)?.next(conn)
        } else {
            self.find(conn, sid)?.next(conn)
        }
    }

    pub fn remove(&mut self, conn: &u16, sid: &ConnID) -> Option<TableItem> {
        let mut item = None;
        for key in TableItem::router_key(conn, sid) {
            match self.key_all.remove(&key) {
                Some(v) => match self.item_all.remove(&v) {
                    Some(v) => item = Some(v),
                    None => (),
                },
                None => (),
            }
        }
        item
    }

    pub fn remove_all_next(&mut self, conn: &u16) -> Vec<TableItem> {
        let mut id_all = Vec::new();
        for v in self.item_all.values() {
            if v.exists(conn) {
                id_all.push(v.id);
            }
        }
        let mut item_all = Vec::new();
        for id in &id_all {
            match self.item_all.remove(&id) {
                Some(v) => {
                    for key in v.all_key() {
                        self.key_all.remove(&key);
                    }
                    item_all.push(v);
                }
                None => todo!(),
            }
        }
        item_all
    }

    pub fn list_all_raw(&self) -> HashSet<u16> {
        let mut conn_all = HashSet::new();
        for v in self.item_all.values() {
            if v.from_type == ConnType::Raw {
                conn_all.insert(v.from_conn);
            }
            if v.next_type == ConnType::Raw {
                conn_all.insert(v.next_conn);
            }
        }
        conn_all
    }

    pub fn clear(&mut self) {
        self.item_all.clear();
        self.key_all.clear();
    }

    pub async fn display(&self) -> Vec<String> {
        let mut info = Vec::new();
        for item in self.item_all.values() {
            info.push(item.to_string())
        }
        info
    }
}

pub enum HandlerAction {
    Dial,
    Login,
    Close,
    Join,
}

#[async_trait]
pub trait Handler {
    //on connection dial uri
    async fn on_conn_dial_uri(&self, channel: &Conn, conn: &String, parts: &Vec<String>) -> tokio::io::Result<()>;
    //on connection login
    async fn on_conn_login(&self, channel: &Conn, args: &String) -> tokio::io::Result<(String, Arc<JSON>)>;
    //on connection close
    async fn on_conn_close(&self, conn: &Conn);
    //OnConnJoin is event on channel join
    async fn on_conn_join(&self, conn: &Conn, option: Arc<JSON>, result: Arc<JSON>);
}

#[derive(Clone)]
pub struct Job {
    pub name: Arc<String>,
    pub job: JobWaiter,
    router: Arc<Mutex<Router_>>,
}

impl Job {
    pub fn new(name: Arc<String>, router: Arc<Mutex<Router_>>) -> Self {
        let job = JobWaiter::new(name.clone());
        Self { name, job, router }
    }
    pub fn add(&self) {
        self.job.add();
    }

    pub fn done(&self) {
        self.job.done()
    }

    pub async fn wait(&self) {
        self.job.wait().await;
    }

    pub async fn proc_conn_frame(&self, conn_id: &u16, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        let mut router = self.router.lock().await;
        let task = router.proc_conn_frame(conn_id, frame).await?;
        Ok(task)
    }
}

pub struct Channel {
    pub name: Arc<String>,
    pub conn_all: HashMap<u16, u32>,
}

impl Channel {
    pub fn new(name: Arc<String>) -> Self {
        Self { name, conn_all: HashMap::new() }
    }

    pub fn add(&mut self, conn: u16) {
        self.conn_all.insert(conn, 0);
    }

    pub fn remove(&mut self, id: &u16) -> Option<u32> {
        let used = self.conn_all.remove(id)?;
        Some(used)
    }

    pub fn find(&self, id: &u16) -> Option<u32> {
        let used = self.conn_all.get(id)?;
        Some(used.clone())
    }

    pub fn select(&mut self) -> Option<u16> {
        let mut conn = None;
        let mut min = std::u32::MAX;
        for (c, used) in self.conn_all.iter() {
            if min > *used {
                min = *used;
                conn = Some(c.clone());
            }
        }
        let conn = conn?;
        self.conn_all.insert(conn, ((min as u64) + 1) as u32);
        Some(conn.clone())
    }

    pub fn list(&mut self) -> Vec<u16> {
        let mut conn_all = Vec::new();
        for conn in self.conn_all.keys() {
            conn_all.push(conn.clone());
        }
        conn_all
    }

    pub fn len(&self) -> usize {
        self.conn_all.len()
    }
}

#[derive(Debug)]
pub enum TaskAction {
    Write,
    Ready,
    Shutdown,
    None,
}

pub struct TaskItem {
    pub conn: Arc<Conn>,
    pub action: TaskAction,
    pub frame: Option<Vec<u8>>,
    pub state: Option<String>,
}

impl TaskItem {
    pub fn none(conn: Arc<Conn>) -> Self {
        Self { conn, action: TaskAction::None, frame: None, state: None }
    }

    pub fn message(conn: Arc<Conn>, sid: &ConnID, cmd: &Cmd, message: &[u8]) -> Self {
        let message = Frame::copy(&conn.header, sid, cmd, message);
        Self { conn, action: TaskAction::Write, frame: Some(message), state: None }
    }

    pub fn ready(conn: Arc<Conn>, state: Option<String>) -> Self {
        Self { conn, action: TaskAction::Ready, frame: None, state: state }
    }

    pub fn shutdown(conn: Arc<Conn>, state: Option<String>) -> Self {
        Self { conn, action: TaskAction::Shutdown, frame: None, state: state }
    }

    pub async fn call(&mut self, job: &Job) -> tokio::io::Result<()> {
        match self.action {
            TaskAction::Write => match self.frame.as_mut() {
                Some(mut buf) => {
                    _ = self.conn.write(&Frame::prase_local(&self.conn.header, &mut buf)).await?;
                    Ok(())
                }
                None => Err(new_message_err("buf is none")),
            },
            TaskAction::Ready => {
                self.conn.ready(self.state.clone()).await;
                _ = Conn::start_read(job.clone(), self.conn.clone());
                Ok(())
            }
            TaskAction::Shutdown => {
                self.conn.shutdown().await;
                Ok(())
            }
            TaskAction::None => Ok(()),
        }
    }
}

pub struct Task {
    pub all: Vec<TaskItem>,
}

impl Task {
    pub fn new() -> Self {
        Self { all: Vec::new() }
    }

    pub fn message(conn: Arc<Conn>, sid: &ConnID, cmd: &Cmd, message: &[u8]) -> Self {
        Self { all: vec![TaskItem::message(conn, sid, cmd, message)] }
    }

    pub fn ready(conn: Arc<Conn>, state: Option<String>) -> Self {
        Self { all: vec![TaskItem::ready(conn, state)] }
    }

    pub fn shutdown(conn: Arc<Conn>, state: Option<String>) -> Self {
        Self { all: vec![TaskItem::shutdown(conn, state)] }
    }

    pub fn more(&mut self, item: TaskItem) {
        self.all.push(item);
    }

    pub async fn call(&mut self, job: &Job) -> tokio::io::Result<()> {
        for i in &mut self.all {
            i.call(job).await?
        }
        Ok(())
    }
}

pub struct Router_ {
    pub name: Arc<String>,
    pub handler: Arc<dyn Handler + Send + Sync>,
    table: Table,
    channels: HashMap<String, Channel>,
    conn_all: HashMap<u16, Conn>,
    cseq_all: HashMap<u16, ConnSeq>,
    cid_seq: u16,
}

impl Router_ {
    pub fn new(name: Arc<String>, handler: Arc<dyn Handler + Send + Sync>) -> Self {
        Self { name, table: Table::new(), handler, channels: HashMap::new(), conn_all: HashMap::new(), cseq_all: HashMap::new(), cid_seq: 0 }
    }

    pub fn select_channel(&mut self, name: &String) -> Option<Arc<Conn>> {
        let channel = self.channels.get_mut(name)?;
        let conn_id = channel.select()?;
        let conn = self.conn_all.get(&conn_id)?;
        Some(Arc::new(conn.clone()))
    }

    pub fn select_channel_must(&mut self, name: &String) -> tokio::io::Result<Arc<Conn>> {
        match self.select_channel(&name) {
            Some(conn) => Ok(conn),
            None => Err(new_message_err(format!("channel {} is not exists", name))),
        }
    }

    pub fn add_channel(&mut self, name: Arc<String>, conn_id: u16) {
        match self.channels.get_mut(&*name) {
            Some(c) => {
                c.add(conn_id);
            }
            None => {
                let mut c = Channel::new(name.clone());
                c.add(conn_id);
                self.channels.insert((*name).clone(), c);
            }
        }
    }

    pub fn remove_channel(&mut self, name: &Arc<String>, conn: &Conn) {
        match self.channels.get_mut(&**name) {
            Some(c) => {
                c.remove(&conn.id);
            }
            None => (),
        }
    }

    fn must_conn(&mut self, conn_id: &u16) -> tokio::io::Result<&mut Conn> {
        match self.conn_all.get_mut(conn_id) {
            Some(conn) => Ok(conn),
            None => Err(new_message_err(format!("conn {} is not exists", conn_id))),
        }
    }

    fn add_table(&mut self, from_conn: &Arc<Conn>, from_sid: ConnID, next_conn: &Arc<Conn>, next_sid: ConnID) {
        self.table.add(&from_conn, from_sid, &next_conn, next_sid);
    }

    fn next_table(&mut self, conn: &u16, sid: &ConnID, update: bool) -> Option<(Arc<Conn>, ConnID)> {
        let (next_id, _, next_sid) = self.table.next(conn, sid, update)?;
        let next = self.conn_all.get(&next_id)?;
        Some((Arc::new(next.clone()), next_sid))
    }

    fn remove_table(&mut self, conn: &u16, sid: &ConnID) -> Option<(Arc<Conn>, ConnID)> {
        let item = self.table.remove(conn, sid)?;
        let (next_id, _, next_sid) = item.next(conn)?;
        let next = self.conn_all.get(&next_id)?;
        Some((Arc::new(next.clone()), next_sid))
    }

    pub fn alloc_conn_id(&mut self, conn: &u16) -> tokio::io::Result<u8> {
        match self.cseq_all.get_mut(conn) {
            Some(seq) => seq.alloc_conn_id(),
            None => {
                let mut seq = ConnSeq::new();
                let v = seq.alloc_conn_id();
                self.cseq_all.insert(conn.clone(), seq);
                v
            }
        }
    }

    pub fn free_conn_id(&mut self, conn: &u16, lid: &u8) {
        match self.cseq_all.get_mut(conn) {
            Some(seq) => seq.free_conn_id(lid),
            None => (),
        }
    }

    pub async fn proc_conn_frame(&mut self, conn_id: &u16, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        match self.conn_all.get(conn_id) {
            Some(conn) => {
                let conn = conn.clone();
                debug!("Router({}) receive conn frame {} on channel {}", self.name, frame.to_string(), conn.to_string());
                match frame.cmd {
                    Cmd::None => self.proc_invalid_cmd(conn, frame).await,
                    Cmd::LoginBack => self.proc_invalid_cmd(conn, frame).await,
                    Cmd::LoginChannel => self.proc_login_channel(conn, frame).await,
                    Cmd::PingConn => self.proc_ping_conn(conn, frame).await,
                    Cmd::PingBack => self.proc_ping_back(conn, frame).await,
                    Cmd::DialConn => self.proc_dial_conn(conn, frame).await,
                    Cmd::DialBack => self.proc_dial_back(conn, frame).await,
                    Cmd::ConnData => self.proc_conn_data(conn, frame).await,
                    Cmd::ConnClosed => self.proc_conn_closed(conn, frame).await,
                }
            }
            None => Err(new_message_err(format!("conn {} is not exists", conn_id))),
        }
    }

    pub async fn proc_invalid_cmd(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        warn!("Router({}) the channel receive invalid cmd {:?} on {}", self.name, frame.cmd, conn.to_string());
        Err(new_message_err(format!("invalid cmd {:?}", frame.cmd)))
    }

    pub async fn proc_login_channel(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        let mut conn = conn;
        let args = frame.str();
        let (name, _) = self.handler.on_conn_login(&conn, &args).await?;
        let name = Arc::new(name);
        conn.name = name.clone();
        self.add_channel(name.clone(), conn.id.clone());
        info!("Router({}) the channel({}) is login success on {}", self.name, &name, conn.to_string());
        Ok(Task::message(Arc::new(conn), &frame.sid, &Cmd::LoginBack, CONN_OK.as_bytes()))
    }

    pub async fn proc_ping_conn(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        Ok(Task::message(Arc::new(conn), &frame.sid, &Cmd::PingBack, frame.data()))
    }

    pub async fn proc_ping_back(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        let start_time: i64 = match conn.header.byte_order {
            crate::frame::ByteOrder::BE => i64::from_be_bytes(frame.data().try_into().unwrap()),
            crate::frame::ByteOrder::LE => i64::from_le_bytes(frame.data().try_into().unwrap()),
        };
        let now_time = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_millis() as i64;
        let conn = self.must_conn(&conn.id)?; //for update
        conn.ping = Some(Ping::new(now_time - start_time, now_time));
        Ok(Task::new())
    }

    pub async fn proc_dial_conn(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        Ok(Task::message(Arc::new(conn), &frame.sid, &Cmd::DialBack, "not supported".as_bytes()))
    }

    pub async fn proc_dial_back(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        match frame.str().as_str() {
            CONN_OK => match self.next_table(&conn.id, &frame.sid, true) {
                Some((next, next_sid)) => match next.conn_type {
                    ConnType::Channel => {
                        //forward to next
                        info!("Router({}) proc dial back {}->{} success", self.name, next.to_string(), conn.to_string());
                        Ok(Task::message(next, &next_sid, &Cmd::DialBack, frame.data()))
                    }
                    ConnType::Raw => {
                        //ready to read
                        info!("Router({}) proc dial back {}->{} success", self.name, next.to_string(), conn.to_string());
                        Ok(Task::ready(next, None))
                    }
                },
                None => {
                    info!("Router({}) proc dial back fail with not router by {},{} on {}", self.name, conn.id, frame.sid.to_string(), conn.to_string());
                    let conn = Arc::new(conn.clone());
                    Ok(Task::message(conn, &frame.sid, &Cmd::ConnClosed, CONN_CLOSED.as_bytes()))
                }
            },
            m => match self.remove_table(&conn.id, &frame.sid) {
                Some((next, next_sid)) => match next.conn_type {
                    ConnType::Channel => {
                        //forward to next
                        info!("Router({}) proc dial back {}->{} fail with {}", self.name, next.to_string(), conn.to_string(), m);
                        Ok(Task::message(next, &next_sid, &Cmd::DialBack, frame.data()))
                    }
                    ConnType::Raw => {
                        //ready to read
                        info!("Router({}) proc dial back {}->{} fail with {}", self.name, next.to_string(), conn.to_string(), m);
                        Ok(Task::shutdown(next, Some(m.to_string())))
                    }
                },
                None => {
                    info!("Router({}) proc dial back fail with not router by {},{} on {}", self.name, conn.id, frame.sid.to_string(), conn.to_string());
                    Ok(Task::new())
                }
            },
        }
    }

    pub async fn proc_conn_data(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        match self.next_table(&conn.id, &frame.sid, false) {
            Some((next, next_sid)) => Ok(Task::message(next, &next_sid, &Cmd::ConnData, frame.data())),
            None => match conn.conn_type {
                ConnType::Channel => Ok(Task::message(Arc::new(conn), &frame.sid, &Cmd::ConnClosed, CONN_CLOSED.as_bytes())),
                ConnType::Raw => Ok(Task::shutdown(Arc::new(conn), Some(CONN_CLOSED.to_string()))),
            },
        }
    }

    pub async fn proc_conn_closed(&mut self, conn: Conn, frame: &mut Frame<'_>) -> tokio::io::Result<Task> {
        let message = frame.str();
        debug!("Router({}) the session({}) is closed by {}", self.name, frame.sid.to_string(), message);
        match self.remove_table(&conn.id, &frame.sid) {
            Some((next, next_sid)) => match next.conn_type {
                ConnType::Channel => {
                    //forward to next
                    info!("Router({}) proc conn closed {}->{} success by {}", self.name, next.to_string(), conn.to_string(), message);
                    Ok(Task::message(next, &next_sid, &Cmd::ConnClosed, frame.data()))
                }
                ConnType::Raw => {
                    //ready to read
                    info!("Router({}) proc conn closed {}->{} success by {}", self.name, next.to_string(), conn.to_string(), message);
                    Ok(Task::shutdown(next, Some(message)))
                }
            },
            None => {
                info!("Router({}) proc conn closed fail with not router by {},{} on {}", self.name, conn.id, frame.sid.to_string(), conn.to_string());
                Ok(Task::new())
            }
        }
    }

    pub async fn close_conn(&mut self, conn_id: &u16) -> Task {
        let mut task = Task::new();
        for item in self.table.remove_all_next(conn_id) {
            match item.next(conn_id) {
                Some((next_id, next_type, next_sid)) => {
                    if next_type == ConnType::Channel {
                        self.free_conn_id(&next_id, &next_sid.lid);
                    }
                    match self.conn_all.get_mut(&next_id) {
                        Some(next) => match next_type {
                            ConnType::Channel => task.more(TaskItem::message(Arc::new(next.clone()), &next_sid, &Cmd::ConnClosed, CONN_CLOSED.as_bytes())),
                            ConnType::Raw => {
                                task.more(TaskItem::shutdown(Arc::new(next.clone()), Some(CONN_CLOSED.to_string())));
                            }
                        },
                        None => (),
                    }
                }
                None => (),
            }
        }
        match self.conn_all.remove(conn_id) {
            Some(conn) => {
                if conn.conn_type == ConnType::Channel {
                    self.remove_channel(&conn.name, &conn)
                }
                self.handler.on_conn_close(&conn).await;
                task.more(TaskItem::shutdown(Arc::new(conn), Some(CONN_CLOSED.to_string())));
            }
            None => (),
        }
        self.cseq_all.remove(conn_id);
        task
    }

    pub fn list_channel_count(&self) -> HashMap<String, usize> {
        let mut counts = HashMap::new();
        for channel in self.channels.values() {
            counts.insert(channel.name.to_string(), channel.len());
        }
        counts
    }

    pub async fn shutdown(&mut self) -> Task {
        let mut task = Task::new();
        for conn in self.conn_all.values() {
            task.more(TaskItem::shutdown(Arc::new(conn.clone()), Some(CONN_CLOSED.to_string())))
        }
        self.table.clear();
        self.channels.clear();
        self.cseq_all.clear();
        self.conn_all.clear();
        task
    }

    pub async fn register(&mut self, conn: Conn) -> tokio::io::Result<Task> {
        if conn.name.is_empty() {
            return Err(new_message_err("conn name is empty"));
        }
        self.add_channel(conn.name.clone(), conn.id.clone());
        self.conn_all.insert(conn.id.clone(), conn.clone());
        Ok(Task::ready(Arc::new(conn), None))
    }

    pub fn new_conn_id(&mut self) -> u16 {
        self.cid_seq += 1;
        self.cid_seq
    }

    pub async fn dial_conn(&mut self, conn: Conn, uri: Arc<String>) -> tokio::io::Result<Task> {
        let parts: Vec<&str> = uri.splitn(2, "->").collect();
        if parts.len() < 2 {
            return Err(new_message_err("invalid uri"));
        }
        let mut conn = conn;
        let name = parts[0].to_string();
        let channel = self.select_channel_must(&name)?;
        let lid = self.alloc_conn_id(&channel.id)?;
        let channel_sid = ConnID::new(lid, 0);
        let conn_sid = ConnID::new(lid, lid);
        conn.name = Arc::new(format!("Raw{}", conn.id));
        conn.set_conn_id(conn_sid.clone()).await;
        debug!("Router({}) start dial({}-{}->{}-{}) to {} on channel({})", self.name, conn.id, conn_sid.to_string(), channel.id, channel_sid.to_string(), uri, channel.to_string());
        self.conn_all.insert(conn.id.clone(), conn.clone());
        let conn = Arc::new(conn);
        self.add_table(&channel, channel_sid.clone(), &conn, conn_sid);
        // match RouterForward::write_message(&channel, &channel_sid, &Cmd::DialConn, format!("{}|{}", parts[0], parts[1]).as_bytes()).await {
        //     Ok(_) => Ok(()),
        //     Err(e) => {
        //         self.remove_table(&channel, &channel_sid).await;
        //         channel.free_conn_id(&lid).await;
        //         conn.shutdown().await;
        //         Err(e)
        //     }
        // }
        Ok(Task::message(channel, &channel_sid, &Cmd::DialConn, format!("{}|{}", parts[0], parts[1]).as_bytes()))
    }

    pub async fn display(&self) -> json::JsonValue {
        // let mut channels = HashMap::new();
        // for v in self.channels.values() {
        //     channels.insert(v.name.clone(), v.display().await);
        // }
        json::object! {
            "name": self.name.to_string(),
            "table": self.table.display().await,
            // "channels": channels,
        }
    }
}

#[derive(Clone)]
pub struct Router {
    pub name: Arc<String>,
    pub header: Arc<frame::Header>,
    pub buffer_size: usize,
    pub job: Job,
    pub handler: Arc<dyn Handler + Send + Sync>,
    // pub forward: Arc<Mutex<RouterForward>>,
    router: Arc<Mutex<Router_>>,
}

impl Router {
    // pub async fn display(&self) -> Vec<String> {
    //     let mut item_all = Vec::new();
    //     for v in self.table.values() {
    //         item_all.push(v.lock().await.to_string());
    //     }
    //     item_all
    // }
    pub fn new(name: Arc<String>, handler: Arc<dyn Handler + Send + Sync>) -> Self {
        let mut h = frame::Header::new();
        h.length_field_length = 2;
        h.data_offset = 2;
        let header = Arc::new(h);
        let router = Arc::new(Mutex::new(Router_::new(name.clone(), handler.clone())));
        let job = Job::new(name.clone(), router.clone());
        Self { name, header, buffer_size: 2 * 1024, router, job, handler: handler }
    }

    pub async fn new_conn_id(&self) -> u16 {
        self.router.lock().await.new_conn_id()
    }

    pub async fn register(&self, conn: Conn) -> tokio::io::Result<Task> {
        self.router.lock().await.register(conn).await
    }

    pub async fn close_conn(&self, conn_id: &u16) -> Task {
        self.router.lock().await.close_conn(&conn_id).await
    }

    pub async fn join_base(&self, reader: Box<dyn frame::RawReader + Send + Sync>, writer: Box<dyn frame::RawWriter + Send + Sync>, option: Arc<JSON>) -> tokio::io::Result<()> {
        let frame_reader = frame::Reader::new(self.header.clone(), reader, self.buffer_size);
        let frame_writer = frame::Writer::new(self.header.clone(), writer);
        let id: u16 = self.new_conn_id().await;
        let conn_waiter = StateWaiter::new();
        let conn_reader = Arc::new(Mutex::new(ChannelReader::new(frame_reader, conn_waiter.clone()).await));
        let conn_writer = Arc::new(Mutex::new(ChannelWriter::new(frame_writer, conn_waiter.clone()).await));
        let conn = Conn::new(id, ConnType::Channel, self.header.clone(), conn_waiter, conn_reader, conn_writer);
        self.join_conn(conn, option).await
    }

    pub async fn join_conn(&self, conn: Conn, option: Arc<JSON>) -> tokio::io::Result<()> {
        info!("Router({}) start login join connection {} by options {:?}", self.name, conn.to_string(), option);
        let mut task = match self.join_call(conn.clone(), option).await {
            Ok(v) => Ok(v),
            Err(e) => {
                warn!("Router({}) login to {} fail with {}", self.name, conn.to_string(), e);
                conn.writer.lock().await.shutdown().await;
                Err(e)
            }
        }?;
        task.call(&self.job).await
    }

    async fn join_call(&self, conn: Conn, option: Arc<JSON>) -> tokio::io::Result<Task> {
        let login_option = serde_json::to_string(option.as_ref())?;
        let mut conn = conn;
        conn.write_message(&ConnID::zero(), &Cmd::LoginChannel, login_option.as_bytes()).await?;
        let message = conn.read_str().await?;
        let result: JSON = serde_json::from_str(&message)?;
        let result = Arc::new(result);
        let code = json_must_i64(&result, &"code")?;
        if code != 0 {
            return Err(new_message_err(message));
        }
        let name = String::from(json_must_str(&result, &"name")?);
        conn.name = Arc::new(name.clone());
        info!("Router({}) login to {} success, bind to {}", self.name, conn.to_string(), name);
        self.handler.on_conn_join(&conn, option, result).await;
        self.register(conn).await
    }

    pub async fn dial_base(&self, reader: Box<dyn frame::RawReader + Send + Sync>, writer: Box<dyn frame::RawWriter + Send + Sync>, uri: Arc<String>) -> tokio::io::Result<Conn> {
        let id: u16 = self.new_conn_id().await;
        let conn_waiter = StateWaiter::new();
        let conn_reader = Arc::new(Mutex::new(RawReader::new(self.header.clone(), reader, conn_waiter.clone(), self.buffer_size).await));
        let conn_writer = Arc::new(Mutex::new(RawWriter::new(self.header.clone(), writer, conn_waiter.clone()).await));
        let conn = Conn::new(id, ConnType::Raw, self.header.clone(), conn_waiter, conn_reader, conn_writer);
        self.dial_conn(conn.clone(), uri).await?;
        Ok(conn)
    }

    pub async fn dial_conn(&self, conn: Conn, uri: Arc<String>) -> tokio::io::Result<()> {
        let mut task = self.router.lock().await.dial_conn(conn.clone(), uri).await?;
        match task.call(&self.job).await {
            Ok(_) => Ok(()),
            Err(e) => {
                conn.shutdown().await;
                _ = self.close_conn(&conn.id).await.call(&self.job).await;
                Err(e)
            }
        }
    }

    pub async fn dial_socks(&self, reader: Box<dyn frame::RawReader + Send + Sync>, writer: Box<dyn frame::RawWriter + Send + Sync>, remote: Arc<String>) -> tokio::io::Result<()> {
        let mut reader = reader;
        let mut writer = writer;
        let mut buf = vec![0; 1024];

        let mut need;
        let mut readed;

        //header
        need = 2;
        readed = frame::read_full(&mut reader, &mut buf, 0, need).await?;
        if buf[0] != 0x05 {
            return Err(new_message_err("invalid version"));
        }
        need = buf[1] as usize;
        _ = frame::read_full(&mut reader, &mut buf, readed, need).await?;

        //respone auth
        _ = writer.write(&[0x05, 0x00]).await?;

        //
        readed = 0;
        need = 10;
        readed = frame::read_full(&mut reader, &mut buf, readed, need).await?;
        if buf[0] != 0x05 {
            return Err(new_message_err("invalid version"));
        }
        let uri: String;
        match buf[3] {
            0x01 => {
                let ip = format!("{}.{}.{}.{}", buf[4], buf[5], buf[6], buf[7]);
                let port = (buf[8] as u16) * 256 + (buf[9] as u16);
                uri = format!("tcp://{}:{}", ip, port);
            }
            0x03 => {
                need = buf[4] as usize + 2;
                readed = frame::read_full(&mut reader, &mut buf, readed, need).await?;
                let s = buf[4] as usize + 5;
                let remote = String::from_utf8_lossy(&buf[5..s]).to_string();
                let port = (buf[s] as u16) * 256 + (buf[s + 1] as u16);
                uri = format!("tcp://{}:{}", remote, port);
            }
            _ => {
                need = buf[4] as usize + 2;
                readed = frame::read_full(&mut reader, &mut buf, readed, need).await?;
                let l = buf[4] as usize + 2;
                let remote = String::from_utf8_lossy(&buf[5..l]).to_string();
                if remote.contains("://") {
                    uri = remote;
                } else {
                    uri = format!("tcp://{}", remote);
                }
            }
        }
        _ = readed;

        let target_uri = Arc::new(remote.replace("${HOST}", &uri));
        info!("start proxy {}", target_uri);
        let conn = self.dial_base(reader, writer, target_uri).await?;
        match conn.wait().await {
            Ok(_) => {
                let o = conn.header.data_offset + 3;
                let mut data = vec![0; o + 10];
                data[o] = 0x05;
                data[o + 3] = 0x01;
                let frame = Frame::new(&conn.header, &mut data, &ConnID::zero(), &Cmd::ConnData);
                _ = conn.write(&frame).await;
            }
            Err(_) => {
                let o = conn.header.data_offset + 3;
                let mut data = vec![0; o + 10];
                data[o] = 0x05;
                data[o + 1] = 0x04;
                data[o + 3] = 0x01;
                let frame = Frame::new(&conn.header, &mut data, &ConnID::zero(), &Cmd::ConnData);
                _ = conn.write(&frame).await;
            }
        }
        Ok(())
    }

    pub async fn wait(&self) {
        self.job.wait().await;
    }

    pub async fn list_channel_count(&self) -> HashMap<String, usize> {
        self.router.lock().await.list_channel_count()
    }

    pub async fn shutdown(&self) {
        info!("Router({}) is stopping", self.name);
        _ = self.router.lock().await.shutdown().await.call(&self.job).await;
    }

    pub async fn display(&self) -> json::JsonValue {
        self.router.lock().await.display().await
    }
}

pub struct NormalAcessEvent {
    pub action: HandlerAction,
    pub conn: Conn,
}

impl NormalAcessEvent {
    pub fn new(action: HandlerAction, conn: Conn) -> Self {
        Self { action, conn }
    }
}

pub struct NormalAcessHandler {
    pub sender: Option<Sender<NormalAcessEvent>>,
    pub send_raw: bool,
}

impl NormalAcessHandler {
    pub fn new() -> Self {
        Self { sender: None, send_raw: false }
    }
}

#[async_trait]
impl Handler for NormalAcessHandler {
    //on connection dial uri
    async fn on_conn_dial_uri(&self, _: &Conn, _: &String, _: &Vec<String>) -> tokio::io::Result<()> {
        Err(new_message_err("not supported"))
    }
    //on connection login
    async fn on_conn_login(&self, _: &Conn, _: &String) -> tokio::io::Result<(String, Arc<JSON>)> {
        Err(new_message_err("not supported"))
    }
    //on connection close
    async fn on_conn_close(&self, conn: &Conn) {
        if !self.send_raw && conn.conn_type == ConnType::Raw {
            return;
        }
        match &self.sender {
            Some(sender) => {
                let event = NormalAcessEvent::new(HandlerAction::Close, conn.clone());
                _ = sender.send_timeout(event, Duration::from_millis(500)).await;
            }
            None => (),
        }
    }
    //OnConnJoin is event on channel join
    async fn on_conn_join(&self, conn: &Conn, _: Arc<JSON>, _: Arc<JSON>) {
        match &self.sender {
            Some(sender) => {
                let event = NormalAcessEvent::new(HandlerAction::Join, conn.clone());
                _ = sender.send_timeout(event, Duration::from_millis(500)).await;
            }
            None => (),
        }
    }
}

pub struct NormalEventHandler {
    pub reciver: Receiver<NormalAcessEvent>,
    pub next: HashSet<Arc<dyn Handler + Send + Sync>>,
}

impl NormalEventHandler {
    pub fn new(reciver: Receiver<NormalAcessEvent>) -> Self {
        Self { reciver, next: HashSet::new() }
    }

    pub async fn receive(&mut self) {
        loop {
            match self.reciver.recv().await {
                Some(event) => {
                    for next in &self.next {
                        match event.action {
                            HandlerAction::Dial => (),
                            HandlerAction::Login => (),
                            HandlerAction::Close => next.on_conn_close(&event.conn).await,
                            HandlerAction::Join => next.on_conn_join(&event.conn, Arc::new(JSON::new()), Arc::new(JSON::new())).await,
                        }
                    }
                }
                None => break,
            }
        }
    }
}
