package xarchive

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformXArchive,
		Name:    "X / Twitter Archive",
		Status:  platform.StatusPrototype,
		Summary: "Import an X archive ZIP and browse supported posts locally after restart.",
		Capabilities: []platform.Capability{
			{ID: platform.CapabilityLocalImport, Label: "Local import", Support: platform.SupportSupported, Description: "Reads an X archive ZIP selected from disk."},
			{ID: platform.CapabilityOfficialAPIScan, Label: "Official API scan", Support: platform.SupportUnsupported, Description: "X APIs are not used."},
			{ID: platform.CapabilityReview, Label: "Review", Support: platform.SupportSupported, Description: "Imported posts remain browsable in local storage."},
			{ID: platform.CapabilityCleanupPlanning, Label: "Cleanup planning", Support: platform.SupportUnsupported, Description: "X cleanup plans are not created."},
			{ID: platform.CapabilityAssistedCleanup, Label: "Assisted cleanup", Support: platform.SupportUnsupported, Description: "Remote X targets are not opened."},
			{ID: platform.CapabilityAutomaticCleanup, Label: "Automatic cleanup", Support: platform.SupportUnsupported, Description: "Vanish performs no X changes."},
			{ID: platform.CapabilityAccountAuthentication, Label: "Account authentication", Support: platform.SupportUnsupported, Description: "No X login or credentials are used."},
			{ID: platform.CapabilityNetworkAPIAccess, Label: "Network/API access", Support: platform.SupportUnsupported, Description: "Import reads local files only."},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionRequestXArchive, Label: "Request archive", RequiredCapability: platform.CapabilityLocalImport},
			{ID: platform.ActionChooseXArchiveZIP, Label: "Choose archive ZIP", RequiredCapability: platform.CapabilityLocalImport},
			{ID: platform.ActionXDemoImport, Label: "Try demo archive", RequiredCapability: platform.CapabilityLocalImport},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Vanish reads only supported post and account files from the selected ZIP.",
			"Supported post text and linked media are retained locally for browsing after restart.",
			"No X API, login, scraping, browser automation, or remote post opening is used.",
		},
	}
}
