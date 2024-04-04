package components

import "github.com/charmbracelet/lipgloss"

const (
	black     = lipgloss.Color("#000000")
	blue      = lipgloss.Color("6")
	greenblue = lipgloss.Color("#00A095")
	pink      = lipgloss.Color("#E760FC")
	darkred   = lipgloss.Color("#FF0000")
	darkgreen = lipgloss.Color("#00FF00")
	grey      = lipgloss.Color("#737373")
	red       = lipgloss.Color("#FF5353")
	yellow    = lipgloss.Color("#DBBD70")
)

var (
	BaseStyle         = lipgloss.NewStyle()
	RoundedBorderBase = BaseStyle.Copy().
				Border(lipgloss.RoundedBorder())

	HeaderBorder = RoundedBorderBase.Copy()

	FilterEditing = BaseStyle.Copy().Foreground(black).Background(blue)
	FilterApplied = BaseStyle.Copy().Foreground(black).Background(greenblue)
	FilterPrefix  = BaseStyle.Copy().Padding(0, 3).Border(lipgloss.NormalBorder(), true)

	DebugBorderPink   = BaseStyle.Copy().Border(lipgloss.NormalBorder()).BorderForeground(pink)
	DebugBorderRed    = BaseStyle.Copy().Border(lipgloss.NormalBorder()).BorderForeground(red)
	DebugBorderGreen  = BaseStyle.Copy().Border(lipgloss.NormalBorder()).BorderForeground(darkgreen)
	DebugBorderYellow = BaseStyle.Copy().Border(lipgloss.NormalBorder()).BorderForeground(yellow)
)
