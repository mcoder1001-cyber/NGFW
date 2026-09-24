package acl

import (
	"errors"
	"testing"

	"go.fd.io/govpp/adapter"
	"google.golang.org/protobuf/proto"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/scheduler"
)

func TestStatsEnableDescriptor(t *testing.T) {
	ctx := t.Context()
	for _, proper := range []bool{false, true} {
		v := newFakeVPP()
		v.properStatsReply = proper
		d := NewStatsEnable(v)
		if d.KeyOf(StatsEnable{}.Proto()) != KeyStatsEnable || KeyStatsEnable != "acl.stats-enable/global" {
			t.Fatalf("key %q", d.KeyOf(StatsEnable{}.Proto()))
		}
		if kvs := mustRetrieve(t, d); len(kvs) != 0 {
			t.Fatalf("fresh descriptor must report nothing (VPP has no getter): %+v", kvs)
		}
		desired := StatsEnable{Enabled: true}
		if _, err := d.Create(ctx, desired.Proto()); err != nil {
			t.Fatalf("Create (proper reply=%v): %v", proper, err)
		}
		req := v.CallsNamed("acl_stats_intf_counters_enable")
		if len(req) != 1 || !req[0].(*vppacl.ACLStatsIntfCountersEnable).Enable || !v.countersEnabled {
			t.Fatalf("enable request %+v", req)
		}
		kvs := mustRetrieve(t, d)
		if len(kvs) != 1 || kvs[0].Key != KeyStatsEnable || !proto.Equal(kvs[0].Value, desired.Proto()) {
			t.Fatalf("Retrieve = %+v", kvs)
		}
		assertEmptyPlan(t, d, kv(d, desired.Proto()))
		// enabled=false is recorded but never sent: VPP keeps the counters on
		if _, err := d.Update(ctx, desired.Proto(), StatsEnable{Enabled: false}.Proto(), nil); err != nil {
			t.Fatal(err)
		}
		if v.countersCalls != 1 || !v.countersEnabled {
			t.Fatal("descriptor must never disable the global counters")
		}
		assertEmptyPlan(t, d, kv(d, StatsEnable{Enabled: false}.Proto()))
		if err := d.Delete(ctx, desired.Proto(), nil); err != nil || v.countersCalls != 1 || !v.countersEnabled {
			t.Fatalf("Delete must be a no-op on VPP: %v calls=%d", err, v.countersCalls)
		}
		if kvs := mustRetrieve(t, d); len(kvs) != 0 {
			t.Fatalf("after Delete: %+v", kvs)
		}
	}
	// the request itself is exercised through EnableCounters; a bad retval surfaces
	v := newFakeVPP()
	v.Reply("acl_stats_intf_counters_enable", &vppacl.ACLDelReply{Retval: rvInvalidValue})
	if err := EnableCounters(ctx, v); err == nil {
		t.Fatal("retval must surface")
	}
	v.Reply("acl_stats_intf_counters_enable", &vppacl.ACLAddReplaceReply{})
	if err := EnableCounters(ctx, v); err == nil {
		t.Fatal("unexpected reply type must be an error")
	}
	if _, err := NewStatsEnable(v).Create(ctx, ACL{Name: "not-a-stats-spec"}.Proto()); err != nil {
		t.Fatalf("a struct without 'enabled' reads as false (tolerant decode): %v", err)
	}
}

// Review finding 1: after a VPP restart the counters flag is off again; Retrieve must stop
// reporting the applied value so the scheduler re-enables it.
func TestStatsEnableSurvivesVPPRestart(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	d := NewStatsEnable(v)
	desired := StatsEnable{Enabled: true}
	if _, err := d.Create(ctx, desired.Proto()); err != nil {
		t.Fatal(err)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))

	v.restartVPP()
	kvs := mustRetrieve(t, d)
	if len(kvs) != 0 {
		t.Fatalf("after a VPP restart Retrieve must report nothing, got %+v", kvs)
	}
	if p := diffPlan([]scheduler.KV{kv(d, desired.Proto())}, kvs); len(p.Create) != 1 {
		t.Fatalf("the scheduler must plan a Create: %s", planString(p))
	}
	if _, err := d.Create(ctx, desired.Proto()); err != nil {
		t.Fatal(err)
	}
	if !v.countersEnabled || v.countersCalls != 2 {
		t.Fatalf("counters must be enabled again: enabled=%v calls=%d", v.countersEnabled, v.countersCalls)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))

	// explicit Reset (reconnect hook) forgets the value too
	d.Reset()
	if kvs := mustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("after Reset: %+v", kvs)
	}

	// the identity read failing is an error, not a silent "enabled"
	if _, err := d.Create(ctx, desired.Proto()); err != nil {
		t.Fatal(err)
	}
	v.Reply("control_ping", &memclnt.ControlPingReply{Retval: rvInvalidValue})
	if _, err := d.Retrieve(ctx); err == nil {
		t.Fatal("boot identity (control_ping) failure must surface from Retrieve")
	}
	if _, err := d.Create(ctx, desired.Proto()); err == nil {
		t.Fatal("boot identity (control_ping) failure must surface from Create")
	}
}

