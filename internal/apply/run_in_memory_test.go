package apply

import (
	"context"

	"github.com/itsmeares/vanish/internal/domain"
)

// Run keeps legacy focused unit tests concise while production code has no
// implicit in-memory execution entrypoint.
func (runner Runner) Run(ctx context.Context, plan domain.CleanupPlan, mode ExecutionMode) Execution {
	return runner.runInMemory(ctx, plan, mode)
}
