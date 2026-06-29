package tui

import (
	"context"
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
	"github.com/itsmeares/vanish/internal/reddit"
	"github.com/itsmeares/vanish/internal/secretstore"
	"github.com/itsmeares/vanish/internal/workspace"
)

type screen int

const (
	screenHome screen = iota
	screenPlatformDetail
	screenInstagramExportGuide
	screenRedditNotes
	screenRedditConnect
	screenRedditAuthCode
	screenRedditBusy
	screenImportPath
	screenImporting
	screenImportResult
	screenItemsBrowser
	screenReviewEmpty
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
	resultViewItems = iota
	resultViewWarnings
	resultReviewSelection
	resultBackHome
)

const (
	filterRowLike = iota
	filterRowComment
	filterRowPost
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

type itemBrowserFocus int

const (
	itemFocusList itemBrowserFocus = iota
	itemFocusActions
)

const filterEditNone = -1

var resultMenuItems = []string{
	"View parsed items",
	"View warnings",
	"Review selection",
	"Back home",
}

const (
	redditConnectEnterCode = iota
	redditConnectScan
	redditConnectAllowFileFallback
	redditConnectForgetLocal
	redditConnectRevoke
	redditConnectBack
)

var redditConnectMenuItems = []string{
	"Enter returned code",
	"Scan supported activity",
	"Allow local token file fallback",
	"Forget local metadata",
	"Disconnect and revoke access",
	"Back",
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
	parsedActionToggle = iota
	parsedActionReviewSelection
	parsedActionGeneratePlan
	parsedActionBack
)

var parsedItemActionItems = []string{
	"Toggle selected",
	"Review selection",
	"Generate dry-run plan",
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

type hitKind int

const (
	hitNone hitKind = iota
	hitTab
	hitHomeAction
	hitPlatformAction
	hitImportPickerRow
	hitImportResultAction
	hitParsedItemRow
	hitParsedAction
	hitSelectionAction
	hitSelectedItemRow
	hitPlanPreviewAction
	hitLoadedPlanAction
	hitLoadedPlanRow
	hitFilterRow
	hitWarningRow
	hitLocalDataAction
	hitRecentImportRow
	hitRecentPlanRow
	hitAuditRow
	hitWipeAction
	hitQuitAction
)

type hitTarget struct {
	Kind  hitKind
	Index int
	Label string
}

type hitBox struct {
	X      int
	Y      int
	Width  int
	Height int
	Target hitTarget
}

type importPickerEntry struct {
	Name     string
	Path     string
	Kind     string
	Parent   bool
	Dir      bool
	Zip      bool
	Disabled bool
}

// Model is the central state for a Bubble Tea app.
//
// A struct groups related fields together. Here it stores the current screen,
// terminal dimensions, styles, and reusable Bubbles components. Bubble Tea
// passes this value through Init, Update, and View as the app runs.
type Model struct {
	current              screen
	width                int
	height               int
	styles               styles
	keys                 keyMap
	help                 help.Model
	localWorkspace       *workspace.Workspace
	planPathInput        textinput.Model
	redditCodeInput      textinput.Model
	filterActorInput     textinput.Model
	filterTargetInput    textinput.Model
	filterOlderInput     textinput.Model
	filterNewerInput     textinput.Model
	spinner              spinner.Model
	hoverTarget          hitTarget
	hitBoxes             []hitBox
	importPickerDir      string
	importPickerEntries  []importPickerEntry
	importPickerCursor   int
	importPickerOffset   int
	importPickerError    string
	importSource         string
	importPlatform       domain.PlatformName
	importResult         activityResult
	importErr            error
	itemFilter           domain.ActivityItemFilter
	selection            domain.ActivitySelection
	itemFocus            itemBrowserFocus
	itemActionCursor     int
	planResult           planBuildResult
	loadedPlan           domain.CleanupPlan
	loadedPlanSummary    domain.CleanupPlanSummary
	draftFilter          domain.ActivityItemFilter
	draftOlderDate       string
	draftNewerDate       string
	filterError          string
	selectionMessage     string
	planExportStatus     string
	planExportError      string
	planLoadError        string
	recentPlanError      string
	localDataStatus      string
	localDataWarning     string
	localConfig          workspace.Config
	recentImports        []workspace.RecentImport
	recentPlans          []workspace.RecentPlan
	auditEvents          []workspace.AuditEvent
	auditMalformed       int
	homeCursor           int
	selectedPlatformID   platform.PlatformID
	platformActionCursor int
	redditConnectCursor  int
	redditFileFallback   bool
	redditAuthState      string
	redditAuthURL        string
	redditStatus         string
	redditError          string
	redditBusyTitle      string
	redditBusyDetail     string
	resultCursor         int
	itemCursor           int
	itemOffset           int
	filterCursor         int
	filterEditing        int
	selectionCursor      int
	selectedCursor       int
	selectedOffset       int
	planPreviewCursor    int
	planListOffset       int
	loadedPlanCursor     int
	loadedActionCursor   int
	loadedActionOffset   int
	warningCursor        int
	warningOffset        int
	localDataCursor      int
	recentImportCursor   int
	recentImportOffset   int
	recentPlanCursor     int
	recentPlanOffset     int
	auditCursor          int
	auditOffset          int
	wipeLocalDataCursor  int
	helpReturnScreen     screen
	quitReturnScreen     screen
	quitCursor           int
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

	m := Model{
		current:            screenHome,
		styles:             newStyles(isDark),
		keys:               newKeyMap(),
		help:               helpModel,
		localWorkspace:     localWorkspace,
		planPathInput:      newPlanPathInput(),
		redditCodeInput:    newRedditCodeInput(),
		filterActorInput:   newFilterInput("username"),
		filterTargetInput:  newFilterInput("URL or ID"),
		filterOlderInput:   newFilterInput("YYYY-MM-DD"),
		filterNewerInput:   newFilterInput("YYYY-MM-DD"),
		filterEditing:      filterEditNone,
		spinner:            spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		selectedPlatformID: platform.PlatformInstagramExport,
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
	m.openImportPicker(initialImportPickerDir())
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
		m.planPathInput.SetWidth(inputWidth(msg.Width))
		m.redditCodeInput.SetWidth(inputWidth(msg.Width))
		m.setFilterInputWidths(inputWidth(msg.Width))
		m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(m.visibleItems()), m.parsedItemsViewport().VisibleRows)
		m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(m.selectedItems()), m.itemListHeight())
		m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
		m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
		m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
		m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
		m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
		m.importPickerOffset = ensureOffset(m.importPickerCursor, m.importPickerOffset, len(m.importPickerEntries), m.importPickerListHeight())

	case importFinishedMsg:
		m.importResult = activityResultFromAny(msg.result)
		m.importErr = msg.err
		m.importSource = msg.source
		m.importPlatform = msg.platform
		if m.importPlatform == "" {
			m.importPlatform = domain.PlatformInstagram
		}
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

	case redditConnectFinishedMsg:
		m.redditBusyTitle = ""
		m.redditBusyDetail = ""
		if msg.err != nil {
			m.redditError = friendlyRedditError(msg.err)
			m.redditStatus = ""
			m.current = screenRedditConnect
			return m, nil
		}
		m.redditError = ""
		m.redditStatus = fmt.Sprintf("Connected as u/%s.", msg.username)
		m.redditCodeInput.SetValue("")
		m.redditCodeInput.Blur()
		m.updateConfig("save Reddit connection metadata", func(config *workspace.Config) {
			config.Reddit = msg.metadata
		})
		m.refreshLocalData()
		m.current = screenRedditConnect
		return m, nil

	case redditScanFinishedMsg:
		m.redditBusyTitle = ""
		m.redditBusyDetail = ""
		if msg.err != nil {
			m.redditError = friendlyRedditError(msg.err)
			m.redditStatus = ""
			m.current = screenRedditConnect
			return m, nil
		}
		if msg.metadata != nil {
			m.updateConfig("update Reddit connection metadata", func(config *workspace.Config) {
				config.Reddit = msg.metadata
			})
		}
		m.refreshLocalData()
		activity := activityResultFromReddit(msg.result)
		m.importResult = activity
		m.importErr = nil
		m.importSource = redditSourceLabel()
		m.importPlatform = domain.PlatformReddit
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
		m.recordActivityFinished(activity, nil, redditSourceLabel(), domain.PlatformReddit)
		m.current = screenImportResult
		return m, nil

	case redditDisconnectFinishedMsg:
		m.redditBusyTitle = ""
		m.redditBusyDetail = ""
		if msg.err != nil {
			m.redditError = friendlyRedditError(msg.err)
			m.redditStatus = ""
			m.current = screenRedditConnect
			return m, nil
		}
		m.updateConfig("clear Reddit metadata", func(config *workspace.Config) {
			config.Reddit = nil
		})
		m.refreshLocalData()
		m.redditError = ""
		m.redditStatus = msg.message
		m.current = screenRedditConnect
		return m, nil

	case spinner.TickMsg:
		if m.current == screenImporting || m.current == screenRedditBusy {
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

	case tea.MouseMotionMsg:
		return m.updateMouseMotion(msg)

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
		case screenPlatformDetail:
			return m.updatePlatformDetail(msg)
		case screenInstagramExportGuide, screenRedditNotes:
			return m.updatePlatformStaticScreen(msg)
		case screenRedditConnect:
			return m.updateRedditConnect(msg)
		case screenRedditAuthCode:
			return m.updateRedditAuthCode(msg)
		case screenImportPath:
			return m.updateImportPath(msg)
		case screenImportResult:
			return m.updateImportResult(msg)
		case screenItemsBrowser:
			return m.updateItemsBrowser(msg)
		case screenReviewEmpty:
			return m.updateReviewEmpty(msg)
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
	platforms := m.platforms()
	switch {
	case key.Matches(msg, m.keys.up):
		m.homeCursor = moveCursor(m.homeCursor, len(platforms), -1)
	case key.Matches(msg, m.keys.down):
		m.homeCursor = moveCursor(m.homeCursor, len(platforms), 1)
	case key.Matches(msg, m.keys.selectItem):
		m.openPlatformDetail(m.homeCursor)
	}

	return m, nil
}

func (m Model) updatePlatformDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	current := m.selectedPlatform()
	switch {
	case key.Matches(msg, m.keys.up):
		m.platformActionCursor = moveCursor(m.platformActionCursor, len(current.Actions), -1)
	case key.Matches(msg, m.keys.down):
		m.platformActionCursor = moveCursor(m.platformActionCursor, len(current.Actions), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenHome
	case key.Matches(msg, m.keys.selectItem):
		return m.activatePlatformAction()
	}
	return m, nil
}

func (m Model) updatePlatformStaticScreen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.back) {
		m.current = screenPlatformDetail
	}
	return m, nil
}

