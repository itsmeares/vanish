package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

const (
	narrowBreakpoint = 80
	wideBreakpoint   = 110

	defaultTerminalWidth  = 200
	defaultTerminalHeight = 32
	minTerminalWidth      = 24
	minTerminalHeight     = 8

	footerHome         = "up/down move · enter/click select · ? help · ctrl+q quit"
	footerActionMenu   = "up/down move · enter/click select · esc back · ? help · ctrl+q quit"
	footerParsedItems  = "up/down move · enter/space toggle · pgup/pgdn page · tab focus · f filters · s selection · esc back"
	footerList         = "up/down move · click highlight · esc back · ? help · ctrl+q quit"
	footerImportPicker = "up/down move · enter open/import · left/backspace parent · esc back · wheel scroll"
	footerForm         = "enter submit · esc cancel · backspace edit · ctrl+q quit"
	footerSaveForm     = "enter save · esc cancel · backspace edit · ctrl+q quit"
	footerBusy         = "? help · ctrl+q quit"
	footerConfirm      = "up/down move · enter select · click focus · esc back"
	footerHelp         = "esc/backspace back · ctrl+q quit"
	footerEmpty        = "esc back · home/import tabs · ? help · ctrl+q quit"
)

var tabLabels = []string{"Home", "Import", "Review", "Plans", "Local", "Help"}

type layoutMetrics struct {
	width        int
	height       int
	contentWidth int
	bodyHeight   int
	listHeight   int
	sidebarWidth int
	mainWidth    int
	detailWidth  int
	gap          int
	narrow       bool
	wide         bool
}

type keyValue struct {
	Key   string
	Value string
}

type fixedColumn struct {
	Text  string
	Width int
}

type rowState struct {
	Selected bool
	Hovered  bool
	Disabled bool
}

func layoutSpec(width, height int) layoutMetrics {
	if width <= 0 {
		width = defaultTerminalWidth
	}
	if height <= 0 {
		height = defaultTerminalHeight
	}
	width = maxInt(width, minTerminalWidth)
	height = maxInt(height, minTerminalHeight)

	contentWidth := maxInt(20, width-4)
	bodyHeight := maxInt(5, height-4)
	spec := layoutMetrics{
		width:        width,
		height:       height,
		contentWidth: contentWidth,
		bodyHeight:   bodyHeight,
		listHeight:   boundedListHeight(height, 18, 3, 10),
		gap:          1,
		narrow:       width < narrowBreakpoint,
		wide:         width >= wideBreakpoint,
	}

	switch {
	case spec.wide:
		spec.sidebarWidth = maxInt(42, contentWidth*32/100)
		spec.detailWidth = maxInt(38, contentWidth*34/100)
		spec.mainWidth = contentWidth - spec.sidebarWidth - spec.detailWidth - (spec.gap * 2)
		if spec.mainWidth < 30 {
			spec.mainWidth = 30
			spec.detailWidth = maxInt(24, contentWidth-spec.sidebarWidth-spec.mainWidth-(spec.gap*2))
		}
	case spec.narrow:
		spec.sidebarWidth = contentWidth
		spec.mainWidth = contentWidth
		spec.detailWidth = contentWidth
	default:
		spec.sidebarWidth = maxInt(24, contentWidth/3)
		spec.detailWidth = maxInt(28, contentWidth-spec.sidebarWidth-spec.gap)
		spec.mainWidth = spec.detailWidth
	}

	spec.sidebarWidth = clampWidth(spec.sidebarWidth)
	spec.mainWidth = clampWidth(spec.mainWidth)
	spec.detailWidth = clampWidth(spec.detailWidth)
	return spec
}

