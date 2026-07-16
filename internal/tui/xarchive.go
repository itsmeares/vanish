package tui

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/itsmeares/vanish/internal/xarchive"
)

const xArchiveSettingsURL = "https://x.com/settings/download_your_data"

type xImportFinishedMsg struct {
	result xarchive.ImportResult
	err    error
}

type xArchivePageOpenedMsg struct{ err error }
type xMediaOpenedMsg struct{ err error }

var openLocalMedia = openLocalFile

func xImportZIPCmd(store *xarchive.Store, archivePath string) tea.Cmd {
	return func() tea.Msg {
		if store == nil {
			return xImportFinishedMsg{err: fmt.Errorf("local workspace is unavailable")}
		}
		result, err := store.ImportZIP(archivePath, false)
		return xImportFinishedMsg{result: result, err: err}
	}
}

func xDemoImportCmd(store *xarchive.Store) tea.Cmd {
	return func() tea.Msg {
		if store == nil {
			return xImportFinishedMsg{err: fmt.Errorf("local workspace is unavailable")}
		}
		result, err := store.ImportDemo()
		return xImportFinishedMsg{result: result, err: err}
	}
}

func openXArchivePageCmd() tea.Cmd {
	return func() tea.Msg { return xArchivePageOpenedMsg{err: openExternalURL(xArchiveSettingsURL)} }
}

func openXMediaCmd(path string) tea.Cmd {
	return func() tea.Msg { return xMediaOpenedMsg{err: openLocalMedia(path)} }
}

func (m Model) xStore() *xarchive.Store {
	if m.localWorkspace == nil {
		return nil
	}
	return xarchive.NewStore(m.localWorkspace.Dir())
}

func (m *Model) refreshXDatasets() {
	store := m.xStore()
	if store == nil {
		m.xDatasets = nil
		return
	}
	datasets, err := store.List()
	if err != nil {
		m.xDatasets = nil
		m.warnLocalData("load X archives", err)
		return
	}
	m.xDatasets = datasets
	m.xArchiveCursor = clampCursor(m.xArchiveCursor, len(m.xDatasets))
	m.xArchiveOffset = ensureOffset(m.xArchiveCursor, m.xArchiveOffset, len(m.xDatasets), m.localDataListHeight())
}

func (m *Model) openLatestXDataset() bool {
	m.refreshXDatasets()
	if len(m.xDatasets) == 0 {
		return false
	}
	if !m.openXDataset(m.xDatasets[0].DatasetID) {
		return false
	}
	m.xBrowserReturn = screenHome
	return true
}

func (m *Model) openXDataset(id string) bool {
	store := m.xStore()
	if store == nil {
		m.xError = "Local data is unavailable in this run."
		return false
	}
	dataset, err := store.Open(id)
	if err != nil {
		m.xError = "The selected X archive could not be opened."
		return false
	}
	m.xDataset = dataset
	m.xCursor = 0
	m.xOffset = 0
	m.xDetailOffset = 0
	m.xDetailTab = 0
	m.xMediaCursor = 0
	m.xError = ""
	m.xStatus = ""
	m.refreshXSelected()
	return true
}

func (m Model) xDatasetLen() int {
	if m.xDataset == nil {
		return 0
	}
	return m.xDataset.Len()
}

func (m *Model) refreshXSelected() {
	m.xSelected = xarchive.Activity{}
	m.xSelectedLoaded = false
	if m.xDataset == nil || m.xDataset.Len() == 0 {
		return
	}
	m.xCursor = clampCursor(m.xCursor, m.xDataset.Len())
	activity, err := m.xDataset.ActivityAt(m.xCursor)
	if err != nil {
		m.xError = "The selected post could not be read."
		return
	}
	m.xSelected = activity
	m.xSelectedLoaded = true
	m.xError = ""
	m.xDetailOffset = 0
	m.xMediaCursor = clampCursor(m.xMediaCursor, len(activity.Media))
}

