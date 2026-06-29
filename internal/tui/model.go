package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

const defaultPlanExportPath = workspace.DefaultPlanExportPath

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
	if localWorkspace != nil {
		m.refreshLocalData()
		if planPath := m.defaultPlanPathValue(); planPath != "" {
			m.planPathInput.SetValue(planPath)
		}
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

	case tea.MouseClickMsg:
		return m.updateMouseClick(msg)

	case tea.MouseWheelMsg:
		return m.updateMouseWheel(msg)

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
			m.planPathInput.SetValue(m.loadPlanPathValue())
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
			m.planPathInput.SetValue(m.defaultPlanPathValue())
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
				m.planPathInput.SetValue(m.defaultPlanPathValue())
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
			outputPath = m.defaultPlanPathValue()
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
			planPath = m.loadPlanPathValue()
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

func (m Model) updateMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	content := m.View().Content

	switch m.current {
	case screenHome:
		if index := menuIndexAtY(content, mouse.Y, homeMenuItems); index >= 0 {
			if index == m.homeCursor {
				return m.updateHome(selectKeyPress())
			}
			m.homeCursor = index
		}
	case screenImportResult:
		if m.importErr != nil {
			return m, nil
		}
		if index := menuIndexAtY(content, mouse.Y, resultMenuItems); index >= 0 {
			if index == m.resultCursor {
				return m.updateImportResult(selectKeyPress())
			}
			m.resultCursor = index
		}
	case screenItemsBrowser:
		return m.updateItemListClick(content, mouse.Y)
	case screenSelectionSummary:
		if index := menuIndexAtY(content, mouse.Y, selectionMenuItems); index >= 0 {
			if index == m.selectionCursor {
				return m.updateSelectionSummary(selectKeyPress())
			}
			m.selectionCursor = index
			m.selectionMessage = ""
		}
	case screenSelectedItems:
		m.updateSelectedItemListClick(content, mouse.Y)
	case screenPlanPreview:
		if index := menuIndexAtY(content, mouse.Y, planPreviewMenuItems); index >= 0 {
			if index == m.planPreviewCursor {
				return m.updatePlanPreview(selectKeyPress())
			}
			m.planPreviewCursor = index
		}
	case screenLoadedPlanSummary:
		if index := menuIndexAtY(content, mouse.Y, loadedPlanSummaryMenuItems); index >= 0 {
			if index == m.loadedPlanCursor {
				return m.updateLoadedPlanSummary(selectKeyPress())
			}
			m.loadedPlanCursor = index
		}
	case screenLoadedPlanActions:
		m.updatePlanActionListClick(content, mouse.Y)
	case screenWarnings:
		m.updateWarningListClick(content, mouse.Y)
	case screenLocalDataOverview:
		if index := menuIndexAtY(content, mouse.Y, localDataMenuItems); index >= 0 {
			if index == m.localDataCursor {
				return m.updateLocalDataOverview(selectKeyPress())
			}
			m.localDataCursor = index
		}
	case screenRecentImports:
		m.updateRecentImportListClick(content, mouse.Y)
	case screenRecentPlans:
		m.updateRecentPlanListClick(content, mouse.Y)
	case screenAuditLog:
		m.updateAuditListClick(content, mouse.Y)
	case screenWipeLocalDataConfirm:
		if index := menuIndexAtY(content, mouse.Y, wipeLocalDataMenuItems); index >= 0 {
			if index == m.wipeLocalDataCursor {
				return m.updateWipeLocalDataConfirm(selectKeyPress())
			}
			m.wipeLocalDataCursor = index
		}
	case screenQuitConfirm:
		if index := menuIndexAtY(content, mouse.Y, quitConfirmMenuItems); index >= 0 {
			if index == m.quitCursor {
				return m.updateQuitConfirm(selectKeyPress())
			}
			m.quitCursor = index
		}
	}

	return m, nil
}

func (m Model) updateMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	delta := 0
	switch mouse.Button {
	case tea.MouseWheelUp:
		delta = -1
	case tea.MouseWheelDown:
		delta = 1
	default:
		return m, nil
	}

	switch m.current {
	case screenItemsBrowser:
		items := m.visibleItems()
		m.itemCursor = moveCursor(m.itemCursor, len(items), delta)
		m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.itemListHeight())
	case screenSelectedItems:
		items := m.selectedItems()
		m.selectedCursor = moveCursor(m.selectedCursor, len(items), delta)
		m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(items), m.itemListHeight())
	case screenLoadedPlanActions:
		m.loadedActionCursor = moveCursor(m.loadedActionCursor, len(m.loadedPlan.Actions), delta)
		m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
	case screenWarnings:
		m.warningCursor = moveCursor(m.warningCursor, len(m.importResult.Warnings), delta)
		m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
	case screenRecentImports:
		m.recentImportCursor = moveCursor(m.recentImportCursor, len(m.recentImports), delta)
		m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
	case screenRecentPlans:
		m.recentPlanError = ""
		m.recentPlanCursor = moveCursor(m.recentPlanCursor, len(m.recentPlans), delta)
		m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
	case screenAuditLog:
		m.auditCursor = moveCursor(m.auditCursor, len(m.auditEvents), delta)
		m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
	default:
		return m, nil
	}

	return m, nil
}

func (m Model) updateItemListClick(content string, y int) (tea.Model, tea.Cmd) {
	items := m.visibleItems()
	ordinal := rowOrdinalAtY(content, y, isSelectionRowLine)
	index := m.itemOffset + ordinal
	if ordinal < 0 || index < 0 || index >= len(items) {
		return m, nil
	}
	if index == clampCursor(m.itemCursor, len(items)) {
		return m.updateItemsBrowser(selectKeyPress())
	}
	m.itemCursor = index
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.itemListHeight())
	return m, nil
}

func (m *Model) updateSelectedItemListClick(content string, y int) {
	items := m.selectedItems()
	ordinal := rowOrdinalAtY(content, y, isSelectionRowLine)
	index := m.selectedOffset + ordinal
	if ordinal < 0 || index < 0 || index >= len(items) {
		return
	}
	m.selectedCursor = index
	m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(items), m.itemListHeight())
}