func (m Model) updateRedditConnect(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	actions, disabled := m.redditConnectActions()
	switch {
	case key.Matches(msg, m.keys.up):
		m.redditConnectCursor = moveCursor(m.redditConnectCursor, len(actions), -1)
	case key.Matches(msg, m.keys.down):
		m.redditConnectCursor = moveCursor(m.redditConnectCursor, len(actions), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenPlatformDetail
	case key.Matches(msg, m.keys.selectItem):
		index := clampCursor(m.redditConnectCursor, len(actions))
		if disabled[index] {
			return m, nil
		}
		switch index {
		case redditConnectEnterCode:
			if m.ensureRedditAuthURL() {
				m.redditError = ""
				m.redditCodeInput.SetValue("")
				m.current = screenRedditAuthCode
				return m, m.redditCodeInput.Focus()
			}
		case redditConnectScan:
			return m.startRedditScan()
		case redditConnectAllowFileFallback:
			m.redditFileFallback = true
			m.redditStatus = "Local token file fallback allowed for this session if credential store is unavailable."
			m.redditError = ""
		case redditConnectForgetLocal:
			return m.startRedditDisconnect(false)
		case redditConnectRevoke:
			return m.startRedditDisconnect(true)
		case redditConnectBack:
			m.current = screenPlatformDetail
		}
	}
	return m, nil
}

func (m Model) updateRedditAuthCode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.selectItem):
		input := strings.TrimSpace(m.redditCodeInput.Value())
		if input == "" {
			m.redditError = "Paste the returned Reddit code or redirect URL."
			return m, nil
		}
		m.redditError = ""
		m.redditBusyTitle = "Connecting Reddit"
		m.redditBusyDetail = "Exchanging the code and storing the refresh token safely."
		m.current = screenRedditBusy
		return m, tea.Batch(startSpinnerCmd(m.spinner), redditConnectCmd(input, m.redditAuthState, m.redditAllowFileFallback(), m.localAppDir()))
	case key.Matches(msg, m.keys.cancel), key.Matches(msg, m.keys.back):
		m.redditCodeInput.Blur()
		m.current = screenRedditConnect
		return m, nil
	default:
		var cmd tea.Cmd
		m.redditCodeInput, cmd = m.redditCodeInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateImportPath(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.importPickerCursor = moveCursor(m.importPickerCursor, len(m.importPickerEntries), -1)
	case key.Matches(msg, m.keys.down):
		m.importPickerCursor = moveCursor(m.importPickerCursor, len(m.importPickerEntries), 1)
	case msg.Code == tea.KeyLeft || msg.Code == tea.KeyBackspace:
		m.openImportPicker(filepath.Dir(m.importPickerDir))
	case key.Matches(msg, m.keys.cancel):
		m.current = screenHome
		return m, nil
	case key.Matches(msg, m.keys.selectItem):
		return m.activateImportPickerEntry(m.importPickerCursor)
	}
	m.importPickerOffset = ensureOffset(m.importPickerCursor, m.importPickerOffset, len(m.importPickerEntries), m.importPickerListHeight())
	return m, nil
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
			m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.parsedItemsViewport().VisibleRows)
			m.itemFocus = itemFocusList
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
	case msg.Code == tea.KeyTab:
		if m.itemFocus == itemFocusActions {
			m.itemFocus = itemFocusList
		} else {
			m.itemFocus = itemFocusActions
			m.itemActionCursor = m.clampParsedItemActionCursor(m.itemActionCursor)
		}
	case msg.Code == tea.KeyPgUp:
		if m.itemFocus == itemFocusList {
			m.pageItems(-1)
		}
	case msg.Code == tea.KeyPgDown:
		if m.itemFocus == itemFocusList {
			m.pageItems(1)
		}
	case key.Matches(msg, m.keys.up):
		if m.itemFocus == itemFocusActions {
			m.itemActionCursor = m.moveParsedItemActionCursor(-1)
		} else {
			m.itemCursor = moveCursor(m.itemCursor, len(items), -1)
		}
	case key.Matches(msg, m.keys.down):
		if m.itemFocus == itemFocusActions {
			m.itemActionCursor = m.moveParsedItemActionCursor(1)
		} else {
			m.itemCursor = moveCursor(m.itemCursor, len(items), 1)
		}
	case key.Matches(msg, m.keys.filter):
		m.beginFilterDraft()
		m.current = screenFilters
	case key.Matches(msg, m.keys.selectItem), key.Matches(msg, m.keys.toggleSelection):
		if m.itemFocus == itemFocusActions {
			return m.activateParsedItemAction()
		}
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
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.parsedItemsViewport().VisibleRows)
	return m, nil
}

func (m Model) updateReviewEmpty(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.back) {
		m.current = screenHome
	}
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
		case filterRowPost:
			m.toggleDraftType(domain.ActivityFilterPost)
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
			m.generatePlanFromSelection()
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

func (m *Model) generatePlanFromSelection() {
	selected := m.selectedItems()
	if len(selected) == 0 {
		m.selectionMessage = "Select at least one item before generating a plan."
		return
	}
	req := platform.BuildPlanRequest{
		Platform:   m.currentActivityPlatform(selected),
		SourceName: emptyFallback(m.importSource, m.activitySourceFallback()),
		CreatedAt:  time.Now().UTC(),
		Items:      selected,
	}
	result, err := m.buildCleanupPlan(req)
	if err != nil {
		m.selectionMessage = err.Error()
		return
	}
	m.planResult = result
	m.recordPlanGenerated(result)
	m.planPreviewCursor = 0
	m.planListOffset = 0
	m.planExportStatus = ""
	m.planExportError = ""
	m.planPathInput.SetValue(m.defaultPlanPathValue())
	m.current = screenPlanPreview
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
	_, hitBoxes := m.renderView()
	m.hitBoxes = hitBoxes
	x, y := normalizedMousePoint(mouse)
	target := hitTargetAt(hitBoxes, x, y)
	m.hoverTarget = target
	if target.Kind == hitTab {
		return m.activateTab(target.Label)
	}

	switch m.current {
	case screenHome:
		if target.Kind == hitHomeAction {
			m.homeCursor = target.Index
			return m.updateHome(selectKeyPress())
		}
	case screenPlatformDetail:
		if target.Kind == hitPlatformAction {
			m.platformActionCursor = target.Index
			return m.updatePlatformDetail(selectKeyPress())
		}
	case screenRedditConnect:
		if target.Kind == hitPlatformAction {
			m.redditConnectCursor = target.Index
			return m.updateRedditConnect(selectKeyPress())
		}
	case screenImportPath:
		if target.Kind == hitImportPickerRow {
			m.importPickerCursor = target.Index
			m.importPickerOffset = ensureOffset(m.importPickerCursor, m.importPickerOffset, len(m.importPickerEntries), m.importPickerListHeight())
			return m.activateImportPickerEntry(target.Index)
		}
	case screenImportResult:
		if m.importErr != nil {
			return m, nil
		}
		if target.Kind == hitImportResultAction {
			m.resultCursor = target.Index
			return m.updateImportResult(selectKeyPress())
		}
	case screenItemsBrowser:
		if target.Kind == hitParsedAction {
			m.itemFocus = itemFocusActions
			m.itemActionCursor = target.Index
			return m.activateParsedItemAction()
		}
		return m.updateItemListClick(target)
	case screenSelectionSummary:
		if target.Kind == hitSelectionAction {
			m.selectionCursor = target.Index
			m.selectionMessage = ""
			return m.updateSelectionSummary(selectKeyPress())
		}
	case screenSelectedItems:
		m.updateSelectedItemListClick(target)
	case screenPlanPreview:
		if target.Kind == hitPlanPreviewAction {
			m.planPreviewCursor = target.Index
			return m.updatePlanPreview(selectKeyPress())
		}
	case screenLoadedPlanSummary:
		if target.Kind == hitLoadedPlanAction {
			m.loadedPlanCursor = target.Index
			return m.updateLoadedPlanSummary(selectKeyPress())
		}
	case screenLoadedPlanActions:
		m.updatePlanActionListClick(target)
	case screenFilters:
		if m.filterEditing == filterEditNone {
			if target.Kind == hitFilterRow {
				m.filterCursor = target.Index
				return m.updateFilters(selectKeyPress())
			}
		}
	case screenWarnings:
		m.updateWarningListClick(target)
	case screenLocalDataOverview:
		if target.Kind == hitLocalDataAction {
			m.localDataCursor = target.Index
			return m.updateLocalDataOverview(selectKeyPress())
		}
	case screenRecentImports:
		m.updateRecentImportListClick(target)
	case screenRecentPlans:
		m.updateRecentPlanListClick(target)
	case screenAuditLog:
		m.updateAuditListClick(target)
	case screenWipeLocalDataConfirm:
		if target.Kind == hitWipeAction {
			if target.Index == m.wipeLocalDataCursor {
				return m.updateWipeLocalDataConfirm(selectKeyPress())
			}
			m.wipeLocalDataCursor = target.Index
		}
	case screenQuitConfirm:
		if target.Kind == hitQuitAction {
			if target.Index == m.quitCursor {
				return m.updateQuitConfirm(selectKeyPress())
			}
			m.quitCursor = target.Index
		}
	}

	return m, nil
}

func (m Model) updateMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	_, hitBoxes := m.renderView()
	m.hitBoxes = hitBoxes
	x, y := normalizedMousePoint(mouse)
	m.hoverTarget = hitTargetAt(hitBoxes, x, y)
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
	case screenImportPath:
		m.importPickerCursor = moveCursor(m.importPickerCursor, len(m.importPickerEntries), delta)
		m.importPickerOffset = ensureOffset(m.importPickerCursor, m.importPickerOffset, len(m.importPickerEntries), m.importPickerListHeight())
	case screenItemsBrowser:
		items := m.visibleItems()
		m.itemCursor = moveCursor(m.itemCursor, len(items), delta)
		m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.parsedItemsViewport().VisibleRows)
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

func (m Model) updateItemListClick(target hitTarget) (tea.Model, tea.Cmd) {
	items := m.visibleItems()
	index := target.Index
	if target.Kind != hitParsedItemRow || index < 0 || index >= len(items) {
		return m, nil
	}
	if index == clampCursor(m.itemCursor, len(items)) {
		m.itemFocus = itemFocusList
		return m.updateItemsBrowser(selectKeyPress())
	}
	m.itemFocus = itemFocusList
	m.itemCursor = index
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.parsedItemsViewport().VisibleRows)
	return m, nil
}

func (m *Model) updateSelectedItemListClick(target hitTarget) {
	items := m.selectedItems()
	index := target.Index
	if target.Kind != hitSelectedItemRow || index < 0 || index >= len(items) {
		return
	}
	m.selectedCursor = index
	m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(items), m.itemListHeight())
}

func (m *Model) updatePlanActionListClick(target hitTarget) {
	actions := m.loadedPlan.Actions
	index := target.Index
	if target.Kind != hitLoadedPlanRow || index < 0 || index >= len(actions) {
		return
	}
	m.loadedActionCursor = index
	m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(actions), m.planActionListHeight())
}

func (m *Model) updateWarningListClick(target hitTarget) {
	index := target.Index
	if target.Kind != hitWarningRow || index < 0 || index >= len(m.importResult.Warnings) {
		return
	}
	m.warningCursor = index
	m.warningOffset = ensureOffset(m.warningCursor, m.warningOffset, len(m.importResult.Warnings), m.warningListHeight())
}

func (m *Model) updateRecentImportListClick(target hitTarget) {
	index := target.Index
	if target.Kind != hitRecentImportRow || index < 0 || index >= len(m.recentImports) {
		return
	}
	m.recentImportCursor = index
	m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
}

func (m *Model) updateRecentPlanListClick(target hitTarget) {
	index := target.Index
	if target.Kind != hitRecentPlanRow || index < 0 || index >= len(m.recentPlans) {
		return
	}
	m.recentPlanError = ""
	m.recentPlanCursor = index
	m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
}

func (m *Model) updateAuditListClick(target hitTarget) {
	index := target.Index
	if target.Kind != hitAuditRow || index < 0 || index >= len(m.auditEvents) {
		return
	}
	m.auditCursor = index
	m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
}

// View renders current model as terminal content.
func (m Model) View() tea.View {
	view, _ := m.renderView()
	return view
}

func (m Model) renderView() (tea.View, []hitBox) {
	content := m.renderContent()
	view := tea.NewView(content)
	// Bubble Tea v2.0.7 models full-screen and mouse behavior on tea.View.
	// Mouse coordinates are zero-based within this same terminal frame.
	view.AltScreen = true
	view.MouseMode = tea.MouseModeAllMotion
	return view, m.hitBoxesForContent(content)
}

