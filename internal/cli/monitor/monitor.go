package monitor

type sharedMonitorOptions struct {
	NodeId     string `placeholder:"SNEXISCOOL..." help:"Node to monitor" group:"Monitor Configuration"`
	WorkloadId string `placeholder:"<workload_id>" help:"Workload to monitor" group:"Monitor Configuration"`
	Level      string `default:"info" enum:"debug,warn,info,error" help:"Level of logs to monitor" group:"Monitor Configuration"`
}

type MonitorOptions struct {
	Log   LogCmd   `cmd:"" help:"Live monitor workload log emissions" aliases:"logs"`
	Event EventCmd `cmd:"" help:"Live monitor events from nex nodes" aliases:"events"`
}
