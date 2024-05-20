package globals

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (g Globals) Table() error {
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
