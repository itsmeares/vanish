package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/itsmeares/vanish/internal/apply"
	"github.com/itsmeares/vanish/internal/domain"
	"github.com/itsmeares/vanish/internal/instagram"
	"github.com/itsmeares/vanish/internal/manualcleanup"
	"github.com/itsmeares/vanish/internal/platform"
	"github.com/itsmeares/vanish/internal/reddit"
	"github.com/itsmeares/vanish/internal/workspace"
)

type unsafeMessageTestProvider struct {
	message string
}

func (provider unsafeMessageTestProvider) Platform() domain.PlatformName {
	return domain.PlatformInstagram
}

func (provider unsafeMessageTestProvider) Mode() apply.ExecutionMode {
	return apply.ExecutionModeSimulation
}

func (provider unsafeMessageTestProvider) ExecutorID() apply.ExecutorID {
	return "unsafe-message-test"
}

func (provider unsafeMessageTestProvider) Supports(domain.ActionType) bool {
	return true
}

func (provider unsafeMessageTestProvider) Prerequisites(domain.CleanupPlan, apply.RuntimeState) []apply.Prerequisite {
	return nil
}

func (provider unsafeMessageTestProvider) Executor() apply.Executor {
	return unsafeMessageTestExecutor{message: provider.message}
}

type unsafeMessageTestExecutor struct {
	message string
}

func (executor unsafeMessageTestExecutor) Execute(context.Context, domain.CleanupAction) (apply.ProviderResult, error) {
	return apply.ProviderResult{
		Outcome:      apply.OutcomePermanentFailure,
		Message:      apply.ProviderMessage(executor.message),
		ProviderCode: "safe_code",
	}, nil
}

func TestInitialViewContainsSelectableHomeMenu(t *testing.T) {
	m := NewModel()
	view := m.View().Content
	plain := stripANSI(view)

	if !strings.Contains(plain, "Vanish") || !strings.Contains(plain, "Home") {
		t.Fatalf("expected initial view to contain app name and section")
	}
	if m.View().MouseMode != tea.MouseModeAllMotion {
		t.Fatalf("expected mouse all motion mode")
	}
	if !m.View().AltScreen {
		t.Fatalf("expected alternate screen mode")
	}
	for _, want := range []string{
		"Vanish",
		"/ Home",
		"Platforms",
		"Choose a platform",
		"Instagram Export",
		"Reddit",
		"Import a local Instagram export ZIP",
		"Enter to continue.",
		footerHome,
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected initial view to show %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{
		"[LOCAL]",
		"[DRY-RUN]",
		"[NO NETWORK]",
		"Capabilities",
		"Status: prototype",
		"Local ZIP scan: prototype",
		"Command Center",
		"Getting Started",
		"Local-only review of files you choose.",
		"No login, browser automation, deletion, or network requests.",
		"i  Import Instagram export ZIP",
		"d  Demo import with fake local data",
		"X/Twitter",
		"Load cleanup plan",
	} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("expected %q to be absent, got:\n%s", unwanted, view)
		}
	}
	if strings.Contains(plain, "Local State") {
		t.Fatalf("expected old Local State pane to be removed, got:\n%s", view)
	}
}

func TestHeaderOmitsGlobalSafetyBadges(t *testing.T) {
	m := NewModel()
	view := m.View().Content
	if !strings.Contains(view, "Vanish") || !strings.Contains(view, "/ Home") {
		t.Fatalf("expected section header, got:\n%s", view)
	}
	for _, badge := range []string{"[LOCAL]", "[DRY-RUN]", "[NO NETWORK]"} {
		if strings.Contains(view, badge) {
			t.Fatalf("expected header not to contain %q, got:\n%s", badge, view)
		}
	}
}

func TestTabAndSelectedRowStylesUseBackgrounds(t *testing.T) {
	m := NewModel()
	if m.styles.activeTab.GetBackground() == nil {
		t.Fatalf("expected active tab style to have a background")
	}
	if colorKey(m.styles.activeTab.GetBackground()) == colorKey(m.styles.tab.GetBackground()) {
		t.Fatalf("expected active and inactive tabs to have distinguishable backgrounds")
	}
	if m.styles.selected.GetBackground() == nil {
		t.Fatalf("expected selected row style to have a background")
	}
	if m.styles.hoveredRow.GetBackground() == nil {
		t.Fatalf("expected hovered row style to have a background")
	}
	if colorKey(m.styles.hoveredRow.GetBackground()) == colorKey(m.styles.selected.GetBackground()) {
		t.Fatalf("expected hover and selected row styles to differ")
	}
	if m.styles.disabledRow.GetForeground() == nil {
		t.Fatalf("expected disabled row style to be muted")
	}
	if m.styles.hoveredTab.GetBackground() == nil {
		t.Fatalf("expected hovered tab style to have a background")
	}

	row := m.selectableLine("Import Instagram export ZIP", true, 48)
	if strings.Contains(row, ">") {
		t.Fatalf("expected selected row not to rely on > marker, got %q", row)
	}
	if !strings.Contains(row, "Import Instagram export ZIP") {
		t.Fatalf("expected selected row to keep label, got %q", row)
	}
	if got, want := lipgloss.Width(stripANSI(row)), paneTextWidth(48); got != want {
		t.Fatalf("expected selected row to fill width %d, got %d in %q", want, got, stripANSI(row))
	}
}

func TestHomeDetailChangesWithPlatformCursor(t *testing.T) {
	m := NewModel()
	instagramView := m.View().Content
	if !strings.Contains(instagramView, "Instagram Export") || !strings.Contains(instagramView, "Import a local Instagram export ZIP") {
		t.Fatalf("expected Instagram platform detail, got:\n%s", instagramView)
	}
	if strings.Contains(instagramView, "Command Center") || strings.Contains(instagramView, "Getting Started") {
		t.Fatalf("expected static home copy to be removed, got:\n%s", instagramView)
	}

	m.homeCursor = 1
	redditView := m.View().Content
	for _, want := range []string{
		"Reddit",
		"Sign in to scan your Reddit activity.",
	} {
		if !strings.Contains(redditView, want) {
			t.Fatalf("expected Reddit detail to contain %q, got:\n%s", want, redditView)
		}
	}
	if strings.Contains(redditView, "Import a local Instagram export ZIP") {
		t.Fatalf("expected home detail to change with cursor, got:\n%s", redditView)
	}
	for _, unwanted := range []string{"prototype", "planned", "Capabilities", "OAuth:", "Network/API access"} {
		if strings.Contains(redditView, unwanted) {
			t.Fatalf("expected Reddit home detail not to show %q, got:\n%s", unwanted, redditView)
		}
	}
}

func TestHomeRendersBorderedFullHeightPanesAtTallSize(t *testing.T) {
	next := updateModel(t, NewModel(), tea.WindowSizeMsg{Width: 160, Height: 60})

	view := next.View().Content
	for _, want := range []string{"Platforms", "Instagram Export", "Reddit", "Enter to continue."} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected home to contain %q, got:\n%s", want, view)
		}
	}
	if lines := strings.Count(view, "\n") + 1; lines < 54 {
		t.Fatalf("expected home shell to use terminal height, got %d lines:\n%s", lines, view)
	}
	plain := stripANSI(view)
	if strings.Count(plain, "└") < 2 {
		t.Fatalf("expected bordered home panes, got:\n%s", view)
	}
}

func TestFooterStylesKeysAndKeepsReadableText(t *testing.T) {
	m := NewModel()
	footer := m.footer(footerHome)
	if !strings.Contains(stripANSI(footer), footerHome) {
		t.Fatalf("expected footer text to survive styling, got %q", footer)
	}
	if !strings.Contains(footer, "\x1b[") {
		t.Fatalf("expected footer key styling to add ANSI, got %q", footer)
	}
	if m.styles.footerKey.GetForeground() == nil {
		t.Fatalf("expected footer key style to define a foreground")
	}
}

func TestHomeEnterOpensPlatformDetail(t *testing.T) {
	m := NewModel()
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenPlatformDetail || next.selectedPlatformID != platform.PlatformInstagramExport {
		t.Fatalf("expected enter to open Instagram platform detail, screen=%v id=%q", next.current, next.selectedPlatformID)
	}
}

func TestPlatformDetailShowsTypedCapabilitiesAfterActions(t *testing.T) {
	m := NewModel()
	m.openPlatformDetail(0)
	plain := stripANSI(m.View().Content)
	actions := strings.Index(plain, "Actions")
	status := strings.Index(plain, "Status")
	capabilities := strings.Index(plain, "Capabilities")
	if actions < 0 || status < 0 || capabilities < 0 {
		t.Fatalf("expected actions, status, and capabilities, got:\n%s", plain)
	}
	if actions > status || status > capabilities {
		t.Fatalf("expected actions before status and capabilities, got:\n%s", plain)
	}
	for _, want := range []string{"Local import: supported", "Automatic cleanup: unsupported", "Account authentication: unsupported"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected platform detail to show %q, got:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Notes / Guide") {
		t.Fatalf("expected concise platform detail, got:\n%s", plain)
	}
}

func TestPlatformDetailBackReturnsHome(t *testing.T) {
	m := NewModel()
	m.openPlatformDetail(0)
	next := updateModel(t, m, keyPress("esc"))
	if next.current != screenHome {
		t.Fatalf("expected esc to return home, got %v", next.current)
	}
}

func TestHomeMenuNavigationUsesArrowAndJK(t *testing.T) {
	m := NewModel()

	next := updateModel(t, m, keyPress("down"))
	if next.homeCursor != 1 {
		t.Fatalf("expected down to select Reddit, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.homeCursor != 1 {
		t.Fatalf("expected cursor to stay at bottom, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("up"))
	if next.homeCursor != 0 {
		t.Fatalf("expected up to select Instagram Export, got %d", next.homeCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.homeCursor != 0 {
		t.Fatalf("expected cursor to stay at top, got %d", next.homeCursor)
	}
}

func TestInstagramPlatformActionsRouteToExistingScreens(t *testing.T) {
	cases := []struct {
		name       string
		actionID   string
		wantScreen screen
		wantCmd    bool
	}{
		{name: "choose export ZIP", actionID: platform.ActionChooseExportZIP, wantScreen: screenImportPath},
		{name: "request export", actionID: platform.ActionRequestInstagramExport, wantScreen: screenInstagramExportGuide, wantCmd: true},
		{name: "demo import", actionID: platform.ActionDemoImport, wantScreen: screenImporting, wantCmd: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel()
			m.openPlatformDetail(0)
			m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), tc.actionID)

			updated, cmd := m.Update(keyPress("enter"))
			if tc.wantCmd && cmd == nil {
				t.Fatalf("expected action %q to return command", tc.actionID)
			}
			if !tc.wantCmd && cmd != nil {
				t.Fatalf("expected action %q not to return command", tc.actionID)
			}
			next := requireModel(t, updated)
			if next.current != tc.wantScreen {
				t.Fatalf("expected action %q to route to %v, got %v", tc.actionID, tc.wantScreen, next.current)
			}
			if tc.actionID == platform.ActionDemoImport && next.importSource != "demo instagram export" {
				t.Fatalf("expected demo import source, got %q", next.importSource)
			}
		})
	}
}

func TestInstagramGuideBackReturnsToPlatformDetail(t *testing.T) {
	m := NewModel()
	m.openPlatformDetail(0)
	m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), platform.ActionRequestInstagramExport)
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenInstagramExportGuide {
		t.Fatalf("expected guide screen, got %v", next.current)
	}
	for _, want := range []string{
		"Open Instagram export page",
		"I have the ZIP",
		"1. Open Instagram settings",
		"2. Go to Accounts Centre",
		"3. Open Your information and permissions",
		"4. Select Export your information",
		"5. Create an export",
		"6. Select the Instagram profile",
		"7. Choose Export to device",
		"8. Customise the included information",
		"9. Set Date range to All time",
		"10. Set Format to JSON",
		"11. Start export",
	} {
		if !strings.Contains(next.View().Content, want) {
			t.Fatalf("expected Instagram guide to contain %q, got:\n%s", want, next.View().Content)
		}
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenPlatformDetail || next.selectedPlatformID != platform.PlatformInstagramExport {
		t.Fatalf("expected back to Instagram platform detail, screen=%v id=%q", next.current, next.selectedPlatformID)
	}
}

func TestInstagramExportBrowserOpensOnlyAfterExplicitSelection(t *testing.T) {
	previous := openExternalURL
	called := 0
	opened := ""
	openExternalURL = func(rawURL string) error {
		called++
		opened = rawURL
		return nil
	}
	t.Cleanup(func() { openExternalURL = previous })

	m := NewModel()
	m.openPlatformDetail(0)
	if called != 0 {
		t.Fatalf("browser opened before selection")
	}
	m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), platform.ActionRequestInstagramExport)
	updated, cmd := m.Update(keyPress("enter"))
	if cmd == nil || called != 0 {
		t.Fatalf("request selection must return deferred browser command")
	}
	next := requireModel(t, updated)
	msg := cmd()
	if called != 1 || opened != instagramExportPageURL {
		t.Fatalf("browser calls=%d url=%q", called, opened)
	}
	next = updateModel(t, next, msg)
	next.instagramGuideCursor = instagramGuideOpenPage
	_, cmd = next.Update(keyPress("enter"))
	if cmd == nil || called != 1 {
		t.Fatalf("guide render/navigation opened browser without command execution")
	}
	_ = cmd()
	if called != 2 {
		t.Fatalf("reopen calls=%d, want 2", called)
	}
}

func TestInstagramGuideHaveZIPOpensPickerAndReturnsToGuide(t *testing.T) {
	m := NewModel()
	m.current = screenInstagramExportGuide
	m.instagramGuideCursor = instagramGuideHaveZIP
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenImportPath || next.importPlatform != domain.PlatformInstagram {
		t.Fatalf("expected Instagram ZIP picker, screen=%v platform=%q", next.current, next.importPlatform)
	}
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenInstagramExportGuide {
		t.Fatalf("expected picker cancel to return to guide, got %v", next.current)
	}
}

func TestImportResultOnlyListsAvailableCategories(t *testing.T) {
	m := NewModel()
	m.current = screenImportResult
	m.importSource = "synthetic partial export"
	m.importResult = activityResult{
		Summary: activitySummary{Total: 2, Comments: 2},
	}
	view := m.View().Content
	for _, want := range []string{"Total", "Comments"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected partial import summary to contain %q, got:\n%s", want, view)
		}
	}
	for _, absent := range []string{"Likes", "Following", "Followers"} {
		if strings.Contains(view, absent) {
			t.Fatalf("missing category %q should not be implied, got:\n%s", absent, view)
		}
	}
}

func TestRedditNotConnectedShowsSimpleSignInState(t *testing.T) {
	t.Setenv(reddit.ClientIDEnv, "")

	m := NewModel()
	m.openPlatformDetail(1)
	if m.current != screenRedditConnect || m.selectedPlatformID != platform.PlatformReddit {
		t.Fatalf("expected Reddit connect screen, screen=%v id=%q", m.current, m.selectedPlatformID)
	}
	view := m.View().Content
	for _, want := range []string{
		"Reddit",
		"Sign in to scan your Reddit activity.",
		"Create a Reddit installed app.",
		"Set VANISH_REDDIT_CLIENT_ID to its client ID.",
		"Redirect URI: http://127.0.0.1:53682/reddit/oauth/callback",
		"Sign in with Reddit",
		"Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected Reddit connect screen to contain %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Enter returned code", "Allow local token file fallback", "Forget local metadata", "Disconnect and revoke access", "Manual OAuth only", "Status: not connected"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected Reddit connect screen not to contain %q, got:\n%s", unwanted, view)
		}
	}
}

func TestRedditSignInWithoutClientIDShowsShortError(t *testing.T) {
	t.Setenv(reddit.ClientIDEnv, "")

	m := NewModel()
	m.openPlatformDetail(1)

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenRedditConnect {
		t.Fatalf("expected missing client ID to stay on connect screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Reddit sign-in is not configured. Set VANISH_REDDIT_CLIENT_ID and try again.") {
		t.Fatalf("expected short configuration error, got:\n%s", next.View().Content)
	}
	for _, want := range []string{
		"Create a Reddit installed app.",
		"Set VANISH_REDDIT_CLIENT_ID to its client ID.",
		"Redirect URI: http://127.0.0.1:53682/reddit/oauth/callback",
	} {
		if !strings.Contains(next.View().Content, want) {
			t.Fatalf("expected Reddit setup hint %q, got:\n%s", want, next.View().Content)
		}
	}
}

func TestRedditSignInOpensBrowserWaitScreen(t *testing.T) {
	t.Setenv(reddit.ClientIDEnv, "test-client")

	m := NewModel()
	m.openPlatformDetail(1)

	updated, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected sign-in command")
	}
	next := requireModel(t, updated)
	if next.current != screenRedditSigningIn {
		t.Fatalf("expected Reddit sign-in screen, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Waiting for browser sign-in",
		"https://www.reddit.com/api/v1/authorize",
		"Cancel",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected Reddit sign-in screen to contain %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Paste the returned code", "Requested scopes", "token file fallback", "browser automation"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected Reddit sign-in screen not to contain %q, got:\n%s", unwanted, view)
		}
	}
}

func TestRedditSignInCancelReturnsToSimpleState(t *testing.T) {
	cancelled := false
	m := NewModel()
	m.current = screenRedditSigningIn
	m.selectedPlatformID = platform.PlatformReddit
	m.redditAuthState = "state-123"
	m.redditAuthURL = "https://www.reddit.com/api/v1/authorize?client_id=test"
	m.redditSignInCancel = func() { cancelled = true }

	next := updateModel(t, m, keyPress("esc"))
	if !cancelled {
		t.Fatalf("expected cancel func to be called")
	}
	if next.current != screenRedditConnect || next.redditAuthState != "" || next.redditAuthURL != "" {
		t.Fatalf("expected cancel to return to Reddit connect state, got screen=%v state=%q url=%q", next.current, next.redditAuthState, next.redditAuthURL)
	}
	if !strings.Contains(next.View().Content, "Sign in to scan your Reddit activity.") {
		t.Fatalf("expected simple Reddit state after cancel, got:\n%s", next.View().Content)
	}
}

