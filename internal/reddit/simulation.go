package reddit

import (
	"github.com/itsmeares/vanish/internal/apply"
	"github.com/itsmeares/vanish/internal/domain"
)

const SimulationExecutorID apply.ExecutorID = "reddit-simulation"

type simulationProvider struct{}

func SimulationProvider() apply.Provider {
	return simulationProvider{}
}

func (simulationProvider) Platform() domain.PlatformName {
	return domain.PlatformReddit
}

func (simulationProvider) Mode() apply.ExecutionMode {
	return apply.ExecutionModeSimulation
}

func (simulationProvider) ExecutorID() apply.ExecutorID {
	return SimulationExecutorID
}

func (simulationProvider) Supports(action domain.ActionType) bool {
	switch action {
	case domain.ActionRedditDeleteComment, domain.ActionRedditDeletePost:
		return true
	default:
		return false
	}
}

func (simulationProvider) Prerequisites(_ domain.CleanupPlan, state apply.RuntimeState) []apply.Prerequisite {
	if state.Connected(domain.PlatformReddit) {
		return nil
	}
	return []apply.Prerequisite{{
		Code:     "reddit_account_required",
		Message:  "Connect Reddit before simulating this plan.",
		Blocking: true,
	}}
}

func (simulationProvider) Executor() apply.Executor {
	return apply.NoopExecutor{}
}

func (simulationProvider) Reconciler() apply.Reconciler {
	return nil
}