func (m *Model) updatePlanActionListClick(content string, y int) {
	actions := m.loadedPlan.Actions
	anchors := make([]string, 0, len(actions))
	for _, action := range actions {
		anchors = append(anchors, actionRowAnchor(action))
	}
	ordinal := rowOrdinalAtY(content, y, lineMatchesAnyAnchor(anchors))
	index := m.loadedActionOffset + ordinal
	if ordinal < 0 || index < 0 || index >= len(actions) {
		return
	}
	m.loadedActionCursor = index
	m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(actions), m.planActionListHeight())
}

func (m *Model) updateWarningListClick(content string, y int) {
	ordinal := rowOrdinalAtY(content, y, lineMatchesAnyAnchor(m.importResult.Warnings))
	index := m.warningOffset + ordinal
	if ordinal < 0 || index < 0 || index >= len(m.importResult.Warnings) {
		return
	}
	m.warningCursor = index
	m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
}

func (m *Model) updateRecentImportListClick(content string, y int) {
	anchors := make([]string, 0, len(m.recentImports))
	for _, entry := range m.recentImports {
		anchors = append(anchors, recentImportRow(entry))
	}
	ordinal := rowOrdinalAtY(content, y, lineMatchesAnyAnchor(anchors))
	index := m.recentImportOffset + ordinal
	if ordinal < 0 || index < 0 || index >= len(m.recentImports) {
		return
	}
	m.recentImportCursor = index
	m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
}

func (m *Model) updateRecentPlanListClick(content string, y int) {
	anchors := make([]string, 0, len(m.recentPlans))
	for _, entry := range m.recentPlans {
		anchors = append(anchors, recentPlanRow(entry))
	}
	ordinal := rowOrdinalAtY(content, y, lineMatchesAnyAnchor(anchors))
	index := m.recentPlanOffset + ordinal
	if ordinal < 0 || index < 0 || index >= len(m.recentPlans) {
		return
	}
	m.recentPlanError = ""
	m.recentPlanCursor = index
	m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
}

func (m *Model) updateAuditListClick(content string, y int) {
	anchors := make([]string, 0, len(m.auditEvents))
	for _, event := range m.auditEvents {
		anchors = append(anchors, auditEventRow(event))
	}
	ordinal := rowOrdinalAtY(content, y, lineMatchesAnyAnchor(anchors))
	index := m.auditOffset + ordinal
	if ordinal < 0 || index < 0 || index >= len(m.auditEvents) {
		return
	}
	m.auditCursor = index
	m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
}

// View renders current model as terminal content.
func (m Model) View() tea.View {
	var content string
	switch m.current {
	case screenHome:
		content = m.homeView()
	case screenImportPath:
		content = m.importPathView()
	case screenImporting:
		content = m.importingView()
	case screenImportResult:
		content = m.importResultView()
	case screenItemsBrowser:
		content = m.itemsBrowserView()
	case screenFilters:
		content = m.filtersView()
	case screenSelectionSummary:
		content = m.selectionSummaryView()
	case screenSelectedItems:
		content = m.selectedItemsView()
	case screenPlanPreview:
		content = m.planPreviewView()
	case screenPlanExportPath:
		content = m.planExportPathView()
	case screenPlanLoadPath:
		content = m.planLoadPathView()
	case screenLoadedPlanSummary:
		content = m.loadedPlanSummaryView()
	case screenLoadedPlanActions:
		content = m.loadedPlanActionsView()
	case screenWarnings:
		content = m.warningsView()
	case screenLocalDataOverview:
		content = m.localDataOverviewView()
	case screenRecentImports:
		content = m.recentImportsView()
	case screenRecentPlans:
		content = m.recentPlansView()
	case screenAuditLog:
		content = m.auditLogView()
	case screenWipeLocalDataConfirm:
		content = m.wipeLocalDataConfirmView()
	case screenKeybindings:
		content = m.keybindingsView()
	case screenQuitConfirm:
		content = m.quitConfirmView()
	default:
		content = m.homeView()
	}

	view := tea.NewView(content)
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m Model) homeView() string {
	spec := layoutSpec(m.width, m.height)
	menu := m.menuRows(homeMenuItems, m.homeCursor, spec.sidebarWidth)
	workspaceLines := m.keyValueRows([]keyValue{
		{Key: "App directory", Value: m.localDataDirLabel()},
		{Key: "Telemetry", Value: enabledLabel(m.localConfig.Telemetry.Enabled)},
		{Key: "Recent imports", Value: compactCount(len(m.recentImports))},
		{Key: "Recent plans", Value: compactCount(len(m.recentPlans))},
		{Key: "Audit events", Value: compactCount(len(m.auditEvents))},
	})
	if m.auditMalformed > 0 {
		workspaceLines = append(workspaceLines, m.notice("warning", fmt.Sprintf("Malformed audit lines: %d", m.auditMalformed)))
	}

	right := m.dashboardSections(
		spec.detailWidth,
		m.warningBanner(m.localDataWarning, spec.detailWidth),
		m.section("Safety", []string{
			m.styles.body.Render("Local-only review of files you choose."),
			m.styles.body.Render("Dry-run cleanup planning only."),
			m.styles.body.Render("No login, browser automation, deletion, or network requests."),
		}),
		m.section("Workspace", workspaceLines),
		m.section("Getting Started", []string{
			m.styles.body.Render("Import a local Instagram export ZIP or run the demo data."),
			m.styles.body.Render("Review parsed items, filters, and selections."),
			m.styles.body.Render("Generate and export a local dry-run plan JSON."),
		}),
	)

	body := m.twoPane(
		spec,
		"Start", "Choose a local workflow", menu,
		"Command Center", "Safety and local workspace", right,
	)
	return m.appShell("Home", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.help, m.keys.quit))
}

func (m Model) importPathView() string {
	lines := []string{
		m.styles.body.Render("Type the path to a local Instagram data export .zip file."),
		m.styles.muted.Render("Vanish will only read local JSON files from the ZIP."),
		"",
		m.pathInput.View(),
	}
	return m.singlePane("Import Instagram Export", "Local ZIP path", lines, m.keys.start, m.keys.cancel, m.keys.help, m.keys.quit)
}

