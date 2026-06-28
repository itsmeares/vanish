package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/itsmeares/vanish/internal/platform"
	"github.com/itsmeares/vanish/internal/workspace"
)

type screen int

const (
	screenHome screen = iota
	screenImportPath
	screenImporting
	screenImportResult
	screenItemsBrowser
	screenFilters
	screenSelectionSummary
	screenSelectedItems
	screenPlanPreview
	screenPlanExportPath
	screenPlanLoadPath
	screenLoadedPlanSummary
	screenLoadedPlanActions
	screenWarnings
	screenLocalDataOverview
	screenRecentImports
	screenRecentPlans
	screenAuditLog
	screenWipeLocalDataConfirm
	screenKeybindings
	screenQuitConfirm
)

const (
	homeImportZip = iota
	homeLoadPlan
	homeDemo
	homeLocalData
	homeQuit
)

const (
	resultViewItems = iota
	resultViewWarnings
	resultReviewSelection
	resultBackHome
)

const (
	filterRowLike = iota
	filterRowComment
	filterRowFollowing
	filterRowFollower
	filterRowActor
	filterRowTarget
	filterRowOlder
	filterRowNewer
	filterRowApply
	filterRowClear
	filterRowCount
)

const filterEditNone = -1

var homeMenuItems = []string{
	"Import Instagram export ZIP",
	"Load cleanup plan",
	"Demo import with fake local data",
	"Local data",
	"Quit",
}

var resultMenuItems = []string{
	"View parsed items",
	"View warnings",
	"Review selection",
	"Back home",
}

const (
	selectionGeneratePlan = iota
	selectionViewSelected
	selectionSelectVisible
	selectionDeselectVisible
	selectionClear
	selectionBack
)

var selectionMenuItems = []string{
	"Generate dry-run plan",
	"View selected items",
	"Select all visible items",
	"Deselect all visible items",
	"Clear selection",
	"Back",
}

const (
	planPreviewExport = iota
	planPreviewBack
)

var planPreviewMenuItems = []string{
	"Export JSON",
	"Back",
}

const defaultPlanExportPath = "vanish-plan.json"

const (
	loadedPlanViewActions = iota
	loadedPlanBackHome
)

var loadedPlanSummaryMenuItems = []string{
	"View actions",
	"Back home",
}

const (
	localDataRecentImports = iota
	localDataRecentPlans
	localDataAuditLog
	localDataWipe
	localDataBackHome
)

var localDataMenuItems = []string{
	"Recent imports",
	"Recent plans",
	"Audit log",
	"Wipe local data",
	"Back home",
}

const (
	wipeLocalDataConfirm = iota
	wipeLocalDataCancel
)

var wipeLocalDataMenuItems = []string{
	"Wipe local data",
	"Cancel",
}

const (
	quitConfirmQuit = iota
	quitConfirmCancel
)

var quitConfirmMenuItems = []string{
	"Quit Vanish",
	"Cancel",
}

// Model is the central state for a Bubble Tea app.
//
// A struct groups related fields together. Here it stores the current screen,
// terminal dimensions, styles, and reusable Bubbles components. Bubble Tea
// passes this value through Init, Update, and View as the app runs.
type Model struct {
	current             screen
	width               int
	height              int
	styles              styles
	keys                keyMap
	help                help.Model
	localWorkspace      *workspace.Workspace
	pathInput           textinput.Model
	planPathInput       textinput.Model
	filterActorInput    textinput.Model
	filterTargetInput   textinput.Model
	filterOlderInput    textinput.Model
	filterNewerInput    textinput.Model
	spinner             spinner.Model
	importSource        string
	importResult        instagram.ImportResult
	importErr           error
	itemFilter          domain.ActivityItemFilter
	selection           domain.ActivitySelection
	planResult          instagram.PlanBuildResult
	loadedPlan          domain.CleanupPlan
	loadedPlanSummary   domain.CleanupPlanSummary
	draftFilter         domain.ActivityItemFilter
	draftOlderDate      string
	draftNewerDate      string
	filterError         string
	selectionMessage    string
	planExportStatus    string
	planExportError     string
	planLoadError       string
	recentPlanError     string
	localDataStatus     string
	localDataWarning    string
	localConfig         workspace.Config
	recentImports       []workspace.RecentImport
	recentPlans         []workspace.RecentPlan
	auditEvents         []workspace.AuditEvent
	auditMalformed      int
	homeCursor          int
	resultCursor        int
	itemCursor          int
	itemOffset          int
	filterCursor        int
	filterEditing       int
	selectionCursor     int
	selectedCursor      int
	selectedOffset      int
	planPreviewCursor   int
	planListOffset      int
	loadedPlanCursor    int
	loadedActionCursor  int
	loadedActionOffset  int
	warningCursor       int
	warningOffset       int
	localDataCursor     int
	recentImportCursor  int
	recentImportOffset  int
	recentPlanCursor    int
	recentPlanOffset    int
	auditCursor         int
	auditOffset         int
	wipeLocalDataCursor int
	helpReturnScreen    screen
	quitReturnScreen    screen
	quitCursor          int
}

// NewModel builds the initial app state before Bubble Tea starts sending
// terminal messages.
func NewModel() Model {
	return NewModelWithWorkspace(nil, nil)
}

// NewModelWithWorkspace builds app state with an optional local metadata
// workspace. Passing nil keeps the TUI usable without touching local disk.
func NewModelWithWorkspace(localWorkspace *workspace.Workspace, localErr error) Model {
	isDark := true
	helpModel := help.New()
	helpModel.Styles = help.DefaultStyles(isDark)

	pathInput := textinput.New()
	pathInput.Placeholder = `C:\path\to\instagram-export.zip`
	pathInput.Prompt = "> "
	pathInput.CharLimit = 1024
	pathInput.SetWidth(74)

	m := Model{
		current:           screenHome,
		styles:            newStyles(isDark),
		keys:              newKeyMap(),
		help:              helpModel,
		localWorkspace:    localWorkspace,
		pathInput:         pathInput,
		planPathInput:     newPlanPathInput(),
		filterActorInput:  newFilterInput("username"),
		filterTargetInput: newFilterInput("URL or ID"),
		filterOlderInput:  newFilterInput("YYYY-MM-DD"),
		filterNewerInput:  newFilterInput("YYYY-MM-DD"),
		filterEditing:     filterEditNone,
		spinner:           spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	if localErr != nil {
		m.localDataWarning = "Local data unavailable: " + localErr.Error()
	}
	return m
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
		m.planPathInput.SetWidth(inputWidth(msg.Width))
		m.setFilterInputWidths(inputWidth(msg.Width))
		m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(m.visibleItems()), m.itemListHeight())
		m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(m.selectedItems()), m.itemListHeight())
		m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
		m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
		m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
		m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
		m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())

	case importFinishedMsg:
		m.importResult = msg.result
		m.importErr = msg.err
		m.importSource = msg.source
		m.resultCursor = 0
		m.itemCursor = 0
		m.itemOffset = 0
		m.selection = domain.ActivitySelection{}
		m.selectionCursor = 0
		m.selectionMessage = ""
		m.resetPlanState()
		m.selectedCursor = 0
		m.selectedOffset = 0
		m.warningCursor = 0
		m.warningOffset = 0
		m.clearFilterState()
		m.recordImportFinished(msg)
		m.current = screenImportResult

	case spinner.TickMsg:
		if m.current == screenImporting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case exportPlanFinishedMsg:
		if msg.err != nil {
			m.planExportError = msg.err.Error()
			m.planExportStatus = ""
			return m, nil
		}
		m.planExportError = ""
		m.planExportStatus = fmt.Sprintf("Saved plan to %s", msg.path)
		m.planPathInput.SetValue(msg.path)
		m.recordPlanExported(msg.path)
		return m, nil

	case loadPlanFinishedMsg:
		if msg.err != nil {
			m.recordPlanLoadFailed(msg.path, msg.err)
			if msg.fromRecent {
				m.recentPlanError = friendlyPlanLoadError(msg.err)
				m.current = screenRecentPlans
				return m, nil
			}
			m.planLoadError = friendlyPlanLoadError(msg.err)
			return m, nil
		}
		m.loadedPlan = msg.plan
		m.loadedPlanSummary = msg.summary
		m.planLoadError = ""
		m.loadedPlanCursor = 0
		m.loadedActionCursor = 0
		m.loadedActionOffset = 0
		m.recentPlanError = ""
		m.planPathInput.SetValue(msg.path)
		m.planPathInput.Blur()
		m.recordPlanLoaded(msg.path, msg.plan, msg.summary)
		m.current = screenLoadedPlanSummary
		return m, nil

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.quit) {
			if m.current != screenQuitConfirm {
				m.openQuitConfirm()
			}
			return m, nil
		}
		if key.Matches(msg, m.keys.help) && m.current != screenKeybindings {
			m.openKeybindings()
			return m, nil
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
		case screenFilters:
			return m.updateFilters(msg)
		case screenSelectionSummary:
			return m.updateSelectionSummary(msg)
		case screenSelectedItems:
			return m.updateSelectedItems(msg)
		case screenPlanPreview:
			return m.updatePlanPreview(msg)
		case screenPlanExportPath:
			return m.updatePlanExportPath(msg)
		case screenPlanLoadPath:
			return m.updatePlanLoadPath(msg)
		case screenLoadedPlanSummary:
			return m.updateLoadedPlanSummary(msg)
		case screenLoadedPlanActions:
			return m.updateLoadedPlanActions(msg)
		case screenWarnings:
			return m.updateWarnings(msg)
		case screenLocalDataOverview:
			return m.updateLocalDataOverview(msg)
		case screenRecentImports:
			return m.updateRecentImports(msg)
		case screenRecentPlans:
			return m.updateRecentPlans(msg)
		case screenAuditLog:
			return m.updateAuditLog(msg)
		case screenWipeLocalDataConfirm:
			return m.updateWipeLocalDataConfirm(msg)
		case screenKeybindings:
			return m.updateKeybindings(msg)
		case screenQuitConfirm:
			return m.updateQuitConfirm(msg)
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
		case homeLoadPlan:
			m.resetLoadedPlanState()
			m.planPathInput.SetValue(defaultPlanExportPath)
			m.current = screenPlanLoadPath
			return m, m.planPathInput.Focus()
		case homeDemo:
			m = m.resetImportState()
			m.current = screenImporting
			m.importSource = "demo instagram export"
			return m, tea.Batch(startSpinnerCmd(m.spinner), demoImportCmd())
		case homeLocalData:
			m.openLocalDataOverview()
		case homeQuit:
			m.openQuitConfirm()
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
	case key.Matches(msg, m.keys.cancel):
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
			items := m.visibleItems()
			m.itemCursor = clampCursor(m.itemCursor, len(items))
			m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.itemListHeight())
			m.current = screenItemsBrowser
		case resultViewWarnings:
			m.warningCursor = clampCursor(m.warningCursor, len(m.importResult.Warnings))
			m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
			m.current = screenWarnings
		case resultReviewSelection:
			m.selectionCursor = 0
			m.current = screenSelectionSummary
		case resultBackHome:
			m.current = screenHome
		}
	}

	return m, nil
}

