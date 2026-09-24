package agent

import (
	"context"
	"errors"
	"testing"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// TD-11c (review 3.2): every transaction — apply, resync, revert — is one batch of the keyed claim
// stores: opened before the projection, ended once before the outcome is recorded and the desired
// state saved. Fix round 1: a failed end is DEGRADED, never APPLIED (review F2: the desired state
// is not merged, so the API re-applies running); a transaction that panics still ends the batch
// (review F4).
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

	// F2: the claims cannot be written → DEGRADED (not APPLIED), the new document is not stored.
	failWith = errors.New("disk full")
	more := doc(t, sampleDoc)
	more.Interfaces["loop703"] = &vrxv1.Interface{}
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "c3", DesiredState: more})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED)
	if h := s.Health(); !h.GetDegraded() {
		t.Fatalf("a failed claim-store write must degrade the agent: %v", h)
	}
	if _, stored := s.st.desired.GetInterfaces()["loop703"]; stored || s.st.meta.LastTxnID == "c3" {
		t.Fatal("the document of a transaction whose claims were not persisted was stored")
	}
	// … a refused transaction keeps its status, the agent is degraded all the same
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "c3b", DesiredState: doc(t, `{"interfaces":{"loop1":{"vrf":"nope"}}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	if !s.Health().GetDegraded() {
		t.Fatal("refused transaction with a failed claim write: agent not degraded")
	}
	// … and the next good write clears it; now the document is stored.
	failWith = nil
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "c4", DesiredState: more}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if h := s.Health(); h.GetDegraded() || begins != 5 || flushes != 5 {
		t.Fatalf("after a good write: degraded %v, begins %d flushes %d", h.GetDegraded(), begins, flushes)
	}
	if _, stored := s.st.desired.GetInterfaces()["loop703"]; !stored {
		t.Fatal("document not stored after the good write")
	}

	// F4: a transaction that panics after the batch opened still ends it (the stores never stay
	// in batch mode).
	before := s.beforeTxn
	s.beforeTxn = func() { panic("boom") }
	func() {
		defer func() { _ = recover() }()
		_, _ = s.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: "c5", DesiredState: more})
	}()
	s.beforeTxn = before
	if begins != 6 || flushes != 6 {
		t.Fatalf("panicking transaction: begins %d flushes %d, want 6 and 6", begins, flushes)
	}
}
