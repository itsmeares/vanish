package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
)

type screen int

const (
	screenHome screen = iota
	screenImportPath
	screenImporting
	screenImportResult
	screenItemsBrowser
	screenWarnings
)

const (
	homeImportZip = iota
	homeDemo
	homeQuit
)

const (
	resultViewItems = iota
	resultViewWarnings
	resultBackHome
)

var homeMenuItems = []string{
	"Import Instagram export ZIP",
	"Demo import with fake local data",
	"Quit",
}

var resultMenuItems = []string{
	"View parsed items",
	"View warnings",
	"Back home",
}

// Model is the central state for a Bubble Tea app.
//
// A struct groups related fields together. Here it stores the current screen,
// terminal dimensions, styles, and reusable Bubbles components. Bubble Tea
// passes this value through Init, Update, and View as the app runs.
type Model struct {
	current       screen
	width         int
	height        int
	styles        styles
	keys          keyMap
	help          help.Model
	pathInput     textinput.Model
	spinner       spinner.Model
	importSource  string
	importResult  instagram.ImportResult
	importErr     error
	homeCursor    int
	resultCursor  int
	itemCursor    int
	itemOffset    int
	warningCursor int
	warningOffset int
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

// Init is called once when Bubble Tea starts.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update receives messages from Bubble Tea and returns the next model.
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
		m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(m.importResult.Items), m.itemListHeight())
		m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())

	case importFinishedMsg:
		m.importResult = msg.result
		m.importErr = msg.err
		m.importSource = msg.source
		m.resultCursor = 0
		m.itemCursor = 0
		m.itemOffset = 0
		m.warningCursor = 0
		m.warningOffset = 0
		m.current = screenImportResult

	case spinner.TickMsg:
		if m.current == screenImporting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.quit) {
			return m, tea.Quit
		}

		switch m.current {
		case screenHome:
			return m.updateHome(msg)
		case screenImportPath:
			return m.updateImportPath(msg)
		case screenImportResult:
			return m.updateImportResult(msg)
		case screenItemsBrowser:
			return m.updateItemsBrowser(msg)
		case screenWarnings:
			return m.updateWarnings(msg)
		}
	}

	return m, nil
}

