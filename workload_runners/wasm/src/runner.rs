use crate::host_services::HostServicesClient;
use futures::StreamExt;
use log::{debug, info, trace};
use std::time::Instant;

use anyhow::{anyhow, Context};
use async_trait::async_trait;
use nexagent::wasm::blobstore::ObjectInfo;
use nexagent::wasm::{blobstore, keyvalue, messaging};
use wasmtime::{component::*, Config};
use wasmtime::{Engine, Store};
use wasmtime_wasi::{WasiCtx, WasiView};

bindgen!({async: true});

struct ComponentCtx {
    table: ResourceTable,
    wasi: WasiCtx,
    hs_client: HostServicesClient,
}

pub struct WasmRunner {
    engine: Engine,
    component: Component,
    linker: Linker<ComponentCtx>,
    hs_client: HostServicesClient,
}

impl WasiView for ComponentCtx {
    fn table(&mut self) -> &mut ResourceTable {
        &mut self.table
    }

    fn ctx(&mut self) -> &mut WasiCtx {
        &mut self.wasi
    }
}

pub async fn new_runner(
    hs_client: HostServicesClient,
    file_path: String,
) -> crate::Result<WasmRunner> {
    let mut config = Config::new();
    config.async_support(true);
    let engine = Engine::new(&config)?;

    let component = Component::from_file(&engine, file_path)?;
    let mut linker = Linker::new(&engine);
    wasmtime_wasi::add_to_linker_async(&mut linker)?;
    NexComponent::add_to_linker(&mut linker, |state: &mut ComponentCtx| state)?;

    Ok(WasmRunner {
        linker,
        component,
        engine,
        hs_client: hs_client.clone(),
    })
}

impl WasmRunner {
    pub async fn start(&mut self) -> crate::Result<()> {
        let subject = format!("agentint.{}.trigger", self.hs_client.workload_info.vm_id);
        let mut sub = self.hs_client.nc.subscribe(subject.clone()).await?;
        info!("Workload runner now listening on {}", subject);
        while let Some(message) = sub.next().await {
            debug!("Executing trigger on '{}'", message.subject);
            let table = ResourceTable::new();
            let wasi = WasiCtx::builder().inherit_stderr().build();

            let ctx = ComponentCtx {
                table,
                wasi,
                hs_client: self.hs_client.clone(),
            };

            let mut store = Store::new(&self.engine, ctx);
            let bindings =
                NexComponent::instantiate_async(&mut store, &self.component, &self.linker).await?;

            let start = Instant::now();
            let res = bindings
                .nexagent_wasm_nexfunction()
                .call_run(store, &message.payload)
                .await?
                .map_err(|e| anyhow!(e))
                .context("failed to execute wasm function")?;
            let nanos = start.elapsed().as_nanos();

            if let Some(reply) = message.reply {
                let mut headers = async_nats::HeaderMap::new();
                let ns: &str = &nanos.to_string();
                headers.insert("x-nex-runtime-ns", ns);

                self.hs_client
                    .nc
                    .publish_with_headers(reply, headers, res.into())
                    .await?;
            }
        }
        Ok(())
    }
}

#[async_trait]
impl keyvalue::Host for ComponentCtx {
    async fn set(
        &mut self,
        bucket: String,
        key: String,
        value: String,
    ) -> ::std::result::Result<bool, String> {
        trace!("Keyvalue set: {}, {}, {}", bucket, key, value);

        self.hs_client
            .kv_set(bucket, key, value.as_bytes().to_vec())
            .await
            .map_err(|e| format!("{}", e))?;

        Ok(true)
    }

    async fn get(&mut self, bucket: String, key: String) -> Result<Vec<u8>, String> {
        trace!("Keyvalue get: {}, {}", bucket, key);
        self.hs_client
            .kv_get(bucket, key)
            .await
            .map_err(|e| format!("{}", e))
    }

    async fn delete(&mut self, bucket: String, key: String) -> Result<bool, String> {
        trace!("Keyvalue delete: {}, {}", bucket, key);
        self.hs_client
            .kv_delete(bucket, key)
            .await
            .map_err(|e| format!("{}", e))
            .map(|_| true) // right now the kv host service always returns true if the call suceeded
    }

    async fn keys(&mut self, bucket: String) -> Result<Vec<String>, String> {
        trace!("Keyvalue list keys: {}", bucket);
        self.hs_client
            .kv_keys(bucket)
            .await
            .map_err(|e| format!("{}", e))
    }
}

#[async_trait]
impl blobstore::Host for ComponentCtx {
    async fn get(&mut self, bucket: String, key: String) -> Result<Vec<u8>, String> {
        trace!("Blob get: {}, {}", bucket, key);
        self.hs_client
            .blob_get(bucket, key)
            .await
            .map_err(|e| format!("{}", e))
    }

    async fn items(&mut self, bucket: String) -> Result<Vec<ObjectInfo>, String> {
        trace!("Blob list: {}", bucket);
        Ok(self
            .hs_client
            .blob_list(bucket)
            .await
            .map_err(|e| format!("{}", e))?
            .into_iter()
            .map(|oi| ObjectInfo {
                nuid: oi.nuid,
                size: oi.size as u64,
                digest: oi.digest.unwrap_or_default(),
                bucket: oi.bucket,
            })
            .collect())
    }

    async fn put(
        &mut self,
        bucket: String,
        key: String,
        payload: Vec<u8>,
    ) -> Result<ObjectInfo, String> {
        trace!("Blob put: {}, {} {} bytes", bucket, key, payload.len());
        self.hs_client
            .blob_put(bucket, key, payload)
            .await
            .map_err(|e| format!("{}", e))
            .map(|oi| ObjectInfo {
                nuid: oi.nuid,
                size: oi.size as u64,
                digest: oi.digest.unwrap_or_default(),
                bucket: oi.bucket,
            })
    }

    async fn delete(&mut self, bucket: String, key: String) -> Result<bool, String> {
        trace!("Blob delete: {bucket}, {key}");
        self.hs_client
            .blob_delete(bucket, key)
            .await
            .map_err(|e| format!("{e}"))
            .map(|_| true)
    }
}

#[async_trait]
impl messaging::Host for ComponentCtx {
    async fn publish(&mut self, subject: String, payload: Vec<u8>) -> Result<bool, String> {
        todo!()
    }

    async fn request(&mut self, subject: String, payload: Vec<u8>) -> Result<Vec<u8>, String> {
        todo!()
    }
}
