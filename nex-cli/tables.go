package nexcli

import (
	"fmt"
	"sort"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

var styles = map[string]table.Style{
	"":        table.StyleRounded,
	"rounded": table.StyleRounded,
	"double":  table.StyleDouble,
	"yellow":  coloredBorderStyle(text.FgYellow),
	"blue":    coloredBorderStyle(text.FgBlue),
	"cyan":    coloredBorderStyle(text.FgCyan),
	"green":   coloredBorderStyle(text.FgGreen),
	"magenta": coloredBorderStyle(text.FgMagenta),
	"red":     coloredBorderStyle(text.FgRed),
}

type tbl struct {
	writer table.Writer
}

func (t *tbl) AddHeaders(items ...any) {
	t.writer.AppendHeader(items)
}

func (t *tbl) AddFooter(items ...any) {
	t.writer.AppendFooter(items)
}

func (t *tbl) AddSeparator() {
	t.writer.AppendSeparator()
}

func (t *tbl) AddRow(items ...any) {
	t.writer.AppendRow(items)
}

func (t *tbl) Render() string {
	return fmt.Sprintln(t.writer.Render())
}

func ValidStyles() []string {
	var res []string

	for k := range styles {
		if k == "" {
			continue
		}

		res = append(res, k)
	}

	sort.Strings(res)

	return res
}

func coloredBorderStyle(c text.Color) table.Style {
	s := table.StyleRounded
	s.Color.Border = text.Colors{c}
	s.Color.Separator = text.Colors{c}
	s.Format.Footer = text.FormatDefault

	return s
}

func newTableWriter(title string) *tbl {
	tbl := &tbl{
		writer: table.NewWriter(),
	}

	tbl.writer.SetStyle(styles["rounded"])

	tbl.writer.Style().Title.Align = text.AlignCenter
	tbl.writer.Style().Format.Header = text.FormatDefault

	if title != "" {
		tbl.writer.SetTitle(title)
	}

	return tbl
}
