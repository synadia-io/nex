package up

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (u UpCmd) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Node Up Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"Agent Handshake Timeout Millisecond", u.AgentHandshakeTimeoutMillisecond, reflect.TypeOf(u.AgentHandshakeTimeoutMillisecond).String()},
		{"Default Resource Directory", u.DefaultResourceDir, reflect.TypeOf(u.DefaultResourceDir).String()},
		{"Internal Node NATS Host", u.InternalNodeHost, reflect.TypeOf(u.InternalNodeHost).String()},
		{"Internal Node NATS Port", u.InternalNodePort, reflect.TypeOf(u.InternalNodePort).String()},
		{"Machine Pool Size", u.MachinePoolSize, reflect.TypeOf(u.MachinePoolSize).String()},
		{"No Sandbox", u.NoSandbox, reflect.TypeOf(u.NoSandbox).String()},
		{"Preserve Network", u.PreserveNetwork, reflect.TypeOf(u.PreserveNetwork).String()},
		{"Kernel Filepath", u.KernelFilepath, reflect.TypeOf(u.KernelFilepath).String()},
		{"RootFs Filepath", u.RootFsFilepath, reflect.TypeOf(u.RootFsFilepath).String()},
		{"Tags", u.Tags, reflect.TypeOf(u.Tags).String()},
		{"Valid Issuers", u.ValidIssuers, reflect.TypeOf(u.ValidIssuers).String()},
		{"Workload Type", u.WorkloadType, reflect.TypeOf(u.WorkloadType).String()},
		{"CNI Bin Paths", u.CniBinPaths, reflect.TypeOf(u.CniBinPaths).String()},
		{"CNI Interface Name", u.CniInterfaceName, reflect.TypeOf(u.CniInterfaceName).String()},
		{"CNI Network Name", u.CniNetworkName, reflect.TypeOf(u.CniNetworkName).String()},
		{"CNI Subnet", u.CniSubnet, reflect.TypeOf(u.CniSubnet).String()},
		{"Firecracker Vcpu Count", u.FirecrackerVcpuCount, reflect.TypeOf(u.FirecrackerVcpuCount).String()},
		{"Firecracker Mem Size Mib", u.FirecrackerMemSizeMib, reflect.TypeOf(u.FirecrackerMemSizeMib).String()},
		{"Bandwidth One Time Burst", u.Limiters.Bandwidth.OneTimeBurst, reflect.TypeOf(u.Limiters.Bandwidth.OneTimeBurst).String()},
		{"Bandwidth Refill Time", u.Limiters.Bandwidth.RefillTime, reflect.TypeOf(u.Limiters.Bandwidth.RefillTime).String()},
		{"Bandwidth Size", u.Limiters.Bandwidth.Size, reflect.TypeOf(u.Limiters.Bandwidth.Size).String()},
		{"Operations One Time Burst", u.Limiters.Operations.OneTimeBurst, reflect.TypeOf(u.Limiters.Operations.OneTimeBurst).String()},
		{"Operations Refill Time", u.Limiters.Operations.RefillTime, reflect.TypeOf(u.Limiters.Operations.RefillTime).String()},
		{"Operations Size", u.Limiters.Operations.Size, reflect.TypeOf(u.Limiters.Operations.Size).String()},
		{"OpenTelemetry Metrics Enabled", u.OtelMetrics, reflect.TypeOf(u.OtelMetrics).String()},
		{"OpenTelemetry Metrics Port", u.OtelMetricsPort, reflect.TypeOf(u.OtelMetricsPort).String()},
		{"OpenTelemetry Metrics Exporter", u.OtelMetricsExporter, reflect.TypeOf(u.OtelMetricsExporter).String()},
		{"OpenTelemetry Traces Enabled", u.OtelTraces, reflect.TypeOf(u.OtelTraces).String()},
		{"OpenTelemetry Traces Exporter", u.OtelTracesExporter, reflect.TypeOf(u.OtelTracesExporter).String()},
		{"OpenTelemetry Exporter URL", u.OtlpExporterUrl, reflect.TypeOf(u.OtlpExporterUrl).String()},
	})
	fmt.Println(tw.Render())
	return nil
}