func (m Model) updateHome(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.homeCursor = moveCursor(m.homeCursor, len(homeMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.homeCursor = moveCursor(m.homeCursor, len(homeMenuItems), 1)
	case key.Matches(msg, m.keys.selectItem):
		switch m.homeCursor {
		case homeImportZip:
			m = m.resetImportState()
			m.current = screenImportPath
			cmd := m.pathInput.Focus()
			return m, cmd
		case homeDemo:
			m = m.resetImportState()
			m.current = screenImporting
			m.importSource = "demo instagram export"
			return m, tea.Batch(startSpinnerCmd(m.spinner), demoImportCmd())
		case homeQuit:
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) updateImportPath(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.start):
		zipPath := strings.TrimSpace(m.pathInput.Value())
		m.current = screenImporting
		m.importSource = zipPath
		m.importErr = nil
		m.importResult = instagram.ImportResult{}
		return m, tea.Batch(startSpinnerCmd(m.spinner), importZIPCmd(zipPath, zipPath))
	case key.Matches(msg, m.keys.back):
		m.current = screenHome
		m.pathInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.pathInput, cmd = m.pathInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateImportResult(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.importErr != nil {
		if key.Matches(msg, m.keys.back) {
			m.current = screenHome
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.up):
		m.resultCursor = moveCursor(m.resultCursor, len(resultMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.resultCursor = moveCursor(m.resultCursor, len(resultMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenHome
	case key.Matches(msg, m.keys.selectItem):
		switch m.resultCursor {
		case resultViewItems:
			m.itemCursor = clampCursor(m.itemCursor, len(m.importResult.Items))
			m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(m.importResult.Items), m.itemListHeight())
			m.current = screenItemsBrowser
		case resultViewWarnings:
			m.warningCursor = clampCursor(m.warningCursor, len(m.importResult.Warnings))
			m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
			m.current = screenWarnings
		case resultBackHome:
			m.current = screenHome
		}
	}

	return m, nil
}

func (m Model) updateItemsBrowser(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.itemCursor = moveCursor(m.itemCursor, len(m.importResult.Items), -1)
	case key.Matches(msg, m.keys.down):
		m.itemCursor = moveCursor(m.itemCursor, len(m.importResult.Items), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenImportResult
	}
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(m.importResult.Items), m.itemListHeight())
	return m, nil
}

func (m Model) updateWarnings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.warningCursor = moveCursor(m.warningCursor, len(m.importResult.Warnings), -1)
	case key.Matches(msg, m.keys.down):
		m.warningCursor = moveCursor(m.warningCursor, len(m.importResult.Warnings), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenImportResult
	}
	m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
	return m, nil
}

// View renders current model as terminal content.
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
	case screenItemsBrowser:
		return tea.NewView(m.itemsBrowserView())
	case screenWarnings:
		return tea.NewView(m.warningsView())
	default:
		return tea.NewView(m.homeView())
	}
}

func (m Model) homeView() string {
	lines := []string{
		m.styles.title.Render("Vanish"),
		"",
		m.styles.body.Render("Local-first review and cleanup planning for your social media footprint."),
		m.styles.muted.Render("No cloud backend. No telemetry by default. No hidden background actions."),
		"",
		m.styles.body.Render("Import a local Instagram data export ZIP to see parsed activity counts."),
		"",
	}
	lines = append(lines, m.renderMenu(homeMenuItems, m.homeCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.quit))

	return m.frame(strings.Join(lines, "\n"))
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
		m.helpLine(m.keys.start, m.keys.back, m.keys.quit),
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
		m.helpLine(m.keys.quit),
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
			m.helpLine(m.keys.back, m.keys.quit),
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
		"",
	}

	lines = append(lines, m.renderMenu(resultMenuItems, m.resultCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) itemsBrowserView() string {
	items := m.importResult.Items
	visibleRows := m.itemListHeight()
	cursor := clampCursor(m.itemCursor, len(items))
	offset := ensureOffset(cursor, m.itemOffset, len(items), visibleRows)

	lines := []string{
		m.styles.title.Render("Parsed Items"),
		"",
		m.styles.muted.Render(fmt.Sprintf("%d parsed items from %s", len(items), emptyFallback(m.importSource, "instagram export"))),
		"",
	}

	if len(items) == 0 {
		lines = append(lines, m.styles.muted.Render("No parsed items."))
	} else {
		end := minInt(len(items), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(itemRow(items[i]), i == cursor))
		}
		if end < len(items) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}

	lines = append(lines, "", m.styles.muted.Render("Details"))
	if len(items) == 0 {
		lines = append(lines, m.styles.muted.Render("No item selected."))
	} else {
		for _, line := range itemDetailLines(items[cursor]) {
			lines = append(lines, m.styles.body.Render(line))
		}
	}

	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.back, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) warningsView() string {
	warnings := m.importResult.Warnings
	visibleRows := m.warningListHeight()
	cursor := clampCursor(m.warningCursor, len(warnings))
	offset := ensureOffset(cursor, m.warningOffset, len(warnings), visibleRows)

	lines := []string{
		m.styles.title.Render("Import Warnings"),
		"",
		m.styles.muted.Render(fmt.Sprintf("%d warnings from %s", len(warnings), emptyFallback(m.importSource, "instagram export"))),
		"",
	}

	if len(warnings) == 0 {
		lines = append(lines, m.styles.muted.Render("No warnings."))
	} else {
		end := minInt(len(warnings), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(warnings[i], i == cursor))
		}
		if end < len(warnings) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}

	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.back, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) renderMenu(items []string, cursor int) []string {
	lines := make([]string, 0, len(items))
	for i, item := range items {
		lines = append(lines, m.renderSelectableLine(item, i == cursor))
	}
	return lines
}

func (m Model) renderSelectableLine(value string, selected bool) string {
	prefix := "  "
	style := m.styles.row
	if selected {
		prefix = "> "
		style = m.styles.selected
	}
	return style.Render(prefix + value)
}

func (m Model) helpLine(bindings ...key.Binding) string {
	return m.styles.help.Render(m.help.View(screenHelp(bindings)))
}

func (m Model) frame(body string) string {
	if m.width > 0 {
		return m.styles.frame.Width(m.width).Render(body)
	}

	return m.styles.frame.Render(body)
}

func (m Model) resetImportState() Model {
	m.importSource = ""
	m.importResult = instagram.ImportResult{}
	m.importErr = nil
	m.resultCursor = 0
	m.itemCursor = 0
	m.itemOffset = 0
	m.warningCursor = 0
	m.warningOffset = 0
	return m
}

func (m Model) itemListHeight() int {
	return boundedListHeight(m.height, 15, 3, 10)
}

func (m Model) warningListHeight() int {
	return boundedListHeight(m.height, 8, 3, 18)
}

type keyMap struct {
	up         key.Binding
	down       key.Binding
	selectItem key.Binding
	start      key.Binding
	back       key.Binding
	quit       key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("up/k", "up"),
		),
		down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("down/j", "down"),
		),
		selectItem: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		start: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "start"),
		),
		back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp and FullHelp make keyMap satisfy the Bubbles help.KeyMap interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.up, k.down, k.selectItem, k.back, k.quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.up, k.down, k.selectItem, k.start, k.back, k.quit}}
}