func TestRedditCodeFromInputParsesRedirectAndChecksState(t *testing.T) {
	code, err := redditCodeFromInput("plain-code", "state-123")
	if err != nil || code != "plain-code" {
		t.Fatalf("plain code = %q err=%v", code, err)
	}

	code, err = redditCodeFromInput(reddit.DefaultRedirectURI+"?state=state-123&code=url-code", "state-123")
	if err != nil || code != "url-code" {
		t.Fatalf("redirect code = %q err=%v", code, err)
	}

	if _, err := redditCodeFromInput(reddit.DefaultRedirectURI+"?state=wrong&code=url-code", "state-123"); err == nil {
		t.Fatalf("expected state mismatch error")
	}
}

func TestRedditConnectedShowsScanDisconnectBack(t *testing.T) {
	m := NewModel()
	m.current = screenRedditConnect
	m.selectedPlatformID = platform.PlatformReddit
	m.localConfig.Reddit = &workspace.RedditConfig{Username: "test_user"}
	view := m.View().Content
	for _, want := range []string{
		"Signed in as u/test_user",
		"Scan activity",
		"Disconnect",
		"Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected Reddit connected state to contain %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Sign in with Reddit", "Allow local token file fallback", "Forget local metadata", "Disconnect and revoke access"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("expected Reddit connected state not to contain %q, got:\n%s", unwanted, view)
		}
	}
	if strings.Contains(view, "Capabilities") || strings.Contains(view, "Official API scan: prototype") {
		t.Fatalf("expected connection screen to omit capability list, got:\n%s", view)
	}
}

func TestRedditConnectionActionsUseTypedCapabilityAvailability(t *testing.T) {
	tests := []struct {
		name         string
		connected    bool
		capabilityID platform.CapabilityID
		support      platform.CapabilitySupport
		actionID     string
		disable      bool
	}{
		{name: "sign-in planned", capabilityID: platform.CapabilityAccountAuthentication, support: platform.SupportPlanned, actionID: platform.ActionConnectAccount},
		{name: "sign-in later", capabilityID: platform.CapabilityAccountAuthentication, support: platform.SupportLater, actionID: platform.ActionConnectAccount},
		{name: "sign-in unsupported", capabilityID: platform.CapabilityAccountAuthentication, support: platform.SupportUnsupported, actionID: platform.ActionConnectAccount},
		{name: "sign-in explicitly disabled", capabilityID: platform.CapabilityAccountAuthentication, support: platform.SupportPrototype, actionID: platform.ActionConnectAccount, disable: true},
		{name: "scan planned", connected: true, capabilityID: platform.CapabilityOfficialAPIScan, support: platform.SupportPlanned, actionID: platform.ActionScanActivity},
		{name: "scan later", connected: true, capabilityID: platform.CapabilityOfficialAPIScan, support: platform.SupportLater, actionID: platform.ActionScanActivity},
		{name: "scan unsupported", connected: true, capabilityID: platform.CapabilityOfficialAPIScan, support: platform.SupportUnsupported, actionID: platform.ActionScanActivity},
		{name: "scan explicitly disabled", connected: true, capabilityID: platform.CapabilityOfficialAPIScan, support: platform.SupportPrototype, actionID: platform.ActionScanActivity, disable: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			installRedditPlatformForTest(t, func(current *platform.Platform) {
				for i := range current.Capabilities {
					if current.Capabilities[i].ID == tc.capabilityID {
						current.Capabilities[i].Support = tc.support
					}
				}
				for i := range current.Actions {
					if current.Actions[i].ID == tc.actionID {
						current.Actions[i].Disabled = tc.disable
					}
				}
			})

			m := NewModel()
			m.current = screenRedditConnect
			m.selectedPlatformID = platform.PlatformReddit
			if tc.connected {
				m.localConfig.Reddit = &workspace.RedditConfig{Username: "test_user"}
			}
			actions := m.redditConnectActions()
			if len(actions) == 0 || !actions[0].Disabled {
				t.Fatalf("expected typed capability to disable action: %#v", actions)
			}

			updated, cmd := m.Update(keyPress("enter"))
			keyboard := requireModel(t, updated)
			if cmd != nil || keyboard.current != screenRedditConnect {
				t.Fatalf("keyboard activated disabled action: screen=%v cmd=%v", keyboard.current, cmd != nil)
			}

			box := requireHitBox(t, hitBoxesForTest(t, m), hitPlatformAction, 0, "")
			updated, cmd = m.Update(mouseClick(box.X, box.Y))
			mouse := requireModel(t, updated)
			if cmd != nil || mouse.current != screenRedditConnect {
				t.Fatalf("mouse activated disabled action: screen=%v cmd=%v", mouse.current, cmd != nil)
			}
		})
	}
}

func TestApplyEventAuditFieldsPreserveSkippedRouteIdentity(t *testing.T) {
	fields := applyEventAuditFields(apply.ExecutionEvent{
		Type: apply.EventActionSkipped, PlanID: "plan-1", Platform: domain.PlatformReddit,
		ActionID: "action-1", ActionType: domain.ActionRedditDeleteComment, Status: domain.ActionStatusSkipped,
		Mode: apply.ExecutionModeSimulation, Executor: reddit.SimulationExecutorID,
	})
	if fields["execution_mode"] != "simulation" || fields["executor"] != "reddit-simulation" {
		t.Fatalf("skipped audit fields lost route identity: %#v", fields)
	}
}

func TestApplyEventAuditFieldsIncludeTypedOutcomeMetadata(t *testing.T) {
	fields := applyEventAuditFields(apply.ExecutionEvent{
		Type:         apply.EventActionResult,
		PlanID:       "plan-1",
		Platform:     domain.PlatformReddit,
		ActionID:     "action-1",
		ActionType:   domain.ActionRedditDeleteComment,
		Status:       domain.ActionStatusFailed,
		Mode:         apply.ExecutionModeSimulation,
		Executor:     reddit.SimulationExecutorID,
		Outcome:      apply.OutcomeRetryableFailure,
		Attempt:      2,
		Retryable:    true,
		RetryAfter:   1500 * time.Millisecond,
		ProviderCode: apply.ProviderCodeTemporaryFailure,
		Message:      "safe user-facing message",
	})
	for key, want := range map[string]any{
		"execution_mode": "simulation",
		"executor":       "reddit-simulation",
		"outcome":        "retryable_failure",
		"attempt":        2,
		"retryable":      true,
		"retry_after_ms": int64(1500),
		"provider_code":  "temporary_failure",
	} {
		if fields[key] != want {
			t.Fatalf("audit field %q = %#v, want %#v; fields=%#v", key, fields[key], want, fields)
		}
	}
	for _, forbidden := range []string{"message", "target_url", "raw_response", "error", "authorization", "token"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatalf("audit fields included forbidden %q: %#v", forbidden, fields)
		}
	}
	if _, ok := fields["pending_count"]; ok {
		t.Fatalf("action event included misleading zero counts: %#v", fields)
	}
}

func TestApplyEventAuditFieldsOmitEmptyOptionalValues(t *testing.T) {
	fields := applyEventAuditFields(apply.ExecutionEvent{Type: apply.EventExecutionFinished, PlanID: "plan-1", Platform: domain.PlatformInstagram})
	for _, optional := range []string{"execution_mode", "executor", "outcome", "attempt", "retryable", "retry_after_ms", "provider_code", "halt_reason"} {
		if _, ok := fields[optional]; ok {
			t.Fatalf("empty optional field %q was emitted: %#v", optional, fields)
		}
	}
}

func TestApplyEventAuditFieldsRejectUnknownProviderCode(t *testing.T) {
	fields := applyEventAuditFields(apply.ExecutionEvent{
		Type:         apply.EventActionResult,
		PlanID:       "plan-1",
		Platform:     domain.PlatformInstagram,
		Outcome:      apply.OutcomePermanentFailure,
		Attempt:      1,
		ProviderCode: apply.ProviderCode("sk_live_Q7w9J2m4N8p6R3x5"),
	})
	if _, ok := fields["provider_code"]; ok {
		t.Fatalf("unknown provider code entered audit: %#v", fields)
	}
}

func TestApplyResultViewShowsConciseTypedOutcomeDetails(t *testing.T) {
	t.Run("authentication halt", func(t *testing.T) {
		m := NewModel()
		m.width = 120
		m.height = 40
		m.current = screenApplyResult
		m.applyExecution = apply.Execution{
			State:      apply.ExecutionStateHalted,
			HaltReason: apply.OutcomeAuthenticationRequired,
			Preview:    apply.Preview{Executor: reddit.SimulationExecutorID},
			Counts:     apply.ResultCounts{Failed: 1, Pending: 1},
			Results: []apply.ActionResult{{
				ActionID:     "action-1",
				Platform:     domain.PlatformReddit,
				Type:         domain.ActionRedditDeleteComment,
				Status:       domain.ActionStatusFailed,
				Outcome:      apply.OutcomeAuthenticationRequired,
				Attempt:      1,
				ProviderCode: "auth_expired",
				Message:      "Reconnect the account before trying again.",
			}},
		}
		view := stripANSI(m.View().Content)
		for _, want := range []string{"State: halted", "Reason: authentication required", "Reconnect the account before trying again.", "Pending: 1"} {
			if !strings.Contains(view, want) {
				t.Fatalf("halted apply result missing %q:\n%s", want, view)
			}
		}
		if strings.Contains(view, "auth_expired") {
			t.Fatalf("provider code became primary UI copy:\n%s", view)
		}
		if strings.Count(view, "Reconnect the account before trying again.") != 1 {
			t.Fatalf("authentication guidance was duplicated:\n%s", view)
		}
	})

	t.Run("retry succeeds on second attempt", func(t *testing.T) {
		m := NewModel()
		m.width = 120
		m.height = 40
		m.current = screenApplyResult
		m.applyExecution = apply.Execution{
			State:  apply.ExecutionStateDone,
			Counts: apply.ResultCounts{Done: 1},
			Results: []apply.ActionResult{
				{ActionID: "action-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnlike, Status: domain.ActionStatusFailed, Outcome: apply.OutcomeRetryableFailure, Attempt: 1},
				{ActionID: "action-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnlike, Status: domain.ActionStatusDone, Outcome: apply.OutcomeSucceeded, Attempt: 2},
			},
		}
		view := stripANSI(m.View().Content)
		if !strings.Contains(view, "Attempt: 2") {
			t.Fatalf("retry attempt was not shown concisely:\n%s", view)
		}
		if len(finalActionResults(m.applyExecution.Results)) != 1 {
			t.Fatalf("retry history was not collapsed: %#v", finalActionResults(m.applyExecution.Results))
		}
	})

	t.Run("retry after halt", func(t *testing.T) {
		m := NewModel()
		m.width = 120
		m.height = 40
		m.current = screenApplyResult
		m.applyExecution = apply.Execution{
			State:  apply.ExecutionStateHalted,
			Counts: apply.ResultCounts{Failed: 1},
			Results: []apply.ActionResult{{
				ActionID: "action-1", Type: domain.ActionUnlike, Status: domain.ActionStatusFailed,
				Outcome: apply.OutcomeRetryableFailure, Attempt: 1, RetryAfter: 30 * time.Second,
			}},
		}
		view := stripANSI(m.View().Content)
		if !strings.Contains(view, "Retry after: 30s") || !strings.Contains(view, "retryable failure") {
			t.Fatalf("retry-after halt details missing:\n%s", view)
		}
	})
}

func TestApplyResultViewNeverShowsUnsafeProviderMessage(t *testing.T) {
	const unsafeMessage = "Authorization: Bearer top-secret-token"
	registry, err := apply.NewProviderRegistry(unsafeMessageTestProvider{message: unsafeMessage})
	if err != nil {
		t.Fatal(err)
	}
	plan := fakeCleanupPlan()
	execution := (apply.Runner{Providers: registry}).Run(context.Background(), plan, apply.ExecutionModeSimulation)
	if execution.State != apply.ExecutionStateFailed || len(execution.Results) == 0 {
		t.Fatalf("unexpected execution: %#v", execution)
	}
	for _, result := range execution.Results {
		if strings.Contains(result.Message, unsafeMessage) || result.ProviderCode != "" || result.Message != "Action failed." {
			t.Fatalf("unsafe provider text entered result: %#v", result)
		}
	}
	for _, event := range execution.Events {
		if strings.Contains(event.Message, unsafeMessage) || event.ProviderCode != "" {
			t.Fatalf("unsafe provider text entered event: %#v", event)
		}
	}

	m := NewModel()
	m.width = 120
	m.height = 40
	m.current = screenApplyResult
	m.applyExecution = execution
	view := stripANSI(m.View().Content)
	for _, forbidden := range []string{unsafeMessage, "top-secret-token", "Bearer"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("unsafe provider text entered TUI: %q\n%s", forbidden, view)
		}
	}
}

func TestFinalActionResultsAssociatesUniqueActionRetryHistory(t *testing.T) {
	results := []apply.ActionResult{
		{ActionID: "action-1", Outcome: apply.OutcomeRetryableFailure, Attempt: 1},
		{ActionID: "action-2", Outcome: apply.OutcomeSucceeded, Attempt: 1},
		{ActionID: "action-1", Outcome: apply.OutcomeSucceeded, Attempt: 2},
	}
	final := finalActionResults(results)
	if len(final) != 2 || final[0].ActionID != "action-1" || final[0].Attempt != 2 || final[1].ActionID != "action-2" || final[1].Attempt != 1 {
		t.Fatalf("unique action results associated incorrectly: %#v", final)
	}
}

func TestTabClicksRouteToSafeScreens(t *testing.T) {
	imported := importedModel(t, fakeImportResult())

	next := clickTab(t, imported, "Home")
	if next.current != screenHome {
		t.Fatalf("expected Home tab to route home, got %v", next.current)
	}

	next = clickTab(t, next, "Local")
	if next.current != screenLocalDataOverview {
		t.Fatalf("expected Local tab to route to local data overview, got %v", next.current)
	}

	next = clickTab(t, next, "Help")
	if next.current != screenKeybindings {
		t.Fatalf("expected Help tab to route to help, got %v", next.current)
	}

	next = clickTab(t, imported, "Import")
	if next.current != screenImportResult {
		t.Fatalf("expected active Import tab to no-op, got %v", next.current)
	}
	if len(next.importResult.Items) == 0 {
		t.Fatalf("expected active tab no-op to preserve imported data")
	}

	next = clickTab(t, NewModel(), "Import")
	if next.current != screenImportPath {
		t.Fatalf("expected inactive Import tab to route to import picker, got %v", next.current)
	}
	next = clickTab(t, next, "Home")
	if next.current != screenHome {
		t.Fatalf("expected Home tab from Import to route home, got %v", next.current)
	}

	next = clickTab(t, imported, "Review")
	if next.current != screenItemsBrowser {
		t.Fatalf("expected Review tab with import data to route to parsed items, got %v", next.current)
	}

	next = clickTab(t, NewModel(), "Review")
	if next.current != screenReviewEmpty || !strings.Contains(next.View().Content, "No parsed items yet") {
		t.Fatalf("expected Review tab without import data to show empty state, screen=%v view:\n%s", next.current, next.View().Content)
	}

	next = clickTab(t, NewModel(), "Plans")
	if next.current != screenPlanLoadPath {
		t.Fatalf("expected Plans tab without plans to route to load plan, got %v", next.current)
	}
}

func TestTabHitBoxesMapVisibleTabs(t *testing.T) {
	m := NewModel()
	boxes := hitBoxesForTest(t, m)

	for _, label := range tabLabels {
		box := requireHitBox(t, boxes, hitTab, -1, label)
		target := hitTargetAt(boxes, box.X, box.Y)
		if target.Kind != hitTab || target.Label != label {
			t.Fatalf("expected %q tab at (%d,%d), got %#v", label, box.X, box.Y, target)
		}
	}
}

func TestPlansTabPrefersLoadedPlanThenGeneratedPreview(t *testing.T) {
	loaded := NewModel()
	loaded.loadedPlan = fakeCleanupPlan()
	loaded.loadedPlanSummary = domain.SummarizeCleanupPlan(loaded.loadedPlan)
	next := clickTab(t, loaded, "Plans")
	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected Plans tab to prefer loaded plan summary, got %v", next.current)
	}

	preview := planPreviewModel(t)
	preview.current = screenHome
	next = clickTab(t, preview, "Plans")
	if next.current != screenPlanPreview {
		t.Fatalf("expected Plans tab to route to generated preview, got %v", next.current)
	}
}

func TestMouseClickHomeMenuRowActivatesOnSingleClick(t *testing.T) {
	m := NewModel()
	box := requireHitBox(t, hitBoxesForTest(t, m), hitHomeAction, 1, "")

	next := updateModel(t, m, mouseClick(box.X, box.Y))
	if next.current != screenRedditConnect || next.selectedPlatformID != platform.PlatformReddit {
		t.Fatalf("expected single click on Reddit to open sign-in state, screen=%v id=%q", next.current, next.selectedPlatformID)
	}
}

func TestMouseClickOutsideMenuRowBoundsDoesNotActivate(t *testing.T) {
	m := NewModel()
	box := requireHitBox(t, hitBoxesForTest(t, m), hitHomeAction, 1, "")

	next := updateModel(t, m, mouseClick(box.X+box.Width+1, box.Y))
	if next.current != screenHome {
		t.Fatalf("expected out-of-row click to no-op, got %v", next.current)
	}
}

