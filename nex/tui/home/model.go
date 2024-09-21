package home

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nats-io/nats.go"
	controlapi "github.com/synadia-io/nex/control-api"
	"github.com/synadia-io/nex/nex/tui/format"
	"golang.org/x/term"
)

var (
	borderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("838"))
)

type HomeModel struct {
	width  int
	height int

	selectedNode     nexNode
	selectedWorkload workload

	nc *nats.Conn

	nodeList     list.Model
	workloadList list.Model
	nodeData     viewport.Model
	workloadData viewport.Model

	selectedList int

	help help.Model
}

func NewHomeModel(h, w int, nc *nats.Conn) HomeModel {
	var selectedNode nexNode

	nodeList := list.New([]list.Item{}, nexNodeDelegate{}, int(float32(w)*0.4), int(float32(h)*0.4))
	workloadList := list.New([]list.Item{}, workloadDelegate{}, int(float32(w)*0.4), int(float32(h)*0.4))
	nodeData := viewport.New(int(float32(w)*0.4), int(float32(h)*0.4))

	if nc != nil {
		api := controlapi.NewApiClient(nc, time.Second, slog.Default())
		nodes := []list.Item{}
		ns, _ := api.PingNodes()
		for _, n := range ns {
			workloads := []list.Item{}
			info, _ := api.NodeInfo(n.NodeId)
			for _, w := range info.Machines {
				workloads = append(workloads, workload(w))
			}

			nn := nexNode{
				name: func() string {
					if name, ok := n.Tags["node_name"]; !ok {
						return "no-name-provided"
					} else {
						return name
					}
				}(),
				publicKey: n.NodeId,
				version:   n.Version,
				tags:      n.Tags,
				memory:    *info.Memory,
				uptime:    info.Uptime,
				xkey:      info.PublicXKey,
				workloads: workloads,
			}

			nodes = append(nodes, nn)
		}
		nodeList.SetItems(nodes)
		if len(nodes) > 0 {
			selectedNode, _ = nodeList.Items()[0].(nexNode)
			nodeData.SetContent(fmt.Sprint(selectedNode))
			workloadList.SetItems(selectedNode.workloads)
		}
	}

	nodeList.SetShowTitle(false)
	nodeList.SetShowHelp(false)
	workloadList.SetShowTitle(false)
	workloadList.SetShowHelp(false)

	return HomeModel{
		width:  w,
		height: h,

		nc:           nc,
		nodeList:     nodeList,
		nodeData:     nodeData,
		workloadList: workloadList,
		workloadData: viewport.New(int(float32(w)*0.4), int(float32(h)*0.4)),
		selectedNode: selectedNode,
		selectedList: 0,
		help:         help.New(),
	}
}

func (m HomeModel) Init() tea.Cmd {
	return nil
}

func (m HomeModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	if !m.isInitialized() {
		if _, ok := message.(tea.WindowSizeMsg); !ok {
			return m, nil
		}
	}

	if m.selectedList == 0 {
		m.nodeList, cmd = m.nodeList.Update(message)
	} else if m.selectedList == 1 {
		m.workloadList, cmd = m.workloadList.Update(message)
	}

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, _ = term.GetSize(int(os.Stdout.Fd()))
		m.nodeList.SetSize(int(float32(m.width)*0.4), int(float32(m.height)*0.4))
		m.workloadList.SetSize(int(float32(m.width)*0.4), int(float32(m.height)*0.4))
		m.nodeData.Width = int(float32(m.width) * 0.4)
		m.nodeData.Height = int(float32(m.height) * 0.4)
		m.workloadData.Width = int(float32(m.width) * 0.4)
		m.workloadData.Height = int(float32(m.height) * 0.4)
	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if m.nodeList.FilterState() == list.Filtering {
			break
		}
		switch keypress := msg.String(); keypress {
		case "j", "k", "up", "down":
			if m.selectedList == 0 {
				m.selectedNode = m.nodeList.SelectedItem().(nexNode)
				m.nodeData.SetContent(fmt.Sprint(m.selectedNode))

			}
			if m.selectedList == 1 {
				m.selectedWorkload = m.workloadList.SelectedItem().(workload)
				m.workloadData.SetContent(fmt.Sprint(m.selectedWorkload))
			}
			return m, nil
		case "R":
			return m.refreshNodeData(), nil
		case "tab":
			if m.selectedList == 0 && len(m.workloadList.Items()) > 0 {
				m.selectedList = 1
			} else {
				m.selectedList = 0
			}
			return m, nil
		}

	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m HomeModel) View() string {
	if !m.isInitialized() {
		return "\n Initializing..."
	}

	s := format.DocStyle.MaxHeight(m.height).MaxWidth(m.width).Padding(1, 2, 1, 2)
	ss := borderStyle.Width(int(float32(m.width) * 0.4)).Height(int(float32(m.height) * 0.4))
	return s.Render(
		lipgloss.JoinVertical(lipgloss.Top,
			lipgloss.JoinHorizontal(
				lipgloss.Top,
				lipgloss.JoinVertical(
					lipgloss.Left,
					"Nex Nodes",
					ss.Render(m.nodeList.View()),
				),
				lipgloss.JoinVertical(
					lipgloss.Right,
					"Node Data",
					ss.Render(m.nodeData.View()),
				),
			),
			lipgloss.JoinHorizontal(
				lipgloss.Top,
				lipgloss.JoinVertical(
					lipgloss.Left,
					"Running Workloads",
					ss.Render(m.workloadList.View()),
				),
				lipgloss.JoinVertical(
					lipgloss.Right,
					"Workload Statistics",
					ss.Render(m.workloadData.View()),
				),
			),
			lipgloss.JoinHorizontal(lipgloss.Bottom,
				m.helpView(),
			),
		),
	)
}

func (m HomeModel) helpView() string {
	return m.help.View(Keys)
}

func (m HomeModel) isInitialized() bool {
	return m.height != 0 && m.width != 0
}

func (m HomeModel) refreshNodeData() tea.Model {
	if m.nc != nil {
		api := controlapi.NewApiClient(m.nc, time.Second, slog.Default())
		nodes := []list.Item{}
		ns, _ := api.PingNodes()
		for _, n := range ns {
			workloads := []list.Item{}
			info, _ := api.NodeInfo(n.NodeId)
			for _, w := range info.Machines {
				workloads = append(workloads, workload(w))
			}

			nn := nexNode{
				name: func() string {
					if name, ok := n.Tags["node_name"]; !ok {
						return "no-name-provided"
					} else {
						return name
					}
				}(),
				publicKey: n.NodeId,
				version:   n.Version,
				tags:      n.Tags,
				memory:    *info.Memory,
				uptime:    info.Uptime,
				xkey:      info.PublicXKey,
				workloads: workloads,
			}

			nodes = append(nodes, nn)
		}
		m.nodeList.SetItems(nodes)
		if len(nodes) > 0 {
			m.nodeData.SetContent(fmt.Sprint(m.selectedNode))
			m.workloadList.SetItems(m.selectedNode.workloads)
		}
	}
	return m
}