func (m Model) appShell(section, body, footer string) string {
	spec := layoutSpec(m.width, m.height)
	parts := []string{
		m.shellHeader(section),
		m.tabs(spec),
	}
	if strings.TrimSpace(body) != "" {
		parts = append(parts, body)
	}
	if strings.TrimSpace(footer) != "" {
		parts = append(parts, m.styles.footer.Width(spec.contentWidth).Render(footer))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return m.styles.frame.Width(spec.contentWidth).Render(content)
}

func (m Model) shellHeader(section string) string {
	return m.styles.title.Render("Vanish") + m.styles.muted.Render(" / "+section)
}

func (m Model) tabs(spec layoutMetrics) string {
	active := m.activeTab()
	lines := []string{}
	lineTabs := []string{}
	lineWidth := 0

	flush := func() {
		if len(lineTabs) == 0 {
			return
		}
		lines = append(lines, strings.Join(lineTabs, " "))
		lineTabs = nil
		lineWidth = 0
	}

	for _, label := range tabLabels {
		style := m.styles.tab
		if label == active {
			style = m.styles.activeTab
		} else if m.hoverTarget.Kind == hitTab && m.hoverTarget.Label == label {
			style = m.styles.hoveredTab
		}
		tab := style.Render(label)
		tabWidth := lipgloss.Width(tab)
		separatorWidth := 0
		if len(lineTabs) > 0 {
			separatorWidth = 1
		}
		if len(lineTabs) > 0 && lineWidth+separatorWidth+tabWidth > spec.contentWidth {
			flush()
			separatorWidth = 0
		}
		lineTabs = append(lineTabs, tab)
		lineWidth += separatorWidth + tabWidth
	}
	flush()
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) activeTab() string {
	switch m.current {
	case screenImportPath, screenImporting, screenImportResult, screenWarnings:
		return "Import"
	case screenItemsBrowser, screenReviewEmpty, screenFilters, screenSelectionSummary, screenSelectedItems:
		return "Review"
	case screenPlanPreview, screenPlanExportPath, screenPlanLoadPath, screenLoadedPlanSummary, screenLoadedPlanActions, screenApplyPreview, screenApplyConfirm, screenApplyRunning, screenApplyResult:
		return "Plans"
	case screenLocalDataOverview, screenRecentImports, screenRecentPlans, screenAuditLog, screenWipeLocalDataConfirm:
		return "Local"
	case screenKeybindings:
		return "Help"
	default:
		return "Home"
	}
}

func (m Model) pane(title, subtitle, body string, width, height int) string {
	return m.paneFocused(title, subtitle, body, width, height, false)
}

func (m Model) paneFocused(title, subtitle, body string, width, height int, focused bool) string {
	width = clampWidth(width)
	height = maxInt(height, 4)
	innerWidth := maxInt(10, width-4)
	innerHeight := maxInt(2, height-2)

	lines := []string{}
	if title != "" {
		lines = append(lines, m.styles.paneTitle.Render(truncateMiddle(title, innerWidth)))
	}
	if subtitle != "" {
		lines = append(lines, m.styles.paneSubtitle.Render(truncateMiddle(subtitle, innerWidth)))
	}
	if strings.TrimSpace(body) != "" {
		bodyLines := clipBodyLines(strings.Split(body, "\n"), maxInt(1, innerHeight-len(lines)))
		lines = append(lines, bodyLines...)
	}

	style := m.styles.pane
	if focused {
		style = m.styles.focusedPane
	}
	return style.Width(innerWidth).Height(innerHeight).Render(strings.Join(lines, "\n"))
}

func (m Model) paneFocusedRenderedHeight(title, subtitle, body string, width, height int, focused bool) string {
	height = maxInt(height, 4)
	renderHeight := height
	rendered := m.paneFocused(title, subtitle, body, width, renderHeight, focused)
	for i := 0; i < 8 && blockHeight(rendered) < height; i++ {
		renderHeight += height - blockHeight(rendered)
		rendered = m.paneFocused(title, subtitle, body, width, renderHeight, focused)
	}
	return rendered
}

func (m Model) paneRenderedHeight(title, subtitle, body string, width, height int) string {
	return m.paneFocusedRenderedHeight(title, subtitle, body, width, height, false)
}

func (m Model) singlePane(section, subtitle string, lines []string, bindings ...key.Binding) string {
	return m.singlePaneFooter(section, subtitle, lines, m.contextFooter(bindings...))
}

func (m Model) singlePaneFooter(section, subtitle string, lines []string, footer string) string {
	spec := layoutSpec(m.width, m.height)
	body := m.pane(section, subtitle, strings.Join(lines, "\n"), spec.contentWidth, spec.bodyHeight)
	return m.appShell(section, body, footer)
}

func (m Model) centeredPaneFooter(section, subtitle string, lines []string, footer string) string {
	spec := layoutSpec(m.width, m.height)
	body := m.pane(section, subtitle, strings.Join(lines, "\n"), spec.contentWidth, spec.bodyHeight)
	return m.appShell(section, body, footer)
}

func centeredActionWidth(width int) int {
	return layoutSpec(width, defaultTerminalHeight).contentWidth
}

func twoPaneBodyHeight(spec layoutMetrics) int {
	if spec.narrow {
		return maxInt(5, spec.bodyHeight/2)
	}
	return spec.bodyHeight
}

func (m Model) twoPane(spec layoutMetrics, leftTitle, leftSubtitle string, leftLines []string, rightTitle, rightSubtitle string, rightLines []string) string {
	return m.twoPaneFocused(spec, leftTitle, leftSubtitle, leftLines, false, rightTitle, rightSubtitle, rightLines, false)
}

func (m Model) twoPaneFocused(spec layoutMetrics, leftTitle, leftSubtitle string, leftLines []string, leftFocused bool, rightTitle, rightSubtitle string, rightLines []string, rightFocused bool) string {
	if spec.narrow {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.paneFocused(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), spec.contentWidth, twoPaneBodyHeight(spec), leftFocused),
			m.paneFocused(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), spec.contentWidth, twoPaneBodyHeight(spec), rightFocused),
		)
	}

	leftWidth, rightWidth := twoPaneWidths(spec, leftTitle)

	left := m.paneFocused(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), leftWidth, spec.bodyHeight, leftFocused)
	right := m.paneFocused(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), rightWidth, spec.bodyHeight, rightFocused)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", spec.gap), right)
}

