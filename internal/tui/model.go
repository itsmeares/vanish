package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/itsmeares/vanish/internal/instagram"
)

type screen int

const (
	screenHome screen = iota
	screenImportPath
	screenImporting
	screenImportResult
)

// Model is the central state for a Bubble Tea app.
//
// A struct groups related fields together. Here it stores the current screen,
// terminal dimensions, styles, and reusable Bubbles components. Bubble Tea
// passes this value through Init, Update, and View as the app runs.
type Model struct {
	current      screen
	width        int
	height       int
	styles       styles
	keys         keyMap
	help         help.Model
	pathInput    textinput.Model
	spinner      spinner.Model
	importSource string
	importResult instagram.ImportResult
	importErr    error
}

// NewModel builds the initial app state before Bubble Tea starts sending
// terminal messages.
func NewModel() Model {
	isDark := true
	helpModel := help.New()
	helpModel.Styles = help.DefaultStyles(isDark)

	pathInput := textinput.New()
	pathInput.Placeholder = `C:\path\to\instagram-export.zip`
	pathInput.Prompt = "> "
	pathInput.CharLimit = 1024
	pathInput.SetWidth(74)

	return Model{
		current:   screenHome,
		styles:    newStyles(isDark),
		keys:      newKeyMap(),
		help:      helpModel,
		pathInput: pathInput,
		spinner:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
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
		m.pathInput.SetWidth(inputWidth(msg.Width))

	case importFinishedMsg:
		m.importResult = msg.result
		m.importErr = msg.err
		m.importSource = msg.source
		m.current = screenImportResult

	case spinner.TickMsg:
		if m.current == screenImporting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.quit):
			return m, tea.Quit
		case m.current != screenImportPath && key.Matches(msg, m.keys.home):
			m.current = screenHome
		}

		switch m.current {
		case screenHome:
			switch {
			case key.Matches(msg, m.keys.importZip):
				m.current = screenImportPath
				m.importErr = nil
				m.importResult = instagram.ImportResult{}
				cmd := m.pathInput.Focus()
				return m, cmd
			case key.Matches(msg, m.keys.demo):
				m.current = screenImporting
				m.importSource = "demo instagram export"
				m.importErr = nil
				m.importResult = instagram.ImportResult{}
				return m, tea.Batch(startSpinnerCmd(m.spinner), demoImportCmd())
			}

		case screenImportPath:
			switch {
			case key.Matches(msg, m.keys.start):
				zipPath := strings.TrimSpace(m.pathInput.Value())
				m.current = screenImporting
				m.importSource = zipPath
				m.importErr = nil
				m.importResult = instagram.ImportResult{}
				return m, tea.Batch(startSpinnerCmd(m.spinner), importZIPCmd(zipPath, zipPath))
			case msg.String() == "esc":
				m.current = screenHome
				m.pathInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.pathInput, cmd = m.pathInput.Update(msg)
				return m, cmd
			}
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
	case screenImportPath:
		return tea.NewView(m.importPathView())
	case screenImporting:
		return tea.NewView(m.importingView())
	case screenImportResult:
		return tea.NewView(m.importResultView())
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
		m.styles.body.Render("Import a local Instagram data export ZIP to see parsed activity counts."),
		"",
		m.styles.body.Render("i  Import Instagram export ZIP"),
		m.styles.body.Render("d  Demo import with fake local data"),
		"",
		m.styles.help.Render(m.help.View(m.keys)),
	}, "\n")

	return m.frame(body)
}

func (m Model) importPathView() string {
	body := strings.Join([]string{
		m.styles.title.Render("Import Instagram Export"),
		"",
		m.styles.body.Render("Type the path to a local Instagram data export .zip file."),
		m.styles.muted.Render("Vanish will only read local JSON files from the ZIP."),
		"",
		m.pathInput.View(),
		"",
		m.styles.help.Render("enter start import  esc home  shift+q quit"),
	}, "\n")

	return m.frame(body)
}

func (m Model) importingView() string {
	source := m.importSource
	if source == "" {
		source = "instagram export"
	}

	body := strings.Join([]string{
		m.styles.title.Render("Importing"),
		"",
		m.styles.body.Render(fmt.Sprintf("%s Parsing local ZIP...", m.spinner.View())),
		m.styles.muted.Render(source),
		"",
		m.styles.help.Render("h home  shift+q quit"),
	}, "\n")

	return m.frame(body)
}

