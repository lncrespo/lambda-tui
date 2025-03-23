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
	activeLambda    string
	activeLogGroup  string
	activeLogStream string
	err             string
	loading         bool
	lambdas         list.Model
	lambdaDetail    viewport.Model
	logStreams      list.Model
	logEvents       viewport.Model
	spinner         spinner.Model
	reqCh           chan<- interface{}
	winHeight       int
	winWidth        int
}

const (
	viewLambda       = "lambda"
	viewLambdaDetail = "lambdadetail"
	viewLogStream    = "logStream"
	viewLogEvent     = "logEvent"
)

var (
	logEventStyle          = lipgloss.NewStyle().AlignVertical(lipgloss.Center)
	logEventTimestampStyle = logEventStyle.Bold(true).
				Padding(1).
				Foreground(lipgloss.Color("#3275C4"))

	lambdaDetailFieldNameStyle = lipgloss.
					NewStyle().
					Bold(true).
					Background(lipgloss.Color("#192a4a")).
					PaddingLeft(2).
					PaddingRight(2)

	lambdaDetailFieldValueStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("235")).
					PaddingLeft(2).
					PaddingRight(2)

	lambdaDetailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				MarginLeft(1).
				AlignHorizontal(lipgloss.Left)
)

type logStreamReq struct {
	logGroup string
}

type logEventReq struct {
	logGroup  string
	logStream string
}

type lambdaDetailReq struct {
	name string
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
		m.lambdaDetail.Width = msg.Width
		m.lambdaDetail.Height = msg.Height

	case lambdaDetailMsg:
		m.onRcvLambdaDetailMsg(msg)

	case logStreamMsg:
		m.onRcvLogStreamMsg(msg)

	case logEventMsg:
		m.onRcvLogEventMsg(msg)

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
	case viewLambdaDetail:
		return m.viewLambdaDetailUpdate(msg)
	case viewLogStream:
		return m.viewLogStreamUpdate(msg)
	case viewLogEvent:
		return m.viewLogEventUpdate(msg)
	}

	panic("unknown update function for view " + m.activeView)
}

func (m model) viewErrUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok && (msg.String() == "enter" || msg.String() == "esc") {
		m.err = ""
		m.loading = false
	}

	return m, nil
}

func (m model) viewLambdaUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.lambdas, cmd = m.lambdas.Update(msg)

	selectedItem := lambdaItem{}
	hasSelectedItem := false
	if i, ok := m.lambdas.SelectedItem().(lambdaItem); ok {
		selectedItem = i
		hasSelectedItem = true
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			if !hasSelectedItem {
				break
			}

			if selectedItem.name == m.activeLambda {
				m.activeView = viewLambdaDetail
				cmd = nil
				break
			}

			select {
			case m.reqCh <- lambdaDetailReq{name: selectedItem.name}:
				m.loading = true
				cmd = m.spinner.Tick
				m.activeLambda = selectedItem.name
			default:
			}
		case "l":
			if !hasSelectedItem {
				break
			}

			select {
			case m.reqCh <- logStreamReq{logGroup: selectedItem.logGroup}:
				m.activeLogGroup = selectedItem.logGroup
				m.loading = true
				cmd = m.spinner.Tick
			default:
			}
		case "esc":
			m.lambdas.ResetFilter()
			cmd = nil
		}
	}
	return m, cmd
}

func (m model) viewLambdaDetailUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
		m.activeView = viewLambda
		cmd = nil
	}

	return m, cmd
}

func (m model) viewLogStreamUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.logStreams, _ = m.logStreams.Update(msg)

	selectedItem := logStreamItem{}
	hasSelectedItem := false
	if i, ok := m.logStreams.SelectedItem().(logStreamItem); ok {
		selectedItem = i
		hasSelectedItem = true
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "esc":
			if m.logStreams.FilterValue() == "" {
				m.activeView = viewLambda
				m.activeLogGroup = ""
			}
			m.logStreams.ResetFilter()
			cmd = nil
		case "enter":
			if !hasSelectedItem {
				break
			}

			if selectedItem.name == m.activeLogStream {
				m.activeView = viewLogEvent

				break
			}

			select {
			case m.reqCh <- logEventReq{
				logGroup:  m.activeLogGroup,
				logStream: selectedItem.name,
			}:
				m.activeLogStream = selectedItem.name
				m.loading = true
				cmd = m.spinner.Tick
			default:
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

	if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
		m.activeView = viewLogStream
		cmd = nil
	}

	return m, cmd
}

