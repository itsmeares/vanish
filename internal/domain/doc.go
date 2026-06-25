// Package domain contains Vanish's platform-independent cleanup model.
//
// The important split is:
//   - ActivityItem: something Vanish found during scan/import.
//   - CleanupAction: something Vanish may do later for one item.
//   - CleanupPlan: a saved dry-run list of actions.
//
// Keeping these concepts separate makes the future scan -> review -> plan ->
// apply -> audit flow easier to test and safer to extend. A scanner can collect
// many activity items, the TUI can let the user review them, and the planner can
// save only the selected cleanup actions.
package domain
