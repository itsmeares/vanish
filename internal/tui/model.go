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

	"github.com/itsmeares/vanish/internal/apply"
	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
	"github.com/itsmeares/vanish/internal/manualcleanup"
	"github.com/itsmeares/vanish/internal/platform"
	"github.com/itsmeares/vanish/internal/reddit"
	"github.com/itsmeares/vanish/internal/secretstore"
	"github.com/itsmeares/vanish/internal/workspace"
)

var (
	builtInPlatforms           = mustPlatformRegistry()
	builtInSimulationProviders = mustSimulationProviderRegistry()
)

func mustPlatformRegistry() platform.Registry {
	registry, err := platform.NewRegistry(instagram.Platform(), reddit.Platform())
	if err != nil {
		panic(err)
	}
	return registry
}

func mustSimulationProviderRegistry() apply.ProviderRegistry {
	registry, err := apply.NewProviderRegistry(instagram.SimulationProvider(), reddit.SimulationProvider())
	if err != nil {
		panic(err)
	}
	return registry
}

type screen int

const (
	screenHome screen = iota
	screenPlatformDetail
	screenInstagramExportGuide
	screenRedditNotes
	screenRedditConnect
	screenRedditSigningIn
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
	screenApplyPreview
	screenApplyConfirm
	screenApplyRunning
	screenApplyResult
	screenExecutionList
	screenExecutionDetail
	screenExecutionAbandonConfirm
	screenExecutionDeleteConfirm
	screenManualCleanupChoice
	screenManualCleanupAction
	screenManualCleanupResult
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

const instagramExportPageURL = "https://accountscenter.instagram.com/info_and_permissions/dyi/"

const (
	instagramGuideOpenPage = iota
	instagramGuideHaveZIP
	instagramGuideBack
)

var instagramGuideMenuItems = []string{
	"Open Instagram export page",
	"I have the ZIP",
	"Back",
}

type redditActionID int

const (
	redditActionSignIn redditActionID = iota
	redditActionScan
	redditActionDisconnect
	redditActionBack
)

type redditAction struct {
	ID       redditActionID
	Label    string
	Disabled bool
	Reason   string
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
	planPreviewApply = iota
	planPreviewExport
	planPreviewBack
)

var planPreviewMenuItems = []string{
	"Apply preview",
	"Export JSON",
	"Back",
}

const defaultPlanExportPath = workspace.DefaultPlanExportPath

const (
	loadedPlanApplyPreview = iota
	loadedPlanViewActions
	loadedPlanBackHome
)

var loadedPlanSummaryMenuItems = []string{
	"Apply preview",
	"View actions",
	"Back home",
}

const (
	applyPreviewRun = iota
	applyPreviewBack
)

var applyPreviewMenuItems = []string{
	"Simulate no-op run",
	"Back",
}

var applyPreviewBlockedMenuItems = []string{
	"Back",
}

const (
	applyConfirmRun = iota
	applyConfirmCancel
)

var applyConfirmMenuItems = []string{
	"Simulate no-op run",
	"Cancel",
}

const (
	applyResultBack = iota
	applyResultViewActions
)

var applyResultMenuItems = []string{
	"Back to plan",
	"View actions",
}

const (
	executionActionResume = iota
	executionActionAbandon
	executionActionDelete
	executionActionBack
)

type executionAction struct {
	ID       int
	Label    string
	Disabled bool
	Reason   string
}

var executionConfirmMenuItems = []string{"Confirm", "Cancel"}

var manualCleanupActionItems = []string{
	"Open target",
	"Mark as done",
	"Skip",
	"Stop",
}

var manualCleanupResultItems = []string{"Back to plan"}

type applyPlanSource int

const (
	applySourceGenerated applyPlanSource = iota
	applySourceLoaded
)

const (
	localDataRecentImports = iota
	localDataRecentPlans
	localDataExecutions
	localDataAuditLog
	localDataWipe
	localDataBackHome
)

var localDataMenuItems = []string{
	"Recent imports",
	"Recent plans",
	"Executions",
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
	hitApplyPreviewAction
	hitApplyConfirmAction
	hitApplyResultAction
	hitExecutionRow
	hitExecutionAction
	hitExecutionConfirm
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
	current                screen
	width                  int
	height                 int
	styles                 styles
	keys                   keyMap
	help                   help.Model
	localWorkspace         *workspace.Workspace
	planPathInput          textinput.Model
	filterActorInput       textinput.Model
	filterTargetInput      textinput.Model
	filterOlderInput       textinput.Model
	filterNewerInput       textinput.Model
	spinner                spinner.Model
	hoverTarget            hitTarget
	hitBoxes               []hitBox
	importPickerDir        string
	importPickerEntries    []importPickerEntry
	importPickerCursor     int
	importPickerOffset     int
	importPickerError      string
	importReturnScreen     screen
	importSource           string
	importPlatform         domain.PlatformName
	importResult           activityResult
	importErr              error
	itemFilter             domain.ActivityItemFilter
	itemIndex              activityItemIndex
	selection              domain.ActivitySelection
	selectionCounts        domain.ActivitySelectionCounts
	selectedItemIndexes    []int
	selectedItemsDirty     bool
	itemFocus              itemBrowserFocus
	itemActionCursor       int
	planResult             planBuildResult
	loadedPlan             domain.CleanupPlan
	loadedPlanSummary      domain.CleanupPlanSummary
	applyPlanSource        applyPlanSource
	applyPreview           apply.Preview
	applyExecution         apply.Execution
	executionSummaries     []apply.ExecutionSummary
	executionView          apply.ExecutionView
	executionSelected      apply.ExecutionSummary
	executionCursor        int
	executionOffset        int
	executionActionCursor  int
	executionConfirmCursor int
	executionError         string
	applyResumeReturn      bool
	manualSession          manualcleanup.Session
	manualSessionLoaded    bool
	manualPlanSource       applyPlanSource
	manualPreviews         map[string]string
	manualPreviewPlanID    string
	manualEligibleCount    int
	manualUnavailable      int
	manualChoiceCursor     int
	manualActionCursor     int
	manualResultCursor     int
	manualStatus           string
	manualError            string
	draftFilter            domain.ActivityItemFilter
	draftOlderDate         string
	draftNewerDate         string
	filterError            string
	selectionMessage       string
	planExportStatus       string
	planExportError        string
	planLoadError          string
	recentPlanError        string
	localDataStatus        string
	localDataWarning       string
	localConfig            workspace.Config
	recentImports          []workspace.RecentImport
	recentPlans            []workspace.RecentPlan
	auditEvents            []workspace.AuditEvent
	auditMalformed         int
	homeCursor             int
	selectedPlatformID     platform.PlatformID
	platformActionCursor   int
	instagramGuideCursor   int
	instagramGuideError    string
	redditConnectCursor    int
	redditFileFallback     bool
	redditAuthState        string
	redditAuthURL          string
	redditSignInCancel     context.CancelFunc
	redditStatus           string
	redditError            string
	redditBusyTitle        string
	redditBusyDetail       string
	resultCursor           int
	itemCursor             int
	itemOffset             int
	filterCursor           int
	filterEditing          int
	selectionCursor        int
	selectedCursor         int
	selectedOffset         int
	planPreviewCursor      int
	planListOffset         int
	loadedPlanCursor       int
	loadedActionCursor     int
	loadedActionOffset     int
	applyPreviewCursor     int
	applyConfirmCursor     int
	applyResultCursor      int
	warningCursor          int
	warningOffset          int
	localDataCursor        int
	recentImportCursor     int
	recentImportOffset     int
	recentPlanCursor       int
	recentPlanOffset       int
	auditCursor            int
	auditOffset            int
	wipeLocalDataCursor    int
	helpReturnScreen       screen
	quitReturnScreen       screen
	quitCursor             int
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
		store := manualcleanup.NewStore(localWorkspace.Dir())
		if session, ok, err := store.LatestUnfinished(); err == nil && ok {
			m.manualSession = session
			m.manualSessionLoaded = true
		}
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
		m.setFilterInputWidths(inputWidth(msg.Width))
		m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, m.visibleItemCount(), m.parsedItemsViewport().VisibleRows)
		m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, m.selection.Len(), m.itemListHeight())
		_, m.warningOffset, _ = m.warningViewport()
		m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
		m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
		m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
		m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
		m.executionOffset = ensureOffset(m.executionCursor, m.executionOffset, len(m.executionSummaries), m.localDataListHeight())
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
		m.selectionCounts = domain.ActivitySelectionCounts{}
		m.selectedItemIndexes = m.selectedItemIndexes[:0]
		m.selectedItemsDirty = false
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

	case instagramExportPageOpenedMsg:
		if msg.err != nil {
			m.instagramGuideError = "Could not open Instagram. Select Open Instagram export page to try again."
		} else {
			m.instagramGuideError = ""
		}
		return m, nil

	case manualTargetOpenedMsg:
		if msg.err != nil {
			m.manualError = "Target unavailable. Skip this action or return to the plan."
			return m, nil
		}
		m.manualError = ""
		m.appendAudit("manual_cleanup_target_opened", map[string]any{
			"plan_id":     m.manualSession.PlanID,
			"action_id":   msg.actionID,
			"action_type": string(msg.actionType),
			"target_kind": string(msg.targetKind),
		})
		return m, nil

	case redditConnectFinishedMsg:
		if !m.currentRedditSignIn(msg.state) {
			return m, nil
		}
		m.cancelRedditSignIn()
		m.redditBusyTitle = ""
		m.redditBusyDetail = ""
		if msg.err != nil {
			m.redditError = friendlyRedditError(msg.err)
			m.redditStatus = ""
			m.current = screenRedditConnect
			return m, nil
		}
		m.redditError = ""
		m.redditStatus = ""
		m.redditAuthState = ""
		m.redditAuthURL = ""
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
		m.selectionCounts = domain.ActivitySelectionCounts{}
		m.selectedItemIndexes = m.selectedItemIndexes[:0]
		m.selectedItemsDirty = false
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
		if m.current == screenImporting || m.current == screenRedditSigningIn || m.current == screenRedditBusy || m.current == screenApplyRunning {
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
		m.refreshManualAvailability(msg.plan)
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

	case applyRunFinishedMsg:
		m.applyExecution = msg.execution
		m.applyPreview = msg.execution.Preview
		if msg.err != nil && errors.Is(msg.err, apply.ErrExecutionExists) && msg.execution.ID != "" {
			m.executionError = ""
			m.applyResumeReturn = false
			m.current = screenApplyRunning
			return m, loadExecutionCmd(m.executionStore(), msg.execution.ID, apply.ExecutionSummary{
				ExecutionID:  msg.execution.ID,
				Resumability: msg.execution.Resumability,
				BlockReason:  msg.execution.BlockReason,
			})
		}
		if msg.err != nil && m.applyExecution.BlockReason == "" {
			m.applyExecution.BlockReason = apply.RuntimeErrorMessage(msg.err)
		}
		if msg.source == applySourceLoaded {
			m.loadedPlan = msg.execution.Plan
			m.loadedPlanSummary = domain.SummarizeCleanupPlan(msg.execution.Plan)
		} else {
			m.planResult.Plan = msg.execution.Plan
		}
		m.recordApplyExecution(msg.execution)
		m.applyResultCursor = 0
		m.applyResumeReturn = false
		m.current = screenApplyResult
		return m, nil

	case executionLoadedMsg:
		m.executionSelected = msg.summary
		m.executionError = ""
		if msg.err != nil {
			m.executionView = apply.ExecutionView{}
			m.executionError = apply.RuntimeErrorMessage(msg.err)
			if errors.Is(msg.err, apply.ErrExecutionCorrupt) {
				m.executionSelected.Resumability = apply.ResumabilityCorrupt
			}
		} else {
			m.executionView = msg.view
			m.executionSelected = executionSummaryFromView(msg.view)
			if msg.summary.Resumability == apply.ResumabilityLocked {
				m.executionSelected.Resumability = apply.ResumabilityLocked
				m.executionSelected.BlockReason = msg.summary.BlockReason
			}
		}
		m.executionActionCursor = 0
		m.current = screenExecutionDetail
		return m, nil

	case executionRunFinishedMsg:
		m.applyExecution = msg.execution
		if msg.err != nil && m.applyExecution.BlockReason == "" {
			m.applyExecution.BlockReason = apply.RuntimeErrorMessage(msg.err)
		}
		m.recordApplyExecution(msg.execution)
		m.applyResultCursor = 0
		m.applyResumeReturn = true
		m.current = screenApplyResult
		return m, nil

	case executionAbandonedMsg:
		if msg.err != nil {
			m.executionError = apply.RuntimeErrorMessage(msg.err)
			m.current = screenExecutionDetail
			return m, nil
		}
		m.recordApplyExecution(msg.execution)
		m.refreshExecutions()
		m.executionError = ""
		return m, loadExecutionCmd(m.executionStore(), msg.execution.ID, apply.ExecutionSummary{})

	case executionDeletedMsg:
		if msg.err != nil {
			m.executionError = apply.RuntimeErrorMessage(msg.err)
			m.current = screenExecutionDetail
			return m, nil
		}
		m.executionError = ""
		m.refreshExecutions()
		m.current = screenExecutionList
		return m, nil

	case tea.MouseClickMsg:
		return m.updateMouseClick(msg)

	case tea.MouseWheelMsg:
		return m.updateMouseWheel(msg)

	case tea.MouseMotionMsg:
		return m.updateMouseMotion(msg)

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.quit) {
			if m.current == screenRedditSigningIn {
				m.cancelRedditSignIn()
				m.redditAuthState = ""
			}
			if m.current != screenQuitConfirm {
				m.openQuitConfirm()
			}
			return m, nil
		}
		if key.Matches(msg, m.keys.help) && m.current != screenKeybindings && m.current != screenRedditSigningIn {
			m.openKeybindings()
			return m, nil
		}

		switch m.current {
		case screenHome:
			return m.updateHome(msg)
		case screenPlatformDetail:
			return m.updatePlatformDetail(msg)
		case screenInstagramExportGuide:
			return m.updateInstagramExportGuide(msg)
		case screenRedditNotes:
			return m.updatePlatformStaticScreen(msg)
		case screenRedditConnect:
			return m.updateRedditConnect(msg)
		case screenRedditSigningIn:
			return m.updateRedditSigningIn(msg)
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
		case screenApplyPreview:
			return m.updateApplyPreview(msg)
		case screenApplyConfirm:
			return m.updateApplyConfirm(msg)
		case screenApplyRunning:
			return m, nil
		case screenApplyResult:
			return m.updateApplyResult(msg)
		case screenExecutionList:
			return m.updateExecutionList(msg)
		case screenExecutionDetail:
			return m.updateExecutionDetail(msg)
		case screenExecutionAbandonConfirm:
			return m.updateExecutionConfirm(msg, true)
		case screenExecutionDeleteConfirm:
			return m.updateExecutionConfirm(msg, false)
		case screenManualCleanupChoice:
			return m.updateManualCleanupChoice(msg)
		case screenManualCleanupAction:
			return m.updateManualCleanupAction(msg)
		case screenManualCleanupResult:
			return m.updateManualCleanupResult(msg)
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

func (m Model) updateInstagramExportGuide(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.instagramGuideCursor = moveCursor(m.instagramGuideCursor, len(instagramGuideMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.instagramGuideCursor = moveCursor(m.instagramGuideCursor, len(instagramGuideMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenPlatformDetail
	case key.Matches(msg, m.keys.selectItem):
		switch m.instagramGuideCursor {
		case instagramGuideOpenPage:
			m.instagramGuideError = ""
			return m, openInstagramExportPageCmd()
		case instagramGuideHaveZIP:
			m.openInstagramZIPPicker(screenInstagramExportGuide)
		case instagramGuideBack:
			m.current = screenPlatformDetail
		}
	}
	return m, nil
}

func (m Model) updateRedditConnect(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	actions := m.redditConnectActions()
	switch {
	case key.Matches(msg, m.keys.up):
		m.redditConnectCursor = moveCursor(m.redditConnectCursor, len(actions), -1)
	case key.Matches(msg, m.keys.down):
		m.redditConnectCursor = moveCursor(m.redditConnectCursor, len(actions), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenPlatformDetail
	case key.Matches(msg, m.keys.selectItem):
		index := clampCursor(m.redditConnectCursor, len(actions))
		if actions[index].Disabled {
			return m, nil
		}
		switch actions[index].ID {
		case redditActionSignIn:
			return m.startRedditSignIn()
		case redditActionScan:
			return m.startRedditScan()
		case redditActionDisconnect:
			return m.startRedditDisconnect(true)
		case redditActionBack:
			m.current = screenPlatformDetail
		}
	}
	return m, nil
}

func (m Model) updateRedditSigningIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.selectItem), key.Matches(msg, m.keys.cancel), key.Matches(msg, m.keys.back):
		m.cancelRedditSignIn()
		m.redditAuthState = ""
		m.redditAuthURL = ""
		m.redditError = ""
		m.redditStatus = "Reddit sign-in cancelled."
		m.current = screenRedditConnect
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) startRedditSignIn() (tea.Model, tea.Cmd) {
	m.cancelRedditSignIn()
	m.redditAuthState = fmt.Sprintf("vanish-%d", time.Now().UnixNano())
	m.redditAuthURL = ""
	if !m.ensureRedditAuthURL() {
		m.redditStatus = ""
		m.current = screenRedditConnect
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := m.redditAuthState
	m.redditSignInCancel = cancel
	m.redditError = ""
	m.redditStatus = ""
	m.current = screenRedditSigningIn
	return m, tea.Batch(
		startSpinnerCmd(m.spinner),
		redditSignInCmd(ctx, m.redditAuthURL, state, m.redditAllowFileFallback(), m.localAppDir()),
	)
}

func (m *Model) cancelRedditSignIn() {
	if m.redditSignInCancel != nil {
		m.redditSignInCancel()
		m.redditSignInCancel = nil
	}
}

func (m Model) currentRedditSignIn(state string) bool {
	if strings.TrimSpace(state) == "" {
		return true
	}
	return strings.TrimSpace(state) == strings.TrimSpace(m.redditAuthState)
}

func redditSignInCmd(ctx context.Context, authURL, state string, allowFileFallback bool, appDir string) tea.Cmd {
	return func() tea.Msg {
		waiter, err := reddit.NewCallbackWaiter()
		if err != nil {
			return redditConnectFinishedMsg{state: state, err: err}
		}
		if err := openExternalURL(authURL); err != nil {
			_ = waiter.Close()
			return redditConnectFinishedMsg{state: state, err: fmt.Errorf("open reddit sign-in URL: %w", err)}
		}
		code, err := waiter.Wait(ctx, state)
		if err != nil {
			return redditConnectFinishedMsg{state: state, err: err}
		}
		return redditConnectWithCode(code, state, allowFileFallback, appDir)
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
		m.current = m.importReturnScreen
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
			itemCount := m.visibleItemCount()
			m.itemCursor = clampCursor(m.itemCursor, itemCount)
			m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, itemCount, m.parsedItemsViewport().VisibleRows)
			m.itemFocus = itemFocusList
			m.current = screenItemsBrowser
		case resultViewWarnings:
			m.warningCursor = clampCursor(m.warningCursor, len(m.importResult.Warnings))
			_, m.warningOffset, _ = m.warningViewport()
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
	itemCount := m.visibleItemCount()
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
			m.itemCursor = moveCursor(m.itemCursor, itemCount, -1)
		}
	case key.Matches(msg, m.keys.down):
		if m.itemFocus == itemFocusActions {
			m.itemActionCursor = m.moveParsedItemActionCursor(1)
		} else {
			m.itemCursor = moveCursor(m.itemCursor, itemCount, 1)
		}
	case key.Matches(msg, m.keys.filter):
		m.beginFilterDraft()
		m.current = screenFilters
	case key.Matches(msg, m.keys.selectItem), key.Matches(msg, m.keys.toggleSelection):
		if m.itemFocus == itemFocusActions {
			return m.activateParsedItemAction()
		}
		if itemCount > 0 {
			m.toggleVisibleItemSelection(clampCursor(m.itemCursor, itemCount))
		}
	case key.Matches(msg, m.keys.selectVisible):
		m.setVisibleItemsSelected(true)
	case key.Matches(msg, m.keys.deselectVisible):
		m.setVisibleItemsSelected(false)
	case key.Matches(msg, m.keys.selectionSummary):
		m.selectionCursor = 0
		m.current = screenSelectionSummary
	case key.Matches(msg, m.keys.back):
		m.current = screenImportResult
	}
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, itemCount, m.parsedItemsViewport().VisibleRows)
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
			m.rebuildSelectedItemIndexes()
			m.selectedCursor = clampCursor(m.selectedCursor, len(m.selectedItemIndexes))
			m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, len(m.selectedItemIndexes), m.itemListHeight())
			m.current = screenSelectedItems
		case selectionSelectVisible:
			m.setVisibleItemsSelected(true)
			m.selectionMessage = "Selected all visible items."
		case selectionDeselectVisible:
			m.setVisibleItemsSelected(false)
			m.selectionMessage = "Deselected all visible items."
		case selectionClear:
			m.clearSelection()
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
	selected := m.selectedItemsCopy()
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
	m.refreshManualAvailability(result.Plan)
	m.recordPlanGenerated(result)
	m.planPreviewCursor = 0
	m.planListOffset = 0
	m.planExportStatus = ""
	m.planExportError = ""
	m.planPathInput.SetValue(m.defaultPlanPathValue())
	m.current = screenPlanPreview
}

