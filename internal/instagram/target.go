package instagram

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/itsmeares/vanish/internal/domain"
)

type TargetKind string

const (
	TargetProfile TargetKind = "profile"
	TargetPost    TargetKind = "post"
	TargetReel    TargetKind = "reel"
	TargetTV      TargetKind = "tv"
)

type TrustedTarget struct {
	URL        string
	Kind       TargetKind
	Identifier string
}

var (
	instagramUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9._]+$`)
	instagramMediaIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	errUnsafeInstagramTarget = errors.New("Instagram target cannot be opened safely")
)

func ValidateCleanupTarget(actionType domain.ActionType, rawURL, targetID string) (TrustedTarget, error) {
	rawURL = strings.TrimSpace(rawURL)
	if actionType == domain.ActionUnfollow && rawURL == "" {
		username := strings.TrimPrefix(strings.TrimSpace(targetID), "@")
		if validInstagramUsername(username) {
			return TrustedTarget{
				URL:        "https://www.instagram.com/" + username + "/",
				Kind:       TargetProfile,
				Identifier: username,
			}, nil
		}
	}

	target, err := parseTrustedInstagramTarget(rawURL)
	if err != nil {
		return TrustedTarget{}, errUnsafeInstagramTarget
	}
	switch actionType {
	case domain.ActionUnfollow:
		if target.Kind != TargetProfile {
			return TrustedTarget{}, errUnsafeInstagramTarget
		}
	case domain.ActionUnlike, domain.ActionDeleteComment:
		if target.Kind == TargetProfile {
			return TrustedTarget{}, errUnsafeInstagramTarget
		}
	default:
		return TrustedTarget{}, errUnsafeInstagramTarget
	}
	return target, nil
}

func parseTrustedInstagramTarget(rawURL string) (TrustedTarget, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.Port() != "" {
		return TrustedTarget{}, errUnsafeInstagramTarget
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "instagram.com" && host != "www.instagram.com" {
		return TrustedTarget{}, errUnsafeInstagramTarget
	}
	if parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") {
		return TrustedTarget{}, errUnsafeInstagramTarget
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) == 1 && validInstagramUsername(segments[0]) {
		identifier := segments[0]
		return TrustedTarget{
			URL:        "https://www.instagram.com/" + identifier + "/",
			Kind:       TargetProfile,
			Identifier: identifier,
		}, nil
	}
	if len(segments) != 2 || !instagramMediaIDPattern.MatchString(segments[1]) {
		return TrustedTarget{}, errUnsafeInstagramTarget
	}

	kind := TargetKind("")
	switch strings.ToLower(segments[0]) {
	case "p":
		kind = TargetPost
	case "reel":
		kind = TargetReel
	case "tv":
		kind = TargetTV
	default:
		return TrustedTarget{}, errUnsafeInstagramTarget
	}
	return TrustedTarget{
		URL:        "https://www.instagram.com/" + strings.ToLower(segments[0]) + "/" + segments[1] + "/",
		Kind:       kind,
		Identifier: segments[1],
	}, nil
}

func validInstagramUsername(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 30 || !instagramUsernamePattern.MatchString(value) {
		return false
	}
	switch strings.ToLower(value) {
	case "accounts", "about", "developer", "direct", "explore", "legal", "p", "reel", "tv":
		return false
	default:
		return true
	}
}