func (m Model) importingView() string {
	source := m.importSource
	if source == "" {
		source = "instagram export"
	}

	lines := []string{
		m.styles.body.Render(fmt.Sprintf("%s Parsing local ZIP...", m.spinner.View())),
		m.styles.muted.Render(truncateMiddle(source, layoutSpec(m.width, m.height).contentWidth-4)),
	}
	return m.singlePane("Importing", "Reading local files only", lines, m.keys.help, m.keys.quit)
}

func (m Model) importResultView() string {
	if m.importErr != nil {
		lines := []string{
			m.notice("error", m.importErr.Error()),
			m.styles.muted.Render("Check that the path points to a local Instagram export .zip, then try again."),
			"",
			m.styles.muted.Render(truncateMiddle(m.importSource, layoutSpec(m.width, m.height).contentWidth-4)),
		}
		lines = append(lines, m.localDataMessages()...)
		return m.singlePane("Import Failed", "No data was imported", lines, m.keys.back, m.keys.help, m.keys.quit)
	}

	spec := layoutSpec(m.width, m.height)
	summary := m.importResult.Summary
	summaryLines := []string{
		m.styles.body.Render(fmt.Sprintf("Source: %s", emptyFallback(m.importSource, "instagram export"))),
		"",
		m.styles.body.Render(fmt.Sprintf("Parsed: %d total | Likes: %d | Comments: %d | Following: %d | Followers: %d", summary.Total, summary.Likes, summary.Comments, summary.Following, summary.Followers)),
		m.styles.body.Render(fmt.Sprintf("Skipped or unknown: %d | Warnings: %d", summary.Skipped, len(m.importResult.Warnings))),
	}
	summaryLines = append(summaryLines, m.localDataMessages()...)
	body := m.twoPane(
		spec,
		"Actions", "Next review step", m.menuRows(resultMenuItems, m.resultCursor, spec.sidebarWidth),
		"Import Complete", "Parsed local export", summaryLines,
	)
	return m.appShell("Import Complete", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) itemsBrowserView() string {
	spec := layoutSpec(m.width, m.height)
	items := m.visibleItems()
	listWidth, detailWidth := twoPaneWidths(spec, "Parsed Items")
	total := len(m.importResult.Items)
	visibleRows := m.itemListHeight()
	cursor := clampCursor(m.itemCursor, len(items))
	offset := ensureOffset(cursor, m.itemOffset, len(items), visibleRows)

	filterStatus := "off"
	if m.itemFilter.Active() {
		filterStatus = "active"
	}
	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("Visible: %s / %s | Selected: %s | Filters: %s", compactCount(len(items)), compactCount(total), compactCount(m.selection.Len()), filterStatus)),
		m.styles.muted.Render(fmt.Sprintf("Source: %s", emptyFallback(m.importSource, "instagram export"))),
		"",
	}
	if m.itemFilter.Active() {
		listLines = append(listLines, m.notice("warning", "Filters active"), "")
	}

	if len(items) == 0 {
		listLines = append(listLines, m.emptyState("No parsed items."))
	} else {
		rows := make([]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, m.selectableItemRow(item))
		}
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth)...)
	}

	detailLines := []string{}
	if len(items) == 0 {
		detailLines = append(detailLines, m.emptyState("No items match the current filters. Clear filters or import another ZIP."))
	} else {
		detailLines = append(detailLines, m.detailRows(itemDetailLines(items[cursor]), detailWidth)...)
	}

	body := m.twoPane(spec, "Parsed Items", "Review and toggle", listLines, "Details", "Highlighted item", detailLines)
	return m.appShell("Parsed Items", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.toggleSelection, m.keys.selectionSummary, m.keys.filter, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) selectionSummaryView() string {
	spec := layoutSpec(m.width, m.height)
	counts := m.selection.Counts(m.importResult.Items)
	visibleCount := len(m.visibleItems())
	summaryLines := m.dashboardSections(
		spec.detailWidth,
		m.warningBanner(m.selectionMessage, spec.detailWidth),
		m.section("Selection Totals", m.keyValueRows([]keyValue{
			{Key: "Total selected", Value: compactCount(counts.Total)},
			{Key: "Visible items", Value: compactCount(visibleCount)},
			{Key: "All parsed items", Value: compactCount(len(m.importResult.Items))},
		})),
		m.section("Selected Type Counts", m.keyValueRows([]keyValue{
			{Key: "Likes", Value: compactCount(counts.Likes)},
			{Key: "Comments", Value: compactCount(counts.Comments)},
			{Key: "Following", Value: compactCount(counts.Following)},
			{Key: "Followers", Value: compactCount(counts.Followers)},
		})),
		m.section("Current Filters", m.filterSummaryLines()),
		m.section("Next Suggested Action", []string{m.styles.body.Render(m.selectionNextAction(counts.Total))}),
	)
	body := m.twoPane(
		spec,
		"Actions", "Selection workflow", m.menuRows(selectionMenuItems, m.selectionCursor, spec.sidebarWidth),
		"Selection Dashboard", "Current review set", summaryLines,
	)
	return m.appShell("Selection Summary", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) selectedItemsView() string {
	spec := layoutSpec(m.width, m.height)
	items := m.selectedItems()
	listWidth, detailWidth := twoPaneWidths(spec, "Selected Items")
	total := len(m.importResult.Items)
	visibleRows := m.itemListHeight()
	cursor := clampCursor(m.selectedCursor, len(items))
	offset := ensureOffset(cursor, m.selectedOffset, len(items), visibleRows)

	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("Selected: %d / Total: %d", len(items), total)),
		"",
	}

	if len(items) == 0 {
		listLines = append(listLines, m.emptyState("No selected items yet. Toggle items in the parsed item list or select visible items from the summary."))
	} else {
		rows := make([]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, m.selectableItemRow(item))
		}
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth)...)
	}

	detailLines := []string{}
	if len(items) == 0 {
		detailLines = append(detailLines, m.emptyState("No item selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(itemDetailLines(items[cursor]), detailWidth)...)
	}

	body := m.twoPane(spec, "Selected Items", "Chosen cleanup candidates", listLines, "Details", "Highlighted item", detailLines)
	return m.appShell("Selected Items", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) planPreviewView() string {
	spec := layoutSpec(m.width, m.height)
	result := m.planResult
	summaryWidth, actionWidth := twoPaneWidths(spec, "Plan Summary")
	rows := planPreviewRows(result.Plan.Actions, result.Skipped)
	visibleRows := m.planListHeight()
	offset := ensureOffset(0, m.planListOffset, len(rows), visibleRows)

	summaryLines := []string{
		m.styles.body.Render(fmt.Sprintf("Plan mode: %s", result.Plan.Mode)),
		m.styles.body.Render(fmt.Sprintf("Source platform: %s", result.Plan.Platform)),
		m.styles.body.Render(fmt.Sprintf("Selected items: %d", result.SelectedCount)),
		m.styles.body.Render(fmt.Sprintf("Supported actions: %d", len(result.Plan.Actions))),
		m.styles.body.Render(fmt.Sprintf("Unsupported/skipped selected items: %d", len(result.Skipped))),
		m.styles.body.Render(fmt.Sprintf("Action counts: unlike %d, delete_comment %d, unfollow %d", result.Counts.Unlike, result.Counts.DeleteComment, result.Counts.Unfollow)),
		"",
	}
	summaryLines = append(summaryLines, m.menuRows(planPreviewMenuItems, m.planPreviewCursor, summaryWidth)...)

	actionLines := []string{}
	if len(rows) == 0 {
		actionLines = append(actionLines, m.emptyState("No supported actions."))
	} else {
		actionLines = append(actionLines, m.plainRows(rows, offset, visibleRows, actionWidth)...)
	}

	body := m.twoPane(spec, "Plan Summary", "Dry-run only", summaryLines, "Planned actions", "Supported and skipped", actionLines)
	return m.appShell("Dry-Run Plan Preview", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) planExportPathView() string {
	lines := []string{
		m.styles.body.Render("Output path"),
		m.planPathInput.View(),
		"",
	}

	if strings.TrimSpace(m.planExportStatus) != "" {
		lines = append(lines, m.notice("success", m.planExportStatus), "")
	}
	if strings.TrimSpace(m.planExportError) != "" {
		lines = append(lines, m.notice("error", m.planExportError), "")
	}
	lines = append(lines, m.localDataMessages()...)

	return m.singlePane("Export Plan JSON", "Write a local dry-run plan", lines, m.keys.save, m.keys.cancel, m.keys.help, m.keys.quit)
}

func (m Model) planLoadPathView() string {
	lines := []string{
		m.styles.body.Render("Type the path to a local cleanup plan JSON file."),
		m.styles.muted.Render("Vanish will only read and validate the local file."),
		"",
		m.planPathInput.View(),
		"",
	}

	if strings.TrimSpace(m.planLoadError) != "" {
		lines = append(lines, m.notice("error", m.planLoadError), "")
	}

	return m.singlePane("Load Cleanup Plan", "Local JSON path", lines, m.keys.start, m.keys.cancel, m.keys.help, m.keys.quit)
}

func (m Model) loadedPlanSummaryView() string {
	spec := layoutSpec(m.width, m.height)
	plan := m.loadedPlan
	summary := m.loadedPlanSummary

	detailLines := m.dashboardSections(
		spec.detailWidth,
		m.section("Plan", m.keyValueRows([]keyValue{
			{Key: "Plan ID", Value: truncateMiddle(emptyFallback(plan.ID, "-"), 24)},
			{Key: "Platform", Value: emptyFallback(string(plan.Platform), "-")},
			{Key: "Mode", Value: emptyFallback(string(plan.Mode), "-")},
			{Key: "Source", Value: emptyFallback(plan.SourceName, "-")},
			{Key: "Created at", Value: formatPlanTime(plan.CreatedAt)},
			{Key: "Total actions", Value: compactCount(summary.TotalActions)},
		})),
		m.section("Action Counts", m.actionCountLines(summary.ActionCounts)),
		m.section("Status Counts", m.statusCountLines(summary.StatusCounts)),
	)
	body := m.twoPane(
		spec,
		"Actions", "Loaded plan", m.menuRows(loadedPlanSummaryMenuItems, m.loadedPlanCursor, spec.sidebarWidth),
		"Loaded Cleanup Plan", "Plan metadata", detailLines,
	)
	return m.appShell("Loaded Cleanup Plan", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) loadedPlanActionsView() string {
	spec := layoutSpec(m.width, m.height)
	actions := m.loadedPlan.Actions
	listWidth, detailWidth := twoPaneWidths(spec, "Plan Actions")
	visibleRows := m.planActionListHeight()
	cursor := clampCursor(m.loadedActionCursor, len(actions))
	offset := ensureOffset(cursor, m.loadedActionOffset, len(actions), visibleRows)

	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("Actions: %d | Plan: %s", len(actions), emptyFallback(m.loadedPlan.ID, "-"))),
		"",
	}

	if len(actions) == 0 {
		listLines = append(listLines, m.emptyState("No actions in this plan."))
	} else {
		rows := make([]string, 0, len(actions))
		for _, action := range actions {
			rows = append(rows, planActionRow(action))
		}
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth)...)
	}

	detailLines := []string{}
	if len(actions) == 0 {
		detailLines = append(detailLines, m.emptyState("No action selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(planActionDetailLines(actions[cursor]), detailWidth)...)
	}

	body := m.twoPane(spec, "Plan Actions", "Read-only dry-run actions", listLines, "Details", "Highlighted action", detailLines)
	return m.appShell("Plan Actions", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) filtersView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.styles.muted.Render(fmt.Sprintf("Visible: %d / %d | Filters: %s", len(m.visibleItems()), len(m.importResult.Items), activeLabel(m.itemFilter.Active()))),
		"",
	}

	if m.itemFilter.Active() {
		lines = append(lines, m.notice("warning", "Filters active"), "")
	}
	if strings.TrimSpace(m.filterError) != "" {
		lines = append(lines, m.notice("error", m.filterError), "")
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
		lines = append(lines, m.selectableLine(row, i == m.filterCursor, spec.contentWidth))
	}

	if m.filterEditing == filterEditNone {
		return m.singlePane("Filters", "Constrain parsed items", lines, m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit)
	} else {
		return m.singlePane("Filters", "Editing filter value", lines, m.keys.save, m.keys.cancel, m.keys.help, m.keys.quit)
	}
}

func (m Model) warningsView() string {
	spec := layoutSpec(m.width, m.height)
	warnings := m.importResult.Warnings
	visibleRows := m.warningListHeight()
	cursor := clampCursor(m.warningCursor, len(warnings))
	offset := ensureOffset(cursor, m.warningOffset, len(warnings), visibleRows)

	lines := []string{
		m.styles.muted.Render(fmt.Sprintf("%d warnings from %s", len(warnings), emptyFallback(m.importSource, "instagram export"))),
		"",
	}

	if len(warnings) == 0 {
		lines = append(lines, m.emptyState("No warnings."))
	} else {
		lines = append(lines, m.tableRows(warnings, cursor, offset, visibleRows, spec.contentWidth)...)
	}

	return m.singlePane("Import Warnings", "Skipped or unsupported local files", lines, m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit)
}

func (m Model) localDataOverviewView() string {
	spec := layoutSpec(m.width, m.height)
	stats := []string{
		m.styles.body.Render("Vanish stores local metadata only in its app directory."),
		m.styles.muted.Render("Imports and cleanup plans stay at the local paths you choose."),
		"",
		m.styles.body.Render(fmt.Sprintf("App directory: %s", m.localDataDirLabel())),
		m.styles.body.Render(fmt.Sprintf("Telemetry: %s", enabledLabel(m.localConfig.Telemetry.Enabled))),
		m.styles.body.Render(fmt.Sprintf("Recent imports: %d", len(m.recentImports))),
		m.styles.body.Render(fmt.Sprintf("Recent plans: %d", len(m.recentPlans))),
		m.styles.body.Render(fmt.Sprintf("Audit events: %d", len(m.auditEvents))),
	}
	if m.auditMalformed > 0 {
		stats = append(stats, m.notice("warning", fmt.Sprintf("Skipped malformed audit lines: %d", m.auditMalformed)))
	}
	stats = append(stats, "")
	stats = append(stats, m.localDataMessages()...)
	body := m.twoPane(
		spec,
		"Actions", "Local metadata", m.menuRows(localDataMenuItems, m.localDataCursor, spec.sidebarWidth),
		"Local Data", "Workspace overview", stats,
	)
	return m.appShell("Local Data", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) recentImportsView() string {
	spec := layoutSpec(m.width, m.height)
	visibleRows := m.localDataListHeight()
	listWidth, detailWidth := twoPaneWidths(spec, "Recent Imports")
	cursor := clampCursor(m.recentImportCursor, len(m.recentImports))
	offset := ensureOffset(cursor, m.recentImportOffset, len(m.recentImports), visibleRows)
	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("%d recent imports from %s", len(m.recentImports), m.localDataDirLabel())),
		"",
	}
	listLines = append(listLines, m.localDataMessages()...)
	if len(m.recentImports) == 0 {
		listLines = append(listLines, m.emptyState("No recent imports yet. Import demo data or a local Instagram ZIP to add one."))
	} else {
		rows := make([]string, 0, len(m.recentImports))
		for _, entry := range m.recentImports {
			rows = append(rows, recentImportRow(entry))
		}
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth)...)
	}
	detailLines := []string{}
	if len(m.recentImports) == 0 {
		detailLines = append(detailLines, m.emptyState("No import selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(recentImportDetailLines(m.recentImports[cursor]), detailWidth)...)
	}
	body := m.twoPane(spec, "Recent Imports", "Newest first", listLines, "Details", "Highlighted import", detailLines)
	return m.appShell("Recent Imports", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) recentPlansView() string {
	spec := layoutSpec(m.width, m.height)
	visibleRows := m.localDataListHeight()
	listWidth, detailWidth := twoPaneWidths(spec, "Recent Plans")
	cursor := clampCursor(m.recentPlanCursor, len(m.recentPlans))
	offset := ensureOffset(cursor, m.recentPlanOffset, len(m.recentPlans), visibleRows)
	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("%d recent plans from %s", len(m.recentPlans), m.localDataDirLabel())),
		"",
	}
	listLines = append(listLines, m.localDataMessages()...)
	if strings.TrimSpace(m.recentPlanError) != "" {
		listLines = append(listLines, m.notice("error", m.recentPlanError), "")
	}
	if len(m.recentPlans) == 0 {
		listLines = append(listLines, m.emptyState("No recent plans yet. Export or load a dry-run cleanup plan to add one."))
	} else {
		rows := make([]string, 0, len(m.recentPlans))
		for _, entry := range m.recentPlans {
			rows = append(rows, recentPlanRow(entry))
		}
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth)...)
	}
	detailLines := []string{}
	if len(m.recentPlans) == 0 {
		detailLines = append(detailLines, m.emptyState("No plan selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(recentPlanDetailLines(m.recentPlans[cursor]), detailWidth)...)
	}
	body := m.twoPane(spec, "Recent Plans", "Enter loads selected", listLines, "Details", "Highlighted plan", detailLines)
	return m.appShell("Recent Plans", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) auditLogView() string {
	spec := layoutSpec(m.width, m.height)
	visibleRows := m.localDataListHeight()
	listWidth, detailWidth := twoPaneWidths(spec, "Audit Log")
	cursor := clampCursor(m.auditCursor, len(m.auditEvents))
	offset := ensureOffset(cursor, m.auditOffset, len(m.auditEvents), visibleRows)
	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("%d audit events from %s", len(m.auditEvents), m.localDataDirLabel())),
		"",
	}
	listLines = append(listLines, m.localDataMessages()...)
	if m.auditMalformed > 0 {
		listLines = append(listLines, m.notice("warning", fmt.Sprintf("Skipped malformed audit lines: %d", m.auditMalformed)), "")
	}
	if len(m.auditEvents) == 0 {
		listLines = append(listLines, m.emptyState("No audit events yet."))
	} else {
		rows := make([]string, 0, len(m.auditEvents))
		for _, entry := range m.auditEvents {
			rows = append(rows, auditEventRow(entry))
		}
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth)...)
	}
	detailLines := []string{}
	if len(m.auditEvents) == 0 {
		detailLines = append(detailLines, m.emptyState("No audit event selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(auditEventDetailLines(m.auditEvents[cursor]), detailWidth)...)
	}
	body := m.twoPane(spec, "Audit Log", "Local metadata events", listLines, "Details", "Highlighted event", detailLines)
	return m.appShell("Audit Log", body, m.contextFooter(m.keys.up, m.keys.down, m.keys.back, m.keys.help, m.keys.quit))
}

func (m Model) wipeLocalDataConfirmView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.notice("warning", "This clears Vanish-managed config, recent history, and audit records."),
		m.styles.body.Render("It does not delete Instagram export ZIPs or cleanup plan JSON files outside the app directory."),
		m.styles.body.Render(fmt.Sprintf("App directory: %s", m.localDataDirLabel())),
		"",
	}
	lines = append(lines, m.localDataMessages()...)
	lines = append(lines, m.menuRows(wipeLocalDataMenuItems, m.wipeLocalDataCursor, spec.contentWidth)...)
	return m.singlePane("Wipe Local Data?", "Defaults to Cancel", lines, m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help, m.keys.quit)
}

