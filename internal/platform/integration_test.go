package platform_test

import (
	"testing"

	"github.com/itsmeares/vanish/internal/instagram"
	"github.com/itsmeares/vanish/internal/platform"
	"github.com/itsmeares/vanish/internal/reddit"
)

var capabilityOrder = []platform.CapabilityID{
	platform.CapabilityLocalImport,
	platform.CapabilityOfficialAPIScan,
	platform.CapabilityReview,
	platform.CapabilityCleanupPlanning,
	platform.CapabilityAssistedCleanup,
	platform.CapabilityAutomaticCleanup,
	platform.CapabilityAccountAuthentication,
	platform.CapabilityNetworkAPIAccess,
}

func TestRegistryOrderLookupAndDefensiveCopies(t *testing.T) {
	instagramPlatform := instagram.Platform()
	registry, err := platform.NewRegistry(instagramPlatform, reddit.Platform())
	if err != nil {
		t.Fatal(err)
	}
	instagramPlatform.Capabilities[0].Label = "mutated input"
	instagramPlatform.Actions[0].Label = "mutated input"
	instagramPlatform.Notes[0] = "mutated input"

	list := registry.List()
	if len(list) != 2 || list[0].ID != platform.PlatformInstagramExport || list[1].ID != platform.PlatformReddit {
		t.Fatalf("unexpected registry order: %#v", list)
	}
	list[0].Capabilities[0].Label = "mutated output"
	list[0].Actions[0].Label = "mutated output"
	list[0].Notes[0] = "mutated output"

	current, ok := registry.Get(platform.PlatformInstagramExport)
	if !ok || current.Capabilities[0].Label != "Local import" || current.Actions[0].Label != "Request Instagram export" || current.Notes[0] == "mutated output" {
		t.Fatalf("registry leaked shared references: %#v", current)
	}
	current.Capabilities[0].Label = "another mutation"
	again, _ := registry.Get(platform.PlatformInstagramExport)
	if again.Capabilities[0].Label != "Local import" {
		t.Fatalf("Get returned shared capability storage: %#v", again)
	}
	if _, ok := registry.Get(platform.PlatformID("unknown")); ok {
		t.Fatal("expected unknown platform lookup to fail")
	}
}

func TestRegistryRejectsDuplicateAndInvalidDefinitions(t *testing.T) {
	current := instagram.Platform()
	if _, err := platform.NewRegistry(current, current); err == nil {
		t.Fatal("expected duplicate platform id rejection")
	}
	current = instagram.Platform()
	current.Capabilities = append(current.Capabilities, current.Capabilities[0])
	if _, err := platform.NewRegistry(current); err == nil {
		t.Fatal("expected duplicate capability id rejection")
	}
	current = instagram.Platform()
	current.ID = ""
	if _, err := platform.NewRegistry(current); err == nil {
		t.Fatal("expected empty platform id rejection")
	}
	current = instagram.Platform()
	current.Capabilities[0].ID = ""
	if _, err := platform.NewRegistry(current); err == nil {
		t.Fatal("expected empty capability id rejection")
	}
}

func TestTypedCapabilityLookupAndActionAvailability(t *testing.T) {
	current := instagram.Platform()
	current.Capabilities[0].Label = "Display label may change"
	capability, ok := current.Capability(platform.CapabilityLocalImport)
	if !ok || capability.Support != platform.SupportSupported {
		t.Fatalf("typed capability lookup failed: %#v %v", capability, ok)
	}
	state, ok := current.CapabilityState(platform.CapabilityAutomaticCleanup)
	if !ok || state != platform.SupportUnsupported {
		t.Fatalf("typed capability state failed: %q %v", state, ok)
	}
	if available, _ := current.ActionAvailable(current.Actions[0]); !available {
		t.Fatal("supported local import action should be available")
	}
	automatic := platform.PlatformAction{ID: "automatic", Label: "Automatic cleanup", RequiredCapability: platform.CapabilityAutomaticCleanup}
	if available, reason := current.ActionAvailable(automatic); available || reason == "" {
		t.Fatalf("unsupported automatic action should block, available=%v reason=%q", available, reason)
	}
}

func TestInstagramCapabilityDeclaration(t *testing.T) {
	assertCapabilities(t, instagram.Platform(), []platform.CapabilitySupport{
		platform.SupportSupported,
		platform.SupportUnsupported,
		platform.SupportSupported,
		platform.SupportSupported,
		platform.SupportSupported,
		platform.SupportUnsupported,
		platform.SupportUnsupported,
		platform.SupportUnsupported,
	})
}

func TestRedditCapabilityDeclaration(t *testing.T) {
	assertCapabilities(t, reddit.Platform(), []platform.CapabilitySupport{
		platform.SupportUnsupported,
		platform.SupportPrototype,
		platform.SupportSupported,
		platform.SupportPrototype,
		platform.SupportUnsupported,
		platform.SupportUnsupported,
		platform.SupportPrototype,
		platform.SupportPrototype,
	})
}

func assertCapabilities(t *testing.T, current platform.Platform, support []platform.CapabilitySupport) {
	t.Helper()
	if len(current.Capabilities) != len(capabilityOrder) || len(support) != len(capabilityOrder) {
		t.Fatalf("unexpected capability count: %#v", current.Capabilities)
	}
	for i, id := range capabilityOrder {
		capability := current.Capabilities[i]
		if capability.ID != id || capability.Support != support[i] || capability.Label == "" || capability.Description == "" {
			t.Fatalf("capability[%d] mismatch: %#v", i, capability)
		}
	}
}
