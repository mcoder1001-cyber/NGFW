package subsystems

import (
	"context"
	"log/slog"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"

	"ngfw/agent/binapi/acl_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// TD-11b's guard (dfkit/persist): every descriptor of the acl family declares exactly one of
// RecordsNoOwnership and CheckPersistent; the whitelist's claim store must be persisted.
func TestACLDescriptorsDeclareOwnership(t *testing.T) {
	type checker interface{ CheckPersistent() error }
	type none interface{ RecordsNoOwnership() }
	rt := aclstate.Open(aclstate.Config{StateDir: t.TempDir(), Owner: "w3", Client: fake.New()})
	t.Cleanup(rt.Close)
	c := fake.New()
	all := []scheduler.Descriptor{
		rt.ConfigDescriptor(), rt.Tracker().WrapACL(descacl.NewACL(c, "w3")), rt.Tracker().WrapMacip(descacl.NewMacipACL(c, "w3")),
		descacl.NewInterfaceBinding(c, "w3"), descacl.NewEtypeWhitelist(c, "w3"), descacl.NewMacipBinding(c, "w3"), descacl.NewStatsEnable(c),
	}
	for _, d := range all {
		_, ck := d.(checker)
		_, no := d.(none)
		if ck == no {
			t.Fatalf("%s (%T): CheckPersistent %v, RecordsNoOwnership %v — exactly one is required", d.Name(), d, ck, no)
		}
	}
	if err := descacl.NewEtypeWhitelist(c, "w3").CheckPersistent(); err == nil {
		t.Fatal("the in-memory whitelist claim store must be refused")
	}
	if err := descacl.NewEtypeWhitelist(c, "w3", descacl.WithEtypeClaims(persistedClaims{})).CheckPersistent(); err != nil {
		t.Fatalf("persisted store refused: %v", err)
	}
	if got := strings.Join(aclDescriptors(), ","); got != "acl.config,acl.acl,acl.macip-acl,acl.interface-binding,acl.etype-whitelist,acl.macip-interface-binding,acl.stats-enable" {
		t.Fatalf("Domains[acl] = %s", got)
	}
}

type persistedClaims struct{}

func (persistedClaims) Claim(string) error   { return nil }
func (persistedClaims) Release(string) error { return nil }
func (persistedClaims) Claimed(string) bool  { return false }
func (persistedClaims) Persistent() bool     { return true }

// The watcher compares the APPLIED expansion (tracker fingerprint → record) with now: a schedule
// that turned on or off, or a change of an FQDN object an applied rule uses, asks for one resync;
// unrelated FQDN changes do not; bursts are coalesced; without the hook it only warns.
func TestACLWatcher(t *testing.T) {
	rec := aclstate.NewRecord(0)
	rt := aclstate.Open(aclstate.Config{StateDir: t.TempDir(), Owner: "w3", Client: fake.New(), Record: rec})
	t.Cleanup(rt.Close)
	rules := []descacl.Rule{{Action: descacl.ActionPermit, Src: descacl.AnyV4, Dst: descacl.AnyV4, SrcPortLast: 65535, DstPortLast: 65535}}
	fp := aclstate.Fingerprint(rules)
	office := &vrxv1.Schedule{Type: proto.String("recurring"), Days: []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}, Start: proto.String("08:00"), End: proto.String("18:00")}
	cfg := &vrxv1.AclList{Description: proto.String("applied")}
	if _, err := rt.ConfigDescriptor().Create(context.Background(), aclstate.ConfigList("l", cfg)); err != nil {
		t.Fatal(err)
	}
	rec.PutACL(&aclstate.Expansion{Name: "l", Fingerprint: fp, ConfigHash: aclstate.ConfigHash(cfg), VPPRules: 1, Schedules: map[string]*vrxv1.Schedule{"office": office}, Rules: []aclstate.RuleInfo{
		{Sequence: 10, Status: vrxv1.AclRuleStatus_ACL_RULE_STATUS_APPLIED, Count: 1, Schedule: "office"},
		{Sequence: 20, Status: vrxv1.AclRuleStatus_ACL_RULE_STATUS_APPLIED, Count: 0, FQDN: []string{"cdn"}},
	}})
	trackApplied(t, rt, "l", fp)

	var calls atomic.Int32
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) // inside office hours: rule 10 applied = consistent
	w := newACLWatcher(rt, func() { calls.Add(1) }, true, slog.Default(), func() time.Time { return now }, time.UTC)
	if reason, ok := w.scheduleChanged(now); ok {
		t.Fatalf("consistent state reported: %s", reason)
	}
	now = time.Date(2026, 9, 25, 19, 0, 0, 0, time.UTC) // after 18:00: rule 10 must go
	reason, ok := w.scheduleChanged(now)
	if !ok || !strings.Contains(reason, `schedule "office" turned off`) {
		t.Fatalf("schedule change not seen: %q", reason)
	}
	w.check()
	waitFor(t, func() bool { return calls.Load() == 1 })

	// FQDN: an unrelated object does nothing; the referenced one asks (after the coalescing gap)
	w.fqdnChanged(objects.Change{Host: "x.w3.test", Objects: []string{"other"}})
	w.fqdnChanged(objects.Change{Host: "cdn.w3.test", Objects: []string{"cdn"}, Addresses: []netip.Addr{netip.MustParseAddr("192.0.2.9")}})
	w.fqdnChanged(objects.Change{Host: "cdn.w3.test", Objects: []string{"cdn"}})
	if w.pending == nil {
		t.Fatal("a request within the gap must be delayed, not dropped")
	}
	w.close()
	if calls.Load() != 1 {
		t.Fatalf("calls %d", calls.Load())
	}

	// no hook: warn, never call
	var none atomic.Int32
	w2 := newACLWatcher(rt, func() { none.Add(1) }, false, slog.Default(), func() time.Time { return now }, time.UTC)
	w2.check()
	w2.fqdnChanged(objects.Change{Objects: []string{"cdn"}})
	time.Sleep(20 * time.Millisecond)
	if none.Load() != 0 || w2.Requested() != 0 || !w2.warned {
		t.Fatalf("unhooked watcher called the hook: %d", none.Load())
	}
}

