// Package apply validates cleanup plans, routes explicit simulation providers,
// and normalizes provider-owned outcomes into runner-owned action results.
//
// This package does not call platform APIs or mutate platform content. It gives
// the TUI a shared preview, bounded retry, halt, durable execution journal,
// safe-resume, result, and audit-event foundation for future real executors.
// Runtime outcomes and attempts are persisted only in the independent execution
// journal format, never in cleanup-plan JSON.
package apply
