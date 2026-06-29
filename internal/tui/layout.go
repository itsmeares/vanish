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
)

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
	bodyHeight := maxInt(5, height-8)
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
		m.shellHeader(section, spec),
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

func (m Model) shellHeader(section string, spec layoutMetrics) string {
	left := m.styles.title.Render("Vanish") + m.styles.muted.Render(" / "+section)
	badges := m.statusBadges()
	line := left + "  " + badges
	if lipgloss.Width(line) <= spec.contentWidth {
		return line
	}
	return lipgloss.JoinVertical(lipgloss.Left, left, badges)
}

func (m Model) tabs(spec layoutMetrics) string {
	active := m.activeTab()
	labels := []string{"Home", "Import", "Review", "Plans", "Local", "Help"}
	tabs := make([]string, 0, len(labels))
	for _, label := range labels {
		style := m.styles.tab
		if label == active {
			style = m.styles.activeTab
		}
		tabs = append(tabs, style.Render(" "+label+" "))
	}
	line := strings.Join(tabs, " ")
	if lipgloss.Width(line) > spec.contentWidth {
		line = strings.Join(tabs[:minInt(4, len(tabs))], " ")
	}
	return line
}

func (m Model) activeTab() string {
	switch m.current {
	case screenImportPath, screenImporting, screenImportResult, screenWarnings:
		return "Import"
	case screenItemsBrowser, screenFilters, screenSelectionSummary, screenSelectedItems:
		return "Review"
	case screenPlanPreview, screenPlanExportPath, screenPlanLoadPath, screenLoadedPlanSummary, screenLoadedPlanActions:
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

	return m.styles.pane.Width(innerWidth).Height(innerHeight).Render(strings.Join(lines, "\n"))
}

func (m Model) singlePane(section, subtitle string, lines []string, bindings ...key.Binding) string {
	spec := layoutSpec(m.width, m.height)
	body := m.pane(section, subtitle, strings.Join(lines, "\n"), spec.contentWidth, compactPaneHeight(lines, spec.bodyHeight))
	return m.appShell(section, body, m.contextFooter(bindings...))
}

func compactPaneHeight(lines []string, maxHeight int) int {
	height := len(lines) + 4
	if height < 6 {
		height = 6
	}
	if height > maxHeight {
		return maxHeight
	}
	return height
}

func (m Model) twoPane(spec layoutMetrics, leftTitle, leftSubtitle string, leftLines []string, rightTitle, rightSubtitle string, rightLines []string) string {
	if spec.narrow {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.pane(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), spec.contentWidth, maxInt(5, spec.bodyHeight/2)),
			m.pane(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), spec.contentWidth, maxInt(5, spec.bodyHeight/2)),
		)
	}

	leftWidth, rightWidth := twoPaneWidths(spec, leftTitle)

	left := m.pane(leftTitle, leftSubtitle, strings.Join(leftLines, "\n"), leftWidth, spec.bodyHeight)
	right := m.pane(rightTitle, rightSubtitle, strings.Join(rightLines, "\n"), rightWidth, spec.bodyHeight)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", spec.gap), right)
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
	innerWidth := maxInt(10, width-4)
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

func (m Model) statusBadges() string {
	return strings.Join([]string{
		m.styles.badge.Render("[LOCAL]"),
		m.styles.badge.Render("[DRY-RUN]"),
		m.styles.badge.Render("[NO NETWORK]"),
	}, " ")
}

func (m Model) contextFooter(bindings ...key.Binding) string {
	return m.styles.help.Render(m.help.View(screenHelp(bindings)))
}

func (m Model) menuRows(items []string, cursor, width int) []string {
	lines := make([]string, 0, len(items))
	for i, item := range items {
		lines = append(lines, m.selectableLine(item, i == cursor, width))
	}
	return lines
}

func (m Model) tableRows(rows []string, cursor, offset, visible, width int) []string {
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
		out = append(out, m.selectableLine(rows[i], i == cursor, width))
	}
	if end < len(rows) {
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
	innerWidth := maxInt(10, width-4)

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

func (m Model) detailRows(lines []string, width int) []string {
	if len(lines) == 0 {
		return []string{}
	}
	innerWidth := maxInt(10, width-4)
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
	prefix := "  "
	style := m.styles.row
	if selected {
		prefix = "> "
		style = m.styles.selected
	}
	valueWidth := maxInt(4, width-lipgloss.Width(prefix)-4)
	return style.Render(prefix + truncateEnd(value, valueWidth))
}

func twoPaneWidths(spec layoutMetrics, leftTitle string) (int, int) {
	leftWidth := maxInt(36, spec.contentWidth*58/100)
	if leftTitle == "Actions" {
		leftWidth = spec.sidebarWidth
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
