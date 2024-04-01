package tui

import (
	"encoding/json"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/nex/tui/deploy"
	"github.com/synadia-io/nex/nex/tui/home"
	"github.com/synadia-io/nex/nex/tui/natsContext"
	"golang.org/x/term"
)

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if !m.isInitialized() {
		if _, ok := message.(tea.WindowSizeMsg); !ok {
			return m, nil
		}
	}

	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, _ = term.GetSize(int(os.Stdout.Fd()))
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "back":
			m.state = homeView
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	switch m.state {
	case homeView:
		m.homeView, cmd = m.homeView.Update(message)
		switch msg := message.(type) {
		case tea.KeyMsg:
			switch {
			case key.Matches(msg, home.Keys.EnterNatsContext):
				m.natsContextView = natsContext.NewNatsContextModel(m.width, m.height)
				m.state = natsContextView
			case key.Matches(msg, home.Keys.EnterDeployView):
				m.deployView = deploy.NewDeployModel()
				m.state = deployView
			}
		}

	case natsContextView:
		m.natsContextView, cmd = m.natsContextView.Update(message)
		switch msg := message.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "enter":

				newCtx := m.natsContextView.(natsContext.NatsContextModel).SelectedContext.(natsContext.NatsContext).FP()
				m.state = homeView

				if newCtx != m.nContext {
					m.nContext = newCtx
					if m.nc != nil {
						_ = m.nc.Drain()
					}

					f, err := os.ReadFile(m.nContext)
					if err != nil {
						panic(m.nContext + err.Error())
					}
					nctx := NatsContext{}
					_ = json.Unmarshal(f, &nctx)

					opts := []nats.Option{}
					if nctx.Creds != "" {
						opts = append(opts, nats.UserCredentials(nctx.Creds))
					}
					nc, _ := nats.Connect(nctx.URL, opts...)
					m.homeView = home.NewHomeModel(m.height, m.width, nc)
				}
				return m, nil
			}
		}

	case confirmView:
		m.confirmView, cmd = m.confirmView.Update(message)
	case deployView:
		m.deployView, cmd = m.deployView.Update(message)
	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

type NatsContext struct {
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
