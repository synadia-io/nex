use std::collections::HashMap;

use anyhow::bail;
use async_nats::jetstream::object_store::ObjectInfo;

use crate::host_services::builtins::{
    HEADER_BUCKET, HEADER_OBJECT_NAME, SERVICE_NAME_OBJECT_STORE,
};
use crate::host_services::HostServicesClient;
use crate::Result;

const METHOD_GET: &str = "get";
const METHOD_LIST: &str = "list";
const METHOD_PUT: &str = "put";
const METHOD_DELETE: &str = "delete";

impl HostServicesClient {
    pub async fn blob_get(&self, bucket: String, key: String) -> Result<Vec<u8>> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_OBJECT_NAME.to_owned(), key);
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_OBJECT_STORE, METHOD_GET, Vec::new(), hm)
            .await?;
        if res.is_error() {
            bail!("failed to perform blob get: {}", res.message);
        }

        Ok(res.data.unwrap_or_default())
    }

    pub async fn blob_put(&self, bucket: String, key: String, data: Vec<u8>) -> Result<ObjectInfo> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_OBJECT_NAME.to_owned(), key);
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_OBJECT_STORE, METHOD_PUT, data, hm)
            .await?;
        if res.is_error() {
            bail!("failed to perform blob put: {}", res.message);
        }

        let Some(data) = res.data else {
            bail!("no data returned from blob put");
        };
        let info: ObjectInfo = serde_json::from_slice(&data)?;
        Ok(info)
    }

    pub async fn blob_delete(&self, bucket: String, key: String) -> Result<()> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_OBJECT_NAME.to_owned(), key);
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_OBJECT_STORE, METHOD_DELETE, Vec::new(), hm)
            .await?;
        if res.is_error() {
            bail!("failed to perform blob delete: {}", res.message);
        }

        // don't current use the return value, which is always a hardcoded "success" field.
        Ok(())
    }

    pub async fn blob_list(&self, bucket: String) -> Result<Vec<ObjectInfo>> {
        let mut hm = HashMap::new();
        hm.insert(HEADER_BUCKET.to_owned(), bucket);

        let res = self
            .perform_rpc(SERVICE_NAME_OBJECT_STORE, METHOD_LIST, Vec::new(), hm)
            .await?;
        if res.is_error() {
            bail!("failed to perform blob list: {}", res.message);
        }

        let Some(bytes) = res.data else {
            bail!("no data returned from object store list");
        };

        // TODO: use a non-jetstream-coupled type for this in the future
        let obj_info: Vec<ObjectInfo> = serde_json::from_slice(&bytes)?;
        Ok(obj_info)
    }
}
