package scheduler

import "time"

// TD-13 test helpers: the tests reach the new Issue fields and Scheduler settings only through
// these functions, so the base-first run can swap this file for a shim with the base's behaviour
// (docs/status/tasks/TD-13.md §2). Exported for the external scheduler_test package.

// IssueRule returns is.Rule.
func IssueRule(is Issue) string { return is.Rule }

// IssuePointer returns is.Pointer.
func IssuePointer(is Issue) string { return is.Pointer }

// SetValidateTimeout sets s.ValidateTimeout.
func SetValidateTimeout(s *Scheduler, d time.Duration) { s.ValidateTimeout = d }

// stagedMem is a mem descriptor with a declared stage.
type stagedMem struct {
	*mem
	stage Stage
}

func (m stagedMem) Stage() Stage { return m.stage }
