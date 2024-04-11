package home

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/inhies/go-bytesize"

	controlapi "github.com/synadia-io/nex/control-api"
)

var (
	workloadStyle         = lipgloss.NewStyle()
	selectedWorkloadStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF06B7"))
)

type nexNode struct {
	name      string
	publicKey string
	xkey      string
	version   string
	uptime    string
	tags      map[string]string
	memory    controlapi.MemoryStat
	workloads []list.Item
}

func (i nexNode) Title() string       { return i.name }
func (i nexNode) Description() string { return i.publicKey }
func (i nexNode) FilterValue() string { return i.name }

type nexNodeDelegate struct{}

func (d nexNodeDelegate) Height() int                             { return 1 }
func (d nexNodeDelegate) Spacing() int                            { return 0 }
func (d nexNodeDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d nexNodeDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(nexNode)
	if !ok {
		return
	}

	str := fmt.Sprintf("  %s\n%s", i.name, i.publicKey)

	fn := workloadStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedWorkloadStyle.Render("> " + strings.TrimSpace(strings.Join(s, " ")))
		}
	}

	fmt.Fprint(w, fn(str))
}

func (n nexNode) String() string {
	ret := strings.Builder{}
	ret.WriteString("Name: " + n.name)
	ret.WriteString("\n")
	ret.WriteString("\n")
	ret.WriteString("Node: " + n.publicKey)
	ret.WriteString("\n")
	ret.WriteString("XKey: " + n.xkey)
	ret.WriteString("\n")
	ret.WriteString("Version: " + n.version)
	ret.WriteString("\n")
	ret.WriteString("Uptime: " + n.uptime)
	ret.WriteString("\n")
	ret.WriteString("Tags: " + tagsString(n.tags))
	ret.WriteString("\n")
	ret.WriteString("\n")
	ret.WriteString("Memory:")
	ret.WriteString("\n")
	ret.WriteString(fmt.Sprintf("\tFree: %s", bytesize.New(float64(n.memory.MemFree*1000))))
	ret.WriteString("\n")
	ret.WriteString(fmt.Sprintf("\tAvailable: %s", bytesize.New(float64(n.memory.MemAvailable*1000))))
	ret.WriteString("\n")
	ret.WriteString(fmt.Sprintf("\tTotle: %s", bytesize.New(float64(n.memory.MemTotal*1000))))
	ret.WriteString("\n")
	ret.WriteString("\n")
	ret.WriteString(fmt.Sprintf("Number of workloads: %d", len(n.workloads)))
	ret.WriteString("\n")
	return ret.String()
}

func tagsString(t map[string]string) string {
	ret := strings.Builder{}

	count := 0
	for k, v := range t {
		ret.WriteString(fmt.Sprintf("%s=%s", k, v))
		if count < len(t)-1 {
			ret.WriteString(", ")
		}
		count++
	}

	return ret.String()
}

type workloadDelegate struct{}

func (d workloadDelegate) Height() int                             { return 1 }
func (d workloadDelegate) Spacing() int                            { return 0 }
func (d workloadDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d workloadDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(workload)
	if !ok {
		return
	}

	str := fmt.Sprintf("  %s\n%s",
		func() string {
			if i.Workload.Name == "" {
				return "no-name-provided"
			}
			return i.Workload.Name
		}(),
		i.Id,
	)

	fn := workloadStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedWorkloadStyle.Render("> " + strings.TrimSpace(strings.Join(s, " ")))
		}
	}

	fmt.Fprint(w, fn(str))
}

type workload controlapi.MachineSummary

func (w workload) Title() string {
	if w.Workload.Name != "" {
		return w.Workload.Name
	} else {
		return w.Id
	}
}
func (w workload) FilterValue() string { return w.Id }

func (w workload) String() string {
	ret := strings.Builder{}
	ret.WriteString("ID: " + w.Id)
	ret.WriteString("\n")
	ret.WriteString("Healthy: " + strconv.FormatBool(w.Healthy))
	ret.WriteString("\n")
	ret.WriteString("Uptime: " + w.Uptime)
	ret.WriteString("\n")
	ret.WriteString("\n")
	ret.WriteString("Workload Stats: ")
	ret.WriteString("\n")
	ret.WriteString("\tName: " + w.Workload.Name)
	ret.WriteString("\n")
	ret.WriteString("\tDescription: " + w.Workload.Description)
	ret.WriteString("\n")
	ret.WriteString("\tHash: " + w.Workload.Hash)
	ret.WriteString("\n")
	ret.WriteString("\tRuntime: " + w.Workload.Runtime)
	ret.WriteString("\n")
	ret.WriteString("\tWorkload Type: " + w.Workload.WorkloadType)
	ret.WriteString("\n")
	ret.WriteString("\n")
	return ret.String()
}
