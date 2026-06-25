package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type screen int

const (
	screenHome screen = iota
)

// Model is the central state for a Bubble Tea app.
//
// A struct groups related fields together. Here it stores the current screen,
// terminal dimensions, styles, and reusable Bubbles components. Bubble Tea
// passes this value through Init, Update, and View as the app runs.
type Model struct {
	current screen
	width   int
	height  int
	styles  styles
	keys    keyMap
	help    help.Model
}

// NewModel builds the initial app state before Bubble Tea starts sending
// terminal messages.
func NewModel() Model {
	isDark := true
	helpModel := help.New()
	helpModel.Styles = help.DefaultStyles(isDark)

	return Model{
		current: screenHome,
		styles:  newStyles(isDark),
		keys:    newKeyMap(),
		help:    helpModel,
	}
}

// Init is called once when the Bubble Tea program starts.
//
// A command is a function Bubble Tea can run to do side-effectful work, such as
// asking the terminal for information. The result comes back later as a message
// handled by Update.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update receives messages from Bubble Tea and returns the next model.
//
// Messages describe things that happened: a key press, a resize, a command
// result, and so on. Update is where the app reacts to those events and may
// return another command, like tea.Quit.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		isDark := msg.IsDark()
		m.styles = newStyles(isDark)
		m.help.Styles = help.DefaultStyles(isDark)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.home):
			m.current = screenHome
		}
	}

	return m, nil
}

// View renders the current model as a terminal view.
//
// Bubble Tea redraws the terminal after each update. Keeping View as a pure
// state-to-rendered-output function makes the UI easier to reason about and
// test. Bubble Tea v2 wraps the string content in tea.View so it can also carry
// terminal metadata later, such as cursor and mouse settings.
func (m Model) View() tea.View {
	switch m.current {
	case screenHome:
		return tea.NewView(m.homeView())
	default:
		return tea.NewView(m.homeView())
	}
}

func (m Model) homeView() string {
	body := strings.Join([]string{
		m.styles.title.Render("Vanish"),
		"",
		m.styles.body.Render("Local-first review and cleanup planning for your social media footprint."),
		m.styles.muted.Render("No cloud backend. No telemetry by default. No hidden background actions."),
		"",
		m.styles.body.Render("v0.1 will focus on importing local data exports, reviewing activity, and writing a dry-run cleanup plan."),
		"",
		m.styles.help.Render(m.help.View(m.keys)),
	}, "\n")

	if m.width > 0 {
		return m.styles.frame.Width(m.width).Render(body)
	}

	return m.styles.frame.Render(body)
}

type keyMap struct {
	home key.Binding
	quit key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		home: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "home"),
		),
		quit: key.NewBinding(
			// Bubble Tea reports shift+q as the printable character "Q".
			key.WithKeys("Q"),
			key.WithHelp("shift+q", "quit"),
		),
	}
}

// ShortHelp and FullHelp make keyMap satisfy the Bubbles help.KeyMap
// interface. Go interfaces are implicit: a type implements an interface by
// having the methods it asks for, without an "implements" declaration.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.home, k.quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.home, k.quit}}
}

type styles struct {
	frame lipgloss.Style
	title lipgloss.Style
	body  lipgloss.Style
	muted lipgloss.Style
	help  lipgloss.Style
}

func newStyles(isDark bool) styles {
	lightDark := lipgloss.LightDark(isDark)

	return styles{
		frame: lipgloss.NewStyle().
			Padding(1, 2),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#4B2E83"), lipgloss.Color("#D6C7FF"))),
		body: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#24292F"), lipgloss.Color("#E6EDF3"))).
			Width(74),
		muted: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))).
			Width(74),
		help: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#6E7781"), lipgloss.Color("#8B949E"))),
	}
}
