package main

type AgentConfig struct {
	Uri  string            `name:"uri" help:"URI to the agent binary to download and install in resource directory" placeholder:"nats://bucket/key"`
	Argv []string          `name:"argv" help:"Arguments to pass to the agent on start" placeholder:"--config=/tmp/file"`
	Env  map[string]string `name:"env" help:"Environment variables to pass to the agent on start" placeholder:"NEX_NODE_ID=1234"`
}
type AgentConfigs []AgentConfig
