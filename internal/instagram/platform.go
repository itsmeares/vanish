package instagram

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformInstagramExport,
		Name:    "Instagram Export",
		Status:  platform.StatusPrototype,
		Summary: "Import a local Instagram export ZIP, review parsed activity, and build a dry-run cleanup plan.",
		Capabilities: []platform.Capability{
			{
				Label:       "Local ZIP scan",
				Support:     platform.SupportPrototype,
				Description: "Reads an export ZIP you choose from disk; no platform login is used.",
			},
			{
				Label:       "Review",
				Support:     platform.SupportYes,
				Description: "Parsed items can be browsed, filtered, selected, and inspected locally.",
			},
			{
				Label:       "Dry-run plan",
				Support:     platform.SupportPrototype,
				Description: "Selected likes, comments, and following records can become inspectable JSON actions.",
			},
			{
				Label:       "Apply / execution",
				Support:     platform.SupportNo,
				Description: "Vanish does not delete, unlike, unfollow, or change platform content.",
			},
			{
				Label:       "Login / account auth",
				Support:     platform.SupportNo,
				Description: "No passwords, cookies, tokens, OAuth grants, or sessions are collected.",
			},
			{
				Label:       "Network access",
				Support:     platform.SupportNo,
				Description: "The importer reads local files only and does not contact Instagram.",
			},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionRequestInstagramExport, Label: "Request Instagram export"},
			{ID: platform.ActionChooseExportZIP, Label: "I already have an export ZIP"},
			{ID: platform.ActionDemoImport, Label: "Try demo data"},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Use Instagram's data export/download flow and keep the ZIP on your machine.",
			"Instagram changes menu names over time; look for Download your information or a similar export option.",
			"Vanish reads local JSON files from the ZIP and never contacts Instagram.",
		},
	}
}
