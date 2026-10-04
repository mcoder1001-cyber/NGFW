package agent

import (
	"context"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/detectors"
	"testing"
	"time"
)

func TestAutoBlockObservationPreservesPortThroughStream(t *testing.T) {
	b := newBus()
	sub := b.subscribe(&ngfwv1.StreamEventsRequest{Kinds: []ngfwv1.EventKind{ngfwv1.EventKind_EVENT_KIND_AUTOBLOCK_OBSERVED}})
	defer b.unsubscribe(sub)
	b.publishFeature(autoBlockObserved(detectors.Observation{Source: "192.0.2.7", Kind: "portScan", Port: "443"}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	events, err := sub.next(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("stream: events=%v err=%v", events, err)
	}
	attrs := events[0].GetAttributes()
	if attrs["source_ip"] != "192.0.2.7" || attrs["detector"] != "portScan" || attrs["destination_port"] != "443" {
		t.Fatalf("observation attrs: %v", attrs)
	}
	if events[0].GetSeq() != 1 || events[0].GetTs() == nil {
		t.Fatal("missing stream timestamp/sequence")
	}
}
