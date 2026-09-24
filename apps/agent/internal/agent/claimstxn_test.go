package agent

import (
	"errors"
	"testing"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// TD-11c (review 3.2): every transaction — apply, resync, revert — is one batch of the keyed claim
// stores: opened before the projection, written once after the outcome is known and before the new
// desired state is saved. A failed write leaves the agent DEGRADED; the next successful one clears it.
func TestClaimStoresBracketEveryTransaction(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	var begins, flushes int
	var failWith error
	s.claimsTxn = func() func() error {
		begins++
		return func() error { flushes++; return failWith }
	}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "c1", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "c2", DesiredState: doc(t, `{"interfaces":{"loop1":{"vrf":"nope"}}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED) // validation failure: still one (empty) batch
	if begins != 2 || flushes != 2 {
		t.Fatalf("begins %d flushes %d, want 2 and 2", begins, flushes)
	}
	failWith = errors.New("disk full")
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "c3", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if h := s.Health(); !h.GetDegraded() {
		t.Fatalf("a failed claim-store write must degrade the agent: %v", h)
	}
	failWith = nil
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "c4", DesiredState: doc(t, sampleDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if h := s.Health(); h.GetDegraded() || begins != 4 || flushes != 4 {
		t.Fatalf("after a good write: degraded %v, begins %d flushes %d", h.GetDegraded(), begins, flushes)
	}
}