func (m Model) updateItemsBrowser(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.visibleItems()
	switch {
	case key.Matches(msg, m.keys.up):
		m.itemCursor = moveCursor(m.itemCursor, len(items), -1)
	case key.Matches(msg, m.keys.down):
		m.itemCursor = moveCursor(m.itemCursor, len(items), 1)
	case key.Matches(msg, m.keys.filter):
		m.beginFilterDraft()
		m.current = screenFilters
	case key.Matches(msg, m.keys.selectItem), key.Matches(msg, m.keys.toggleSelection):
		if len(items) > 0 {
			m.selection.Toggle(items[clampCursor(m.itemCursor, len(items))].ID)
		}
	case key.Matches(msg, m.keys.selectVisible):
		m.selection.SelectItems(items)
	case key.Matches(msg, m.keys.deselectVisible):
		m.selection.DeselectItems(items)
	case key.Matches(msg, m.keys.selectionSummary):
		m.selectionCursor = 0
		m.current = screenSelectionSummary
	case key.Matches(msg, m.keys.back):
		m.current = screenImportResult
	}
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.itemListHeight())
	return m, nil
}

func (m Model) updateFilters(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.filterEditing != filterEditNone {
		switch {
		case key.Matches(msg, m.keys.selectItem):
			m.acceptFilterInput()
			return m, nil
		case key.Matches(msg, m.keys.cancel):
			m.cancelFilterInput()
			return m, nil
		default:
			var cmd tea.Cmd
			m.updateFocusedFilterInput(msg, &cmd)
			return m, cmd
		}
	}

	switch {
	case key.Matches(msg, m.keys.up):
		m.filterCursor = moveCursor(m.filterCursor, filterRowCount, -1)
	case key.Matches(msg, m.keys.down):
		m.filterCursor = moveCursor(m.filterCursor, filterRowCount, 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenItemsBrowser
	case key.Matches(msg, m.keys.selectItem):
		switch m.filterCursor {
		case filterRowLike:
			m.toggleDraftType(domain.ActivityFilterLike)
		case filterRowComment:
			m.toggleDraftType(domain.ActivityFilterComment)
		case filterRowFollowing:
			m.toggleDraftType(domain.ActivityFilterFollowing)
		case filterRowFollower:
			m.toggleDraftType(domain.ActivityFilterFollower)
		case filterRowActor, filterRowTarget, filterRowOlder, filterRowNewer:
			return m.startFilterInput(m.filterCursor)
		case filterRowApply:
			m.applyDraftFilter()
		case filterRowClear:
			m.clearFilterState()
		}
	}

	return m, nil
}

func (m Model) updateSelectionSummary(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.selectionMessage = ""
	switch {
	case key.Matches(msg, m.keys.up):
		m.selectionCursor = moveCursor(m.selectionCursor, len(selectionMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.selectionCursor = moveCursor(m.selectionCursor, len(selectionMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenItemsBrowser
	case key.Matches(msg, m.keys.selectItem):
		switch m.selectionCursor {
		case selectionGeneratePlan:
			selected := m.selectedItems()
			if len(selected) == 0 {
				m.selectionMessage = "Select at least one item before generating a plan."
				return m, nil
			}
			result, err := instagram.BuildCleanupPlan(platform.BuildPlanRequest{
				Platform:   domain.PlatformInstagram,
				SourceName: emptyFallback(m.importSource, "instagram export"),
				CreatedAt:  time.Now().UTC(),
				Items:      selected,
			})
			if err != nil {
				m.selectionMessage = err.Error()
				return m, nil
			}
			m.planResult = result
			m.recordPlanGenerated(result)
			m.planPreviewCursor = 0
			m.planListOffset = 0
			m.planExportStatus = ""
			m.planExportError = ""
			m.planPathInput.SetValue(defaultPlanExportPath)
			m.current = screenPlanPreview
		case selectionViewSelected:
			items := m.selectedItems()
			m.selectedCursor = clampCursor(m.selectedCursor, len(items))
			m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(items), m.itemListHeight())
			m.current = screenSelectedItems
		case selectionSelectVisible:
			m.selection.SelectItems(m.visibleItems())
			m.selectionMessage = "Selected all visible items."
		case selectionDeselectVisible:
			m.selection.DeselectItems(m.visibleItems())
			m.selectionMessage = "Deselected all visible items."
		case selectionClear:
			m.selection.Clear()
			m.selectedCursor = 0
			m.selectedOffset = 0
			m.selectionMessage = "Selection cleared."
		case selectionBack:
			m.current = screenItemsBrowser
		}
	}

	return m, nil
}

func (m Model) updatePlanPreview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.planPreviewCursor = moveCursor(m.planPreviewCursor, len(planPreviewMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.planPreviewCursor = moveCursor(m.planPreviewCursor, len(planPreviewMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenSelectionSummary
	case key.Matches(msg, m.keys.selectItem):
		switch m.planPreviewCursor {
		case planPreviewExport:
			m.current = screenPlanExportPath
			if strings.TrimSpace(m.planPathInput.Value()) == "" {
				m.planPathInput.SetValue(defaultPlanExportPath)
			}
			m.planExportStatus = ""
			m.planExportError = ""
			return m, m.planPathInput.Focus()
		case planPreviewBack:
			m.current = screenSelectionSummary
		}
	}
	return m, nil
}

func (m Model) updatePlanExportPath(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.save):
		outputPath := strings.TrimSpace(m.planPathInput.Value())
		if outputPath == "" {
			outputPath = defaultPlanExportPath
			m.planPathInput.SetValue(outputPath)
		}
		m.planExportStatus = ""
		m.planExportError = ""
		return m, writePlanJSONCmd(outputPath, m.planResult.Plan)
	case key.Matches(msg, m.keys.cancel):
		m.planPathInput.Blur()
		m.current = screenPlanPreview
		return m, nil
	default:
		var cmd tea.Cmd
		m.planPathInput, cmd = m.planPathInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updatePlanLoadPath(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.start):
		planPath := strings.TrimSpace(m.planPathInput.Value())
		if planPath == "" {
			planPath = defaultPlanExportPath
			m.planPathInput.SetValue(planPath)
		}
		m.planLoadError = ""
		return m, loadPlanJSONCmd(planPath, false)
	case key.Matches(msg, m.keys.cancel):
		m.planPathInput.Blur()
		m.current = screenHome
		return m, nil
	default:
		var cmd tea.Cmd
		m.planPathInput, cmd = m.planPathInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateLoadedPlanSummary(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.loadedPlanCursor = moveCursor(m.loadedPlanCursor, len(loadedPlanSummaryMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.loadedPlanCursor = moveCursor(m.loadedPlanCursor, len(loadedPlanSummaryMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenHome
	case key.Matches(msg, m.keys.selectItem):
		switch m.loadedPlanCursor {
		case loadedPlanViewActions:
			m.loadedActionCursor = clampCursor(m.loadedActionCursor, len(m.loadedPlan.Actions))
			m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
			m.current = screenLoadedPlanActions
		case loadedPlanBackHome:
			m.current = screenHome
		}
	}
	return m, nil
}

func (m Model) updateLoadedPlanActions(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	actions := m.loadedPlan.Actions
	switch {
	case key.Matches(msg, m.keys.up):
		m.loadedActionCursor = moveCursor(m.loadedActionCursor, len(actions), -1)
	case key.Matches(msg, m.keys.down):
		m.loadedActionCursor = moveCursor(m.loadedActionCursor, len(actions), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenLoadedPlanSummary
	}
	m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(actions), m.planActionListHeight())
	return m, nil
}

func (m Model) updateSelectedItems(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.selectedItems()
	switch {
	case key.Matches(msg, m.keys.up):
		m.selectedCursor = moveCursor(m.selectedCursor, len(items), -1)
	case key.Matches(msg, m.keys.down):
		m.selectedCursor = moveCursor(m.selectedCursor, len(items), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenSelectionSummary
	}
	m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(items), m.itemListHeight())
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

func (m Model) updateLocalDataOverview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.localDataCursor = moveCursor(m.localDataCursor, len(localDataMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.localDataCursor = moveCursor(m.localDataCursor, len(localDataMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenHome
	case key.Matches(msg, m.keys.selectItem):
		switch m.localDataCursor {
		case localDataRecentImports:
			m.refreshLocalData()
			m.recentImportCursor = clampCursor(m.recentImportCursor, len(m.recentImports))
			m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
			m.current = screenRecentImports
		case localDataRecentPlans:
			m.refreshLocalData()
			m.recentPlanCursor = clampCursor(m.recentPlanCursor, len(m.recentPlans))
			m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
			m.current = screenRecentPlans
		case localDataAuditLog:
			m.refreshLocalData()
			m.auditCursor = clampCursor(m.auditCursor, len(m.auditEvents))
			m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
			m.current = screenAuditLog
		case localDataWipe:
			m.wipeLocalDataCursor = wipeLocalDataCancel
			m.current = screenWipeLocalDataConfirm
		case localDataBackHome:
			m.current = screenHome
		}
	}
	return m, nil
}

func (m Model) updateRecentImports(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.recentImportCursor = moveCursor(m.recentImportCursor, len(m.recentImports), -1)
	case key.Matches(msg, m.keys.down):
		m.recentImportCursor = moveCursor(m.recentImportCursor, len(m.recentImports), 1)
	case key.Matches(msg, m.keys.back):
		m.openLocalDataOverview()
	}
	m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
	return m, nil
}

func (m Model) updateRecentPlans(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.recentPlanError = ""
		m.recentPlanCursor = moveCursor(m.recentPlanCursor, len(m.recentPlans), -1)
	case key.Matches(msg, m.keys.down):
		m.recentPlanError = ""
		m.recentPlanCursor = moveCursor(m.recentPlanCursor, len(m.recentPlans), 1)
	case key.Matches(msg, m.keys.back):
		m.recentPlanError = ""
		m.openLocalDataOverview()
	case key.Matches(msg, m.keys.selectItem):
		if len(m.recentPlans) == 0 {
			return m, nil
		}
		plan := m.recentPlans[clampCursor(m.recentPlanCursor, len(m.recentPlans))]
		planPath := strings.TrimSpace(plan.Path)
		if planPath == "" {
			m.recentPlanError = "Recent plan does not have a local path."
			return m, nil
		}
		m.recentPlanError = ""
		return m, loadPlanJSONCmd(planPath, true)
	}
	m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
	return m, nil
}

func (m Model) updateAuditLog(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.auditCursor = moveCursor(m.auditCursor, len(m.auditEvents), -1)
	case key.Matches(msg, m.keys.down):
		m.auditCursor = moveCursor(m.auditCursor, len(m.auditEvents), 1)
	case key.Matches(msg, m.keys.back):
		m.openLocalDataOverview()
	}
	m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
	return m, nil
}

func (m Model) updateWipeLocalDataConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.wipeLocalDataCursor = moveCursor(m.wipeLocalDataCursor, len(wipeLocalDataMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.wipeLocalDataCursor = moveCursor(m.wipeLocalDataCursor, len(wipeLocalDataMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.openLocalDataOverview()
	case key.Matches(msg, m.keys.selectItem):
		switch m.wipeLocalDataCursor {
		case wipeLocalDataConfirm:
			m.wipeLocalData()
			m.openLocalDataOverview()
		case wipeLocalDataCancel:
			m.openLocalDataOverview()
		}
	}
	return m, nil
}

func (m Model) updateKeybindings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.back) {
		m.current = m.helpReturnScreen
	}
	return m, nil
}

func (m Model) updateQuitConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.quitCursor = moveCursor(m.quitCursor, len(quitConfirmMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.quitCursor = moveCursor(m.quitCursor, len(quitConfirmMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = m.quitReturnScreen
	case key.Matches(msg, m.keys.selectItem):
		switch m.quitCursor {
		case quitConfirmQuit:
			return m, tea.Quit
		case quitConfirmCancel:
			m.current = m.quitReturnScreen
		}
	}

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
	case screenFilters:
		return tea.NewView(m.filtersView())
	case screenSelectionSummary:
		return tea.NewView(m.selectionSummaryView())
	case screenSelectedItems:
		return tea.NewView(m.selectedItemsView())
	case screenPlanPreview:
		return tea.NewView(m.planPreviewView())
	case screenPlanExportPath:
		return tea.NewView(m.planExportPathView())
	case screenPlanLoadPath:
		return tea.NewView(m.planLoadPathView())
	case screenLoadedPlanSummary:
		return tea.NewView(m.loadedPlanSummaryView())
	case screenLoadedPlanActions:
		return tea.NewView(m.loadedPlanActionsView())
	case screenWarnings:
		return tea.NewView(m.warningsView())
	case screenLocalDataOverview:
		return tea.NewView(m.localDataOverviewView())
	case screenRecentImports:
		return tea.NewView(m.recentImportsView())
	case screenRecentPlans:
		return tea.NewView(m.recentPlansView())
	case screenAuditLog:
		return tea.NewView(m.auditLogView())
	case screenWipeLocalDataConfirm:
		return tea.NewView(m.wipeLocalDataConfirmView())
	case screenKeybindings:
		return tea.NewView(m.keybindingsView())
	case screenQuitConfirm:
		return tea.NewView(m.quitConfirmView())
	default:
		return tea.NewView(m.homeView())
	}
}

func (m Model) homeView() string {
	lines := m.header("Home")
	lines = append(lines,
		"",
		m.styles.body.Render("Local-first review and cleanup planning for your social media footprint."),
		m.styles.muted.Render("No cloud backend. No telemetry by default. No hidden background actions."),
		"",
		m.styles.body.Render("Import a local Instagram export ZIP or inspect an exported cleanup plan."),
		"",
	)
	lines = append(lines, m.localDataMessages()...)
	lines = append(lines, m.renderMenu(homeMenuItems, m.homeCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.help, m.keys.quit))

	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) importPathView() string {
	lines := m.header("Import Instagram Export")
	lines = append(lines,
		"",
		m.styles.body.Render("Type the path to a local Instagram data export .zip file."),
		m.styles.muted.Render("Vanish will only read local JSON files from the ZIP."),
		"",
		m.pathInput.View(),
		"",
		m.helpLine(m.keys.start, m.keys.cancel, m.keys.help, m.keys.quit),
	)

	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) importingView() string {
	source := m.importSource
	if source == "" {
		source = "instagram export"
	}

	lines := m.header("Importing")
	lines = append(lines,
		"",
		m.styles.body.Render(fmt.Sprintf("%s Parsing local ZIP...", m.spinner.View())),
		m.styles.muted.Render(source),
		"",
		m.helpLine(m.keys.help, m.keys.quit),
	)

	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) importResultView() string {
	if m.importErr != nil {
		lines := m.header("Import Failed")
		lines = append(lines,
			"",
			m.styles.error.Render(m.importErr.Error()),
			m.styles.muted.Render("Check that the path points to a local Instagram export .zip, then try again."),
			m.styles.muted.Render(m.importSource),
			"",
		)
		lines = append(lines, m.localDataMessages()...)
		lines = append(lines, m.helpLine(m.keys.back, m.keys.help, m.keys.quit))

		return m.frame(strings.Join(lines, "\n"))
	}

	summary := m.importResult.Summary
	lines := m.header("Import Complete")
	lines = append(lines,
		"",
		m.styles.body.Render(fmt.Sprintf("Source: %s", emptyFallback(m.importSource, "instagram export"))),
		"",
		m.styles.body.Render(fmt.Sprintf("Parsed: %d total | Likes: %d | Comments: %d | Following: %d | Followers: %d", summary.Total, summary.Likes, summary.Comments, summary.Following, summary.Followers)),
		m.styles.body.Render(fmt.Sprintf("Skipped or unknown: %d | Warnings: %d", summary.Skipped, len(m.importResult.Warnings))),
		"",
	)
	lines = append(lines, m.localDataMessages()...)

	lines = append(lines, m.renderMenu(resultMenuItems, m.resultCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) itemsBrowserView() string {
	items := m.visibleItems()
	total := len(m.importResult.Items)
	visibleRows := m.itemListHeight()
	cursor := clampCursor(m.itemCursor, len(items))
	offset := ensureOffset(cursor, m.itemOffset, len(items), visibleRows)

	filterStatus := "off"
	if m.itemFilter.Active() {
		filterStatus = "active"
	}
	lines := m.header("Parsed Items")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("Visible: %d / %d | Selected: %d | Filters: %s", len(items), total, m.selection.Len(), filterStatus)),
		m.styles.muted.Render(fmt.Sprintf("Source: %s", emptyFallback(m.importSource, "instagram export"))),
		"",
	)
	if m.itemFilter.Active() {
		lines = append(lines, m.styles.warning.Render("Filters active"), "")
	}

	if len(items) == 0 {
		lines = append(lines, m.styles.muted.Render("No parsed items."))
	} else {
		end := minInt(len(items), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(m.selectableItemRow(items[i]), i == cursor))
		}
		if end < len(items) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}

	lines = append(lines, "", m.styles.separator.Render("Details"))
	if len(items) == 0 {
		lines = append(lines, m.styles.muted.Render("No items match the current filters. Clear filters or import another ZIP."))
	} else {
		for _, line := range itemDetailLines(items[cursor]) {
			lines = append(lines, m.styles.body.Render(line))
		}
	}

	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.toggleSelection, m.keys.selectionSummary, m.keys.filter, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) selectionSummaryView() string {
	counts := m.selection.Counts(m.importResult.Items)
	visibleCount := len(m.visibleItems())
	lines := m.header("Selection Summary")
	lines = append(lines,
		"",
		m.styles.body.Render(fmt.Sprintf("Total selected: %d", counts.Total)),
		m.styles.body.Render(fmt.Sprintf("Visible items: %d", visibleCount)),
		m.styles.body.Render(fmt.Sprintf("Selected likes: %d", counts.Likes)),
		m.styles.body.Render(fmt.Sprintf("Selected comments: %d", counts.Comments)),
		m.styles.body.Render(fmt.Sprintf("Selected following: %d", counts.Following)),
		m.styles.body.Render(fmt.Sprintf("Selected followers: %d", counts.Followers)),
		"",
	)

	if strings.TrimSpace(m.selectionMessage) != "" {
		lines = append(lines, m.styles.warning.Render(m.selectionMessage), "")
	}
	lines = append(lines, m.renderMenu(selectionMenuItems, m.selectionCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) selectedItemsView() string {
	items := m.selectedItems()
	total := len(m.importResult.Items)
	visibleRows := m.itemListHeight()
	cursor := clampCursor(m.selectedCursor, len(items))
	offset := ensureOffset(cursor, m.selectedOffset, len(items), visibleRows)

	lines := m.header("Selected Items")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("Selected: %d / Total: %d", len(items), total)),
		"",
	)

	if len(items) == 0 {
		lines = append(lines, m.styles.muted.Render("No selected items yet. Toggle items in the parsed item list or select visible items from the summary."))
	} else {
		end := minInt(len(items), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(m.selectableItemRow(items[i]), i == cursor))
		}
		if end < len(items) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}

	lines = append(lines, "", m.styles.separator.Render("Details"))
	if len(items) == 0 {
		lines = append(lines, m.styles.muted.Render("No item selected."))
	} else {
		for _, line := range itemDetailLines(items[cursor]) {
			lines = append(lines, m.styles.body.Render(line))
		}
	}

	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) planPreviewView() string {
	result := m.planResult
	rows := planPreviewRows(result.Plan.Actions, result.Skipped)
	visibleRows := m.planListHeight()
	offset := ensureOffset(0, m.planListOffset, len(rows), visibleRows)

	lines := m.header("Dry-Run Plan Preview")
	lines = append(lines,
		"",
		m.styles.body.Render(fmt.Sprintf("Plan mode: %s", result.Plan.Mode)),
		m.styles.body.Render(fmt.Sprintf("Source platform: %s", result.Plan.Platform)),
		m.styles.body.Render(fmt.Sprintf("Selected items: %d", result.SelectedCount)),
		m.styles.body.Render(fmt.Sprintf("Supported actions: %d", len(result.Plan.Actions))),
		m.styles.body.Render(fmt.Sprintf("Unsupported/skipped selected items: %d", len(result.Skipped))),
		m.styles.body.Render(fmt.Sprintf("Action counts: unlike %d, delete_comment %d, unfollow %d", result.Counts.Unlike, result.Counts.DeleteComment, result.Counts.Unfollow)),
		"",
	)

	lines = append(lines, m.renderMenu(planPreviewMenuItems, m.planPreviewCursor)...)
	lines = append(lines, "", m.styles.separator.Render("Planned actions"))
	if len(rows) == 0 {
		lines = append(lines, m.styles.muted.Render("No supported actions."))
	} else {
		end := minInt(len(rows), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.styles.body.Render(rows[i]))
		}
		if end < len(rows) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}

	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) planExportPathView() string {
	lines := m.header("Export Plan JSON")
	lines = append(lines,
		"",
		m.styles.body.Render("Output path"),
		m.planPathInput.View(),
		"",
	)

	if strings.TrimSpace(m.planExportStatus) != "" {
		lines = append(lines, m.styles.success.Render(m.planExportStatus), "")
	}
	if strings.TrimSpace(m.planExportError) != "" {
		lines = append(lines, m.styles.error.Render(m.planExportError), "")
	}
	lines = append(lines, m.localDataMessages()...)

	lines = append(lines, m.helpLine(m.keys.save, m.keys.cancel, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) planLoadPathView() string {
	lines := m.header("Load Cleanup Plan")
	lines = append(lines,
		"",
		m.styles.body.Render("Type the path to a local cleanup plan JSON file."),
		m.styles.muted.Render("Vanish will only read and validate the local file."),
		"",
		m.planPathInput.View(),
		"",
	)

	if strings.TrimSpace(m.planLoadError) != "" {
		lines = append(lines, m.styles.error.Render(m.planLoadError), "")
	}

	lines = append(lines, m.helpLine(m.keys.start, m.keys.cancel, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) loadedPlanSummaryView() string {
	plan := m.loadedPlan
	summary := m.loadedPlanSummary

	lines := m.header("Loaded Cleanup Plan")
	lines = append(lines,
		"",
		m.styles.body.Render(fmt.Sprintf("Plan ID: %s", emptyFallback(plan.ID, "-"))),
		m.styles.body.Render(fmt.Sprintf("Format version: %d", plan.FormatVersion)),
		m.styles.body.Render(fmt.Sprintf("Platform: %s", emptyFallback(string(plan.Platform), "-"))),
		m.styles.body.Render(fmt.Sprintf("Source name: %s", emptyFallback(plan.SourceName, "-"))),
		m.styles.body.Render(fmt.Sprintf("Mode: %s", emptyFallback(string(plan.Mode), "-"))),
		m.styles.body.Render(fmt.Sprintf("Created at: %s", formatPlanTime(plan.CreatedAt))),
		m.styles.body.Render(fmt.Sprintf("Total actions: %d", summary.TotalActions)),
		"",
		m.styles.separator.Render("Action counts by type"),
	)
	lines = append(lines, m.actionCountLines(summary.ActionCounts)...)
	lines = append(lines,
		"",
		m.styles.separator.Render("Status counts"),
		m.styles.body.Render(fmt.Sprintf("pending: %d", summary.StatusCounts[domain.ActionStatusPending])),
		m.styles.body.Render(fmt.Sprintf("running: %d", summary.StatusCounts[domain.ActionStatusRunning])),
		m.styles.body.Render(fmt.Sprintf("done: %d", summary.StatusCounts[domain.ActionStatusDone])),
		m.styles.body.Render(fmt.Sprintf("failed: %d", summary.StatusCounts[domain.ActionStatusFailed])),
		m.styles.body.Render(fmt.Sprintf("skipped: %d", summary.StatusCounts[domain.ActionStatusSkipped])),
		"",
	)
	lines = append(lines, m.renderMenu(loadedPlanSummaryMenuItems, m.loadedPlanCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) loadedPlanActionsView() string {
	actions := m.loadedPlan.Actions
	visibleRows := m.planActionListHeight()
	cursor := clampCursor(m.loadedActionCursor, len(actions))
	offset := ensureOffset(cursor, m.loadedActionOffset, len(actions), visibleRows)

	lines := m.header("Plan Actions")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("Actions: %d | Plan: %s", len(actions), emptyFallback(m.loadedPlan.ID, "-"))),
		"",
	)

	if len(actions) == 0 {
		lines = append(lines, m.styles.muted.Render("No actions in this plan."))
	} else {
		end := minInt(len(actions), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(planActionRow(actions[i]), i == cursor))
		}
		if end < len(actions) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}

	lines = append(lines, "", m.styles.separator.Render("Details"))
	if len(actions) == 0 {
		lines = append(lines, m.styles.muted.Render("No action selected."))
	} else {
		for _, line := range planActionDetailLines(actions[cursor]) {
			lines = append(lines, m.styles.body.Render(line))
		}
	}

	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) filtersView() string {
	lines := m.header("Filters")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("Visible: %d / %d | Filters: %s", len(m.visibleItems()), len(m.importResult.Items), activeLabel(m.itemFilter.Active()))),
		"",
	)

	if m.itemFilter.Active() {
		lines = append(lines, m.styles.warning.Render("Filters active"), "")
	}
	if strings.TrimSpace(m.filterError) != "" {
		lines = append(lines, m.styles.error.Render(m.filterError), "")
	}

	rows := []string{
		filterTypeRow("Like", m.draftFilter.IncludeTypes[domain.ActivityFilterLike]),
		filterTypeRow("Comment", m.draftFilter.IncludeTypes[domain.ActivityFilterComment]),
		filterTypeRow("Following", m.draftFilter.IncludeTypes[domain.ActivityFilterFollowing]),
		filterTypeRow("Follower", m.draftFilter.IncludeTypes[domain.ActivityFilterFollower]),
		m.filterInputRow("Actor contains", filterRowActor, m.filterActorInput, m.draftFilter.ActorContains),
		m.filterInputRow("Target contains", filterRowTarget, m.filterTargetInput, m.draftFilter.TargetContains),
		m.filterInputRow("Older than", filterRowOlder, m.filterOlderInput, m.draftOlderDate),
		m.filterInputRow("Newer than", filterRowNewer, m.filterNewerInput, m.draftNewerDate),
		"Apply filters",
		"Clear all filters",
	}

	for i, row := range rows {
		lines = append(lines, m.renderSelectableLine(row, i == m.filterCursor))
	}

	if m.filterEditing == filterEditNone {
		lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	} else {
		lines = append(lines, "", m.helpLine(m.keys.save, m.keys.cancel, m.keys.help, m.keys.quit))
	}
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) warningsView() string {
	warnings := m.importResult.Warnings
	visibleRows := m.warningListHeight()
	cursor := clampCursor(m.warningCursor, len(warnings))
	offset := ensureOffset(cursor, m.warningOffset, len(warnings), visibleRows)

	lines := m.header("Import Warnings")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("%d warnings from %s", len(warnings), emptyFallback(m.importSource, "instagram export"))),
		"",
	)

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

	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) localDataOverviewView() string {
	lines := m.header("Local Data")
	lines = append(lines,
		"",
		m.styles.body.Render("Vanish stores local metadata only in its app directory."),
		m.styles.muted.Render("Imports and cleanup plans stay at the local paths you choose."),
		"",
		m.styles.body.Render(fmt.Sprintf("App directory: %s", m.localDataDirLabel())),
		m.styles.body.Render(fmt.Sprintf("Telemetry: %s", enabledLabel(m.localConfig.Telemetry.Enabled))),
		m.styles.body.Render(fmt.Sprintf("Recent imports: %d", len(m.recentImports))),
		m.styles.body.Render(fmt.Sprintf("Recent plans: %d", len(m.recentPlans))),
		m.styles.body.Render(fmt.Sprintf("Audit events: %d", len(m.auditEvents))),
	)
	if m.auditMalformed > 0 {
		lines = append(lines, m.styles.warning.Render(fmt.Sprintf("Skipped malformed audit lines: %d", m.auditMalformed)))
	}
	lines = append(lines, "")
	lines = append(lines, m.localDataMessages()...)
	lines = append(lines, m.renderMenu(localDataMenuItems, m.localDataCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) recentImportsView() string {
	visibleRows := m.localDataListHeight()
	cursor := clampCursor(m.recentImportCursor, len(m.recentImports))
	offset := ensureOffset(cursor, m.recentImportOffset, len(m.recentImports), visibleRows)
	lines := m.header("Recent Imports")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("%d recent imports from %s", len(m.recentImports), m.localDataDirLabel())),
		"",
	)
	lines = append(lines, m.localDataMessages()...)
	if len(m.recentImports) == 0 {
		lines = append(lines, m.styles.muted.Render("No recent imports yet. Import demo data or a local Instagram ZIP to add one."))
	} else {
		end := minInt(len(m.recentImports), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(recentImportRow(m.recentImports[i]), i == cursor))
		}
		if end < len(m.recentImports) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}
	lines = append(lines, "", m.styles.separator.Render("Details"))
	if len(m.recentImports) == 0 {
		lines = append(lines, m.styles.muted.Render("No import selected."))
	} else {
		for _, line := range recentImportDetailLines(m.recentImports[cursor]) {
			lines = append(lines, m.styles.body.Render(line))
		}
	}
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) recentPlansView() string {
	visibleRows := m.localDataListHeight()
	cursor := clampCursor(m.recentPlanCursor, len(m.recentPlans))
	offset := ensureOffset(cursor, m.recentPlanOffset, len(m.recentPlans), visibleRows)
	lines := m.header("Recent Plans")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("%d recent plans from %s", len(m.recentPlans), m.localDataDirLabel())),
		"",
	)
	lines = append(lines, m.localDataMessages()...)
	if strings.TrimSpace(m.recentPlanError) != "" {
		lines = append(lines, m.styles.error.Render(m.recentPlanError), "")
	}
	if len(m.recentPlans) == 0 {
		lines = append(lines, m.styles.muted.Render("No recent plans yet. Export or load a dry-run cleanup plan to add one."))
	} else {
		end := minInt(len(m.recentPlans), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(recentPlanRow(m.recentPlans[i]), i == cursor))
		}
		if end < len(m.recentPlans) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}
	lines = append(lines, "", m.styles.separator.Render("Details"))
	if len(m.recentPlans) == 0 {
		lines = append(lines, m.styles.muted.Render("No plan selected."))
	} else {
		for _, line := range recentPlanDetailLines(m.recentPlans[cursor]) {
			lines = append(lines, m.styles.body.Render(line))
		}
	}
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) auditLogView() string {
	visibleRows := m.localDataListHeight()
	cursor := clampCursor(m.auditCursor, len(m.auditEvents))
	offset := ensureOffset(cursor, m.auditOffset, len(m.auditEvents), visibleRows)
	lines := m.header("Audit Log")
	lines = append(lines,
		"",
		m.styles.muted.Render(fmt.Sprintf("%d audit events from %s", len(m.auditEvents), m.localDataDirLabel())),
		"",
	)
	lines = append(lines, m.localDataMessages()...)
	if m.auditMalformed > 0 {
		lines = append(lines, m.styles.warning.Render(fmt.Sprintf("Skipped malformed audit lines: %d", m.auditMalformed)), "")
	}
	if len(m.auditEvents) == 0 {
		lines = append(lines, m.styles.muted.Render("No audit events yet."))
	} else {
		end := minInt(len(m.auditEvents), offset+visibleRows)
		if offset > 0 {
			lines = append(lines, m.styles.muted.Render("..."))
		}
		for i := offset; i < end; i++ {
			lines = append(lines, m.renderSelectableLine(auditEventRow(m.auditEvents[i]), i == cursor))
		}
		if end < len(m.auditEvents) {
			lines = append(lines, m.styles.muted.Render("..."))
		}
	}
	lines = append(lines, "", m.styles.separator.Render("Details"))
	if len(m.auditEvents) == 0 {
		lines = append(lines, m.styles.muted.Render("No audit event selected."))
	} else {
		for _, line := range auditEventDetailLines(m.auditEvents[cursor]) {
			lines = append(lines, m.styles.body.Render(line))
		}
	}
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) wipeLocalDataConfirmView() string {
	lines := m.header("Wipe Local Data?")
	lines = append(lines,
		"",
		m.styles.warning.Render("This clears Vanish-managed config, recent history, and audit records."),
		m.styles.body.Render("It does not delete Instagram export ZIPs or cleanup plan JSON files outside the app directory."),
		m.styles.body.Render(fmt.Sprintf("App directory: %s", m.localDataDirLabel())),
		"",
	)
	lines = append(lines, m.localDataMessages()...)
	lines = append(lines, m.renderMenu(wipeLocalDataMenuItems, m.wipeLocalDataCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) keybindingsView() string {
	lines := m.header("Help")
	lines = append(lines,
		"",
		m.styles.separator.Render("Navigation"),
		m.styles.body.Render("Up/Down or j/k: move"),
		m.styles.body.Render("Enter: primary action; toggles highlighted parsed item"),
		m.styles.body.Render("Space: toggle highlighted parsed item"),
		m.styles.body.Render("Esc: back"),
		m.styles.body.Render("Backspace: back when no text input is focused"),
		m.styles.body.Render("?: show this help"),
		m.styles.body.Render("Ctrl+Q or Ctrl+C: quit confirmation"),
		"",
		m.styles.separator.Render("Selection and plans"),
		m.styles.body.Render("Review selection: generate a dry-run plan, view selected items, select or deselect visible items."),
		m.styles.body.Render("Plan export and load only read/write local JSON files."),
		"",
		m.styles.separator.Render("Safety"),
		m.styles.body.Render("Vanish is local-only and dry-run only in this alpha."),
		m.styles.body.Render("No login, browser automation, deletion, telemetry, or network requests."),
		"",
		m.helpLine(m.keys.back, m.keys.quit),
	)
	return m.frame(strings.Join(lines, "\n"))
}

func (m Model) quitConfirmView() string {
	lines := m.header("Quit Vanish?")
	lines = append(lines,
		"",
		m.styles.body.Render("Your current in-memory review state will be discarded."),
		"",
	)

	lines = append(lines, m.renderMenu(quitConfirmMenuItems, m.quitCursor)...)
	lines = append(lines, "", m.helpLine(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help))
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

func (m Model) header(section string) []string {
	return []string{
		m.styles.title.Render("Vanish") + m.styles.muted.Render(" / "+section),
		m.statusLine(),
		m.separatorLine(),
	}
}

func (m Model) statusLine() string {
	return strings.Join([]string{
		m.styles.badge.Render("[LOCAL]"),
		m.styles.badge.Render("[DRY-RUN]"),
		m.styles.badge.Render("[NO NETWORK]"),
	}, " ")
}

func (m Model) separatorLine() string {
	width := 74
	if m.width > 8 && m.width-4 < width {
		width = m.width - 4
	}
	if width < 20 {
		width = 20
	}
	return m.styles.separator.Render(strings.Repeat("-", width))
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
	m.selection = domain.ActivitySelection{}
	m.selectionCursor = 0
	m.selectionMessage = ""
	m.resetPlanState()
	m.selectedCursor = 0
	m.selectedOffset = 0
	m.warningCursor = 0
	m.warningOffset = 0
	m.clearFilterState()
	return m
}

func (m *Model) resetPlanState() {
	m.planResult = instagram.PlanBuildResult{}
	m.planPreviewCursor = 0
	m.planListOffset = 0
	m.planPathInput.SetValue(defaultPlanExportPath)
	m.planPathInput.Blur()
	m.planExportStatus = ""
	m.planExportError = ""
}

func (m *Model) resetLoadedPlanState() {
	m.loadedPlan = domain.CleanupPlan{}
	m.loadedPlanSummary = domain.CleanupPlanSummary{}
	m.loadedPlanCursor = 0
	m.loadedActionCursor = 0
	m.loadedActionOffset = 0
	m.planLoadError = ""
	m.planPathInput.Blur()
}

func (m *Model) openQuitConfirm() {
	m.quitReturnScreen = m.current
	m.quitCursor = quitConfirmCancel
	m.current = screenQuitConfirm
}

func (m *Model) openKeybindings() {
	m.helpReturnScreen = m.current
	m.current = screenKeybindings
}

func (m *Model) openLocalDataOverview() {
	m.refreshLocalData()
	m.localDataCursor = clampCursor(m.localDataCursor, len(localDataMenuItems))
	m.current = screenLocalDataOverview
}

func (m *Model) refreshLocalData() {
	if m.localWorkspace == nil {
		m.localConfig = workspace.Config{}
		m.recentImports = nil
		m.recentPlans = nil
		m.auditEvents = nil
		m.auditMalformed = 0
		if strings.TrimSpace(m.localDataWarning) == "" {
			m.localDataWarning = "Local data unavailable in this run."
		}
		return
	}

	config, err := m.localWorkspace.LoadConfig()
	if err != nil {
		m.warnLocalData("load config", err)
	} else {
		m.localConfig = config
	}
	imports, err := m.localWorkspace.RecentImports()
	if err != nil {
		m.warnLocalData("load recent imports", err)
	} else {
		m.recentImports = imports
	}
	plans, err := m.localWorkspace.RecentPlans()
	if err != nil {
		m.warnLocalData("load recent plans", err)
	} else {
		m.recentPlans = plans
	}
	audit, err := m.localWorkspace.ReadAudit()
	if err != nil {
		m.warnLocalData("load audit log", err)
	} else {
		m.auditEvents = audit.Events
		m.auditMalformed = audit.MalformedLines
	}

	m.recentImportCursor = clampCursor(m.recentImportCursor, len(m.recentImports))
	m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
	m.recentPlanCursor = clampCursor(m.recentPlanCursor, len(m.recentPlans))
	m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
	m.auditCursor = clampCursor(m.auditCursor, len(m.auditEvents))
	m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
}

func (m *Model) recordImportFinished(msg importFinishedMsg) {
	if msg.err != nil {
		m.appendAudit("import_failed", map[string]any{
			"source_label": sourceLabel(msg.source),
			"source_path":  sourcePath(msg.source),
			"platform":     string(domain.PlatformInstagram),
			"demo":         isDemoSource(msg.source),
			"error":        msg.err.Error(),
		})
		return
	}
	entry := workspace.RecentImport{
		SourceLabel:  sourceLabel(msg.source),
		SourcePath:   sourcePath(msg.source),
		Platform:     string(domain.PlatformInstagram),
		ImportedAt:   time.Now().UTC(),
		Demo:         isDemoSource(msg.source),
		ItemCount:    msg.result.Summary.Total,
		WarningCount: len(msg.result.Warnings),
		SkippedCount: msg.result.Summary.Skipped,
	}
	if m.localWorkspace != nil {
		if err := m.localWorkspace.UpsertRecentImport(entry); err != nil {
			m.warnLocalData("save recent import", err)
		}
	}
	m.appendAudit("import_completed", map[string]any{
		"source_label":  entry.SourceLabel,
		"source_path":   entry.SourcePath,
		"platform":      entry.Platform,
		"demo":          entry.Demo,
		"item_count":    entry.ItemCount,
		"warning_count": entry.WarningCount,
		"skipped_count": entry.SkippedCount,
	})
	m.refreshLocalData()
}

func (m *Model) recordPlanGenerated(result instagram.PlanBuildResult) {
	m.appendAudit("plan_generated", map[string]any{
		"plan_id":              result.Plan.ID,
		"mode":                 string(result.Plan.Mode),
		"source_name":          result.Plan.SourceName,
		"platform":             string(result.Plan.Platform),
		"selected_count":       result.SelectedCount,
		"action_count":         len(result.Plan.Actions),
		"skipped_count":        len(result.Skipped),
		"unlike_count":         result.Counts.Unlike,
		"delete_comment_count": result.Counts.DeleteComment,
		"unfollow_count":       result.Counts.Unfollow,
	})
}

func (m *Model) recordPlanExported(path string) {
	m.upsertRecentPlan(path, m.planResult.Plan)
	m.appendAudit("plan_exported", planAuditFields(path, m.planResult.Plan, domain.SummarizeCleanupPlan(m.planResult.Plan)))
	m.refreshLocalData()
}

func (m *Model) recordPlanLoaded(path string, plan domain.CleanupPlan, summary domain.CleanupPlanSummary) {
	m.upsertRecentPlan(path, plan)
	m.appendAudit("plan_loaded", planAuditFields(path, plan, summary))
	m.refreshLocalData()
}

func (m *Model) recordPlanLoadFailed(path string, err error) {
	m.appendAudit("plan_load_failed", map[string]any{
		"path":  path,
		"error": friendlyPlanLoadError(err),
	})
	m.refreshLocalData()
}

func (m *Model) upsertRecentPlan(path string, plan domain.CleanupPlan) {
	if m.localWorkspace == nil {
		return
	}
	summary := domain.SummarizeCleanupPlan(plan)
	entry := workspace.RecentPlan{
		ID:           plan.ID,
		Path:         strings.TrimSpace(path),
		Mode:         string(plan.Mode),
		SourceName:   plan.SourceName,
		CreatedAt:    plan.CreatedAt,
		ActionCounts: actionCountsForWorkspace(summary.ActionCounts),
		StatusCounts: statusCountsForWorkspace(summary.StatusCounts),
	}
	if err := m.localWorkspace.UpsertRecentPlan(entry); err != nil {
		m.warnLocalData("save recent plan", err)
	}
}

func (m *Model) appendAudit(eventType string, fields map[string]any) {
	if m.localWorkspace == nil {
		return
	}
	if err := m.localWorkspace.AppendAudit(workspace.AuditEvent{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Fields:    cleanAuditFields(fields),
	}); err != nil {
		m.warnLocalData("append audit event", err)
	}
}

func (m *Model) wipeLocalData() {
	if m.localWorkspace == nil {
		m.warnLocalData("wipe local data", errors.New("local workspace is unavailable"))
		return
	}
	if err := m.localWorkspace.Wipe(); err != nil {
		m.warnLocalData("wipe local data", err)
		return
	}
	m.localDataStatus = "Local data wiped. Vanish-managed defaults were recreated."
	m.localDataWarning = ""
	m.localConfig = workspace.Config{}
	m.recentImports = nil
	m.recentPlans = nil
	m.auditEvents = nil
	m.auditMalformed = 0
	m.recentImportCursor = 0
	m.recentImportOffset = 0
	m.recentPlanCursor = 0
	m.recentPlanOffset = 0
	m.auditCursor = 0
	m.auditOffset = 0
	m.refreshLocalData()
}

func (m *Model) warnLocalData(action string, err error) {
	if err == nil {
		return
	}
	m.localDataWarning = fmt.Sprintf("Local data warning: %s: %v", action, err)
}

func (m Model) itemListHeight() int {
	return boundedListHeight(m.height, 15, 3, 10)
}

func (m Model) warningListHeight() int {
	return boundedListHeight(m.height, 8, 3, 18)
}

func (m Model) planListHeight() int {
	return boundedListHeight(m.height, 20, 3, 8)
}

func (m Model) planActionListHeight() int {
	return boundedListHeight(m.height, 18, 3, 10)
}

func (m Model) localDataListHeight() int {
	return boundedListHeight(m.height, 18, 3, 8)
}

type keyMap struct {
	up               key.Binding
	down             key.Binding
	selectItem       key.Binding
	start            key.Binding
	save             key.Binding
	filter           key.Binding
	toggleSelection  key.Binding
	selectVisible    key.Binding
	deselectVisible  key.Binding
	selectionSummary key.Binding
	back             key.Binding
	cancel           key.Binding
	help             key.Binding
	quit             key.Binding
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
		save: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "save"),
		),
		filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filters"),
		),
		toggleSelection: key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "toggle"),
		),
		selectVisible: key.NewBinding(
			key.WithKeys("a", "A"),
			key.WithHelp("a", "select visible"),
		),
		deselectVisible: key.NewBinding(
			key.WithKeys("n", "N"),
			key.WithHelp("n", "deselect visible"),
		),
		selectionSummary: key.NewBinding(
			key.WithKeys("s", "S"),
			key.WithHelp("s", "selection"),
		),
		back: key.NewBinding(
			key.WithKeys("esc", "backspace"),
			key.WithHelp("esc/backspace", "back"),
		),
		cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		quit: key.NewBinding(
			key.WithKeys("ctrl+c", "ctrl+q"),
			key.WithHelp("ctrl+q", "quit"),
		),
	}
}

