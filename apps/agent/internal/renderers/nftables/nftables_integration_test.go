package nftables_test

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/renderers/nftables/nftest"
	"ngfw/agent/internal/scheduler"
)

// Integration (VRX_INTEGRATION=1, lab lock shared): the host firewall in the slot's own namespace
// ns-<prefix>-hacl through the real scheduler and nft 1.1.6 — never the root netns (nftest compares the
// root `nft list tables` before and after). Covers: apply → the kernel table (pasted into the log) and
// Retrieve == desired; packets from the veth peer: an allowed port connects, a dropped port times out and
// its rule counter increments; update and rollback (Retrieve, not assumption); a lost table re-rendered
// identically by a fresh descriptor (agent restart); a foreign table in the same namespace untouched
// throughout; removal.

var counterRe = regexp.MustCompile(`counter packets \d+ bytes \d+`)

func normalise(listing string) string { return counterRe.ReplaceAllString(listing, "counter") }

func hostDoc(t *testing.T, peer string, extra string) *vrxv1.DesiredState {
	t.Helper()
	js := fmt.Sprintf(`{
	  "objects": {"addresses": {"peer": {"type": "host", "address": %q}}},
	  "acl": {
	    "host": {"local-in": {"rules": [
	      {"sequence": 10, "action": "accept", "source": {"kind": "object", "name": "peer"}, "service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["2222"]}}},
	      {"sequence": 20, "action": "drop", "log": true, "service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["2323"]}}}
	      %s
	    ]}},
	    "hostAttachments": [{"list": "local-in", "chain": "input", "priority": 0}],
	    "hostSettings": {"antiLockout": {"enabled": true, "sources": [%q], "ports": [22]}}
	  }
	}`, peer, extra, peer+"/32")
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func value(t *testing.T, ds *vrxv1.DesiredState) *nftables.HostTable {
	t.Helper()
	v, issues := nftables.Build(nftables.Input{ACL: ds.GetAcl(), Objects: ds.GetObjects()})
	for _, is := range issues {
		if !is.Warning {
			t.Fatalf("build: %+v", is)
		}
	}
	return v
}

type agent struct {
	d     *nftables.Descriptor
	sched *scheduler.Scheduler
	rt    *nftables.Runtime
}

// newAgent is what the product wiring does (subsystems/host_acl.go), on the test paths.
func newAgent(h *nftest.Harness) *agent {
	p := h.Paths()
	d := nftables.NewDescriptor(nftables.New(h.Runner(), p), nftables.NewStore(p.StoreFile), slog.Default())
	reg := scheduler.NewRegistry()
	reg.Register(d)
	return &agent{d: d, sched: scheduler.New(reg, slog.Default()), rt: &nftables.Runtime{Owner: h.Prefix, Descriptor: d}}
}

func (a *agent) apply(t *testing.T, v *nftables.HostTable, resync bool) {
	t.Helper()
	var kvs []scheduler.KV
	if v != nil {
		kvs = []scheduler.KV{{Key: nftables.Key, Value: v}}
	}
	res := a.sched.ApplyWith(context.Background(), kvs, scheduler.Only(nftables.DescriptorName), scheduler.ApplyOptions{Resync: resync})
	if res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply: %s %v", res.Outcome, res.Err)
	}
	t.Logf("apply: %s summary=%+v in %s", res.Outcome, res.Summary, res.Duration)
}

func (a *agent) retrieve(t *testing.T) *nftables.HostTable {
	t.Helper()
	kvs, err := a.d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(kvs) == 0 {
		return nil
	}
	return kvs[0].Value.(*nftables.HostTable)
}

