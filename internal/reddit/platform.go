package reddit

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformReddit,
		Name:    "Reddit",
		Status:  platform.StatusPlanned,
		Summary: "Official API planner planned for v0.5. v0.4 shows direction only; nothing connects to Reddit yet.",
		Capabilities: []platform.Capability{
			{
				Label:       "Scan own comments/posts",
				Support:     platform.SupportPlanned,
				Description: "Planned for v0.5 through Reddit's official API; not implemented in v0.4.",
			},
			{
				Label:       "Scan saved items",
				Support:     platform.SupportPlanned,
				Description: "Planned for v0.5; no saved-item scan exists in v0.4.",
			},
			{
				Label:       "Scan votes",
				Support:     platform.SupportPlanned,
				Description: "Planned for v0.5; no vote scan exists in v0.4.",
			},
			{
				Label:       "Generate dry-run plans",
				Support:     platform.SupportPlanned,
				Description: "Planned for reviewed Reddit items; no Reddit planner exists in v0.4.",
			},
			{
				Label:       "Apply cleanup",
				Support:     platform.SupportLater,
				Description: "Deferred beyond v0.5 planning; no Reddit deletion, edit, vote, or account changes are implemented.",
			},
			{
				Label:       "OAuth",
				Support:     platform.SupportPlanned,
				Description: "Planned for a future official API flow; v0.4 stores no OAuth app, token, session, cookie, or credential.",
			},
			{
				Label:       "Network/API access",
				Support:     platform.SupportNo,
				Description: "Not implemented in v0.4; no Reddit API client, network calls, scraping, or browser automation exists.",
			},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionViewIntegrationNote, Label: "View integration notes"},
			{
				ID:       platform.ActionConnectAccount,
				Label:    "Connect Reddit account",
				Disabled: true,
				Reason:   "Planned for v0.5. v0.4 has no Reddit OAuth, tokens, sessions, or API calls.",
			},
			{
				ID:       platform.ActionScanActivity,
				Label:    "Scan Reddit activity",
				Disabled: true,
				Reason:   "Planned for v0.5. v0.4 has no Reddit API client, scan, or network request path.",
			},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Official API planner planned for v0.5.",
			"No Reddit OAuth, API, token, session, browser automation, or scraping code exists in v0.4.",
			"Disabled actions mark future direction only; Reddit does not work today.",
		},
	}
}
