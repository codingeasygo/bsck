use std::{io::ErrorKind, sync::Arc};

use async_trait::async_trait;

#[derive(Clone)]
pub enum ByteOrder {
    BE,
    LE,
}

#[derive(Clone)]
pub struct Header {
    pub byte_order: ByteOrder,
    pub length_field_magic: usize,
    pub length_field_offset: usize,
    pub length_field_length: usize,
    pub length_adjustment: usize,
    pub data_offset: usize,
}

impl Header {
    pub fn new() -> Self {
        Self { byte_order: ByteOrder::BE, length_field_magic: 0, length_field_offset: 0, length_field_length: 4, length_adjustment: 0, data_offset: 4 }
    }

    // pub fn set_date_prefix(&mut self, sid: &ConnID, cmd: &RouterCmd) {
    //     self.data_prefix = vec![0; 3];
    //     self.data_prefix[0] = sid.lid.clone();
    //     self.data_prefix[1] = sid.rid.clone();
    //     self.data_prefix[3] = cmd.as_u8();
    // }

    // pub fn data_prefix_offset(&self) -> usize {
    //     self.data_offset + self.data_prefix.len()
    // }

    pub fn write_head(&self, buffer: &mut [u8]) {
        let n = buffer.len() + self.length_adjustment;
        let target = &mut buffer[self.length_field_offset..];
        match self.length_field_length {
            1 => target[0] = n as u8,
            2 => match self.byte_order {
                ByteOrder::BE => target[0..2].copy_from_slice(&(n as u16).to_be_bytes() as &[u8]),
                ByteOrder::LE => target[0..2].copy_from_slice(&(n as u16).to_le_bytes() as &[u8]),
            },
            4 => match self.byte_order {
                ByteOrder::BE => target[0..4].copy_from_slice(&(n as u32).to_be_bytes() as &[u8]),
                ByteOrder::LE => target[0..4].copy_from_slice(&(n as u32).to_le_bytes() as &[u8]),
            },
            _ => panic!("not supported lenght {}", self.length_field_length),
        }
        for i in 0..self.length_field_magic {
            target[i] = 0
        }
    }

    pub fn read_head(&self, buffer: &mut [u8]) -> usize {
        let target = &mut buffer[self.length_field_offset..];
        for i in 0..self.length_field_magic {
            target[i] = 0
        }
        match self.length_field_length {
            1 => target[0] as usize,
            2 => match self.byte_order {
                ByteOrder::BE => u16::from_be_bytes(target[0..2].try_into().unwrap()) as usize,
                ByteOrder::LE => u16::from_le_bytes(target[0..2].try_into().unwrap()) as usize,
            },
            4 => match self.byte_order {
                ByteOrder::BE => u32::from_be_bytes(target[0..4].try_into().unwrap()) as usize,
                ByteOrder::LE => u32::from_le_bytes(target[0..4].try_into().unwrap()) as usize,
            },
            _ => panic!("not supported lenght {}", self.length_field_length),
        }
    }
}

#[async_trait]
pub trait Reader {
    async fn read(&mut self, buf: &mut [u8]) -> tokio::io::Result<usize>;
}

#[async_trait]
pub trait Writer {
    async fn write(&mut self, buf: &[u8]) -> tokio::io::Result<usize>;
    async fn shutdown(&mut self);
}

pub struct FrameReader {
    pub header: Arc<Header>,
    pub inner: Box<dyn Reader + Send + Sync>,
    buf: Box<Vec<u8>>,
    filled: usize,
    readed: usize,
}

impl FrameReader {
    pub fn new(header: Arc<Header>, inner: Box<dyn Reader + Send + Sync>, buffer_size: usize) -> Self {
        if buffer_size < 1 {
            panic!("buffer size is {}", buffer_size);
        }
        Self { header, inner, buf: Box::new(vec![0; buffer_size]), filled: 0, readed: 0 }
    }

    pub async fn read(&mut self) -> tokio::io::Result<&mut [u8]> {
        let s = &mut *self;
        let header = &s.header;
        let sbuf = s.buf.as_mut();
        if s.filled > s.readed {
            sbuf.copy_within(s.readed..s.filled, 0);
        }
        s.filled -= s.readed;
        loop {
            let n = match s.inner.read(&mut sbuf[s.filled..]).await {
                Ok(n) => {
                    if n < 1 {
                        break Err(std::io::Error::new(ErrorKind::UnexpectedEof, "EOF"));
                    } else {
                        n
                    }
                }
                Err(e) => break Err(e),
            };
            s.filled += n;
            if s.filled < 3 {
                continue;
            }
            let h = header.read_head(sbuf);
            if s.filled >= h {
                s.readed = h;
                return Ok(&mut sbuf[0..h]);
            }
        }
    }
}

pub struct FrameWriter {
    pub header: Arc<Header>,
    pub inner: Box<dyn Writer + Send + Sync>,
}

impl FrameWriter {
    pub fn new(header: Arc<Header>, inner: Box<dyn Writer + Send + Sync>) -> Self {
        Self { header, inner }
    }

    pub async fn write(&mut self, frame: &[u8]) -> tokio::io::Result<usize> {
        self.inner.write(frame).await
    }

    pub async fn shutdown(&mut self) {
        self.inner.shutdown().await;
    }
}
