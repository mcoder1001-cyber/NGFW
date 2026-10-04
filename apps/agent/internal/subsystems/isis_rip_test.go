package subsystems

import (
	"context"
	"fmt"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
	"testing"
)

func TestIGPEventProductionMapping(t *testing.T) {
	for _, tc := range []struct {
		poller string
		kind   ngfwv1.EventKind
	}{
		{"isis-adjacencies", ngfwv1.EventKind_EVENT_KIND_ISIS_ADJACENCY_CHANGED},
		{"ospf-neighbors", ngfwv1.EventKind_EVENT_KIND_OSPF_NEIGHBOR_CHANGED},
		{"ospf6-neighbors", ngfwv1.EventKind_EVENT_KIND_OSPF_NEIGHBOR_CHANGED},
	} {
		ev := EventOf(frr.Event{Poller: tc.poller, Key: "default|peer", Old: "Up", New: "Down"})
		if ev == nil || ev.Kind != tc.kind || ev.Attributes["old"] != "Up" || ev.Attributes["new"] != "Down" {
			t.Fatalf("%s event not delivered: %v", tc.poller, ev)
		}
	}
}

func TestIsisSealedSecretResolver(t *testing.T) {
	rt := &FRR{}
	raw := []byte("NGFW_TEST_PSK_isis")
	rt.secretSource = func(ref string) ([]byte, error) {
		if ref != "password/isis" {
			return nil, fmt.Errorf("unavailable")
		}
		return raw, nil
	}
	value, err := rt.resolveSecret(context.Background(), "password/isis")
	if err != nil || value != "NGFW_TEST_PSK_isis" {
		t.Fatalf("resolve: %q %v", value, err)
	}
	for _, b := range raw {
		if b != 0 {
			t.Fatal("temporary plaintext not cleared")
		}
	}
	if _, err := rt.resolveSecret(context.Background(), "key/ca"); err == nil {
		t.Fatal("foreign kind accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rt.resolveSecret(ctx, "password/isis"); err == nil {
		t.Fatal("canceled resolve accepted")
	}
}