func (m Model) renderContent() string {
	var content string
	switch m.current {
	case screenHome:
		content = m.homeView()
	case screenPlatformDetail:
		content = m.platformDetailView()
	case screenInstagramExportGuide:
		content = m.instagramExportGuideView()
	case screenRedditNotes:
		content = m.redditNotesView()
	case screenRedditConnect:
		content = m.redditConnectView()
	case screenRedditAuthCode:
		content = m.redditAuthCodeView()
	case screenRedditBusy:
		content = m.redditBusyView()
	case screenImportPath:
		content = m.importPathView()
	case screenImporting:
		content = m.importingView()
	case screenImportResult:
		content = m.importResultView()
	case screenItemsBrowser:
		content = m.itemsBrowserView()
	case screenReviewEmpty:
		content = m.reviewEmptyView()
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
	return content
}

func (m Model) homeView() string {
	spec := layoutSpec(m.width, m.height)
	platforms := m.platforms()
	menu := append([]string{""}, m.menuRows(platformLabels(platforms), m.homeCursor, spec.sidebarWidth, hitHomeAction)...)
	detailTitle, detailLines := m.homeDetail(spec.detailWidth)

	body := m.twoPane(
		spec,
		"Platforms", "Choose a platform", menu,
		detailTitle, "", detailLines,
	)
	return m.appShell("Home", body, m.footer(footerHome))
}

func (m Model) homeDetail(width int) (string, []string) {
	platforms := m.platforms()
	if len(platforms) == 0 {
		return "No platforms", []string{m.emptyState("No platforms are registered.")}
	}
	current := platforms[clampCursor(m.homeCursor, len(platforms))]
	lines := []string{
		m.styles.body.Render(current.Summary),
		"",
	}
	lines = append(lines, m.keyValueRows([]keyValue{
		{Key: "Status", Value: string(current.Status)},
	})...)
	lines = append(lines, "", m.styles.separator.Render("Capabilities"))
	for _, capability := range current.Capabilities {
		lines = append(lines, m.styles.body.Render(platformCapabilityLine(capability, maxInt(12, width-4))))
	}
	lines = append(lines, "", m.styles.muted.Render("Enter opens actions and details."))
	return current.Name, lines
}

func (m Model) platformDetailView() string {
	current := m.selectedPlatform()
	if current.ID == "" {
		return m.singlePaneFooter("Platform", "", []string{m.emptyState("No platform selected.")}, m.footer(footerEmpty))
	}
	lines := m.platformDetailLines(current)
	return m.singlePaneFooter(current.Name, "Platform detail", lines, m.footer(footerActionMenu))
}

func (m Model) platformDetailLines(current platform.Platform) []string {
	actionLabels, disabled := platformActionRows(current.Actions)
	lines := []string{m.styles.separator.Render("Actions")}
	lines = append(lines, m.menuRowsWithDisabled(actionLabels, disabled, m.platformActionCursor, layoutSpec(m.width, m.height).contentWidth, hitPlatformAction)...)
	if len(current.Actions) > 0 {
		action := current.Actions[clampCursor(m.platformActionCursor, len(current.Actions))]
		if action.Disabled && strings.TrimSpace(action.Reason) != "" {
			lines = append(lines, m.notice("warning", action.Reason))
		}
	}

	lines = append(lines, "", m.styles.separator.Render("Status"))
	lines = append(lines, m.styles.body.Render(fmt.Sprintf("%s - %s", current.Status, current.Summary)))

	lines = append(lines, "", m.styles.separator.Render("Capabilities"))
	for _, capability := range current.Capabilities {
		lines = append(lines, m.styles.body.Render(platformCapabilityLine(capability, layoutSpec(m.width, m.height).contentWidth-4)))
	}

	lines = append(lines, "", m.styles.separator.Render("Notes / Guide"))
	if len(current.Notes) > 0 {
		lines = append(lines, m.styles.body.Render(current.Notes[0]))
	}
	return lines
}

func (m Model) instagramExportGuideView() string {
	lines := []string{
		m.styles.separator.Render("How to get your Instagram export"),
		m.styles.body.Render("1. Open Instagram Accounts Center."),
		m.styles.body.Render("2. Go to Your information and permissions."),
		m.styles.body.Render("3. Choose Download your information."),
		m.styles.body.Render("4. Select your Instagram account."),
		m.styles.body.Render("5. Request download in JSON format."),
		m.styles.body.Render("6. Download the ZIP when Instagram prepares it."),
		m.styles.body.Render("7. Import that ZIP in Vanish."),
		"",
		m.styles.body.Render("Instagram may rename these menus. Look for Download your information or a similar data export option."),
		"",
		m.styles.body.Render("Vanish reads the local ZIP only."),
		m.styles.body.Render("Vanish does not contact Instagram."),
		m.styles.body.Render("Vanish does not apply account changes."),
	}
	return m.singlePaneFooter("Instagram Export Guide", "Static local guide", lines, m.footer(footerEmpty))
}

func (m Model) redditNotesView() string {
	lines := []string{
		m.styles.body.Render("Official API planner prototype targets v0.5."),
		m.styles.body.Render("OAuth/API, own comments/posts scan, and dry-run planning foundations exist."),
		m.styles.body.Render("The TUI can connect with manual OAuth, scan own comments/posts, and build dry-run plans."),
		m.styles.body.Render("No Reddit content mutation, scraping, browser automation, password collection, cookie paste, or session paste exists."),
		"",
		m.styles.separator.Render("Implementation notes"),
	}
	for _, note := range reddit.Platform().Notes {
		lines = append(lines, m.styles.body.Render(note))
	}
	return m.singlePaneFooter("Reddit Notes", "Prototype foundation", lines, m.footer(footerEmpty))
}

func (m Model) redditConnectView() string {
	spec := layoutSpec(m.width, m.height)
	status := m.redditConnectionRows(spec.detailWidth)
	actions, disabled := m.redditConnectActions()
	actionLines := m.menuRowsWithDisabled(actions, disabled, m.redditConnectCursor, spec.sidebarWidth, hitPlatformAction)

	body := m.twoPane(
		spec,
		"Reddit", "Connect and scan", actionLines,
		"Connection", "Official API prototype", status,
	)
	return m.appShell("Reddit", body, m.footer(footerActionMenu))
}

func (m Model) redditAuthCodeView() string {
	lines := []string{
		m.styles.body.Render("Open this Reddit authorization URL in your browser:"),
		m.styles.muted.Render(m.redditAuthURL),
		"",
		m.styles.body.Render("Paste the returned code or full redirect URL."),
		m.redditCodeInput.View(),
		"",
		m.styles.muted.Render("Requested scopes: identity history"),
		m.styles.muted.Render("No password, cookie, session, or browser automation is used."),
	}
	if strings.TrimSpace(m.redditError) != "" {
		lines = append(lines, "", m.notice("error", m.redditError))
	}
	return m.singlePaneFooter("Reddit OAuth", "Manual installed-app flow", lines, m.footer(footerForm))
}

func (m Model) redditBusyView() string {
	title := emptyFallback(m.redditBusyTitle, "Reddit")
	detail := emptyFallback(m.redditBusyDetail, "Working with Reddit official API.")
	lines := []string{
		m.styles.body.Render(fmt.Sprintf("%s %s", m.spinner.View(), detail)),
		m.styles.muted.Render("No Reddit content changes are performed."),
	}
	return m.singlePaneFooter(title, "Official API prototype", lines, m.footer(footerBusy))
}

func (m Model) platforms() []platform.Platform {
	return platform.NewRegistry(
		instagram.Platform(),
		reddit.Platform(),
	).List()
}

func (m Model) selectedPlatform() platform.Platform {
	registry := platform.NewRegistry(
		instagram.Platform(),
		reddit.Platform(),
	)
	if current, ok := registry.Get(m.selectedPlatformID); ok {
		return current
	}
	platforms := registry.List()
	if len(platforms) == 0 {
		return platform.Platform{}
	}
	current := platforms[clampCursor(m.homeCursor, len(platforms))]
	return current
}

func platformLabels(platforms []platform.Platform) []string {
	labels := make([]string, 0, len(platforms))
	for _, current := range platforms {
		labels = append(labels, current.Name)
	}
	return labels
}

func platformActionRows(actions []platform.PlatformAction) ([]string, map[int]bool) {
	rows := make([]string, 0, len(actions))
	disabled := make(map[int]bool)
	for i, action := range actions {
		rows = append(rows, action.Label)
		if action.Disabled {
			disabled[i] = true
		}
	}
	if len(disabled) == 0 {
		disabled = nil
	}
	return rows, disabled
}

func platformCapabilityLine(capability platform.Capability, width int) string {
	line := fmt.Sprintf("%s: %s - %s", capability.Label, capability.Support, capability.Description)
	return truncateEnd(line, maxInt(8, width))
}

func (m Model) importPathView() string {
	spec := layoutSpec(m.width, m.height)
	listWidth, detailWidth := twoPaneWidths(spec, "Import ZIP")
	visibleRows := m.importPickerListHeight()
	cursor := clampCursor(m.importPickerCursor, len(m.importPickerEntries))
	offset := ensureOffset(cursor, m.importPickerOffset, len(m.importPickerEntries), visibleRows)

	listLines := []string{
		m.styles.muted.Render(truncateMiddle(emptyFallback(m.importPickerDir, "."), maxInt(10, listWidth-4))),
		"",
	}
	if strings.TrimSpace(m.importPickerError) != "" {
		listLines = append(listLines, m.notice("error", m.importPickerError), "")
	}
	if len(m.importPickerEntries) == 0 {
		listLines = append(listLines, m.emptyState("No files in this directory."))
	} else {
		rows := make([]string, 0, len(m.importPickerEntries))
		disabled := make(map[int]bool, len(m.importPickerEntries))
		for i, entry := range m.importPickerEntries {
			rows = append(rows, importPickerRow(entry))
			disabled[i] = entry.Disabled
		}
		listLines = append(listLines, m.tableRowsWithDisabled(rows, disabled, cursor, offset, visibleRows, listWidth, hitImportPickerRow)...)
	}

	detailLines := []string{
		m.styles.body.Render("Choose a local Instagram export ZIP."),
		m.styles.muted.Render("Directories open in place; non-ZIP files are disabled."),
	}
	if len(m.importPickerEntries) > 0 {
		detailLines = append(detailLines, "")
		detailLines = append(detailLines, m.detailRows(importPickerDetailLines(m.importPickerEntries[cursor]), detailWidth)...)
	}

	body := m.twoPane(
		spec,
		"Import ZIP", "Local file picker", listLines,
		"Selection", "", detailLines,
	)
	return m.appShell("Import Instagram Export", body, m.footer(footerImportPicker))
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
	return m.singlePaneFooter("Importing", "Reading local files only", lines, m.footer(footerBusy))
}

func (m Model) importResultView() string {
	if m.importErr != nil {
		lines := []string{
			m.notice("error", m.importErr.Error()),
			m.styles.muted.Render(m.activityFailureHint()),
			"",
			m.styles.muted.Render(truncateMiddle(m.importSource, layoutSpec(m.width, m.height).contentWidth-4)),
		}
		lines = append(lines, m.localDataMessages()...)
		return m.singlePaneFooter(m.activityFailedTitle(), "No data was imported", lines, m.footer(footerEmpty))
	}

	spec := layoutSpec(m.width, m.height)
	summary := m.importResult.Summary
	summaryLines := m.dashboardSections(
		spec.detailWidth,
		m.section("Source", []string{
			m.styles.body.Render(emptyFallback(m.importSource, "instagram export")),
		}),
		m.section("Parsed Items", m.keyValueRows([]keyValue{
			{Key: "Total", Value: compactCount(summary.Total)},
			{Key: "Likes", Value: compactCount(summary.Likes)},
			{Key: "Comments", Value: compactCount(summary.Comments)},
			{Key: "Posts", Value: compactCount(summary.Posts)},
			{Key: "Following", Value: compactCount(summary.Following)},
			{Key: "Followers", Value: compactCount(summary.Followers)},
		})),
		m.section("Import Notes", m.keyValueRows([]keyValue{
			{Key: "Skipped or unknown", Value: compactCount(summary.Skipped)},
			{Key: "Warnings", Value: compactCount(len(m.importResult.Warnings))},
		})),
	)
	summaryLines = append(summaryLines, m.localDataMessages()...)
	body := m.twoPane(
		spec,
		"Actions", "Next review step", m.menuRows(resultMenuItems, m.resultCursor, spec.sidebarWidth, hitImportResultAction),
		m.activityCompleteTitle(), m.activityCompleteSubtitle(), summaryLines,
	)
	return m.appShell(m.activityCompleteTitle(), body, m.footer(footerActionMenu))
}

func (m Model) itemsBrowserView() string {
	spec := layoutSpec(m.width, m.height)
	items := m.visibleItems()
	listWidth, detailWidth := twoPaneWidths(spec, "Parsed Items")
	total := len(m.importResult.Items)
	viewport := m.parsedItemsViewport()
	visibleRows := viewport.VisibleRows
	cursor := clampCursor(m.itemCursor, len(items))
	offset := viewport.Offset

	filterStatus := "off"
	if m.itemFilter.Active() {
		filterStatus = "active"
	}
	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("%s · Matching %s/%s · Selected %s · Filters %s",
			viewport.ShowingLabel(),
			compactCount(len(items)),
			compactCount(total),
			compactCount(m.selection.Len()),
			filterStatus,
		)),
		m.styles.muted.Render(fmt.Sprintf("Page %d/%d · Source: %s", viewport.Page, viewport.Pages, emptyFallback(m.importSource, m.activitySourceFallback()))),
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
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth, hitParsedItemRow)...)
	}

	detailLines := []string{}
	if len(items) == 0 {
		detailLines = append(detailLines, m.emptyState("No items match the current filters. Clear filters or scan/import another source."))
	} else {
		detailLines = append(detailLines, m.detailRows(parsedItemDetailLines(items[cursor]), detailWidth)...)
	}
	detailLines = append(detailLines, m.parsedItemsCockpitLines(detailWidth)...)

	body := m.twoPaneFocused(
		spec,
		"Parsed Items", "Review and toggle", listLines, m.itemFocus == itemFocusList,
		"Details", "Highlighted item", detailLines, m.itemFocus == itemFocusActions,
	)
	return m.appShell("Parsed Items", body, m.footer(footerParsedItems))
}

func (m Model) parsedItemsCockpitLines(width int) []string {
	counts := m.selection.Counts(m.importResult.Items)
	lines := []string{""}
	lines = append(lines, m.warningBanner(m.selectionMessage, width)...)
	lines = append(lines, m.section("Selection", m.keyValueRows([]keyValue{
		{Key: "Selected", Value: compactCount(counts.Total)},
		{Key: "Likes", Value: compactCount(counts.Likes)},
		{Key: "Comments", Value: compactCount(counts.Comments)},
		{Key: "Posts", Value: compactCount(counts.Posts)},
		{Key: "Following", Value: compactCount(counts.Following)},
		{Key: "Followers", Value: compactCount(counts.Followers)},
	}))...)
	lines = append(lines, "")
	lines = append(lines, m.styles.separator.Render("Actions"))
	lines = append(lines, m.parsedItemActionRows(width)...)
	return lines
}

