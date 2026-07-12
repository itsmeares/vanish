package instagram

import (
	"github.com/itsmeares/vanish/internal/apply"
	"github.com/itsmeares/vanish/internal/domain"
)

const SimulationExecutorID apply.ExecutorID = "instagram-simulation"

type simulationProvider struct{}

func SimulationProvider() apply.Provider {
	return simulationProvider{}
}

func (simulationProvider) Platform() domain.PlatformName {
	return domain.PlatformInstagram
}

func (simulationProvider) Mode() apply.ExecutionMode {
	return apply.ExecutionModeSimulation
}

func (simulationProvider) ExecutorID() apply.ExecutorID {
	return SimulationExecutorID
}

func (simulationProvider) Supports(action domain.ActionType) bool {
	switch action {
	case domain.ActionUnlike, domain.ActionDeleteComment, domain.ActionUnfollow:
		return true
	default:
		return false
	}
}

func (simulationProvider) Prerequisites(domain.CleanupPlan, apply.RuntimeState) []apply.Prerequisite {
	return nil
}

func (simulationProvider) Executor() apply.Executor {
	return apply.NoopExecutor{}
}