func (m Model) compactTwoPane(spec layoutMetrics, leftTitle, leftSubtitle string, leftLines []string, rightTitle, rightSubtitle string, rightLines []string) string {
	return m.compactTwoPaneFocused(spec, leftTitle, leftSubtitle, leftLines, false, rightTitle, rightSubtitle, rightLines, false)
}

func (m Model) compactTwoPaneFocused(spec layoutMetrics, leftTitle, leftSubtitle string, leftLines []string, leftFocused bool, rightTitle, rightSubtitle string, rightLines []string, rightFocused bool) string {
	if spec.narrow {
		leftHeight := compactPaneContentHeight(leftTitle, leftSubtitle, leftLines, twoPaneBodyHeight(spec))
		rightHeight := compactPaneContentHeight(rightTitle, rightSubtitle, rightLines, twoPaneBodyHeight(spec))
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.paneFocused(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), spec.contentWidth, leftHeight, leftFocused),
			m.paneFocused(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), spec.contentWidth, rightHeight, rightFocused),
		)
	}

	leftWidth, rightWidth := twoPaneWidths(spec, leftTitle)
	leftHeight := compactPaneContentHeight(leftTitle, leftSubtitle, leftLines, spec.bodyHeight)
	rightHeight := compactPaneContentHeight(rightTitle, rightSubtitle, rightLines, spec.bodyHeight)
	height := maxInt(leftHeight, rightHeight)
	left := m.paneFocused(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), leftWidth, height, leftFocused)
	right := m.paneFocused(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), rightWidth, height, rightFocused)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", spec.gap), right)
}

func compactPaneContentHeight(title, subtitle string, lines []string, maxHeight int) int {
	bodyLines := len(lines)
	if bodyLines == 0 {
		bodyLines = 1
	}
	innerHeight := bodyLines
	if title != "" {
		innerHeight++
	}
	if subtitle != "" {
		innerHeight++
	}
	height := innerHeight + 2
	if height < 5 {
		height = 5
	}
	if maxHeight > 0 && height > maxHeight {
		return maxHeight
	}
	return height
}

