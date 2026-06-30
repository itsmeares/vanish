package reddit

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformReddit,
		Name:    "Reddit",
		Status:  platform.StatusPlanned,
		Summary: "Official API planner planned for v0.5.",
		Capabilities: []platform.Capability{
			{
				Label:       "Scan own comments/posts",
				Support:     platform.SupportPlanned,
				Description: "Planned for v0.5 through Reddit's official API; not available in v0.4.",
			},
			{
				Label:       "Scan saved items",
				Support:     platform.SupportPlanned,
				Description: "Planned for v0.5 through Reddit's official API.",
			},
			{
				Label:       "Scan votes",
				Support:     platform.SupportPlanned,
				Description: "Planned for v0.5 through Reddit's official API.",
			},
			{
				Label:       "Generate dry-run plans",
				Support:     platform.SupportPlanned,
				Description: "Planned for selected Reddit activity after official API scanning exists.",
			},
			{
				Label:       "Apply cleanup",
				Support:     platform.SupportLater,
				Description: "Later; no Reddit deletion, edit, vote, or account changes are implemented.",
			},
			{
				Label:       "OAuth",
				Support:     platform.SupportPlanned,
				Description: "Planned for v0.5; v0.4 has no Reddit OAuth.",
			},
			{
				Label:       "Network/API access",
				Support:     platform.SupportNo,
				Description: "Not implemented in v0.4.",
			},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionViewIntegrationNote, Label: "View integration notes"},
			{ID: platform.ActionConnectAccount, Label: "Connect Reddit account", Disabled: true, Reason: "Planned for v0.5; no Reddit OAuth is implemented in v0.4."},
			{ID: platform.ActionScanActivity, Label: "Scan Reddit activity", Disabled: true, Reason: "Planned for v0.5; no Reddit API/network access is implemented in v0.4."},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Official API planner planned for v0.5.",
			"v0.4 has no Reddit OAuth, API client, network access, scraping, browser automation, or apply behavior.",
			"Planned scope: scan own comments/posts, saved items, and votes; generate dry-run plans.",
			"Apply cleanup is later.",
		},
	}
}
