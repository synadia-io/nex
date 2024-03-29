package home

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	selectedNode string

	nodeList     list.Model
	workloadList list.Model
	nodeData     viewport.Model
	workloadData viewport.Model

	keyMap keyMap

	help help.Model
}

func NewHomeModel() HomeModel {
	delegate := list.NewDefaultDelegate()
	nodeList := list.New(nodes, delegate, 0, 0)
	selectedNode := nodeList.Items()[0].(nexNode).publicKey

	nodeData := viewport.New(0, 0)
	n, _ := nodeList.Items()[0].(nexNode)
	nodeData.SetContent(fmt.Sprint(n))

	workloadList := list.New(n.workloads, delegate, 0, 0)

	nodeList.SetShowTitle(false)
	nodeList.SetShowHelp(false)
	workloadList.SetShowTitle(false)
	workloadList.SetShowHelp(false)

	h := help.New()
	h.ShowAll = false
	return HomeModel{
		nodeList:     nodeList,
		nodeData:     nodeData,
		workloadList: workloadList,
		selectedNode: selectedNode,
		help:         h,
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

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, _ = term.GetSize(int(os.Stdout.Fd()))
		m.nodeList.SetSize(int(float32(m.width)*0.4), int(float32(m.height)*0.4))
		m.workloadList.SetSize(int(float32(m.width)*0.4), int(float32(m.height)*0.4))
		m.nodeData.Width = int(float32(m.width) * 0.4)
		m.nodeData.Height = int(float32(m.height) * 0.4)
		//m.workloadList.SetSize(int(float32(m.width)*0.4), int(float32(m.height)*0.4))
	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		// if m.nodeList.FilterState() == list.Filtering {
		// 	break
		// }
		switch keypress := msg.String(); keypress {
		//case key.Matches(msg, m.keyMap.Select):
		case "enter":
			m.selectedNode = m.nodeList.SelectedItem().(nexNode).publicKey
			n, _ := m.nodeList.SelectedItem().(nexNode)
			m.nodeData.SetContent(fmt.Sprint(n))
			m.workloadList.SetItems(n.workloads)
			return m, nil
		}
	}

	m.nodeList, cmd = m.nodeList.Update(message)

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
					"NEX Nodes",
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
