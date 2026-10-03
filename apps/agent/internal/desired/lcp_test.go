package desired

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/scheduler"
)

// TestLcpDesiredDriftRefused (S-lcp-netns-224-accept ruling, TD-lcp-leftover-local-path):
// lcp.ItfPair.Drift is reported by Retrieve only, never desired. A desired document cannot set it
// (no such leaf: strict decoding refuses it, the lenient decoding of the agent's stored state drops
// it), Lcp never emits it, the validator Lcp runs (lcp.ItfPair.Validate) refuses a pair carrying
// it, and a retrieved drifted pair assembles into a document without it — projected back, it
// differs from the retrieved value, so the scheduler recreates the pair.
func TestLcpDesiredDriftRefused(t *testing.T) {
	doc := []byte(`{"interfaces": {"loop501": {"lcp": {"hostIfName": "w5-lcp0", "netns": "ns-w5", "drift": "api-accept-missing"}}}}`)
	if err := protojson.Unmarshal(doc, &vrxv1.DesiredState{}); err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("strict decoding of interfaces.loop501.lcp.drift: %v, want an unknown-field error", err)
	}
	ds := &vrxv1.DesiredState{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(doc, ds); err != nil {
		t.Fatal(err)
	}
	s := &sink{}
	Lcp(s, ds.GetInterfaces())
	key := scheduler.Join(lcp.NameItfPair, "loop501")
	var p lcp.ItfPair
	if v := s.value(key); v == nil || dfkit.Decode(v, &p) != nil || len(s.errs) != 0 {
		t.Fatalf("projection %v, errors %v", v, s.errs)
	}
	if want := (lcp.ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}); p != want {
		t.Fatalf("projected %+v, want %+v (no drift)", p, want)
	}

	drifted := p
	drifted.Drift = lcp.DriftAPIAcceptMissing
	if err := drifted.Validate(); !errors.Is(err, dfkit.ErrSpec) || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("validate a desired pair carrying drift: %v, want a spec error", err)
	}

	out := &vrxv1.DesiredState{}
	AssembleLcp(out, []scheduler.KV{{Key: key, Value: drifted.Proto()}})
	if js := protojson.Format(out); strings.Contains(js, "drift") || strings.Contains(js, lcp.DriftAPIAcceptMissing) {
		t.Fatalf("assembled document carries the drift: %s", js)
	}
	s2 := &sink{}
	Lcp(s2, out.GetInterfaces())
	back := s2.value(key)
	if back == nil || !proto.Equal(back, p.Proto()) || proto.Equal(back, drifted.Proto()) {
		t.Fatalf("assembled and projected back: %v, want the pair without drift", back)
	}
}
