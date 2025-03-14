package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type model struct {
	accountId       string
	activeView      string
	activeLogGroup  string
	activeLogStream string
	err             string
	loading         bool
	lambdas         list.Model
	logStreams      list.Model
	logEvents       viewport.Model
	spinner         spinner.Model
	reqCh           chan<- interface{}
	winHeight       int
	winWidth        int
}

const (
	viewLambda    = "lambda"
	viewLogStream = "logStream"
	viewLogEvent  = "logEvent"
)

var (
	logEventStyle          = lipgloss.NewStyle().Padding(1, 0)
	logEventTimestampStyle = logEventStyle.PaddingRight(2).Foreground(lipgloss.Color("#fcba03"))
)

type logStreamReq struct {
	logGroup string
}

type logEventReq struct {
	logGroup  string
	logStream string
}

var (
	docStyle     = lipgloss.NewStyle().Margin(1, 2)
	spinnerStyle = lipgloss.NewStyle().Align(lipgloss.Center)
)

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()

		m.winHeight = msg.Height
		m.winWidth = msg.Width
		m.lambdas.SetSize(msg.Width-h, msg.Height-v)
		m.logStreams.SetSize(msg.Width-h, msg.Height-v)
		m.logEvents.Width = msg.Width
		m.logEvents.Height = msg.Height

	case logStreamMsg:
		m.handleLogStreamMsg(msg)

	case logEventMsg:
		m.handleLogEventMsg(msg)

	case errMsg:
		m.err = wrapString(msg.err.Error(), m.winWidth*7/10)
		m.err += "\n\n Press enter/esc to continue"
	}

	if m.err != "" {
		return m.viewErrUpdate(msg)
	}

	if m.loading {
		return m, nil
	}

	switch m.activeView {
	case viewLambda:
		return m.viewLambdaUpdate(msg)
	case viewLogStream:
		return m.viewLogStreamUpdate(msg)
	case viewLogEvent:
		return m.viewLogEventUpdate(msg)
	}

	panic("unknown update function for view " + m.activeView)
}

func (m model) View() string {
	if m.err != "" {
		return lipgloss.Place(m.winWidth, m.winHeight, lipgloss.Center, lipgloss.Center, m.err)
	}

	if m.loading {
		return lipgloss.Place(m.winWidth, m.winHeight, lipgloss.Center, lipgloss.Center, m.spinner.View())
	}

	switch m.activeView {
	case viewLambda:
		return m.lambdas.View()
	case viewLogStream:
		return m.logStreams.View()
	case viewLogEvent:
		return m.logEvents.View()
	default:
		return m.lambdas.View()
	}
}

func (m model) viewErrUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.err = ""
			m.loading = false
		}
	}

	return m, nil
}

func (m model) viewLambdaUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.lambdas, cmd = m.lambdas.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "l":
			if i, ok := m.lambdas.SelectedItem().(lambdaItem); ok {
				select {
				case m.reqCh <- logStreamReq{logGroup: i.logGroup}:
					m.activeLogGroup = i.logGroup
					m.loading = true
					cmd = m.spinner.Tick
				default:
				}
			}
		case "esc":
			m.lambdas.ResetFilter()
			cmd = nil
		}
	}
	return m, cmd
}

func (m model) viewLogStreamUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.logStreams, _ = m.logStreams.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.logStreams.FilterValue() == "" {
				m.activeView = viewLambda
				m.activeLogGroup = ""
			}
			m.logStreams.ResetFilter()
			cmd = nil
		case "enter":
			if i, ok := m.logStreams.SelectedItem().(logStreamItem); ok {
				if i.name == m.activeLogStream {
					m.activeView = viewLogEvent

					break
				}

				select {
				case m.reqCh <- logEventReq{
					logGroup:  m.activeLogGroup,
					logStream: i.name,
				}:
					m.activeLogStream = i.name
					m.loading = true
					cmd = m.spinner.Tick
				default:
				}
			}
		case "r":
			select {
			case m.reqCh <- logStreamReq{logGroup: m.activeLogGroup}:
				m.loading = true
				cmd = m.spinner.Tick
			default:
			}
		}
	}

	return m, cmd
}

func (m model) viewLogEventUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.logEvents, cmd = m.logEvents.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.activeView = viewLogStream
			cmd = nil
		}
	}
	return m, cmd
}

func (m model) viewLoadingUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m *model) handleLogStreamMsg(msg logStreamMsg) {
	listItems := make([]list.Item, 0, len(msg.items))

	for _, item := range msg.items {
		listItems = append(listItems, logStreamItem{item[0], item[1]})
	}

	m.logStreams.SetItems(listItems)
	m.logStreams.Title = fmt.Sprintf(
		"Viewing Log Streams - Log Group \"%s\" - Account ID: %s",
		m.activeLogGroup,
		m.accountId,
	)
	m.activeView = viewLogStream
	m.loading = false
}

func (m *model) handleLogEventMsg(msg logEventMsg) {
	t := table.
		New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := logEventStyle
			if col == 0 {
				style = logEventTimestampStyle
			}

			if row%2 == 1 {
				style = style.Background(lipgloss.Color("235"))
			}
			return style
		}).
		Width(m.logEvents.Width).
		Rows(msg.events...)

	m.logEvents.SetContent(t.Render())
	m.activeView = viewLogEvent
	m.loading = false
}

func newModel(reqCh chan interface{}, accountId string, lambdas []list.Item) model {
	simpleListDelegate := list.NewDefaultDelegate()
	simpleListDelegate.ShowDescription = false
	model := model{
		accountId:  accountId,
		reqCh:      reqCh,
		lambdas:    list.New(lambdas, list.NewDefaultDelegate(), 0, 0),
		logStreams: list.New(nil, simpleListDelegate, 0, 0),
		logEvents:  viewport.New(0, 0),
		spinner:    spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle)),
		activeView: viewLambda,
	}
	model.lambdas.Title = fmt.Sprintf("Viewing Lambdas - Account ID: %s", accountId)
	model.lambdas.KeyMap.NextPage = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "next page"))
	model.lambdas.KeyMap.PrevPage = key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "prev page"))
	model.lambdas.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "invoke")),
		}
	}

	return model
}