func (m Model) importResultView() string {
	if m.importErr != nil {
		body := strings.Join([]string{
			m.styles.title.Render("Import Failed"),
			"",
			m.styles.error.Render(m.importErr.Error()),
			m.styles.muted.Render(m.importSource),
			"",
			m.styles.help.Render("h home  shift+q quit"),
		}, "\n")

		return m.frame(body)
	}

	summary := m.importResult.Summary
	lines := []string{
		m.styles.title.Render("Import Complete"),
		"",
		m.styles.body.Render(fmt.Sprintf("Source: %s", emptyFallback(m.importSource, "instagram export"))),
		"",
		m.styles.body.Render(fmt.Sprintf("Total parsed items: %d", summary.Total)),
		m.styles.body.Render(fmt.Sprintf("Likes: %d", summary.Likes)),
		m.styles.body.Render(fmt.Sprintf("Comments: %d", summary.Comments)),
		m.styles.body.Render(fmt.Sprintf("Following: %d", summary.Following)),
		m.styles.body.Render(fmt.Sprintf("Followers: %d", summary.Followers)),
		m.styles.body.Render(fmt.Sprintf("Skipped or unknown files: %d", summary.Skipped)),
		m.styles.body.Render(fmt.Sprintf("Warnings: %d", len(m.importResult.Warnings))),
	}

	if len(m.importResult.Warnings) > 0 {
		lines = append(lines, "", m.styles.muted.Render("Warnings"))
		for _, warning := range previewWarnings(m.importResult.Warnings, 5) {
			lines = append(lines, m.styles.warning.Render("- "+warning))
		}
		if len(m.importResult.Warnings) > 5 {
			lines = append(lines, m.styles.muted.Render(fmt.Sprintf("...and %d more", len(m.importResult.Warnings)-5)))
		}
	}

	lines = append(lines, "", m.styles.help.Render("h home  shift+q quit"))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) frame(body string) string {
	if m.width > 0 {
		return m.styles.frame.Width(m.width).Render(body)
	}

	return m.styles.frame.Render(body)
}

type keyMap struct {
	home      key.Binding
	quit      key.Binding
	importZip key.Binding
	demo      key.Binding
	start     key.Binding
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
		importZip: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "import"),
		),
		demo: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "demo"),
		),
		start: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "start"),
		),
	}
}

// ShortHelp and FullHelp make keyMap satisfy the Bubbles help.KeyMap
// interface. Go interfaces are implicit: a type implements an interface by
// having the methods it asks for, without an "implements" declaration.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.importZip, k.demo, k.home, k.quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.importZip, k.demo, k.start, k.home, k.quit}}
}

type styles struct {
	frame   lipgloss.Style
	title   lipgloss.Style
	body    lipgloss.Style
	muted   lipgloss.Style
	help    lipgloss.Style
	error   lipgloss.Style
	warning lipgloss.Style
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
		error: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#B42318"), lipgloss.Color("#FFB4A8"))).
			Width(74),
		warning: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#8A6100"), lipgloss.Color("#FFD479"))).
			Width(74),
	}
}

type importFinishedMsg struct {
	result instagram.ImportResult
	err    error
	source string
}

func importZIPCmd(zipPath, source string) tea.Cmd {
	return func() tea.Msg {
		result, err := instagram.ImportZIP(zipPath)
		return importFinishedMsg{result: result, err: err, source: source}
	}
}

func demoImportCmd() tea.Cmd {
	return func() tea.Msg {
		demoPath, err := instagram.CreateDemoExportZIP("")
		if err != nil {
			return importFinishedMsg{err: err, source: "demo instagram export"}
		}
		defer os.Remove(demoPath)

		result, err := instagram.ImportZIP(demoPath)
		return importFinishedMsg{result: result, err: err, source: "demo instagram export"}
	}
}

func startSpinnerCmd(spinnerModel spinner.Model) tea.Cmd {
	return func() tea.Msg {
		return spinnerModel.Tick()
	}
}

func previewWarnings(warnings []string, limit int) []string {
	if len(warnings) <= limit {
		return warnings
	}
	return warnings[:limit]
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func inputWidth(width int) int {
	if width <= 0 {
		return 74
	}
	available := width - 6
	if available < 24 {
		return 24
	}
	if available > 90 {
		return 90
	}
	return available
}
