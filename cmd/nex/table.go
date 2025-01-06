package main

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func printTable(title string, in ...table.Row) error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle(title)
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows(in)
	fmt.Println(tw.Render())
	return nil
}

func (g Globals) Table() []table.Row {
	return []table.Row{
		{"Config File", g.Config, reflect.TypeOf(g.Config).String()},
		{"Nex Namespace", g.Namespace, reflect.TypeOf(g.Namespace).String()},
		{"NATS Server", g.NatsServers, reflect.TypeOf(g.NatsServers).String()},
		{"NATS Context", g.NatsContext, reflect.TypeOf(g.NatsContext).String()},
		{"Logger Targets", g.Target, reflect.TypeOf(g.Target).String()},
		{"Logger Level", g.LogLevel, reflect.TypeOf(g.LogLevel).String()},
		{"Logger JSON", g.LogJSON, reflect.TypeOf(g.LogJSON).String()},
		{"Logger Colorized", g.LogColor, reflect.TypeOf(g.LogColor).String()},
		{"Logger ShortLevel", g.LogShortLevels, reflect.TypeOf(g.LogShortLevels).String()},
		{"Logger Time Format", g.LogTimeFormat, reflect.TypeOf(g.LogTimeFormat).String()},
		{"NATS Context", g.NatsContext, reflect.TypeOf(g.NatsContext).String()},
		{"NATS Servers", g.NatsServers, reflect.TypeOf(g.NatsServers).String()},
		{"NATS User Nkey", g.NatsUserNkey, reflect.TypeOf(g.NatsUserNkey).String()},
		{"NATS User Seed", g.NatsUserSeed, reflect.TypeOf(g.NatsUserSeed).String()},
		{"NATS User JWT", g.NatsUserJWT, reflect.TypeOf(g.NatsUserJWT).String()},
		{"NATS User", g.NatsUser, reflect.TypeOf(g.NatsUser).String()},
		{"NATS User Password", g.NatsUserPassword, reflect.TypeOf(g.NatsUserPassword).String()},
		{"NATS JS Domain", g.NatsJSDomain, reflect.TypeOf(g.NatsJSDomain).String()},
		{"NATS Connection Name", g.NatsConnectionName, reflect.TypeOf(g.NatsConnectionName).String()},
		{"NATS Credentials File", g.NatsCredentialsFile, reflect.TypeOf(g.NatsCredentialsFile).String()},
		{"NATS Timeout", g.NatsTimeout, reflect.TypeOf(g.NatsTimeout).String()},
		{"NATS TLS Cert", g.NatsTLSCert, reflect.TypeOf(g.NatsTLSCert).String()},
		{"NATS TLS Key", g.NatsTLSKey, reflect.TypeOf(g.NatsTLSKey).String()},
		{"NATS TLS CA", g.NatsTLSCA, reflect.TypeOf(g.NatsTLSCA).String()},
		{"NATS TLS First", g.NatsTLSFirst, reflect.TypeOf(g.NatsTLSFirst).String()},
	}
}

func (u Up) Table() []table.Row {
	return []table.Row{
		{"Agent Handshake Timeout Millisecond", u.AgentHandshakeTimeoutMillisecond, reflect.TypeOf(u.AgentHandshakeTimeoutMillisecond).String()},
		{"Default Resource Directory", u.DefaultResourceDir, reflect.TypeOf(u.DefaultResourceDir).String()},
		{"Nexus Name", u.NexusName, reflect.TypeOf(u.NexusName)},
		{"Tags", u.Tags, reflect.TypeOf(u.Tags).String()},
		{"Valid Issuers", u.ValidIssuers, reflect.TypeOf(u.ValidIssuers).String()},
		{"Agents", u.Agents, reflect.TypeOf(u.Agents).String()},
		{"OpenTelemetry Metrics Enabled", u.OtelConfig.OtelMetrics, reflect.TypeOf(u.OtelConfig.OtelMetrics).String()},
		{"OpenTelemetry Metrics Port", u.OtelConfig.OtelMetricsPort, reflect.TypeOf(u.OtelConfig.OtelMetricsPort).String()},
		{"OpenTelemetry Metrics Exporter", u.OtelConfig.OtelMetricsExporter, reflect.TypeOf(u.OtelConfig.OtelMetricsExporter).String()},
		{"OpenTelemetry Traces Enabled", u.OtelConfig.OtelTraces, reflect.TypeOf(u.OtelConfig.OtelTraces).String()},
		{"OpenTelemetry Traces Exporter", u.OtelConfig.OtelTracesExporter, reflect.TypeOf(u.OtelConfig.OtelTracesExporter).String()},
		{"OpenTelemetry Exporter URL", u.OtelConfig.OtlpExporterUrl, reflect.TypeOf(u.OtelConfig.OtlpExporterUrl).String()},
	}
}