func TestHomeHitBoxesMapVisibleRowsWithFrameOffsets(t *testing.T) {
	m := NewModel()
	boxes := hitBoxesForTest(t, m)
	homeTab := requireHitBox(t, boxes, hitTab, -1, "Home")
	spec := layoutSpec(m.width, m.height)
	homeMenuWidth, _ := twoPaneWidths(spec, "Platforms")

	for index, label := range platformLabels(m.platforms()) {
		box := requireHitBox(t, boxes, hitHomeAction, index, "")
		if box.Y <= homeTab.Y {
			t.Fatalf("expected home row %q to include header/tab offset, tab y=%d row y=%d", label, homeTab.Y, box.Y)
		}
		line := renderedLineAt(m.View().Content, box.Y)
		segment := firstPaneSegment(line)
		if !strings.Contains(stripANSI(segment), label) {
			t.Fatalf("expected hit box for row %d to point at %q, got line %q", index, label, stripANSI(line))
		}
		if index == m.homeCursor {
			want := paneTextWidth(homeMenuWidth) + 2
			if got := lipgloss.Width(stripANSI(segment)); got != want {
				t.Fatalf("expected selected home row to fill width %d, got %d in %q", want, got, stripANSI(segment))
			}
		}
		target := hitTargetAt(boxes, box.X, box.Y)
		if target.Kind != hitHomeAction || target.Index != index {
			t.Fatalf("expected row %d at (%d,%d), got %#v", index, box.X, box.Y, target)
		}
	}
}

func TestMouseHoverHomeRowUsesRenderedHitBox(t *testing.T) {
	m := NewModel()
	box := requireHitBox(t, hitBoxesForTest(t, m), hitHomeAction, 1, "")

	next := updateModel(t, m, mouseMotion(box.X, box.Y))
	if next.hoverTarget.Kind != hitHomeAction || next.hoverTarget.Index != 1 {
		t.Fatalf("expected hover target for Reddit row, got %#v", next.hoverTarget)
	}
	if next.homeCursor != 0 {
		t.Fatalf("expected hover not to move home cursor, got %d", next.homeCursor)
	}
}

func TestMouseHitBoxesStillMatchRowsAfterResize(t *testing.T) {
	m := updateModel(t, NewModel(), tea.WindowSizeMsg{Width: 90, Height: 18})
	box := requireHitBox(t, hitBoxesForTest(t, m), hitHomeAction, 1, "")

	next := updateModel(t, m, mouseMotion(box.X, box.Y))
	if next.hoverTarget.Kind != hitHomeAction || next.hoverTarget.Index != 1 {
		t.Fatalf("expected resized hover target for Reddit row, got %#v", next.hoverTarget)
	}

	next = updateModel(t, m, mouseClick(box.X, box.Y))
	if next.current != screenRedditConnect || next.selectedPlatformID != platform.PlatformReddit {
		t.Fatalf("expected resized click on Reddit row to activate, screen=%v id=%q", next.current, next.selectedPlatformID)
	}
}

func TestEnterOnDemoStartsDemoImport(t *testing.T) {
	m := NewModel()
	m.openPlatformDetail(0)
	m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), platform.ActionDemoImport)

	updated, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected demo import to return a command")
	}

	next := requireModel(t, updated)
	if next.current != screenImporting {
		t.Fatalf("expected demo import to move to importing screen, got %v", next.current)
	}
	if next.importSource != "demo instagram export" {
		t.Fatalf("expected demo source, got %q", next.importSource)
	}
}

func TestQuitKeysOpenConfirmationBeforeQuit(t *testing.T) {
	for _, keyName := range []string{"ctrl+c", "ctrl+q"} {
		m := NewModel()

		updated, cmd := m.Update(keyPress(keyName))
		if cmd != nil {
			t.Fatalf("expected %s not to quit immediately", keyName)
		}

		next := requireModel(t, updated)
		if next.current != screenQuitConfirm {
			t.Fatalf("expected %s to open quit confirmation, got %v", keyName, next.current)
		}
		view := next.View().Content
		for _, want := range []string{"Quit Vanish?", "Quit Vanish", "Cancel"} {
			if !strings.Contains(view, want) {
				t.Fatalf("expected quit confirmation to contain %q, got:\n%s", want, view)
			}
		}

		next = updateModel(t, next, keyPress("up"))
		updated, cmd = next.Update(keyPress("enter"))
		if cmd == nil {
			t.Fatalf("expected confirmed quit to return command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected confirmed quit command to return tea.QuitMsg")
		}
	}
}

func TestQNoLongerQuits(t *testing.T) {
	m := NewModel()

	updated, cmd := m.Update(keyPress("q"))
	if cmd != nil {
		t.Fatalf("expected q not to return quit command")
	}
	next := requireModel(t, updated)
	if next.current != screenHome {
		t.Fatalf("expected q to leave screen unchanged, got %v", next.current)
	}
}

func TestQuestionMarkOpensHelpScreen(t *testing.T) {
	m := NewModel()

	next := updateModel(t, m, keyPress("?"))
	if next.current != screenKeybindings {
		t.Fatalf("expected help screen, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Vanish",
		"Help",
		"Navigation",
		"Lists",
		"Selection",
		"Forms",
		"Plans",
		"Notes",
		"Up/Down or j/k: move",
		"Esc/Backspace: back when no text input is focused",
		"Ctrl+Q or Ctrl+C: quit confirmation",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("backspace"))
	if next.current != screenHome {
		t.Fatalf("expected backspace to return from help, got %v", next.current)
	}
}

func TestQuitConfirmationEscReturnsToPreviousScreen(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected test setup to open items browser")
	}

	next = updateModel(t, next, keyPress("ctrl+c"))
	if next.current != screenQuitConfirm {
		t.Fatalf("expected quit confirmation, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected esc to return to previous screen, got %v", next.current)
	}
}

func TestQuitConfirmationUsesTerminalHeightAtTallSize(t *testing.T) {
	next := updateModel(t, NewModel(), keyPress("ctrl+q"))
	next = updateModel(t, next, tea.WindowSizeMsg{Width: 160, Height: 60})

	view := next.View().Content
	for _, want := range []string{"Quit Vanish?", "Quit Vanish", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected quit confirmation to contain %q, got:\n%s", want, view)
		}
	}
	if lines := strings.Count(view, "\n") + 1; lines < 54 {
		t.Fatalf("expected quit confirmation to use terminal height, got %d lines:\n%s", lines, view)
	}
}

func TestLocalDataScreensEmptyStatesAndBackspaceNavigation(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	next := openLocalDataOverview(t, NewModelWithWorkspace(w, nil))

	view := next.View().Content
	for _, want := range []string{
		"Local Data",
		"Manage Vanish data on this device.",
		"App directory:",
		filepath.Base(w.Dir()),
		"Recent imports: 0",
		"Recent plans: 0",
		"Audit events: 0",
		"Recent imports",
		"Recent plans",
		"Audit log",
		"Wipe local data",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected local data overview to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenRecentImports {
		t.Fatalf("expected recent imports screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "No recent imports yet.") {
		t.Fatalf("expected recent imports empty state, got:\n%s", next.View().Content)
	}
	next = updateModel(t, next, keyPress("backspace"))
	if next.current != screenLocalDataOverview {
		t.Fatalf("expected backspace to return to local data overview, got %v", next.current)
	}

	next.localDataCursor = localDataRecentPlans
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenRecentPlans {
		t.Fatalf("expected recent plans screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "No recent plans yet.") {
		t.Fatalf("expected recent plans empty state, got:\n%s", next.View().Content)
	}
	next = updateModel(t, next, keyPress("backspace"))
	if next.current != screenLocalDataOverview {
		t.Fatalf("expected backspace to return from recent plans, got %v", next.current)
	}

	next.localDataCursor = localDataAuditLog
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenAuditLog {
		t.Fatalf("expected audit log screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "No audit events yet.") {
		t.Fatalf("expected audit empty state, got:\n%s", next.View().Content)
	}
	next = updateModel(t, next, keyPress("backspace"))
	if next.current != screenLocalDataOverview {
		t.Fatalf("expected backspace to return from audit log, got %v", next.current)
	}
}

func TestRecentPlanEnterLoadsExistingPlanThroughLoadPath(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	plan := fakeCleanupPlan()
	path := writeTUIPlan(t, plan)
	summary := domain.SummarizeCleanupPlan(plan)
	if err := w.UpsertRecentPlan(workspace.RecentPlan{
		ID:            plan.ID,
		Path:          path,
		Mode:          string(plan.Mode),
		SourceName:    plan.SourceName,
		PlanCreatedAt: plan.CreatedAt,
		LastUsedAt:    plan.CreatedAt.Add(time.Hour),
		LastOperation: "exported",
		ActionCounts:  map[string]int{"unlike": summary.ActionCounts[domain.ActionUnlike]},
		StatusCounts:  map[string]int{"pending": summary.StatusCounts[domain.ActionStatusPending]},
	}); err != nil {
		t.Fatalf("seed recent plan: %v", err)
	}

	next := openLocalDataOverview(t, NewModelWithWorkspace(w, nil))
	next.localDataCursor = localDataRecentPlans
	next = updateModel(t, next, keyPress("enter"))
	view := next.View().Content
	for _, want := range []string{"Plan created at:", "Last used at:", "Last operation: exported"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected recent plans screen to contain %q, got:\n%s", want, view)
		}
	}
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected recent plan load command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected loaded plan summary, got %v", next.current)
	}
	if next.loadedPlan.ID != plan.ID {
		t.Fatalf("loaded plan id = %q, want %q", next.loadedPlan.ID, plan.ID)
	}
	if next.planPathInput.Value() != path {
		t.Fatalf("plan path input = %q, want %q", next.planPathInput.Value(), path)
	}
}

func TestRecentImportsScreenRendersPerTypeCounts(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	if err := w.UpsertRecentImport(workspace.RecentImport{
		SourceLabel:    "instagram-export.zip",
		SourcePath:     filepath.Join(t.TempDir(), "instagram-export.zip"),
		Platform:       string(domain.PlatformInstagram),
		ImportedAt:     time.Date(2026, 6, 28, 8, 0, 0, 0, time.UTC),
		Demo:           true,
		ItemCount:      10,
		LikeCount:      4,
		CommentCount:   3,
		PostCount:      1,
		FollowingCount: 1,
		FollowerCount:  1,
		SkippedCount:   6,
		WarningCount:   5,
	}); err != nil {
		t.Fatalf("seed recent import: %v", err)
	}

	next := openLocalDataOverview(t, NewModelWithWorkspace(w, nil))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenRecentImports {
		t.Fatalf("expected recent imports screen, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Total items: 10",
		"Likes: 4",
		"Comments: 3",
		"Posts: 1",
		"Following: 1",
		"Followers: 1",
		"Skipped or unknown: 6",
		"Warnings: 5",
		"Demo: true",
		"Source label: instagram-export.zip",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected recent imports screen to contain %q, got:\n%s", want, view)
		}
	}
}

func TestRecentPlanMissingFileShowsErrorOnRecentPlans(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	missingPath := filepath.Join(t.TempDir(), "missing-plan.json")
	if err := w.UpsertRecentPlan(workspace.RecentPlan{
		ID:            "missing-plan",
		Path:          missingPath,
		Mode:          string(domain.PlanModeDryRun),
		SourceName:    "instagram-export",
		PlanCreatedAt: time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC),
		LastUsedAt:    time.Date(2026, 6, 28, 9, 1, 0, 0, time.UTC),
		LastOperation: "loaded",
	}); err != nil {
		t.Fatalf("seed recent plan: %v", err)
	}

	next := openLocalDataOverview(t, NewModelWithWorkspace(w, nil))
	next.localDataCursor = localDataRecentPlans
	next = updateModel(t, next, keyPress("enter"))
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected recent plan load command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	if next.current != screenRecentPlans {
		t.Fatalf("expected to stay on recent plans after missing file, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Plan file not found") {
		t.Fatalf("expected friendly missing file error, got:\n%s", next.View().Content)
	}
}

func TestWipeLocalDataDefaultsToCancelAndPreservesExternalPlan(t *testing.T) {
	root := t.TempDir()
	w, err := workspace.Open(filepath.Join(root, "app"))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	externalPlan := filepath.Join(root, "vanish-plan.json")
	if err := os.WriteFile(externalPlan, []byte(`{"id":"outside"}`), 0o600); err != nil {
		t.Fatalf("write external plan: %v", err)
	}
	if err := w.UpsertRecentPlan(workspace.RecentPlan{
		ID:            "outside",
		Path:          externalPlan,
		Mode:          string(domain.PlanModeDryRun),
		PlanCreatedAt: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC),
		LastUsedAt:    time.Date(2026, 6, 28, 10, 1, 0, 0, time.UTC),
		LastOperation: "exported",
	}); err != nil {
		t.Fatalf("seed recent plan: %v", err)
	}
	if err := w.AppendAudit(workspace.AuditEvent{
		Type:      "plan_exported",
		Timestamp: time.Date(2026, 6, 28, 10, 1, 0, 0, time.UTC),
		Fields:    map[string]any{"path": externalPlan},
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	next := openLocalDataOverview(t, NewModelWithWorkspace(w, nil))
	next.localDataCursor = localDataWipe
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenWipeLocalDataConfirm {
		t.Fatalf("expected wipe confirm screen, got %v", next.current)
	}
	if next.wipeLocalDataCursor != wipeLocalDataCancel {
		t.Fatalf("expected wipe confirmation to default to cancel, got %d", next.wipeLocalDataCursor)
	}
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenLocalDataOverview || len(next.recentPlans) == 0 {
		t.Fatalf("expected cancel to preserve local data, screen=%v plans=%d", next.current, len(next.recentPlans))
	}

	next.localDataCursor = localDataWipe
	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, keyPress("up"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenLocalDataOverview {
		t.Fatalf("expected wipe to return to overview, got %v", next.current)
	}
	if len(next.recentPlans) != 0 || len(next.auditEvents) != 0 {
		t.Fatalf("expected in-memory local data cleared, plans=%d audit=%d", len(next.recentPlans), len(next.auditEvents))
	}
	if _, err := os.Stat(externalPlan); err != nil {
		t.Fatalf("expected external plan to survive wipe: %v", err)
	}
	if !strings.Contains(next.View().Content, "Local data wiped") {
		t.Fatalf("expected wipe status, got:\n%s", next.View().Content)
	}
}

func TestImportPickerNavigatesAndStartsZip(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "nested")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}
	zipPath := filepath.Join(root, "instagram-export.zip")
	if err := os.WriteFile(zipPath, []byte("not a real zip"), 0o600); err != nil {
		t.Fatalf("write zip placeholder: %v", err)
	}
	textPath := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(textPath, []byte("notes"), 0o600); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	m := NewModel()
	m.openImportPicker(root)
	m.openPlatformDetail(0)
	m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), platform.ActionChooseExportZIP)
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenImportPath {
		t.Fatalf("expected import picker screen, got %v", next.current)
	}
	plain := stripANSI(next.View().Content)
	for _, want := range []string{"Import ZIP", "Local file picker", "dir", "nested", "zip", "instagram-export.zip", "file", "notes.txt", footerImportPicker} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected import picker to contain %q, got:\n%s", want, next.View().Content)
		}
	}

	next.importPickerCursor = 3
	updated, cmd := next.Update(keyPress("enter"))
	if cmd != nil {
		t.Fatalf("expected disabled non-ZIP file not to start import")
	}
	next = requireModel(t, updated)
	if next.current != screenImportPath || next.importSource != "" {
		t.Fatalf("expected disabled file to leave picker unchanged, screen=%v source=%q", next.current, next.importSource)
	}

	next.importPickerCursor = 1
	next = updateModel(t, next, keyPress("enter"))
	if next.importPickerDir != subdir {
		t.Fatalf("expected enter on directory to open %q, got %q", subdir, next.importPickerDir)
	}

	next = updateModel(t, next, keyPress("left"))
	if next.importPickerDir != root {
		t.Fatalf("expected left to open parent %q, got %q", root, next.importPickerDir)
	}

	next.importPickerCursor = 1
	next = updateModel(t, next, keyPress("enter"))
	if next.importPickerDir != subdir {
		t.Fatalf("expected enter on directory to reopen %q, got %q", subdir, next.importPickerDir)
	}

	next = updateModel(t, next, keyPress("backspace"))
	if next.importPickerDir != root {
		t.Fatalf("expected backspace to open parent %q, got %q", root, next.importPickerDir)
	}

	next.importPickerCursor = 2
	updated, cmd = next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected enter on ZIP to start import command")
	}
	next = requireModel(t, updated)
	if next.current != screenImporting || next.importSource != zipPath {
		t.Fatalf("expected ZIP import to start, screen=%v source=%q", next.current, next.importSource)
	}

	next = updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenPlatformDetail {
		t.Fatalf("expected esc to return to Instagram platform, got %v", next.current)
	}
}

func TestImportPickerMouseClickAndWheel(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 8; i++ {
		path := filepath.Join(root, fmt.Sprintf("export-%02d.zip", i))
		if err := os.WriteFile(path, []byte("zip"), 0o600); err != nil {
			t.Fatalf("write zip placeholder: %v", err)
		}
	}

	m := NewModel()
	m.openImportPicker(root)
	m.openPlatformDetail(0)
	m.platformActionCursor = platformActionIndex(t, m.selectedPlatform(), platform.ActionChooseExportZIP)
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, tea.WindowSizeMsg{Width: 100, Height: 24})

	next = updateModel(t, next, mouseWheel(4, tea.MouseWheelDown))
	if next.importPickerCursor != 1 {
		t.Fatalf("expected picker wheel to move cursor, got %d", next.importPickerCursor)
	}

	y := lineIndexContaining(t, next.View().Content, "export-02.zip")
	updated, cmd := next.Update(mouseClick(4, y))
	if cmd == nil {
		t.Fatalf("expected click on ZIP row to start import")
	}
	next = requireModel(t, updated)
	if next.current != screenImporting || !strings.HasSuffix(next.importSource, "export-02.zip") {
		t.Fatalf("expected clicked ZIP to start import, screen=%v source=%q", next.current, next.importSource)
	}
}