func TestStatsReader(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	acls := NewACL(v, owner)
	m, err := acls.Create(ctx, ACL{Name: "lan-in", Rules: sampleRules()[:3]}.Proto())
	if err != nil {
		t.Fatal(err)
	}
	idx := m.(Meta).ACLIndex
	stats := newFakeStats()
	// two workers, 3 rules + VPP's spare slot
	stats.set(StatsPath(idx), adapter.CombinedCounterStat{
		{{10, 1000}, {0, 0}, {5, 500}, {0, 0}},
		{{1, 100}, {2, 200}, {0, 0}, {0, 0}},
	})
	stats.set("/acl/77/matches", adapter.CombinedCounterStat{{{99, 99}}}) // someone else's ACL
	stats.set("/err/acl-plugin-in-ip4-fa/ACL deny packets", nil)          // not a matches vector

	r := NewStatsReader(stats, v, owner)
	raw, err := r.ReadIndex(idx)
	if err != nil {
		t.Fatal(err)
	}
	want := []RuleCounter{{11, 1100}, {2, 200}, {5, 500}, {0, 0}}
	if len(raw) != 4 || raw[0] != want[0] || raw[1] != want[1] || raw[2] != want[2] {
		t.Fatalf("ReadIndex = %+v, want %+v", raw, want)
	}
	owned, err := r.ReadOwned(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[0].Name != "lan-in" || owned[0].ACLIndex != idx || len(owned[0].Rules) != 3 || owned[0].Rules[0] != want[0] {
		t.Fatalf("ReadOwned = %+v", owned)
	}
	paths, err := r.ListPaths()
	if err != nil || len(paths) != 2 || paths[0] != StatsPath(idx) || paths[1] != "/acl/77/matches" {
		t.Fatalf("ListPaths = %v, %v", paths, err)
	}
	if _, err := r.ReadIndex(12345); !errors.Is(err, ErrNoCounters) {
		t.Fatalf("missing vector: %v", err)
	}
	// an owned ACL without a vector yet reads as zeros with the right shape
	if _, err := acls.Create(ctx, ACL{Name: "new", Rules: sampleRules()}.Proto()); err != nil {
		t.Fatal(err)
	}
	owned, err = r.ReadOwned(ctx)
	if err != nil || len(owned) != 2 || len(owned[1].Rules) != len(sampleRules()) || owned[1].Rules[0] != (RuleCounter{}) {
		t.Fatalf("zero-fill: %+v, %v", owned, err)
	}
	stats.fail = errors.New("stats socket gone")
	if _, err := r.ReadOwned(ctx); err == nil {
		t.Fatal("stats errors must surface")
	}
	if StatsPath(3) != "/acl/3/matches" {
		t.Fatal(StatsPath(3))
	}
}

func TestPluginInfo(t *testing.T) {
	info, err := GetPluginInfo(t.Context(), newFakeVPP())
	if err != nil || info.Major != 1 || info.ConnTableMaxEntries != 1<<20 {
		t.Fatalf("%+v %v", info, err)
	}
	if info.String() == "" {
		t.Fatal("String")
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, newFakeVPP(), owner)
	want := []string{NameACL, NameMacipACL, NameInterfaceBinding, NameEtypeWhitelist, NameMacipInterfaceBinding, NameStatsEnable}
	got := reg.Names()
	if len(got) != len(want) {
		t.Fatalf("registered %v", got)
	}
	for i := range want {
		if got[i] != want[i] || !scheduler.ValidName(want[i]) {
			t.Fatalf("registered %v, want %v", got, want)
		}
	}
	for _, k := range []scheduler.Key{KeyACL("a"), KeyMacipACL("m"), KeyInterfaceBinding("i"), KeyEtypeWhitelist("i"), KeyMacipBinding("i"), KeyStatsEnable} {
		if _, ok := reg.ForKey(k); !ok {
			t.Fatalf("key %s does not route to a descriptor", k)
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("registering twice must panic (duplicate name)")
		}
	}()
	Register(reg, newFakeVPP(), owner)
}
