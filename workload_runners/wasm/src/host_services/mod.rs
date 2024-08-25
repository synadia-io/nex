use crate::{metadata::MachineMetadata, Result, WorkloadInfo};

use async_nats::Client;

use log::debug;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

mod builtins;

const HEADER_CODE: &str = "x-nex-hs-code";
const HEADER_MESSAGE: &str = "x-nex-hs-message";

#[derive(Debug, Clone)]
pub struct HostServicesClient {
    pub nc: Client,
    pub metadata: MachineMetadata,
    pub workload_info: WorkloadInfo,
}

impl HostServicesClient {
    pub fn new(nc: Client, metadata: MachineMetadata, workload_info: WorkloadInfo) -> Self {
        HostServicesClient {
            metadata,
            nc,
            workload_info,
        }
    }

    pub async fn perform_rpc(
        &self,
        service: &str,
        method: &str,
        payload: Vec<u8>,
        metadata: HashMap<String, String>,
    ) -> Result<HostServicesReply> {
        let subject = format!(
            "hostint.{}.rpc.{}.{}.{}.{}",
            self.workload_info.vm_id,
            self.workload_info.namespace,
            self.workload_info.workload_name,
            service,
            method
        );
        debug!("Performing RPC on '{}'", subject);
        let mut hm = async_nats::HeaderMap::new();
        for (k, v) in metadata {
            hm.insert(k.as_ref(), v.as_ref())
        }
        let res = self
            .nc
            .request_with_headers(subject, hm, payload.into())
            .await?;
        let r = res
            .headers
            .as_ref()
            .and_then(|h| h.get(HEADER_CODE))
            .map(|hv| str::parse::<u16>(hv.as_str()).unwrap_or(0))
            .unwrap_or_default();

        let m = res
            .headers
            .as_ref()
            .and_then(|hm| hm.get(HEADER_MESSAGE))
            .map(|v| v.as_str())
            .unwrap_or("");

        let sr = HostServicesReply {
            code: r,
            message: m.to_owned(),
            data: Some(res.payload.to_vec()),
        };

        Ok(sr)
    }
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct HostServicesReply {
    code: u16,
    message: String,
    data: Option<Vec<u8>>,
}

impl HostServicesReply {
    pub fn is_error(&self) -> bool {
        self.code != 200
    }
}
