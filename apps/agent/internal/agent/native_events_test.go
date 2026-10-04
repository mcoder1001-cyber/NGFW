package agent

import (
	"context"
	"errors"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeSAEventsIgnoreCountersAndDetectRekey(t *testing.T) {
	sa := ikev2.SAState{Profile: "site", ISPI: 42, State: "AUTHENTICATED", Children: []ikev2.ChildSAState{{ISPI: 1, RSPI: 2}}}
	before, err := nativeSASnapshot([]ikev2.SAState{sa})
	if err != nil {
		t.Fatal(err)
	}
	sa.Uptime = 90
	sa.Children[0].Uptime = 45
	same, _ := nativeSASnapshot([]ikev2.SAState{sa})
	if len(nativeSAChanges(before, same)) != 0 {
		t.Fatal("uptime emitted transition")
	}
	sa.Children[0].ISPI = 3
	after, _ := nativeSASnapshot([]ikev2.SAState{sa})
	events := nativeSAChanges(before, after)
	if len(events) != 1 || events[0].GetAttributes()["change"] != "rekey" {
		t.Fatal(events)
	}
	if len(events[0].Attributes) != 4 {
		t.Fatal("unexpected public fields", events)
	}
	if events[0].Attributes["engine"] != "vpp-ikev2" || events[0].Kind != ngfwv1.EventKind_EVENT_KIND_IPSEC_SA_CHANGED {
		t.Fatal(events)
	}
	down := nativeSAChanges(after, map[string]string{})
	if len(down) != 1 || down[0].Attributes["change"] != "down" {
		t.Fatal(down)
	}
	if _, err := nativeSASnapshot([]ikev2.SAState{sa, sa}); err == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestNativeSAWatcherRetainsBaselineAfterFailedReadAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var reads atomic.Int32
	events := make(chan *ngfwv1.Event, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchNativeSAEvents(ctx, time.Millisecond, func(context.Context) (map[string]string, error) {
			switch reads.Add(1) {
			case 1:
				return map[string]string{"site/1": "AUTHENTICATED|1:2"}, nil
			case 2:
				return nil, errors.New("unavailable")
			default:
				return map[string]string{}, nil
			}
		}, func(e *ngfwv1.Event) { events <- e })
	}()
	select {
	case e := <-events:
		if e.Attributes["change"] != "down" {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("transition missing")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher failed to stop")
	}
	if len(events) != 0 {
		t.Fatal("spurious failure/baseline events")
	}
}