// ShortHelp and FullHelp make keyMap satisfy the Bubbles help.KeyMap interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.up, k.down, k.selectItem, k.filter, k.back, k.help, k.quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.up, k.down, k.selectItem, k.start, k.save, k.filter, k.toggleSelection, k.selectVisible, k.deselectVisible, k.selectionSummary, k.back, k.help, k.quit}}
}

type screenHelp []key.Binding

func (h screenHelp) ShortHelp() []key.Binding {
	return []key.Binding(h)
}

func (h screenHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{h.ShortHelp()}
}

type styles struct {
	frame     lipgloss.Style
	title     lipgloss.Style
	body      lipgloss.Style
	row       lipgloss.Style
	selected  lipgloss.Style
	muted     lipgloss.Style
	help      lipgloss.Style
	error     lipgloss.Style
	success   lipgloss.Style
	warning   lipgloss.Style
	badge     lipgloss.Style
	separator lipgloss.Style
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
		success: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#1A7F37"), lipgloss.Color("#7EE787"))).
			Width(74),
		warning: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#8A6100"), lipgloss.Color("#FFD479"))).
			Width(74),
		badge: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))),
		separator: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))),
	}
}

type importFinishedMsg struct {
	result instagram.ImportResult
	err    error
	source string
}

type exportPlanFinishedMsg struct {
	path string
	err  error
}

