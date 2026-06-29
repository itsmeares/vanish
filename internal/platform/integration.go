package platform

import (
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

type CapabilitySupport string

const (
	SupportYes       CapabilitySupport = "yes"
	SupportPrototype CapabilitySupport = "prototype"
	SupportPlanned   CapabilitySupport = "planned"
	SupportLater     CapabilitySupport = "later"
	SupportNo        CapabilitySupport = "no"
)

type Capability struct {
	Label       string
	Support     CapabilitySupport
	Description string
}

type PlatformAction struct {
	ID       string
	Label    string
	Disabled bool
	Reason   string
}

const (
	ActionChooseExportZIP     = "choose-export-zip"
	ActionExportGuide         = "export-guide"
	ActionViewRecentImports   = "view-recent-imports"
	ActionDemoImport          = "demo-import"
	ActionBack                = "back"
	ActionViewIntegrationNote = "view-integration-notes"
	ActionConnectAccount      = "connect-account"
	ActionScanActivity        = "scan-activity"
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

type Registry struct {
	platforms []Platform
}

func NewRegistry(platforms ...Platform) Registry {
	copied := append([]Platform(nil), platforms...)
	return Registry{platforms: copied}
}

func (r Registry) List() []Platform {
	return append([]Platform(nil), r.platforms...)
}

func (r Registry) Get(id PlatformID) (Platform, bool) {
	for _, candidate := range r.platforms {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Platform{}, false
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
