package home

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	tags      map[string]any
	memory    memory
	workloads []list.Item
}

type workload string

func (w workload) Title() string       { return string(w) }
func (w workload) Description() string { return "" }
func (w workload) FilterValue() string { return string(w) }

type memory struct {
	free      int64
	available int64
	total     int64
}

func (i nexNode) Title() string       { return i.name }
func (i nexNode) Description() string { return i.publicKey }
func (i nexNode) FilterValue() string { return i.name }

var nodes []list.Item = []list.Item{
	nexNode{name: "nex-01", publicKey: "NCR4EGARH4CGMUVZZCNA45DIWJ4GHAT22EGCSFKLCPZTSLKCFV2DXPQN", xkey: "XDBQ35Y6GHRNGCOYMG4JKPUVVFWUGWSVKF6OKF42OVU753NU6CQR33HY", version: "0.1.5", uptime: "1d18h43m20s", tags: map[string]any{"cluster": "nexus-01", "environment": "nex-qa", "nex.arch": "amd64", "nex.cpucount": 2, "nex.os": "linux", "node_name": "nex-01"}, memory: memory{1_337_492, 3_420_024, 4_018_128}, workloads: []list.Item{workload("EchoService")}},
	nexNode{name: "nex-02", publicKey: "NAQTW2LULRRID5HNIBUKY3KVPFCCEFRTURT5AC5OA3AGERPP24CLM5XI", xkey: "XBR4LB45RQUNSVNCJWWPSFZ4R26FRBPPYSB4LTUHZ46DTEHNSHVQV5V6", version: "0.1.5", uptime: "1d18h51m10s", tags: map[string]any{"cluster": "nexus-01", "environment": "nex-qa", "nex.arch": "amd64", "nex.cpucount": 2, "nex.os": "linux", "node_name": "nex-02"}, memory: memory{282_540, 1_548_756, 2_023_128}, workloads: []list.Item{workload("WorldDonimator")}},
	nexNode{name: "nex-03", publicKey: "ND6OBBPF7IO7C66E5L6NYRNE3MRGQSGEIZC3ONUUSLYFMTSPFS4G2HXM", xkey: "XDR5TIZA5T3QMNVXIIZMA3JIOGMM6Q3ICNCZBFTCQ2FWX2XJH57WAVH6", version: "0.1.5", uptime: "1d18h52m17s", tags: map[string]any{"cluster": "nexus-01", "environment": "nex-qa", "nex.arch": "amd64", "nex.cpucount": 1, "nex.os": "linux", "node_name": "nex-03"}, memory: memory{241_764, 1_540_968, 2_023_368}, workloads: []list.Item{workload("coffee_machine"), workload("heartRateMonitor")}},
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
	ret.WriteString("Memory in kB:")
	ret.WriteString("\n")
	ret.WriteString("\tFree: " + strconv.Itoa(int(n.memory.free)))
	ret.WriteString("\n")
	ret.WriteString("\tAvailable: " + strconv.Itoa(int(n.memory.available)))
	ret.WriteString("\n")
	ret.WriteString("\tTotal: " + strconv.Itoa(int(n.memory.total)))
	ret.WriteString("\n")
	return ret.String()
}

func tagsString(t map[string]any) string {
	ret := strings.Builder{}

	count := 0
	for k, v := range t {
		ret.WriteString(fmt.Sprintf("%s=%v", k, v))
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

	str := string("  " + i)

	fn := workloadStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedWorkloadStyle.Render("> " + strings.TrimSpace(strings.Join(s, " ")))
		}
	}

	fmt.Fprint(w, fn(str))
}
