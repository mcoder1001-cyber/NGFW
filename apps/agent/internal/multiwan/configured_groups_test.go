package multiwan

import (
	"context"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"testing"
)

func TestConfiguredGroupsIndependentOfReadinessAndGateway(t *testing.T) {
	r := NewRuntime(func(context.Context, string, *ngfwv1.WanMonitor) CheckResult {
		return CheckResult{Sent: 1, Unavailable: true}
	})
	t.Cleanup(func() {
		if err := r.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if r.HasConfiguredGroups() {
		t.Fatal("empty runtime configured")
	}
	group := runtimeGroup()
	if err := r.Replace(t.Context(), []*ngfwv1.WanGroup{group}); err != nil {
		t.Fatal(err)
	}
	state := r.HealthFor([]*ngfwv1.WanGroup{group}, "")
	if len(state) != 1 || state[0].Members[0].Up {
		t.Fatal("all-down precondition missing")
	}
	if !r.HasConfiguredGroups() {
		t.Fatal("configured all-down group omitted")
	}
	bad := proto.Clone(group).(*ngfwv1.WanGroup)
	bad.Monitors[0].IntervalMs = proto.Uint32(1)
	if err := r.Replace(t.Context(), []*ngfwv1.WanGroup{bad}); err == nil {
		t.Fatal("invalid monitor accepted")
	}
	if !r.HasConfiguredGroups() {
		t.Fatal("rejected configuration silenced committed group")
	}
	changed := proto.Clone(group).(*ngfwv1.WanGroup)
	changed.Members[0].Priority = proto.Uint32(2)
	if err := r.Replace(t.Context(), []*ngfwv1.WanGroup{changed}); err != nil {
		t.Fatal(err)
	}
	if !r.HasConfiguredGroups() {
		t.Fatal("committed change silenced repair")
	}
	if err := r.Replace(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if r.HasConfiguredGroups() {
		t.Fatal("withdrawn group kept polling")
	}
}
