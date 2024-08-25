use std::env;

use anyhow::Context;
use serde::{Deserialize, Serialize};

use crate::Result;

const NEX_ENV_WORKLOAD_ID: &str = "NEX_WORKLOADID";
const NEX_ENV_NODE_NATS_HOST: &str = "NEX_NODE_NATS_HOST";
const NEX_ENV_NODE_NATS_PORT: &str = "NEX_NODE_NATS_PORT";
const NEX_ENV_NODE_NATS_SEED: &str = "NEX_NODE_NATS_NKEY_SEED";

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct MachineMetadata {
    pub vm_id: String,
    pub node_nats_host: String,
    pub node_nats_port: Option<i32>,
    pub node_nats_nkey: Option<String>,
}

pub async fn get_metadata() -> Result<MachineMetadata> {
    Ok(MachineMetadata {
        vm_id: env::var(NEX_ENV_WORKLOAD_ID).context("env variable: vm_id")?,
        node_nats_host: env::var(NEX_ENV_NODE_NATS_HOST).context("env variable: nats host")?,
        node_nats_port: env::var(NEX_ENV_NODE_NATS_PORT)
            .ok()
            .map(|s| s.parse::<i32>().unwrap_or(4222)),
        node_nats_nkey: env::var(NEX_ENV_NODE_NATS_SEED).ok(),
    })
}