type loadPlanFinishedMsg struct {
	path       string
	plan       domain.CleanupPlan
	summary    domain.CleanupPlanSummary
	err        error
	fromRecent bool
}

func importZIPCmd(zipPath, source string) tea.Cmd {
	return func() tea.Msg {
		result, err := instagram.ImportZIP(zipPath)
		return importFinishedMsg{result: result, err: err, source: source}
	}
}

func writePlanJSONCmd(outputPath string, plan domain.CleanupPlan) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Create(outputPath)
		if err != nil {
			return exportPlanFinishedMsg{path: outputPath, err: fmt.Errorf("export plan: %w", err)}
		}
		defer file.Close()

		if err := domain.WritePlanJSON(file, plan); err != nil {
			return exportPlanFinishedMsg{path: outputPath, err: fmt.Errorf("export plan: %w", err)}
		}
		return exportPlanFinishedMsg{path: outputPath}
	}
}

func loadPlanJSONCmd(planPath string, fromRecent bool) tea.Cmd {
	return func() tea.Msg {
		plan, err := domain.LoadPlanJSONFile(planPath)
		if err != nil {
			return loadPlanFinishedMsg{path: planPath, err: err, fromRecent: fromRecent}
		}
		return loadPlanFinishedMsg{
			path:       planPath,
			plan:       plan,
			summary:    domain.SummarizeCleanupPlan(plan),
			fromRecent: fromRecent,
		}
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

func (m Model) selectableItemRow(item domain.ActivityItem) string {
	checked := " "
	if m.selection.Contains(item.ID) {
		checked = "x"
	}
	return fmt.Sprintf("[%s] %s", checked, itemRow(item))
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

func planPreviewRows(actions []domain.CleanupAction, skipped []instagram.PlanBuildSkip) []string {
	rows := make([]string, 0, len(actions)+len(skipped))
	for _, action := range actions {
		target := firstNonEmptyString(action.TargetURL, action.TargetID)
		rows = append(rows, fmt.Sprintf(
			"%s | %s | %s",
			action.Type,
			action.SourceActivityItemID,
			emptyFallback(target, "-"),
		))
	}
	for _, skip := range skipped {
		rows = append(rows, fmt.Sprintf(
			"skipped | %s | %s | %s",
			emptyFallback(skip.SourceActivityItemID, "-"),
			emptyFallback(skip.TargetRef, "-"),
			emptyFallback(skip.Reason, "unsupported"),
		))
	}
	return rows
}

func planActionRow(action domain.CleanupAction) string {
	target := firstNonEmptyString(action.TargetURL, action.TargetID)
	return fmt.Sprintf(
		"%s | %s | %s",
		action.Type,
		action.Status,
		emptyFallback(target, "-"),
	)
}

func planActionDetailLines(action domain.CleanupAction) []string {
	lines := []string{
		"ID: " + action.ID,
		"Type: " + string(action.Type),
		"Status: " + string(action.Status),
	}
	if action.TargetURL != "" {
		lines = append(lines, "Target URL: "+action.TargetURL)
	}
	if action.TargetID != "" {
		lines = append(lines, "Target ID: "+action.TargetID)
	}
	if action.SourceActivityItemID != "" {
		lines = append(lines, "Source activity item ID: "+action.SourceActivityItemID)
	}
	if !action.CreatedAt.IsZero() {
		lines = append(lines, "Created at: "+action.CreatedAt.Format(time.RFC3339))
	}
	return lines
}

func (m Model) actionCountLines(counts map[domain.ActionType]int) []string {
	if len(counts) == 0 {
		return []string{m.styles.muted.Render("none")}
	}

	keys := make([]string, 0, len(counts))
	for actionType := range counts {
		keys = append(keys, string(actionType))
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, m.styles.body.Render(fmt.Sprintf("%s: %d", key, counts[domain.ActionType(key)])))
	}
	return lines
}

func (m Model) localDataMessages() []string {
	lines := []string{}
	if strings.TrimSpace(m.localDataStatus) != "" {
		lines = append(lines, m.styles.success.Render(m.localDataStatus))
	}
	if strings.TrimSpace(m.localDataWarning) != "" {
		lines = append(lines, m.styles.warning.Render(m.localDataWarning))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) localDataDirLabel() string {
	if m.localWorkspace == nil {
		return "unavailable"
	}
	return m.localWorkspace.Dir()
}

func recentImportRow(entry workspace.RecentImport) string {
	return fmt.Sprintf(
		"%s | %s | items %d | warnings %d",
		formatPlanTime(entry.ImportedAt),
		emptyFallback(entry.SourceLabel, "-"),
		entry.ItemCount,
		entry.WarningCount,
	)
}

func recentImportDetailLines(entry workspace.RecentImport) []string {
	lines := []string{
		"Source label: " + emptyFallback(entry.SourceLabel, "-"),
		"Source path: " + emptyFallback(entry.SourcePath, "-"),
		"Platform: " + emptyFallback(entry.Platform, "-"),
		"Imported at: " + formatPlanTime(entry.ImportedAt),
		"Demo: " + boolLabel(entry.Demo),
		fmt.Sprintf("Items: %d", entry.ItemCount),
		fmt.Sprintf("Warnings: %d", entry.WarningCount),
		fmt.Sprintf("Skipped or unknown: %d", entry.SkippedCount),
	}
	return lines
}

func recentPlanRow(entry workspace.RecentPlan) string {
	return fmt.Sprintf(
		"%s | %s | actions %d",
		formatPlanTime(entry.CreatedAt),
		emptyFallback(entry.SourceName, entry.ID),
		sumStringCounts(entry.ActionCounts),
	)
}

func recentPlanDetailLines(entry workspace.RecentPlan) []string {
	lines := []string{
		"Plan ID: " + emptyFallback(entry.ID, "-"),
		"Path: " + emptyFallback(entry.Path, "-"),
		"Mode: " + emptyFallback(entry.Mode, "-"),
		"Source name: " + emptyFallback(entry.SourceName, "-"),
		"Created at: " + formatPlanTime(entry.CreatedAt),
		"Action counts: " + formatStringCounts(entry.ActionCounts),
		"Status counts: " + formatStringCounts(entry.StatusCounts),
	}
	return lines
}

func auditEventRow(event workspace.AuditEvent) string {
	return fmt.Sprintf("%s | %s", formatPlanTime(event.Timestamp), emptyFallback(event.Type, "-"))
}

func auditEventDetailLines(event workspace.AuditEvent) []string {
	lines := []string{
		"Type: " + emptyFallback(event.Type, "-"),
		"Timestamp: " + formatPlanTime(event.Timestamp),
	}
	if len(event.Fields) == 0 {
		return append(lines, "Fields: none")
	}
	lines = append(lines, "Fields:")
	keys := make([]string, 0, len(event.Fields))
	for key := range event.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("  %s: %s", key, formatAuditValue(event.Fields[key])))
	}
	return lines
}

func actionCountsForWorkspace(counts map[domain.ActionType]int) map[string]int {
	if len(counts) == 0 {
		return nil
	}
	result := make(map[string]int, len(counts))
	for actionType, count := range counts {
		result[string(actionType)] = count
	}
	return result
}

func statusCountsForWorkspace(counts map[domain.ActionStatus]int) map[string]int {
	if len(counts) == 0 {
		return nil
	}
	result := make(map[string]int, len(counts))
	for status, count := range counts {
		result[string(status)] = count
	}
	return result
}

func planAuditFields(path string, plan domain.CleanupPlan, summary domain.CleanupPlanSummary) map[string]any {
	return map[string]any{
		"path":                 strings.TrimSpace(path),
		"plan_id":              plan.ID,
		"mode":                 string(plan.Mode),
		"source_name":          plan.SourceName,
		"platform":             string(plan.Platform),
		"action_count":         summary.TotalActions,
		"unlike_count":         summary.ActionCounts[domain.ActionUnlike],
		"delete_comment_count": summary.ActionCounts[domain.ActionDeleteComment],
		"unfollow_count":       summary.ActionCounts[domain.ActionUnfollow],
		"pending_count":        summary.StatusCounts[domain.ActionStatusPending],
		"running_count":        summary.StatusCounts[domain.ActionStatusRunning],
		"done_count":           summary.StatusCounts[domain.ActionStatusDone],
		"failed_count":         summary.StatusCounts[domain.ActionStatusFailed],
		"skipped_count":        summary.StatusCounts[domain.ActionStatusSkipped],
	}
}

func cleanAuditFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	cleaned := make(map[string]any, len(fields))
	for key, value := range fields {
		switch typed := value.(type) {
		case string:
			cleaned[key] = strings.TrimSpace(typed)
		case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, nil:
			cleaned[key] = typed
		default:
			cleaned[key] = fmt.Sprint(typed)
		}
	}
	return cleaned
}