func (m Model) keybindingsView() string {
	lines := []string{
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
	}
	return m.singlePane("Help", "Keyboard and safety model", lines, m.keys.back, m.keys.quit)
}

func (m Model) quitConfirmView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.styles.body.Render("Your current in-memory review state will be discarded."),
		"",
	}

	lines = append(lines, m.menuRows(quitConfirmMenuItems, m.quitCursor, spec.contentWidth)...)
	return m.singlePane("Quit Vanish?", "Defaults to Cancel", lines, m.keys.up, m.keys.down, m.keys.selectItem, m.keys.back, m.keys.help)
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
	m.planPathInput.SetValue(m.defaultPlanPathValue())
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
		SourceLabel:    sourceLabel(msg.source),
		SourcePath:     sourcePath(msg.source),
		Platform:       string(domain.PlatformInstagram),
		ImportedAt:     time.Now().UTC(),
		Demo:           isDemoSource(msg.source),
		ItemCount:      msg.result.Summary.Total,
		LikeCount:      msg.result.Summary.Likes,
		CommentCount:   msg.result.Summary.Comments,
		FollowingCount: msg.result.Summary.Following,
		FollowerCount:  msg.result.Summary.Followers,
		WarningCount:   len(msg.result.Warnings),
		SkippedCount:   msg.result.Summary.Skipped,
	}
	if m.localWorkspace != nil {
		if err := m.localWorkspace.UpsertRecentImport(entry); err != nil {
			m.warnLocalData("save recent import", err)
		}
	}
	m.appendAudit("import_completed", map[string]any{
		"source_label":    entry.SourceLabel,
		"source_path":     entry.SourcePath,
		"platform":        entry.Platform,
		"demo":            entry.Demo,
		"item_count":      entry.ItemCount,
		"like_count":      entry.LikeCount,
		"comment_count":   entry.CommentCount,
		"following_count": entry.FollowingCount,
		"follower_count":  entry.FollowerCount,
		"warning_count":   entry.WarningCount,
		"skipped_count":   entry.SkippedCount,
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
	m.upsertRecentPlan(path, m.planResult.Plan, "exported")
	m.updateConfig("update default plan export path", func(config *workspace.Config) {
		config.DefaultPlanExportPath = strings.TrimSpace(path)
	})
	m.appendAudit("plan_exported", planAuditFields(path, m.planResult.Plan, domain.SummarizeCleanupPlan(m.planResult.Plan)))
	m.refreshLocalData()
}

func (m *Model) recordPlanLoaded(path string, plan domain.CleanupPlan, summary domain.CleanupPlanSummary) {
	m.upsertRecentPlan(path, plan, "loaded")
	m.updateConfig("update last opened plan path", func(config *workspace.Config) {
		config.LastOpenedPlanPath = strings.TrimSpace(path)
	})
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

func (m *Model) upsertRecentPlan(path string, plan domain.CleanupPlan, operation string) {
	if m.localWorkspace == nil {
		return
	}
	summary := domain.SummarizeCleanupPlan(plan)
	entry := workspace.RecentPlan{
		ID:            plan.ID,
		Path:          strings.TrimSpace(path),
		Mode:          string(plan.Mode),
		SourceName:    plan.SourceName,
		PlanCreatedAt: plan.CreatedAt,
		LastUsedAt:    time.Now().UTC(),
		LastOperation: operation,
		ActionCounts:  actionCountsForWorkspace(summary.ActionCounts),
		StatusCounts:  statusCountsForWorkspace(summary.StatusCounts),
	}
	if err := m.localWorkspace.UpsertRecentPlan(entry); err != nil {
		m.warnLocalData("save recent plan", err)
	}
}

func (m *Model) updateConfig(action string, update func(*workspace.Config)) {
	if m.localWorkspace == nil {
		return
	}
	if err := m.localWorkspace.UpdateConfig(update); err != nil {
		m.warnLocalData(action, err)
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
	return layoutSpec(m.width, m.height).listHeight
}

func (m Model) warningListHeight() int {
	spec := layoutSpec(m.width, m.height)
	return maxInt(3, minInt(18, spec.bodyHeight-4))
}

func (m Model) planListHeight() int {
	spec := layoutSpec(m.width, m.height)
	return maxInt(3, minInt(8, spec.bodyHeight-10))
}

func (m Model) planActionListHeight() int {
	spec := layoutSpec(m.width, m.height)
	return maxInt(3, minInt(10, spec.bodyHeight-8))
}

func (m Model) localDataListHeight() int {
	spec := layoutSpec(m.width, m.height)
	return maxInt(3, minInt(8, spec.bodyHeight-8))
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
	frame        lipgloss.Style
	title        lipgloss.Style
	body         lipgloss.Style
	row          lipgloss.Style
	selected     lipgloss.Style
	muted        lipgloss.Style
	help         lipgloss.Style
	error        lipgloss.Style
	success      lipgloss.Style
	warning      lipgloss.Style
	badge        lipgloss.Style
	separator    lipgloss.Style
	footer       lipgloss.Style
	tab          lipgloss.Style
	activeTab    lipgloss.Style
	pane         lipgloss.Style
	paneTitle    lipgloss.Style
	paneSubtitle lipgloss.Style
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
			Foreground(lightDark(lipgloss.Color("#24292F"), lipgloss.Color("#E6EDF3"))),
		row: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#24292F"), lipgloss.Color("#E6EDF3"))),
		selected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#0A3069"), lipgloss.Color("#79C0FF"))),
		muted: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))),
		help: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#6E7781"), lipgloss.Color("#8B949E"))),
		error: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#B42318"), lipgloss.Color("#FFB4A8"))),
		success: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#1A7F37"), lipgloss.Color("#7EE787"))),
		warning: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#8A6100"), lipgloss.Color("#FFD479"))),
		badge: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))),
		separator: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))),
		footer: lipgloss.NewStyle().
			MarginTop(1),
		tab: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))),
		activeTab: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))),
		pane: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lightDark(lipgloss.Color("#D0D7DE"), lipgloss.Color("#30363D"))).
			Padding(0, 1),
		paneTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#24292F"), lipgloss.Color("#E6EDF3"))),
		paneSubtitle: lipgloss.NewStyle().
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
	return fixedWidthRow(
		fixedColumn{Text: activityTypeLabel(item), Width: 9},
		fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: 18},
		fixedColumn{Text: targetListLabel(item.TargetURL, item.TargetID), Width: 26},
		fixedColumn{Text: compactTime(item.OccurredAt), Width: 10},
	)
}