func (l LameDuck) Table() []table.Row {
	return []table.Row{
		{"Node ID", l.NodeID, reflect.TypeOf(l.NodeID).String()},
		{"Label", l.Label, reflect.TypeOf(l.Label).String()},
	}
}

func (l List) Table() []table.Row {
	return []table.Row{
		{"Tag Filter", l.Filter, reflect.TypeOf(l.Filter).String()},
		{"JSON Output", l.JSON, reflect.TypeOf(l.JSON).String()},
	}
}

func (i Info) Table() []table.Row {
	return []table.Row{
		{"Node ID", i.NodeID, reflect.TypeOf(i.NodeID).String()},
		{"JSON Output", i.JSON, reflect.TypeOf(i.JSON).String()},
	}
}

func (r RunWorkload) Table() []table.Row {
	return []table.Row{}
}

func (r StopWorkload) Table() []table.Row {
	return []table.Row{}
}

func (e Events) Table() []table.Row {
	return []table.Row{}
}

func (l Logs) Table() []table.Row {
	return []table.Row{}
}

func (b BundleWorkload) Table() []table.Row {
	return []table.Row{
		{"Binaries", b.Binaries, reflect.TypeOf(b.Binaries).String()},
		{"Operating System", b.OS, reflect.TypeOf(b.OS).String()},
		{"Architecture", b.Arch, reflect.TypeOf(b.Arch).String()},
		{"Output Path", b.Output, reflect.TypeOf(b.Output).String()},
		{"Push", b.Push, reflect.TypeOf(b.Push).String()},
		{"Registry", b.Registry, reflect.TypeOf(b.Registry).String()},
		{"Registry User", b.RegistryUser, reflect.TypeOf(b.RegistryUser).String()},
		{"Registry Password", b.RegistryPassword, reflect.TypeOf(b.RegistryPassword).String()},
		{"Workload name", b.WorkloadName, reflect.TypeOf(b.WorkloadName).String()},
		{"Workload Tag", b.WorkloadTag, reflect.TypeOf(b.WorkloadTag).String()},
		{"Workload Description", b.WorkloadDescription, reflect.TypeOf(b.WorkloadDescription).String()},
		{"Workload Signing Key Path", b.WorkloadSigningKey, reflect.TypeOf(b.WorkloadSigningKey).String()},
		{"Workload Type", b.WorkloadType, reflect.TypeOf(b.WorkloadType).String()},
	}
}

func (i InfoWorkload) Table() []table.Row {
	return []table.Row{
		{"Workload ID", i.WorkloadId, reflect.TypeOf(i.WorkloadId).String()},
	}
}

func (c CopyWorkload) Table() []table.Row {
	return []table.Row{
		{"Workload ID", c.WorkloadId, reflect.TypeOf(c.WorkloadId).String()},
		{"Node ID", c.NodeId, reflect.TypeOf(c.NodeId).String()},
		{"Node Tags", c.NodeTags, reflect.TypeOf(c.NodeTags).String()},
	}
}

func (l ListWorkload) Table() []table.Row {
	return []table.Row{
		{"JSON Output", l.Json, reflect.TypeOf(l.Json).String()},
	}
}