func sourceLabel(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "instagram export"
	}
	if isDemoSource(source) {
		return "demo instagram export"
	}
	base := filepath.Base(strings.Trim(source, `"'`))
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		return source
	}
	return base
}

func sourcePath(source string) string {
	source = strings.Trim(strings.TrimSpace(source), `"'`)
	if source == "" || isDemoSource(source) {
		return ""
	}
	return filepath.Clean(source)
}

func isDemoSource(source string) bool {
	return strings.EqualFold(strings.TrimSpace(source), "demo instagram export")
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func formatStringCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

func sumStringCounts(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func formatAuditValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "-"
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%g", typed)
	default:
		return fmt.Sprint(typed)
	}
}

func formatPlanTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
}

func friendlyPlanLoadError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), "plan path is required") {
		return "Plan path is required."
	}
	if errors.Is(err, os.ErrNotExist) {
		return "Plan file not found. Check the path and try again."
	}
	if errors.Is(err, domain.ErrUnsupportedPlanMode) {
		return "Unsupported plan mode. Vanish can only inspect dry-run plans right now."
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) || strings.Contains(err.Error(), "unexpected EOF") {
		return "Plan file is malformed JSON. Fix the JSON and try again."
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return "Plan file contains unsupported data. Check the field types and try again."
	}
	if strings.Contains(err.Error(), "unknown field") {
		return "Plan file has an unknown field. Export a fresh plan or remove unsupported fields."
	}
	if strings.Contains(err.Error(), "validation failed") {
		return "Plan file failed validation: " + err.Error()
	}

	return "Could not load plan: " + err.Error()
}

