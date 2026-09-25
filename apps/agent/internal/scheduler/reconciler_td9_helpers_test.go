package scheduler

import "time"

// TD-9 test accessors for the new scheduler API. The base-first evidence (docs/status/tasks/TD-9.md)
// swaps this file for a shim that encodes the base's behaviour: no rollback bound, no Uncertain, no
// ErrDescriptorPanic.

var errDescriptorPanic = ErrDescriptorPanic

func withRollbackTimeout(s *Scheduler, d time.Duration) { s.RollbackTimeout = d }

func uncertainOf(r *TxnResult) bool { return r.Uncertain }