func (m Model) updatePlanPreview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.generatedPlanMenuItems()
	switch {
	case key.Matches(msg, m.keys.up):
		m.planPreviewCursor = moveCursor(m.planPreviewCursor, len(items), -1)
	case key.Matches(msg, m.keys.down):
		m.planPreviewCursor = moveCursor(m.planPreviewCursor, len(items), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenSelectionSummary
	case key.Matches(msg, m.keys.selectItem):
		switch items[clampCursor(m.planPreviewCursor, len(items))] {
		case "Start manual cleanup":
			m.openManualCleanup(applySourceGenerated)
		case "Apply preview":
			m.openApplyPreview(applySourceGenerated)
		case "Export JSON":
			m.current = screenPlanExportPath
			if strings.TrimSpace(m.planPathInput.Value()) == "" {
				m.planPathInput.SetValue(m.defaultPlanPathValue())
			}
			m.planExportStatus = ""
			m.planExportError = ""
			return m, m.planPathInput.Focus()
		case "Back":
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
	items := m.loadedPlanMenuItems()
	switch {
	case key.Matches(msg, m.keys.up):
		m.loadedPlanCursor = moveCursor(m.loadedPlanCursor, len(items), -1)
	case key.Matches(msg, m.keys.down):
		m.loadedPlanCursor = moveCursor(m.loadedPlanCursor, len(items), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenHome
	case key.Matches(msg, m.keys.selectItem):
		switch items[clampCursor(m.loadedPlanCursor, len(items))] {
		case "Start manual cleanup":
			m.openManualCleanup(applySourceLoaded)
		case "Apply preview":
			m.openApplyPreview(applySourceLoaded)
		case "View actions":
			m.loadedActionCursor = clampCursor(m.loadedActionCursor, len(m.loadedPlan.Actions))
			m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
			m.current = screenLoadedPlanActions
		case "Back home":
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

func (m Model) updateApplyPreview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.currentApplyPreviewMenuItems()
	switch {
	case key.Matches(msg, m.keys.up):
		m.applyPreviewCursor = moveCursor(m.applyPreviewCursor, len(items), -1)
	case key.Matches(msg, m.keys.down):
		m.applyPreviewCursor = moveCursor(m.applyPreviewCursor, len(items), 1)
	case key.Matches(msg, m.keys.back):
		m.returnToApplySource()
	case key.Matches(msg, m.keys.selectItem):
		index := clampCursor(m.applyPreviewCursor, len(items))
		if !m.applyPreview.CanApply {
			m.returnToApplySource()
			return m, nil
		}
		switch index {
		case applyPreviewRun:
			m.applyConfirmCursor = applyConfirmCancel
			m.current = screenApplyConfirm
		case applyPreviewBack:
			m.returnToApplySource()
		}
	}
	return m, nil
}

func (m Model) updateApplyConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.applyConfirmCursor = moveCursor(m.applyConfirmCursor, len(applyConfirmMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.applyConfirmCursor = moveCursor(m.applyConfirmCursor, len(applyConfirmMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenApplyPreview
	case key.Matches(msg, m.keys.selectItem):
		switch m.applyConfirmCursor {
		case applyConfirmRun:
			m.recordApplyConfirmed(m.applyPreview)
			m.current = screenApplyRunning
			return m, tea.Batch(startSpinnerCmd(m.spinner), runApplyCmd(m.currentApplyPlan(), m.applyPlanSource, m.applyRunner()))
		case applyConfirmCancel:
			m.current = screenApplyPreview
		}
	}
	return m, nil
}

func (m Model) updateApplyResult(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.applyResultCursor = moveCursor(m.applyResultCursor, len(applyResultMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.applyResultCursor = moveCursor(m.applyResultCursor, len(applyResultMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		if m.applyResumeReturn {
			m.refreshExecutions()
			m.current = screenExecutionList
		} else {
			m.returnToApplySource()
		}
	case key.Matches(msg, m.keys.selectItem):
		switch m.applyResultCursor {
		case applyResultBack:
			if m.applyResumeReturn {
				m.refreshExecutions()
				m.current = screenExecutionList
			} else {
				m.returnToApplySource()
			}
		case applyResultViewActions:
			if m.applyPlanSource == applySourceLoaded {
				m.loadedActionCursor = clampCursor(m.loadedActionCursor, len(m.loadedPlan.Actions))
				m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
				m.current = screenLoadedPlanActions
			} else {
				m.current = screenPlanPreview
			}
		}
	}
	return m, nil
}

func (m Model) updateExecutionList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.executionCursor = moveCursor(m.executionCursor, len(m.executionSummaries), -1)
	case key.Matches(msg, m.keys.down):
		m.executionCursor = moveCursor(m.executionCursor, len(m.executionSummaries), 1)
	case key.Matches(msg, m.keys.back):
		m.openLocalDataOverview()
	case key.Matches(msg, m.keys.selectItem):
		if len(m.executionSummaries) == 0 {
			return m, nil
		}
		summary := m.executionSummaries[clampCursor(m.executionCursor, len(m.executionSummaries))]
		m.current = screenApplyRunning
		return m, loadExecutionCmd(m.executionStore(), summary.ExecutionID, summary)
	}
	m.executionOffset = ensureOffset(m.executionCursor, m.executionOffset, len(m.executionSummaries), m.localDataListHeight())
	return m, nil
}

func (m Model) updateExecutionDetail(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	actions := m.executionActions()
	switch {
	case key.Matches(msg, m.keys.up):
		m.executionActionCursor = moveCursor(m.executionActionCursor, len(actions), -1)
	case key.Matches(msg, m.keys.down):
		m.executionActionCursor = moveCursor(m.executionActionCursor, len(actions), 1)
	case key.Matches(msg, m.keys.back):
		m.refreshExecutions()
		m.current = screenExecutionList
	case key.Matches(msg, m.keys.selectItem):
		if len(actions) == 0 {
			return m, nil
		}
		action := actions[clampCursor(m.executionActionCursor, len(actions))]
		if action.Disabled {
			m.executionError = action.Reason
			return m, nil
		}
		switch action.ID {
		case executionActionResume:
			m.executionError = ""
			m.current = screenApplyRunning
			return m, tea.Batch(startSpinnerCmd(m.spinner), resumeExecutionCmd(m.applyRunner(), m.executionSelected.ExecutionID))
		case executionActionAbandon:
			m.executionConfirmCursor = 1
			m.current = screenExecutionAbandonConfirm
		case executionActionDelete:
			m.executionConfirmCursor = 1
			m.current = screenExecutionDeleteConfirm
		case executionActionBack:
			m.refreshExecutions()
			m.current = screenExecutionList
		}
	}
	return m, nil
}

func (m Model) updateExecutionConfirm(msg tea.KeyPressMsg, abandon bool) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.executionConfirmCursor = moveCursor(m.executionConfirmCursor, len(executionConfirmMenuItems), -1)
	case key.Matches(msg, m.keys.down):
		m.executionConfirmCursor = moveCursor(m.executionConfirmCursor, len(executionConfirmMenuItems), 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenExecutionDetail
	case key.Matches(msg, m.keys.selectItem):
		if m.executionConfirmCursor != 0 {
			m.current = screenExecutionDetail
			return m, nil
		}
		m.current = screenApplyRunning
		if abandon {
			return m, abandonExecutionCmd(m.applyRunner(), m.executionSelected.ExecutionID)
		}
		return m, deleteExecutionCmd(m.executionStore(), m.executionSelected)
	}
	return m, nil
}

func (m Model) updateSelectedItems(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	itemCount := len(m.selectedItemIndexes)
	switch {
	case key.Matches(msg, m.keys.up):
		m.selectedCursor = moveCursor(m.selectedCursor, itemCount, -1)
	case key.Matches(msg, m.keys.down):
		m.selectedCursor = moveCursor(m.selectedCursor, itemCount, 1)
	case key.Matches(msg, m.keys.back):
		m.current = screenSelectionSummary
	}
	m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, itemCount, m.itemListHeight())
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
	_, m.warningOffset, _ = m.warningViewport()
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
		case localDataExecutions:
			m.refreshExecutions()
			m.executionCursor = clampCursor(m.executionCursor, len(m.executionSummaries))
			m.executionOffset = ensureOffset(m.executionCursor, m.executionOffset, len(m.executionSummaries), m.localDataListHeight())
			m.current = screenExecutionList
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
	case screenInstagramExportGuide:
		if target.Kind == hitPlatformAction {
			m.instagramGuideCursor = target.Index
			return m.updateInstagramExportGuide(selectKeyPress())
		}
	case screenRedditConnect:
		if target.Kind == hitPlatformAction {
			m.redditConnectCursor = target.Index
			return m.updateRedditConnect(selectKeyPress())
		}
	case screenRedditSigningIn:
		if target.Kind == hitPlatformAction {
			return m.updateRedditSigningIn(selectKeyPress())
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
	case screenApplyPreview:
		if target.Kind == hitApplyPreviewAction {
			m.applyPreviewCursor = target.Index
			return m.updateApplyPreview(selectKeyPress())
		}
	case screenApplyConfirm:
		if target.Kind == hitApplyConfirmAction {
			if target.Index == m.applyConfirmCursor {
				return m.updateApplyConfirm(selectKeyPress())
			}
			m.applyConfirmCursor = target.Index
		}
	case screenApplyResult:
		if target.Kind == hitApplyResultAction {
			m.applyResultCursor = target.Index
			return m.updateApplyResult(selectKeyPress())
		}
	case screenExecutionList:
		if target.Kind == hitExecutionRow {
			m.executionCursor = target.Index
			m.executionOffset = ensureOffset(m.executionCursor, m.executionOffset, len(m.executionSummaries), m.localDataListHeight())
			return m.updateExecutionList(selectKeyPress())
		}
	case screenExecutionDetail:
		if target.Kind == hitExecutionAction {
			m.executionActionCursor = target.Index
			return m.updateExecutionDetail(selectKeyPress())
		}
	case screenExecutionAbandonConfirm:
		if target.Kind == hitExecutionConfirm {
			m.executionConfirmCursor = target.Index
			return m.updateExecutionConfirm(selectKeyPress(), true)
		}
	case screenExecutionDeleteConfirm:
		if target.Kind == hitExecutionConfirm {
			m.executionConfirmCursor = target.Index
			return m.updateExecutionConfirm(selectKeyPress(), false)
		}
	case screenManualCleanupChoice:
		if target.Kind == hitPlatformAction {
			m.manualChoiceCursor = target.Index
			return m.updateManualCleanupChoice(selectKeyPress())
		}
	case screenManualCleanupAction:
		if target.Kind == hitPlatformAction {
			m.manualActionCursor = target.Index
			return m.updateManualCleanupAction(selectKeyPress())
		}
	case screenManualCleanupResult:
		if target.Kind == hitPlatformAction {
			return m.updateManualCleanupResult(selectKeyPress())
		}
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
		itemCount := m.visibleItemCount()
		m.itemCursor = moveCursor(m.itemCursor, itemCount, delta)
		m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, itemCount, m.parsedItemsViewport().VisibleRows)
	case screenSelectedItems:
		itemCount := len(m.selectedItemIndexes)
		m.selectedCursor = moveCursor(m.selectedCursor, itemCount, delta)
		m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, itemCount, m.itemListHeight())
	case screenLoadedPlanActions:
		m.loadedActionCursor = moveCursor(m.loadedActionCursor, len(m.loadedPlan.Actions), delta)
		m.loadedActionOffset = ensureOffset(m.loadedActionCursor, m.loadedActionOffset, len(m.loadedPlan.Actions), m.planActionListHeight())
	case screenWarnings:
		m.warningCursor = moveCursor(m.warningCursor, len(m.importResult.Warnings), delta)
		_, m.warningOffset, _ = m.warningViewport()
	case screenRecentImports:
		m.recentImportCursor = moveCursor(m.recentImportCursor, len(m.recentImports), delta)
		m.recentImportOffset = ensureOffset(m.recentImportCursor, m.recentImportOffset, len(m.recentImports), m.localDataListHeight())
	case screenRecentPlans:
		m.recentPlanError = ""
		m.recentPlanCursor = moveCursor(m.recentPlanCursor, len(m.recentPlans), delta)
		m.recentPlanOffset = ensureOffset(m.recentPlanCursor, m.recentPlanOffset, len(m.recentPlans), m.localDataListHeight())
	case screenExecutionList:
		m.executionCursor = moveCursor(m.executionCursor, len(m.executionSummaries), delta)
		m.executionOffset = ensureOffset(m.executionCursor, m.executionOffset, len(m.executionSummaries), m.localDataListHeight())
	case screenAuditLog:
		m.auditCursor = moveCursor(m.auditCursor, len(m.auditEvents), delta)
		m.auditOffset = ensureOffset(m.auditCursor, m.auditOffset, len(m.auditEvents), m.localDataListHeight())
	default:
		return m, nil
	}

	return m, nil
}

func (m Model) updateItemListClick(target hitTarget) (tea.Model, tea.Cmd) {
	itemCount := m.visibleItemCount()
	index := target.Index
	if target.Kind != hitParsedItemRow || index < 0 || index >= itemCount {
		return m, nil
	}
	if index == clampCursor(m.itemCursor, itemCount) {
		m.itemFocus = itemFocusList
		return m.updateItemsBrowser(selectKeyPress())
	}
	m.itemFocus = itemFocusList
	m.itemCursor = index
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, itemCount, m.parsedItemsViewport().VisibleRows)
	return m, nil
}

func (m *Model) updateSelectedItemListClick(target hitTarget) {
	itemCount := len(m.selectedItemIndexes)
	index := target.Index
	if target.Kind != hitSelectedItemRow || index < 0 || index >= itemCount {
		return
	}
	m.selectedCursor = index
	m.selectedOffset = ensureOffset(m.selectedCursor, m.selectedOffset, itemCount, m.itemListHeight())
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
	_, m.warningOffset, _ = m.warningViewport()
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
	case screenRedditSigningIn:
		content = m.redditSigningInView()
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
	case screenApplyPreview:
		content = m.applyPreviewView()
	case screenApplyConfirm:
		content = m.applyConfirmView()
	case screenApplyRunning:
		content = m.applyRunningView()
	case screenApplyResult:
		content = m.applyResultView()
	case screenExecutionList:
		content = m.executionListView()
	case screenExecutionDetail:
		content = m.executionDetailView()
	case screenExecutionAbandonConfirm:
		content = m.executionConfirmView(true)
	case screenExecutionDeleteConfirm:
		content = m.executionConfirmView(false)
	case screenManualCleanupChoice:
		content = m.manualCleanupChoiceView()
	case screenManualCleanupAction:
		content = m.manualCleanupActionView()
	case screenManualCleanupResult:
		content = m.manualCleanupResultView()
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
	menuWidth, _ := twoPaneWidths(spec, "Platforms")
	menu := m.menuRows(platformLabels(platforms), m.homeCursor, menuWidth, hitHomeAction)
	detailTitle, detailLines := m.homeDetail()

	body := m.twoPane(
		spec,
		"Platforms", "Choose a platform", menu,
		detailTitle, "", detailLines,
	)
	return m.appShell("Home", body, m.footer(footerHome))
}

func (m Model) homeDetail() (string, []string) {
	platforms := m.platforms()
	if len(platforms) == 0 {
		return "No platforms", []string{m.emptyState("No platforms are registered.")}
	}
	current := platforms[clampCursor(m.homeCursor, len(platforms))]
	lines := []string{m.styles.body.Render(current.Summary)}
	if current.ID == platform.PlatformReddit {
		lines = m.redditHomeDetailLines()
	}
	lines = append(lines, "", m.styles.muted.Render("Enter to continue."))
	return current.Name, lines
}

func (m Model) platformDetailView() string {
	current := m.selectedPlatform()
	if current.ID == "" {
		return m.singlePaneFooter("Platform", "", []string{m.emptyState("No platform selected.")}, m.footer(footerEmpty))
	}
	lines := m.platformDetailLines(current)
	return m.singlePaneFooter(current.Name, "", lines, m.footer(footerActionMenu))
}

func (m Model) platformDetailLines(current platform.Platform) []string {
	actionLabels, disabled := platformActionRows(current)
	lines := []string{m.styles.separator.Render("Actions")}
	lines = append(lines, m.menuRowsWithDisabled(actionLabels, disabled, m.platformActionCursor, layoutSpec(m.width, m.height).contentWidth, hitPlatformAction)...)
	if len(current.Actions) > 0 {
		action := current.Actions[clampCursor(m.platformActionCursor, len(current.Actions))]
		if available, reason := current.ActionAvailable(action); !available && strings.TrimSpace(reason) != "" {
			lines = append(lines, m.notice("warning", reason))
		}
	}

	lines = append(lines, "", m.styles.separator.Render("Status"))
	lines = append(lines, m.styles.body.Render(current.Summary))
	lines = append(lines, "", m.styles.separator.Render("Capabilities"))
	for _, capability := range current.Capabilities {
		lines = append(lines, m.styles.body.Render(platformCapabilityLine(capability, layoutSpec(m.width, m.height).contentWidth-4)))
	}
	return lines
}

func (m Model) instagramExportGuideView() string {
	lines := []string{
		m.styles.separator.Render("Actions"),
	}
	lines = append(lines, m.menuRows(instagramGuideMenuItems, m.instagramGuideCursor, layoutSpec(m.width, m.height).contentWidth, hitPlatformAction)...)
	if m.instagramGuideError != "" {
		lines = append(lines, "", m.notice("error", m.instagramGuideError))
	}
	lines = append(lines,
		"",
		m.styles.separator.Render("Request your export"),
		m.styles.body.Render("1. Open Instagram settings"),
		m.styles.body.Render("2. Go to Accounts Centre"),
		m.styles.body.Render("3. Open Your information and permissions"),
		m.styles.body.Render("4. Select Export your information"),
		m.styles.body.Render("5. Create an export"),
		m.styles.body.Render("6. Select the Instagram profile"),
		m.styles.body.Render("7. Choose Export to device"),
		m.styles.body.Render("8. Customise the included information"),
		m.styles.body.Render("9. Set Date range to All time"),
		m.styles.body.Render("10. Set Format to JSON"),
		m.styles.body.Render("11. Start export"),
	)
	return m.singlePaneFooter("Instagram Export", "Return when the ZIP is ready", lines, m.footer(footerActionMenu))
}

func (m Model) redditNotesView() string {
	lines := []string{
		m.styles.body.Render("Scan your own Reddit comments and posts."),
		m.styles.body.Render("Vanish builds dry-run plans only."),
	}
	return m.singlePaneFooter("Reddit", "", lines, m.footer(footerEmpty))
}

func (m Model) redditConnectView() string {
	spec := layoutSpec(m.width, m.height)
	status := m.redditConnectionRows()
	actions := m.redditConnectActions()
	actionLabels, disabled := redditActionRows(actions)
	actionLines := m.menuRowsWithDisabled(actionLabels, disabled, m.redditConnectCursor, spec.sidebarWidth, hitPlatformAction)
	if len(actions) > 0 {
		selected := actions[clampCursor(m.redditConnectCursor, len(actions))]
		if selected.Disabled && strings.TrimSpace(selected.Reason) != "" {
			status = append(status, "", m.notice("warning", selected.Reason))
		}
	}

	body := m.twoPane(
		spec,
		"Actions", "", actionLines,
		"Reddit", "", status,
	)
	return m.appShell("Reddit", body, m.footer(footerActionMenu))
}

func (m Model) redditSigningInView() string {
	lines := []string{
		m.styles.body.Render(fmt.Sprintf("%s Waiting for browser sign-in...", m.spinner.View())),
		m.styles.muted.Render(m.redditAuthURL),
		"",
		m.menuRows([]string{"Cancel"}, 0, centeredActionWidth(m.width), hitPlatformAction)[0],
	}
	if strings.TrimSpace(m.redditError) != "" {
		lines = append([]string{m.notice("error", m.redditError), ""}, lines...)
	}
	return m.centeredPaneFooter("Reddit", "", lines, m.footer("enter cancel · esc cancel · ctrl+q quit"))
}

func (m Model) redditBusyView() string {
	title := emptyFallback(m.redditBusyTitle, "Reddit")
	detail := emptyFallback(m.redditBusyDetail, "Working with Reddit.")
	lines := []string{
		m.styles.body.Render(fmt.Sprintf("%s %s", m.spinner.View(), detail)),
	}
	return m.centeredPaneFooter(title, "", lines, m.footer(footerBusy))
}

func (m Model) platforms() []platform.Platform {
	return builtInPlatforms.List()
}

func (m Model) selectedPlatform() platform.Platform {
	if current, ok := builtInPlatforms.Get(m.selectedPlatformID); ok {
		return current
	}
	platforms := builtInPlatforms.List()
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

func platformActionRows(current platform.Platform) ([]string, map[int]bool) {
	rows := make([]string, 0, len(current.Actions))
	disabled := make(map[int]bool)
	for i, action := range current.Actions {
		rows = append(rows, action.Label)
		if available, _ := current.ActionAvailable(action); !available {
			disabled[i] = true
		}
	}
	if len(disabled) == 0 {
		disabled = nil
	}
	return rows, disabled
}

func platformCapabilityLine(capability platform.Capability, width int) string {
	line := fmt.Sprintf("%s: %s", capability.Label, capability.Support)
	return truncateEnd(line, maxInt(8, width))
}

func (m Model) importPathView() string {
	spec := layoutSpec(m.width, m.height)
	listWidth, detailWidth := twoPaneWidths(spec, "Import ZIP")
	visibleRows := m.importPickerListHeight()
	cursor := clampCursor(m.importPickerCursor, len(m.importPickerEntries))
	offset := ensureOffset(cursor, m.importPickerOffset, len(m.importPickerEntries), visibleRows)

	listLines := []string{
		m.styles.muted.Render(truncateMiddle(emptyFallback(m.importPickerDir, "."), maxInt(10, paneTextWidth(listWidth)))),
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
	return m.centeredPaneFooter("Importing", "", lines, m.footer(footerBusy))
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
		m.section("Parsed Items", m.keyValueRows(activitySummaryKeyValues(summary))),
		m.section("Import Notes", m.keyValueRows([]keyValue{
			{Key: "Skipped or unknown", Value: compactCount(summary.Skipped)},
			{Key: "Warnings", Value: compactCount(m.importResult.WarningCount)},
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
	itemCount := m.visibleItemCount()
	listWidth, detailWidth := twoPaneWidths(spec, "Parsed Items")
	total := len(m.importResult.Items)
	viewport := m.parsedItemsViewport()
	visibleRows := viewport.VisibleRows
	cursor := clampCursor(m.itemCursor, itemCount)
	offset := viewport.Offset

	filterStatus := "off"
	if m.itemFilter.Active() {
		filterStatus = "active"
	}
	listInnerWidth := maxInt(10, paneTextWidth(listWidth))
	statusLine := parsedItemsStatusLine(viewport, itemCount, total, m.selection.Len(), filterStatus, listInnerWidth)
	sourceLine := fmt.Sprintf("Page %d/%d · Source: %s", viewport.Page, viewport.Pages, emptyFallback(m.importSource, m.activitySourceFallback()))
	listLines := []string{
		m.styles.muted.Render(truncateEnd(statusLine, listInnerWidth)),
		m.styles.muted.Render(truncateEnd(sourceLine, listInnerWidth)),
		"",
	}
	if m.itemFilter.Active() {
		listLines = append(listLines, m.notice("warning", "Filters active"), "")
	}

	if itemCount == 0 {
		listLines = append(listLines, m.emptyState("No parsed items."))
	} else {
		listLines = append(listLines, m.parsedItemRows(itemCount, cursor, offset, visibleRows, listWidth, func(item domain.ActivityItem) string {
			return m.selectableItemRowForWidth(item, listWidth)
		})...)
	}

	detailLines := []string{}
	if itemCount == 0 {
		detailLines = append(detailLines, m.emptyState("No matches. Clear filters or scan again."))
	} else if item, ok := m.visibleItemAt(cursor); ok {
		detailLines = append(detailLines, m.detailRows(parsedItemDetailLines(item), detailWidth)...)
	}
	actionLines := m.parsedItemsCockpitLines(detailWidth)

	body := m.parsedItemsBody(spec, listWidth, detailWidth, listLines, detailLines, actionLines)
	return m.appShell("Parsed Items", body, m.footer(footerParsedItems))
}

func (m Model) parsedItemRows(itemCount, cursor, offset, visibleRows, width int, format func(domain.ActivityItem) string) []string {
	return m.windowedTableRows(itemCount, cursor, offset, visibleRows, width, hitParsedItemRow, func(index int) string {
		item, _ := m.visibleItemAt(index)
		return format(item)
	})
}

func (m Model) parsedItemsBody(spec layoutMetrics, listWidth, detailWidth int, listLines, detailLines, actionLines []string) string {
	listHeight := parsedItemsPaneHeight(spec)
	if spec.narrow {
		list := m.paneFocused("Parsed Items", "Review and toggle", strings.Join(listLines, "\n"), spec.contentWidth, listHeight, m.itemFocus == itemFocusList)
		detail := m.pane("Details", "Highlighted item", strings.Join(detailLines, "\n"), spec.contentWidth, twoPaneBodyHeight(spec))
		actions := m.paneFocused("Actions", "Selection and plan", strings.Join(actionLines, "\n"), spec.contentWidth, twoPaneBodyHeight(spec), m.itemFocus == itemFocusActions)
		return lipgloss.JoinVertical(lipgloss.Left, list, detail, actions)
	}

	list := m.paneFocused("Parsed Items", "Review and toggle", strings.Join(listLines, "\n"), listWidth, listHeight, m.itemFocus == itemFocusList)
	rightHeight := blockHeight(list)
	detailHeight := maxInt(8, minInt(rightHeight/3, rightHeight-8))
	detail := m.paneRenderedHeight("Details", "Highlighted item", strings.Join(detailLines, "\n"), detailWidth, detailHeight)
	actionHeight := maxInt(5, rightHeight-blockHeight(detail))
	actions := m.paneFocusedRenderedHeight("Actions", "Selection and plan", strings.Join(actionLines, "\n"), detailWidth, actionHeight, m.itemFocus == itemFocusActions)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, strings.Repeat(" ", spec.gap), lipgloss.JoinVertical(lipgloss.Left, detail, actions))
}

func parsedItemsPaneHeight(spec layoutMetrics) int {
	if spec.narrow {
		return twoPaneBodyHeight(spec)
	}
	return maxInt(5, spec.bodyHeight-1)
}

func parsedItemsStatusLine(viewport listViewport, matching, total, selected int, filterStatus string, width int) string {
	full := fmt.Sprintf("%s · Matching %s/%s · Selected %s · Filters %s",
		viewport.ShowingLabel(),
		compactCount(matching),
		compactCount(total),
		compactCount(selected),
		filterStatus,
	)
	if lipgloss.Width(full) <= width {
		return full
	}
	rangeLabel := "0/0"
	if viewport.Total > 0 {
		rangeLabel = fmt.Sprintf("%d-%d/%d", viewport.Start, viewport.End, viewport.Total)
	}
	return fmt.Sprintf("%s · Match %s/%s · Sel %s · Filters %s",
		rangeLabel,
		compactCount(matching),
		compactCount(total),
		compactCount(selected),
		filterStatus,
	)
}

func (m Model) parsedItemsCockpitLines(width int) []string {
	counts := m.selectionCounts
	lines := []string{}
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
		itemCount := m.visibleItemCount()
		if itemCount > 0 {
			m.toggleVisibleItemSelection(clampCursor(m.itemCursor, itemCount))
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
		m.styles.body.Render("Import or scan to review items."),
		m.styles.muted.Render("Items you can select will appear here."),
	}
	return m.singlePaneFooter("Review", "No parsed items yet", lines, m.footer(footerEmpty))
}

func (m Model) selectionSummaryView() string {
	spec := layoutSpec(m.width, m.height)
	counts := m.selectionCounts
	visibleCount := m.visibleItemCount()
	_, dashboardWidth := twoPaneWidths(spec, "Actions")
	summaryLines := m.dashboardSections(
		dashboardWidth,
		m.warningBanner(m.selectionMessage, dashboardWidth),
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
	itemCount := len(m.selectedItemIndexes)
	listWidth, detailWidth := twoPaneWidths(spec, "Selected Items")
	total := len(m.importResult.Items)
	visibleRows := m.itemListHeight()
	cursor := clampCursor(m.selectedCursor, itemCount)
	offset := ensureOffset(cursor, m.selectedOffset, itemCount, visibleRows)

	listLines := []string{
		m.styles.muted.Render(fmt.Sprintf("Selected: %d / Total: %d", itemCount, total)),
		"",
	}

	if itemCount == 0 {
		listLines = append(listLines, m.emptyState("No selected items yet."))
	} else {
		listLines = append(listLines, m.windowedTableRows(itemCount, cursor, offset, visibleRows, listWidth, hitSelectedItemRow, func(index int) string {
			item, _ := m.selectedItemAt(index)
			return m.selectableItemRowForWidth(item, listWidth)
		})...)
	}

	detailLines := []string{}
	if itemCount == 0 {
		detailLines = append(detailLines, m.emptyState("No item selected."))
	} else if item, ok := m.selectedItemAt(cursor); ok {
		detailLines = append(detailLines, m.detailRows(itemDetailLines(item), detailWidth)...)
	}

	body := m.twoPane(spec, "Selected Items", "Chosen cleanup candidates", listLines, "Details", "Highlighted item", detailLines)
	return m.appShell("Selected Items", body, m.footer(footerList))
}

func (m Model) planPreviewView() string {
	spec := layoutSpec(m.width, m.height)
	result := m.planResult
	summaryWidth, actionWidth := twoPaneWidths(spec, "Plan Summary")
	rowCount := len(result.Plan.Actions) + len(result.Skipped)
	visibleRows := m.planListHeight()
	offset := ensureOffset(0, m.planListOffset, rowCount, visibleRows)

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
	if m.manualUnavailable > 0 {
		summaryLines = append(summaryLines, m.styles.muted.Render(fmt.Sprintf("Manual cleanup unavailable: %d", m.manualUnavailable)))
	}
	if m.manualStatus != "" {
		summaryLines = append(summaryLines, m.notice("success", m.manualStatus))
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, m.menuRows(m.generatedPlanMenuItems(), m.planPreviewCursor, summaryWidth, hitPlanPreviewAction)...)

	actionLines := []string{}
	if rowCount == 0 {
		actionLines = append(actionLines, m.emptyState("No supported actions."))
	} else {
		actionLines = append(actionLines, m.windowedPlainRows(rowCount, offset, visibleRows, actionWidth, func(index int) string {
			if index < len(result.Plan.Actions) {
				return planActionRowForWidth(result.Plan.Actions[index], actionWidth)
			}
			return skippedPlanRowForWidth(result.Skipped[index-len(result.Plan.Actions)], actionWidth)
		})...)
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
		m.styles.body.Render("Open a saved cleanup plan JSON file."),
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
	if m.manualUnavailable > 0 {
		detailLines = append(detailLines, m.styles.muted.Render(fmt.Sprintf("Manual cleanup unavailable: %d", m.manualUnavailable)))
	}
	if m.manualStatus != "" {
		detailLines = append(detailLines, m.notice("success", m.manualStatus))
	}
	body := m.twoPane(
		spec,
		"Actions", "Loaded plan", m.menuRows(m.loadedPlanMenuItems(), m.loadedPlanCursor, spec.sidebarWidth, hitLoadedPlanAction),
		"Loaded Plan", "", detailLines,
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
		listLines = append(listLines, m.windowedTableRows(len(actions), cursor, offset, visibleRows, listWidth, hitLoadedPlanRow, func(index int) string {
			return planActionRowForWidth(actions[index], listWidth)
		})...)
	}

	detailLines := []string{}
	if len(actions) == 0 {
		detailLines = append(detailLines, m.emptyState("No action selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(planActionDetailLines(actions[cursor]), detailWidth)...)
	}

	body := m.twoPane(spec, "Plan Actions", "Read-only", listLines, "Details", "Selected action", detailLines)
	return m.appShell("Plan Actions", body, m.footer(footerList))
}

func (m Model) loadedPlanActionRowsForViewport() (int, []string) {
	actions := m.loadedPlan.Actions
	visibleRows := m.planActionListHeight()
	cursor := clampCursor(m.loadedActionCursor, len(actions))
	offset := ensureOffset(cursor, m.loadedActionOffset, len(actions), visibleRows)
	end := minInt(len(actions), offset+visibleRows)
	listWidth, _ := twoPaneWidths(layoutSpec(m.width, m.height), "Plan Actions")
	rows := make([]string, 0, maxInt(0, end-offset))
	for index := offset; index < end; index++ {
		rows = append(rows, planActionRowForWidth(actions[index], listWidth))
	}
	return offset, rows
}

func (m Model) applyPreviewView() string {
	spec := layoutSpec(m.width, m.height)
	preview := m.applyPreview
	menuItems := m.currentApplyPreviewMenuItems()
	_, actionWidth := twoPaneWidths(spec, "Actions")

	status := "Ready"
	if !preview.CanApply {
		status = "Blocked"
	}
	summaryLines := m.dashboardSections(
		spec.detailWidth,
		m.section("Apply Preview", m.keyValueRows([]keyValue{
			{Key: "Platform", Value: emptyFallback(string(preview.Platform), "-")},
			{Key: "Mode", Value: emptyFallback(string(preview.Mode), "-")},
			{Key: "Executor", Value: emptyFallback(string(preview.Executor), "-")},
			{Key: "Status", Value: status},
			{Key: "Provider ready", Value: readyLabel(preview.ProviderReady)},
		})),
		m.section("Counts", m.keyValueRows([]keyValue{
			{Key: "Actions", Value: compactCount(preview.Summary.TotalActions)},
			{Key: "Pending", Value: compactCount(preview.PendingCount)},
			{Key: "Unsupported", Value: compactCount(preview.UnsupportedCount)},
			{Key: "Failed", Value: compactCount(preview.Summary.StatusCounts[domain.ActionStatusFailed])},
			{Key: "Skipped", Value: compactCount(preview.Summary.StatusCounts[domain.ActionStatusSkipped])},
		})),
	)
	if len(preview.Blockers) > 0 {
		summaryLines = append(summaryLines, m.section("Blocker", []string{m.styles.body.Render(preview.Blockers[0].Message)})...)
	}
	summaryLines = append(summaryLines, "")
	summaryLines = append(summaryLines, m.menuRows(menuItems, m.applyPreviewCursor, spec.detailWidth, hitApplyPreviewAction)...)

	actionLines := m.applyActionSummaryLines(preview, actionWidth)
	body := m.twoPane(spec, "Actions", "No-op simulation", summaryLines, "Pending Work", "What would be simulated", actionLines)
	return m.appShell("Apply Preview", body, m.footer(footerActionMenu))
}

func (m Model) applyConfirmView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.styles.body.Render(fmt.Sprintf("Simulate no-op run for %d pending actions?", m.applyPreview.PendingCount)),
		m.styles.body.Render("No platform content changes."),
		"",
	}
	lines = append(lines, m.menuRows(applyConfirmMenuItems, m.applyConfirmCursor, spec.contentWidth, hitApplyConfirmAction)...)
	return m.singlePaneFooter("Confirm Apply", "No-op execution", lines, m.footer(footerConfirm))
}

func (m Model) applyRunningView() string {
	lines := []string{
		m.styles.body.Render(fmt.Sprintf("%s Simulating no-op run...", m.spinner.View())),
		m.styles.muted.Render("No platform content changes."),
	}
	return m.centeredPaneFooter("Applying", "No-op simulation", lines, m.footer(footerBusy))
}

func (m Model) applyResultView() string {
	spec := layoutSpec(m.width, m.height)
	execution := m.applyExecution
	lines := m.dashboardSections(
		spec.detailWidth,
		m.section("Result", m.keyValueRows([]keyValue{
			{Key: "State", Value: string(execution.State)},
			{Key: "Executor", Value: emptyFallback(string(execution.Preview.Executor), "-")},
			{Key: "No platform changes", Value: "yes"},
		})),
		m.section("Counts", m.keyValueRows([]keyValue{
			{Key: "Done", Value: compactCount(execution.Counts.Done)},
			{Key: "Failed", Value: compactCount(execution.Counts.Failed)},
			{Key: "Skipped", Value: compactCount(execution.Counts.Skipped)},
			{Key: "Stopped", Value: compactCount(execution.Counts.Stopped)},
			{Key: "Cancelled", Value: compactCount(execution.Counts.Cancelled)},
			{Key: "Pending", Value: compactCount(execution.Counts.Pending)},
		})),
	)
	if detailRows := applyOutcomeDetailRows(execution); len(detailRows) > 0 {
		lines = append(lines, m.section("Outcome", m.keyValueRows(detailRows))...)
	}
	if strings.TrimSpace(execution.BlockReason) != "" {
		lines = append(lines, m.notice("warning", execution.BlockReason))
	}
	if strings.TrimSpace(execution.RecoveryWarning) != "" {
		lines = append(lines, m.notice("warning", execution.RecoveryWarning))
	}
	lines = append(lines, "")
	lines = append(lines, m.menuRows(applyResultMenuItems, m.applyResultCursor, spec.detailWidth, hitApplyResultAction)...)

	resultLines := []string{}
	if len(execution.Results) == 0 {
		resultLines = append(resultLines, m.emptyState("No actions ran."))
	} else {
		finalResults := finalActionResults(execution.Results)
		rows := make([]string, 0, len(finalResults))
		for _, result := range finalResults {
			rows = append(rows, applyResultRow(result))
		}
		resultLines = append(resultLines, m.plainRows(rows, 0, m.planActionListHeight(), spec.mainWidth)...)
	}
	body := m.twoPane(spec, "Apply Result", "No-op simulation", lines, "Action Results", "Safe summary", resultLines)
	return m.appShell("Apply Result", body, m.footer(footerActionMenu))
}

func (m Model) executionListView() string {
	spec := layoutSpec(m.width, m.height)
	visibleRows := m.localDataListHeight()
	cursor := clampCursor(m.executionCursor, len(m.executionSummaries))
	offset := ensureOffset(cursor, m.executionOffset, len(m.executionSummaries), visibleRows)
	listWidth, detailWidth := twoPaneWidths(spec, "Executions")
	listLines := []string{m.styles.muted.Render(fmt.Sprintf("%d durable executions", len(m.executionSummaries))), ""}
	if strings.TrimSpace(m.executionError) != "" {
		listLines = append(listLines, m.notice("error", m.executionError), "")
	}
	if len(m.executionSummaries) == 0 {
		listLines = append(listLines, m.emptyState("No durable executions yet."))
	} else {
		listLines = append(listLines, m.windowedTableRows(len(m.executionSummaries), cursor, offset, visibleRows, listWidth, hitExecutionRow, func(index int) string {
			return executionSummaryRow(m.executionSummaries[index])
		})...)
	}
	detailLines := []string{}
	if len(m.executionSummaries) == 0 {
		detailLines = append(detailLines, m.emptyState("No execution selected."))
	} else {
		detailLines = append(detailLines, m.detailRows(executionSummaryDetails(m.executionSummaries[cursor]), detailWidth)...)
	}
	body := m.twoPane(spec, "Executions", "Newest first", listLines, "Details", "Enter opens selected", detailLines)
	return m.appShell("Executions", body, m.footer("up/down move · enter/click open · esc back · ? help · ctrl+q quit"))
}

func (m Model) executionDetailView() string {
	spec := layoutSpec(m.width, m.height)
	actions := m.executionActions()
	labels := make([]string, len(actions))
	disabled := make(map[int]bool, len(actions))
	for index, action := range actions {
		labels[index] = action.Label
		disabled[index] = action.Disabled
	}
	lines := m.dashboardSections(spec.detailWidth,
		m.section("Execution", m.keyValueRows(executionSummaryKeyValues(m.executionSelected))),
		m.section("Progress", m.keyValueRows(executionCountKeyValues(m.executionSelected.Counts))),
	)
	if strings.TrimSpace(m.executionSelected.BlockReason) != "" {
		lines = append(lines, m.notice("warning", m.executionSelected.BlockReason))
	}
	if strings.TrimSpace(m.executionSelected.RecoveryWarning) != "" {
		lines = append(lines, m.notice("warning", m.executionSelected.RecoveryWarning))
	}
	if strings.TrimSpace(m.executionError) != "" {
		lines = append(lines, m.notice("error", m.executionError))
	}
	lines = append(lines, "")
	lines = append(lines, m.menuRowsWithDisabled(labels, disabled, m.executionActionCursor, spec.detailWidth, hitExecutionAction)...)
	if len(actions) > 0 && actions[clampCursor(m.executionActionCursor, len(actions))].Disabled {
		lines = append(lines, "", m.notice("warning", actions[clampCursor(m.executionActionCursor, len(actions))].Reason))
	}
	return m.singlePaneFooter("Execution", "Durable local progress", lines, m.footer(footerActionMenu))
}

func (m Model) executionConfirmView(abandon bool) string {
	title := "Delete Execution?"
	message := "Delete this terminal execution from local storage?"
	if abandon {
		title = "Abandon Execution?"
		message = "Abandon this execution without invoking any provider action?"
	}
	lines := []string{m.notice("warning", message), ""}
	lines = append(lines, m.menuRows(executionConfirmMenuItems, m.executionConfirmCursor, layoutSpec(m.width, m.height).contentWidth, hitExecutionConfirm)...)
	return m.singlePaneFooter(title, "Defaults to Cancel", lines, m.footer(footerConfirm))
}

func (m Model) filtersView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.styles.muted.Render(fmt.Sprintf("Matching: %d / %d | Filters: %s", m.visibleItemCount(), len(m.importResult.Items), activeLabel(m.itemFilter.Active()))),
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

func (m Model) manualCleanupChoiceView() string {
	items := m.manualChoiceItems()
	done, skipped, pending := m.manualSession.Counts()
	lines := []string{
		m.styles.separator.Render("Progress"),
		m.styles.body.Render(fmt.Sprintf("%d done · %d skipped · %d pending", done, skipped, pending)),
		"",
		m.styles.separator.Render("Actions"),
	}
	lines = append(lines, m.menuRows(items, m.manualChoiceCursor, layoutSpec(m.width, m.height).contentWidth, hitPlatformAction)...)
	if m.manualError != "" {
		lines = append(lines, "", m.notice("error", m.manualError))
	}
	return m.singlePaneFooter("Manual cleanup", truncateMiddle(m.currentManualPlanID(), 40), lines, m.footer(footerActionMenu))
}

func (m Model) currentManualPlanID() string {
	if m.manualSessionLoaded && strings.TrimSpace(m.manualSession.PlanID) != "" {
		return m.manualSession.PlanID
	}
	return m.currentManualPlan(m.manualPlanSource).ID
}

func (m Model) manualActionItems() []string {
	items := append([]string(nil), manualCleanupActionItems...)
	if action, ok := m.manualSession.Current(); ok {
		switch action.TargetKind {
		case instagram.TargetProfile:
			items[0] = "Open profile"
		case instagram.TargetReel:
			items[0] = "Open reel"
		default:
			items[0] = "Open post"
		}
	}
	return items
}

func (m Model) manualCleanupActionView() string {
	action, ok := m.manualSession.Current()
	if !ok {
		return m.manualCleanupResultView()
	}
	progress := fmt.Sprintf("%d of %d", m.manualSession.CurrentPosition+1, len(m.manualSession.Actions))
	title := "Manual cleanup"
	details := []string{m.styles.body.Render(progress), ""}
	switch action.Type {
	case domain.ActionUnfollow:
		title = "Unfollow @" + action.TargetID
		details = append(details, m.styles.body.Render(title))
	case domain.ActionUnlike:
		kind := "post"
		if action.TargetKind == instagram.TargetReel {
			kind = "reel"
		}
		title = "Unlike " + kind
		details = append(details, m.styles.body.Render(title))
		if actor, valid := instagram.NormalizeUsername(action.Actor); valid {
			details = append(details, m.styles.muted.Render("Owner: @"+actor))
		}
		if action.OccurredAt != nil {
			details = append(details, m.styles.muted.Render("Date: "+compactTime(action.OccurredAt)))
		}
		details = append(details, m.styles.muted.Render("Target: "+manualTargetLabel(action)))
	case domain.ActionDeleteComment:
		title = "Delete own comment"
		details = append(details, m.styles.body.Render(title))
		if actor, valid := instagram.NormalizeUsername(action.Actor); valid {
			details = append(details, m.styles.muted.Render("Post owner: @"+actor))
		}
		if action.OccurredAt != nil {
			details = append(details, m.styles.muted.Render("Comment date: "+compactTime(action.OccurredAt)))
		}
		if preview := instagram.SanitizeCommentPreview(m.manualPreviews[action.ActionID]); preview != "" {
			details = append(details, m.styles.muted.Render("Comment: "+preview))
		}
		details = append(details, m.styles.muted.Render("Post: "+manualTargetLabel(action)))
	}
	details = append(details, "", m.styles.separator.Render("Actions"))
	details = append(details, m.menuRows(m.manualActionItems(), m.manualActionCursor, layoutSpec(m.width, m.height).contentWidth, hitPlatformAction)...)
	if m.manualError != "" {
		details = append(details, "", m.notice("error", m.manualError))
	}
	return m.singlePaneFooter("Manual cleanup", title, details, m.footer(footerActionMenu))
}

func (m Model) manualCleanupResultView() string {
	done, skipped, _ := m.manualSession.Counts()
	lines := []string{
		m.styles.separator.Render("Result"),
		m.styles.body.Render(fmt.Sprintf("%d done · %d skipped", done, skipped)),
		"",
	}
	lines = append(lines, m.menuRows(manualCleanupResultItems, m.manualResultCursor, layoutSpec(m.width, m.height).contentWidth, hitPlatformAction)...)
	return m.singlePaneFooter("Manual cleanup complete", "", lines, m.footer(footerActionMenu))
}

func manualTargetLabel(action manualcleanup.Action) string {
	switch action.TargetKind {
	case instagram.TargetProfile:
		return "@" + action.TargetID
	case instagram.TargetReel:
		return "/reel/" + action.TargetID
	case instagram.TargetTV:
		return "/tv/" + action.TargetID
	default:
		return "/p/" + action.TargetID
	}
}

func (m Model) warningsView() string {
	spec := layoutSpec(m.width, m.height)
	warnings := m.importResult.Warnings
	cursor, offset, visibleRows := m.warningViewport()

	lines := []string{
		m.styles.muted.Render(fmt.Sprintf(
			"%s warnings in %s groups from %s",
			exactCount(m.importResult.WarningCount),
			exactCount(len(warnings)),
			emptyFallback(m.importSource, "instagram export"),
		)),
		"",
	}

	if len(warnings) == 0 {
		lines = append(lines, m.emptyState("No warnings."))
	} else {
		lines = append(lines, m.windowedTableRows(len(warnings), cursor, offset, visibleRows, spec.contentWidth, hitWarningRow, func(index int) string {
			return warningGroupRow(warnings[index])
		})...)
		selected := warnings[cursor]
		if selected.SourceFile != "" || len(selected.Examples) > 0 {
			lines = append(lines, "")
		}
		if selected.SourceFile != "" {
			lines = append(lines, m.styles.muted.Render("Source: "+selected.SourceFile))
		}
		for _, example := range selected.Examples {
			lines = append(lines, m.styles.muted.Render("Structure: "+example))
		}
	}

	return m.singlePaneFooter("Import Warnings", "Grouped skipped records and files", lines, m.footer(footerList))
}

func (m Model) warningViewport() (cursor, offset, visibleRows int) {
	warnings := m.importResult.Warnings
	cursor = clampCursor(m.warningCursor, len(warnings))
	visibleRows = m.warningListHeight()
	if len(warnings) > 0 {
		detailLines := len(warnings[cursor].Examples)
		if warnings[cursor].SourceFile != "" {
			detailLines++
		}
		if detailLines > 0 {
			visibleRows = maxInt(3, visibleRows-detailLines-1)
		}
	}
	offset = ensureOffset(cursor, m.warningOffset, len(warnings), visibleRows)
	return cursor, offset, visibleRows
}

func warningGroupRow(group activityWarningGroup) string {
	count := maxInt(0, group.Count)
	unit := strings.TrimSpace(group.Unit)
	if unit == "" {
		unit = "warning"
	}
	plural := ""
	if count != 1 {
		plural = "s"
	}
	category := strings.TrimSpace(group.Category)
	if category == "" {
		category = "activity"
	}
	if unit == "warning" {
		return fmt.Sprintf("%s %s warning%s: %s", exactCount(count), category, plural, group.Reason)
	}
	return fmt.Sprintf("%s %s %s%s skipped: %s", exactCount(count), category, unit, plural, group.Reason)
}

func (m Model) warningRowsForViewport() (int, []string) {
	_, offset, visibleRows := m.warningViewport()
	end := minInt(len(m.importResult.Warnings), offset+visibleRows)
	rows := make([]string, 0, maxInt(0, end-offset))
	for index := offset; index < end; index++ {
		rows = append(rows, warningGroupRow(m.importResult.Warnings[index]))
	}
	return offset, rows
}

func (m Model) warningAnchorsForViewport() (int, []string) {
	offset, rows := m.warningRowsForViewport()
	innerWidth := paneTextWidth(layoutSpec(m.width, m.height).contentWidth)
	for index := range rows {
		rows[index] = truncateEnd(rows[index], innerWidth)
	}
	return offset, rows
}

func (m Model) localDataOverviewView() string {
	spec := layoutSpec(m.width, m.height)
	stats := []string{
		m.styles.body.Render("Manage Vanish data on this device."),
		"",
		m.styles.body.Render(fmt.Sprintf("App directory: %s", m.localDataDirLabel())),
		m.styles.body.Render(fmt.Sprintf("Recent imports: %d", len(m.recentImports))),
		m.styles.body.Render(fmt.Sprintf("Recent plans: %d", len(m.recentPlans))),
		m.styles.body.Render(fmt.Sprintf("Executions: %d", len(m.executionSummaries))),
		m.styles.body.Render(fmt.Sprintf("Audit events: %d", len(m.auditEvents))),
	}
	if m.auditMalformed > 0 {
		stats = append(stats, m.notice("warning", fmt.Sprintf("Skipped malformed audit lines: %d", m.auditMalformed)))
	}
	stats = append(stats, "")
	stats = append(stats, m.localDataMessages()...)
	body := m.twoPane(
		spec,
		"Actions", "", m.menuRows(localDataMenuItems, m.localDataCursor, spec.sidebarWidth, hitLocalDataAction),
		"Local Data", "", stats,
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
		listLines = append(listLines, m.emptyState("No recent imports yet."))
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
	body := m.twoPane(spec, "Recent Imports", "Newest first", listLines, "Details", "Selected import", detailLines)
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
		listLines = append(listLines, m.emptyState("No recent plans yet."))
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
	body := m.twoPane(spec, "Recent Plans", "Enter loads selected", listLines, "Details", "Selected plan", detailLines)
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
	body := m.twoPane(spec, "Audit Log", "Local events", listLines, "Details", "Selected event", detailLines)
	return m.appShell("Audit Log", body, m.footer(footerList))
}

func (m Model) wipeLocalDataConfirmView() string {
	spec := layoutSpec(m.width, m.height)
	lines := []string{
		m.notice("warning", "This clears Vanish-managed config, history, audit records, and saved execution progress."),
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
	m.selectionCounts = domain.ActivitySelectionCounts{}
	m.selectedItemIndexes = m.selectedItemIndexes[:0]
	m.selectedItemsDirty = false
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
	m.resetApplyState()
	m.planPathInput.SetValue(m.defaultPlanPathValue())
	m.planPathInput.Blur()
	m.planExportStatus = ""
	m.planExportError = ""
	m.manualEligibleCount = 0
	m.manualUnavailable = 0
	m.manualStatus = ""
	m.manualError = ""
}

func (m *Model) resetLoadedPlanState() {
	m.loadedPlan = domain.CleanupPlan{}
	m.loadedPlanSummary = domain.CleanupPlanSummary{}
	m.loadedPlanCursor = 0
	m.loadedActionCursor = 0
	m.loadedActionOffset = 0
	m.resetApplyState()
	m.planLoadError = ""
	m.planPathInput.Blur()
}

func (m *Model) resetApplyState() {
	m.applyPlanSource = applySourceGenerated
	m.applyPreview = apply.Preview{}
	m.applyExecution = apply.Execution{}
	m.applyPreviewCursor = 0
	m.applyConfirmCursor = applyConfirmCancel
	m.applyResultCursor = 0
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
	if selected.ID == platform.PlatformReddit {
		m.openRedditConnect("")
		return
	}
	m.current = screenPlatformDetail
}

func (m Model) activatePlatformAction() (tea.Model, tea.Cmd) {
	current := m.selectedPlatform()
	if len(current.Actions) == 0 {
		return m, nil
	}
	m.platformActionCursor = clampCursor(m.platformActionCursor, len(current.Actions))
	action := current.Actions[m.platformActionCursor]
	if available, _ := current.ActionAvailable(action); !available {
		return m, nil
	}

	switch action.ID {
	case platform.ActionChooseExportZIP:
		m.openInstagramZIPPicker(screenPlatformDetail)
	case platform.ActionRequestInstagramExport, platform.ActionExportGuide:
		m.selectedPlatformID = platform.PlatformInstagramExport
		m.instagramGuideCursor = 0
		m.instagramGuideError = ""
		m.current = screenInstagramExportGuide
		return m, openInstagramExportPageCmd()
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
		m.openRedditConnect("")
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
	if m.current == screenRedditSigningIn {
		m.cancelRedditSignIn()
		m.redditAuthState = ""
		m.redditAuthURL = ""
	}
	switch label {
	case "Home":
		m.current = screenHome
	case "Import":
		m.importReturnScreen = screenHome
		m.current = screenImportPath
		if strings.TrimSpace(m.importPickerDir) == "" {
			m.openImportPicker(initialImportPickerDir())
		}
	case "Review":
		if m.hasImportData() {
			itemCount := m.visibleItemCount()
			m.itemCursor = clampCursor(m.itemCursor, itemCount)
			m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, itemCount, m.parsedItemsViewport().VisibleRows)
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
		case m.manualSessionLoaded && m.manualSession.State != manualcleanup.StateCompleted:
			m.manualPlanSource = applySourceLoaded
			m.loadedPlan = m.manualSession.OriginalPlan()
			m.loadedPlanSummary = domain.SummarizeCleanupPlan(m.loadedPlan)
			m.refreshManualAvailability(m.loadedPlan)
			m.manualChoiceCursor = 0
			m.current = screenManualCleanupChoice
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
	m.refreshExecutions()
	m.localDataCursor = clampCursor(m.localDataCursor, len(localDataMenuItems))
	m.current = screenLocalDataOverview
}

func (m Model) hasImportData() bool {
	return len(m.importResult.Items) > 0 || m.importResult.Summary.Total > 0 || m.importResult.WarningCount > 0
}

func (m Model) hasLoadedPlan() bool {
	return strings.TrimSpace(m.loadedPlan.ID) != "" || len(m.loadedPlan.Actions) > 0
}

func (m Model) hasPlanPreview() bool {
	return strings.TrimSpace(m.planResult.Plan.ID) != "" || len(m.planResult.Plan.Actions) > 0 || len(m.planResult.Skipped) > 0
}

func (m *Model) openApplyPreview(source applyPlanSource) {
	m.applyPlanSource = source
	m.applyPreview = m.applyRunner().Preview(m.currentApplyPlan(), apply.ExecutionModeSimulation)
	m.applyExecution = apply.Execution{}
	m.applyPreviewCursor = 0
	m.applyConfirmCursor = applyConfirmCancel
	m.applyResultCursor = 0
	m.recordApplyPreviewed(m.applyPreview)
	m.current = screenApplyPreview
}

func (m Model) applyRunner() apply.Runner {
	return apply.Runner{
		Providers: builtInSimulationProviders,
		State:     m.applyRuntimeState(),
		Policy:    apply.DefaultRunPolicy(),
		Store:     m.executionStore(),
	}
}

func (m Model) executionStore() *apply.ExecutionStore {
	if m.localWorkspace == nil {
		return nil
	}
	return apply.NewExecutionStore(m.localWorkspace.Dir())
}

func (m *Model) refreshExecutions() {
	store := m.executionStore()
	if store == nil {
		m.executionSummaries = nil
		m.executionError = "Durable execution storage is unavailable."
		return
	}
	summaries, err := store.List()
	if err != nil {
		m.executionSummaries = nil
		m.executionError = apply.RuntimeErrorMessage(err)
		return
	}
	m.executionSummaries = summaries
	m.executionCursor = clampCursor(m.executionCursor, len(summaries))
	m.executionOffset = ensureOffset(m.executionCursor, m.executionOffset, len(summaries), m.localDataListHeight())
}

func (m Model) executionActions() []executionAction {
	summary := m.executionSelected
	switch summary.Resumability {
	case apply.ResumabilityCorrupt:
		return []executionAction{{ID: executionActionDelete, Label: "Delete execution"}, {ID: executionActionBack, Label: "Back"}}
	case apply.ResumabilityTerminal:
		return []executionAction{{ID: executionActionDelete, Label: "Delete execution"}, {ID: executionActionBack, Label: "Back"}}
	case apply.ResumabilityResolution:
		return []executionAction{{ID: executionActionAbandon, Label: "Abandon"}, {ID: executionActionBack, Label: "Back"}}
	}
	resume := executionAction{ID: executionActionResume, Label: "Resume"}
	if summary.Resumability == apply.ResumabilityLocked {
		resume.Disabled = true
		resume.Reason = "Execution is active in another Vanish process."
	} else if summary.Resumability == apply.ResumabilityWaitingRetry && !m.executionView.RetryNotBefore.IsZero() && time.Now().UTC().Before(m.executionView.RetryNotBefore) {
		resume.Disabled = true
		resume.Reason = "Retry time has not arrived."
	} else if summary.Resumability == apply.ResumabilityWaitingProvider {
		provider, err := builtInSimulationProviders.Resolve(m.executionView.Manifest.Platform, m.executionView.Manifest.Mode)
		if err != nil {
			resume.Disabled = true
			resume.Reason = "Execution provider is unavailable."
		} else {
			for _, prerequisite := range provider.Prerequisites(m.executionView.Plan, m.applyRuntimeState()) {
				if prerequisite.Blocking {
					resume.Disabled = true
					resume.Reason = prerequisite.Message
					break
				}
			}
		}
	}
	return []executionAction{resume, {ID: executionActionAbandon, Label: "Abandon"}, {ID: executionActionBack, Label: "Back"}}
}

func executionSummaryFromView(view apply.ExecutionView) apply.ExecutionSummary {
	return apply.ExecutionSummary{
		FormatVersion:   apply.ExecutionJournalFormatVersion,
		ExecutionID:     view.Manifest.ExecutionID,
		Fingerprint:     view.Manifest.Fingerprint,
		CreatedAt:       view.Manifest.CreatedAt,
		UpdatedAt:       view.UpdatedAt,
		SourceLabel:     view.Manifest.Summary.SourceLabel,
		Platform:        view.Manifest.Platform,
		Mode:            view.Manifest.Mode,
		State:           view.State,
		Resumability:    view.Resumability,
		BlockReason:     view.BlockReason,
		Counts:          view.Counts,
		LastSequence:    view.LastSequence,
		RecoveryWarning: view.RecoveryWarning,
	}
}

func (m Model) applyRuntimeState() apply.RuntimeState {
	return apply.NewRuntimeState(map[domain.PlatformName]apply.ConnectionState{
		domain.PlatformReddit: {Connected: m.redditConnected()},
	})
}

func (m Model) currentApplyPlan() domain.CleanupPlan {
	if m.applyPlanSource == applySourceLoaded {
		return m.loadedPlan
	}
	return m.planResult.Plan
}

func (m *Model) refreshManualAvailability(plan domain.CleanupPlan) {
	m.manualEligibleCount = 0
	m.manualUnavailable = 0
	if m.localWorkspace == nil || plan.Platform != domain.PlatformInstagram {
		return
	}
	session, unavailable, err := manualcleanup.New("availability", plan, nil, time.Now().UTC())
	if err == nil {
		m.manualEligibleCount = len(session.Actions)
	}
	m.manualUnavailable = len(unavailable)
}

func (m Model) generatedPlanMenuItems() []string {
	if m.manualEligibleCount > 0 && m.localWorkspace != nil {
		return []string{"Start manual cleanup", "Apply preview", "Export JSON", "Back"}
	}
	return planPreviewMenuItems
}

func (m Model) loadedPlanMenuItems() []string {
	if m.manualEligibleCount > 0 && m.localWorkspace != nil {
		return []string{"Start manual cleanup", "Apply preview", "View actions", "Back home"}
	}
	return loadedPlanSummaryMenuItems
}

func (m Model) manualCleanupStore() (manualcleanup.Store, bool) {
	if m.localWorkspace == nil {
		return manualcleanup.Store{}, false
	}
	return manualcleanup.NewStore(m.localWorkspace.Dir()), true
}

func (m Model) currentManualPlan(source applyPlanSource) domain.CleanupPlan {
	if source == applySourceLoaded {
		return m.loadedPlan
	}
	return m.planResult.Plan
}

func (m *Model) openManualCleanup(source applyPlanSource) {
	plan := m.currentManualPlan(source)
	previousPlanID := m.manualPreviewPlanID
	previousPreviews := m.manualPreviews
	m.clearManualSessionState()
	if previousPlanID == plan.ID {
		m.manualPreviews = previousPreviews
		m.manualPreviewPlanID = previousPlanID
	}
	m.manualPlanSource = source
	m.manualChoiceCursor = 0
	m.manualActionCursor = 0
	m.manualResultCursor = 0
	m.manualError = ""
	m.manualStatus = ""
	m.current = screenManualCleanupChoice
	store, ok := m.manualCleanupStore()
	if !ok {
		m.manualError = "Manual cleanup needs local progress storage."
		return
	}
	if session, found, err := store.Load(plan.ID); err != nil {
		m.manualError = "Manual cleanup progress could not be loaded."
		return
	} else if found {
		matches, err := manualcleanup.PlansEqual(plan, session.OriginalPlan())
		if err != nil || !matches {
			m.manualPreviews = nil
			m.manualPreviewPlanID = ""
			m.manualError = "Manual cleanup progress could not be loaded."
			return
		}
		m.manualSession = session
		m.manualSessionLoaded = true
		return
	}
	m.startNewManualCleanup(plan)
}

func (m *Model) startNewManualCleanup(plan domain.CleanupPlan) {
	store, ok := m.manualCleanupStore()
	if !ok {
		m.manualError = "Manual cleanup needs local progress storage."
		m.current = screenManualCleanupChoice
		return
	}
	id, err := manualcleanup.NewID()
	if err != nil {
		m.manualError = "Manual cleanup could not be started."
		return
	}
	session, unavailable, err := manualcleanup.New(id, plan, m.importResult.Items, time.Now().UTC())
	if err != nil {
		m.manualError = "No supported manual cleanup actions are available."
		return
	}
	if err := store.Start(session); err != nil {
		m.manualError = "Manual cleanup progress could not be saved."
		m.current = screenManualCleanupChoice
		return
	}
	m.manualSession = session
	m.manualSessionLoaded = true
	m.manualUnavailable = len(unavailable)
	m.manualPreviews = m.manualCommentPreviews(plan)
	m.manualPreviewPlanID = plan.ID
	m.manualError = ""
	m.manualStatus = ""
	m.appendAudit("manual_cleanup_session_started", m.manualAuditFields())
	m.current = screenManualCleanupAction
}

func (m Model) manualCommentPreviews(plan domain.CleanupPlan) map[string]string {
	items := make(map[string]domain.ActivityItem, len(m.importResult.Items))
	for _, item := range m.importResult.Items {
		items[item.ID] = item
	}
	previews := make(map[string]string)
	for _, action := range plan.Actions {
		item, ok := items[action.SourceActivityItemID]
		if !ok || item.Text == nil || strings.TrimSpace(item.Text.Preview) == "" {
			continue
		}
		if preview := instagram.SanitizeCommentPreview(item.Text.Preview); preview != "" {
			previews[action.ID] = preview
		}
	}
	return previews
}

func (m Model) manualChoiceItems() []string {
	if m.manualError != "" && !m.manualSessionLoaded {
		return []string{"Start over", "Back"}
	}
	if m.manualSession.State == manualcleanup.StateCompleted {
		return []string{"View cleanup result", "Start over", "Back"}
	}
	return []string{"Resume manual cleanup", "Start over", "Back"}
}

func (m Model) updateManualCleanupChoice(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.manualChoiceItems()
	switch {
	case key.Matches(msg, m.keys.up):
		m.manualChoiceCursor = moveCursor(m.manualChoiceCursor, len(items), -1)
	case key.Matches(msg, m.keys.down):
		m.manualChoiceCursor = moveCursor(m.manualChoiceCursor, len(items), 1)
	case key.Matches(msg, m.keys.back):
		m.returnToManualPlan()
	case key.Matches(msg, m.keys.selectItem):
		switch items[clampCursor(m.manualChoiceCursor, len(items))] {
		case "Resume manual cleanup":
			store, ok := m.manualCleanupStore()
			plan := m.currentManualPlan(m.manualPlanSource)
			if !ok || !m.manualSessionLoaded || m.manualSession.PlanID != plan.ID || store.Resume(&m.manualSession, time.Now().UTC()) != nil {
				m.manualError = "Manual cleanup progress could not be resumed."
				return m, nil
			}
			m.appendAudit("manual_cleanup_session_resumed", m.manualAuditFields())
			m.current = screenManualCleanupAction
		case "View cleanup result":
			m.current = screenManualCleanupResult
		case "Start over":
			plan := m.currentManualPlan(m.manualPlanSource)
			if strings.TrimSpace(plan.ID) == "" && m.manualSessionLoaded {
				plan = m.manualSession.OriginalPlan()
			}
			store, ok := m.manualCleanupStore()
			if ok && m.manualSessionLoaded && m.manualSession.PlanID == plan.ID {
				session, err := store.StartOver(plan.ID, time.Now().UTC())
				if err != nil {
					m.manualError = "Manual cleanup progress could not be saved."
					return m, nil
				}
				m.manualSession = session
				m.manualSessionLoaded = true
				m.manualError = ""
				m.manualStatus = ""
				m.appendAudit("manual_cleanup_session_started", m.manualAuditFields())
				m.current = screenManualCleanupAction
				return m, nil
			}
			m.startNewManualCleanup(plan)
		case "Back":
			m.returnToManualPlan()
		}
	}
	return m, nil
}

func (m Model) updateManualCleanupAction(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.up):
		m.manualActionCursor = moveCursor(m.manualActionCursor, len(manualCleanupActionItems), -1)
	case key.Matches(msg, m.keys.down):
		m.manualActionCursor = moveCursor(m.manualActionCursor, len(manualCleanupActionItems), 1)
	case key.Matches(msg, m.keys.back):
		m.stopManualCleanup()
	case key.Matches(msg, m.keys.selectItem):
		action, ok := m.manualSession.Current()
		if !ok {
			m.current = screenManualCleanupResult
			return m, nil
		}
		switch m.manualActionCursor {
		case 0:
			m.manualError = ""
			return m, openManualTargetCmd(action)
		case 1:
			m.markManualCleanup(manualcleanup.OutcomeDone)
		case 2:
			m.markManualCleanup(manualcleanup.OutcomeSkipped)
		case 3:
			m.stopManualCleanup()
		}
	}
	return m, nil
}

func (m Model) updateManualCleanupResult(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.selectItem) || key.Matches(msg, m.keys.back) {
		m.returnToManualPlan()
	}
	return m, nil
}

func (m *Model) markManualCleanup(outcome manualcleanup.Outcome) {
	store, ok := m.manualCleanupStore()
	if !ok {
		m.manualError = "Manual cleanup progress could not be saved."
		return
	}
	action, hasAction := m.manualSession.Current()
	if !hasAction {
		m.current = screenManualCleanupResult
		return
	}
	completed, err := store.Mark(&m.manualSession, outcome, time.Now().UTC())
	if err != nil {
		m.manualError = "Manual cleanup progress could not be saved."
		return
	}
	eventType := "manual_cleanup_action_done"
	if outcome == manualcleanup.OutcomeSkipped {
		eventType = "manual_cleanup_action_skipped"
	}
	fields := m.manualAuditFields()
	fields["action_id"] = action.ActionID
	fields["action_type"] = string(action.Type)
	m.appendAudit(eventType, fields)
	if completed {
		m.appendAudit("manual_cleanup_session_completed", m.manualAuditFields())
		m.current = screenManualCleanupResult
	}
	m.manualActionCursor = 0
	m.manualError = ""
}

func (m *Model) stopManualCleanup() {
	store, ok := m.manualCleanupStore()
	if !ok || store.Stop(&m.manualSession, time.Now().UTC()) != nil {
		m.manualError = "Manual cleanup progress could not be saved."
		return
	}
	m.appendAudit("manual_cleanup_session_stopped", m.manualAuditFields())
	m.manualStatus = "Manual cleanup stopped. Progress saved."
	m.returnToManualPlan()
}

func (m *Model) returnToManualPlan() {
	if m.manualPlanSource == applySourceLoaded {
		if strings.TrimSpace(m.loadedPlan.ID) == "" && m.manualSessionLoaded {
			m.loadedPlan = m.manualSession.OriginalPlan()
			m.loadedPlanSummary = domain.SummarizeCleanupPlan(m.loadedPlan)
			m.refreshManualAvailability(m.loadedPlan)
		}
		m.current = screenLoadedPlanSummary
		return
	}
	m.current = screenPlanPreview
}

func (m *Model) clearManualSessionState() {
	m.manualSession = manualcleanup.Session{}
	m.manualSessionLoaded = false
	m.manualPreviews = nil
	m.manualPreviewPlanID = ""
}

func (m Model) manualAuditFields() map[string]any {
	done, skipped, pending := m.manualSession.Counts()
	return map[string]any{
		"plan_id":       m.manualSession.PlanID,
		"mode":          string(m.manualSession.Mode),
		"position":      m.manualSession.CurrentPosition,
		"action_count":  len(m.manualSession.Actions),
		"done_count":    done,
		"skipped_count": skipped,
		"pending_count": pending,
		"state":         string(m.manualSession.State),
	}
}

func (m Model) currentApplyPreviewMenuItems() []string {
	if m.applyPreview.CanApply {
		return applyPreviewMenuItems
	}
	return applyPreviewBlockedMenuItems
}

func (m *Model) returnToApplySource() {
	if m.applyPlanSource == applySourceLoaded {
		m.current = screenLoadedPlanSummary
		return
	}
	m.current = screenPlanPreview
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
	m.redditConnectCursor = clampCursor(m.redditConnectCursor, len(m.redditConnectActions()))
	m.current = screenRedditConnect
}

func (m *Model) ensureRedditAuthURL() bool {
	if strings.TrimSpace(m.redditAuthURL) != "" {
		return true
	}
	clientID, err := reddit.ClientIDFromEnv()
	if err != nil {
		m.redditError = "Reddit sign-in is not configured. Set VANISH_REDDIT_CLIENT_ID and try again."
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

func (m Model) redditConnectActions() []redditAction {
	if m.redditConnected() {
		return []redditAction{
			m.redditCapabilityAction(redditActionScan, "Scan activity", platform.ActionScanActivity),
			{ID: redditActionDisconnect, Label: "Disconnect"},
			{ID: redditActionBack, Label: "Back"},
		}
	}
	return []redditAction{
		m.redditCapabilityAction(redditActionSignIn, "Sign in with Reddit", platform.ActionConnectAccount),
		{ID: redditActionBack, Label: "Back"},
	}
}

func (m Model) redditCapabilityAction(id redditActionID, label, platformActionID string) redditAction {
	action := redditAction{ID: id, Label: label, Disabled: true, Reason: "Action is unavailable."}
	current, ok := builtInPlatforms.Get(platform.PlatformReddit)
	if !ok {
		return action
	}
	metadata, ok := current.Action(platformActionID)
	if !ok {
		return action
	}
	available, reason := current.ActionAvailable(metadata)
	action.Disabled = !available
	if strings.TrimSpace(reason) != "" {
		action.Reason = reason
	}
	return action
}

func redditActionLabels(actions []redditAction) []string {
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		labels = append(labels, action.Label)
	}
	return labels
}

func redditActionRows(actions []redditAction) ([]string, map[int]bool) {
	labels := redditActionLabels(actions)
	disabled := make(map[int]bool)
	for i, action := range actions {
		if action.Disabled {
			disabled[i] = true
		}
	}
	if len(disabled) == 0 {
		disabled = nil
	}
	return labels, disabled
}

func (m Model) redditHomeDetailLines() []string {
	if m.redditConnected() {
		return []string{
			m.styles.body.Render(fmt.Sprintf("Signed in as u/%s", m.localConfig.Reddit.Username)),
			m.styles.muted.Render("Scan your Reddit activity next."),
		}
	}
	return []string{m.styles.body.Render("Sign in to scan your Reddit activity.")}
}

func (m Model) redditConnectionRows() []string {
	lines := []string{}
	if strings.TrimSpace(m.redditStatus) != "" {
		lines = append(lines, m.notice("success", m.redditStatus), "")
	}
	if strings.TrimSpace(m.redditError) != "" {
		lines = append(lines, m.notice("error", m.redditError), "")
		if redditClientIDMissing() || strings.Contains(m.redditError, reddit.ClientIDEnv) {
			lines = append(lines, m.redditSetupHintLines()...)
			lines = append(lines, "")
		}
		if strings.TrimSpace(m.redditAuthURL) != "" {
			lines = append(lines, m.styles.muted.Render(m.redditAuthURL), "")
		}
	}
	config := m.localConfig.Reddit
	if config == nil || strings.TrimSpace(config.Username) == "" {
		lines = append(lines, m.styles.body.Render("Sign in to scan your Reddit activity."))
		if redditClientIDMissing() && strings.TrimSpace(m.redditError) == "" {
			lines = append(lines, "")
			lines = append(lines, m.redditSetupHintLines()...)
		}
	} else {
		lines = append(lines, m.styles.body.Render(fmt.Sprintf("Signed in as u/%s", config.Username)))
		lines = append(lines, m.styles.muted.Render("Ready to scan your comments and posts."))
	}
	return lines
}

func (m Model) redditSetupHintLines() []string {
	return []string{
		m.styles.separator.Render("Setup"),
		m.styles.body.Render("Create a Reddit installed app."),
		m.styles.body.Render("Set " + reddit.ClientIDEnv + " to its client ID."),
		m.styles.body.Render("Redirect URI: " + reddit.DefaultRedirectURI),
	}
}

func redditClientIDMissing() bool {
	return strings.TrimSpace(os.Getenv(reddit.ClientIDEnv)) == ""
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
	m.redditBusyDetail = "Scanning your Reddit activity..."
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
	m.redditBusyDetail = "Disconnecting Reddit..."
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
		WarningCount:   result.WarningCount,
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

func (m *Model) recordApplyPreviewed(preview apply.Preview) {
	m.appendAudit(string(apply.EventPreviewed), applyPreviewAuditFields(preview))
	m.refreshLocalData()
}

func (m *Model) recordApplyConfirmed(preview apply.Preview) {
	m.appendAudit(string(apply.EventConfirmed), applyPreviewAuditFields(preview))
	m.refreshLocalData()
}

func (m *Model) recordApplyExecution(execution apply.Execution) {
	for _, event := range execution.Events {
		m.appendAudit(string(event.Type), applyEventAuditFields(event))
	}
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
	m.executionSummaries = nil
	m.executionView = apply.ExecutionView{}
	m.executionSelected = apply.ExecutionSummary{}
	m.executionCursor = 0
	m.executionOffset = 0
	m.executionError = ""
	m.manualSession = manualcleanup.Session{}
	m.manualSessionLoaded = false
	m.manualPreviews = nil
	m.manualPreviewPlanID = ""
	m.manualEligibleCount = 0
	m.manualUnavailable = 0
	m.manualStatus = ""
	m.manualError = ""
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
	total := m.visibleItemCount()
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
	bodyCapacity := paneBodyLineCapacity(parsedItemsPaneHeight(spec), "Parsed Items", "Review and toggle")
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
	itemCount := m.visibleItemCount()
	if itemCount == 0 {
		m.itemCursor = 0
		m.itemOffset = 0
		return
	}
	viewport := m.parsedItemsViewport()
	visibleRows := viewport.VisibleRows
	maxOffset := maxInt(0, itemCount-visibleRows)
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
	m.itemCursor = clampCursor(nextOffset, itemCount)
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
	Items        []domain.ActivityItem
	Summary      activitySummary
	WarningCount int
	Warnings     []activityWarningGroup
}

type activityWarningGroup struct {
	SourceFile string
	Category   string
	Reason     string
	Unit       string
	Count      int
	Examples   []string
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
	state    string
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

type applyRunFinishedMsg struct {
	source    applyPlanSource
	execution apply.Execution
	err       error
}

type executionLoadedMsg struct {
	view    apply.ExecutionView
	summary apply.ExecutionSummary
	err     error
}

type executionRunFinishedMsg struct {
	execution apply.Execution
	err       error
}

type executionAbandonedMsg struct {
	execution apply.Execution
	err       error
}

type executionDeletedMsg struct{ err error }

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
	warnings := make([]activityWarningGroup, 0, len(result.Warnings.Groups))
	for _, group := range result.Warnings.Groups {
		warnings = append(warnings, activityWarningGroup{
			SourceFile: group.SourceFile,
			Category:   group.Category,
			Reason:     group.Reason,
			Unit:       string(group.Unit),
			Count:      group.Count,
			Examples:   append([]string(nil), group.Examples...),
		})
	}
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
		WarningCount: result.Warnings.Total,
		Warnings:     warnings,
	}
}

func activityResultFromReddit(result reddit.ScanResult) activityResult {
	warnings := make([]activityWarningGroup, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, activityWarningGroup{
			Category: "reddit",
			Reason:   warning,
			Unit:     "warning",
			Count:    1,
		})
	}
	return activityResult{
		Items: result.Items,
		Summary: activitySummary{
			Total:    result.Summary.Total,
			Comments: result.Summary.Comments,
			Posts:    result.Summary.Posts,
			Skipped:  result.Summary.Skipped,
		},
		WarningCount: len(result.Warnings),
		Warnings:     warnings,
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

func runApplyCmd(plan domain.CleanupPlan, source applyPlanSource, runner apply.Runner) tea.Cmd {
	return func() tea.Msg {
		execution, err := runner.Start(context.Background(), plan, apply.ExecutionModeSimulation)
		return applyRunFinishedMsg{
			source:    source,
			execution: execution,
			err:       err,
		}
	}
}

func loadExecutionCmd(store *apply.ExecutionStore, id apply.ExecutionID, summary apply.ExecutionSummary) tea.Cmd {
	return func() tea.Msg {
		if store == nil {
			return executionLoadedMsg{summary: summary, err: apply.ErrExecutionStoreUnavailable}
		}
		if id == "" && summary.Resumability == apply.ResumabilityCorrupt {
			return executionLoadedMsg{summary: summary, err: apply.ErrExecutionCorrupt}
		}
		view, err := store.Replay(id)
		return executionLoadedMsg{view: view, summary: summary, err: err}
	}
}

func resumeExecutionCmd(runner apply.Runner, id apply.ExecutionID) tea.Cmd {
	return func() tea.Msg {
		execution, err := runner.Resume(context.Background(), id)
		return executionRunFinishedMsg{execution: execution, err: err}
	}
}

func abandonExecutionCmd(runner apply.Runner, id apply.ExecutionID) tea.Cmd {
	return func() tea.Msg {
		execution, err := runner.Abandon(id)
		return executionAbandonedMsg{execution: execution, err: err}
	}
}

func deleteExecutionCmd(store *apply.ExecutionStore, summary apply.ExecutionSummary) tea.Cmd {
	return func() tea.Msg {
		if store == nil {
			return executionDeletedMsg{err: apply.ErrExecutionStoreUnavailable}
		}
		return executionDeletedMsg{err: store.Delete(summary)}
	}
}

func redditConnectCmd(input, state string, allowFileFallback bool, appDir string) tea.Cmd {
	return func() tea.Msg {
		code, err := redditCodeFromInput(input, state)
		if err != nil {
			return redditConnectFinishedMsg{state: state, err: err}
		}
		return redditConnectWithCode(code, state, allowFileFallback, appDir)
	}
}

func redditConnectWithCode(code, state string, allowFileFallback bool, appDir string) redditConnectFinishedMsg {
	if err := ensureRedditSecretStoreReady(allowFileFallback, appDir); err != nil {
		return redditConnectFinishedMsg{state: state, err: err}
	}
	oauth, err := newRedditOAuth(allowFileFallback, appDir)
	if err != nil {
		return redditConnectFinishedMsg{state: state, err: err}
	}
	tokens, err := oauth.ExchangeCode(context.Background(), code)
	if err != nil {
		return redditConnectFinishedMsg{state: state, err: err}
	}
	client, err := reddit.NewClient(tokens.Access, reddit.ClientOptions{})
	if err != nil {
		return redditConnectFinishedMsg{state: state, err: err}
	}
	user, err := client.Me(context.Background())
	if err != nil {
		return redditConnectFinishedMsg{state: state, err: err}
	}
	result, err := oauth.SaveRefreshToken(user.Name, tokens)
	if err != nil {
		return redditConnectFinishedMsg{state: state, err: err}
	}
	metadata := reddit.WorkspaceMetadata(user.Name, tokens, result, time.Now().UTC())
	return redditConnectFinishedMsg{state: state, username: user.Name, metadata: metadata}
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
		message := "Disconnected Reddit."
		if revoke {
			refresh, _, err := oauth.LoadRefreshToken(config.Username)
			if err == nil {
				if err := oauth.Revoke(context.Background(), refresh); err != nil {
					return redditDisconnectFinishedMsg{err: err}
				}
				message = "Disconnected Reddit."
			} else if errors.Is(err, secretstore.ErrNotFound) {
				message = "Disconnected Reddit locally."
			} else {
				return redditDisconnectFinishedMsg{err: err}
			}
		}
		if err := oauth.ForgetLocal(config.Username); err != nil && !errors.Is(err, secretstore.ErrNotFound) {
			if !revoke && errors.Is(err, secretstore.ErrUnavailable) {
				return redditDisconnectFinishedMsg{message: "Disconnected Reddit locally."}
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

type instagramExportPageOpenedMsg struct {
	err error
}

type manualTargetOpenedMsg struct {
	actionID   string
	actionType domain.ActionType
	targetKind instagram.TargetKind
	err        error
}

func openInstagramExportPageCmd() tea.Cmd {
	return func() tea.Msg {
		return instagramExportPageOpenedMsg{err: openExternalURL(instagramExportPageURL)}
	}
}

func openManualTargetCmd(action manualcleanup.Action) tea.Cmd {
	return func() tea.Msg {
		target, err := instagram.ValidateCleanupTarget(action.Type, action.TargetURL, action.TargetID)
		if err == nil {
			err = openExternalURL(target.URL)
		}
		return manualTargetOpenedMsg{
			actionID:   action.ActionID,
			actionType: action.Type,
			targetKind: action.TargetKind,
			err:        err,
		}
	}
}

func (m *Model) openInstagramZIPPicker(returnScreen screen) {
	m.importPlatform = domain.PlatformInstagram
	m.importReturnScreen = returnScreen
	m.current = screenImportPath
	if strings.TrimSpace(m.importPickerDir) == "" {
		m.openImportPicker(initialImportPickerDir())
	}
}

func activitySummaryKeyValues(summary activitySummary) []keyValue {
	rows := []keyValue{{Key: "Total", Value: compactCount(summary.Total)}}
	for _, row := range []struct {
		label string
		count int
	}{
		{"Likes", summary.Likes},
		{"Comments", summary.Comments},
		{"Posts", summary.Posts},
		{"Following", summary.Following},
		{"Followers", summary.Followers},
	} {
		if row.count > 0 {
			rows = append(rows, keyValue{Key: row.label, Value: compactCount(row.count)})
		}
	}
	return rows
}

func startSpinnerCmd(spinnerModel spinner.Model) tea.Cmd {
	return func() tea.Msg {
		return spinnerModel.Tick()
	}
}

func itemRow(item domain.ActivityItem) string {
	return itemRowForWidth(item, defaultTerminalWidth)
}

func itemRowForWidth(item domain.ActivityItem, width int) string {
	innerWidth := paneTextWidth(width)
	if innerWidth >= 40 {
		remaining := innerWidth - 22
		actorWidth := minInt(18, maxInt(8, remaining/2))
		targetWidth := maxInt(8, remaining-actorWidth)
		return fixedWidthRow(
			fixedColumn{Text: activityTypeLabel(item), Width: 9},
			fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: actorWidth},
			fixedColumn{Text: targetListLabel(item.TargetURL, item.TargetID), Width: targetWidth},
			fixedColumn{Text: compactTime(item.OccurredAt), Width: 10},
		)
	}
	if innerWidth >= 32 {
		remaining := innerWidth - 11
		actorWidth := minInt(16, maxInt(8, remaining/2))
		targetWidth := maxInt(8, remaining-actorWidth)
		return fixedWidthRow(
			fixedColumn{Text: activityTypeLabel(item), Width: 9},
			fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: actorWidth},
			fixedColumn{Text: targetListLabel(item.TargetURL, item.TargetID), Width: targetWidth},
		)
	}
	actorWidth := maxInt(8, innerWidth-10)
	return fixedWidthRow(
		fixedColumn{Text: activityTypeLabel(item), Width: 9},
		fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: actorWidth},
	)
}

func (m Model) selectableItemRow(item domain.ActivityItem) string {
	return m.selectableItemRowForWidth(item, defaultTerminalWidth)
}

func (m Model) selectableItemRowForWidth(item domain.ActivityItem, width int) string {
	marker := "[ ]"
	if m.selection.Contains(item.ID) {
		marker = "[x]"
	}
	innerWidth := paneTextWidth(width)
	if innerWidth >= 46 {
		remaining := innerWidth - 26
		actorWidth := minInt(18, maxInt(8, remaining/2))
		targetWidth := maxInt(8, remaining-actorWidth)
		return fixedWidthRow(
			fixedColumn{Text: marker, Width: 3},
			fixedColumn{Text: activityTypeLabel(item), Width: 9},
			fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: actorWidth},
			fixedColumn{Text: targetListLabel(item.TargetURL, item.TargetID), Width: targetWidth},
			fixedColumn{Text: compactTime(item.OccurredAt), Width: 10},
		)
	}
	if innerWidth >= 36 {
		remaining := innerWidth - 15
		actorWidth := minInt(16, maxInt(8, remaining/2))
		targetWidth := maxInt(8, remaining-actorWidth)
		return fixedWidthRow(
			fixedColumn{Text: marker, Width: 3},
			fixedColumn{Text: activityTypeLabel(item), Width: 9},
			fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: actorWidth},
			fixedColumn{Text: targetListLabel(item.TargetURL, item.TargetID), Width: targetWidth},
		)
	}
	actorWidth := maxInt(8, innerWidth-14)
	return fixedWidthRow(
		fixedColumn{Text: marker, Width: 3},
		fixedColumn{Text: activityTypeLabel(item), Width: 9},
		fixedColumn{Text: emptyFallback(item.Actor, "-"), Width: actorWidth},
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
	return planPreviewRowsForWidth(actions, skipped, defaultTerminalWidth)
}

func planPreviewRowsForWidth(actions []domain.CleanupAction, skipped []planBuildSkip, width int) []string {
	rows := make([]string, 0, len(actions)+len(skipped))
	for _, action := range actions {
		rows = append(rows, planActionListRowForWidth(action.Type, action.Status, action.TargetURL, action.TargetID, action.SourceActivityItemID, width))
	}
	for _, skip := range skipped {
		rows = append(rows, skippedPlanRowForWidth(skip, width))
	}
	return rows
}

func planActionRow(action domain.CleanupAction) string {
	return planActionRowForWidth(action, defaultTerminalWidth)
}

func planActionRowForWidth(action domain.CleanupAction, width int) string {
	return planActionListRowForWidth(action.Type, action.Status, action.TargetURL, action.TargetID, action.SourceActivityItemID, width)
}

func (m Model) applyActionSummaryLines(preview apply.Preview, width int) []string {
	innerWidth := maxInt(10, paneTextWidth(width))
	rows := []string{
		m.styles.body.Render(fmt.Sprintf("Pending: %d", preview.PendingCount)),
		m.styles.body.Render(fmt.Sprintf("Unsupported: %d", preview.UnsupportedCount)),
	}
	if preview.UnsupportedCount > 0 && len(preview.Unsupported) > 0 {
		rows = append(rows, "")
		for _, unsupported := range preview.Unsupported {
			row := fmt.Sprintf("%s | %s | %s", unsupported.Status, unsupported.Type, unsupported.Reason)
			rows = append(rows, m.styles.body.Render(truncateEnd(row, innerWidth)))
			if len(rows) >= 7 {
				break
			}
		}
	}
	return rows
}

func applyResultRow(result apply.ActionResult) string {
	outcome := actionOutcomeLabel(result.Outcome)
	if result.Outcome == apply.OutcomeSucceeded || result.Outcome == "" {
		outcome = string(result.Status)
	}
	attempt := "-"
	if result.Attempt > 1 {
		attempt = fmt.Sprintf("attempt %d", result.Attempt)
	}
	return fixedWidthRow(
		fixedColumn{Text: outcome, Width: 20},
		fixedColumn{Text: string(result.Type), Width: 22},
		fixedColumn{Text: attempt, Width: 12},
		fixedColumn{Text: emptyFallback(result.ActionID, "-"), Width: 18},
	)
}

func finalActionResults(results []apply.ActionResult) []apply.ActionResult {
	final := make([]apply.ActionResult, 0, len(results))
	indexes := make(map[string]int, len(results))
	for _, result := range results {
		if index, ok := indexes[result.ActionID]; ok {
			final[index] = result
			continue
		}
		indexes[result.ActionID] = len(final)
		final = append(final, result)
	}
	return final
}

func applyOutcomeDetailRows(execution apply.Execution) []keyValue {
	result, ok := relevantOutcomeResult(execution.Results)
	if !ok {
		return nil
	}
	rows := []keyValue{{Key: "Reason", Value: actionOutcomeLabel(result.Outcome)}}
	if result.Outcome != apply.OutcomeSucceeded && strings.TrimSpace(result.Message) != "" {
		rows = append(rows, keyValue{Key: "Message", Value: result.Message})
	}
	if result.Attempt > 1 {
		rows = append(rows, keyValue{Key: "Attempt", Value: compactCount(result.Attempt)})
	}
	if result.RetryAfter > 0 {
		rows = append(rows, keyValue{Key: "Retry after", Value: result.RetryAfter.String()})
	}
	return rows
}

func relevantOutcomeResult(results []apply.ActionResult) (apply.ActionResult, bool) {
	for i := len(results) - 1; i >= 0; i-- {
		result := results[i]
		if result.Outcome != apply.OutcomeSucceeded || result.Attempt > 1 {
			return result, true
		}
	}
	return apply.ActionResult{}, false
}

func actionOutcomeLabel(outcome apply.ActionOutcome) string {
	switch outcome {
	case apply.OutcomeSucceeded:
		return "succeeded"
	case apply.OutcomeAlreadySatisfied:
		return "already satisfied"
	case apply.OutcomeRetryableFailure:
		return "retryable failure"
	case apply.OutcomePermanentFailure:
		return "permanent failure"
	case apply.OutcomeRateLimited:
		return "rate limited"
	case apply.OutcomeAuthenticationRequired:
		return "authentication required"
	case apply.OutcomeStopped:
		return "stopped"
	case apply.OutcomeCancelled:
		return "cancelled"
	default:
		return "failed safely"
	}
}

func readyLabel(ready bool) string {
	if ready {
		return "ready"
	}
	return "not ready"
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
	return planActionListRowForWidth(actionType, status, targetURL, targetID, sourceID, defaultTerminalWidth)
}

func planActionListRowForWidth(actionType domain.ActionType, status domain.ActionStatus, targetURL, targetID, sourceID string, width int) string {
	innerWidth := paneTextWidth(width)
	if innerWidth >= 68 {
		return fixedWidthRow(
			fixedColumn{Text: string(actionType), Width: 14},
			fixedColumn{Text: string(status), Width: 9},
			fixedColumn{Text: targetListLabel(targetURL, targetID), Width: 26},
			fixedColumn{Text: shortID(sourceID), Width: 16},
		)
	}
	parts := []string{string(actionType), string(status)}
	if innerWidth >= 36 {
		parts = append(parts, targetListLabel(targetURL, targetID))
	}
	if innerWidth >= 58 && strings.TrimSpace(sourceID) != "" {
		parts = append(parts, shortID(sourceID))
	}
	return truncateEnd(strings.Join(parts, " · "), innerWidth)
}

func skippedPlanRowForWidth(skip planBuildSkip, width int) string {
	innerWidth := paneTextWidth(width)
	reason := emptyFallback(skip.Reason, "unsupported")
	target := emptyFallback(skip.TargetRef, "-")
	if innerWidth >= 68 {
		return fixedWidthRow(
			fixedColumn{Text: "skipped", Width: 14},
			fixedColumn{Text: reason, Width: 12},
			fixedColumn{Text: target, Width: 26},
			fixedColumn{Text: emptyFallback(skip.SourceActivityItemID, "-"), Width: 16},
		)
	}
	parts := []string{"skipped", reason}
	if innerWidth >= 58 {
		parts = append(parts, target)
	}
	return truncateEnd(strings.Join(parts, " · "), innerWidth)
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
		domain.ActionStatusStopped,
		domain.ActionStatusCancelled,
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
		if m.visibleItemCount() == 0 {
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

func executionSummaryRow(summary apply.ExecutionSummary) string {
	label := emptyFallback(strings.TrimSpace(summary.SourceLabel), "Execution")
	return fmt.Sprintf("%-18s  %-10s  %s", truncateEnd(label, 18), summary.Platform, executionStateLabel(summary))
}

func executionSummaryDetails(summary apply.ExecutionSummary) []string {
	values := executionSummaryKeyValues(summary)
	values = append(values, executionCountKeyValues(summary.Counts)...)
	lines := make([]string, 0, len(values)+2)
	for _, value := range values {
		lines = append(lines, fmt.Sprintf("%s: %s", value.Key, value.Value))
	}
	if summary.RecoveryWarning != "" {
		lines = append(lines, "Warning: "+summary.RecoveryWarning)
	}
	return lines
}

func executionSummaryKeyValues(summary apply.ExecutionSummary) []keyValue {
	return []keyValue{
		{Key: "Platform", Value: emptyFallback(string(summary.Platform), "-")},
		{Key: "Plan", Value: emptyFallback(summary.SourceLabel, "-")},
		{Key: "State", Value: executionStateLabel(summary)},
		{Key: "Updated", Value: formatPlanTime(summary.UpdatedAt)},
	}
}

func executionCountKeyValues(counts apply.ResultCounts) []keyValue {
	return []keyValue{
		{Key: "Done", Value: compactCount(counts.Done)},
		{Key: "Failed", Value: compactCount(counts.Failed)},
		{Key: "Skipped", Value: compactCount(counts.Skipped)},
		{Key: "Pending", Value: compactCount(counts.Pending)},
	}
}

func executionStateLabel(summary apply.ExecutionSummary) string {
	switch summary.Resumability {
	case apply.ResumabilityResolution:
		return "Resolution required"
	case apply.ResumabilityWaitingRetry:
		return "Waiting"
	case apply.ResumabilityWaitingProvider:
		return "Reconnect required"
	case apply.ResumabilityCorrupt:
		return "Corrupt"
	case apply.ResumabilityLocked:
		return "In use"
	case apply.ResumabilityTerminal:
		return strings.Title(string(summary.State))
	default:
		return "Resumable"
	}
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
		"stopped_count":        summary.StatusCounts[domain.ActionStatusStopped],
		"cancelled_count":      summary.StatusCounts[domain.ActionStatusCancelled],
	}
}

func applyPreviewAuditFields(preview apply.Preview) map[string]any {
	fields := map[string]any{
		"plan_id":           preview.PlanID,
		"platform":          string(preview.Platform),
		"execution_mode":    string(preview.Mode),
		"action_count":      preview.Summary.TotalActions,
		"pending_count":     preview.PendingCount,
		"unsupported_count": preview.UnsupportedCount,
		"failed_count":      preview.Summary.StatusCounts[domain.ActionStatusFailed],
		"skipped_count":     preview.Summary.StatusCounts[domain.ActionStatusSkipped],
		"provider_ready":    preview.ProviderReady,
		"can_apply":         preview.CanApply,
	}
	if preview.Executor != "" {
		fields["executor"] = string(preview.Executor)
	}
	return fields
}

func applyEventAuditFields(event apply.ExecutionEvent) map[string]any {
	fields := map[string]any{
		"plan_id":  event.PlanID,
		"platform": string(event.Platform),
	}
	if event.Mode != "" {
		fields["execution_mode"] = string(event.Mode)
	}
	if event.Executor != "" {
		fields["executor"] = string(event.Executor)
	}
	if event.ActionID != "" {
		fields["action_id"] = event.ActionID
	}
	if event.ActionType != "" {
		fields["action_type"] = string(event.ActionType)
	}
	if event.Status != "" {
		fields["status"] = string(event.Status)
	}
	if event.State != "" {
		fields["state"] = string(event.State)
	}
	if event.Outcome != "" {
		fields["outcome"] = string(event.Outcome)
		fields["attempt"] = event.Attempt
		fields["retryable"] = event.Retryable
	}
	if event.RetryAfter > 0 {
		fields["retry_after_ms"] = retryAfterMilliseconds(event.RetryAfter)
	}
	if event.ProviderCode != "" && event.ProviderCode.Known() {
		fields["provider_code"] = string(event.ProviderCode)
	}
	if event.HaltReason != "" {
		fields["halt_reason"] = string(event.HaltReason)
	}
	if event.ExecutionID != "" {
		fields["execution_id"] = string(event.ExecutionID)
	}
	if event.Sequence > 0 {
		fields["execution_sequence"] = event.Sequence
	}
	if event.Type == apply.EventExecutionStarted || event.Type == apply.EventExecutionFinished {
		fields["pending_count"] = event.Counts.Pending
		fields["running_count"] = event.Counts.Running
		fields["done_count"] = event.Counts.Done
		fields["failed_count"] = event.Counts.Failed
		fields["skipped_count"] = event.Counts.Skipped
		fields["stopped_count"] = event.Counts.Stopped
		fields["cancelled_count"] = event.Counts.Cancelled
	}
	return fields
}

func retryAfterMilliseconds(duration time.Duration) int64 {
	milliseconds := int64(duration / time.Millisecond)
	if duration%time.Millisecond != 0 {
		milliseconds++
	}
	return milliseconds
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
		return secretstore.ErrUnavailable
	}
	if strings.TrimSpace(appDir) == "" {
		return errors.New("Vanish app directory is required")
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
		return "Credential store unavailable. Check your system credential store, then try again."
	case errors.Is(err, secretstore.ErrFallbackConfirmationRequired):
		return "Credential store unavailable. Check your system credential store, then try again."
	case strings.Contains(err.Error(), reddit.ClientIDEnv):
		return "Reddit sign-in is not configured. Set VANISH_REDDIT_CLIENT_ID and try again."
	case strings.Contains(err.Error(), "open reddit sign-in URL"):
		return "Could not open Reddit automatically. Copy the URL into your browser."
	case strings.Contains(err.Error(), "address already in use"):
		return "Reddit sign-in is already waiting. Close the other Vanish window and try again."
	case strings.Contains(err.Error(), "timed out"):
		return "Reddit sign-in timed out. Try again."
	case strings.Contains(err.Error(), "cancelled"):
		return "Reddit sign-in cancelled."
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

func (m Model) visibleItemCount() int {
	return m.itemIndex.len(m.importResult.Items)
}

func (m Model) visibleItemAt(index int) (domain.ActivityItem, bool) {
	return m.itemIndex.item(m.importResult.Items, index)
}

func (m *Model) toggleVisibleItemSelection(visibleIndex int) bool {
	item, ok := m.visibleItemAt(visibleIndex)
	if !ok {
		return false
	}
	selected := m.selection.Toggle(item.ID)
	if selected {
		adjustSelectionCounts(&m.selectionCounts, item, 1)
	} else {
		adjustSelectionCounts(&m.selectionCounts, item, -1)
	}
	m.selectedItemsDirty = true
	return selected
}

func (m *Model) setVisibleItemsSelected(selected bool) {
	for visibleIndex := 0; visibleIndex < m.visibleItemCount(); visibleIndex++ {
		item, ok := m.visibleItemAt(visibleIndex)
		if !ok || m.selection.Contains(item.ID) == selected {
			continue
		}
		if selected {
			m.selection.Select(item.ID)
			adjustSelectionCounts(&m.selectionCounts, item, 1)
		} else {
			m.selection.Deselect(item.ID)
			adjustSelectionCounts(&m.selectionCounts, item, -1)
		}
	}
	m.selectedItemsDirty = true
}

func (m *Model) clearSelection() {
	m.selection.Clear()
	m.selectionCounts = domain.ActivitySelectionCounts{}
	m.selectedItemIndexes = m.selectedItemIndexes[:0]
	m.selectedItemsDirty = false
}

func (m *Model) rebuildSelectedItemIndexes() {
	m.selectedItemIndexes = m.selectedItemIndexes[:0]
	m.selectionCounts = domain.ActivitySelectionCounts{}
	for sourceIndex, item := range m.importResult.Items {
		if !m.selection.Contains(item.ID) {
			continue
		}
		m.selectedItemIndexes = append(m.selectedItemIndexes, sourceIndex)
		adjustSelectionCounts(&m.selectionCounts, item, 1)
	}
	m.selectedItemsDirty = false
}

func (m Model) selectedItemAt(index int) (domain.ActivityItem, bool) {
	if index < 0 || index >= len(m.selectedItemIndexes) {
		return domain.ActivityItem{}, false
	}
	return m.importResult.Items[m.selectedItemIndexes[index]], true
}

func (m *Model) selectedItemsCopy() []domain.ActivityItem {
	m.rebuildSelectedItemIndexes()
	items := make([]domain.ActivityItem, 0, len(m.selectedItemIndexes))
	for _, sourceIndex := range m.selectedItemIndexes {
		items = append(items, m.importResult.Items[sourceIndex])
	}
	return items
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
	m.itemIndex.reset()
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
	m.itemIndex.rebuild(m.importResult.Items, m.itemFilter)
	m.filterError = ""
	itemCount := m.visibleItemCount()
	m.itemCursor = clampCursor(m.itemCursor, itemCount)
	m.itemOffset = ensureOffset(m.itemCursor, m.itemOffset, itemCount, m.parsedItemsViewport().VisibleRows)
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
		actionRows, _ := platformActionRows(current)
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, actionRows)...)
	case screenInstagramExportGuide:
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, instagramGuideMenuItems)...)
	case screenRedditConnect:
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, redditActionLabels(m.redditConnectActions()))...)
	case screenRedditSigningIn:
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, []string{"Cancel"})...)
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
		boxes = append(boxes, rowHitBoxes(content, hitPlanPreviewAction, 0, m.generatedPlanMenuItems())...)
	case screenLoadedPlanSummary:
		boxes = append(boxes, rowHitBoxes(content, hitLoadedPlanAction, 0, m.loadedPlanMenuItems())...)
	case screenLoadedPlanActions:
		offset, rows := m.loadedPlanActionRowsForViewport()
		boxes = append(boxes, rowHitBoxes(content, hitLoadedPlanRow, offset, rows)...)
	case screenApplyPreview:
		boxes = append(boxes, rowHitBoxes(content, hitApplyPreviewAction, 0, m.currentApplyPreviewMenuItems())...)
	case screenApplyConfirm:
		boxes = append(boxes, rowHitBoxes(content, hitApplyConfirmAction, 0, applyConfirmMenuItems)...)
	case screenApplyResult:
		boxes = append(boxes, rowHitBoxes(content, hitApplyResultAction, 0, applyResultMenuItems)...)
	case screenExecutionList:
		boxes = append(boxes, rowHitBoxes(content, hitExecutionRow, m.executionOffset, executionSummaryRows(m.executionSummaries))...)
	case screenExecutionDetail:
		boxes = append(boxes, rowHitBoxes(content, hitExecutionAction, 0, executionActionLabels(m.executionActions()))...)
	case screenExecutionAbandonConfirm, screenExecutionDeleteConfirm:
		boxes = append(boxes, rowHitBoxes(content, hitExecutionConfirm, 0, executionConfirmMenuItems)...)
	case screenManualCleanupChoice:
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, m.manualChoiceItems())...)
	case screenManualCleanupAction:
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, m.manualActionItems())...)
	case screenManualCleanupResult:
		boxes = append(boxes, rowHitBoxes(content, hitPlatformAction, 0, manualCleanupResultItems)...)
	case screenFilters:
		if m.filterEditing == filterEditNone {
			boxes = append(boxes, rowHitBoxes(content, hitFilterRow, 0, m.filterRows())...)
		}
	case screenWarnings:
		offset, rows := m.warningAnchorsForViewport()
		boxes = append(boxes, rowHitBoxes(content, hitWarningRow, offset, rows)...)
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

func planActionAnchors(actions []domain.CleanupAction) []string {
	rows := make([]string, 0, len(actions))
	for _, action := range actions {
		rows = append(rows, actionRowAnchor(action))
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

func executionSummaryRows(entries []apply.ExecutionSummary) []string {
	rows := make([]string, len(entries))
	for index, entry := range entries {
		rows[index] = executionSummaryRow(entry)
	}
	return rows
}

func executionActionLabels(actions []executionAction) []string {
	labels := make([]string, len(actions))
	for index, action := range actions {
		labels[index] = action.Label
	}
	return labels
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
	value = strings.ReplaceAll(value, "·", "")
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
