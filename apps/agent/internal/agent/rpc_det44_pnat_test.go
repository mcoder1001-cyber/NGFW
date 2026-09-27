package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// F-det44-map-dslite-cnat: nat.pnat through the whole service on the fake VPP (slot owner w7): commit → Retrieve ==
// canonical (names become pnat-<n>, configuration-only labels) → re-apply empty → loss + agent restart + resync →
// rollback empties the pool and the flow hash, and no call that would crash VPP 26.06 (V11) was made.

const pnatPart = `,
  "nat": {"pnat": {
    "bindings": [{"name": "web", "match": {"proto": "tcp", "dst": "10.7.2.80", "dport": 80}, "rewrite": {"dst": "10.7.1.80"}},
      {"name": "dns", "match": {"proto": "udp", "dst": "10.7.2.53", "dport": 53}, "rewrite": {"dst": "10.7.1.53", "dport": 5353}}],
    "attachments": [{"binding": "web", "interface": "host-w7w0", "point": "input"},
      {"binding": "dns", "interface": "host-w7w0", "point": "input"},
      {"binding": "dns", "interface": "host-w7l0", "point": "output"}]}}`

// canonicalPnat is the applied document itself: Retrieve takes binding names and the binding / attachment order from
// it (matched by the match tuple), so drift stays empty (review BLOCK 1).
const canonicalPnat = `{"pnat": {
    "bindings": [{"name": "web", "match": {"proto": "tcp", "dst": "10.7.2.80", "dport": 80}, "rewrite": {"dst": "10.7.1.80"}},
      {"name": "dns", "match": {"proto": "udp", "dst": "10.7.2.53", "dport": 53}, "rewrite": {"dst": "10.7.1.53", "dport": 5353}}],
    "attachments": [{"binding": "web", "interface": "host-w7w0", "point": "input"},
      {"binding": "dns", "interface": "host-w7w0", "point": "input"},
      {"binding": "dns", "interface": "host-w7l0", "point": "output"}]}}`

func pnatObjects(v *coretest.VPP) (bindings, flows int, crashes []string) {
	p := v.Pnat()
	p.Lock()
	defer p.Unlock()
	for _, b := range p.Pool {
		if b != nil {
			bindings++
		}
	}
	return bindings, len(p.Flows), append([]string(nil), p.Crashes...)
}

func TestPnatDomainOnFake(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	withPnat := doc(t, strings.Replace(natIfDoc, "%s", pnatPart, 1))
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "p1", DesiredState: withPnat}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	want := doc(t, `{"nat":`+canonicalPnat+`}`).GetNat()
	if got := natNat(t, s); !proto.Equal(got, want) {
		t.Fatalf("Retrieve nat != canonical:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "p2", DesiredState: withPnat}); len(resp.GetResults()) != 0 {
		t.Fatalf("re-apply changed %v", resp.GetResults())
	}

	// loss of the attachment behind the agent's back + agent restart → resync re-attaches, no duplicate binding
	p := v.Pnat()
	p.Lock()
	for k := range p.Flows {
		delete(p.Flows, k)
	}
	p.Ifaces = map[uint32]int{}
	p.Unlock()
	s.Close()
	s2 := newSvc(t, v, dir)
	s2.Resync(context.Background())
	if got := natNat(t, s2); !proto.Equal(got, want) {
		t.Fatalf("after restart + resync:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if b, f, _ := pnatObjects(v); b != 2 || f != 3 {
		t.Fatalf("after resync: %d bindings, %d flows (want 2, 3)", b, f)
	}

	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "p3", DesiredState: doc(t, `{"vrfs": {"cust": {"id": 7001}}, "interfaces": {}, "nat": {}}`)}),
		vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := natNat(t, s2); proto.Size(got) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if b, f, crashes := pnatObjects(v); b != 0 || f != 0 || len(crashes) != 0 {
		t.Fatalf("after rollback: %d bindings, %d flows, crashes %v", b, f, crashes)
	}
}