func (m Model) parsedItemActionRows(width int) []string {
	rows := make([]string, 0, len(parsedItemActionItems))
	for i, item := range parsedItemActionItems {
		enabled := m.parsedItemActionEnabled(i)
		rows = append(rows, m.controlRowState(item, rowState{
			Selected: m.itemFocus == itemFocusActions && i == m.itemActionCursor && enabled,
			Hovered:  m.hoverTarget.Kind == hitParsedAction && m.hoverTarget.Index == i,
			Disabled: !enabled,
		}, width, ""))
	}
	return rows
}

func (m Model) parsedItemActionEnabled(index int) bool {
	if index == parsedActionGeneratePlan {
		return m.selection.Len() > 0
	}
	return true
}

func (m Model) clampParsedItemActionCursor(cursor int) int {
	cursor = clampCursor(cursor, len(parsedItemActionItems))
	if m.parsedItemActionEnabled(cursor) {
		return cursor
	}
	return m.moveParsedItemActionCursorFrom(cursor, 1)
}

func (m Model) moveParsedItemActionCursor(delta int) int {
	return m.moveParsedItemActionCursorFrom(m.itemActionCursor, delta)
}

func (m Model) moveParsedItemActionCursorFrom(cursor, delta int) int {
	if len(parsedItemActionItems) == 0 {
		return 0
	}
	if delta == 0 {
		delta = 1
	}
	next := clampCursor(cursor, len(parsedItemActionItems))
	for range len(parsedItemActionItems) {
		next = moveCursor(next, len(parsedItemActionItems), delta)
		if m.parsedItemActionEnabled(next) {
			return next
		}
	}
	return clampCursor(cursor, len(parsedItemActionItems))
}

func (m Model) activateParsedItemAction() (tea.Model, tea.Cmd) {
	if !m.parsedItemActionEnabled(m.itemActionCursor) {
		m.selectionMessage = "Select at least one item before generating a plan."
		return m, nil
	}
	m.selectionMessage = ""

	switch m.itemActionCursor {
	case parsedActionToggle:
		items := m.visibleItems()
		if len(items) > 0 {
			m.selection.Toggle(items[clampCursor(m.itemCursor, len(items))].ID)
		}
	case parsedActionReviewSelection:
		m.selectionCursor = 0
		m.current = screenSelectionSummary
	case parsedActionGeneratePlan:
		m.generatePlanFromSelection()
	case parsedActionBack:
		m.current = screenImportResult
	}
	return m, nil
}

func (m Model) reviewEmptyView() string {
	lines := []string{
		m.styles.body.Render("Import a local Instagram export ZIP, run Demo Import, or scan Reddit first."),
		m.styles.muted.Render("Parsed items will appear here for review, filtering, and selection."),
	}
	return m.singlePaneFooter("Review", "No parsed items yet", lines, m.footer(footerEmpty))
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
			{Key: "Posts", Value: compactCount(counts.Posts)},
			{Key: "Following", Value: compactCount(counts.Following)},
			{Key: "Followers", Value: compactCount(counts.Followers)},
		})),
		m.section("Current Filters", m.filterSummaryLines()),
		m.section("Next Suggested Action", []string{m.styles.body.Render(m.selectionNextAction(counts.Total))}),
	)
	body := m.twoPane(
		spec,
		"Actions", "Selection workflow", m.menuRows(selectionMenuItems, m.selectionCursor, spec.sidebarWidth, hitSelectionAction),
		"Selection Dashboard", "Current review set", summaryLines,
	)
	return m.appShell("Selection Summary", body, m.footer(footerActionMenu))
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
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth, hitSelectedItemRow)...)
	}

	detailLines := []string{}
	if len(items) == 0 {
		detailLines = append(detailLines, m.emptyState("No item selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(itemDetailLines(items[cursor]), detailWidth)...)
	}

	body := m.twoPane(spec, "Selected Items", "Chosen cleanup candidates", listLines, "Details", "Highlighted item", detailLines)
	return m.appShell("Selected Items", body, m.footer(footerList))
}

func (m Model) planPreviewView() string {
	spec := layoutSpec(m.width, m.height)
	result := m.planResult
	summaryWidth, actionWidth := twoPaneWidths(spec, "Plan Summary")
	rows := planPreviewRows(result.Plan.Actions, result.Skipped)
	visibleRows := m.planListHeight()
	offset := ensureOffset(0, m.planListOffset, len(rows), visibleRows)

	summaryLines := m.dashboardSections(
		summaryWidth,
		m.section("Plan", m.keyValueRows([]keyValue{
			{Key: "Mode", Value: string(result.Plan.Mode)},
			{Key: "Platform", Value: string(result.Plan.Platform)},
			{Key: "Selected items", Value: compactCount(result.SelectedCount)},
		})),
		m.section("Action Counts", append(
			m.keyValueRows([]keyValue{{Key: "Supported actions", Value: compactCount(len(result.Plan.Actions))}}),
			m.planActionCountRows(result.Counts)...,
		)),
		m.section("Skipped", m.keyValueRows([]keyValue{
			{Key: "Unsupported selected items", Value: compactCount(len(result.Skipped))},
		})),
	)
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, m.menuRows(planPreviewMenuItems, m.planPreviewCursor, summaryWidth, hitPlanPreviewAction)...)

	actionLines := []string{}
	if len(rows) == 0 {
		actionLines = append(actionLines, m.emptyState("No supported actions."))
	} else {
		actionLines = append(actionLines, m.plainRows(rows, offset, visibleRows, actionWidth)...)
	}

	body := m.twoPane(spec, "Plan Summary", "Dry-run only", summaryLines, "Planned actions", "Supported and skipped", actionLines)
	return m.appShell("Dry-Run Plan Preview", body, m.footer(footerActionMenu))
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

	return m.singlePaneFooter("Export Plan JSON", "Write a local dry-run plan", lines, m.footer(footerSaveForm))
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

	return m.singlePaneFooter("Load Cleanup Plan", "Local JSON path", lines, m.footer(footerForm))
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
		"Actions", "Loaded plan", m.menuRows(loadedPlanSummaryMenuItems, m.loadedPlanCursor, spec.sidebarWidth, hitLoadedPlanAction),
		"Loaded Cleanup Plan", "Plan metadata", detailLines,
	)
	return m.appShell("Loaded Cleanup Plan", body, m.footer(footerActionMenu))
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
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth, hitLoadedPlanRow)...)
	}

	detailLines := []string{}
	if len(actions) == 0 {
		detailLines = append(detailLines, m.emptyState("No action selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(planActionDetailLines(actions[cursor]), detailWidth)...)
	}

	body := m.twoPane(spec, "Plan Actions", "Read-only dry-run actions", listLines, "Details", "Highlighted action", detailLines)
	return m.appShell("Plan Actions", body, m.footer(footerList))
}

func (m Model) filtersView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.styles.muted.Render(fmt.Sprintf("Matching: %d / %d | Filters: %s", len(m.visibleItems()), len(m.importResult.Items), activeLabel(m.itemFilter.Active()))),
		"",
	}

	if m.itemFilter.Active() {
		lines = append(lines, m.notice("warning", "Filters active"), "")
	}
	if strings.TrimSpace(m.filterError) != "" {
		lines = append(lines, m.notice("error", m.filterError), "")
	}

	rows := m.filterRows()

	for i, row := range rows {
		lines = append(lines, m.selectableLineTarget(row, i == m.filterCursor, spec.contentWidth, hitFilterRow, i))
	}

	if m.filterEditing == filterEditNone {
		return m.singlePaneFooter("Filters", "Constrain parsed items", lines, m.footer("up/down move · enter/click toggle or edit · esc back · ? help · ctrl+q quit"))
	} else {
		return m.singlePaneFooter("Filters", "Editing filter value", lines, m.footer(footerForm))
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
		lines = append(lines, m.tableRows(warnings, cursor, offset, visibleRows, spec.contentWidth, hitWarningRow)...)
	}

	return m.singlePaneFooter("Import Warnings", "Skipped or unsupported local files", lines, m.footer(footerList))
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
		"Actions", "Local metadata", m.menuRows(localDataMenuItems, m.localDataCursor, spec.sidebarWidth, hitLocalDataAction),
		"Local Data", "Workspace overview", stats,
	)
	return m.appShell("Local Data", body, m.footer(footerActionMenu))
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
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth, hitRecentImportRow)...)
	}
	detailLines := []string{}
	if len(m.recentImports) == 0 {
		detailLines = append(detailLines, m.emptyState("No import selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(recentImportDetailLines(m.recentImports[cursor]), detailWidth)...)
	}
	body := m.twoPane(spec, "Recent Imports", "Newest first", listLines, "Details", "Highlighted import", detailLines)
	return m.appShell("Recent Imports", body, m.footer(footerList))
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
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth, hitRecentPlanRow)...)
	}
	detailLines := []string{}
	if len(m.recentPlans) == 0 {
		detailLines = append(detailLines, m.emptyState("No plan selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(recentPlanDetailLines(m.recentPlans[cursor]), detailWidth)...)
	}
	body := m.twoPane(spec, "Recent Plans", "Enter loads selected", listLines, "Details", "Highlighted plan", detailLines)
	return m.appShell("Recent Plans", body, m.footer("up/down move · enter load · click highlight · esc back · ? help · ctrl+q quit"))
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
		listLines = append(listLines, m.tableRows(rows, cursor, offset, visibleRows, listWidth, hitAuditRow)...)
	}
	detailLines := []string{}
	if len(m.auditEvents) == 0 {
		detailLines = append(detailLines, m.emptyState("No audit event selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(auditEventDetailLines(m.auditEvents[cursor]), detailWidth)...)
	}
	body := m.twoPane(spec, "Audit Log", "Local metadata events", listLines, "Details", "Highlighted event", detailLines)
	return m.appShell("Audit Log", body, m.footer(footerList))
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
	lines = append(lines, m.menuRows(wipeLocalDataMenuItems, m.wipeLocalDataCursor, spec.contentWidth, hitWipeAction)...)
	return m.singlePaneFooter("Wipe Local Data?", "Defaults to Cancel", lines, m.footer(footerConfirm))
}

func (m Model) keybindingsView() string {
	lines := []string{
		m.styles.separator.Render("Navigation"),
		m.styles.body.Render("Up/Down or j/k: move"),
		m.styles.body.Render("Enter: primary action"),
		m.styles.body.Render("Esc/Backspace: back when no text input is focused"),
		m.styles.body.Render("?: show this help"),
		m.styles.body.Render("Ctrl+Q or Ctrl+C: quit confirmation"),
		m.styles.separator.Render("Lists"),
		m.styles.body.Render("Space: toggle highlighted parsed item"),
		m.styles.body.Render("Mouse wheel: scroll highlighted list"),
		m.styles.separator.Render("Selection"),
		m.styles.body.Render("A/N: select or deselect visible items"),
		m.styles.body.Render("S: selection summary"),
		m.styles.separator.Render("Forms"),
		m.styles.body.Render("Enter: submit"),
		m.styles.body.Render("Esc: cancel"),
		m.styles.separator.Render("Plans"),
		m.styles.body.Render("Generate, preview, export, and load dry-run JSON plans."),
		m.styles.separator.Render("Notes"),
		m.styles.body.Render("Instagram import uses local files; Reddit uses explicit official API connect only."),
	}
	return m.singlePaneFooter("Help", "Keyboard reference", lines, m.footer(footerHelp))
}

func (m Model) quitConfirmView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.styles.body.Render("Your current in-memory review state will be discarded."),
		"",
	}

	lines = append(lines, m.menuRows(quitConfirmMenuItems, m.quitCursor, spec.contentWidth, hitQuitAction)...)
	return m.singlePaneFooter("Quit Vanish?", "Defaults to Cancel", lines, m.footer(footerConfirm))
}