func (m Model) visibleItems() []domain.ActivityItem {
	return domain.FilterActivityItems(m.importResult.Items, m.itemFilter)
}

func (m Model) selectedItems() []domain.ActivityItem {
	return m.selection.SelectedItems(m.importResult.Items)
}

func (m *Model) beginFilterDraft() {
	m.draftFilter = copyActivityItemFilter(m.itemFilter)
	m.draftOlderDate = filterDateValue(m.itemFilter.OlderThan)
	m.draftNewerDate = filterDateValue(m.itemFilter.NewerThan)
	m.filterError = ""
	m.filterEditing = filterEditNone
	m.setFilterInputValues()
}

func (m *Model) clearFilterState() {
	m.itemFilter = domain.ActivityItemFilter{}
	m.draftFilter = domain.ActivityItemFilter{}
	m.draftOlderDate = ""
	m.draftNewerDate = ""
	m.filterError = ""
	m.filterEditing = filterEditNone
	m.filterCursor = 0
	m.itemCursor = 0
	m.itemOffset = 0
	m.setFilterInputValues()
}

func (m *Model) toggleDraftType(filterType domain.ActivityFilterType) {
	if m.draftFilter.IncludeTypes == nil {
		m.draftFilter.IncludeTypes = make(map[domain.ActivityFilterType]bool)
	}
	m.draftFilter.IncludeTypes[filterType] = !m.draftFilter.IncludeTypes[filterType]
	m.filterError = ""
}

