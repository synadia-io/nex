package node

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (i InfoCmd) Table() error {
	return nil
}

func (l ListCmd) Table() error {
	return nil
}

func (p PreflightCmd) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Node Preflight Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"Force Dependency Install", p.ForceDepInstall, reflect.TypeOf(p.ForceDepInstall).String()},
	})
	fmt.Println(tw.Render())
	return nil
}

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
		{"Workload Types", u.WorkloadTypes, reflect.TypeOf(u.WorkloadTypes).String()},
		{"CNI Bin Paths", u.CNIDefinition.CniBinPaths, reflect.TypeOf(u.CNIDefinition.CniBinPaths).String()},
		{"CNI Interface Name", u.CNIDefinition.CniInterfaceName, reflect.TypeOf(u.CNIDefinition.CniInterfaceName).String()},
		{"CNI Network Name", u.CNIDefinition.CniNetworkName, reflect.TypeOf(u.CNIDefinition.CniNetworkName).String()},
		{"CNI Subnet", u.CNIDefinition.CniSubnet, reflect.TypeOf(u.CNIDefinition.CniSubnet).String()},
		{"Firecracker Vcpu Count", u.MachineTemplate.FirecrackerVcpuCount, reflect.TypeOf(u.MachineTemplate.FirecrackerVcpuCount).String()},
		{"Firecracker Mem Size Mib", u.MachineTemplate.FirecrackerMemSizeMib, reflect.TypeOf(u.MachineTemplate.FirecrackerMemSizeMib).String()},
		{"Bandwidth One Time Burst", u.Limiters.Bandwidth.OneTimeBurst, reflect.TypeOf(u.Limiters.Bandwidth.OneTimeBurst).String()},
		{"Bandwidth Refill Time", u.Limiters.Bandwidth.RefillTime, reflect.TypeOf(u.Limiters.Bandwidth.RefillTime).String()},
		{"Bandwidth Size", u.Limiters.Bandwidth.Size, reflect.TypeOf(u.Limiters.Bandwidth.Size).String()},
		{"Operations One Time Burst", u.Limiters.Operations.OneTimeBurst, reflect.TypeOf(u.Limiters.Operations.OneTimeBurst).String()},
		{"Operations Refill Time", u.Limiters.Operations.RefillTime, reflect.TypeOf(u.Limiters.Operations.RefillTime).String()},
		{"Operations Size", u.Limiters.Operations.Size, reflect.TypeOf(u.Limiters.Operations.Size).String()},
		{"OpenTelemetry Metrics Enabled", u.OtelConfig.OtelMetrics, reflect.TypeOf(u.OtelConfig.OtelMetrics).String()},
		{"OpenTelemetry Metrics Port", u.OtelConfig.OtelMetricsPort, reflect.TypeOf(u.OtelConfig.OtelMetricsPort).String()},
		{"OpenTelemetry Metrics Exporter", u.OtelConfig.OtelMetricsExporter, reflect.TypeOf(u.OtelConfig.OtelMetricsExporter).String()},
		{"OpenTelemetry Traces Enabled", u.OtelConfig.OtelTraces, reflect.TypeOf(u.OtelConfig.OtelTraces).String()},
		{"OpenTelemetry Traces Exporter", u.OtelConfig.OtelTracesExporter, reflect.TypeOf(u.OtelConfig.OtelTracesExporter).String()},
		{"OpenTelemetry Exporter URL", u.OtelConfig.OtlpExporterUrl, reflect.TypeOf(u.OtelConfig.OtlpExporterUrl).String()},
	})
	fmt.Println(tw.Render())
	return nil
}