func (m Model) resetImportState() Model {
	m.importSource = ""
	m.importPlatform = domain.PlatformInstagram
	m.importResult = activityResult{}
	m.importErr = nil
	m.resultCursor = 0
	m.itemCursor = 0
	m.itemOffset = 0
	m.itemFocus = itemFocusList
	m.itemActionCursor = 0
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
	m.planResult = planBuildResult{}
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

func (m *Model) openPlatformDetail(index int) {
	platforms := m.platforms()
	if len(platforms) == 0 {
		return
	}
	m.homeCursor = clampCursor(index, len(platforms))
	selected := platforms[m.homeCursor]
	m.selectedPlatformID = selected.ID
	m.platformActionCursor = 0
	m.current = screenPlatformDetail
}

func (m Model) activatePlatformAction() (tea.Model, tea.Cmd) {
	current := m.selectedPlatform()
	if len(current.Actions) == 0 {
		return m, nil
	}
	m.platformActionCursor = clampCursor(m.platformActionCursor, len(current.Actions))
	action := current.Actions[m.platformActionCursor]
	if action.Disabled {
		return m, nil
	}

	switch action.ID {
	case platform.ActionChooseExportZIP:
		m.current = screenImportPath
		if strings.TrimSpace(m.importPickerDir) == "" {
			m.openImportPicker(initialImportPickerDir())
		}
	case platform.ActionExportGuide:
		m.selectedPlatformID = platform.PlatformInstagramExport
		m.current = screenInstagramExportGuide
	case platform.ActionViewRecentImports:
		m.recentImportCursor = clampCursor(m.recentImportCursor, len(m.recentImports))
		m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
		m.current = screenRecentImports
	case platform.ActionDemoImport:
		m = m.resetImportState()
		m.current = screenImporting
		m.importSource = "demo instagram export"
		return m, tea.Batch(startSpinnerCmd(m.spinner), demoImportCmd())
	case platform.ActionViewIntegrationNote:
		m.selectedPlatformID = platform.PlatformReddit
		m.current = screenRedditNotes
	case platform.ActionConnectAccount:
		m.openRedditConnect("")
	case platform.ActionScanActivity:
		if !m.redditConnected() {
			m.openRedditConnect("Connect Reddit before scanning.")
			return m, nil
		}
		return m.startRedditScan()
	case platform.ActionBack:
		m.current = screenHome
	}
	return m, nil
}

func (m Model) activateTab(label string) (tea.Model, tea.Cmd) {
	if label == "" || label == m.activeTab() {
		return m, nil
	}
	switch label {
	case "Home":
		m.current = screenHome
	case "Import":
		m.current = screenImportPath
		if strings.TrimSpace(m.importPickerDir) == "" {
			m.openImportPicker(initialImportPickerDir())
		}
	case "Review":
		if m.hasImportData() {
			items := m.visibleItems()
			m.itemCursor = clampCursor(m.itemCursor, len(items))
			m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.parsedItemsViewport().VisibleRows)
			m.itemFocus = itemFocusList
			m.current = screenItemsBrowser
		} else {
			m.current = screenReviewEmpty
		}
	case "Plans":
		switch {
		case m.hasLoadedPlan():
			m.current = screenLoadedPlanSummary
		case m.hasPlanPreview():
			m.current = screenPlanPreview
		default:
			m.planPathInput.SetValue(m.loadPlanPathValue())
			m.planLoadError = ""
			m.current = screenPlanLoadPath
			return m, m.planPathInput.Focus()
		}
	case "Local":
		m.openLocalDataOverview()
	case "Help":
		if m.current != screenKeybindings {
			m.openKeybindings()
		}
	}
	return m, nil
}

func (m *Model) openLocalDataOverview() {
	m.refreshLocalData()
	m.localDataCursor = clampCursor(m.localDataCursor, len(localDataMenuItems))
	m.current = screenLocalDataOverview
}

func (m Model) hasImportData() bool {
	return len(m.importResult.Items) > 0 || m.importResult.Summary.Total > 0 || len(m.importResult.Warnings) > 0
}

func (m Model) hasLoadedPlan() bool {
	return strings.TrimSpace(m.loadedPlan.ID) != "" || len(m.loadedPlan.Actions) > 0
}

func (m Model) hasPlanPreview() bool {
	return strings.TrimSpace(m.planResult.Plan.ID) != "" || len(m.planResult.Plan.Actions) > 0 || len(m.planResult.Skipped) > 0
}

func (m Model) currentActivityPlatform(items []domain.ActivityItem) domain.PlatformName {
	if len(items) > 0 && items[0].Platform != "" {
		return items[0].Platform
	}
	if m.importPlatform != "" {
		return m.importPlatform
	}
	return domain.PlatformInstagram
}

func (m Model) buildCleanupPlan(req platform.BuildPlanRequest) (planBuildResult, error) {
	switch req.Platform {
	case domain.PlatformReddit:
		result, err := reddit.BuildCleanupPlan(req)
		if err != nil {
			return planBuildResult{}, err
		}
		return planResultFromReddit(result), nil
	default:
		result, err := instagram.BuildCleanupPlan(req)
		if err != nil {
			return planBuildResult{}, err
		}
		return planResultFromInstagram(result), nil
	}
}

func (m Model) activitySourceFallback() string {
	if m.importPlatform == domain.PlatformReddit {
		return redditSourceLabel()
	}
	return "instagram export"
}

func (m Model) activityCompleteTitle() string {
	if m.importPlatform == domain.PlatformReddit {
		return "Reddit Scan Complete"
	}
	return "Import Complete"
}

func (m Model) activityCompleteSubtitle() string {
	if m.importPlatform == domain.PlatformReddit {
		return "Official API scan"
	}
	return "Parsed local export"
}

func (m Model) activityFailedTitle() string {
	if m.importPlatform == domain.PlatformReddit {
		return "Reddit Scan Failed"
	}
	return "Import Failed"
}

func (m Model) activityFailureHint() string {
	if m.importPlatform == domain.PlatformReddit {
		return "Check the Reddit connection, client ID, and stored refresh token, then try again."
	}
	return "Check that the path points to a local Instagram export .zip, then try again."
}

func (m Model) redditConnected() bool {
	return m.localConfig.Reddit != nil && strings.TrimSpace(m.localConfig.Reddit.Username) != ""
}

func (m Model) redditAllowFileFallback() bool {
	if m.redditFileFallback {
		return true
	}
	if m.localConfig.Reddit != nil && strings.EqualFold(strings.TrimSpace(m.localConfig.Reddit.TokenStorageMode), string(secretstore.ModeFile)) {
		return true
	}
	return false
}

func (m Model) localAppDir() string {
	if m.localWorkspace == nil {
		return ""
	}
	return m.localWorkspace.Dir()
}

func (m *Model) openRedditConnect(message string) {
	m.selectedPlatformID = platform.PlatformReddit
	m.refreshLocalData()
	initialError := strings.TrimSpace(message)
	m.redditError = initialError
	m.redditStatus = ""
	m.redditConnectCursor = clampCursor(m.redditConnectCursor, len(redditConnectMenuItems))
	if !m.ensureRedditAuthURL() && initialError != "" && m.redditError != initialError {
		m.redditError = initialError + " " + m.redditError
	}
	m.current = screenRedditConnect
}

func (m *Model) ensureRedditAuthURL() bool {
	if strings.TrimSpace(m.redditAuthURL) != "" {
		return true
	}
	clientID, err := reddit.ClientIDFromEnv()
	if err != nil {
		m.redditError = "Set VANISH_REDDIT_CLIENT_ID before connecting Reddit."
		return false
	}
	oauth, err := reddit.NewOAuth(reddit.OAuthConfig{ClientID: clientID})
	if err != nil {
		m.redditError = friendlyRedditError(err)
		return false
	}
	if strings.TrimSpace(m.redditAuthState) == "" {
		m.redditAuthState = fmt.Sprintf("vanish-%d", time.Now().UnixNano())
	}
	authURL, err := oauth.AuthURL(m.redditAuthState)
	if err != nil {
		m.redditError = friendlyRedditError(err)
		return false
	}
	m.redditAuthURL = authURL
	return true
}

func (m Model) redditConnectActions() ([]string, map[int]bool) {
	connected := m.redditConnected()
	clientIDPresent := strings.TrimSpace(os.Getenv(reddit.ClientIDEnv)) != ""
	actions := append([]string(nil), redditConnectMenuItems...)
	if m.redditAllowFileFallback() {
		actions[redditConnectAllowFileFallback] = "Local token file fallback allowed"
	}
	disabled := make(map[int]bool)
	disabled[redditConnectEnterCode] = !clientIDPresent
	disabled[redditConnectScan] = !connected
	disabled[redditConnectForgetLocal] = !connected
	disabled[redditConnectRevoke] = !connected
	if m.localWorkspace == nil {
		disabled[redditConnectAllowFileFallback] = true
	}
	for index, value := range disabled {
		if !value {
			delete(disabled, index)
		}
	}
	return actions, disabled
}

func (m Model) redditConnectionRows(width int) []string {
	lines := []string{}
	if strings.TrimSpace(m.redditStatus) != "" {
		lines = append(lines, m.notice("success", m.redditStatus), "")
	}
	if strings.TrimSpace(m.redditError) != "" {
		lines = append(lines, m.notice("error", m.redditError), "")
	}
	config := m.localConfig.Reddit
	if config == nil || strings.TrimSpace(config.Username) == "" {
		lines = append(lines, m.styles.body.Render("Status: not connected"))
	} else {
		lines = append(lines, m.keyValueRows([]keyValue{
			{Key: "Status", Value: "connected"},
			{Key: "Username", Value: config.Username},
			{Key: "Scopes", Value: strings.Join(config.Scopes, " ")},
			{Key: "Token storage", Value: emptyFallback(config.TokenStorageMode, "-")},
			{Key: "Expires", Value: formatTimePtr(config.ExpiresAt)},
		})...)
	}
	clientIDStatus := "set"
	if strings.TrimSpace(os.Getenv(reddit.ClientIDEnv)) == "" {
		clientIDStatus = "missing"
	}
	lines = append(lines, "", m.styles.separator.Render("Safety"))
	lines = append(lines, m.keyValueRows([]keyValue{
		{Key: reddit.ClientIDEnv, Value: clientIDStatus},
		{Key: "Scopes", Value: "identity history"},
		{Key: "Fallback", Value: enabledLabel(m.redditAllowFileFallback())},
	})...)
	lines = append(lines,
		m.styles.muted.Render("Manual OAuth only; Vanish does not open a browser."),
		m.styles.muted.Render("Refresh token uses credential store unless fallback is explicitly allowed."),
		m.styles.muted.Render("Scanner reads own comments/posts only; saved items and votes are deferred."),
		m.styles.muted.Render(truncateMiddle(m.redditAuthURL, maxInt(24, width-4))),
	)
	return lines
}

func (m Model) startRedditScan() (tea.Model, tea.Cmd) {
	if !m.redditConnected() {
		m.openRedditConnect("Connect Reddit before scanning.")
		return m, nil
	}
	config := *m.localConfig.Reddit
	m.redditError = ""
	m.redditStatus = ""
	m.redditBusyTitle = "Scanning Reddit"
	m.redditBusyDetail = "Reading own comments and submitted posts."
	m.current = screenRedditBusy
	return m, tea.Batch(startSpinnerCmd(m.spinner), redditScanCmd(&config, m.redditAllowFileFallback(), m.localAppDir()))
}

func (m Model) startRedditDisconnect(revoke bool) (tea.Model, tea.Cmd) {
	if !m.redditConnected() {
		m.openRedditConnect("No connected Reddit account metadata found.")
		return m, nil
	}
	config := *m.localConfig.Reddit
	m.redditError = ""
	m.redditStatus = ""
	m.redditBusyTitle = "Disconnecting Reddit"
	if revoke {
		m.redditBusyDetail = "Revoking OAuth access, then clearing local metadata."
	} else {
		m.redditBusyDetail = "Clearing local metadata only."
	}
	m.current = screenRedditBusy
	return m, tea.Batch(startSpinnerCmd(m.spinner), redditDisconnectCmd(&config, revoke, m.redditAllowFileFallback(), m.localAppDir()))
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
	result := activityResultFromAny(msg.result)
	platformName := msg.platform
	if platformName == "" {
		platformName = domain.PlatformInstagram
	}
	m.recordActivityFinished(result, msg.err, msg.source, platformName)
}

func (m *Model) recordActivityFinished(result activityResult, err error, source string, platformName domain.PlatformName) {
	if err != nil {
		m.appendAudit("import_failed", map[string]any{
			"source_label": sourceLabel(source),
			"source_path":  sourcePath(source),
			"platform":     string(platformName),
			"demo":         isDemoSource(source),
			"error":        friendlyActivityError(err),
		})
		return
	}
	entry := workspace.RecentImport{
		SourceLabel:    sourceLabel(source),
		SourcePath:     sourcePath(source),
		Platform:       string(platformName),
		ImportedAt:     time.Now().UTC(),
		Demo:           isDemoSource(source),
		ItemCount:      result.Summary.Total,
		LikeCount:      result.Summary.Likes,
		CommentCount:   result.Summary.Comments,
		PostCount:      result.Summary.Posts,
		FollowingCount: result.Summary.Following,
		FollowerCount:  result.Summary.Followers,
		WarningCount:   len(result.Warnings),
		SkippedCount:   result.Summary.Skipped,
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
		"post_count":      entry.PostCount,
		"following_count": entry.FollowingCount,
		"follower_count":  entry.FollowerCount,
		"warning_count":   entry.WarningCount,
		"skipped_count":   entry.SkippedCount,
	})
	m.refreshLocalData()
}