func (m Model) selectableItemRow(item domain.ActivityItem) string {
	marker := "[ ]"
	if m.selection.Contains(item.ID) {
		marker = "[x]"
	}
	return fixedWidthRow(
		fixedColumn{Text: marker, Width: 3},
		fixedColumn{Text: activityTypeLabel(item), Width: 9},
		fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: 18},
		fixedColumn{Text: targetListLabel(item.TargetURL, item.TargetID), Width: 26},
		fixedColumn{Text: compactTime(item.OccurredAt), Width: 10},
	)
}

func itemDetailLines(item domain.ActivityItem) []string {
	lines := []string{}
	lines = appendDetailSection(lines, detailSection("Identity",
		detailKV("ID", item.ID),
		detailKV("Platform", string(item.Platform)),
		detailKV("Type", activityTypeLabel(item)),
		detailKV("Actor", item.Actor),
	))
	lines = appendDetailSection(lines, detailSection("Target",
		detailKV("Target URL", item.TargetURL),
		detailKV("Target ID", item.TargetID),
	))
	lines = appendDetailSection(lines, detailSection("Source",
		detailKV("Source name", item.Source.Name),
		detailKV("Import ID", item.Source.ImportID),
		detailKV("Source file", item.Source.FileName),
	))
	if item.OccurredAt != nil || item.Source.ImportedAt != nil {
		lines = appendDetailSection(lines, detailSection("Timing",
			detailTimeKV("Occurred", item.OccurredAt),
			detailTimeKV("Imported", item.Source.ImportedAt),
		))
	}
	lines = appendDetailSection(lines, detailSection("Safe Metadata", safeActivityMetadataLines(item)...))
	return lines
}