// trackApplied makes the tracker see list name (one permit-any rule, fingerprint fp) in VPP: the
// wrapped descriptor's Retrieve on the fake VPP fills it.
func trackApplied(t *testing.T, rt *aclstate.Runtime, name, fp string) {
	t.Helper()
	v := coretest.New()
	v.ACL().AddACL("w3:"+name, acl_types.ACLRule{
		IsPermit:              acl_types.ACL_ACTION_API_PERMIT,
		SrcPrefix:             ip_types.Prefix{Address: ip_types.Address{Af: ip_types.ADDRESS_IP4}},
		DstPrefix:             ip_types.Prefix{Address: ip_types.Address{Af: ip_types.ADDRESS_IP4}},
		SrcportOrIcmptypeLast: 65535, DstportOrIcmpcodeLast: 65535,
	})
	d := rt.Tracker().WrapACL(descacl.NewACL(v, "w3"))
	if _, err := d.Retrieve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, ok := rt.Tracker().ACL(name); !ok || a.Fingerprint != fp {
		t.Fatalf("tracked %+v", a)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// registerACL in the product wiring: the six descriptors, the projection environment (owner,
// globals flag), the runtime for RPCs, the watcher stopped by Wiring.Close.
func TestRegisterACLWiring(t *testing.T) {
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "w3")
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	t.Setenv(EnvDNSServers, "127.0.0.1:9")
	w, err := Register(reg, Env{Client: fake.New(), Owner: "w3", StateDir: dir, Owned: owned, GlobalsOwner: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range aclDescriptors() {
		if _, ok := reg.Get(n); !ok {
			t.Fatalf("%s not registered", n)
		}
	}
	if w.ACLRuntime() == nil {
		t.Fatal("no acl runtime")
	}
	if !strings.Contains(strings.Join(ImplementedDomains(), ","), "acl") {
		t.Fatal("acl is not an implemented domain")
	}
	w.Close()
	if w.ACLRuntime() != nil {
		t.Fatal("runtime still registered after Close")
	}
	if o := objects.RuntimeFor(dir, "w3"); o != nil {
		o.Close()
	}
}