func (m *Model) recordXImport(result xarchive.ImportResult) {
	if m.localWorkspace == nil {
		return
	}
	counts := result.Summary.Counts
	m.appendAudit("x_archive_import_completed", map[string]any{
		"demo": result.Summary.Demo, "item_count": counts.Total, "post_count": counts.Posts,
		"reply_count": counts.Replies, "quote_post_count": counts.QuotePosts,
		"repost_count": counts.Reposts, "media_count": counts.Media,
		"warning_count": result.Summary.WarningCount,
	})
}

func (m Model) updateXImportResult(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if keyMatchesBack(msg) {
		m.current = screenHome
		return m, nil
	}
	if keyMatchesUp(msg) {
		m.resultCursor = moveCursor(m.resultCursor, 3, -1)
		return m, nil
	}
	if keyMatchesDown(msg) {
		m.resultCursor = moveCursor(m.resultCursor, 3, 1)
		return m, nil
	}
	if !keyMatchesSelect(msg) || m.xImportErr != nil {
		return m, nil
	}
	switch m.resultCursor {
	case 0:
		m.xBrowserReturn = screenXImportResult
		m.current = screenXBrowser
	case 1:
		m.warningCursor = 0
		m.warningOffset = 0
		m.current = screenXWarnings
	default:
		m.current = screenHome
	}
	return m, nil
}

func (m Model) updateXBrowser(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	count := m.xDatasetLen()
	switch {
	case keyMatchesUp(msg):
		m.xCursor = moveCursor(m.xCursor, count, -1)
		m.refreshXSelected()
	case keyMatchesDown(msg):
		m.xCursor = moveCursor(m.xCursor, count, 1)
		m.refreshXSelected()
	case msg.Code == tea.KeyPgUp:
		m.xCursor = maxInt(0, m.xCursor-m.localDataListHeight())
		m.refreshXSelected()
	case msg.Code == tea.KeyPgDown:
		m.xCursor = minInt(maxInt(0, count-1), m.xCursor+m.localDataListHeight())
		m.refreshXSelected()
	case keyMatchesSelect(msg):
		if m.xSelectedLoaded {
			m.xDetailTab = 0
			m.xDetailOffset = 0
			m.current = screenXDetail
		}
	case keyMatchesBack(msg):
		if m.xBrowserReturn == 0 {
			m.current = screenHome
		} else {
			m.current = m.xBrowserReturn
		}
	}
	m.xOffset = ensureOffset(m.xCursor, m.xOffset, count, m.localDataListHeight())
	return m, nil
}

func (m Model) updateXDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyTab {
		m.xDetailTab = (m.xDetailTab + 1) % 2
		m.xError = ""
		m.xStatus = ""
		return m, nil
	}
	if keyMatchesBack(msg) {
		m.current = screenXBrowser
		return m, nil
	}
	if m.xDetailTab == 0 {
		maxOffset := maxInt(0, len(m.xDetailTextLines())-m.xDetailVisibleRows())
		switch {
		case keyMatchesUp(msg):
			m.xDetailOffset = maxInt(0, m.xDetailOffset-1)
		case keyMatchesDown(msg):
			m.xDetailOffset = minInt(maxOffset, m.xDetailOffset+1)
		case msg.Code == tea.KeyPgUp:
			m.xDetailOffset = maxInt(0, m.xDetailOffset-m.xDetailVisibleRows())
		case msg.Code == tea.KeyPgDown:
			m.xDetailOffset = minInt(maxOffset, m.xDetailOffset+m.xDetailVisibleRows())
		}
		return m, nil
	}
	mediaCount := len(m.xSelected.Media)
	switch {
	case keyMatchesUp(msg):
		m.xMediaCursor = moveCursor(m.xMediaCursor, mediaCount, -1)
	case keyMatchesDown(msg):
		m.xMediaCursor = moveCursor(m.xMediaCursor, mediaCount, 1)
	case keyMatchesSelect(msg):
		if mediaCount == 0 || m.xDataset == nil {
			return m, nil
		}
		ref := m.xSelected.Media[clampCursor(m.xMediaCursor, mediaCount)]
		path, err := m.xDataset.ResolveMedia(ref)
		if err != nil {
			m.xError = "Local media is unavailable."
			return m, nil
		}
		m.xError = ""
		m.xStatus = ""
		return m, openXMediaCmd(path)
	}
	return m, nil
}

