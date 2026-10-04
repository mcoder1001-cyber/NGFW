package df7_test

import (
	"context"
	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/igmp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp/fake"
	"testing"
	"time"
)

func TestWatchUnsubscribeHasDeadline(t *testing.T) {
	f := fake.New()
	ctx, cancel := context.WithCancel(context.Background())
	cleanup := make(chan context.Context, 1)
	out, e := df7.Watch(ctx, f, &igmp.IgmpEvent{}, func(ctx context.Context, on bool, _ uint32) error {
		if !on {
			cleanup <- ctx
		}
		return nil
	}, func(context.Context, api.Message) (string, bool) { return "", false })
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	select {
	case c := <-cleanup:
		deadline, ok := c.Deadline()
		if !ok || time.Until(deadline) > 3*time.Second {
			t.Fatal("cleanup lacks bounded deadline")
		}
	case <-time.After(time.Second):
		t.Fatal("no unregister")
	}
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("channel not closed")
		}
	case <-time.After(time.Second):
		t.Fatal("watcher did not close")
	}
}