func blockHeight(block string) int {
	if block == "" {
		return 0
	}
	return strings.Count(block, "\n") + 1
}

func (m Model) dashboardSections(width int, sections ...[]string) []string {
	lines := []string{}
	for _, section := range sections {
		if len(section) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, section...)
	}
	return clipBodyLines(lines, maxInt(4, layoutSpec(m.width, m.height).bodyHeight-2))
}

func (m Model) section(title string, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := []string{m.styles.separator.Render(title)}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (m Model) keyValueRows(rows []keyValue) []string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		key := strings.TrimSpace(row.Key)
		value := strings.TrimSpace(row.Value)
		if key == "" || value == "" {
			continue
		}
		lines = append(lines, m.styles.body.Render(fmt.Sprintf("%s: %s", key, value)))
	}
	return lines
}

func (m Model) warningBanner(message string, width int) []string {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	innerWidth := paneTextWidth(width)
	return []string{m.styles.warning.Render(truncateEnd("Warning: "+message, innerWidth))}
}

func (m Model) threePane(spec layoutMetrics, leftTitle, leftSubtitle string, leftLines []string, mainTitle, mainSubtitle string, mainLines []string, rightTitle, rightSubtitle string, rightLines []string) string {
	if spec.wide {
		left := m.pane(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), spec.sidebarWidth, spec.bodyHeight)
		main := m.pane(mainTitle, mainSubtitle, strings.Join(mainLines, "\n"), spec.mainWidth, spec.bodyHeight)
		right := m.pane(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), spec.detailWidth, spec.bodyHeight)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", spec.gap), main, strings.Repeat(" ", spec.gap), right)
	}

	if spec.narrow {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.pane(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), spec.contentWidth, maxInt(5, spec.bodyHeight/3)),
			m.pane(mainTitle, mainSubtitle, strings.Join(mainLines, "\n"), spec.contentWidth, maxInt(5, spec.bodyHeight/3)),
			m.pane(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), spec.contentWidth, maxInt(5, spec.bodyHeight/3)),
		)
	}

	left := m.pane(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), spec.sidebarWidth, spec.bodyHeight)
	combinedRightLines := append([]string{}, mainLines...)
	combinedRightLines = append(combinedRightLines, "")
	combinedRightLines = append(combinedRightLines, rightLines...)
	right := m.pane(rightTitle, rightSubtitle, strings.Join(combinedRightLines, "\n"), spec.detailWidth, spec.bodyHeight)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", spec.gap), right)
}

func (m Model) contextFooter(bindings ...key.Binding) string {
	if len(bindings) == 0 {
		return ""
	}
	return m.footer("up/down move · enter select · esc back · ? help · ctrl+q quit")
}

func (m Model) footer(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	parts := strings.Split(text, " · ")
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		keyText, desc, ok := strings.Cut(part, " ")
		if !ok {
			rendered = append(rendered, m.styles.footerKey.Render(keyText))
			continue
		}
		rendered = append(rendered, m.styles.footerKey.Render(keyText)+" "+m.styles.help.Render(desc))
	}
	return strings.Join(rendered, m.styles.help.Render(" · "))
}

func (m Model) menuRows(items []string, cursor, width int, kind hitKind) []string {
	return m.menuRowsWithDisabled(items, nil, cursor, width, kind)
}

func (m Model) menuRowsWithDisabled(items []string, disabled map[int]bool, cursor, width int, kind hitKind) []string {
	lines := make([]string, 0, len(items))
	for i, item := range items {
		lines = append(lines, m.controlRowState(item, rowState{
			Selected: i == cursor,
			Hovered:  m.hoverTarget.Kind == kind && m.hoverTarget.Index == i,
			Disabled: disabled != nil && disabled[i],
		}, width, ""))
	}
	return lines
}

func (m Model) tableRows(rows []string, cursor, offset, visible, width int, kind hitKind) []string {
	return m.tableRowsWithDisabled(rows, nil, cursor, offset, visible, width, kind)
}