func (m Model) updateXWarnings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	groups := m.xImportResult.Warnings.Groups
	switch {
	case keyMatchesUp(msg):
		m.warningCursor = moveCursor(m.warningCursor, len(groups), -1)
	case keyMatchesDown(msg):
		m.warningCursor = moveCursor(m.warningCursor, len(groups), 1)
	case keyMatchesBack(msg):
		m.current = screenXImportResult
	}
	m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(groups), m.localDataListHeight())
	return m, nil
}

func (m Model) updateXArchives(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatchesUp(msg):
		m.xArchiveCursor = moveCursor(m.xArchiveCursor, len(m.xDatasets), -1)
	case keyMatchesDown(msg):
		m.xArchiveCursor = moveCursor(m.xArchiveCursor, len(m.xDatasets), 1)
	case keyMatchesSelect(msg):
		if len(m.xDatasets) > 0 && m.openXDataset(m.xDatasets[clampCursor(m.xArchiveCursor, len(m.xDatasets))].DatasetID) {
			m.xBrowserReturn = screenXArchives
			m.current = screenXBrowser
		}
	case keyMatchesBack(msg):
		m.openLocalDataOverview()
	}
	m.xArchiveOffset = ensureOffset(m.xArchiveCursor, m.xArchiveOffset, len(m.xDatasets), m.localDataListHeight())
	return m, nil
}

func (m Model) xImportingView() string {
	lines := []string{
		m.styles.body.Render(fmt.Sprintf("%s Reading supported current posts...", m.spinner.View())),
		m.styles.muted.Render("The original ZIP is not modified or copied."),
	}
	return m.centeredPaneFooter("Importing X Archive", "Local-only", lines, m.footer(footerBusy))
}

func (m Model) xImportResultView() string {
	if m.xImportErr != nil {
		lines := []string{m.notice("error", m.xImportErr.Error()), m.styles.muted.Render("No X archive dataset was retained.")}
		return m.singlePaneFooter("X Archive Import Failed", "No remote changes", lines, m.footer(footerEmpty))
	}
	counts := m.xImportResult.Summary.Counts
	items := []string{"Browse posts", "View warnings", "Back home"}
	m.resultCursor = clampCursor(m.resultCursor, len(items))
	lines := []string{
		m.styles.body.Render("Account: " + xAccountIdentity(m.xImportResult.Summary.Account)),
		m.styles.body.Render(fmt.Sprintf("Posts: %d", counts.Posts)),
		m.styles.body.Render(fmt.Sprintf("Replies: %d", counts.Replies)),
		m.styles.body.Render(fmt.Sprintf("Quote posts: %d", counts.QuotePosts)),
		m.styles.body.Render(fmt.Sprintf("Reposts: %d", counts.Reposts)),
		m.styles.body.Render(fmt.Sprintf("Media: %d", counts.Media)),
		m.styles.body.Render(fmt.Sprintf("Warnings: %d", m.xImportResult.Warnings.Total)),
		"",
	}
	lines = append(lines, m.menuRows(items, m.resultCursor, layoutSpec(m.width, m.height).contentWidth, hitImportResultAction)...)
	return m.singlePaneFooter("X Archive Imported", "Stored locally for restart-safe browsing", lines, m.footer(footerActionMenu))
}

