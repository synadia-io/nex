package natsContext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type ChangeNatContextMsg struct{}

type NatsContextModel struct {
	width  int
	height int

	SelectedContext list.Item
	contextList     list.Model
	help            help.Model

	natsContextHome string
}

func NewNatsContextModel(w, h int) NatsContextModel {
	ncHome, _ := os.UserHomeDir()
	ncHome = filepath.Join(ncHome, ".config/nats/context")

	contexts := getContexts(ncHome)
	delegate := list.NewDefaultDelegate()

	contextList := list.New(contexts, delegate, int(float32(w)*0.4), int(float32(h)*0.4))
	contextList.SetShowTitle(false)
	contextList.SetShowHelp(false)

	return NatsContextModel{
		width:           w,
		height:          h,
		contextList:     contextList,
		help:            help.New(),
		natsContextHome: ncHome,
	}
}

func (m NatsContextModel) Init() tea.Cmd {
	return nil
}

func (m NatsContextModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
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
		m.contextList.SetSize(int(float32(m.width)*0.4), int(float32(m.height)*0.4))
	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if m.contextList.FilterState() == list.Filtering {
			break
		}
		switch keypress := msg.String(); keypress {
		case "enter":
			m.SelectedContext = m.contextList.SelectedItem()
			return m, nil
		}
	}

	m.contextList, cmd = m.contextList.Update(message)

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m NatsContextModel) View() string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("838"))
	ss := borderStyle.Width(int(float32(m.width) * 0.4)).Height(int(float32(m.height) * 0.4))
	return ss.Render(m.contextList.View())
}

func (m NatsContextModel) isInitialized() bool {
	return m.height != 0 && m.width != 0
}

func getContexts(homeDir string) []list.Item {
	ret := []list.Item{}
	entries, _ := os.ReadDir(homeDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		tContext := NatsContext{}
		if strings.HasSuffix(entry.Name(), ".json") {
			tContext.Name = strings.TrimSuffix(entry.Name(), ".json")
			tContext.FilePath = filepath.Join(homeDir, entry.Name())
			f, _ := os.ReadFile(filepath.Join(homeDir, entry.Name()))
			if err := json.Unmarshal(f, &tContext); err == nil {
				ret = append(ret, tContext)
			}
		}
	}
	return ret
}

type NatsContext struct {
	Name     string `json:"-"`
	FilePath string `json:"-"`

	CA                   string `json:"ca"`
	Cert                 string `json:"cert"`
	ColorScheme          string `json:"color_scheme"`
	Creds                string `json:"creds"`
	Desc                 string `json:"description"`
	InboxPrefix          string `json:"inbox_prefix"`
	JetstreamApiPrefix   string `json:"jetstream_api_prefix"`
	JetstreamDomain      string `json:"jetstream_domain"`
	JetstreamEventPrefix string `json:"jetstream_event_prefix"`
	Key                  string `json:"key"`
	Nkey                 string `json:"nkey"`
	Nsc                  string `json:"nsc"`
	Password             string `json:"password"`
	SocksProxy           string `json:"socks_proxy"`
	Token                string `json:"token"`
	URL                  string `json:"url"`
	User                 string `json:"user"`
}

func (n NatsContext) Title() string       { return n.Name }
func (n NatsContext) Description() string { return n.Desc }
func (n NatsContext) FP() string          { return n.FilePath }
func (n NatsContext) FilterValue() string { return n.Name }
