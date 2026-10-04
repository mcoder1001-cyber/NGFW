package agent

import (
	"context"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWANCleanupRejectsObservationReplacedWhileWaitingForTransaction(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	saved := s.st.wanSaved
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.lock(ctx); err != nil {
		t.Fatal(err)
	}
	var called atomic.Bool
	result := make(chan bool, 1)
	go func() {
		current, err := s.withCurrentWAN(ctx, saved, func(context.Context) error { called.Store(true); return nil })
		result <- current && err == nil
	}()
	s.st.wanSaved = &ngfwv1.DesiredState{}
	s.unlock()
	if <-result || called.Load() {
		t.Fatal("cleanup acted on a superseded durable snapshot")
	}
	current, err := s.withCurrentWAN(ctx, s.st.wanSaved, func(context.Context) error { called.Store(true); return nil })
	if !current || err != nil || !called.Load() {
		t.Fatal(current, err, called.Load())
	}
}

func TestWANUnwiredEmptyConfigurationIsReadable(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	reply, err := (&server{svc: s}).WanState(context.Background(), &ngfwv1.WanStateRequest{})
	if err != nil || len(reply.GetGroups()) != 0 {
		t.Fatal(reply, err)
	}
	s.st.wanSaved = &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{WanGroups: []*ngfwv1.WanGroup{{}}}}
	if _, err := (&server{svc: s}).WanState(context.Background(), &ngfwv1.WanStateRequest{}); err == nil {
		t.Fatal("configured unwired monitor reported healthy empty state")
	}
}
