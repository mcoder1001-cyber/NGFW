package ravpn

import (
	"context"
	"testing"
	"time"
)

type immutableLifecycleVerifier struct {
	plan    *NetworkPlan
	handoff Handoff
}

func (v immutableLifecycleVerifier) Verify(context.Context, EngineSpec) (*NetworkPlan, Handoff, error) {
	return v.plan, v.handoff, nil
}

func TestSessionsProceedDuringReadinessWhileLifecycleMutationWaits(t *testing.T) {
	r, v, _, _, _, _, spec := lifecycleFixture(t)
	if _, err := r.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	r.verifier = immutableLifecycleVerifier{v.plan, v.handoff}
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r.SetReadiness(func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ErrEngine
		}
	})
	ready := make(chan error, 1)
	go func() { ready <- r.Ready(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("readiness did not enter")
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	sessions := make(chan error, 1)
	go func() { _, err := r.Sessions(ctx, spec.Profile); sessions <- err }()
	select {
	case err := <-sessions:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("session read blocked behind read-only readiness")
	}
	mutation := make(chan struct{})
	go func() { r.SetInitializationReady(true); close(mutation) }()
	select {
	case <-mutation:
		t.Fatal("lifecycle state changed during observation")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("readiness failed to release")
	}
	select {
	case <-mutation:
	case <-ctx.Done():
		t.Fatal("lifecycle mutation remained blocked")
	}
}

func TestObservationGateWaitIsCancelledWithoutLosingToken(t *testing.T) {
	var g observationGate
	if !g.acquire(context.Background()) {
		t.Fatal("first acquisition")
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan bool, 1)
	go func() { waiting <- g.acquire(ctx) }()
	cancel()
	select {
	case ok := <-waiting:
		if ok {
			t.Fatal("concurrent or cancelled acquisition")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled wait did not return")
	}
	g.release()
	if g.acquire(ctx) {
		t.Fatal("cancelled caller acquired token")
	}
	fresh, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if !g.acquire(fresh) {
		t.Fatal("cancelled wait consumed the token")
	}
	g.release()
}