func (m Model) xBrowserView() string {
	spec := layoutSpec(m.width, m.height)
	listWidth, detailWidth := twoPaneWidths(spec, "X Posts")
	visible := m.localDataListHeight()
	count := m.xDatasetLen()
	rows := make([]string, 0, minInt(count, visible))
	start := ensureOffset(m.xCursor, m.xOffset, count, visible)
	end := minInt(count, start+visible)
	for index := start; index < end; index++ {
		entry, ok := m.xDataset.Entry(index)
		if !ok {
			continue
		}
		relevant := terminalSafeInline(entry.RelevantAccount)
		if relevant != "" {
			relevant = "@" + relevant
		}
		rows = append(rows, fixedWidthRow(
			fixedColumn{Text: string(entry.Type), Width: 11},
			fixedColumn{Text: entry.OccurredAt.Format("2006-01-02"), Width: 10},
			fixedColumn{Text: relevant, Width: 16},
			fixedColumn{Text: fmt.Sprintf("media %d", entry.Media), Width: 8},
		))
	}
	listLines := []string{m.styles.muted.Render(fmt.Sprintf("%d current posts · newest first", count)), ""}
	if count == 0 {
		listLines = append(listLines, m.emptyState("No supported current posts."))
	} else {
		for index, row := range rows {
			absolute := start + index
			style := m.styles.body
			prefix := "  "
			if absolute == m.xCursor {
				style = m.styles.selected
				prefix = "> "
			}
			listLines = append(listLines, style.Render(truncateEnd(prefix+row, paneTextWidth(listWidth))))
		}
	}
	detail := []string{}
	if m.xSelectedLoaded {
		detail = append(detail,
			m.styles.body.Render("Type: "+terminalSafeInline(string(m.xSelected.Type))),
			m.styles.body.Render("Date: "+m.xSelected.OccurredAt.Format("2006-01-02 15:04 UTC")),
			m.styles.body.Render(fmt.Sprintf("Media: %d", len(m.xSelected.Media))), "",
			m.styles.body.Render(truncateEnd(terminalSafeInline(m.xSelected.Text), paneTextWidth(detailWidth))),
		)
	} else {
		detail = append(detail, m.emptyState("No post selected."))
	}
	if m.xError != "" {
		detail = append(detail, "", m.notice("error", m.xError))
	}
	body := m.twoPane(spec, "X Posts", "Read-only", listLines, "Selected Post", "Enter for full text", detail)
	return m.appShell("X Archive", body, m.footer("up/down move · enter details · esc back · ? help · ctrl+q quit"))
}

func (m Model) xDetailView() string {
	if !m.xSelectedLoaded {
		return m.singlePaneFooter("X Post", "Read-only", []string{m.emptyState("Post unavailable.")}, m.footer(footerEmpty))
	}
	tab := "Text"
	lines := []string{m.styles.muted.Render("Text  |  Media actions"), ""}
	if m.xDetailTab == 0 {
		all := m.xDetailTextLines()
		start := minInt(m.xDetailOffset, maxInt(0, len(all)-1))
		end := minInt(len(all), start+m.xDetailVisibleRows())
		for _, line := range all[start:end] {
			lines = append(lines, m.styles.body.Render(line))
		}
		if end < len(all) {
			lines = append(lines, m.styles.muted.Render("More below..."))
		}
	} else {
		tab = "Media actions"
		if len(m.xSelected.Media) == 0 {
			lines = append(lines, m.emptyState("No retained media for this post."))
		} else {
			items := make([]string, 0, len(m.xSelected.Media))
			disabled := make(map[int]bool)
			for index, media := range m.xSelected.Media {
				items = append(items, xMediaActionLabel(media))
				if m.xDataset == nil || !m.xDataset.MediaAvailable(media) {
					disabled[index] = true
				}
			}
			lines = append(lines, m.menuRowsWithDisabled(items, disabled, m.xMediaCursor, layoutSpec(m.width, m.height).contentWidth, hitNone)...)
		}
	}
	if m.xError != "" {
		lines = append(lines, "", m.notice("error", m.xError))
	} else if m.xStatus != "" {
		lines = append(lines, "", m.notice("success", m.xStatus))
	}
	return m.singlePaneFooter("X Post", tab+" · local archive only", lines, m.footer("tab switch · up/down move or scroll · enter open local media · esc back"))
}

func (m Model) xWarningsView() string {
	groups := m.xImportResult.Warnings.Groups
	visible := m.localDataListHeight()
	start := ensureOffset(m.warningCursor, m.warningOffset, len(groups), visible)
	end := minInt(len(groups), start+visible)
	lines := []string{m.styles.muted.Render(fmt.Sprintf("%d grouped structural warnings", m.xImportResult.Warnings.Total)), ""}
	if len(groups) == 0 {
		lines = append(lines, m.emptyState("No import warnings."))
	}
	for index := start; index < end; index++ {
		group := groups[index]
		prefix := "  "
		style := m.styles.body
		if index == m.warningCursor {
			prefix = "> "
			style = m.styles.selected
		}
		line := fmt.Sprintf("%s · %s · %s · %d", group.Source, group.Category, group.Reason, group.Count)
		lines = append(lines, style.Render(truncateEnd(prefix+line, layoutSpec(m.width, m.height).contentWidth-4)))
	}
	return m.singlePaneFooter("X Import Warnings", "No raw values retained", lines, m.footer(footerList))
}

