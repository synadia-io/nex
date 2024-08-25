use std::env;

use async_nats::ConnectOptions;
use env_logger::{Builder, Env};
use log::info;
use nex_runner_wasm::{
    host_services::HostServicesClient, metadata::get_metadata, runner::new_runner, Result,
    WorkloadInfo,
};

#[tokio::main]
async fn main() -> Result<()> {
    init_logger();

    info!("Wasm runner");
    let args: Vec<String> = env::args().collect();
    if args.len() != 4 {
        info!("Invalid parameters {args:?}\nUsage: wasm {{namespace}} {{workload_name}} {{wasm_path}}\n");
        return Ok(());
    }
    let md = get_metadata().await?;
    let wi: WorkloadInfo = WorkloadInfo {
        vm_id: md.vm_id.to_string(),
        artifact_path: args[3].to_string(),
        namespace: args[1].to_string(),
        workload_name: args[2].to_string(),
    };

    let pair = nkeys::KeyPair::from_seed(&md.node_nats_nkey.clone().unwrap())?;
    info!("Connecting to internal NATS as user {}", pair.public_key());

    let connection_options = ConnectOptions::new().nkey(md.node_nats_nkey.clone().unwrap());
    let url = format!(
        "nats://{}:{}",
        md.node_nats_host,
        md.node_nats_port.unwrap_or_default()
    );

    let client = match async_nats::connect_with_options(url, connection_options).await {
        Ok(client) => client,
        Err(e) => {
            println!("failed to connect to nats internal server: {e}");
            panic!("{}", e)
        }
    };

    info!(
        "Connected to {0}:{1} for internal NATS ({2})",
        md.node_nats_host,
        md.node_nats_port.unwrap_or_default(),
        md.node_nats_nkey.clone().unwrap_or_default()
    );

    let hs_client = HostServicesClient::new(client.clone(), md.clone(), wi.clone());
    let mut runner = new_runner(hs_client, wi.artifact_path.clone()).await?;
    info!(
        "WebAssembly workload runner for Nex - started, vmid={0}, workload={1}",
        wi.vm_id, wi.workload_name
    );
    runner.start().await?;

    Ok(())
}

fn init_logger() {
    let env = Env::default()
        .filter_or(
            "NEXRUNNER_LOG_LEVEL",
            "debug, cranelift_codegen=error, wasmtime=error, wasmtime_cranelift=error",
        )
        .write_style_or("NEXAGENT_LOG_STYLE", "never");

    Builder::from_env(env)
        .default_format()
        .format_target(false)
        .format_level(true)
        .format_timestamp_secs()
        .init();
}
