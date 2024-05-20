package main

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (g globals) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Global Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRow(table.Row{"Config File", g.Config, reflect.TypeOf(g.Config).String()})
	tw.AppendRow(table.Row{"Nex Namespace", g.Namespace, reflect.TypeOf(g.Namespace).String()})
	tw.AppendRow(table.Row{"NATS Server", g.Server, reflect.TypeOf(g.Server).String()})
	tw.AppendRow(table.Row{"NATS Context", g.NatsContext, reflect.TypeOf(g.NatsContext).String()})
	tw.AppendRow(table.Row{"NATS Creds", g.Creds, reflect.TypeOf(g.Creds).String()})
	tw.AppendRow(table.Row{"NATS TLS Cert", g.TlsCert, reflect.TypeOf(g.TlsCert).String()})
	tw.AppendRow(table.Row{"NATS TLS Key", g.TlsKey, reflect.TypeOf(g.TlsKey).String()})
	tw.AppendRow(table.Row{"NATS TLS CA", g.TlsCA, reflect.TypeOf(g.TlsCA).String()})
	tw.AppendRow(table.Row{"NATS TLS First", g.TlsFirst, reflect.TypeOf(g.TlsFirst).String()})
	tw.AppendRow(table.Row{"NATS Timeout", g.Timeout, reflect.TypeOf(g.Timeout).String()})
	tw.AppendRow(table.Row{"NATS Connection Name", g.ConnectionName, reflect.TypeOf(g.ConnectionName).String()})
	tw.AppendRow(table.Row{"NATS Username", g.Username, reflect.TypeOf(g.Username).String()})
	tw.AppendRow(table.Row{"NATS Password", g.Password, reflect.TypeOf(g.Password).String()})
	tw.AppendRow(table.Row{"NATS Nkey", g.Nkey, reflect.TypeOf(g.Nkey).String()})
	tw.AppendRow(table.Row{"NATS Socks Proxy", g.SocksProxy, reflect.TypeOf(g.SocksProxy).String()})
	tw.AppendRow(table.Row{"Logger Writers", g.Logger, reflect.TypeOf(g.Logger).String()})
	tw.AppendRow(table.Row{"Logger Level", g.LogLevel, reflect.TypeOf(g.LogLevel).String()})
	tw.AppendRow(table.Row{"Logger Time Format", g.LogTimeFormat, reflect.TypeOf(g.LogTimeFormat).String()})
	tw.AppendRow(table.Row{"Logger Colorized", g.LogsColorized, reflect.TypeOf(g.LogsColorized).String()})
	tw.AppendRow(table.Row{"Logger JSON", g.LogJSON, reflect.TypeOf(g.LogJSON).String()})

	fmt.Println(tw.Render())
	return nil
}

func (i nodeInfoCmd) Table() error {
	return nil
}

func (l nodeListCmd) Table() error {
	return nil
}

func (n nodeOptions) Table() error {
	return nil
}

func (p nodePreflightCmd) Table() error {
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

func (u nodeUpCmd) Table() error {
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

func (l lameDuckOptions) Table() error {
	return nil
}

func (m monitorOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Monitor Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"Node Identifier", m.NodeId, reflect.TypeOf(m.NodeId)},
		{"Workload Identifier", m.WorkloadId, reflect.TypeOf(m.WorkloadId)},
		{"Logging Level", m.Level, reflect.TypeOf(m.Level)},
	})
	fmt.Println(tw.Render())
	return nil
}

func (l monitorLogCmd) Table() error {
	return nil
}

func (l monitorEventCmd) Table() error {
	return nil
}

func (r rootfsOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("RootFS Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"OutName", r.OutName, reflect.TypeOf(r.OutName)},
		{"BaseImage", r.BaseImage, reflect.TypeOf(r.BaseImage)},
		{"BuildScriptPath", r.BuildScriptPath, reflect.TypeOf(r.BuildScriptPath)},
		{"AgentBinaryPath", r.AgentBinaryPath, reflect.TypeOf(r.AgentBinaryPath)},
		{"RootFSSize", r.RootFSSize, reflect.TypeOf(r.RootFSSize)},
	})

	fmt.Println(tw.Render())
	return nil
}

func (s stopOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("DevRun Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})

	tw.AppendRows([]table.Row{
		{"WorkloadId", s.WorkloadId, reflect.TypeOf(s.WorkloadId)},
		{"TargetNode", s.TargetNode, reflect.TypeOf(s.TargetNode)},
		{"ClaimsIssuerFilePath", s.ClaimsIssuerFilePath, reflect.TypeOf(s.ClaimsIssuerFilePath)},
	})

	fmt.Println(tw.Render())
	return nil
}

func (u upgradeOptions) Table() error {
	return nil
}

func (d devRunOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("DevRun Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})

	tw.AppendRows([]table.Row{
		{"Filename", d.Filename, reflect.TypeOf(d.Filename)},
		{"AutoStop", d.AutoStop, reflect.TypeOf(d.AutoStop)},
		{"BucketMaxBytes", d.DevBucketMaxBytes, reflect.TypeOf(d.DevBucketMaxBytes)},
		{"Argv", d.Argv, reflect.TypeOf(d.Argv)},
		{"Env", d.Env, reflect.TypeOf(d.Env)},
		{"TargetNode", d.TargetNode, reflect.TypeOf(d.TargetNode)},
		{"Name", d.Name, reflect.TypeOf(d.Name)},
		{"Description", d.Description, reflect.TypeOf(d.Description)},
		{"WorkloadType", d.WorkloadType, reflect.TypeOf(d.WorkloadType)},
		{"PublisherXkeyFile", d.PublisherXkeyFile, reflect.TypeOf(d.PublisherXkeyFile)},
		{"ClaimsIssuerFile", d.ClaimsIssuerFile, reflect.TypeOf(d.ClaimsIssuerFile)},
		{"TriggerSubjects", d.TriggerSubjects, reflect.TypeOf(d.TriggerSubjects)},
	})

	fmt.Println(tw.Render())
	return nil
}

func (r runOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Run Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"WorkloadUrl", r.WorkloadUrl, reflect.TypeOf(r.WorkloadUrl)},
		{"Essential", r.Essential, reflect.TypeOf(r.Essential)},
		{"Argv", r.Argv, reflect.TypeOf(r.Argv)},
		{"Env", r.Env, reflect.TypeOf(r.Env)},
		{"TargetNode", r.TargetNode, reflect.TypeOf(r.TargetNode)},
		{"Name", r.Name, reflect.TypeOf(r.Name)},
		{"Description", r.Description, reflect.TypeOf(r.Description)},
		{"WorkloadType", r.WorkloadType, reflect.TypeOf(r.WorkloadType)},
		{"PublisherXkeyFile", r.PublisherXkeyFile, reflect.TypeOf(r.PublisherXkeyFile)},
		{"ClaimsIssuerFile", r.ClaimsIssuerFile, reflect.TypeOf(r.ClaimsIssuerFile)},
		{"TriggerSubjects", r.TriggerSubjects, reflect.TypeOf(r.TriggerSubjects)},
	})

	fmt.Println(tw.Render())
	return nil
}