func (m model) viewLoadingUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
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
		return lipgloss.Place(m.winWidth, m.winHeight, lipgloss.Center, lipgloss.Center, m.lambdas.View())
	case viewLambdaDetail:
		return lipgloss.Place(m.winWidth, m.winHeight, lipgloss.Center, lipgloss.Center, m.lambdaDetail.View())
	case viewLogStream:
		return lipgloss.Place(m.winWidth, m.winHeight, lipgloss.Center, lipgloss.Center, m.logStreams.View())
	case viewLogEvent:
		return m.logEvents.View()
	default:
		return lipgloss.Place(m.winWidth, m.winHeight, lipgloss.Center, lipgloss.Center, m.lambdas.View())
	}
}

func (m *model) onRcvLambdaDetailMsg(msg lambdaDetailMsg) {
	generalInfoRows := [][]string{
		{"ARN", msg.info.arn},
		{"Name", msg.info.name},
		{"Last Modified", msg.info.lastModified},
		{"Runtime", msg.info.runtime},
		{"Architecture", msg.info.arch},
		{"Memory Size", fmt.Sprintf("%d MB", msg.info.memory)},
		{"Ephemeral Storage", fmt.Sprintf("%d MB", msg.info.ephemeralStorage)},
		{"Timeout", fmt.Sprintf("%d seconds", msg.info.timeout)},
	}

	generalInfo := table.
		New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(lambdaDetailTableStyleFunc).
		Width(m.logEvents.Width).
		Rows(generalInfoRows...)

	envVars := table.
		New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(lambdaDetailTableStyleFunc).
		Width(m.logEvents.Width).
		Rows(msg.info.envVars...)

	tags := table.
		New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(lambdaDetailTableStyleFunc).
		Width(m.logEvents.Width).
		Rows(msg.info.tags...)

	content := lipgloss.JoinVertical(
		0,
		lambdaDetailTitleStyle.Render("General"),
		generalInfo.Render(),
		lambdaDetailTitleStyle.Render("Environment Variables"),
		envVars.Render(),
		lambdaDetailTitleStyle.Render("Tags"),
		tags.Render(),
	)
	m.lambdaDetail.SetContent(content)
	m.lambdaDetail.Style = m.lambdaDetail.Style.Align(lipgloss.Center)
	m.activeView = viewLambdaDetail
	m.loading = false
}

func (m *model) onRcvLogStreamMsg(msg logStreamMsg) {
	listItems := make([]list.Item, 0, len(msg.items))

	for _, item := range msg.items {
		itemDescription := fmt.Sprintf("Last Event: %s", item[1])
		listItems = append(listItems, logStreamItem{item[0], itemDescription})
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

func (m *model) onRcvLogEventMsg(msg logEventMsg) {
	t := table.
		New().
		Border(lipgloss.HiddenBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := logEventStyle
			if col == 0 {
				style = logEventTimestampStyle
			}

			if row%2 == 1 {
				style = style.Background(lipgloss.Color("236"))
			}
			return style
		}).
		Width(m.logEvents.Width).
		Rows(msg.events...)

	m.logEvents.SetContent(t.Render())
	m.activeView = viewLogEvent
	m.loading = false
}

func lambdaDetailTableStyleFunc(row, col int) lipgloss.Style {
	style := lambdaDetailFieldValueStyle

	switch {
	case col == 0 && row%2 == 0:
		style = lambdaDetailFieldNameStyle.Background(lipgloss.Color("#20355c"))
	case col == 0 && row%2 == 1:
		style = lambdaDetailFieldNameStyle
	case col == 1 && row%2 == 0:
		style = lambdaDetailFieldValueStyle.Background(lipgloss.Color("236"))
	case col == 1 && row%2 == 1:
		style = lambdaDetailFieldValueStyle
	}

	return style
}

func newModel(reqCh chan interface{}, accountId string, lambdas []list.Item) model {
	model := model{
		accountId:    accountId,
		reqCh:        reqCh,
		lambdas:      list.New(lambdas, list.NewDefaultDelegate(), 0, 0),
		logStreams:   list.New(nil, list.NewDefaultDelegate(), 0, 0),
		logEvents:    viewport.New(0, 0),
		lambdaDetail: viewport.New(0, 0),
		spinner:      spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle)),
		activeView:   viewLambda,
	}
	model.lambdas.Title = fmt.Sprintf("Viewing Lambdas - Account ID: %s", accountId)
	model.lambdas.KeyMap.NextPage = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "next page"))
	model.lambdas.KeyMap.PrevPage = key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "prev page"))
	model.lambdas.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "invoke")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		}
	}

	return model
}
