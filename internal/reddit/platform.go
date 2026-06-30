package reddit

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformReddit,
		Name:    "Reddit",
		Status:  platform.StatusPrototype,
		Summary: "Sign in with Reddit, scan your activity, and build a dry-run cleanup plan.",
		Capabilities: []platform.Capability{
			{
				Label:       "Scan own comments/posts",
				Support:     platform.SupportPrototype,
				Description: "Uses Reddit's official API for your comments and posts.",
			},
			{
				Label:       "Scan saved items",
				Support:     platform.SupportPlanned,
				Description: "Deferred until saved-item support and cleanup mapping are kept narrow and clear.",
			},
			{
				Label:       "Scan votes",
				Support:     platform.SupportPlanned,
				Description: "Deferred until vote-history support and cleanup mapping are kept narrow and clear.",
			},
			{
				Label:       "Generate dry-run plans",
				Support:     platform.SupportPrototype,
				Description: "Builds local dry-run actions for selected comments and posts only.",
			},
			{
				Label:       "Apply cleanup",
				Support:     platform.SupportLater,
				Description: "Deferred beyond v0.5 planning; no Reddit deletion, edit, vote, or account changes are implemented.",
			},
			{
				Label:       "OAuth",
				Support:     platform.SupportPrototype,
				Description: "Uses installed-app OAuth with secure refresh-token storage.",
			},
			{
				Label:       "Network/API access",
				Support:     platform.SupportPrototype,
				Description: "Uses official Reddit OAuth/API requests only.",
			},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionConnectAccount, Label: "Sign in with Reddit"},
			{ID: platform.ActionScanActivity, Label: "Scan activity"},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Official API planner prototype targets v0.5.",
			"Current scanner supports own comments and submitted posts through Reddit's official API.",
			"Saved items and vote history stay deferred until support and cleanup mapping are clean.",
			"No Reddit content mutations, scraping, browser automation, password collection, cookie paste, or session paste exists.",
			"TUI flow is manual OAuth: Vanish shows the URL and accepts the returned code or redirect URL.",
		},
	}
}
