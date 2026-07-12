package instagram

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformInstagramExport,
		Name:    "Instagram Export",
		Status:  platform.StatusPrototype,
		Summary: "Import a local Instagram export ZIP, review parsed activity, and build a cleanup plan.",
		Capabilities: []platform.Capability{
			{ID: platform.CapabilityLocalImport, Label: "Local import", Support: platform.SupportSupported, Description: "Reads an Instagram export ZIP selected from disk."},
			{ID: platform.CapabilityOfficialAPIScan, Label: "Official API scan", Support: platform.SupportUnsupported, Description: "Instagram APIs are not used."},
			{ID: platform.CapabilityReview, Label: "Review", Support: platform.SupportSupported, Description: "Parsed items can be reviewed, filtered, and selected locally."},
			{ID: platform.CapabilityCleanupPlanning, Label: "Cleanup planning", Support: platform.SupportSupported, Description: "Selections can become inspectable local cleanup plans."},
			{ID: platform.CapabilityAssistedCleanup, Label: "Assisted cleanup", Support: platform.SupportSupported, Description: "Vanish can guide user-performed cleanup in the system browser."},
			{ID: platform.CapabilityAutomaticCleanup, Label: "Automatic cleanup", Support: platform.SupportUnsupported, Description: "Vanish does not perform Instagram changes."},
			{ID: platform.CapabilityAccountAuthentication, Label: "Account authentication", Support: platform.SupportUnsupported, Description: "No Instagram login or account credentials are used."},
			{ID: platform.CapabilityNetworkAPIAccess, Label: "Network/API access", Support: platform.SupportUnsupported, Description: "The importer reads local files only."},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionRequestInstagramExport, Label: "Request Instagram export", RequiredCapability: platform.CapabilityLocalImport},
			{ID: platform.ActionChooseExportZIP, Label: "I already have an export ZIP", RequiredCapability: platform.CapabilityLocalImport},
			{ID: platform.ActionDemoImport, Label: "Try demo data", RequiredCapability: platform.CapabilityLocalImport},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Use Instagram's data export/download flow and keep the ZIP on your machine.",
			"Instagram changes menu names over time; look for Download your information or a similar export option.",
			"Vanish reads local JSON files from the ZIP and never contacts Instagram.",
		},
	}
}
