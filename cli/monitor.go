package main

type monitorOptions struct {
	Log   monitorLogCmd   `cmd:"" help:"Live monitor workload log emissions" aliases:"logs"`
	Event monitorEventCmd `cmd:"" help:"Live monitor events from nex nodes" aliases:"events"`

	NodeId     string `placeholder:"SNEXISCOOL..." help:"Node to monitor" group:"Monitor Configuration"`
	WorkloadId string `placeholder:"<workload_id>" help:"Workload to monitor" group:"Monitor Configuration"`
	Level      string `default:"info" enum:"debug,warn,info,error" help:"Level of logs to monitor" group:"Monitor Configuration"`
}