func TestPlanLoadPathShowsFriendlyMissingFileError(t *testing.T) {
	next := openPlanLoadPath(t, NewModel())
	if next.current != screenPlanLoadPath {
		t.Fatalf("expected plan load path screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Load Cleanup Plan") {
		t.Fatalf("expected load screen, got:\n%s", next.View().Content)
	}

	missingPath := filepath.Join(t.TempDir(), "missing-plan.json")
	next.planPathInput.SetValue(missingPath)
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected plan load command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	view := next.View().Content
	if next.current != screenPlanLoadPath {
		t.Fatalf("expected to stay on load path after error, got %v", next.current)
	}
	if !strings.Contains(view, "Plan file not found") {
		t.Fatalf("expected friendly missing file error, got:\n%s", view)
	}
}

func TestPlanLoadSuccessShowsSummaryAndActionsBrowser(t *testing.T) {
	plan := fakeCleanupPlan()
	path := writeTUIPlan(t, plan)

	next := openPlanLoadPath(t, NewModel())
	next.planPathInput.SetValue(path)
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected plan load command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected loaded plan summary, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Loaded Cleanup Plan",
		"Plan ID: plan-loaded",
		"Platform: instagram",
		"Source: instagram-export",
		"Mode: dry-run",
		"Total actions: 3",
		"Action Counts",
		"delete_comment: 1",
		"unfollow: 1",
		"unlike: 1",
		"Status Counts",
		"pending: 1",
		"done: 1",
		"skipped: 1",
		"Apply preview",
		"View actions",
		"Back home",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected summary to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenLoadedPlanActions {
		t.Fatalf("expected actions browser, got %v", next.current)
	}
	view = next.View().Content
	for _, want := range []string{
		"Plan Actions",
		"unlike",
		"pending",
		"/p/1",
		"Type: unlike",
		"Status: pending",
		"Target URL: https://instagram.example/p/1",
		"Target ID: target-1",
		"Source activity item ID: item-1",
		"Created at: 2026-06-26T12:00:00Z",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected actions browser to contain %q, got:\n%s", want, view)
		}
	}
	actionRow := lineContaining(t, view, "unlike", "pending")
	if strings.Contains(actionRow, "https://") {
		t.Fatalf("expected compact action row not to expose full URL, got %q", actionRow)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.loadedActionCursor != 1 {
		t.Fatalf("expected j to move action cursor, got %d", next.loadedActionCursor)
	}
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected esc to return to summary, got %v", next.current)
	}
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenHome {
		t.Fatalf("expected esc to return home, got %v", next.current)
	}
}

func TestLoadedPlanQuitConfirmation(t *testing.T) {
	plan := fakeCleanupPlan()
	path := writeTUIPlan(t, plan)

	next := openPlanLoadPath(t, NewModel())
	next.planPathInput.SetValue(path)
	updated, cmd := next.Update(keyPress("enter"))
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	next = updateModel(t, next, keyPress("ctrl+q"))
	if next.current != screenQuitConfirm {
		t.Fatalf("expected ctrl+q to open quit confirmation, got %v", next.current)
	}
	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenLoadedPlanSummary {
		t.Fatalf("expected esc to return to loaded summary, got %v", next.current)
	}
}

func TestImportResultViewShowsSummaryCountsAndActions(t *testing.T) {
	m := NewModel()
	result := instagram.ImportResult{
		Summary: instagram.ImportSummary{
			Total:     4,
			Likes:     1,
			Comments:  1,
			Following: 1,
			Followers: 1,
			Skipped:   1,
		},
		Warnings: instagram.ImportWarningSummary{
			Total: 1,
			Groups: []instagram.ImportWarningGroup{{
				SourceFile: "settings/unknown_shape.json",
				Category:   "instagram-json",
				Reason:     "unsupported activity file",
				Unit:       instagram.WarningUnitFile,
				Count:      1,
			}},
		},
	}

	updated, cmd := m.Update(importFinishedMsg{result: result, source: "demo instagram export"})
	if cmd != nil {
		t.Fatalf("expected result message not to return a command")
	}

	next := requireModel(t, updated)
	view := next.View().Content
	for _, want := range []string{
		"Import Complete",
		"Source",
		"Parsed Items",
		"Total: 4",
		"Likes: 1",
		"Comments: 1",
		"Following: 1",
		"Followers: 1",
		"Import Notes",
		"Skipped or unknown: 1",
		"Warnings: 1",
		"View parsed items",
		"View warnings",
		"Review selection",
		"Back home",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected result view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestImportResultActionsOpenItemsAndWarnings(t *testing.T) {
	m := importedModel(t, fakeImportResult())

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected enter to open items browser, got %v", next.current)
	}
	itemView := next.View().Content
	for _, want := range []string{
		"Parsed Items",
		"[ ] like",
		"demo_artist",
		"/p/demo_like",
		"Type: like",
		"Actor: demo_artist",
		"Target: /p/demo_like/",
		"Date: 2024-03-09",
		"Toggle selected",
		"Review selection",
	} {
		if !strings.Contains(itemView, want) {
			t.Fatalf("expected items browser to contain %q, got:\n%s", want, itemView)
		}
	}
	itemRow := lineContaining(t, itemView, "[ ]", "demo_artist")
	if strings.Contains(itemRow, "https://") {
		t.Fatalf("expected compact item row not to expose full URL, got %q", itemRow)
	}
	if strings.Contains(itemView, "raw private comment") {
		t.Fatalf("expected items browser not to render raw text preview, got:\n%s", itemView)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenImportResult {
		t.Fatalf("expected esc to return to import summary, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenWarnings {
		t.Fatalf("expected enter to open warnings, got %v", next.current)
	}
	warningView := next.View().Content
	if !strings.Contains(warningView, "Import Warnings") || !strings.Contains(warningView, "unsupported activity file") {
		t.Fatalf("expected warnings view, got:\n%s", warningView)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenImportResult {
		t.Fatalf("expected esc to return to import summary, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenSelectionSummary {
		t.Fatalf("expected review selection action to open summary, got %v", next.current)
	}
}

func TestItemsBrowserNavigationUsesArrowAndJK(t *testing.T) {
	result := fakeImportResult()
	result.Items = append(result.Items, domain.ActivityItem{
		ID:        "item-follow",
		Platform:  domain.PlatformInstagram,
		Type:      domain.ItemTypeFollow,
		Actor:     "demo_following",
		TargetID:  "demo_following",
		Metadata:  map[string]string{"relationship": "following"},
		Source:    domain.SourceMetadata{FileName: "following.json"},
		TargetURL: "https://www.instagram.com/demo_following/",
	})

	m := importedModel(t, result)
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("down"))
	if next.itemCursor != 1 {
		t.Fatalf("expected down to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.itemCursor != 2 {
		t.Fatalf("expected j to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("j"))
	if next.itemCursor != 2 {
		t.Fatalf("expected item cursor to stay at bottom, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("up"))
	if next.itemCursor != 1 {
		t.Fatalf("expected up to move item cursor, got %d", next.itemCursor)
	}

	next = updateModel(t, next, keyPress("k"))
	if next.itemCursor != 0 {
		t.Fatalf("expected k to move item cursor, got %d", next.itemCursor)
	}
}

func TestItemsBrowserShowsVisibleAndTotalCount(t *testing.T) {
	m := importedModel(t, fakeImportResult())

	next := updateModel(t, m, keyPress("enter"))
	view := next.View().Content
	plain := stripANSI(view)

	for _, want := range []string{
		"Showing 1-2 of 2 · Matching 2/2 · Selected 0 · Filters off",
		"Page 1/1",
		"Selection",
		"Actions",
		"Toggle selected",
		"Review selection",
		"Generate dry-run plan",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected parsed items cockpit to contain %q, got:\n%s", want, view)
		}
	}
	if !strings.Contains(plain, footerParsedItems) {
		t.Fatalf("expected parsed items footer, got:\n%s", view)
	}
}

func TestItemsBrowserRowsHideLowPriorityColumnsWhenTight(t *testing.T) {
	next := importedModel(t, fakeImportResult())
	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, tea.WindowSizeMsg{Width: 84, Height: 28})

	view := next.View().Content
	row := lineContaining(t, view, "[ ]", "demo_artist")
	segment := strings.TrimSpace(stripANSI(firstPaneSegment(row)))
	for _, unwanted := range []string{"2024-03-09", "https://"} {
		if strings.Contains(segment, unwanted) {
			t.Fatalf("expected tight parsed row to hide %q, got %q in:\n%s", unwanted, segment, view)
		}
	}
	listWidth, _ := twoPaneWidths(layoutSpec(next.width, next.height), "Parsed Items")
	if lipgloss.Width(segment) > maxInt(1, paneTextWidth(listWidth)) {
		t.Fatalf("tight parsed row width %d exceeds pane content width %d: %q", lipgloss.Width(segment), paneTextWidth(listWidth), segment)
	}
}

func TestItemsBrowserRowsExposeDatesAtMediumWidth(t *testing.T) {
	for _, width := range []int{120, 132} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			next := importedModel(t, fakeImportResultWithManyItems(24))
			next = updateModel(t, next, keyPress("enter"))
			next = updateModel(t, next, tea.WindowSizeMsg{Width: width, Height: 36})

			view := next.View().Content
			foundDate := false
			for _, line := range strings.Split(view, "\n") {
				segment := strings.TrimSpace(stripANSI(firstPaneSegment(line)))
				if !strings.Contains(segment, "[ ]") {
					continue
				}
				if strings.Contains(segment, "2024-") {
					foundDate = true
				}
				if strings.Contains(segment, "2024-") && strings.HasSuffix(segment, "-") {
					t.Fatalf("expected date to stay single-line, got %q in:\n%s", segment, view)
				}
			}
			if !foundDate {
				t.Fatalf("expected medium parsed rows to show date column, got:\n%s", view)
			}
		})
	}
}

func TestItemsBrowserRightColumnFillsListHeight(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 132, Height: 36},
		{Width: 160, Height: 60},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			next := importedModel(t, fakeImportResultWithManyItems(24))
			next = updateModel(t, next, keyPress("enter"))
			next = updateModel(t, next, size)

			lines := strings.Split(stripANSI(next.View().Content), "\n")
			lastPaneBottom := -1
			for i, line := range lines {
				if strings.Contains(line, "└") {
					lastPaneBottom = i
				}
			}
			if lastPaneBottom < 0 {
				t.Fatalf("expected pane bottom borders, got:\n%s", next.View().Content)
			}
			if strings.Count(lines[lastPaneBottom], "└") < 2 {
				t.Fatalf("expected parsed list and action pane bottoms to align on one line, got line %d %q in:\n%s", lastPaneBottom, lines[lastPaneBottom], next.View().Content)
			}
		})
	}
}

func TestItemsBrowserSelectionRowsAndToggleWithSpace(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	view := next.View().Content
	if !strings.Contains(view, "[ ] like") || !strings.Contains(view, "demo_artist") {
		t.Fatalf("expected unselected item row, got:\n%s", view)
	}

	next = updateModel(t, next, keyPress(" "))
	view = next.View().Content
	if next.selection.Len() != 1 {
		t.Fatalf("expected one selected item, got %d", next.selection.Len())
	}
	for _, want := range []string{"[x] like", "demo_artist", "Showing 1-2 of 2 · Matching 2/2 · Selected 1 · Filters off", "Selected: 1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress(" "))
	if next.selection.Len() != 0 {
		t.Fatalf("expected toggle to deselect item, got %d", next.selection.Len())
	}
}

func TestItemsBrowserToggleWithEnter(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("enter"))
	if next.selection.Len() != 1 || !next.selection.Contains("item-like") {
		t.Fatalf("expected enter to toggle highlighted item selection")
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected second enter to deselect highlighted item, got %d", next.selection.Len())
	}
}

func TestItemsBrowserFillsAvailablePaneAndShowsPageRange(t *testing.T) {
	m := importedModel(t, fakeImportResultWithManyItems(24))
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, tea.WindowSizeMsg{Width: 120, Height: 32})

	viewport := next.parsedItemsViewport()
	if viewport.VisibleRows <= 10 {
		t.Fatalf("expected parsed item viewport to use pane height, got %d rows", viewport.VisibleRows)
	}
	boxes := hitBoxesForTest(t, next)
	rowBoxes := 0
	for _, box := range boxes {
		if box.Target.Kind == hitParsedItemRow {
			rowBoxes++
		}
	}
	if rowBoxes != viewport.End-viewport.Offset {
		t.Fatalf("expected %d row hit boxes, got %d", viewport.End-viewport.Offset, rowBoxes)
	}

	plain := stripANSI(next.View().Content)
	for _, want := range []string{
		fmt.Sprintf("1-%d/24", viewport.End),
		"Match 24/24",
		"Page 1/",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected viewport status to contain %q, got:\n%s", want, next.View().Content)
		}
	}
}

func TestItemsBrowserPageKeysMoveByRenderedViewport(t *testing.T) {
	next := updateModel(t, importedModel(t, fakeImportResultWithManyItems(24)), keyPress("enter"))
	next = updateModel(t, next, tea.WindowSizeMsg{Width: 120, Height: 24})
	pageSize := next.parsedItemsViewport().VisibleRows

	next = updateModel(t, next, keyPress("pgdown"))
	if next.itemCursor != pageSize {
		t.Fatalf("expected pgdown to move by page size %d, got cursor %d", pageSize, next.itemCursor)
	}
	if next.parsedItemsViewport().Page < 2 {
		t.Fatalf("expected pgdown to advance page indicator, got %#v", next.parsedItemsViewport())
	}

	next = updateModel(t, next, keyPress("pgup"))
	if next.itemCursor != 0 || next.itemOffset != 0 {
		t.Fatalf("expected pgup to return to first page, cursor=%d offset=%d", next.itemCursor, next.itemOffset)
	}
}

func TestItemsBrowserTabFocusActionsToggleAndReview(t *testing.T) {
	next := updateModel(t, importedModel(t, fakeImportResult()), keyPress("enter"))

	next = updateModel(t, next, keyPress("tab"))
	if next.itemFocus != itemFocusActions || next.itemActionCursor != parsedActionToggle {
		t.Fatalf("expected tab to focus actions at toggle, focus=%d cursor=%d", next.itemFocus, next.itemActionCursor)
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.selection.Len() != 1 || !next.selection.Contains("item-like") {
		t.Fatalf("expected action enter to toggle highlighted item, selection=%d", next.selection.Len())
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenSelectionSummary {
		t.Fatalf("expected review selection action to open summary, got %v", next.current)
	}
}

func TestItemsBrowserGenerateActionReusesPlanFlow(t *testing.T) {
	next := updateModel(t, importedModel(t, fakeImportResult()), keyPress("enter"))
	next = updateModel(t, next, keyPress(" "))
	next = updateModel(t, next, keyPress("tab"))
	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("down"))
	if next.itemActionCursor != parsedActionGeneratePlan {
		t.Fatalf("expected generate action to be focusable after selection, got %d", next.itemActionCursor)
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenPlanPreview || !next.hasPlanPreview() {
		t.Fatalf("expected generate action to open plan preview, screen=%v", next.current)
	}
}

func TestItemsBrowserMouseClickActionUsesRightPaneHitBox(t *testing.T) {
	next := updateModel(t, importedModel(t, fakeImportResult()), keyPress("enter"))
	box := requireHitBox(t, hitBoxesForTest(t, next), hitParsedAction, parsedActionReviewSelection, "")

	next = updateModel(t, next, mouseClick(box.X, box.Y))
	if next.current != screenSelectionSummary {
		t.Fatalf("expected right-pane review action click to open summary, got %v", next.current)
	}
}

func TestMouseClickParsedItemHighlightsThenTogglesFocusedRow(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	y := lineIndexContaining(t, next.View().Content, "comment", "demo_bakery")

	next = updateModel(t, next, mouseClick(4, y))
	if next.itemCursor != 1 || next.selection.Len() != 0 {
		t.Fatalf("expected first click to highlight second row only, cursor=%d selection=%d", next.itemCursor, next.selection.Len())
	}

	y = lineIndexContaining(t, next.View().Content, "comment", "demo_bakery")
	next = updateModel(t, next, mouseClick(4, y))
	if !next.selection.Contains("item-comment") {
		t.Fatalf("expected second click on highlighted row to toggle selection")
	}
}

func TestMouseHoverDoesNotChangeSelectionOrCursor(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	y := lineIndexContaining(t, next.View().Content, "comment", "demo_bakery")

	next = updateModel(t, next, mouseMotion(4, y))
	if next.itemCursor != 0 {
		t.Fatalf("expected hover not to move item cursor, got %d", next.itemCursor)
	}
	if next.selection.Len() != 0 {
		t.Fatalf("expected hover not to change selection, got %d", next.selection.Len())
	}
	if next.hoverTarget.Kind != hitParsedItemRow || next.hoverTarget.Index != 1 {
		t.Fatalf("expected hover target for second parsed row, got %#v", next.hoverTarget)
	}
}

func TestMouseWheelMovesScrollableParsedItemList(t *testing.T) {
	result := fakeImportResult()
	result.Items = nil
	for i := 0; i < 12; i++ {
		occurred := time.Date(2026, 6, 1+i, 12, 0, 0, 0, time.UTC)
		result.Items = append(result.Items, domain.ActivityItem{
			ID:         fmt.Sprintf("item-%02d", i),
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeLike,
			Actor:      fmt.Sprintf("demo_%02d", i),
			TargetURL:  fmt.Sprintf("https://www.instagram.com/p/demo_%02d/", i),
			TargetID:   fmt.Sprintf("target-%02d", i),
			OccurredAt: &occurred,
		})
	}
	result.Summary.Total = len(result.Items)
	result.Summary.Likes = len(result.Items)

	next := updateModel(t, importedModel(t, result), keyPress("enter"))
	next = updateModel(t, next, tea.WindowSizeMsg{Width: 80, Height: 12})

	for range 6 {
		next = updateModel(t, next, mouseWheel(4, tea.MouseWheelDown))
	}
	if next.itemCursor != 6 {
		t.Fatalf("expected wheel down to move cursor to 6, got %d", next.itemCursor)
	}
	if next.itemOffset <= 0 {
		t.Fatalf("expected scroll offset to advance, got %d", next.itemOffset)
	}

	for range 20 {
		next = updateModel(t, next, mouseWheel(4, tea.MouseWheelDown))
	}
	if next.itemCursor != len(result.Items)-1 {
		t.Fatalf("expected wheel down to clamp at bottom, got %d", next.itemCursor)
	}
	if next.itemOffset < 0 || next.itemOffset >= len(result.Items) {
		t.Fatalf("expected offset bounds to be preserved, got %d", next.itemOffset)
	}

	next = updateModel(t, next, mouseWheel(4, tea.MouseWheelUp))
	if next.itemCursor != len(result.Items)-2 {
		t.Fatalf("expected wheel up to move one row, got %d", next.itemCursor)
	}
}

func TestMouseClickScrolledLoadedPlanActionHighlightsAbsoluteRow(t *testing.T) {
	plan := fakeCleanupPlan()
	plan.Actions = make([]domain.CleanupAction, 30)
	for index := range plan.Actions {
		plan.Actions[index] = domain.CleanupAction{
			ID:                   fmt.Sprintf("action-%02d", index),
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionUnlike,
			TargetURL:            fmt.Sprintf("https://www.instagram.com/p/SYNTHETIC%02d/", index),
			SourceActivityItemID: fmt.Sprintf("item-%02d", index),
			Status:               domain.ActionStatusPending,
		}
	}

	m := NewModel()
	m.current = screenLoadedPlanActions
	m.loadedPlan = plan
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 88, Height: 20})
	for range 14 {
		m = updateModel(t, m, mouseWheel(4, tea.MouseWheelDown))
	}
	if m.loadedActionOffset == 0 {
		t.Fatal("expected loaded actions to scroll beyond first viewport")
	}

	boxes := hitBoxesOfKind(hitBoxesForTest(t, m), hitLoadedPlanRow)
	if len(boxes) < 3 || len(boxes) > m.planActionListHeight() {
		t.Fatalf("expected only visible loaded-action hit boxes, got %d for height %d", len(boxes), m.planActionListHeight())
	}
	middle := boxes[len(boxes)/2]
	wantIndex := m.loadedActionOffset + len(boxes)/2
	if middle.Target.Index != wantIndex {
		t.Fatalf("loaded-action hit index = %d, want absolute index %d", middle.Target.Index, wantIndex)
	}

	next := updateModel(t, m, mouseClick(middle.X, middle.Y))
	if next.loadedActionCursor != wantIndex || next.loadedPlan.Actions[next.loadedActionCursor].ID != fmt.Sprintf("action-%02d", wantIndex) {
		t.Fatalf("expected clicked loaded action %d to be highlighted, cursor=%d action=%q", wantIndex, next.loadedActionCursor, next.loadedPlan.Actions[next.loadedActionCursor].ID)
	}
}

