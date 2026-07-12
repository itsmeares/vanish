package platform

import (
	"fmt"
	"strings"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

type PlatformID string

const (
	PlatformInstagramExport PlatformID = "instagram-export"
	PlatformReddit          PlatformID = "reddit"
)

type PlatformStatus string

const (
	StatusPrototype PlatformStatus = "prototype"
	StatusPlanned   PlatformStatus = "planned"
)

type CapabilityID string

const (
	CapabilityLocalImport           CapabilityID = "local_import"
	CapabilityOfficialAPIScan       CapabilityID = "official_api_scan"
	CapabilityReview                CapabilityID = "review"
	CapabilityCleanupPlanning       CapabilityID = "cleanup_planning"
	CapabilityAssistedCleanup       CapabilityID = "assisted_cleanup"
	CapabilityAutomaticCleanup      CapabilityID = "automatic_cleanup"
	CapabilityAccountAuthentication CapabilityID = "account_authentication"
	CapabilityNetworkAPIAccess      CapabilityID = "network_api_access"
)

type CapabilitySupport string

const (
	SupportSupported   CapabilitySupport = "supported"
	SupportPrototype   CapabilitySupport = "prototype"
	SupportPlanned     CapabilitySupport = "planned"
	SupportLater       CapabilitySupport = "later"
	SupportUnsupported CapabilitySupport = "unsupported"
)

func (support CapabilitySupport) Available() bool {
	return support == SupportSupported || support == SupportPrototype
}

func (support CapabilitySupport) valid() bool {
	switch support {
	case SupportSupported, SupportPrototype, SupportPlanned, SupportLater, SupportUnsupported:
		return true
	default:
		return false
	}
}

type Capability struct {
	ID          CapabilityID
	Label       string
	Support     CapabilitySupport
	Description string
}

type PlatformAction struct {
	ID                 string
	Label              string
	RequiredCapability CapabilityID
	Disabled           bool
	Reason             string
}

const (
	ActionRequestInstagramExport = "request-instagram-export"
	ActionChooseExportZIP        = "choose-export-zip"
	ActionExportGuide            = "export-guide"
	ActionViewRecentImports      = "view-recent-imports"
	ActionDemoImport             = "demo-import"
	ActionBack                   = "back"
	ActionViewIntegrationNote    = "view-integration-notes"
	ActionConnectAccount         = "connect-account"
	ActionScanActivity           = "scan-activity"
)

type Platform struct {
	ID           PlatformID
	Name         string
	Status       PlatformStatus
	Summary      string
	Capabilities []Capability
	Actions      []PlatformAction
	Notes        []string
}

func (p Platform) Capability(id CapabilityID) (Capability, bool) {
	for _, capability := range p.Capabilities {
		if capability.ID == id {
			return capability, true
		}
	}
	return Capability{}, false
}

func (p Platform) CapabilityState(id CapabilityID) (CapabilitySupport, bool) {
	capability, ok := p.Capability(id)
	if !ok {
		return "", false
	}
	return capability.Support, true
}

func (p Platform) Action(id string) (PlatformAction, bool) {
	for _, action := range p.Actions {
		if action.ID == id {
			return action, true
		}
	}
	return PlatformAction{}, false
}

func (p Platform) ActionAvailable(action PlatformAction) (bool, string) {
	if action.Disabled {
		return false, strings.TrimSpace(action.Reason)
	}
	if action.RequiredCapability == "" {
		return true, ""
	}
	capability, ok := p.Capability(action.RequiredCapability)
	if !ok {
		return false, "Required capability is unavailable."
	}
	if capability.Support.Available() {
		return true, ""
	}
	return false, fmt.Sprintf("%s is %s.", capability.Label, capability.Support)
}

type Registry struct {
	order     []PlatformID
	platforms map[PlatformID]Platform
}

func NewRegistry(platforms ...Platform) (Registry, error) {
	registry := Registry{
		order:     make([]PlatformID, 0, len(platforms)),
		platforms: make(map[PlatformID]Platform, len(platforms)),
	}
	for _, candidate := range platforms {
		if err := validatePlatform(candidate); err != nil {
			return Registry{}, err
		}
		if _, exists := registry.platforms[candidate.ID]; exists {
			return Registry{}, fmt.Errorf("platform id %q is registered more than once", candidate.ID)
		}
		registry.order = append(registry.order, candidate.ID)
		registry.platforms[candidate.ID] = clonePlatform(candidate)
	}
	return registry, nil
}

func (r Registry) List() []Platform {
	platforms := make([]Platform, 0, len(r.order))
	for _, id := range r.order {
		if current, ok := r.platforms[id]; ok {
			platforms = append(platforms, clonePlatform(current))
		}
	}
	return platforms
}

func (r Registry) Get(id PlatformID) (Platform, bool) {
	current, ok := r.platforms[id]
	if !ok {
		return Platform{}, false
	}
	return clonePlatform(current), true
}

func validatePlatform(current Platform) error {
	if strings.TrimSpace(string(current.ID)) == "" {
		return fmt.Errorf("platform id is required")
	}
	if strings.TrimSpace(current.Name) == "" {
		return fmt.Errorf("platform %q name is required", current.ID)
	}
	capabilityIDs := make(map[CapabilityID]struct{}, len(current.Capabilities))
	for _, capability := range current.Capabilities {
		if strings.TrimSpace(string(capability.ID)) == "" {
			return fmt.Errorf("platform %q capability id is required", current.ID)
		}
		if strings.TrimSpace(capability.Label) == "" {
			return fmt.Errorf("platform %q capability %q label is required", current.ID, capability.ID)
		}
		if !capability.Support.valid() {
			return fmt.Errorf("platform %q capability %q support %q is invalid", current.ID, capability.ID, capability.Support)
		}
		if _, exists := capabilityIDs[capability.ID]; exists {
			return fmt.Errorf("platform %q capability id %q is registered more than once", current.ID, capability.ID)
		}
		capabilityIDs[capability.ID] = struct{}{}
	}
	actionIDs := make(map[string]struct{}, len(current.Actions))
	for _, action := range current.Actions {
		if strings.TrimSpace(action.ID) == "" || strings.TrimSpace(action.Label) == "" {
			return fmt.Errorf("platform %q action id and label are required", current.ID)
		}
		if _, exists := actionIDs[action.ID]; exists {
			return fmt.Errorf("platform %q action id %q is registered more than once", current.ID, action.ID)
		}
		actionIDs[action.ID] = struct{}{}
		if action.RequiredCapability != "" {
			if _, exists := capabilityIDs[action.RequiredCapability]; !exists {
				return fmt.Errorf("platform %q action %q requires unknown capability %q", current.ID, action.ID, action.RequiredCapability)
			}
		}
	}
	return nil
}

func clonePlatform(current Platform) Platform {
	current.Capabilities = append([]Capability(nil), current.Capabilities...)
	current.Actions = append([]PlatformAction(nil), current.Actions...)
	current.Notes = append([]string(nil), current.Notes...)
	return current
}

// BuildPlanRequest asks a planner to turn already-reviewed selections into
// cleanup actions inside a local dry-run plan.
type BuildPlanRequest struct {
	PlanID     string
	Platform   domain.PlatformName
	SourceName string
	CreatedAt  time.Time
	Items      []domain.ActivityItem
}
