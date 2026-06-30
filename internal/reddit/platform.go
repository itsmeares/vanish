package reddit

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformReddit,
		Name:    "Reddit",
		Status:  platform.StatusPrototype,
		Summary: "Official API planner prototype for v0.5. Vanish can use manual installed-app OAuth, scan own comments/posts, and generate local dry-run plans.",
		Capabilities: []platform.Capability{
			{
				Label:       "Scan own comments/posts",
				Support:     platform.SupportPrototype,
				Description: "Prototype scanner uses Reddit's official API for the connected user's own comments and submitted posts.",
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
				Description: "Prototype planner generates local dry-run actions for selected Reddit comments and posts only.",
			},
			{
				Label:       "Apply cleanup",
				Support:     platform.SupportLater,
				Description: "Deferred beyond v0.5 planning; no Reddit deletion, edit, vote, or account changes are implemented.",
			},
			{
				Label:       "OAuth",
				Support:     platform.SupportPrototype,
				Description: "Installed-app OAuth foundation exists with secure refresh-token storage; no embedded browser, password, cookie, or session paste.",
			},
			{
				Label:       "Network/API access",
				Support:     platform.SupportPrototype,
				Description: "Official Reddit OAuth/API client foundation exists behind narrow safety checks; no scraping or browser automation.",
			},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionViewIntegrationNote, Label: "View integration notes"},
			{ID: platform.ActionConnectAccount, Label: "Connect Reddit account"},
			{ID: platform.ActionScanActivity, Label: "Scan Reddit activity"},
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