func TestMouseClickScrolledWarningHighlightsAbsoluteGroup(t *testing.T) {
	m := NewModel()
	m.current = screenWarnings
	m.importSource = "synthetic export"
	m.importResult.WarningCount = 30
	m.importResult.Warnings = make([]activityWarningGroup, 30)
	for index := range m.importResult.Warnings {
		m.importResult.Warnings[index] = activityWarningGroup{
			SourceFile: fmt.Sprintf("warning-%02d.json", index),
			Category:   fmt.Sprintf("synthetic-%02d", index),
			Reason:     "unsupported target shape",
			Unit:       "record",
			Count:      1,
		}
	}
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 88, Height: 20})
	for range 14 {
		m = updateModel(t, m, mouseWheel(4, tea.MouseWheelDown))
	}
	if m.warningOffset == 0 {
		t.Fatal("expected warnings to scroll beyond first viewport")
	}

	boxes := hitBoxesOfKind(hitBoxesForTest(t, m), hitWarningRow)
	_, _, visibleRows := m.warningViewport()
	if len(boxes) < 3 || len(boxes) > visibleRows {
		t.Fatalf("expected only visible warning hit boxes, got %d for height %d", len(boxes), visibleRows)
	}
	middle := boxes[len(boxes)/2]
	wantIndex := m.warningOffset + len(boxes)/2
	if middle.Target.Index != wantIndex {
		t.Fatalf("warning hit index = %d, want absolute index %d", middle.Target.Index, wantIndex)
	}

	next := updateModel(t, m, mouseClick(middle.X, middle.Y))
	if next.warningCursor != wantIndex || next.importResult.Warnings[next.warningCursor].SourceFile != fmt.Sprintf("warning-%02d.json", wantIndex) {
		t.Fatalf("expected clicked warning %d to be highlighted, cursor=%d source=%q", wantIndex, next.warningCursor, next.importResult.Warnings[next.warningCursor].SourceFile)
	}
}

func TestBackspaceNavigatesWhenNoInputFocused(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected items browser, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("backspace"))
	if next.current != screenImportResult {
		t.Fatalf("expected backspace to return to import result, got %v", next.current)
	}
}

func TestItemsBrowserSelectsAndDeselectsVisibleItems(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = applyTypeFilter(t, next, filterRowLike)

	next = updateModel(t, next, keyPress("a"))
	if next.selection.Len() != 1 || !next.selection.Contains("item-like") {
		t.Fatalf("expected filtered like to be selected")
	}
	if next.selection.Contains("item-comment") {
		t.Fatalf("expected hidden comment not to be selected")
	}

	next = updateModel(t, next, keyPress("n"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected visible like to be deselected, got %d", next.selection.Len())
	}
}

func TestItemsBrowserSelectionShortcutsAcceptUppercase(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("A"))
	if next.selection.Len() != 2 {
		t.Fatalf("expected uppercase A to select visible items, got %d", next.selection.Len())
	}

	next = updateModel(t, next, keyPress("N"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected uppercase N to deselect visible items, got %d", next.selection.Len())
	}

	next = updateModel(t, next, keyPress("S"))
	if next.current != screenSelectionSummary {
		t.Fatalf("expected uppercase S to open selection summary, got %v", next.current)
	}
}

func TestSelectionPersistsWhenFiltersChangeAndClear(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress(" "))

	next = applyTypeFilter(t, next, filterRowComment)
	if !next.selection.Contains("item-like") {
		t.Fatalf("expected selection to persist after filter changed")
	}
	if !strings.Contains(next.View().Content, "Showing 1-1 of 1 · Matching 1/4 · Selected 1 · Filters active") {
		t.Fatalf("expected selected count to persist, got:\n%s", next.View().Content)
	}

	next = clearFilters(t, next)
	if !next.selection.Contains("item-like") {
		t.Fatalf("expected selection to persist after filters clear")
	}
	if !strings.Contains(next.View().Content, "Showing 1-4 of 4 · Matching 4/4 · Selected 1 · Filters off") {
		t.Fatalf("expected selected count after clear filters, got:\n%s", next.View().Content)
	}
}

func TestNewImportResetsSelection(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress(" "))

	if next.selection.Len() != 1 {
		t.Fatalf("expected test setup to select item")
	}

	updated, _ := next.Update(importFinishedMsg{result: fakeImportResultWithRelationships(), source: "new demo"})
	next = requireModel(t, updated)

	if next.selection.Len() != 0 {
		t.Fatalf("expected new import to reset selection, got %d", next.selection.Len())
	}
}

func TestSelectionSummaryShowsCountsAndClearSelection(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("a"))
	next = updateModel(t, next, keyPress("s"))

	view := next.View().Content
	for _, want := range []string{
		"Selection Summary",
		"Selection Dashboard",
		"Selection Totals",
		"Total selected: 4",
		"Visible items: 4",
		"All parsed items: 4",
		"Selected Type Counts",
		"Likes: 1",
		"Comments: 1",
		"Following: 1",
		"Followers: 1",
		"Current Filters",
		"Filters: off",
		"Next Suggested Action",
		"Generate a dry-run plan.",
		"Generate dry-run plan",
		"View selected items",
		"Select all visible items",
		"Deselect all visible items",
		"Clear selection",
		"Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected selection summary to contain %q, got:\n%s", want, view)
		}
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenSelectedItems {
		t.Fatalf("expected selected items screen, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Selected Items") || !strings.Contains(next.View().Content, "[x] like") || !strings.Contains(next.View().Content, "demo_artist") {
		t.Fatalf("expected selected items list, got:\n%s", next.View().Content)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenSelectionSummary {
		t.Fatalf("expected esc to return to selection summary, got %v", next.current)
	}
	for range 3 {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected clear selection to remove all selected IDs, got %d", next.selection.Len())
	}
	if !strings.Contains(next.View().Content, "Total selected: 0") {
		t.Fatalf("expected summary to show cleared selection, got:\n%s", next.View().Content)
	}
}

func TestSelectionSummarySelectsAndDeselectsVisibleItems(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = applyTypeFilter(t, next, filterRowLike)
	next = updateModel(t, next, keyPress("s"))

	for range selectionSelectVisible {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	if next.selection.Len() != 1 || !next.selection.Contains("item-like") {
		t.Fatalf("expected select visible to select only filtered item")
	}
	if !strings.Contains(next.View().Content, "Selected all visible items.") {
		t.Fatalf("expected select visible status, got:\n%s", next.View().Content)
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.selection.Len() != 0 {
		t.Fatalf("expected deselect visible to clear filtered item, got %d", next.selection.Len())
	}
}

func TestSelectionSummaryWithoutSelectionShowsPlanMessage(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("s"))

	next = updateModel(t, next, keyPress("enter"))

	if next.current != screenSelectionSummary {
		t.Fatalf("expected to stay on selection summary, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Select at least one item before generating a plan.") {
		t.Fatalf("expected friendly no-selection message, got:\n%s", next.View().Content)
	}
}

func TestPlanPreviewShowsCountsAndUnsupportedFollowers(t *testing.T) {
	next := planPreviewModel(t)

	if next.current != screenPlanPreview {
		t.Fatalf("expected plan preview, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Dry-Run Plan Preview",
		"Plan",
		"Mode: dry-run",
		"Platform: instagram",
		"Selected items: 4",
		"Action Counts",
		"Supported actions: 3",
		"Unlike: 1",
		"Delete comment: 1",
		"Unfollow: 1",
		"Skipped",
		"Unsupported selected items: 1",
		"unlike",
		"pending",
		"/p/demo_like",
		"delete_comment",
		"demo_bakery",
		"unfollow",
		"/demo_following",
		"skipped",
		"item-follower",
		"Export JSON",
		"Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected plan preview to contain %q, got:\n%s", want, view)
		}
	}
	previewRow := lineContaining(t, view, "unlike", "/p/demo_like")
	if strings.Contains(previewRow, "https://") {
		t.Fatalf("expected plan preview row not to expose full URL, got %q", previewRow)
	}
}

func TestRedditSelectionGeneratesDryRunPlan(t *testing.T) {
	occurred := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	next := NewModel()
	next.importPlatform = domain.PlatformReddit
	next.importSource = redditSourceLabel()
	next.importResult = activityResult{
		Items: []domain.ActivityItem{
			{
				ID:         "reddit-comment",
				Platform:   domain.PlatformReddit,
				Type:       domain.ItemTypeComment,
				Actor:      "test_user",
				TargetID:   "t1_comment",
				TargetURL:  "https://www.reddit.com/r/test/comments/post/comment/",
				OccurredAt: &occurred,
				Source:     domain.SourceMetadata{Name: "reddit-api", ImportedAt: &occurred},
			},
			{
				ID:         "reddit-post",
				Platform:   domain.PlatformReddit,
				Type:       domain.ItemTypePost,
				Actor:      "test_user",
				TargetID:   "t3_post",
				TargetURL:  "https://www.reddit.com/r/test/comments/post/title/",
				OccurredAt: &occurred,
				Source:     domain.SourceMetadata{Name: "reddit-api", ImportedAt: &occurred},
			},
		},
		Summary: activitySummary{Total: 2, Comments: 1, Posts: 1},
	}
	next.selection.Toggle("reddit-comment")
	next.selection.Toggle("reddit-post")
	next.current = screenSelectionSummary

	next.generatePlanFromSelection()

	if next.current != screenPlanPreview {
		t.Fatalf("expected Reddit plan preview, got %v", next.current)
	}
	if next.planResult.Plan.Platform != domain.PlatformReddit || len(next.planResult.Plan.Actions) != 2 {
		t.Fatalf("unexpected Reddit plan result: %#v", next.planResult)
	}
	if next.planResult.Plan.Actions[0].Type != domain.ActionRedditDeleteComment || next.planResult.Plan.Actions[1].Type != domain.ActionRedditDeletePost {
		t.Fatalf("unexpected Reddit action types: %#v", next.planResult.Plan.Actions)
	}
	view := next.View().Content
	for _, want := range []string{
		"Platform: reddit",
		"Reddit delete comment: 1",
		"Reddit delete post: 1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected Reddit plan preview to contain %q, got:\n%s", want, view)
		}
	}
}

func TestApplyPreviewConfirmationAndNoopResultForGeneratedPlan(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	next := NewModelWithWorkspace(w, nil)
	next = updateModel(t, next, importFinishedMsg{result: fakeImportResultWithRelationships(), source: "demo instagram export"})
	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, keyPress("a"))
	next = updateModel(t, next, keyPress("s"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenPlanPreview {
		t.Fatalf("expected plan preview, got %v", next.current)
	}

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenApplyPreview {
		t.Fatalf("expected apply preview, got %v", next.current)
	}
	view := next.View().Content
	for _, want := range []string{
		"Apply Preview",
		"Platform: instagram",
		"Mode: simulation",
		"Executor: instagram-simulation",
		"Status: Ready",
		"Provider ready: ready",
		"Pending: 3",
		"Unsupported: 0",
		"Simulate no-op run",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected apply preview to contain %q, got:\n%s", want, view)
		}
	}
	previewEvent := requireAuditEvent(t, w, string(apply.EventPreviewed))
	if previewEvent.Fields["execution_mode"] != "simulation" || previewEvent.Fields["executor"] != "instagram-simulation" || previewEvent.Fields["provider_ready"] != true {
		t.Fatalf("unexpected provider-routed preview audit: %#v", previewEvent.Fields)
	}
	if _, ok := previewEvent.Fields["reddit_account_ready"]; ok {
		t.Fatalf("preview audit retained Reddit-specific readiness: %#v", previewEvent.Fields)
	}

	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenApplyConfirm {
		t.Fatalf("expected apply confirm, got %v", next.current)
	}
	if !strings.Contains(next.View().Content, "Simulate no-op run for 3 pending actions?") ||
		!strings.Contains(next.View().Content, "No platform content changes.") {
		t.Fatalf("expected short confirmation copy, got:\n%s", next.View().Content)
	}

	next = updateModel(t, next, keyPress("up"))
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected no-op apply command")
	}
	next = requireModel(t, updated)
	if next.current != screenApplyRunning {
		t.Fatalf("expected apply running, got %v", next.current)
	}
	next = updateModel(t, next, runApplyCmd(next.currentApplyPlan(), next.applyPlanSource, next.applyRuntimeState())())

	if next.current != screenApplyResult || next.applyExecution.State != apply.ExecutionStateDone {
		t.Fatalf("expected apply result, screen=%v execution=%#v", next.current, next.applyExecution)
	}
	if next.applyExecution.Counts.Done != 3 || next.applyExecution.Counts.Pending != 0 {
		t.Fatalf("unexpected no-op result counts: %#v", next.applyExecution.Counts)
	}
	for _, action := range next.planResult.Plan.Actions {
		if action.Status != domain.ActionStatusDone {
			t.Fatalf("expected no-op action status done, got %#v", next.planResult.Plan.Actions)
		}
	}
	resultView := next.View().Content
	for _, want := range []string{
		"Apply Result",
		"State: done",
		"No platform changes: yes",
		"Done: 3",
		"Back to plan",
	} {
		if !strings.Contains(resultView, want) {
			t.Fatalf("expected apply result to contain %q, got:\n%s", want, resultView)
		}
	}
	requireAuditEvent(t, w, string(apply.EventConfirmed))
	requireAuditEvent(t, w, string(apply.EventExecutionStarted))
	actionEvent := requireAuditEvent(t, w, string(apply.EventActionResult))
	if actionEvent.Fields["execution_mode"] != "simulation" || actionEvent.Fields["executor"] != "instagram-simulation" {
		t.Fatalf("unexpected provider-routed action audit: %#v", actionEvent.Fields)
	}
	if _, ok := actionEvent.Fields["target_url"]; ok {
		t.Fatalf("apply action audit should not include target URL: %#v", actionEvent.Fields)
	}
	if _, ok := actionEvent.Fields["username"]; ok {
		t.Fatalf("apply action audit should not include username: %#v", actionEvent.Fields)
	}
	requireAuditEvent(t, w, string(apply.EventExecutionFinished))
}