func (m *Model) startFilterInput(row int) (tea.Model, tea.Cmd) {
	m.filterEditing = row
	m.filterError = ""
	m.blurFilterInputs()
	switch row {
	case filterRowActor:
		return *m, m.filterActorInput.Focus()
	case filterRowTarget:
		return *m, m.filterTargetInput.Focus()
	case filterRowOlder:
		return *m, m.filterOlderInput.Focus()
	case filterRowNewer:
		return *m, m.filterNewerInput.Focus()
	default:
		m.filterEditing = filterEditNone
		return *m, nil
	}
}

func (m *Model) acceptFilterInput() {
	switch m.filterEditing {
	case filterRowActor:
		m.draftFilter.ActorContains = strings.TrimSpace(m.filterActorInput.Value())
	case filterRowTarget:
		m.draftFilter.TargetContains = strings.TrimSpace(m.filterTargetInput.Value())
	case filterRowOlder:
		m.draftOlderDate = strings.TrimSpace(m.filterOlderInput.Value())
	case filterRowNewer:
		m.draftNewerDate = strings.TrimSpace(m.filterNewerInput.Value())
	}
	m.filterError = ""
	m.filterEditing = filterEditNone
	m.blurFilterInputs()
	m.setFilterInputValues()
}

func (m *Model) cancelFilterInput() {
	m.filterEditing = filterEditNone
	m.blurFilterInputs()
	m.setFilterInputValues()
}

