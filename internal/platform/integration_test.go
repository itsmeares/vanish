package platform_test

import (
	"testing"

	"github.com/itsmeares/vanish/internal/instagram"
	"github.com/itsmeares/vanish/internal/platform"
	"github.com/itsmeares/vanish/internal/reddit"
)

func TestRegistryOrderAndGet(t *testing.T) {
	registry := platform.NewRegistry(instagram.Platform(), reddit.Platform())
	list := registry.List()
	if len(list) != 2 {
		t.Fatalf("expected two platforms, got %d", len(list))
	}
	if list[0].ID != platform.PlatformInstagramExport || list[1].ID != platform.PlatformReddit {
		t.Fatalf("unexpected registry order: %#v", list)
	}

	current, ok := registry.Get(platform.PlatformReddit)
	if !ok || current.ID != platform.PlatformReddit {
		t.Fatalf("expected Get(reddit) to succeed, got platform=%#v ok=%v", current, ok)
	}
	if _, ok := registry.Get(platform.PlatformID("unknown")); ok {
		t.Fatalf("expected unknown platform lookup to return false")
	}
}

func TestInstagramPlatformCapabilitiesAndActions(t *testing.T) {
	current := instagram.Platform()
	if current.ID != platform.PlatformInstagramExport || current.Status != platform.StatusPrototype {
		t.Fatalf("unexpected Instagram platform identity: %#v", current)
	}

	wantCapabilities := map[string]platform.CapabilitySupport{
		"Local ZIP scan":       platform.SupportPrototype,
		"Review":               platform.SupportYes,
		"Dry-run plan":         platform.SupportPrototype,
		"Apply / execution":    platform.SupportNo,
		"Login / account auth": platform.SupportNo,
		"Network access":       platform.SupportNo,
	}
	assertCapabilities(t, current.Capabilities, wantCapabilities)

	wantActions := []string{
		platform.ActionChooseExportZIP,
		platform.ActionExportGuide,
		platform.ActionViewRecentImports,
		platform.ActionDemoImport,
		platform.ActionBack,
	}
	assertActionIDs(t, current.Actions, wantActions)
	for _, action := range current.Actions {
		if action.Disabled {
			t.Fatalf("expected Instagram action %q to be enabled", action.ID)
		}
	}
}

func TestRedditPlatformPlannedActions(t *testing.T) {
	current := reddit.Platform()
	if current.ID != platform.PlatformReddit || current.Status != platform.StatusPlanned {
		t.Fatalf("unexpected Reddit platform identity: %#v", current)
	}
	if current.Summary != "Official API planner planned for v0.5." {
		t.Fatalf("unexpected Reddit summary: %q", current.Summary)
	}

	wantCapabilities := map[string]platform.CapabilitySupport{
		"Scan own comments/posts": platform.SupportPlanned,
		"Scan saved items":        platform.SupportPlanned,
		"Scan votes":              platform.SupportPlanned,
		"Generate dry-run plans":  platform.SupportPlanned,
		"Apply cleanup":           platform.SupportLater,
		"OAuth":                   platform.SupportPlanned,
		"Network/API access":      platform.SupportNo,
	}
	assertCapabilities(t, current.Capabilities, wantCapabilities)

	wantActions := []string{
		platform.ActionViewIntegrationNote,
		platform.ActionConnectAccount,
		platform.ActionScanActivity,
		platform.ActionBack,
	}
	assertActionIDs(t, current.Actions, wantActions)

	for _, action := range current.Actions {
		switch action.ID {
		case platform.ActionConnectAccount, platform.ActionScanActivity:
			if !action.Disabled {
				t.Fatalf("expected Reddit action %q to stay disabled", action.ID)
			}
			if action.Reason == "" {
				t.Fatalf("expected disabled Reddit action %q to explain why", action.ID)
			}
		default:
			if action.Disabled {
				t.Fatalf("expected Reddit action %q to be enabled", action.ID)
			}
		}
	}
}

func assertCapabilities(t *testing.T, got []platform.Capability, want map[string]platform.CapabilitySupport) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("capability count = %d, want %d: %#v", len(got), len(want), got)
	}
	for _, capability := range got {
		wantSupport, ok := want[capability.Label]
		if !ok {
			t.Fatalf("unexpected capability %q", capability.Label)
		}
		if capability.Support != wantSupport {
			t.Fatalf("capability %q support = %q, want %q", capability.Label, capability.Support, wantSupport)
		}
		if capability.Description == "" {
			t.Fatalf("capability %q missing description", capability.Label)
		}
	}
}

func assertActionIDs(t *testing.T, got []platform.PlatformAction, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("action count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i, wantID := range want {
		if got[i].ID != wantID {
			t.Fatalf("action[%d] = %q, want %q", i, got[i].ID, wantID)
		}
	}
}