func planPreviewRows(actions []domain.CleanupAction, skipped []instagram.PlanBuildSkip) []string {
	rows := make([]string, 0, len(actions)+len(skipped))
	for _, action := range actions {
		rows = append(rows, planActionListRow(action.Type, action.Status, action.TargetURL, action.TargetID, action.SourceActivityItemID))
	}
	for _, skip := range skipped {
		rows = append(rows, fixedWidthRow(
			fixedColumn{Text: "skipped", Width: 14},
			fixedColumn{Text: emptyFallback(skip.Reason, "unsupported"), Width: 12},
			fixedColumn{Text: emptyFallback(skip.TargetRef, "-"), Width: 26},
			fixedColumn{Text: emptyFallback(skip.SourceActivityItemID, "-"), Width: 16},
		))
	}
	return rows
}

func planActionRow(action domain.CleanupAction) string {
	return planActionListRow(action.Type, action.Status, action.TargetURL, action.TargetID, action.SourceActivityItemID)
}

func planActionDetailLines(action domain.CleanupAction) []string {
	lines := []string{}
	lines = appendDetailSection(lines, detailSection("Identity",
		detailKV("ID", action.ID),
		detailKV("Platform", string(action.Platform)),
		detailKV("Type", string(action.Type)),
		detailKV("Status", string(action.Status)),
	))
	lines = appendDetailSection(lines, detailSection("Target",
		detailKV("Target URL", action.TargetURL),
		detailKV("Target ID", action.TargetID),
	))
	lines = appendDetailSection(lines, detailSection("Source",
		detailKV("Source activity item ID", action.SourceActivityItemID),
	))
	if !action.CreatedAt.IsZero() {
		lines = appendDetailSection(lines, detailSection("Timing",
			detailKV("Created at", action.CreatedAt.Format(time.RFC3339)),
		))
	}
	return lines
}