func TestManualCleanupLifecycleResumesAfterRestart(t *testing.T) {
	const rawPreview = "synthetic private comment preview"
	previousOpen := openExternalURL
	opened := []string{}
	openExternalURL = func(rawURL string) error {
		opened = append(opened, rawURL)
		return nil
	}
	t.Cleanup(func() { openExternalURL = previousOpen })

	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan := manualCleanupTestPlan(now)
	plan.Actions[0].Metadata = map[string]string{"selection": "synthetic"}
	plan.Actions[1].Status = domain.ActionStatusStopped
	plan.Actions[1].CreatedAt = now.Add(time.Minute)
	plan.Actions[3].Status = domain.ActionStatusFailed
	plan.Actions[3].CreatedAt = now.Add(3 * time.Minute)
	before, _ := json.Marshal(plan)
	m := NewModelWithWorkspace(w, nil)
	m.current = screenPlanPreview
	m.planResult.Plan = plan
	m.importResult.Items = manualCleanupTestItems(now, rawPreview)
	m.refreshManualAvailability(plan)

	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenManualCleanupAction {
		t.Fatalf("expected manual action screen, got %v error=%q", next.current, next.manualError)
	}
	if !strings.Contains(next.View().Content, "Unfollow @demo_following") {
		t.Fatalf("unexpected unfollow view:\n%s", next.View().Content)
	}
	requireAuditEvent(t, w, "manual_cleanup_session_started")

	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil || len(opened) != 0 {
		t.Fatal("target must open only through explicit deferred command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())
	if len(opened) != 1 || opened[0] != "https://www.instagram.com/demo_following/" {
		t.Fatalf("opened targets=%#v", opened)
	}
	requireAuditEvent(t, w, "manual_cleanup_target_opened")

	next.manualActionCursor = 1
	next = updateModel(t, next, keyPress("enter"))
	if !strings.Contains(next.View().Content, "Unlike reel") {
		t.Fatalf("expected unlike action, got:\n%s", next.View().Content)
	}
	requireAuditEvent(t, w, "manual_cleanup_action_done")
	updated, cmd = next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected unlike target command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())
	if len(opened) != 2 || opened[1] != "https://www.instagram.com/reel/REEL1/" {
		t.Fatalf("unlike target=%#v", opened)
	}

	next.manualActionCursor = 2
	next = updateModel(t, next, keyPress("enter"))
	commentView := next.View().Content
	for _, want := range []string{"Delete own comment", "Post owner: @post_owner", "Comment date: 2026-07-11", "Comment: " + rawPreview, "Post: /p/POST1"} {
		if !strings.Contains(commentView, want) {
			t.Fatalf("comment view missing %q:\n%s", want, commentView)
		}
	}
	requireAuditEvent(t, w, "manual_cleanup_action_skipped")
	updated, cmd = next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected comment target command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())
	if len(opened) != 3 || opened[2] != "https://www.instagram.com/p/POST1/" {
		t.Fatalf("comment target=%#v", opened)
	}

	next.manualActionCursor = 3
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenPlanPreview || !strings.Contains(next.View().Content, "Progress saved") {
		t.Fatalf("stop did not return to plan: screen=%v view=\n%s", next.current, next.View().Content)
	}
	requireAuditEvent(t, w, "manual_cleanup_session_stopped")

	restarted := NewModelWithWorkspace(w, nil)
	if !restarted.manualSessionLoaded {
		t.Fatal("restart did not load unfinished manual cleanup")
	}
	updated, _ = restarted.activateTab("Plans")
	restarted = requireModel(t, updated)
	if restarted.current != screenManualCleanupChoice || !strings.Contains(restarted.View().Content, "Resume manual cleanup") {
		t.Fatalf("restart did not offer resume: screen=%v view=\n%s", restarted.current, restarted.View().Content)
	}
	restarted = updateModel(t, restarted, keyPress("enter"))
	if restarted.current != screenManualCleanupAction || strings.Contains(restarted.View().Content, rawPreview) {
		t.Fatalf("resumed action must work without persisted preview:\n%s", restarted.View().Content)
	}
	requireAuditEvent(t, w, "manual_cleanup_session_resumed")

	restarted.manualActionCursor = 1
	restarted = updateModel(t, restarted, keyPress("enter"))
	if restarted.current != screenManualCleanupResult {
		t.Fatalf("expected completed result, got %v", restarted.current)
	}
	if !strings.Contains(restarted.View().Content, "2 done · 1 skipped") {
		t.Fatalf("unexpected result:\n%s", restarted.View().Content)
	}
	requireAuditEvent(t, w, "manual_cleanup_session_completed")
	restarted = updateModel(t, restarted, keyPress("enter"))
	if restarted.current != screenLoadedPlanSummary {
		t.Fatalf("expected back to original plan, got %v", restarted.current)
	}
	if !reflect.DeepEqual(restarted.loadedPlan, plan) {
		t.Fatalf("restored plan differs:\ngot:  %#v\nwant: %#v", restarted.loadedPlan, plan)
	}
	restarted.loadedPlanCursor = 2
	restarted = updateModel(t, restarted, keyPress("enter"))
	if restarted.current != screenLoadedPlanActions || len(restarted.loadedPlan.Actions) != len(plan.Actions) {
		t.Fatalf("View actions did not use original plan: screen=%v actions=%d", restarted.current, len(restarted.loadedPlan.Actions))
	}
	restarted.current = screenLoadedPlanSummary
	restarted.loadedPlanCursor = 1
	restarted = updateModel(t, restarted, keyPress("enter"))
	if restarted.current != screenApplyPreview || restarted.applyPreview.Summary.TotalActions != len(plan.Actions) {
		t.Fatalf("Apply preview did not use original plan: screen=%v preview=%#v", restarted.current, restarted.applyPreview)
	}

	after, _ := json.Marshal(plan)
	if string(before) != string(after) {
		t.Fatal("manual cleanup mutated original plan")
	}
	err = filepath.Walk(w.Dir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), rawPreview) {
			t.Fatalf("raw preview persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("privacy walk: %v", err)
	}
}

func TestManualCleanupRejectsTamperedTargetBeforeOpen(t *testing.T) {
	previousOpen := openExternalURL
	called := 0
	openExternalURL = func(string) error {
		called++
		return nil
	}
	t.Cleanup(func() { openExternalURL = previousOpen })

	m := NewModel()
	m.current = screenManualCleanupAction
	m.manualSession = manualcleanup.Session{
		Manifest: manualcleanup.Manifest{
			ID: "manual-test", PlanID: "plan-test", Mode: manualcleanup.ModeInstagramManual,
			Actions: []manualcleanup.Action{{
				ActionID: "action-1", Type: domain.ActionUnlike,
				TargetURL: "https://instagram.com.evil.example/p/ABC/",
				TargetID:  "ABC", TargetKind: instagram.TargetPost,
			}},
		},
		State: manualcleanup.StateActive, Outcomes: []manualcleanup.Outcome{manualcleanup.OutcomePending},
	}
	_, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected deferred target command")
	}
	next := updateModel(t, m, cmd())
	if called != 0 || !strings.Contains(next.View().Content, "Target unavailable") {
		t.Fatalf("tampered target opened=%d view=\n%s", called, next.View().Content)
	}
}

func TestManualCleanupStartOverRequiresExplicitChoice(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan := manualCleanupTestPlan(now)
	session, _, err := manualcleanup.New("old-progress", plan, manualCleanupTestItems(now, "memory only"), now)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	store := manualcleanup.NewStore(w.Dir())
	if err := store.Start(session); err != nil {
		t.Fatalf("start session: %v", err)
	}
	if _, err := store.Mark(&session, manualcleanup.OutcomeDone, now.Add(time.Minute)); err != nil {
		t.Fatalf("mark session: %v", err)
	}
	if err := store.Stop(&session, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("stop session: %v", err)
	}

	m := NewModelWithWorkspace(w, nil)
	m.planResult.Plan = plan
	m.refreshManualAvailability(plan)
	m.current = screenPlanPreview
	m.openManualCleanup(applySourceGenerated)
	if m.current != screenManualCleanupChoice || m.manualSession.ID != "old-progress" {
		t.Fatalf("expected existing-session choice, got screen=%v id=%q", m.current, m.manualSession.ID)
	}
	m.manualChoiceCursor = 1
	next := updateModel(t, m, keyPress("enter"))
	if next.current != screenManualCleanupAction || next.manualSession.ID != "old-progress" || next.manualSession.CurrentPosition != 0 {
		t.Fatalf("start over failed: screen=%v id=%q position=%d error=%q", next.current, next.manualSession.ID, next.manualSession.CurrentPosition, next.manualError)
	}
	loaded, ok, err := store.Load(plan.ID)
	if err != nil || !ok || loaded.ID != next.manualSession.ID || loaded.CurrentPosition != 0 {
		t.Fatalf("replacement progress=%#v ok=%t err=%v", loaded, ok, err)
	}
}

func TestManualCleanupSameIDPlanMismatchRequiresStartOver(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*domain.CleanupPlan)
	}{
		{
			name: "target",
			mutate: func(plan *domain.CleanupPlan) {
				plan.Actions[0].TargetURL = "https://www.instagram.com/changed_target/"
				plan.Actions[0].TargetID = "changed_target"
			},
		},
		{
			name: "action status",
			mutate: func(plan *domain.CleanupPlan) {
				plan.Actions[1].Status = domain.ActionStatusStopped
			},
		},
		{
			name: "metadata",
			mutate: func(plan *domain.CleanupPlan) {
				plan.Actions[2].Metadata = map[string]string{"batch": "changed"}
			},
		},
		{
			name: "action ordering",
			mutate: func(plan *domain.CleanupPlan) {
				plan.Actions[0], plan.Actions[1] = plan.Actions[1], plan.Actions[0]
			},
		},
		{
			name: "action set",
			mutate: func(plan *domain.CleanupPlan) {
				plan.Actions = append(plan.Actions, domain.CleanupAction{
					ID:                   "new-unsupported",
					Platform:             domain.PlatformInstagram,
					Type:                 domain.ActionDeletePost,
					TargetURL:            "https://www.instagram.com/p/NEWPOST/",
					TargetID:             "NEWPOST",
					SourceActivityItemID: "new-post-item",
					Status:               domain.ActionStatusPending,
					CreatedAt:            now,
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w, err := workspace.Open(t.TempDir())
			if err != nil {
				t.Fatalf("open workspace: %v", err)
			}
			currentPlan := manualCleanupTestPlan(now)
			storedSession, _, err := manualcleanup.New("old-same-id-session", currentPlan, manualCleanupTestItems(now, "old preview"), now)
			if err != nil {
				t.Fatalf("New stored session: %v", err)
			}
			store := manualcleanup.NewStore(w.Dir())
			if err := store.Start(storedSession); err != nil {
				t.Fatalf("Start stored session: %v", err)
			}
			test.mutate(&currentPlan)

			m := NewModelWithWorkspace(w, nil)
			m.planResult.Plan = currentPlan
			m.importResult.Items = manualCleanupTestItems(now, "current preview")
			m.current = screenPlanPreview
			m.openManualCleanup(applySourceGenerated)
			view := m.View().Content
			if m.current != screenManualCleanupChoice || m.manualSessionLoaded || strings.Contains(view, "Resume manual cleanup") || !strings.Contains(view, "Manual cleanup progress could not be loaded.") {
				t.Fatalf("mismatch offered stale resume: screen=%v loaded=%t view=\n%s", m.current, m.manualSessionLoaded, view)
			}

			next := updateModel(t, m, keyPress("enter"))
			if next.current != screenManualCleanupAction || !next.manualSessionLoaded || next.manualSession.ID == storedSession.ID || next.manualError != "" {
				t.Fatalf("Start over did not replace session: screen=%v loaded=%t id=%q error=%q", next.current, next.manualSessionLoaded, next.manualSession.ID, next.manualError)
			}
			loaded, ok, err := store.Load(currentPlan.ID)
			if err != nil || !ok {
				t.Fatalf("Load replacement ok=%t err=%v", ok, err)
			}
			matches, err := manualcleanup.PlansEqual(currentPlan, loaded.OriginalPlan())
			if err != nil || !matches {
				t.Fatalf("replacement snapshot differs: matches=%t err=%v\ngot:  %#v\nwant: %#v", matches, err, loaded.OriginalPlan(), currentPlan)
			}
		})
	}
}

func TestManualCleanupCorruptProgressClearsStaleSession(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	planA := manualCleanupTestPlan(now)
	planA.ID = "plan-a"
	planB := manualCleanupTestPlan(now.Add(time.Hour))
	planB.ID = "plan-b"
	store := manualcleanup.NewStore(w.Dir())
	for _, input := range []struct {
		id   string
		plan domain.CleanupPlan
	}{{"session-a", planA}, {"session-b", planB}} {
		session, _, err := manualcleanup.New(input.id, input.plan, manualCleanupTestItems(now, "preview"), now)
		if err != nil || store.Start(session) != nil {
			t.Fatalf("start %s: %v", input.id, err)
		}
	}
	if err := os.WriteFile(manualProgressPath(t, w, planB.ID), []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("corrupt progress: %v", err)
	}

	m := NewModelWithWorkspace(w, nil)
	if !m.manualSessionLoaded || m.manualSession.PlanID != planA.ID {
		t.Fatalf("expected valid plan A preload, got loaded=%t plan=%q", m.manualSessionLoaded, m.manualSession.PlanID)
	}
	m.manualPreviews = map[string]string{"comment-1": "plan A private preview"}
	m.manualPreviewPlanID = planA.ID
	m.planResult.Plan = planB
	m.current = screenPlanPreview
	m.openManualCleanup(applySourceGenerated)
	if m.current != screenManualCleanupChoice || m.manualSessionLoaded || m.manualSession.ID != "" {
		t.Fatalf("stale session survived corrupt load: screen=%v loaded=%t session=%#v", m.current, m.manualSessionLoaded, m.manualSession)
	}
	view := m.View().Content
	if strings.Contains(view, "Resume manual cleanup") || strings.Contains(view, "plan A private preview") || !strings.Contains(view, "could not be loaded") {
		t.Fatalf("corrupt load state is unsafe:\n%s", view)
	}
}

func TestManualCleanupStartFailureVisibleOnChoiceScreen(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plan := manualCleanupTestPlan(now)
	blockedProgress := filepath.Join(w.Dir(), "manual-cleanup", manualStoreFileName(plan.ID)+".events.jsonl")
	if err := os.MkdirAll(blockedProgress, 0o700); err != nil {
		t.Fatalf("block progress file: %v", err)
	}
	m := NewModelWithWorkspace(w, nil)
	m.planResult.Plan = plan
	m.importResult.Items = manualCleanupTestItems(now, "memory only")
	m.current = screenPlanPreview
	m.openManualCleanup(applySourceGenerated)
	if m.current != screenManualCleanupChoice || m.manualSessionLoaded || !strings.Contains(m.View().Content, "progress could not be saved") {
		t.Fatalf("start failure not visible: screen=%v loaded=%t view=\n%s", m.current, m.manualSessionLoaded, m.View().Content)
	}
}

func TestManualCleanupSwitchesPlansWithoutPreviewLeak(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	planA := manualCleanupTestPlan(now)
	planA.ID = "plan-a"
	planB := manualCleanupTestPlan(now.Add(time.Hour))
	planB.ID = "plan-b"
	store := manualcleanup.NewStore(w.Dir())
	for _, input := range []struct {
		id   string
		plan domain.CleanupPlan
	}{{"session-a", planA}, {"session-b", planB}} {
		session, _, err := manualcleanup.New(input.id, input.plan, manualCleanupTestItems(now, "memory only"), now)
		if err != nil {
			t.Fatalf("New %s: %v", input.id, err)
		}
		if err := store.Start(session); err != nil {
			t.Fatalf("Start %s: %v", input.id, err)
		}
	}

	m := NewModelWithWorkspace(w, nil)
	sessionA, ok, err := store.Load(planA.ID)
	if err != nil || !ok {
		t.Fatalf("load plan A session: ok=%t err=%v", ok, err)
	}
	m.manualSession = sessionA
	m.manualSessionLoaded = true
	m.manualPreviews = map[string]string{"comment-1": "plan A private preview"}
	m.manualPreviewPlanID = planA.ID
	m.planResult.Plan = planB
	m.current = screenPlanPreview
	m.openManualCleanup(applySourceGenerated)
	if !m.manualSessionLoaded || m.manualSession.PlanID != planB.ID || len(m.manualPreviews) != 0 || !strings.Contains(m.View().Content, "Resume manual cleanup") {
		t.Fatalf("plan B session mismatch: loaded=%t plan=%q previews=%#v view=\n%s", m.manualSessionLoaded, m.manualSession.PlanID, m.manualPreviews, m.View().Content)
	}
	m.planResult.Plan = planA
	m.openManualCleanup(applySourceGenerated)
	if !m.manualSessionLoaded || m.manualSession.PlanID != planA.ID || len(m.manualPreviews) != 0 {
		t.Fatalf("plan A session mismatch: loaded=%t plan=%q previews=%#v", m.manualSessionLoaded, m.manualSession.PlanID, m.manualPreviews)
	}
}

func manualCleanupTestPlan(now time.Time) domain.CleanupPlan {
	actions := []domain.CleanupAction{
		{ID: "unfollow-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnfollow, TargetURL: "https://www.instagram.com/demo_following/", TargetID: "demo_following", SourceActivityItemID: "follow-item", Status: domain.ActionStatusPending, CreatedAt: now},
		{ID: "unlike-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnlike, TargetURL: "https://www.instagram.com/reel/REEL1/", TargetID: "REEL1", SourceActivityItemID: "like-item", Status: domain.ActionStatusPending, CreatedAt: now},
		{ID: "comment-1", Platform: domain.PlatformInstagram, Type: domain.ActionDeleteComment, TargetURL: "https://www.instagram.com/p/POST1/", TargetID: "POST1", SourceActivityItemID: "comment-item", Status: domain.ActionStatusPending, CreatedAt: now},
		{ID: "unsupported-1", Platform: domain.PlatformInstagram, Type: domain.ActionDeletePost, TargetURL: "https://www.instagram.com/p/POST2/", TargetID: "POST2", SourceActivityItemID: "post-item", Status: domain.ActionStatusPending, CreatedAt: now},
	}
	return domain.NewCleanupPlan("instagram-plan:test", domain.PlatformInstagram, "synthetic export", now, actions)
}

func manualCleanupTestItems(now time.Time, preview string) []domain.ActivityItem {
	return []domain.ActivityItem{
		{ID: "follow-item", Platform: domain.PlatformInstagram, Type: domain.ItemTypeFollow, TargetURL: "https://www.instagram.com/demo_following/", TargetID: "demo_following", Actor: "demo_following", Metadata: map[string]string{"relationship": "following"}},
		{ID: "like-item", Platform: domain.PlatformInstagram, Type: domain.ItemTypeLike, TargetURL: "https://www.instagram.com/reel/REEL1/", TargetID: "REEL1", Actor: "reel_owner", OccurredAt: &now},
		{ID: "comment-item", Platform: domain.PlatformInstagram, Type: domain.ItemTypeComment, TargetURL: "https://www.instagram.com/p/POST1/", TargetID: "POST1", Actor: "post_owner", OccurredAt: &now, Text: &domain.SafeTextReference{Hash: "sha256:abc", Preview: preview}},
	}
}

func manualProgressPath(t *testing.T, w *workspace.Workspace, planID string) string {
	t.Helper()
	manifests, err := filepath.Glob(filepath.Join(w.Dir(), "manual-cleanup", "*.json"))
	if err != nil {
		t.Fatalf("glob manifests: %v", err)
	}
	for _, path := range manifests {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		var manifest manualcleanup.Manifest
		if json.Unmarshal(content, &manifest) == nil && manifest.PlanID == planID {
			return strings.TrimSuffix(path, ".json") + ".events.jsonl"
		}
	}
	t.Fatalf("manifest for %q not found", planID)
	return ""
}

func manualStoreFileName(planID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(planID)))
	return hex.EncodeToString(sum[:16])
}

func TestLoadedRedditApplyPreviewRequiresConnectedAccount(t *testing.T) {
	next := NewModel()
	next.loadedPlan = redditApplyTestPlan()
	next.loadedPlanSummary = domain.SummarizeCleanupPlan(next.loadedPlan)
	next.current = screenLoadedPlanSummary

	next = updateModel(t, next, keyPress("enter"))

	if next.current != screenApplyPreview || next.applyPreview.CanApply {
		t.Fatalf("expected blocked apply preview, screen=%v preview=%#v", next.current, next.applyPreview)
	}
	view := next.View().Content
	for _, want := range []string{
		"Status: Blocked",
		"Provider ready: not ready",
		"Connect Reddit before simulating this plan.",
		"Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected blocked reddit preview to contain %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Simulate no-op run") {
		t.Fatalf("blocked preview should not offer run action, got:\n%s", view)
	}

	next.localConfig.Reddit = &workspace.RedditConfig{Username: "test_user"}
	next.openApplyPreview(applySourceLoaded)
	if !next.applyPreview.CanApply || !next.applyPreview.ProviderReady {
		t.Fatalf("expected connected reddit preview to be ready: %#v", next.applyPreview)
	}
	if !strings.Contains(next.View().Content, "Provider ready: ready") {
		t.Fatalf("expected ready provider label, got:\n%s", next.View().Content)
	}
}

func TestUnknownProviderShowsBlockedApplyPreview(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	next := NewModel()
	next.loadedPlan = domain.NewCleanupPlan("unknown-plan", "unknown", "test", now, []domain.CleanupAction{{
		ID: "action-1", Platform: "unknown", Type: domain.ActionUnlike, TargetID: "target-1", SourceActivityItemID: "item-1", Status: domain.ActionStatusPending, CreatedAt: now,
	}})
	next.loadedPlanSummary = domain.SummarizeCleanupPlan(next.loadedPlan)
	next.current = screenLoadedPlanSummary

	next = updateModel(t, next, keyPress("enter"))
	view := next.View().Content
	for _, want := range []string{"Status: Blocked", "Executor: -", "This plan's platform is unavailable for simulation.", "Back"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected unknown provider preview to contain %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Simulate no-op run") {
		t.Fatalf("unknown provider preview offered execution: %s", view)
	}
}

func TestPlanExportPathDefaultsAndWritesReadableJSON(t *testing.T) {
	next := planPreviewModel(t)
	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))

	if next.current != screenPlanExportPath {
		t.Fatalf("expected export path screen, got %v", next.current)
	}
	if next.planPathInput.Value() != defaultPlanExportPath {
		t.Fatalf("expected default export path %q, got %q", defaultPlanExportPath, next.planPathInput.Value())
	}

	outputPath := filepath.Join(t.TempDir(), "plan.json")
	next.planPathInput.SetValue(outputPath)
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected export command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	view := next.View().Content
	if next.planExportStatus != "Saved plan to "+outputPath || !strings.Contains(view, "Saved plan to") {
		t.Fatalf("expected export success message, status=%q view:\n%s", next.planExportStatus, view)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("expected exported plan JSON: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{
		"\n  \"format_version\"",
		"\"mode\": \"dry-run\"",
		"\"type\": \"unlike\"",
		"\"type\": \"delete_comment\"",
		"\"type\": \"unfollow\"",
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("expected exported JSON to contain %q, got:\n%s", want, jsonText)
		}
	}
	if strings.Contains(jsonText, "raw private comment") {
		t.Fatalf("expected raw private text not to be exported, got:\n%s", jsonText)
	}
}