func (m *Model) recordPlanGenerated(result planBuildResult) {
	fields := map[string]any{
		"plan_id":              result.Plan.ID,
		"mode":                 string(result.Plan.Mode),
		"source_name":          result.Plan.SourceName,
		"platform":             string(result.Plan.Platform),
		"selected_count":       result.SelectedCount,
		"action_count":         len(result.Plan.Actions),
		"skipped_count":        len(result.Skipped),
		"unlike_count":         result.Counts[domain.ActionUnlike],
		"delete_comment_count": result.Counts[domain.ActionDeleteComment] + result.Counts[domain.ActionRedditDeleteComment],
		"delete_post_count":    result.Counts[domain.ActionRedditDeletePost],
		"unfollow_count":       result.Counts[domain.ActionUnfollow],
	}
	m.appendAudit("plan_generated", fields)
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

type listViewport struct {
	VisibleRows int
	Offset      int
	Start       int
	End         int
	Page        int
	Pages       int
	Total       int
}

func (v listViewport) ShowingLabel() string {
	if v.Total == 0 {
		return "Showing 0 of 0"
	}
	return fmt.Sprintf("Showing %d-%d of %d", v.Start, v.End, v.Total)
}

func (m Model) parsedItemsViewport() listViewport {
	items := m.visibleItems()
	total := len(items)
	cursor := clampCursor(m.itemCursor, total)
	visibleRows := m.parsedItemsListHeight(total, cursor, m.itemOffset)
	offset := ensureOffset(cursor, m.itemOffset, total, visibleRows)
	end := minInt(total, offset+visibleRows)
	start := 0
	if total > 0 {
		start = offset + 1
	}
	pages := maxInt(1, (total+visibleRows-1)/visibleRows)
	page := 1
	if total > 0 {
		page = minInt(pages, offset/visibleRows+1)
	}
	return listViewport{
		VisibleRows: visibleRows,
		Offset:      offset,
		Start:       start,
		End:         end,
		Page:        page,
		Pages:       pages,
		Total:       total,
	}
}

func (m Model) parsedItemsListHeight(itemCount, cursor, offset int) int {
	spec := layoutSpec(m.width, m.height)
	bodyCapacity := paneBodyLineCapacity(twoPaneBodyHeight(spec), "Parsed Items", "Review and toggle")
	headerLines := 3
	if m.itemFilter.Active() {
		headerLines += 2
	}
	visibleRows := maxInt(1, bodyCapacity-headerLines)
	for visibleRows > 1 {
		nextOffset := ensureOffset(cursor, offset, itemCount, visibleRows)
		extras := 0
		if nextOffset > 0 {
			extras++
		}
		if nextOffset+visibleRows < itemCount {
			extras++
		}
		if headerLines+visibleRows+extras <= bodyCapacity {
			return visibleRows
		}
		visibleRows--
	}
	return visibleRows
}

func paneBodyLineCapacity(height int, title, subtitle string) int {
	innerHeight := maxInt(2, maxInt(height, 4)-2)
	if title != "" {
		innerHeight--
	}
	if subtitle != "" {
		innerHeight--
	}
	return maxInt(1, innerHeight)
}

func (m *Model) pageItems(delta int) {
	items := m.visibleItems()
	if len(items) == 0 {
		m.itemCursor = 0
		m.itemOffset = 0
		return
	}
	viewport := m.parsedItemsViewport()
	visibleRows := viewport.VisibleRows
	maxOffset := maxInt(0, len(items)-visibleRows)
	nextOffset := m.itemOffset + (delta * visibleRows)
	if nextOffset < 0 {
		nextOffset = 0
	}
	if delta < 0 && nextOffset < visibleRows {
		nextOffset = 0
	}
	if nextOffset > maxOffset {
		nextOffset = maxOffset
	}
	m.itemOffset = nextOffset
	m.itemCursor = clampCursor(nextOffset, len(items))
}

func (m Model) itemListHeight() int {
	spec := layoutSpec(m.width, m.height)
	return maxInt(3, paneBodyLineCapacity(spec.bodyHeight, "Items", "List")-5)
}

func (m Model) importPickerListHeight() int {
	spec := layoutSpec(m.width, m.height)
	return maxInt(4, minInt(14, spec.bodyHeight-6))
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
	hoveredRow   lipgloss.Style
	disabledRow  lipgloss.Style
	muted        lipgloss.Style
	help         lipgloss.Style
	footerKey    lipgloss.Style
	error        lipgloss.Style
	success      lipgloss.Style
	warning      lipgloss.Style
	separator    lipgloss.Style
	footer       lipgloss.Style
	tab          lipgloss.Style
	activeTab    lipgloss.Style
	hoveredTab   lipgloss.Style
	pane         lipgloss.Style
	focusedPane  lipgloss.Style
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
			Foreground(lightDark(lipgloss.Color("#FFFFFF"), lipgloss.Color("#0D1117"))).
			Background(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))),
		hoveredRow: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#24292F"), lipgloss.Color("#E6EDF3"))).
			Background(lightDark(lipgloss.Color("#DDF4FF"), lipgloss.Color("#1F2937"))),
		disabledRow: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#8C959F"), lipgloss.Color("#6E7681"))),
		muted: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))),
		help: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#6E7781"), lipgloss.Color("#8B949E"))),
		footerKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))),
		error: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#B42318"), lipgloss.Color("#FFB4A8"))),
		success: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#1A7F37"), lipgloss.Color("#7EE787"))),
		warning: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#8A6100"), lipgloss.Color("#FFD479"))),
		separator: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))),
		footer: lipgloss.NewStyle().
			MarginTop(1),
		tab: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))).
			Background(lightDark(lipgloss.Color("#F6F8FA"), lipgloss.Color("#161B22"))),
		activeTab: lipgloss.NewStyle().
			Padding(0, 1).
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#FFFFFF"), lipgloss.Color("#0D1117"))).
			Background(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))),
		hoveredTab: lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))).
			Background(lightDark(lipgloss.Color("#DDF4FF"), lipgloss.Color("#1F2937"))),
		pane: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lightDark(lipgloss.Color("#D0D7DE"), lipgloss.Color("#30363D"))).
			Padding(0, 1),
		focusedPane: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lightDark(lipgloss.Color("#0969DA"), lipgloss.Color("#79C0FF"))).
			Padding(0, 1),
		paneTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lightDark(lipgloss.Color("#24292F"), lipgloss.Color("#E6EDF3"))),
		paneSubtitle: lipgloss.NewStyle().
			Foreground(lightDark(lipgloss.Color("#57606A"), lipgloss.Color("#8B949E"))),
	}
}

type importFinishedMsg struct {
	result   any
	err      error
	source   string
	platform domain.PlatformName
}

type activityResult struct {
	Items    []domain.ActivityItem
	Summary  activitySummary
	Warnings []string
}

type activitySummary struct {
	Total     int
	Likes     int
	Comments  int
	Posts     int
	Following int
	Followers int
	Skipped   int
}

type planBuildResult struct {
	Plan          domain.CleanupPlan
	SelectedCount int
	Skipped       []planBuildSkip
	Counts        map[domain.ActionType]int
	Message       string
}

type planBuildSkip struct {
	SourceActivityItemID string
	ItemType             domain.ActivityItemType
	TargetRef            string
	Reason               string
}

type redditConnectFinishedMsg struct {
	username string
	metadata *workspace.RedditConfig
	err      error
}

type redditScanFinishedMsg struct {
	result   reddit.ScanResult
	metadata *workspace.RedditConfig
	err      error
}