func (m *Model) applyDraftFilter() {
	next := copyActivityItemFilter(m.draftFilter)
	next.OlderThan = nil
	next.NewerThan = nil
	if strings.TrimSpace(m.draftOlderDate) != "" {
		olderThan, err := domain.ParseFilterDate(m.draftOlderDate)
		if err != nil {
			m.filterError = "Older than date must use YYYY-MM-DD."
			return
		}
		next.OlderThan = &olderThan
	}
	if strings.TrimSpace(m.draftNewerDate) != "" {
		newerThan, err := domain.ParseFilterDate(m.draftNewerDate)
		if err != nil {
			m.filterError = "Newer than date must use YYYY-MM-DD."
			return
		}
		next.NewerThan = &newerThan
	}

	m.itemFilter = next
	m.filterError = ""
	items := m.visibleItems()
	m.itemCursor = clampCursor(m.itemCursor, len(items))
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.itemListHeight())
	m.current = screenItemsBrowser
}

func (m *Model) updateFocusedFilterInput(msg tea.Msg, cmd *tea.Cmd) {
	switch m.filterEditing {
	case filterRowActor:
		m.filterActorInput, *cmd = m.filterActorInput.Update(msg)
	case filterRowTarget:
		m.filterTargetInput, *cmd = m.filterTargetInput.Update(msg)
	case filterRowOlder:
		m.filterOlderInput, *cmd = m.filterOlderInput.Update(msg)
	case filterRowNewer:
		m.filterNewerInput, *cmd = m.filterNewerInput.Update(msg)
	}
}

func (m *Model) blurFilterInputs() {
	m.filterActorInput.Blur()
	m.filterTargetInput.Blur()
	m.filterOlderInput.Blur()
	m.filterNewerInput.Blur()
}

func (m *Model) setFilterInputValues() {
	m.filterActorInput.SetValue(m.draftFilter.ActorContains)
	m.filterTargetInput.SetValue(m.draftFilter.TargetContains)
	m.filterOlderInput.SetValue(m.draftOlderDate)
	m.filterNewerInput.SetValue(m.draftNewerDate)
}

func (m *Model) setFilterInputWidths(width int) {
	m.filterActorInput.SetWidth(width)
	m.filterTargetInput.SetWidth(width)
	m.filterOlderInput.SetWidth(width)
	m.filterNewerInput.SetWidth(width)
}

func (m Model) filterInputRow(label string, row int, input textinput.Model, value string) string {
	if m.filterEditing == row {
		return fmt.Sprintf("%s: %s", label, input.View())
	}
	return fmt.Sprintf("%s: %s", label, emptyFallback(value, "-"))
}

func newFilterInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.Prompt = ""
	input.CharLimit = 256
	input.SetWidth(74)
	return input
}

func newPlanPathInput() textinput.Model {
	input := textinput.New()
	input.Placeholder = defaultPlanExportPath
	input.Prompt = "> "
	input.CharLimit = 1024
	input.SetWidth(74)
	input.SetValue(defaultPlanExportPath)
	return input
}

func filterTypeRow(label string, included bool) string {
	checked := " "
	if included {
		checked = "x"
	}
	return fmt.Sprintf("[%s] %s", checked, label)
}

func filterDateValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}

func copyActivityItemFilter(filter domain.ActivityItemFilter) domain.ActivityItemFilter {
	copied := domain.ActivityItemFilter{
		ActorContains:  filter.ActorContains,
		TargetContains: filter.TargetContains,
		OlderThan:      filter.OlderThan,
		NewerThan:      filter.NewerThan,
	}
	if len(filter.IncludeTypes) > 0 {
		copied.IncludeTypes = make(map[domain.ActivityFilterType]bool, len(filter.IncludeTypes))
		for filterType, included := range filter.IncludeTypes {
			copied.IncludeTypes[filterType] = included
		}
	}
	return copied
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

func activeLabel(active bool) string {
	if active {
		return "active"
	}
	return "off"
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
