use std::{arch::aarch64::int64x1_t, collections::HashMap};

use anyhow::bail;
use serde::{Deserialize, Serialize};

use crate::{host_services::HostServicesClient, Result};

use super::{HEADER_BUCKET, HEADER_KEYVALUE_KEY, SERVICE_NAME_KEYVALUE};

const METHOD_GET: &str = "get";
const METHOD_SET: &str = "set";
const METHOD_DELETE: &str = "delete";
const METHOD_KEYS: &str = "keys";

#[derive(Serialize, Deserialize, Clone, Debug)]
struct KeyValueResponse {
    #[serde(default)]
    revision: i64,
    #[serde(default)]
    success: bool,
    #[serde(default)]
    errors: Vec<String>,
}

impl HostServicesClient {
    pub async fn kv_get(&self, bucket: String, key: String) -> Result<Vec<u8>> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_KEYVALUE_KEY.to_owned(), key);
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_KEYVALUE, METHOD_GET, Vec::new(), hm)
            .await?;
        if res.is_error() {
            bail!("failed to perform KV get: {}", res.message);
        }

        Ok(res.data.unwrap_or_default())
    }

    pub async fn kv_set(&self, bucket: String, key: String, value: Vec<u8>) -> Result<()> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_KEYVALUE_KEY.to_owned(), key);
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_KEYVALUE, METHOD_SET, value, hm)
            .await?;

        if res.is_error() {
            bail!("failed to perform KV set: {}", res.message);
        }

        Ok(())
    }

    pub async fn kv_delete(&self, bucket: String, key: String) -> Result<()> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_KEYVALUE_KEY.to_owned(), key);
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_KEYVALUE, METHOD_DELETE, Vec::new(), hm)
            .await?;

        if res.is_error() {
            bail!("failed to perform KV set: {}", res.message);
        }
        Ok(())
    }

    pub async fn kv_keys(&self, bucket: String) -> Result<Vec<String>> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_KEYVALUE, METHOD_KEYS, Vec::new(), hm)
            .await?;

        if res.is_error() {
            bail!("failed to perform KV list keys: {}", res.message);
        }

        let keys: Vec<String> = match res.data {
            Some(d) => serde_json::from_slice(&d)?,
            None => Vec::new(),
        };

        Ok(keys)
    }
}