func planActionListRow(actionType domain.ActionType, status domain.ActionStatus, targetURL, targetID, sourceID string) string {
	return fixedWidthRow(
		fixedColumn{Text: string(actionType), Width: 14},
		fixedColumn{Text: string(status), Width: 9},
		fixedColumn{Text: targetListLabel(targetURL, targetID), Width: 26},
		fixedColumn{Text: shortID(sourceID), Width: 16},
	)
}

func actionRowAnchor(action domain.CleanupAction) string {
	return string(action.Type) + " " + string(action.Status)
}

func activityTypeLabel(item domain.ActivityItem) string {
	if item.Type == domain.ItemTypeFollow {
		if strings.EqualFold(strings.TrimSpace(item.Metadata["relationship"]), "follower") {
			return "follower"
		}
		return "following"
	}
	return string(item.Type)
}

func targetListLabel(targetURL, targetID string) string {
	if path := pathLikeTarget(targetURL); path != "" {
		return path
	}
	return emptyFallback(targetID, "-")
}

func pathLikeTarget(targetURL string) string {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return ""
	}
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" || path == "/" {
		return ""
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	if path == "" {
		return ""
	}
	return path
}

func compactTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	return value.UTC().Format("2006-01-02")
}

func detailKV(key, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return key + ": " + value
}