func (m Model) xArchivesView() string {
	spec := layoutSpec(m.width, m.height)
	listWidth, detailWidth := twoPaneWidths(spec, "X Archives")
	visible := m.localDataListHeight()
	start := ensureOffset(m.xArchiveCursor, m.xArchiveOffset, len(m.xDatasets), visible)
	end := minInt(len(m.xDatasets), start+visible)
	list := []string{m.styles.muted.Render(fmt.Sprintf("%d locally retained datasets", len(m.xDatasets))), ""}
	if len(m.xDatasets) == 0 {
		list = append(list, m.emptyState("No X archives imported yet."))
	}
	for index := start; index < end; index++ {
		dataset := m.xDatasets[index]
		row := fixedWidthRow(fixedColumn{Text: "@" + terminalSafeInline(dataset.Account.Username), Width: 22}, fixedColumn{Text: dataset.ImportedAt.Format("2006-01-02"), Width: 10}, fixedColumn{Text: fmt.Sprintf("%d posts", dataset.Counts.Total), Width: 12})
		prefix := "  "
		style := m.styles.body
		if index == m.xArchiveCursor {
			prefix = "> "
			style = m.styles.selected
		}
		list = append(list, style.Render(truncateEnd(prefix+row, paneTextWidth(listWidth))))
	}
	detail := []string{m.emptyState("No archive selected.")}
	if len(m.xDatasets) > 0 {
		dataset := m.xDatasets[clampCursor(m.xArchiveCursor, len(m.xDatasets))]
		detail = m.detailRows([]string{
			"Account: " + xAccountIdentity(dataset.Account),
			fmt.Sprintf("Current posts: %d", dataset.Counts.Total),
			fmt.Sprintf("Media: %d", dataset.Counts.Media),
			fmt.Sprintf("Warnings: %d", dataset.WarningCount),
			fmt.Sprintf("Stored bytes: %d", dataset.StoredBytes),
		}, detailWidth)
	}
	if m.xError != "" {
		detail = append(detail, "", m.notice("error", m.xError))
	}
	body := m.twoPane(spec, "X Archives", "Newest import first", list, "Details", "Enter to browse", detail)
	return m.appShell("X Archives", body, m.footer(footerList))
}

func (m Model) xDetailVisibleRows() int { return maxInt(3, layoutSpec(m.width, m.height).bodyHeight-7) }

func (m Model) xDetailTextLines() []string {
	width := maxInt(12, layoutSpec(m.width, m.height).contentWidth-4)
	lines := []string{
		"Type: " + terminalSafeInline(string(m.xSelected.Type)),
		"Date: " + m.xSelected.OccurredAt.Format("2006-01-02 15:04 UTC"),
		m.xDetailSourceLine(),
	}
	for _, relation := range []struct {
		label string
		post  *xarchive.RelatedPost
	}{{"Reply to", m.xSelected.ReplyTo}, {"Quote", m.xSelected.Quote}, {"Repost", m.xSelected.RepostOf}} {
		if relation.post != nil {
			lines = append(lines, relation.label+": "+xRelatedIdentity(relation.post))
		}
	}
	lines = append(lines, fmt.Sprintf("Media: %d retained", len(m.xSelected.Media)), "", "Text:")
	lines = append(lines, wrapPlainText(terminalSafeText(m.xSelected.Text), width)...)
	return lines
}

func (m Model) xDetailSourceLine() string {
	source := "Current posts"
	if m.xSelected.SourceKind == "community_tweet" {
		source = "Community posts"
	}
	if m.xDataset == nil {
		return "Source: " + source
	}
	return "Source: " + xAccountIdentity(m.xDataset.Summary().Account) + " · " + source
}

