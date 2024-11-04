package main

import "github.com/jedib0t/go-pretty/v6/table"
import "github.com/jedib0t/go-pretty/v6/text"

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

func coloredBorderStyle(c text.Color) table.Style {
	s := table.StyleRounded
	s.Color.Border = text.Colors{c}
	s.Color.Separator = text.Colors{c}
	s.Format.Footer = text.FormatDefault

	return s
}

func newTableWriter(title string) table.Writer {
	tW := table.NewWriter()
	tW.SetTitle(title)

	tW.SetStyle(styles["rounded"])

	tW.Style().Title.Align = text.AlignCenter
	tW.Style().Format.Header = text.FormatDefault

	return tW
}