func (m Model) tableRowsWithDisabled(rows []string, disabled map[int]bool, cursor, offset, visible, width int, kind hitKind) []string {
	if len(rows) == 0 {
		return []string{}
	}
	visible = maxInt(1, visible)
	cursor = clampCursor(cursor, len(rows))
	offset = ensureOffset(cursor, offset, len(rows), visible)
	end := minInt(len(rows), offset+visible)

	out := []string{}
	if offset > 0 {
		out = append(out, m.styles.muted.Render("..."))
	}
	for i := offset; i < end; i++ {
		out = append(out, m.controlRowState(rows[i], rowState{
			Selected: i == cursor,
			Hovered:  m.hoverTarget.Kind == kind && m.hoverTarget.Index == i,
			Disabled: disabled != nil && disabled[i],
		}, width, ""))
	}
	if end < len(rows) {
		out = append(out, m.styles.muted.Render("..."))
	}
	return out
}

func (m Model) windowedTableRows(total, cursor, offset, visible, width int, kind hitKind, row func(int) string) []string {
	if total <= 0 || row == nil {
		return []string{}
	}
	visible = maxInt(1, visible)
	cursor = clampCursor(cursor, total)
	offset = ensureOffset(cursor, offset, total, visible)
	end := minInt(total, offset+visible)

	out := make([]string, 0, visible+2)
	if offset > 0 {
		out = append(out, m.styles.muted.Render("..."))
	}
	for i := offset; i < end; i++ {
		out = append(out, m.controlRowState(row(i), rowState{
			Selected: i == cursor,
			Hovered:  m.hoverTarget.Kind == kind && m.hoverTarget.Index == i,
		}, width, ""))
	}
	if end < total {
		out = append(out, m.styles.muted.Render("..."))
	}
	return out
}

func (m Model) plainRows(rows []string, offset, visible, width int) []string {
	if len(rows) == 0 {
		return []string{}
	}
	visible = maxInt(1, visible)
	offset = ensureOffset(0, offset, len(rows), visible)
	end := minInt(len(rows), offset+visible)
	innerWidth := paneTextWidth(width)

	out := []string{}
	if offset > 0 {
		out = append(out, m.styles.muted.Render("..."))
	}
	for i := offset; i < end; i++ {
		out = append(out, m.styles.body.Render(truncateEnd(rows[i], innerWidth)))
	}
	if end < len(rows) {
		out = append(out, m.styles.muted.Render("..."))
	}
	return out
}

func (m Model) windowedPlainRows(total, offset, visible, width int, row func(int) string) []string {
	if total <= 0 || row == nil {
		return []string{}
	}
	visible = maxInt(1, visible)
	offset = ensureOffset(0, offset, total, visible)
	end := minInt(total, offset+visible)
	innerWidth := paneTextWidth(width)

	out := make([]string, 0, visible+2)
	if offset > 0 {
		out = append(out, m.styles.muted.Render("..."))
	}
	for i := offset; i < end; i++ {
		out = append(out, m.styles.body.Render(truncateEnd(row(i), innerWidth)))
	}
	if end < total {
		out = append(out, m.styles.muted.Render("..."))
	}
	return out
}

func (m Model) detailRows(lines []string, width int) []string {
	if len(lines) == 0 {
		return []string{}
	}
	innerWidth := paneTextWidth(width)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, m.styles.body.Render(truncateMiddle(line, innerWidth)))
	}
	return out
}

func detailSection(title string, lines ...string) []string {
	cleaned := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleaned = append(cleaned, line)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return append([]string{title}, cleaned...)
}

func appendDetailSection(lines []string, section []string) []string {
	if len(section) == 0 {
		return lines
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, section...)
}

func (m Model) emptyState(message string) string {
	return m.styles.muted.Render(message)
}

func (m Model) notice(kind, message string) string {
	switch kind {
	case "error":
		return m.styles.error.Render(message)
	case "success":
		return m.styles.success.Render(message)
	case "warning":
		return m.styles.warning.Render(message)
	default:
		return m.styles.body.Render(message)
	}
}

func (m Model) selectableLine(value string, selected bool, width int) string {
	return m.controlRow(value, selected, width, "")
}

