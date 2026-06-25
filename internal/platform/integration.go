package platform

import (
	"context"
	"time"

	"github.com/itsmeares/vanish/internal/domain"
)

// Integration is the future shared shape for platform adapters.
//
// Interfaces in Go describe behavior. A platform type will satisfy this
// interface by having these methods; it does not need a special declaration.
// The lifecycle intentionally mirrors Vanish's product flow:
// scan -> review -> plan -> apply -> audit.
type Integration interface {
	Scan(context.Context, ScanRequest) (ScanResult, error)
	BuildPlan(context.Context, BuildPlanRequest) (domain.CleanupPlan, error)
	Apply(context.Context, ApplyRequest) (ApplyResult, error)
	Audit(context.Context, AuditRequest) (AuditResult, error)
}

// ScanRequest describes a local scan/import request. The first milestone is
// dry-run only, so this struct is intentionally free of credentials.
type ScanRequest struct {
	SourceName string
	StartedAt  time.Time
}

// ScanResult returns platform-independent activity items for review.
type ScanResult struct {
	Items []domain.ActivityItem
}

// BuildPlanRequest asks an integration to turn already-reviewed selections
// into cleanup actions inside a plan.
type BuildPlanRequest struct {
	PlanID     string
	Platform   domain.PlatformName
	SourceName string
	CreatedAt  time.Time
	Items      []domain.ActivityItem
}

// ApplyRequest is present for the future apply phase. Current integrations
// should keep plans in dry-run mode and avoid destructive behavior.
type ApplyRequest struct {
	Plan domain.CleanupPlan
}

// ApplyResult reports action state after a future apply attempt.
type ApplyResult struct {
	Actions []domain.CleanupAction
}

// AuditRequest asks an integration to check what happened after apply.
type AuditRequest struct {
	Plan domain.CleanupPlan
}

// AuditResult summarizes audited action state without storing sensitive data.
type AuditResult struct {
	Actions []domain.CleanupAction
	Notes   []string
}