type screenHelp []key.Binding

func (h screenHelp) ShortHelp() []key.Binding {
	return []key.Binding(h)
}

func (h screenHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{h.ShortHelp()}
}

type styles struct {
	frame    lipgloss.Style
	title    lipgloss.Style
	body     lipgloss.Style
	row      lipgloss.Style
	selected lipgloss.Style
	muted    lipgloss.Style
	help     lipgloss.Style
	error    lipgloss.Style
	warning  lipgloss.Style
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
		row: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#24292F"), lipgloss.Color("#E6EDF3"))),
		selected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#0A3069"), lipgloss.Color("#79C0FF"))),
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

func itemRow(item domain.ActivityItem) string {
	target := firstNonEmptyString(item.TargetURL, item.TargetID)
	occurred := ""
	if item.OccurredAt != nil {
		occurred = item.OccurredAt.Format(time.RFC3339)
	}

	return fmt.Sprintf(
		"%s | %s | %s | %s",
		item.Type,
		emptyFallback(item.Actor, "-"),
		emptyFallback(target, "-"),
		emptyFallback(occurred, "-"),
	)
}

func itemDetailLines(item domain.ActivityItem) []string {
	lines := []string{
		"ID: " + item.ID,
		"Platform: " + string(item.Platform),
		"Type: " + string(item.Type),
	}
	if item.Actor != "" {
		lines = append(lines, "Actor: "+item.Actor)
	}
	if item.TargetURL != "" {
		lines = append(lines, "Target URL: "+item.TargetURL)
	}
	if item.TargetID != "" {
		lines = append(lines, "Target ID: "+item.TargetID)
	}
	if item.OccurredAt != nil {
		lines = append(lines, "Occurred: "+item.OccurredAt.Format(time.RFC3339))
	}
	if item.Source.FileName != "" {
		lines = append(lines, "Source file: "+item.Source.FileName)
	}
	if item.Text != nil && item.Text.Hash != "" {
		lines = append(lines, "Safe text hash: "+item.Text.Hash)
	}
	if len(item.Metadata) > 0 {
		lines = append(lines, "Metadata:")
		keys := make([]string, 0, len(item.Metadata))
		for key := range item.Metadata {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("  %s: %s", key, item.Metadata[key]))
		}
	}

	return lines
}

func moveCursor(cursor, count, delta int) int {
	if count <= 0 {
		return 0
	}
	cursor += delta
	if cursor < 0 {
		return 0
	}
	if cursor >= count {
		return count - 1
	}
	return cursor
}

func clampCursor(cursor, count int) int {
	return moveCursor(cursor, count, 0)
}

func ensureOffset(cursor, offset, count, visible int) int {
	if count <= 0 || visible <= 0 {
		return 0
	}
	cursor = clampCursor(cursor, count)
	if offset < 0 {
		offset = 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visible {
		offset = cursor - visible + 1
	}
	maxOffset := count - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}

func boundedListHeight(height, reserved, minHeight, maxHeight int) int {
	if height <= 0 {
		return maxHeight
	}
	available := height - reserved
	if available < minHeight {
		return minHeight
	}
	if available > maxHeight {
		return maxHeight
	}
	return available
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