func TestWorkspaceHistoryAndAuditHooks(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "instagram-export.zip")
	next := NewModelWithWorkspace(w, nil)
	result := fakeImportResultWithRelationships()
	result.Summary.Skipped = 2
	result.Warnings.Total = 5
	result.Warnings.Groups[0].Count = 5
	next = updateModel(t, next, importFinishedMsg{result: result, source: sourcePath})

	imports, err := w.RecentImports()
	if err != nil {
		t.Fatalf("read recent imports: %v", err)
	}
	if len(imports) != 1 {
		t.Fatalf("recent imports = %d, want 1", len(imports))
	}
	if imports[0].SourcePath != filepath.Clean(sourcePath) || imports[0].ItemCount != 4 || imports[0].LikeCount != 1 || imports[0].CommentCount != 1 || imports[0].PostCount != 0 || imports[0].FollowingCount != 1 || imports[0].FollowerCount != 1 || imports[0].WarningCount != 5 || imports[0].SkippedCount != 2 {
		t.Fatalf("unexpected recent import: %#v", imports[0])
	}
	importEvent := requireAuditEvent(t, w, "import_completed")
	if importEvent.Fields["warning_count"] != float64(5) || importEvent.Fields["skipped_count"] != float64(2) {
		t.Fatalf("expected distinct warning and skipped audit counts, got %#v", importEvent.Fields)
	}

	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, keyPress("a"))
	next = updateModel(t, next, keyPress("s"))
	next = updateModel(t, next, keyPress("enter"))
	if next.current != screenPlanPreview {
		t.Fatalf("expected plan preview, got %v", next.current)
	}
	requireAuditEvent(t, w, "plan_generated")

	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	outputPath := filepath.Join(t.TempDir(), "exported-plan.json")
	next.planPathInput.SetValue(outputPath)
	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected export command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())
	plans, err := w.RecentPlans()
	if err != nil {
		t.Fatalf("read recent plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Path != outputPath || plans[0].ID != next.planResult.Plan.ID {
		t.Fatalf("unexpected recent plans after export: %#v", plans)
	}
	if !plans[0].PlanCreatedAt.Equal(next.planResult.Plan.CreatedAt) || plans[0].LastUsedAt.IsZero() || plans[0].LastOperation != "exported" {
		t.Fatalf("unexpected recent plan usage fields after export: %#v", plans[0])
	}
	config, err := w.LoadConfig()
	if err != nil {
		t.Fatalf("load config after export: %v", err)
	}
	if config.DefaultPlanExportPath != outputPath {
		t.Fatalf("default plan export path = %q, want %q", config.DefaultPlanExportPath, outputPath)
	}
	requireAuditEvent(t, w, "plan_exported")

	reloaded := NewModelWithWorkspace(w, nil)
	reloaded = openPlanLoadPath(t, reloaded)
	reloaded.planPathInput.SetValue(outputPath)
	updated, cmd = reloaded.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected load command")
	}
	reloaded = requireModel(t, updated)
	reloaded = updateModel(t, reloaded, cmd())
	if reloaded.current != screenLoadedPlanSummary {
		t.Fatalf("expected loaded summary after workspace load, got %v", reloaded.current)
	}
	plans, err = w.RecentPlans()
	if err != nil {
		t.Fatalf("read recent plans after load: %v", err)
	}
	if len(plans) != 1 || plans[0].LastOperation != "loaded" || plans[0].LastUsedAt.IsZero() {
		t.Fatalf("unexpected recent plan usage fields after load: %#v", plans)
	}
	config, err = w.LoadConfig()
	if err != nil {
		t.Fatalf("load config after plan load: %v", err)
	}
	if config.LastOpenedPlanPath != outputPath {
		t.Fatalf("last opened plan path = %q, want %q", config.LastOpenedPlanPath, outputPath)
	}
	requireAuditEvent(t, w, "plan_loaded")
}

func TestWorkspaceMetadataWriteFailureIsNonFatalWarning(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(w.Dir(), "recent-imports.json"), []byte(`{"bad":`), 0o600); err != nil {
		t.Fatalf("corrupt recent imports: %v", err)
	}

	next := NewModelWithWorkspace(w, nil)
	next = updateModel(t, next, importFinishedMsg{result: fakeImportResult(), source: "demo instagram export"})
	if next.current != screenImportResult {
		t.Fatalf("expected import result despite metadata failure, got %v", next.current)
	}
	if !strings.Contains(next.localDataWarning, "Local data warning") {
		t.Fatalf("expected nonfatal local data warning, got %q", next.localDataWarning)
	}
	if !strings.Contains(next.View().Content, "Local data warning") {
		t.Fatalf("expected warning to be visible, got:\n%s", next.View().Content)
	}
}

func TestPlanExportConfigWriteFailureIsNonFatalWarning(t *testing.T) {
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	next := NewModelWithWorkspace(w, nil)
	next = updateModel(t, next, importFinishedMsg{result: fakeImportResultWithRelationships(), source: "demo instagram export"})
	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, keyPress("a"))
	next = updateModel(t, next, keyPress("s"))
	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("down"))
	next = updateModel(t, next, keyPress("enter"))
	outputPath := filepath.Join(t.TempDir(), "exported-plan.json")
	next.planPathInput.SetValue(outputPath)
	if err := os.WriteFile(filepath.Join(w.Dir(), "config.json"), []byte(`{"version":`), 0o600); err != nil {
		t.Fatalf("corrupt config: %v", err)
	}

	updated, cmd := next.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatalf("expected export command")
	}
	next = requireModel(t, updated)
	next = updateModel(t, next, cmd())

	if next.planExportStatus != "Saved plan to "+outputPath {
		t.Fatalf("expected successful export despite config failure, status=%q", next.planExportStatus)
	}
	if !strings.Contains(next.localDataWarning, "Local data warning") {
		t.Fatalf("expected nonfatal local data warning, got %q", next.localDataWarning)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected exported plan file: %v", err)
	}
}

func TestFiltersScreenOpensFromItemsBrowser(t *testing.T) {
	m := importedModel(t, fakeImportResult())
	next := updateModel(t, m, keyPress("enter"))

	next = updateModel(t, next, keyPress("f"))
	view := next.View().Content

	if next.current != screenFilters {
		t.Fatalf("expected filters screen, got %v", next.current)
	}
	for _, want := range []string{"Filters", "[ ] Like", "[ ] Post", "Actor contains", "Apply filters", "Clear all filters"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected filters view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestBackspaceEditsFocusedFilterInput(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("f"))
	for range filterRowActor {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	for _, keyName := range []string{"d", "e", "m", "o"} {
		next = updateModel(t, next, keyPress(keyName))
	}

	next = updateModel(t, next, keyPress("backspace"))
	if next.current != screenFilters || next.filterEditing != filterRowActor {
		t.Fatalf("expected backspace to stay editing filter input, screen=%v editing=%d", next.current, next.filterEditing)
	}
	if next.filterActorInput.Value() != "dem" {
		t.Fatalf("expected backspace to edit filter input, got %q", next.filterActorInput.Value())
	}
}

func TestApplyingTypeFilterUpdatesItemsBrowser(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("f"))

	next = updateModel(t, next, keyPress("enter"))
	for range filterRowApply {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))

	view := next.View().Content
	if next.current != screenItemsBrowser {
		t.Fatalf("expected items browser, got %v", next.current)
	}
	for _, want := range []string{
		"Showing 1-1 of 1 · Matching 1/4 · Selected 0 · Filters active",
		"Filters active",
		"[ ] like",
		"demo_artist",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected filtered items view to contain %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "[ ] comment") || strings.Contains(view, "demo_bakery") {
		t.Fatalf("expected comment to be filtered out, got:\n%s", view)
	}
}

func TestInvalidFilterDateShowsFriendlyErrorAndDoesNotApply(t *testing.T) {
	m := importedModel(t, fakeImportResultWithRelationships())
	next := updateModel(t, m, keyPress("enter"))
	next = updateModel(t, next, keyPress("f"))

	for range filterRowOlder {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	for _, keyName := range []string{"n", "o", "t", "-", "a", "-", "d", "a", "t", "e"} {
		next = updateModel(t, next, keyPress(keyName))
	}
	next = updateModel(t, next, keyPress("enter"))
	for range filterRowApply - filterRowOlder {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))

	view := next.View().Content
	if next.current != screenFilters {
		t.Fatalf("expected filters screen after invalid date, got %v", next.current)
	}
	if next.itemFilter.Active() {
		t.Fatalf("expected invalid date not to apply filter")
	}
	if !strings.Contains(view, "Older than date must use YYYY-MM-DD.") {
		t.Fatalf("expected friendly date error, got:\n%s", view)
	}

	next = updateModel(t, next, keyPress("esc"))
	if next.current != screenItemsBrowser {
		t.Fatalf("expected esc to return to items browser, got %v", next.current)
	}
}

func TestWarningsViewShowsEmptyState(t *testing.T) {
	result := fakeImportResult()
	result.Warnings = instagram.ImportWarningSummary{}
	m := importedModel(t, result)
	m.resultCursor = resultViewWarnings

	next := updateModel(t, m, keyPress("enter"))
	view := next.View().Content
	if !strings.Contains(view, "No warnings.") {
		t.Fatalf("expected empty warnings state, got:\n%s", view)
	}
}

func TestImportResultViewShowsFailure(t *testing.T) {
	m := NewModel()

	updated, _ := m.Update(importFinishedMsg{
		err:    errors.New("open instagram export zip: not found"),
		source: "missing.zip",
	})

	next := requireModel(t, updated)
	view := next.View().Content
	if !strings.Contains(view, "Import Failed") || !strings.Contains(view, "not found") {
		t.Fatalf("expected failure view, got:\n%s", view)
	}
}

func TestMajorScreensRenderAtSmallAndWideSizes(t *testing.T) {
	imported := importedModel(t, fakeImportResultWithRelationships())
	items := updateModel(t, imported, keyPress("enter"))
	filters := updateModel(t, items, keyPress("f"))
	selected := updateModel(t, updateModel(t, items, keyPress("a")), keyPress("s"))
	selected.selectionCursor = selectionViewSelected
	selectedItems := updateModel(t, selected, keyPress("enter"))
	preview := planPreviewModel(t)
	applyPreview := updateModel(t, preview, keyPress("enter"))
	applyConfirm := updateModel(t, applyPreview, keyPress("enter"))
	applyRunning := applyConfirm
	applyRunning.current = screenApplyRunning
	applyResult := applyPreview
	applyResult.current = screenApplyResult
	applyResult.applyExecution = apply.Execution{
		State:  apply.ExecutionStateDone,
		Counts: apply.ResultCounts{Done: 3},
		Results: []apply.ActionResult{
			{ActionID: "action-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnlike, Status: domain.ActionStatusDone},
		},
	}
	exportPath := updateModel(t, updateModel(t, preview, keyPress("down")), keyPress("enter"))
	plan := fakeCleanupPlan()
	loadedSummary := NewModel()
	loadedSummary.current = screenLoadedPlanSummary
	loadedSummary.loadedPlan = plan
	loadedSummary.loadedPlanSummary = domain.SummarizeCleanupPlan(plan)
	loadedActions := loadedSummary
	loadedActions.current = screenLoadedPlanActions
	warnings := imported
	warnings.current = screenWarnings
	reviewEmpty := NewModel()
	reviewEmpty.current = screenReviewEmpty
	localData := NewModel()
	localData.current = screenLocalDataOverview
	recentImports := localData
	recentImports.current = screenRecentImports
	recentPlans := localData
	recentPlans.current = screenRecentPlans
	auditLog := localData
	auditLog.current = screenAuditLog
	wipeConfirm := localData
	wipeConfirm.current = screenWipeLocalDataConfirm
	wipeConfirm.wipeLocalDataCursor = wipeLocalDataCancel
	importing := NewModel()
	importing.current = screenImporting
	importing.importSource = "demo instagram export"
	redditConnect := NewModel()
	redditConnect.current = screenRedditConnect
	redditConnect.selectedPlatformID = platform.PlatformReddit
	redditAuth := redditConnect
	redditAuth.current = screenRedditSigningIn
	redditAuth.redditAuthURL = "https://www.reddit.com/api/v1/authorize?client_id=test"
	redditBusy := redditConnect
	redditBusy.current = screenRedditBusy
	redditBusy.redditBusyTitle = "Scanning Reddit"
	redditBusy.redditBusyDetail = "Reading own comments and submitted posts."

	cases := []struct {
		name  string
		model Model
	}{
		{name: "home", model: NewModel()},
		{name: "help", model: updateModel(t, NewModel(), keyPress("?"))},
		{name: "quit", model: updateModel(t, NewModel(), keyPress("ctrl+q"))},
		{name: "import path", model: clickTab(t, NewModel(), "Import")},
		{name: "importing", model: importing},
		{name: "import result", model: imported},
		{name: "reddit connect", model: redditConnect},
		{name: "reddit auth", model: redditAuth},
		{name: "reddit busy", model: redditBusy},
		{name: "items", model: items},
		{name: "review empty", model: reviewEmpty},
		{name: "filters", model: filters},
		{name: "selection summary", model: selected},
		{name: "selected items", model: selectedItems},
		{name: "plan preview", model: preview},
		{name: "apply preview", model: applyPreview},
		{name: "apply confirm", model: applyConfirm},
		{name: "apply running", model: applyRunning},
		{name: "apply result", model: applyResult},
		{name: "plan export", model: exportPath},
		{name: "plan load", model: func() Model {
			return openPlanLoadPath(t, NewModel())
		}()},
		{name: "loaded summary", model: loadedSummary},
		{name: "loaded actions", model: loadedActions},
		{name: "warnings", model: warnings},
		{name: "local data", model: localData},
		{name: "recent imports", model: recentImports},
		{name: "recent plans", model: recentPlans},
		{name: "audit log", model: auditLog},
		{name: "wipe confirm", model: wipeConfirm},
	}
	sizes := []tea.WindowSizeMsg{
		{Width: 24, Height: 8},
		{Width: 132, Height: 36},
	}

	for _, size := range sizes {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				next := updateModel(t, tc.model, size)
				view := next.View().Content
				if strings.TrimSpace(view) == "" {
					t.Fatalf("expected non-empty view at %dx%d", size.Width, size.Height)
				}
				if !strings.Contains(view, "Vanish") {
					t.Fatalf("expected shell header at %dx%d, got:\n%s", size.Width, size.Height, view)
				}
			})
		}
	}
}

