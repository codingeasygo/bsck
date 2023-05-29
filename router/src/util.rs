use std::{
    collections::HashMap,
    fs::File,
    io::{BufReader, ErrorKind},
    path::Path,
    sync::Arc,
    time::{SystemTime, UNIX_EPOCH},
};

use rustls::OwnedTrustAnchor;

pub type JSON = HashMap<String, serde_json::Value>;

pub fn new_message_err<E>(err: E) -> std::io::Error
where
    E: Into<Box<dyn std::error::Error + Send + Sync>>,
{
    std::io::Error::new(ErrorKind::Other, err)
}

pub fn json_must_i64(value: &Arc<JSON>, key: &str) -> tokio::io::Result<i64> {
    match value.get(key) {
        Some(v) => match v {
            serde_json::Value::Number(v) => match v.as_i64() {
                Some(v) => Ok(v),
                _ => Err(new_message_err(format!("read {} fail", key))),
            },
            _ => Err(new_message_err(format!("read {} fail", key))),
        },
        None => Err(new_message_err(format!("read {} fail", key))),
    }
}

pub fn json_option_i64(value: &Arc<JSON>, key: &str) -> Option<i64> {
    match value.get(key) {
        Some(v) => match v {
            serde_json::Value::Number(v) => v.as_i64(),
            _ => None,
        },
        None => None,
    }
}

pub fn json_must_f64(value: &Arc<JSON>, key: &str) -> tokio::io::Result<f64> {
    match value.get(key) {
        Some(v) => match v {
            serde_json::Value::Number(v) => match v.as_f64() {
                Some(v) => Ok(v),
                _ => Err(new_message_err(format!("read {} fail", key))),
            },
            _ => Err(new_message_err(format!("read {} fail", key))),
        },
        None => Err(new_message_err(format!("read {} fail", key))),
    }
}

pub fn json_option_f64(value: &Arc<JSON>, key: &str) -> Option<f64> {
    match value.get(key) {
        Some(v) => match v {
            serde_json::Value::Number(v) => v.as_f64(),
            _ => None,
        },
        None => None,
    }
}

pub fn json_must_str<'a>(value: &'a Arc<JSON>, key: &'a str) -> tokio::io::Result<&'a String> {
    match value.get(key) {
        Some(v) => match v {
            serde_json::Value::String(v) => Ok(v),
            _ => Err(new_message_err(format!("read {} fail", key))),
        },
        None => Err(new_message_err(format!("read {} fail", key))),
    }
}

pub fn json_option_str<'a>(value: &'a Arc<JSON>, key: &'a str) -> Option<&'a String> {
    match value.get(key) {
        Some(v) => match v {
            serde_json::Value::String(v) => Some(v),
            _ => None,
        },
        None => None,
    }
}

pub fn json_option_str_tuple<'a>(value: &'a Arc<JSON>, a: &'a str, b: &'a str) -> Option<(&'a String, &'a String)> {
    match json_option_str(value, a) {
        Some(av) => match json_option_str(value, b) {
            Some(bv) => Some((av, bv)),
            None => None,
        },
        None => None,
    }
}

pub fn now() -> i64 {
    SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_millis() as i64
}

pub fn option_must<T>(v: Option<T>, m: String) -> tokio::io::Result<T> {
    match v {
        Some(v) => Ok(v),
        None => Err(new_message_err(m)),
    }
}

pub fn read_certs(cert: &String, key: &String) -> tokio::io::Result<(Vec<rustls::Certificate>, rustls::PrivateKey)> {
    let mut cert_chain_reader = BufReader::new(File::open(cert)?);
    let certs = rustls_pemfile::certs(&mut cert_chain_reader)?.into_iter().map(rustls::Certificate).collect();
    let mut key_reader = BufReader::new(File::open(key)?);
    let mut keys = rustls_pemfile::pkcs8_private_keys(&mut key_reader)?;
    assert_eq!(keys.len(), 1);
    let key = rustls::PrivateKey(keys.remove(0));
    Ok((certs, key))
}

pub fn wrap_err<T, E>(result: Result<T, E>) -> tokio::io::Result<T>
where
    E: Into<Box<dyn std::error::Error + Send + Sync>>,
{
    match result {
        Ok(v) => Ok(v),
        Err(e) => Err(new_message_err(e)),
    }
}

pub struct SkipServerVerification;

impl SkipServerVerification {
    pub fn new() -> Arc<Self> {
        Arc::new(Self)
    }
}

impl rustls::client::ServerCertVerifier for SkipServerVerification {
    fn verify_server_cert(&self, _end_entity: &rustls::Certificate, _intermediates: &[rustls::Certificate], _server_name: &rustls::ServerName, _scts: &mut dyn Iterator<Item = &[u8]>, _ocsp_response: &[u8], _now: std::time::SystemTime) -> Result<rustls::client::ServerCertVerified, rustls::Error> {
        Ok(rustls::client::ServerCertVerified::assertion())
    }
}

pub fn check_join(dir: &Arc<String>, file: &String) -> String {
    if Path::new(file).is_absolute() {
        file.to_string()
    } else {
        match Path::new(dir.as_ref()).join(file).to_str() {
            Some(v) => v.to_string(),
            None => file.to_string(),
        }
    }
}

pub fn load_tls_config(dir: Arc<String>, option: &Arc<JSON>) -> tokio::io::Result<Arc<rustls::ClientConfig>> {
    let mut root_store = rustls::RootCertStore::empty();
    match json_option_str(&option, "tls_ca") {
        Some(ca) => {
            let ca = check_join(&dir, ca);
            let mut pem = BufReader::new(File::open(ca.as_str())?);
            let certs = rustls_pemfile::certs(&mut pem)?;
            let trust_anchors = certs.iter().map(|cert| {
                let ta = webpki::TrustAnchor::try_from_cert_der(&cert[..]).unwrap();
                OwnedTrustAnchor::from_subject_spki_name_constraints(ta.subject, ta.spki, ta.name_constraints)
            });
            root_store.add_server_trust_anchors(trust_anchors);
        }
        None => {
            root_store.add_server_trust_anchors(webpki_roots::TLS_SERVER_ROOTS.0.iter().map(|ta| OwnedTrustAnchor::from_subject_spki_name_constraints(ta.subject, ta.spki, ta.name_constraints)));
        }
    }

    let verify = match json_option_i64(&option, "tls_verify") {
        Some(v) => v > 0,
        None => true,
    };
    if verify {
        let builder = rustls::ClientConfig::builder().with_safe_defaults().with_root_certificates(root_store);
        let config = match json_option_str_tuple(&option, "tls_cert", "tls_key") {
            Some((cert, key)) => {
                let cert = check_join(&dir, cert);
                let key = check_join(&dir, key);
                let (cert, key) = read_certs(&cert, &key)?;
                match builder.with_single_cert(cert, key) {
                    Ok(v) => Ok(v),
                    Err(e) => Err(new_message_err(e)),
                }?
            }
            None => builder.with_no_client_auth(),
        };
        Ok(Arc::new(config))
    } else {
        let builder = rustls::ClientConfig::builder().with_safe_defaults().with_custom_certificate_verifier(SkipServerVerification::new());
        let config = builder.with_no_client_auth();
        Ok(Arc::new(config))
    }
}