func (m Model) selectableLineTarget(value string, selected bool, width int, kind hitKind, index int) string {
	return m.controlRowState(value, rowState{
		Selected: selected,
		Hovered:  m.hoverTarget.Kind == kind && m.hoverTarget.Index == index,
	}, width, "")
}

func (m Model) controlRow(value string, selected bool, width int, hint string) string {
	return m.controlRowState(value, rowState{Selected: selected}, width, hint)
}

func (m Model) controlRowState(value string, state rowState, width int, hint string) string {
	style := m.styles.row
	switch {
	case state.Selected:
		style = m.styles.selected
	case state.Disabled:
		style = m.styles.disabledRow
	case state.Hovered:
		style = m.styles.hoveredRow
	}
	innerWidth := paneTextWidth(width)
	content := rowContent(value, hint, innerWidth)
	return style.Width(innerWidth).Render(content)
}

func paneTextWidth(width int) int {
	return maxInt(4, width-8)
}

func rowContent(value, hint string, width int) string {
	value = strings.TrimSpace(value)
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return padRight(truncateEnd(value, width), width)
	}
	hintWidth := lipgloss.Width(hint)
	if hintWidth+2 >= width {
		return padRight(truncateEnd(value, width), width)
	}
	valueWidth := maxInt(1, width-hintWidth-1)
	left := truncateEnd(value, valueWidth)
	gap := maxInt(1, width-lipgloss.Width(left)-hintWidth)
	return left + strings.Repeat(" ", gap) + hint
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	valueWidth := lipgloss.Width(value)
	if valueWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-valueWidth)
}

func twoPaneWidths(spec layoutMetrics, leftTitle string) (int, int) {
	leftWidth := maxInt(36, spec.contentWidth*58/100)
	if leftTitle == "Actions" {
		leftWidth = spec.sidebarWidth
	}
	if leftTitle == "Start" {
		leftWidth = maxInt(30, spec.contentWidth*34/100)
	}
	if leftWidth > spec.contentWidth-spec.gap-24 {
		leftWidth = spec.contentWidth - spec.gap - 24
	}
	rightWidth := spec.contentWidth - leftWidth - spec.gap
	return clampWidth(leftWidth), clampWidth(rightWidth)
}

func truncateMiddle(value string, width int) string {
	value = strings.TrimSpace(value)
	if width <= 0 || value == "" {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	if width <= 3 {
		return string(runes[:minInt(width, len(runes))])
	}
	ellipsis := "..."
	keep := width - len(ellipsis)
	if keep <= 0 {
		return ellipsis[:width]
	}
	left := keep/2 + keep%2
	right := keep / 2
	if left+right > len(runes) {
		return value
	}
	return string(runes[:left]) + ellipsis + string(runes[len(runes)-right:])
}

func truncateEnd(value string, width int) string {
	value = strings.TrimSpace(value)
	if width <= 0 || value == "" {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	if width <= 3 {
		return string(runes[:minInt(width, len(runes))])
	}
	return string(runes[:minInt(width-3, len(runes))]) + "..."
}

func fixedWidthRow(columns ...fixedColumn) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		width := maxInt(1, column.Width)
		text := truncateEnd(column.Text, width)
		parts = append(parts, fmt.Sprintf("%-*s", width, text))
	}
	return strings.TrimRight(strings.Join(parts, " "), " ")
}

func compactCount(count int) string {
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	if count < 10000 {
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	}
	return fmt.Sprintf("%dk", count/1000)
}

func exactCount(count int) string {
	digits := fmt.Sprintf("%d", count)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = strings.TrimPrefix(digits, "-")
	}
	for index := len(digits) - 3; index > 0; index -= 3 {
		digits = digits[:index] + "," + digits[index:]
	}
	return sign + digits
}

func clipBodyLines(lines []string, maxLines int) []string {
	maxLines = maxInt(1, maxLines)
	if len(lines) <= maxLines {
		return lines
	}
	clipped := append([]string{}, lines[:maxLines]...)
	clipped[len(clipped)-1] = "..."
	return clipped
}

func clampWidth(width int) int {
	return maxInt(12, width)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