type redditDisconnectFinishedMsg struct {
	message string
	err     error
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

func activityResultFromAny(value any) activityResult {
	switch typed := value.(type) {
	case activityResult:
		return typed
	case instagram.ImportResult:
		return activityResultFromInstagram(typed)
	case reddit.ScanResult:
		return activityResultFromReddit(typed)
	default:
		return activityResult{}
	}
}

func activityResultFromInstagram(result instagram.ImportResult) activityResult {
	return activityResult{
		Items: result.Items,
		Summary: activitySummary{
			Total:     result.Summary.Total,
			Likes:     result.Summary.Likes,
			Comments:  result.Summary.Comments,
			Following: result.Summary.Following,
			Followers: result.Summary.Followers,
			Skipped:   result.Summary.Skipped,
		},
		Warnings: result.Warnings,
	}
}

func activityResultFromReddit(result reddit.ScanResult) activityResult {
	return activityResult{
		Items: result.Items,
		Summary: activitySummary{
			Total:    result.Summary.Total,
			Comments: result.Summary.Comments,
			Posts:    result.Summary.Posts,
			Skipped:  result.Summary.Skipped,
		},
		Warnings: result.Warnings,
	}
}

func planResultFromInstagram(result instagram.PlanBuildResult) planBuildResult {
	return planBuildResult{
		Plan:          result.Plan,
		SelectedCount: result.SelectedCount,
		Skipped:       instagramSkips(result.Skipped),
		Counts: map[domain.ActionType]int{
			domain.ActionUnlike:        result.Counts.Unlike,
			domain.ActionDeleteComment: result.Counts.DeleteComment,
			domain.ActionUnfollow:      result.Counts.Unfollow,
		},
		Message: result.Message,
	}
}

func planResultFromReddit(result reddit.PlanBuildResult) planBuildResult {
	return planBuildResult{
		Plan:          result.Plan,
		SelectedCount: result.SelectedCount,
		Skipped:       redditSkips(result.Skipped),
		Counts: map[domain.ActionType]int{
			domain.ActionRedditDeleteComment: result.Counts.DeleteComment,
			domain.ActionRedditDeletePost:    result.Counts.DeletePost,
		},
		Message: result.Message,
	}
}

func instagramSkips(skips []instagram.PlanBuildSkip) []planBuildSkip {
	result := make([]planBuildSkip, 0, len(skips))
	for _, skip := range skips {
		result = append(result, planBuildSkip{
			SourceActivityItemID: skip.SourceActivityItemID,
			ItemType:             skip.ItemType,
			TargetRef:            skip.TargetRef,
			Reason:               skip.Reason,
		})
	}
	return result
}

func redditSkips(skips []reddit.PlanBuildSkip) []planBuildSkip {
	result := make([]planBuildSkip, 0, len(skips))
	for _, skip := range skips {
		result = append(result, planBuildSkip{
			SourceActivityItemID: skip.SourceActivityItemID,
			ItemType:             skip.ItemType,
			TargetRef:            skip.TargetRef,
			Reason:               skip.Reason,
		})
	}
	return result
}

func importZIPCmd(zipPath, source string) tea.Cmd {
	return func() tea.Msg {
		result, err := instagram.ImportZIP(zipPath)
		return importFinishedMsg{result: activityResultFromInstagram(result), err: err, source: source, platform: domain.PlatformInstagram}
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

func redditConnectCmd(input, state string, allowFileFallback bool, appDir string) tea.Cmd {
	return func() tea.Msg {
		code, err := redditCodeFromInput(input, state)
		if err != nil {
			return redditConnectFinishedMsg{err: err}
		}
		if err := ensureRedditSecretStoreReady(allowFileFallback, appDir); err != nil {
			return redditConnectFinishedMsg{err: err}
		}
		oauth, err := newRedditOAuth(allowFileFallback, appDir)
		if err != nil {
			return redditConnectFinishedMsg{err: err}
		}
		tokens, err := oauth.ExchangeCode(context.Background(), code)
		if err != nil {
			return redditConnectFinishedMsg{err: err}
		}
		client, err := reddit.NewClient(tokens.Access, reddit.ClientOptions{})
		if err != nil {
			return redditConnectFinishedMsg{err: err}
		}
		user, err := client.Me(context.Background())
		if err != nil {
			return redditConnectFinishedMsg{err: err}
		}
		result, err := oauth.SaveRefreshToken(user.Name, tokens)
		if err != nil {
			return redditConnectFinishedMsg{err: err}
		}
		metadata := reddit.WorkspaceMetadata(user.Name, tokens, result, time.Now().UTC())
		return redditConnectFinishedMsg{username: user.Name, metadata: metadata}
	}
}

func redditScanCmd(config *workspace.RedditConfig, allowFileFallback bool, appDir string) tea.Cmd {
	return func() tea.Msg {
		if config == nil || strings.TrimSpace(config.Username) == "" {
			return redditScanFinishedMsg{err: errors.New("connect Reddit before scanning")}
		}
		oauth, err := newRedditOAuth(allowFileFallback, appDir)
		if err != nil {
			return redditScanFinishedMsg{err: err}
		}
		refresh, loadResult, err := oauth.LoadRefreshToken(config.Username)
		if err != nil {
			return redditScanFinishedMsg{err: err}
		}
		tokens, err := oauth.Refresh(context.Background(), refresh)
		if err != nil {
			return redditScanFinishedMsg{err: err}
		}
		client, err := reddit.NewClient(tokens.Access, reddit.ClientOptions{})
		if err != nil {
			return redditScanFinishedMsg{err: err}
		}
		result, err := client.ScanActivity(context.Background(), config.Username, reddit.ScanOptions{})
		if err != nil {
			return redditScanFinishedMsg{result: result, err: err}
		}
		metadata := *config
		metadata.ExpiresAt = timePtr(tokens.ExpiresAt)
		metadata.Scopes = cloneStrings(tokens.Scopes)
		if loadResult.Mode != "" {
			metadata.TokenStorageMode = string(loadResult.Mode)
			metadata.CredentialStore = string(loadResult.Mode)
		}
		return redditScanFinishedMsg{result: result, metadata: &metadata}
	}
}

func redditDisconnectCmd(config *workspace.RedditConfig, revoke bool, allowFileFallback bool, appDir string) tea.Cmd {
	return func() tea.Msg {
		if config == nil || strings.TrimSpace(config.Username) == "" {
			return redditDisconnectFinishedMsg{message: "No Reddit account metadata found."}
		}
		oauth, err := newRedditOAuth(allowFileFallback, appDir)
		if err != nil {
			return redditDisconnectFinishedMsg{err: err}
		}
		message := "Forgot local Reddit metadata."
		if revoke {
			refresh, _, err := oauth.LoadRefreshToken(config.Username)
			if err == nil {
				if err := oauth.Revoke(context.Background(), refresh); err != nil {
					return redditDisconnectFinishedMsg{err: err}
				}
				message = "Revoked Reddit access and cleared local metadata."
			} else if errors.Is(err, secretstore.ErrNotFound) {
				message = "No local refresh token found; cleared local metadata. Remote revoke was not possible."
			} else {
				return redditDisconnectFinishedMsg{err: err}
			}
		}
		if err := oauth.ForgetLocal(config.Username); err != nil && !errors.Is(err, secretstore.ErrNotFound) {
			if !revoke && errors.Is(err, secretstore.ErrUnavailable) {
				return redditDisconnectFinishedMsg{message: "Forgot local Reddit metadata. Stored refresh token could not be cleared because the credential store is unavailable."}
			}
			return redditDisconnectFinishedMsg{err: err}
		}
		return redditDisconnectFinishedMsg{message: message}
	}
}

func demoImportCmd() tea.Cmd {
	return func() tea.Msg {
		demoPath, err := instagram.CreateDemoExportZIP("")
		if err != nil {
			return importFinishedMsg{err: err, source: "demo instagram export", platform: domain.PlatformInstagram}
		}
		defer os.Remove(demoPath)

		result, err := instagram.ImportZIP(demoPath)
		return importFinishedMsg{result: activityResultFromInstagram(result), err: err, source: "demo instagram export", platform: domain.PlatformInstagram}
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

func parsedItemDetailLines(item domain.ActivityItem) []string {
	target := firstNonEmptyString(item.TargetURL, item.TargetID, "-")
	if strings.HasPrefix(target, "https://www.instagram.com") {
		target = strings.TrimPrefix(target, "https://www.instagram.com")
	}
	return []string{
		"Type: " + activityTypeLabel(item),
		"Actor: " + emptyFallback(item.Actor, "-"),
		"Target: " + target,
		"Date: " + compactTime(item.OccurredAt),
	}
}

func itemDetailLines(item domain.ActivityItem) []string {
	lines := []string{}
	lines = appendDetailSection(lines, detailSection("Activity",
		detailKV("ID", item.ID),
		detailKV("Type", activityTypeLabel(item)),
		detailKV("Actor", item.Actor),
	))
	lines = appendDetailSection(lines, detailSection("Target",
		detailKV("Target URL", item.TargetURL),
		detailKV("Target ID", item.TargetID),
	))
	if item.OccurredAt != nil {
		lines = appendDetailSection(lines, detailSection("Timing",
			detailTimeKV("Occurred at", item.OccurredAt),
		))
	}
	lines = appendDetailSection(lines, detailSection("Source",
		detailKV("Source file", item.Source.FileName),
	))
	lines = appendDetailSection(lines, detailSection("Safe Metadata", safeActivityMetadataLines(item)...))
	return lines
}

func planPreviewRows(actions []domain.CleanupAction, skipped []planBuildSkip) []string {
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

func (m Model) planActionCountRows(counts map[domain.ActionType]int) []string {
	if len(counts) == 0 {
		return nil
	}
	keys := make([]domain.ActionType, 0, len(counts))
	for actionType, count := range counts {
		if count > 0 {
			keys = append(keys, actionType)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return string(keys[i]) < string(keys[j])
	})
	rows := make([]string, 0, len(keys))
	for _, actionType := range keys {
		rows = append(rows, m.styles.body.Render(fmt.Sprintf("%s: %d", actionTypeDisplayLabel(actionType), counts[actionType])))
	}
	return rows
}

func actionTypeDisplayLabel(actionType domain.ActionType) string {
	switch actionType {
	case domain.ActionUnlike:
		return "Unlike"
	case domain.ActionDeleteComment:
		return "Delete comment"
	case domain.ActionUnfollow:
		return "Unfollow"
	case domain.ActionRedditDeleteComment:
		return "Reddit delete comment"
	case domain.ActionRedditDeletePost:
		return "Reddit delete post"
	default:
		label := strings.ReplaceAll(string(actionType), "_", " ")
		if label == "" {
			return "-"
		}
		return strings.ToUpper(label[:1]) + label[1:]
	}
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
		"Source",
		"Source label: " + emptyFallback(entry.SourceLabel, "-"),
		"Source path: " + emptyFallback(entry.SourcePath, "-"),
		"Platform: " + emptyFallback(entry.Platform, "-"),
		"Imported at: " + formatPlanTime(entry.ImportedAt),
		fmt.Sprintf("Demo: %t", entry.Demo),
		"",
		"Parsed Items",
		fmt.Sprintf("Total items: %d", entry.ItemCount),
		fmt.Sprintf("Likes: %d", entry.LikeCount),
		fmt.Sprintf("Comments: %d", entry.CommentCount),
		fmt.Sprintf("Posts: %d", entry.PostCount),
		fmt.Sprintf("Following: %d", entry.FollowingCount),
		fmt.Sprintf("Followers: %d", entry.FollowerCount),
		"",
		"Import Notes",
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
		"delete_comment_count": summary.ActionCounts[domain.ActionDeleteComment] + summary.ActionCounts[domain.ActionRedditDeleteComment],
		"delete_post_count":    summary.ActionCounts[domain.ActionRedditDeletePost],
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

func newRedditOAuth(allowFileFallback bool, appDir string) (*reddit.OAuth, error) {
	clientID, err := reddit.ClientIDFromEnv()
	if err != nil {
		return nil, err
	}
	vault, err := redditSecretVault(allowFileFallback, appDir)
	if err != nil {
		return nil, err
	}
	return reddit.NewOAuth(reddit.OAuthConfig{
		ClientID: clientID,
		Vault:    vault,
	})
}

func redditSecretVault(allowFileFallback bool, appDir string) (secretstore.Vault, error) {
	vault := secretstore.Vault{
		Primary:           secretstore.NewKeyringStore(),
		AllowFileFallback: allowFileFallback,
	}
	if strings.TrimSpace(appDir) != "" {
		fallback, err := secretstore.NewFileStore(appDir, allowFileFallback)
		if err != nil {
			return secretstore.Vault{}, err
		}
		vault.Fallback = fallback
	}
	return vault, nil
}

func ensureRedditSecretStoreReady(allowFileFallback bool, appDir string) error {
	primary := secretstore.NewKeyringStore()
	if err := primary.Available(); err == nil {
		return nil
	}
	if !allowFileFallback {
		return errors.New("credential store unavailable; enable explicit local token file fallback before exchanging code")
	}
	if strings.TrimSpace(appDir) == "" {
		return errors.New("local token file fallback requires the Vanish app dir")
	}
	return nil
}

func redditCodeFromInput(input, state string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("reddit auth code is required")
	}
	if strings.Contains(input, "://") {
		parsed, err := url.Parse(input)
		if err != nil {
			return "", errors.New("reddit redirect URL is invalid")
		}
		if gotState := strings.TrimSpace(parsed.Query().Get("state")); gotState != "" && strings.TrimSpace(state) != "" && gotState != strings.TrimSpace(state) {
			return "", errors.New("reddit OAuth state mismatch; start connect again")
		}
		input = strings.TrimSpace(parsed.Query().Get("code"))
	}
	if input == "" {
		return "", errors.New("reddit auth code is required")
	}
	return input, nil
}

func friendlyRedditError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, secretstore.ErrNotFound):
		return "Reddit refresh token missing. Reconnect the account."
	case errors.Is(err, secretstore.ErrUnavailable):
		return "Credential store unavailable. Enable explicit local token file fallback if you want to store the Reddit refresh token in the Vanish app dir."
	case errors.Is(err, secretstore.ErrFallbackConfirmationRequired):
		return "Local token file fallback needs explicit confirmation first."
	case strings.Contains(err.Error(), reddit.ClientIDEnv):
		return "Set VANISH_REDDIT_CLIENT_ID before connecting Reddit."
	default:
		return err.Error()
	}
}

func friendlyActivityError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func redditSourceLabel() string {
	return "reddit API scan"
}

func formatTimePtr(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clone := make([]string, len(values))
	copy(clone, values)
	return clone
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
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
	if source == "" || isDemoSource(source) || strings.EqualFold(source, redditSourceLabel()) {
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
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, len(items), m.parsedItemsViewport().VisibleRows)
	m.itemFocus = itemFocusList
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

func (m Model) filterRows() []string {
	return []string{
		filterTypeRow("Like", m.draftFilter.IncludeTypes[domain.ActivityFilterLike]),
		filterTypeRow("Comment", m.draftFilter.IncludeTypes[domain.ActivityFilterComment]),
		filterTypeRow("Post", m.draftFilter.IncludeTypes[domain.ActivityFilterPost]),
		filterTypeRow("Following", m.draftFilter.IncludeTypes[domain.ActivityFilterFollowing]),
		filterTypeRow("Follower", m.draftFilter.IncludeTypes[domain.ActivityFilterFollower]),
		m.filterInputRow("Actor contains", filterRowActor, m.filterActorInput, m.draftFilter.ActorContains),
		m.filterInputRow("Target contains", filterRowTarget, m.filterTargetInput, m.draftFilter.TargetContains),
		m.filterInputRow("Older than", filterRowOlder, m.filterOlderInput, m.draftOlderDate),
		m.filterInputRow("Newer than", filterRowNewer, m.filterNewerInput, m.draftNewerDate),
		"Apply filters",
		"Clear all filters",
	}
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

func newRedditCodeInput() textinput.Model {
	input := textinput.New()
	input.Placeholder = "code or redirect URL"
	input.Prompt = "> "
	input.CharLimit = 4096
	input.SetWidth(74)
	return input
}

func filterTypeRow(label string, included bool) string {
	checked := " "
	if included {
		checked = "x"
	}
	return fmt.Sprintf("[%s] %s", checked, label)
}

func initialImportPickerDir() string {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		candidates = append(candidates, cwd)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, home)
	}
	candidates = append(candidates, ".")

	for _, candidate := range candidates {
		if _, err := os.ReadDir(candidate); err == nil {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return filepath.Clean(candidate)
		}
	}
	return "."
}

func (m *Model) openImportPicker(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	} else {
		dir = filepath.Clean(dir)
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		m.importPickerDir = dir
		m.importPickerEntries = nil
		m.importPickerCursor = 0
		m.importPickerOffset = 0
		m.importPickerError = fmt.Sprintf("Could not read directory: %v", err)
		return
	}

	entries := []importPickerEntry{}
	parent := filepath.Dir(dir)
	if parent != dir {
		entries = append(entries, importPickerEntry{
			Name:   "..",
			Path:   parent,
			Kind:   "parent",
			Parent: true,
			Dir:    true,
		})
	}

	sort.Slice(dirEntries, func(i, j int) bool {
		left, right := dirEntries[i], dirEntries[j]
		if left.IsDir() != right.IsDir() {
			return left.IsDir()
		}
		return strings.ToLower(left.Name()) < strings.ToLower(right.Name())
	})

	for _, entry := range dirEntries {
		name := entry.Name()
		isDir := entry.IsDir()
		isZip := !isDir && strings.EqualFold(filepath.Ext(name), ".zip")
		kind := "file"
		switch {
		case isDir:
			kind = "dir"
		case isZip:
			kind = "zip"
		}
		entries = append(entries, importPickerEntry{
			Name:     name,
			Path:     filepath.Join(dir, name),
			Kind:     kind,
			Dir:      isDir,
			Zip:      isZip,
			Disabled: !isDir && !isZip,
		})
	}

	m.importPickerDir = dir
	m.importPickerEntries = entries
	m.importPickerCursor = clampCursor(0, len(entries))
	m.importPickerOffset = 0
	m.importPickerError = ""
}

func (m Model) activateImportPickerEntry(index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(m.importPickerEntries) {
		return m, nil
	}
	entry := m.importPickerEntries[index]
	switch {
	case entry.Dir:
		m.openImportPicker(entry.Path)
		return m, nil
	case entry.Disabled:
		return m, nil
	default:
		zipPath := entry.Path
		m = m.resetImportState()
		m.current = screenImporting
		m.importSource = zipPath
		m.importPlatform = domain.PlatformInstagram
		m.importErr = nil
		m.importResult = activityResult{}
		return m, tea.Batch(startSpinnerCmd(m.spinner), importZIPCmd(zipPath, zipPath))
	}
}

func importPickerRow(entry importPickerEntry) string {
	name := entry.Name
	if entry.Dir && !entry.Parent {
		name += string(filepath.Separator)
	}
	return fixedWidthRow(
		fixedColumn{Text: entry.Kind, Width: 6},
		fixedColumn{Text: name, Width: 48},
	)
}

func importPickerDetailLines(entry importPickerEntry) []string {
	action := "Disabled"
	switch {
	case entry.Parent:
		action = "Open parent directory"
	case entry.Dir:
		action = "Open directory"
	case entry.Zip:
		action = "Import ZIP"
	}
	return []string{
		"Name: " + entry.Name,
		"Type: " + entry.Kind,
		"Action: " + action,
		"Path: " + entry.Path,
	}
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

func (m Model) hitBoxesForContent(content string) []hitBox {
	boxes := tabHitBoxes(content)
	switch m.current {
	case screenHome:
		boxes = append(boxes, rowHitBoxes(content, hitHomeAction, 0, platformLabels(m.platforms()))...)
	case screenPlatformDetail:
		current := m.selectedPlatform()
		actionRows, _ := platformActionRows(current.Actions)
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, actionRows)...)
	case screenRedditConnect:
		actionRows, _ := m.redditConnectActions()
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, actionRows)...)
	case screenImportPath:
		boxes = append(boxes, rowHitBoxes(content, hitImportPickerRow, m.importPickerOffset, importPickerRows(m.importPickerEntries))...)
	case screenImportResult:
		if m.importErr == nil {
			boxes = append(boxes, rowHitBoxes(content, hitImportResultAction, 0, resultMenuItems)...)
		}
	case screenItemsBrowser:
		boxes = append(boxes, indexedRowHitBoxes(content, hitParsedItemRow, m.parsedItemsViewport().Offset, isSelectionRowLine)...)
		boxes = append(boxes, rowHitBoxesInAnyPane(content, hitParsedAction, 0, parsedItemActionItems)...)
	case screenSelectionSummary:
		boxes = append(boxes, rowHitBoxes(content, hitSelectionAction, 0, selectionMenuItems)...)
	case screenSelectedItems:
		boxes = append(boxes, indexedRowHitBoxes(content, hitSelectedItemRow, m.selectedOffset, isSelectionRowLine)...)
	case screenPlanPreview:
		boxes = append(boxes, rowHitBoxes(content, hitPlanPreviewAction, 0, planPreviewMenuItems)...)
	case screenLoadedPlanSummary:
		boxes = append(boxes, rowHitBoxes(content, hitLoadedPlanAction, 0, loadedPlanSummaryMenuItems)...)
	case screenLoadedPlanActions:
		boxes = append(boxes, rowHitBoxes(content, hitLoadedPlanRow, m.loadedActionOffset, planActionRows(m.loadedPlan.Actions))...)
	case screenFilters:
		if m.filterEditing == filterEditNone {
			boxes = append(boxes, rowHitBoxes(content, hitFilterRow, 0, m.filterRows())...)
		}
	case screenWarnings:
		boxes = append(boxes, rowHitBoxes(content, hitWarningRow, m.warningOffset, m.importResult.Warnings)...)
	case screenLocalDataOverview:
		boxes = append(boxes, rowHitBoxes(content, hitLocalDataAction, 0, localDataMenuItems)...)
	case screenRecentImports:
		boxes = append(boxes, rowHitBoxes(content, hitRecentImportRow, m.recentImportOffset, recentImportRows(m.recentImports))...)
	case screenRecentPlans:
		boxes = append(boxes, rowHitBoxes(content, hitRecentPlanRow, m.recentPlanOffset, recentPlanRows(m.recentPlans))...)
	case screenAuditLog:
		boxes = append(boxes, rowHitBoxes(content, hitAuditRow, m.auditOffset, auditEventRows(m.auditEvents))...)
	case screenWipeLocalDataConfirm:
		boxes = append(boxes, rowHitBoxes(content, hitWipeAction, 0, wipeLocalDataMenuItems)...)
	case screenQuitConfirm:
		boxes = append(boxes, rowHitBoxes(content, hitQuitAction, 0, quitConfirmMenuItems)...)
	}
	return boxes
}

