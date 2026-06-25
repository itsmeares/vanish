package domain

import (
	"encoding/json"
	"errors"
	"io"
)

// WritePlanJSON validates and writes a human-readable cleanup plan.
//
// The encoding/json package uses the json tags on each struct field to decide
// the object keys. SetIndent keeps the output easy to inspect in a terminal.
func WritePlanJSON(w io.Writer, plan CleanupPlan) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	if plan.Mode != PlanModeDryRun {
		return validationError("cleanup plan mode %q cannot be written yet; only %q plans are supported", plan.Mode, PlanModeDryRun)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(plan)
}

// ReadPlanJSON loads a cleanup plan and validates it before returning it.
func ReadPlanJSON(r io.Reader) (CleanupPlan, error) {
	if r == nil {
		return CleanupPlan{}, errors.New("reader is nil")
	}

	var plan CleanupPlan
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return CleanupPlan{}, err
	}
	if err := plan.Validate(); err != nil {
		return CleanupPlan{}, err
	}
	return plan, nil
}