func detailTimeKV(key string, value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return key + ": " + value.UTC().Format(time.RFC3339)
}

func safeActivityMetadataLines(item domain.ActivityItem) []string {
	lines := []string{}
	if item.Text != nil && item.Text.Hash != "" {
		lines = append(lines, "Safe text hash: "+item.Text.Hash)
	}
	lines = append(lines, safeStringMapLines("Metadata", item.Metadata)...)
	lines = append(lines, safeStringMapLines("Source metadata", item.Source.Metadata)...)
	return lines
}

func safeStringMapLines(label string, values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	if len(values) > 4 {
		return []string{fmt.Sprintf("%s entries: %d", label, len(values))}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", label, key, value))
	}
	return lines
}

func shortID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return truncateMiddle(value, 16)
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

func (m Model) statusCountLines(counts map[domain.ActionStatus]int) []string {
	statuses := []domain.ActionStatus{
		domain.ActionStatusPending,
		domain.ActionStatusRunning,
		domain.ActionStatusDone,
		domain.ActionStatusFailed,
		domain.ActionStatusSkipped,
	}
	lines := make([]string, 0, len(statuses))
	for _, status := range statuses {
		lines = append(lines, m.styles.body.Render(fmt.Sprintf("%s: %d", status, counts[status])))
	}
	return lines
}

func (m Model) filterSummaryLines() []string {
	if !m.itemFilter.Active() {
		return []string{m.styles.body.Render("Filters: off")}
	}
	lines := []string{m.styles.body.Render("Filters: active")}
	if len(m.itemFilter.IncludeTypes) > 0 {
		types := make([]string, 0, len(m.itemFilter.IncludeTypes))
		for filterType, included := range m.itemFilter.IncludeTypes {
			if included {
				types = append(types, string(filterType))
			}
		}
		sort.Strings(types)
		if len(types) > 0 {
			lines = append(lines, m.styles.body.Render("Types: "+strings.Join(types, ", ")))
		}
	}
	if m.itemFilter.ActorContains != "" {
		lines = append(lines, m.styles.body.Render("Actor contains: "+m.itemFilter.ActorContains))
	}
	if m.itemFilter.TargetContains != "" {
		lines = append(lines, m.styles.body.Render("Target contains: "+m.itemFilter.TargetContains))
	}
	if m.itemFilter.OlderThan != nil {
		lines = append(lines, m.styles.body.Render("Older than: "+m.itemFilter.OlderThan.UTC().Format("2006-01-02")))
	}
	if m.itemFilter.NewerThan != nil {
		lines = append(lines, m.styles.body.Render("Newer than: "+m.itemFilter.NewerThan.UTC().Format("2006-01-02")))
	}
	return lines
}

func (m Model) selectionNextAction(selected int) string {
	if selected == 0 {
		if len(m.visibleItems()) == 0 {
			return "Clear filters or return to parsed items."
		}
		return "Select visible items or return to parsed items."
	}
	return "Generate a dry-run plan."
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

func (m Model) defaultPlanPathValue() string {
	path := strings.TrimSpace(m.localConfig.DefaultPlanExportPath)
	if path != "" {
		return path
	}
	return defaultPlanExportPath
}

func (m Model) loadPlanPathValue() string {
	path := strings.TrimSpace(m.localConfig.LastOpenedPlanPath)
	if path != "" {
		return path
	}
	return m.defaultPlanPathValue()
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
		fmt.Sprintf("Demo: %t", entry.Demo),
		fmt.Sprintf("Total items: %d", entry.ItemCount),
		fmt.Sprintf("Likes: %d", entry.LikeCount),
		fmt.Sprintf("Comments: %d", entry.CommentCount),
		fmt.Sprintf("Following: %d", entry.FollowingCount),
		fmt.Sprintf("Followers: %d", entry.FollowerCount),
		fmt.Sprintf("Skipped or unknown: %d", entry.SkippedCount),
		fmt.Sprintf("Warnings: %d", entry.WarningCount),
	}
	return lines
}

func recentPlanRow(entry workspace.RecentPlan) string {
	return fmt.Sprintf(
		"%s | %s | %s | actions %d",
		formatPlanTime(entry.LastUsedAt),
		emptyFallback(entry.LastOperation, "-"),
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
		"Plan created at: " + formatPlanTime(entry.PlanCreatedAt),
		"Last used at: " + formatPlanTime(entry.LastUsedAt),
		"Last operation: " + emptyFallback(entry.LastOperation, "-"),
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

func selectKeyPress() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

func menuIndexAtY(content string, y int, items []string) int {
	line := renderedLineAt(content, y)
	if line == "" {
		return -1
	}
	for index, item := range items {
		if strings.Contains(line, item) {
			return index
		}
	}
	return -1
}

func rowOrdinalAtY(content string, y int, matches func(string) bool) int {
	lines := strings.Split(content, "\n")
	if y < 0 || y >= len(lines) || !matches(lines[y]) {
		return -1
	}
	ordinal := 0
	for i := 0; i < y; i++ {
		if matches(lines[i]) {
			ordinal++
		}
	}
	return ordinal
}

func renderedLineAt(content string, y int) string {
	lines := strings.Split(content, "\n")
	if y < 0 || y >= len(lines) {
		return ""
	}
	return lines[y]
}

func isSelectionRowLine(line string) bool {
	return strings.Contains(line, "[ ]") || strings.Contains(line, "[x]")
}

func lineMatchesAnyAnchor(anchors []string) func(string) bool {
	normalized := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		anchor = normalizeAnchor(anchor)
		if anchor != "" {
			normalized = append(normalized, anchor)
		}
	}
	return func(line string) bool {
		line = normalizeAnchor(line)
		if line == "" {
			return false
		}
		for _, anchor := range normalized {
			if strings.Contains(line, anchor) {
				return true
			}
		}
		return false
	}
}

func normalizeAnchor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "\t", "")
	return value
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