func hitTargetAt(boxes []hitBox, x, y int) hitTarget {
	for _, box := range boxes {
		if box.Contains(x, y) {
			return box.Target
		}
	}
	return hitTarget{}
}

func (b hitBox) Contains(x, y int) bool {
	return x >= b.X && x < b.X+b.Width && y >= b.Y && y < b.Y+b.Height
}

func normalizedMousePoint(mouse tea.Mouse) (int, int) {
	// Bubble Tea v2 Mouse.X/Y are zero-based from the terminal's top-left cell.
	// Keep that normalization centralized so future terminal/input changes do not
	// create one-off row offsets in individual handlers.
	return mouse.X, mouse.Y
}

func rowHitBoxes(content string, kind hitKind, offset int, anchors []string) []hitBox {
	if len(anchors) == 0 {
		return nil
	}
	matches := lineMatchesAnyAnchor(anchors)
	return indexedRowHitBoxes(content, kind, offset, matches)
}

func rowHitBoxesInAnyPane(content string, kind hitKind, offset int, anchors []string) []hitBox {
	if len(anchors) == 0 {
		return nil
	}
	matches := lineMatchesAnyAnchor(anchors)
	return indexedRowHitBoxesInPanes(content, kind, offset, matches)
}

func indexedRowHitBoxes(content string, kind hitKind, offset int, matches func(string) bool) []hitBox {
	lines := strings.Split(content, "\n")
	boxes := []hitBox{}
	ordinal := 0
	for y, line := range lines {
		matchLine := firstPaneSegment(line)
		if !matches(matchLine) {
			continue
		}
		x, width, ok := firstPaneHitBounds(line)
		if !ok {
			continue
		}
		boxes = append(boxes, hitBox{
			X:      x,
			Y:      y,
			Width:  width,
			Height: 1,
			Target: hitTarget{Kind: kind, Index: offset + ordinal},
		})
		ordinal++
	}
	return boxes
}

func indexedRowHitBoxesInPanes(content string, kind hitKind, offset int, matches func(string) bool) []hitBox {
	lines := strings.Split(content, "\n")
	boxes := []hitBox{}
	ordinal := 0
	for y, line := range lines {
		for _, segment := range paneSegments(line) {
			if !matches(segment.Text) {
				continue
			}
			boxes = append(boxes, hitBox{
				X:      segment.X,
				Y:      y,
				Width:  segment.Width,
				Height: 1,
				Target: hitTarget{Kind: kind, Index: offset + ordinal},
			})
			ordinal++
			break
		}
	}
	return boxes
}

func tabHitBoxes(content string) []hitBox {
	lines := strings.Split(content, "\n")
	boxes := []hitBox{}
	for y, line := range lines {
		plain := stripANSI(line)
		if !isTabLine(plain) {
			continue
		}
		for _, label := range tabLabels {
			start := strings.Index(plain, label)
			for start >= 0 {
				x := maxInt(0, start-1)
				width := lipgloss.Width(label) + 2
				boxes = append(boxes, hitBox{
					X:      x,
					Y:      y,
					Width:  width,
					Height: 1,
					Target: hitTarget{Kind: hitTab, Label: label},
				})
				nextStart := start + len(label)
				next := strings.Index(plain[nextStart:], label)
				if next < 0 {
					break
				}
				start = nextStart + next
			}
		}
	}
	return boxes
}

func firstPaneSegment(line string) string {
	plain := stripANSI(line)
	first := strings.Index(plain, "│")
	if first < 0 {
		return plain
	}
	afterFirst := first + len("│")
	secondRel := strings.Index(plain[afterFirst:], "│")
	if secondRel < 0 {
		return plain[afterFirst:]
	}
	return plain[afterFirst : afterFirst+secondRel]
}

type paneSegment struct {
	Text  string
	X     int
	Width int
}

func paneSegments(line string) []paneSegment {
	plain := stripANSI(line)
	borders := []int{}
	searchFrom := 0
	for {
		next := strings.Index(plain[searchFrom:], "│")
		if next < 0 {
			break
		}
		borders = append(borders, searchFrom+next)
		searchFrom += next + len("│")
	}
	if len(borders) < 2 {
		start := firstNonSpaceCell(plain)
		end := lipgloss.Width(strings.TrimRight(plain, " "))
		if end <= start {
			return nil
		}
		return []paneSegment{{Text: plain, X: start, Width: end - start}}
	}

	segments := []paneSegment{}
	for i := 0; i+1 < len(borders); i += 2 {
		startBorder := borders[i]
		endBorder := borders[i+1]
		startByte := startBorder + len("│")
		if endBorder < startByte {
			continue
		}
		startCell := lipgloss.Width(plain[:startBorder]) + 1
		endCell := lipgloss.Width(plain[:endBorder])
		if endCell <= startCell {
			continue
		}
		segments = append(segments, paneSegment{
			Text:  plain[startByte:endBorder],
			X:     startCell,
			Width: endCell - startCell,
		})
	}
	return segments
}

func firstPaneHitBounds(line string) (int, int, bool) {
	plain := stripANSI(line)
	start, end, ok := firstPaneBounds(plain)
	if !ok {
		start = firstNonSpaceCell(plain)
		end = lipgloss.Width(strings.TrimRight(plain, " "))
	}
	if end <= start {
		return 0, 0, false
	}
	return start, end - start, true
}

func firstPaneBounds(line string) (int, int, bool) {
	first := strings.Index(line, "│")
	if first < 0 {
		return 0, 0, false
	}
	secondRel := strings.Index(line[first+len("│"):], "│")
	if secondRel < 0 {
		return 0, 0, false
	}
	second := first + len("│") + secondRel
	start := lipgloss.Width(line[:first]) + 1
	end := lipgloss.Width(line[:second])
	return start, end, end > start
}

func firstNonSpaceCell(line string) int {
	cell := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			return cell
		}
		cell += lipgloss.Width(string(r))
	}
	return 0
}

func importPickerRows(entries []importPickerEntry) []string {
	rows := make([]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, importPickerRow(entry))
	}
	return rows
}

func planActionRows(actions []domain.CleanupAction) []string {
	rows := make([]string, 0, len(actions))
	for _, action := range actions {
		rows = append(rows, planActionRow(action))
	}
	return rows
}

func recentImportRows(entries []workspace.RecentImport) []string {
	rows := make([]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, recentImportRow(entry))
	}
	return rows
}

func recentPlanRows(entries []workspace.RecentPlan) []string {
	rows := make([]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, recentPlanRow(entry))
	}
	return rows
}

func auditEventRows(events []workspace.AuditEvent) []string {
	rows := make([]string, 0, len(events))
	for _, event := range events {
		rows = append(rows, auditEventRow(event))
	}
	return rows
}

func isTabLine(line string) bool {
	matches := 0
	for _, label := range tabLabels {
		if strings.Contains(line, label) {
			matches++
		}
	}
	return matches >= 2
}

func stripANSI(value string) string {
	var out strings.Builder
	escaping := false
	csi := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if escaping {
			if csi {
				if ch >= 0x40 && ch <= 0x7e {
					escaping = false
					csi = false
				}
				continue
			}
			if ch == '[' {
				csi = true
				continue
			}
			if ch >= 0x40 && ch <= 0x5f {
				escaping = false
			}
			continue
		}
		if ch == '\x1b' {
			escaping = true
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
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
	value = stripANSI(value)
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
