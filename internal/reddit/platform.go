package reddit

import "github.com/itsmeares/vanish/internal/platform"

func Platform() platform.Platform {
	return platform.Platform{
		ID:      platform.PlatformReddit,
		Name:    "Reddit",
		Status:  platform.StatusPlanned,
		Summary: "A reserved v0.4 platform slot for future Reddit archive review and planning.",
		Capabilities: []platform.Capability{
			{
				Label:       "Reddit archive scan",
				Support:     platform.SupportPlanned,
				Description: "No Reddit archive importer exists yet.",
			},
			{
				Label:       "Review",
				Support:     platform.SupportPlanned,
				Description: "Future Reddit items should use the same local review surface.",
			},
			{
				Label:       "Dry-run plan",
				Support:     platform.SupportLater,
				Description: "Cleanup planning comes after a safe local import format is defined.",
			},
			{
				Label:       "Apply / execution",
				Support:     platform.SupportNo,
				Description: "No Reddit deletion, edit, vote, or account changes are implemented.",
			},
			{
				Label:       "Login / account auth",
				Support:     platform.SupportNo,
				Description: "No OAuth app, token, session, cookie, or credential flow is present.",
			},
			{
				Label:       "Network / API access",
				Support:     platform.SupportNo,
				Description: "v0.4 does not call Reddit APIs or scan account activity.",
			},
		},
		Actions: []platform.PlatformAction{
			{ID: platform.ActionViewIntegrationNote, Label: "View integration notes"},
			{
				ID:       platform.ActionConnectAccount,
				Label:    "Connect Reddit account",
				Disabled: true,
				Reason:   "Planned only. v0.4 does not implement OAuth, tokens, sessions, or API calls.",
			},
			{
				ID:       platform.ActionScanActivity,
				Label:    "Scan Reddit activity",
				Disabled: true,
				Reason:   "Planned only. v0.4 does not import Reddit data or make network requests.",
			},
			{ID: platform.ActionBack, Label: "Back"},
		},
		Notes: []string{
			"Reddit is visible so the TUI and docs can describe intended support without pretending it works.",
			"No Reddit OAuth, API, token, session, browser automation, or scraping code exists in v0.4.",
			"Future work should start with local archive parsing before any account-connected design.",
		},
	}
}