func (a *agent) counter(t *testing.T, seq uint32) uint64 {
	t.Helper()
	st, err := a.rt.State(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var n uint64
	for _, c := range st.GetChains() {
		for _, r := range c.GetRules() {
			if r.GetKind() == nftables.KindRule && r.GetSequence() == seq {
				n += r.GetPackets()
			}
		}
	}
	return n
}

func TestIntegrationHostFirewallInSlotNetns(t *testing.T) {
	h := nftest.New(t)
	peer := h.PeerAddr.String()
	table := h.Paths().Table

	// A foreign table in the same namespace (stand-in for P10's base policy): never touched.
	h.NftStdin(t, "table inet foreign {\n\tchain c {\n\t\ttype filter hook input priority 100; policy accept;\n\t\tcounter\n\t}\n}\n")
	foreign := normalise(h.Nft(t, "list", "table", "inet", "foreign"))

	a := newAgent(h)
	vA := value(t, hostDoc(t, peer, ""))
	a.apply(t, vA, false)
	listingA := h.Nft(t, "list", "table", "inet", table)
	t.Logf("`nft list table inet %s` in %s after apply:\n%s", table, h.Host, listingA)
	t.Logf("`nft list tables` in %s:\n%s", h.Host, h.Nft(t, "list", "tables"))
	if got := a.retrieve(t); !proto.Equal(got, vA) {
		t.Fatalf("Retrieve != desired after apply:\n got %v\nwant %v", got, vA)
	}

	// Packets from the veth peer.
	for _, port := range []int{2222, 2323, 2424} {
		h.Listen(t, port)
	}
	if err := h.Dial(2222, 3*time.Second); err != nil {
		t.Fatalf("allowed port 2222 must connect: %v", err)
	}
	before := a.counter(t, 20)
	if err := h.Dial(2323, 1500*time.Millisecond); err == nil {
		t.Fatal("port 2323 must be dropped")
	} else {
		t.Logf("port 2323 from %s: %v (dropped)", peer, err)
	}
	after := a.counter(t, 20)
	if after <= before {
		t.Fatalf("drop rule counter did not increment: %d → %d", before, after)
	}
	t.Logf("drop rule (sequence 20) counter: %d → %d packets; allowed rule (sequence 10): %d packets", before, after, a.counter(t, 10))
	if err := h.Dial(2424, 3*time.Second); err != nil {
		t.Fatalf("port 2424 (no rule, policy accept) must connect: %v", err)
	}

	// Update: reject 2424 as well → Retrieve == new value, 2424 refused.
	vB := value(t, hostDoc(t, peer, `, {"sequence": 30, "action": "reject", "service": {"kind": "inline", "spec": {"protocol": "tcp", "destinationPorts": ["2424"]}}}`))
	a.apply(t, vB, false)
	if got := a.retrieve(t); !proto.Equal(got, vB) {
		t.Fatal("Retrieve != desired after update")
	}
	if err := h.Dial(2424, 1500*time.Millisecond); err == nil {
		t.Fatal("port 2424 must be rejected after the update")
	} else {
		t.Logf("port 2424 after the update: %v (rejected)", err)
	}

	// Rollback: the previous revision again → Retrieve == vA and the kernel listing equals the first one.
	a.apply(t, vA, false)
	if got := a.retrieve(t); !proto.Equal(got, vA) {
		t.Fatal("Retrieve != previous revision after rollback")
	}
	if got := normalise(h.Nft(t, "list", "table", "inet", table)); got != normalise(listingA) {
		t.Fatalf("rollback listing differs:\n%s\n---\n%s", got, listingA)
	}

	// Restart simulation: the table is lost while the agent is down; a fresh descriptor (new process)
	// sees the difference and re-renders it identically.
	h.Nft(t, "delete", "table", "inet", table)
	b := newAgent(h)
	if got := b.retrieve(t); proto.Equal(got, vA) || len(got.GetChains()) != 0 {
		t.Fatalf("a lost table must be visible to Retrieve: %v", got)
	}
	start := time.Now()
	b.apply(t, vA, true)
	relisted := h.Nft(t, "list", "table", "inet", table)
	if normalise(relisted) != normalise(listingA) {
		t.Fatalf("re-rendered table differs:\n%s\n---\n%s", relisted, listingA)
	}
	t.Logf("restart simulation: table re-rendered in %s; diff against the first listing (counters normalised): empty", time.Since(start))
	if got := b.retrieve(t); !proto.Equal(got, vA) {
		t.Fatal("Retrieve != desired after re-render")
	}

	if got := normalise(h.Nft(t, "list", "table", "inet", "foreign")); got != foreign {
		t.Fatalf("foreign table changed:\n%s\n---\n%s", got, foreign)
	}

	// Removal: the table goes, the foreign table stays.
	b.apply(t, nil, false)
	tables := h.Nft(t, "list", "tables")
	t.Logf("`nft list tables` in %s after removal:\n%s", h.Host, tables)
	if tables != "table inet foreign\n" {
		t.Fatalf("after removal: %q", tables)
	}
}