func TestScreensUseTerminalHeight(t *testing.T) {
	platformDetail := NewModel()
	platformDetail.openPlatformDetail(0)
	instagramGuide := platformDetail
	instagramGuide.platformActionCursor = platformActionIndex(t, instagramGuide.selectedPlatform(), platform.ActionRequestInstagramExport)
	instagramGuide = updateModel(t, instagramGuide, keyPress("enter"))
	redditNotes := NewModel()
	redditNotes.current = screenRedditNotes
	redditNotes.selectedPlatformID = platform.PlatformReddit

	imported := importedModel(t, fakeImportResultWithRelationships())
	items := updateModel(t, imported, keyPress("enter"))
	filters := updateModel(t, items, keyPress("f"))
	selected := updateModel(t, updateModel(t, items, keyPress("a")), keyPress("s"))
	selected.selectionCursor = selectionViewSelected
	selectedItems := updateModel(t, selected, keyPress("enter"))
	preview := planPreviewModel(t)
	planExport := updateModel(t, updateModel(t, preview, keyPress("down")), keyPress("enter"))
	applyPreview := updateModel(t, preview, keyPress("enter"))
	applyConfirm := updateModel(t, applyPreview, keyPress("enter"))
	applyRunning := applyConfirm
	applyRunning.current = screenApplyRunning
	applyResult := applyPreview
	applyResult.current = screenApplyResult
	applyResult.applyExecution = apply.Execution{
		State:  apply.ExecutionStateDone,
		Counts: apply.ResultCounts{Done: 3},
		Results: []apply.ActionResult{
			{ActionID: "action-1", Platform: domain.PlatformInstagram, Type: domain.ActionUnlike, Status: domain.ActionStatusDone},
		},
	}
	plan := fakeCleanupPlan()
	loadedSummary := NewModel()
	loadedSummary.current = screenLoadedPlanSummary
	loadedSummary.loadedPlan = plan
	loadedSummary.loadedPlanSummary = domain.SummarizeCleanupPlan(plan)
	loadedActions := loadedSummary
	loadedActions.current = screenLoadedPlanActions
	localData := NewModel()
	localData.current = screenLocalDataOverview
	recentImports := localData
	recentImports.current = screenRecentImports
	recentPlans := localData
	recentPlans.current = screenRecentPlans
	auditLog := localData
	auditLog.current = screenAuditLog
	wipeConfirm := localData
	wipeConfirm.current = screenWipeLocalDataConfirm
	wipeConfirm.wipeLocalDataCursor = wipeLocalDataCancel
	importing := NewModel()
	importing.current = screenImporting
	importing.importSource = "demo instagram export"
	importFailure := NewModel()
	updated, _ := importFailure.Update(importFinishedMsg{
		err:    errors.New("open instagram export zip: not found"),
		source: "missing.zip",
	})
	importFailure = requireModel(t, updated)
	warnings := imported
	warnings.current = screenWarnings
	reviewEmpty := NewModel()
	reviewEmpty.current = screenReviewEmpty
	t.Setenv(reddit.ClientIDEnv, "")
	redditConnect := NewModel()
	redditConnect.current = screenRedditConnect
	redditConnect.selectedPlatformID = platform.PlatformReddit
	redditAuth := redditConnect
	redditAuth.current = screenRedditSigningIn
	redditAuth.redditAuthURL = "https://www.reddit.com/api/v1/authorize?client_id=test"
	redditBusy := redditConnect
	redditBusy.current = screenRedditBusy
	redditBusy.redditBusyTitle = "Scanning Reddit"
	redditBusy.redditBusyDetail = "Reading own comments and submitted posts."
	quitConfirm := updateModel(t, NewModel(), keyPress("ctrl+q"))

	cases := []struct {
		name  string
		model Model
	}{
		{name: "home", model: NewModel()},
		{name: "platform detail", model: platformDetail},
		{name: "instagram guide", model: instagramGuide},
		{name: "reddit notes", model: redditNotes},
		{name: "help", model: updateModel(t, NewModel(), keyPress("?"))},
		{name: "import zip", model: clickTab(t, NewModel(), "Import")},
		{name: "importing", model: importing},
		{name: "import failure", model: importFailure},
		{name: "parsed items", model: items},
		{name: "review empty", model: reviewEmpty},
		{name: "filters", model: filters},
		{name: "selection summary", model: selected},
		{name: "selected items", model: selectedItems},
		{name: "plan preview", model: preview},
		{name: "plan export", model: planExport},
		{name: "plan load", model: openPlanLoadPath(t, NewModel())},
		{name: "loaded plan", model: loadedSummary},
		{name: "loaded actions", model: loadedActions},
		{name: "apply preview", model: applyPreview},
		{name: "apply confirm", model: applyConfirm},
		{name: "apply running", model: applyRunning},
		{name: "apply result", model: applyResult},
		{name: "warnings", model: warnings},
		{name: "local data", model: localData},
		{name: "recent imports", model: recentImports},
		{name: "recent plans", model: recentPlans},
		{name: "audit log", model: auditLog},
		{name: "wipe confirm", model: wipeConfirm},
		{name: "reddit setup", model: redditConnect},
		{name: "reddit auth", model: redditAuth},
		{name: "reddit busy", model: redditBusy},
		{name: "quit confirm", model: quitConfirm},
	}

	for _, size := range []tea.WindowSizeMsg{
		{Width: 132, Height: 36},
		{Width: 160, Height: 60},
	} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s_%dx%d", tc.name, size.Width, size.Height), func(t *testing.T) {
				next := updateModel(t, tc.model, size)
				view := next.View().Content
				lines := strings.Count(view, "\n") + 1
				if lines < size.Height-1 || lines > size.Height+1 {
					t.Fatalf("expected screen to match terminal height, got %d lines at %dx%d:\n%s", lines, size.Width, size.Height, view)
				}
				if strings.Count(stripANSI(view), "└") < 1 {
					t.Fatalf("expected screen to render full pane frame, got:\n%s", view)
				}
			})
		}
	}
}

func fakeImportResult() instagram.ImportResult {
	occurred := time.Date(2024, 3, 9, 16, 0, 0, 0, time.UTC)
	return instagram.ImportResult{
		Items: []domain.ActivityItem{
			{
				ID:         "item-like",
				Platform:   domain.PlatformInstagram,
				Type:       domain.ItemTypeLike,
				Actor:      "demo_artist",
				TargetURL:  "https://www.instagram.com/p/demo_like/",
				TargetID:   "demo_artist",
				OccurredAt: &occurred,
				Source:     domain.SourceMetadata{FileName: "liked_posts.json"},
				Metadata:   map[string]string{"instagram_kind": "liked_post", "username": "demo_artist"},
				Text:       &domain.SafeTextReference{Hash: "sha256:abcdef", Preview: "raw private comment"},
			},
			{
				ID:       "item-comment",
				Platform: domain.PlatformInstagram,
				Type:     domain.ItemTypeComment,
				Actor:    "demo_bakery",
				TargetID: "demo_bakery",
				Source:   domain.SourceMetadata{FileName: "post_comments_1.json"},
				Metadata: map[string]string{"instagram_kind": "comment", "media_owner": "demo_bakery"},
				Text:     &domain.SafeTextReference{Hash: "sha256:123456"},
			},
		},
		Summary: instagram.ImportSummary{
			Total:    2,
			Likes:    1,
			Comments: 1,
			Skipped:  1,
		},
		Warnings: instagram.ImportWarningSummary{
			Total: 1,
			Groups: []instagram.ImportWarningGroup{{
				SourceFile: "settings/unknown_shape.json",
				Category:   "instagram-json",
				Reason:     "unsupported activity file",
				Unit:       instagram.WarningUnitFile,
				Count:      1,
			}},
		},
	}
}

func fakeImportResultWithRelationships() instagram.ImportResult {
	result := fakeImportResult()
	followingOccurred := time.Date(2024, 3, 10, 16, 0, 0, 0, time.UTC)
	followerOccurred := time.Date(2024, 3, 11, 16, 0, 0, 0, time.UTC)
	result.Items = append(result.Items,
		domain.ActivityItem{
			ID:         "item-following",
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeFollow,
			Actor:      "demo_following",
			TargetID:   "demo_following",
			TargetURL:  "https://www.instagram.com/demo_following/",
			OccurredAt: &followingOccurred,
			Metadata:   map[string]string{"relationship": "following"},
			Source:     domain.SourceMetadata{FileName: "following.json"},
		},
		domain.ActivityItem{
			ID:         "item-follower",
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeFollow,
			Actor:      "demo_follower",
			TargetID:   "demo_follower",
			TargetURL:  "https://www.instagram.com/demo_follower/",
			OccurredAt: &followerOccurred,
			Metadata:   map[string]string{"relationship": "follower"},
			Source:     domain.SourceMetadata{FileName: "followers_1.json"},
		},
	)
	result.Summary.Total = len(result.Items)
	result.Summary.Following = 1
	result.Summary.Followers = 1
	return result
}

func fakeImportResultWithManyItems(count int) instagram.ImportResult {
	result := fakeImportResult()
	result.Items = nil
	for i := 0; i < count; i++ {
		occurred := time.Date(2024, 3, 1+i, 16, 0, 0, 0, time.UTC)
		result.Items = append(result.Items, domain.ActivityItem{
			ID:         fmt.Sprintf("item-%02d", i),
			Platform:   domain.PlatformInstagram,
			Type:       domain.ItemTypeLike,
			Actor:      fmt.Sprintf("demo_%02d", i),
			TargetURL:  fmt.Sprintf("https://www.instagram.com/p/demo_%02d/", i),
			TargetID:   fmt.Sprintf("demo_%02d", i),
			OccurredAt: &occurred,
			Source:     domain.SourceMetadata{FileName: "liked_posts.json"},
		})
	}
	result.Summary.Total = len(result.Items)
	result.Summary.Likes = len(result.Items)
	result.Summary.Comments = 0
	result.Summary.Following = 0
	result.Summary.Followers = 0
	return result
}

func importedModel(t *testing.T, result instagram.ImportResult) Model {
	t.Helper()

	updated, cmd := NewModel().Update(importFinishedMsg{result: result, source: "demo instagram export"})
	if cmd != nil {
		t.Fatalf("expected import result message not to return command")
	}
	return requireModel(t, updated)
}

func openLocalDataOverview(t *testing.T, model Model) Model {
	t.Helper()

	next := clickTab(t, model, "Local")
	if next.current != screenLocalDataOverview {
		t.Fatalf("expected local data overview, got %v", next.current)
	}
	return next
}

func openPlanLoadPath(t *testing.T, model Model) Model {
	t.Helper()

	next := clickTab(t, model, "Plans")
	if next.current != screenPlanLoadPath {
		t.Fatalf("expected plan load path, got %v", next.current)
	}
	return next
}

func planPreviewModel(t *testing.T) Model {
	t.Helper()

	next := importedModel(t, fakeImportResultWithRelationships())
	next = updateModel(t, next, keyPress("enter"))
	next = updateModel(t, next, keyPress("a"))
	next = updateModel(t, next, keyPress("s"))
	return updateModel(t, next, keyPress("enter"))
}

func fakeCleanupPlan() domain.CleanupPlan {
	createdAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	actions := []domain.CleanupAction{
		{
			ID:                   "action-1",
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionUnlike,
			TargetURL:            "https://instagram.example/p/1",
			TargetID:             "target-1",
			SourceActivityItemID: "item-1",
			Status:               domain.ActionStatusPending,
			CreatedAt:            createdAt,
		},
		{
			ID:                   "action-2",
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionDeleteComment,
			TargetID:             "comment-1",
			SourceActivityItemID: "item-2",
			Status:               domain.ActionStatusDone,
			CreatedAt:            createdAt.Add(time.Minute),
		},
		{
			ID:                   "action-3",
			Platform:             domain.PlatformInstagram,
			Type:                 domain.ActionUnfollow,
			TargetURL:            "https://instagram.example/demo",
			SourceActivityItemID: "item-3",
			Status:               domain.ActionStatusSkipped,
			CreatedAt:            createdAt.Add(2 * time.Minute),
		},
	}
	return domain.NewCleanupPlan("plan-loaded", domain.PlatformInstagram, "instagram-export", createdAt, actions)
}

func redditApplyTestPlan() domain.CleanupPlan {
	createdAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	return domain.NewCleanupPlan("reddit-plan-loaded", domain.PlatformReddit, "reddit-api", createdAt, []domain.CleanupAction{
		{
			ID:                   "reddit-action-1",
			Platform:             domain.PlatformReddit,
			Type:                 domain.ActionRedditDeleteComment,
			TargetURL:            "https://www.reddit.com/r/test/comments/p/comment/c1/",
			TargetID:             "t1_c1",
			SourceActivityItemID: "reddit-comment-1",
			Status:               domain.ActionStatusPending,
			CreatedAt:            createdAt,
		},
	})
}

func writeTUIPlan(t *testing.T, plan domain.CleanupPlan) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "vanish-plan.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	defer file.Close()

	if err := domain.WritePlanJSON(file, plan); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return path
}

func requireAuditEvent(t *testing.T, w *workspace.Workspace, eventType string) workspace.AuditEvent {
	t.Helper()

	audit, err := w.ReadAudit()
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	for _, event := range audit.Events {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("expected audit event %q, got %#v", eventType, audit.Events)
	return workspace.AuditEvent{}
}

func platformActionIndex(t *testing.T, current platform.Platform, actionID string) int {
	t.Helper()

	for i, action := range current.Actions {
		if action.ID == actionID {
			return i
		}
	}
	t.Fatalf("expected platform %q to have action %q", current.ID, actionID)
	return 0
}

func installRedditPlatformForTest(t *testing.T, mutate func(*platform.Platform)) {
	t.Helper()
	previous := builtInPlatforms
	current := reddit.Platform()
	mutate(&current)
	registry, err := platform.NewRegistry(instagram.Platform(), current)
	if err != nil {
		t.Fatal(err)
	}
	builtInPlatforms = registry
	t.Cleanup(func() {
		builtInPlatforms = previous
	})
}

func applyTypeFilter(t *testing.T, model Model, row int) Model {
	t.Helper()

	next := model
	if next.current != screenFilters {
		next = updateModel(t, next, keyPress("f"))
	}
	for next.filterCursor < row {
		next = updateModel(t, next, keyPress("down"))
	}
	for next.filterCursor > row {
		next = updateModel(t, next, keyPress("up"))
	}
	next = updateModel(t, next, keyPress("enter"))
	for next.filterCursor < filterRowApply {
		next = updateModel(t, next, keyPress("down"))
	}
	return updateModel(t, next, keyPress("enter"))
}

func clearFilters(t *testing.T, model Model) Model {
	t.Helper()

	next := model
	if next.current != screenFilters {
		next = updateModel(t, next, keyPress("f"))
	}
	for next.filterCursor < filterRowClear {
		next = updateModel(t, next, keyPress("down"))
	}
	next = updateModel(t, next, keyPress("enter"))
	if next.current == screenFilters {
		next = updateModel(t, next, keyPress("esc"))
	}
	return next
}

func updateModel(t *testing.T, model Model, msg tea.Msg) Model {
	t.Helper()

	updated, _ := model.Update(msg)
	return requireModel(t, updated)
}

func keyPress(text string) tea.KeyPressMsg {
	switch text {
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "left":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	case "backspace":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})
	case "tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
	case "pgup":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp})
	case "pgdown":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown})
	case " ":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})
	case "ctrl+c":
		return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	case "ctrl+q":
		return tea.KeyPressMsg(tea.Key{Code: 'q', Mod: tea.ModCtrl})
	}

	key := tea.Key{Text: text}
	if len(text) == 1 {
		key.Code = []rune(strings.ToLower(text))[0]
		key.ShiftedCode = []rune(text)[0]
	}

	return tea.KeyPressMsg(key)
}

func mouseClick(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func mouseWheel(y int, button tea.MouseButton) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: 4, Y: y, Button: button}
}

func mouseMotion(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y}
}

func clickTab(t *testing.T, model Model, label string) Model {
	t.Helper()

	box := requireHitBox(t, hitBoxesForTest(t, model), hitTab, -1, label)
	return updateModel(t, model, mouseClick(box.X, box.Y))
}

func hitBoxesForTest(t *testing.T, model Model) []hitBox {
	t.Helper()

	_, boxes := model.renderView()
	if len(boxes) == 0 {
		t.Fatalf("expected rendered view to expose hit boxes")
	}
	return boxes
}

func requireHitBox(t *testing.T, boxes []hitBox, kind hitKind, index int, label string) hitBox {
	t.Helper()

	for _, box := range boxes {
		if box.Target.Kind != kind {
			continue
		}
		if label != "" {
			if box.Target.Label == label {
				return box
			}
			continue
		}
		if box.Target.Index == index {
			return box
		}
	}
	t.Fatalf("expected hit box kind=%d index=%d label=%q in %#v", kind, index, label, boxes)
	return hitBox{}
}

func hitBoxesOfKind(boxes []hitBox, kind hitKind) []hitBox {
	filtered := make([]hitBox, 0, len(boxes))
	for _, box := range boxes {
		if box.Target.Kind == kind {
			filtered = append(filtered, box)
		}
	}
	return filtered
}

func lineIndexContaining(t *testing.T, content string, parts ...string) int {
	t.Helper()

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if lineHasParts(line, parts...) {
			return i
		}
	}
	t.Fatalf("expected a rendered line containing %v, got:\n%s", parts, content)
	return -1
}

func lineContaining(t *testing.T, content string, parts ...string) string {
	t.Helper()

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if lineHasParts(line, parts...) {
			return line
		}
	}
	t.Fatalf("expected a rendered line containing %v, got:\n%s", parts, content)
	return ""
}

func lineHasParts(line string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(line, part) {
			return false
		}
	}
	return true
}

func colorKey(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("%d/%d/%d/%d", r, g, b, a)
}

func requireModel(t *testing.T, model tea.Model) Model {
	t.Helper()

	next, ok := model.(Model)
	if !ok {
		t.Fatalf("expected updated model to be tui.Model")
	}
	return next
}
