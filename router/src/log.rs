use chrono::Local;
use log::{LevelFilter, Metadata, Record, SetLoggerError};
use std::path::Path;

pub struct SimpleLogger;

impl log::Log for SimpleLogger {
    fn enabled(&self, _: &Metadata) -> bool {
        true
    }

    fn log(&self, record: &Record) {
        if self.enabled(record.metadata()) {
            let file = match record.file() {
                Some(n) => Path::new(n),
                None => Path::new("_"),
            };
            let filename = file.file_name().unwrap().to_str().unwrap();
            let line = match record.line() {
                Some(n) => n,
                None => 0,
            };
            let level = match record.level() {
                log::Level::Error => "E",
                log::Level::Warn => "W",
                log::Level::Info => "I",
                log::Level::Debug => "D",
                log::Level::Trace => "T",
            };
            let time = Local::now();
            println!("{} {}:{} {} - {}", time.format("%Y/%m/%d %H:%M:%S%.3f"), filename, line, level, record.args());
        }
    }

    fn flush(&self) {}
}

static LOGGER: SimpleLogger = SimpleLogger;

pub fn init_simple_log() -> Result<(), SetLoggerError> {
    log::set_logger(&LOGGER).map(|()| log::set_max_level(LevelFilter::Info))
}
