package agent

import (
	"context"
	"errors"
	"google.golang.org/protobuf/types/known/timestamppb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/scheduler"
	"testing"
	"time"
)

func startAutoBlockWatcher(t *testing.T, s *Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); (&Agent{svc: s}).watchAutoBlock(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
}

func TestInactiveAutoBlockMaintenancePreservesExternalACL(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name := "unconfigured"
		if configured {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			v := coretest.New()
			s, _ := newACLSvc(t, v, t.TempDir(), false)
			if configured {
				mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "inactive-config", DesiredState: doc(t, `{"security":{"autoBlock":{"enabled":false}}}`), Subsystems: []string{"security"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
			}
			idx := v.AddACL(testOwner + ":external-reference")
			sub := s.events().subscribe(&ngfwv1.StreamEventsRequest{Kinds: []ngfwv1.EventKind{ngfwv1.EventKind_EVENT_KIND_RECONCILE_START}})
			defer s.events().unsubscribe(sub)
			startAutoBlockWatcher(t, s)
			ctx, cancel := context.WithTimeout(context.Background(), 1300*time.Millisecond)
			defer cancel()
			events, err := sub.next(ctx)
			if len(events) != 0 {
				t.Fatalf("inactive maintenance emitted reconciliation: %s", events[0].GetMessage())
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("wait: %v", err)
			}
			if !v.ACL().Has(idx) || len(v.CallsNamed("acl_del")) != 0 {
				t.Fatal("inactive maintenance touched external ACL")
			}
		})
	}
}

func TestDirtyEmptyAutoBlockMaintenanceClearsStaleOverlay(t *testing.T) {
	v := coretest.New()
	s, _ := newACLSvc(t, v, t.TempDir(), false)
	idx := v.AddACL(testOwner + ":_gb.auto-block.i00")
	s.autoBlock.dirty = true // an authoritative empty snapshot still owes cleanup
	sub := s.events().subscribe(&ngfwv1.StreamEventsRequest{Kinds: []ngfwv1.EventKind{ngfwv1.EventKind_EVENT_KIND_RECONCILE_DONE}})
	defer s.events().unsubscribe(sub)
	startAutoBlockWatcher(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, err := sub.next(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("dirty cleanup: events=%v err=%v", events, err)
	}
	if err := s.lock(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.unlock()
	if v.ACL().Has(idx) || s.autoBlock.dirty || s.autoBlock.fingerprint == "" {
		t.Fatal("empty dirty snapshot cleanup was skipped")
	}
}

func TestActiveAutoBlockMaintenanceExpiresCachedEntries(t *testing.T) {
	v := coretest.New()
	s, _ := newACLSvc(t, v, t.TempDir(), false)
	host := &autoBlockHostMemory{}
	reg := scheduler.NewRegistry()
	for _, d := range s.sched.Registry().Descriptors() {
		if d.Name() != nftables.DescriptorName {
			reg.Register(d)
		}
	}
	reg.Register(host)
	s.sched = scheduler.New(reg, nil)
	s.sched.VerifyRetries = 0
	ds := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]}},"security":{"autoBlock":{"enabled":true,"maxEntries":100}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "maintenance-enabled", DesiredState: ds}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	now := time.Now()
	s.now = func() time.Time { return now }
	_, err := s.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{{Source: "192.0.2.7", ExpiresAt: timestamppb.New(now.Add(time.Second))}}})
	if err != nil || len(v.ACL().ACLs()) != 3 || host.value == nil {
		t.Fatalf("active enforcement missing before expiry: %v", err)
	}
	sub := s.events().subscribe(&ngfwv1.StreamEventsRequest{Kinds: []ngfwv1.EventKind{ngfwv1.EventKind_EVENT_KIND_RECONCILE_DONE}})
	defer s.events().unsubscribe(sub)
	now = now.Add(2 * time.Second)
	startAutoBlockWatcher(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, err := sub.next(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("watcher expiry: events=%v err=%v", events, err)
	}
	if err := s.lock(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.unlock()
	if len(v.ACL().ACLs()) != 0 || host.value != nil || s.autoBlock.dirty {
		t.Fatal("active watcher did not clear expired VPP/host enforcement")
	}
}