func xAccountIdentity(account xarchive.AccountIdentity) string {
	username := terminalSafeInline(account.Username)
	displayName := terminalSafeInline(account.DisplayName)
	if displayName != "" && username != "" {
		return displayName + " (@" + username + ")"
	}
	if username != "" {
		return "@" + username
	}
	return displayName
}

func xRelatedIdentity(post *xarchive.RelatedPost) string {
	if post == nil {
		return "Unknown"
	}
	username := terminalSafeInline(post.Username)
	postID := terminalSafeInline(post.PostID)
	if username != "" && postID != "" {
		return "@" + username + " · post " + postID
	}
	if username != "" {
		return "@" + username
	}
	if postID != "" {
		return "post " + postID
	}
	return "Unknown"
}

func xMediaActionLabel(media xarchive.MediaRef) string {
	kind := "video"
	if media.Kind == xarchive.MediaPhoto {
		kind = "photo"
	} else if media.Kind == xarchive.MediaAnimation {
		kind = "animation"
	}
	parts := []string{"Open " + kind}
	if media.Width > 0 && media.Height > 0 {
		parts = append(parts, fmt.Sprintf("%d×%d", media.Width, media.Height))
	}
	if media.DurationMillis > 0 {
		parts = append(parts, formatMediaDuration(media.DurationMillis))
	}
	if media.Bytes > 0 {
		parts = append(parts, formatMediaBytes(media.Bytes))
	}
	return strings.Join(parts, " · ")
}

func formatMediaDuration(milliseconds int64) string {
	hours := milliseconds / 3_600_000
	minutes := (milliseconds % 3_600_000) / 60_000
	seconds := (milliseconds % 60_000) / 1_000
	tenths := (milliseconds % 1_000) / 100
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d:%02d", minutes, seconds)
	}
	if tenths > 0 {
		return fmt.Sprintf("%d.%ds", seconds, tenths)
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatMediaBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(value)/1024)
	}
	if value < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MiB", float64(value)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GiB", float64(value)/(1024*1024*1024))
}

func terminalSafeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	var safe strings.Builder
	safe.Grow(len(value))
	for _, current := range value {
		switch {
		case current == '\n':
			safe.WriteByte('\n')
		case current == '\t':
			safe.WriteByte(' ')
		case unicode.IsPrint(current):
			safe.WriteRune(current)
		default:
			safe.WriteRune('�')
		}
	}
	return safe.String()
}

func terminalSafeInline(value string) string {
	return strings.Join(strings.Fields(terminalSafeText(value)), " ")
}

func wrapPlainText(value string, width int) []string {
	paragraphs := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	lines := make([]string, 0)
	for _, paragraph := range paragraphs {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			if lipgloss.Width(word) > width {
				if line != "" {
					lines = append(lines, line)
					line = ""
				}
				chunks := splitDisplayWidth(word, width)
				if len(chunks) > 1 {
					lines = append(lines, chunks[:len(chunks)-1]...)
				}
				if len(chunks) > 0 {
					line = chunks[len(chunks)-1]
				}
				continue
			}
			candidate := word
			if line != "" {
				candidate = line + " " + word
			}
			if lipgloss.Width(candidate) <= width {
				line = candidate
				continue
			}
			if line != "" {
				lines = append(lines, line)
			}
			line = word
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitDisplayWidth(value string, width int) []string {
	chunks := []string{}
	current := ""
	for _, runeValue := range value {
		candidate := current + string(runeValue)
		if current != "" && lipgloss.Width(candidate) > width {
			chunks = append(chunks, current)
			current = string(runeValue)
		} else {
			current = candidate
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}

func singleLine(value string) string { return strings.Join(strings.Fields(value), " ") }

func keyMatchesUp(msg tea.KeyPressMsg) bool   { return msg.Code == tea.KeyUp || msg.Text == "k" }
func keyMatchesDown(msg tea.KeyPressMsg) bool { return msg.Code == tea.KeyDown || msg.Text == "j" }
func keyMatchesBack(msg tea.KeyPressMsg) bool {
	return msg.Code == tea.KeyEscape || msg.Code == tea.KeyBackspace
}
func keyMatchesSelect(msg tea.KeyPressMsg) bool { return msg.Code == tea.KeyEnter }
