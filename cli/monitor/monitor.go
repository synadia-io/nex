package monitor

import (
	"github.com/synadia-io/nex/cli/monitor/event"
	"github.com/synadia-io/nex/cli/monitor/log"
)

type MonitorOptions struct {
	Log   log.LogCmd     `cmd:"" help:"Live monitor workload log emissions" aliases:"logs"`
	Event event.EventCmd `cmd:"" help:"Live monitor events from nex nodes" aliases:"events"`

	NodeId     string `placeholder:"SNEXISCOOL..." help:"Node to monitor" group:"Monitor Configuration"`
	WorkloadId string `placeholder:"<workload_id>" help:"Workload to monitor" group:"Monitor Configuration"`
	Level      string `default:"info" enum:"debug,warn,info,error" help:"Level of logs to monitor" group:"Monitor Configuration"`
}
