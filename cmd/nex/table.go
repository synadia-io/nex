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

func (n Node) Table() []table.Row {
	return []table.Row{
		{"Namespace", n.Namespace, reflect.TypeOf(n.Namespace).String()},
	}
}

func (g Globals) Table() []table.Row {
	return []table.Row{
		{"Config File", g.Config, reflect.TypeOf(g.Config).String()},
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
		{"Workload Types", u.WorkloadTypes, reflect.TypeOf(u.WorkloadTypes).String()},
		{"OpenTelemetry Metrics Enabled", u.OtelConfig.OtelMetrics, reflect.TypeOf(u.OtelConfig.OtelMetrics).String()},
		{"OpenTelemetry Metrics Port", u.OtelConfig.OtelMetricsPort, reflect.TypeOf(u.OtelConfig.OtelMetricsPort).String()},
		{"OpenTelemetry Metrics Exporter", u.OtelConfig.OtelMetricsExporter, reflect.TypeOf(u.OtelConfig.OtelMetricsExporter).String()},
		{"OpenTelemetry Traces Enabled", u.OtelConfig.OtelTraces, reflect.TypeOf(u.OtelConfig.OtelTraces).String()},
		{"OpenTelemetry Traces Exporter", u.OtelConfig.OtelTracesExporter, reflect.TypeOf(u.OtelConfig.OtelTracesExporter).String()},
		{"OpenTelemetry Exporter URL", u.OtelConfig.OtlpExporterUrl, reflect.TypeOf(u.OtelConfig.OtlpExporterUrl).String()},
	}
}

func (p Preflight) Table() []table.Row {
	return []table.Row{
		{"Force Install", p.Force, reflect.TypeOf(p.Force).String()},
		{"Yes", p.Yes, reflect.TypeOf(p.Yes).String()},
		{"Generate Configuration File", p.GenConfig, reflect.TypeOf(p.GenConfig).String()},
		{"Status", p.Status, reflect.TypeOf(p.Status).String()},
		{"Install Version Override", p.InstallVersion, reflect.TypeOf(p.InstallVersion).String()},
		{"Github PAT", p.GithubPAT, reflect.TypeOf(p.GithubPAT).String()},
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

func (w Workload) Table() []table.Row {
	return []table.Row{
		{"Namespace", w.Namespace, reflect.TypeOf(w.Namespace).String()},
	}
}

func (r RunWorkload) Table() []table.Row {
	return []table.Row{
		{"File", r.File, reflect.TypeOf(r.File).String()},
		{"Target ID", r.TargetId, reflect.TypeOf(r.TargetId).String()},
		{"Xkey", r.Xkey, reflect.TypeOf(r.Xkey).String()},
		{"Issuer Key", r.IssuerKey, reflect.TypeOf(r.IssuerKey).String()},
		{"Workload Name", r.Name, reflect.TypeOf(r.Name).String()},
		{"Workload Type", r.WorkloadType, reflect.TypeOf(r.WorkloadType).String()},
		{"Description", r.Description, reflect.TypeOf(r.Description).String()},
		{"Arguments", r.Argv, reflect.TypeOf(r.Argv).String()},
		{"Environment Variables", r.Env, reflect.TypeOf(r.Env).String()},
		{"Trigger Subjects", r.TriggerSubjects, reflect.TypeOf(r.TriggerSubjects).String()},
		{"Host Services URL", r.HostServicesURL, reflect.TypeOf(r.HostServicesURL).String()},
		{"Host Services JWT", r.HostServicesJWT, reflect.TypeOf(r.HostServicesJWT).String()},
		{"Host Services Seed", r.HostServicesSeed, reflect.TypeOf(r.HostServicesSeed).String()},
		{"Tags", r.Tags, reflect.TypeOf(r.Tags).String()},
	}
}

func (s StopWorkload) Table() []table.Row {
	return []table.Row{
		{"WorkloadID", s.WorkloadId, reflect.TypeOf(s.WorkloadId).String()},
		{"TargetID", s.TargetId, reflect.TypeOf(s.TargetId).String()},
		{"IssuerKey", s.IssuerKey, reflect.TypeOf(s.IssuerKey).String()},
		{"Reason", s.Reason, reflect.TypeOf(s.Reason).String()},
		{"Immediate", s.Immediate, reflect.TypeOf(s.Immediate).String()},
		{"Devrun", s.Devrun, reflect.TypeOf(s.Devrun).String()},
	}
}
