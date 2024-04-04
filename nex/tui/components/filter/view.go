package filter

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/lipgloss"
)
import "github.com/synadia-io/nex/nex/tui/components"

func (m filter) View() string {
	if m.textinput.Focused() {
		m.textinput.TextStyle = components.FilterEditing
		m.textinput.PromptStyle = components.FilterEditing
		m.textinput.Cursor.TextStyle = components.FilterEditing
		if len(m.textinput.Value()) > 0 {
			// editing existing filter
			m.textinput.Prompt = "filter: "
		} else {
			// editing but no filter value yet
			m.textinput.Prompt = ""
			m.SetSuffix("")
			m.textinput.Cursor.SetMode(cursor.CursorHide)
			m.textinput.SetValue("type to filter")
		}
	} else {
		if len(m.textinput.Value()) > 0 {
			// filter applied, not editing
			m.textinput.Prompt = "filter: "
			m.textinput.Cursor.TextStyle = components.FilterApplied
			m.textinput.PromptStyle = components.FilterApplied
			m.textinput.TextStyle = components.FilterApplied
		} else {
			// no filter, not editing
			m.textinput.Prompt = ""
			m.textinput.PromptStyle = components.BaseStyle
			m.textinput.TextStyle = components.BaseStyle
			m.textinput.SetValue("'/' to filter")
			m.SetSuffix("")
		}
	}
	m.textinput.SetValue(m.textinput.Value() + m.suffix)
	filterString := m.textinput.View()
	filterStringStyle := m.textinput.TextStyle.Copy().MarginLeft(1).PaddingLeft(1).PaddingRight(0)

	filterPrefixStyle := components.FilterPrefix.Copy()
	if m.compact {
		filterPrefixStyle = filterPrefixStyle.UnsetBorderStyle().Padding(0).MarginLeft(0).MarginRight(1)
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		filterPrefixStyle.Render(m.prefix),
		filterStringStyle.Render(filterString),
	)
}
