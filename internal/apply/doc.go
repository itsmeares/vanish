// Package apply validates cleanup plans and runs no-op execution flows.
//
// This package does not call platform APIs or mutate platform content. It gives
// the TUI a shared preview, confirmation, result, and audit-event foundation
// for future real executors.
package apply
