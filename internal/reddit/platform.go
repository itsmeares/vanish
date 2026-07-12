package reddit

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformReddit,
		Name:    "Reddit",
		Status:  platform.StatusPrototype,
		Summary: "Sign in with Reddit, scan your activity, and build a local cleanup plan.",
		Capabilities: []platform.Capability{
			{ID: platform.CapabilityLocalImport, Label: "Local import", Support: platform.SupportUnsupported, Description: "Reddit export import is not implemented."},
			{ID: platform.CapabilityOfficialAPIScan, Label: "Official API scan", Support: platform.SupportPrototype, Description: "Read-only official API scan; developer access remains pending."},
			{ID: platform.CapabilityReview, Label: "Review", Support: platform.SupportSupported, Description: "Scanned items use the shared local review flow."},
			{ID: platform.CapabilityCleanupPlanning, Label: "Cleanup planning", Support: platform.SupportPrototype, Description: "Selected comments and posts can become local cleanup plans."},
			{ID: platform.CapabilityAssistedCleanup, Label: "Assisted cleanup", Support: platform.SupportUnsupported, Description: "No Reddit assisted-cleanup flow exists."},
			{ID: platform.CapabilityAutomaticCleanup, Label: "Automatic cleanup", Support: platform.SupportUnsupported, Description: "No Reddit content or account changes are implemented."},
			{ID: platform.CapabilityAccountAuthentication, Label: "Account authentication", Support: platform.SupportPrototype, Description: "Installed-app OAuth prototype; developer access remains pending."},
			{ID: platform.CapabilityNetworkAPIAccess, Label: "Network/API access", Support: platform.SupportPrototype, Description: "Read-only official OAuth/API prototype only."},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionConnectAccount, Label: "Sign in with Reddit", RequiredCapability: platform.CapabilityAccountAuthentication},
			{ID: platform.ActionScanActivity, Label: "Scan activity", RequiredCapability: platform.CapabilityOfficialAPIScan},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Developer access is pending; approval has not been granted.",
			"Current scanner supports own comments and submitted posts through Reddit's official API.",
			"No Reddit content mutations, scraping, browser automation, password collection, cookie paste, or session paste exists.",
			"TUI flow is manual OAuth: Vanish shows the URL and accepts the returned code or redirect URL.",
		},
	}
}
