use serde::{Deserialize, Serialize};

pub type Result<T> = anyhow::Result<T>;

pub mod host_services;
pub mod metadata;
pub mod runner;

#[derive(Serialize, Deserialize, Clone, Debug)]
pub struct WorkloadInfo {
    pub vm_id: String,
    pub artifact_path: String,
    pub namespace: String,
    pub workload_name: String,
}
