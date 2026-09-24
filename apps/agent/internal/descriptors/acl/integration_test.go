package acl

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/adapter/statsclient"
	"go.fd.io/govpp/core"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration test against the VPP on this host (docs/lab/shared-host-rules.md): VRX_INTEGRATION=1,
// shared lab lock, every object tagged "<VRX_TEST_PREFIX>:…", attach points are this slot's
// loopbacks, cleanup in t.Cleanup (unbind before delete). Assertions filter by owner — other
// workers' ACLs are on the same VPP at the same time.
//
// The global counters switch (acl.stats-enable) has no getter. It is off on this host (nothing on
// main enables it), so the test switches it on for the stats subtest and restores it to
// disabled in Cleanup. Set VRX_ACL_STATS_KEEP=1 to leave it on (e.g. when the operator knows
// another consumer needs it).

// holdForEvidence pauses when VRX_ACL_EVIDENCE_HOLD (a duration) is set, so an operator can
// capture the VPP CLI `show acl-plugin …` output for the status report while the objects exist.
func holdForEvidence(t *testing.T) {
	t.Helper()
	if v := os.Getenv("VRX_ACL_EVIDENCE_HOLD"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("VRX_ACL_EVIDENCE_HOLD=%q: %v", v, err)
		}
		time.Sleep(d)
	}
}

// hostClient adapts *core.Connection to vpp.Client until P05's client is merged.
type hostClient struct{ *core.Connection }

func (hostClient) Connected() bool { return true }

func connectHost(t *testing.T) vpp.Client {
	t.Helper()
	conn, err := core.Connect(socketclient.NewVppClient(socketclient.DefaultSocketName))
	if err != nil {
		t.Fatalf("connect %s: %v", socketclient.DefaultSocketName, err)
	}
	t.Cleanup(conn.Disconnect)
	return hostClient{conn}
}

func connectStats(t *testing.T) *statsclient.StatsClient {
	t.Helper()
	sc := statsclient.NewStatsClient(statsclient.DefaultSocketName)
	if err := sc.Connect(); err != nil {
		t.Fatalf("connect %s: %v", statsclient.DefaultSocketName, err)
	}
	t.Cleanup(func() { _ = sc.Disconnect() })
	return sc
}

// createLoopback creates this slot's loopback number i (loop<slot><ii>), tags it with the owner
// (unless tagged is false: an untagged interface stands in for a physical port) and deletes it
// in Cleanup. A leftover of the same name from an earlier failed run is removed
// first (same slot ⇒ ours).
func createLoopback(ctx context.Context, t *testing.T, c vpp.Client, owner string, i int, tagged bool) string {
	t.Helper()
	inst := vpptest.LoopbackInstance(t, i)
	name := fmt.Sprintf("loop%d", inst)
	svc := interfaces.NewServiceClient(c)
	if ifaces, err := dumpInterfaces(ctx, c, owner); err == nil {
		if old, ok := ifaces.byName[name]; ok {
			t.Logf("leftover %s (sw_if_index %d, tag %q) from an earlier run: deleting", name, old.Index, old.Tag)
			_ = ifsanitize.BeforeDelete(ctx, c, uint32(interface_types.InterfaceIndex(old.Index)), "test cleanup") // D-095 c: bindings go before the interface (V19)
			if _, err := svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(old.Index)}); err != nil {
				t.Fatalf("delete leftover %s: %v", name, err)
			}
		}
	}
	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatalf("create_loopback_instance %d: %v", inst, err)
	}
	t.Cleanup(func() {
		_ = ifsanitize.BeforeDelete(context.Background(), c, uint32(rep.SwIfIndex), "test cleanup") // D-095 c: bindings go before the interface (V19)
		if _, err := svc.DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: rep.SwIfIndex}); err != nil {
			t.Errorf("cleanup delete_loopback %s: %v", name, err)
		}
	})
	if !tagged {
		t.Logf("created %s sw_if_index %d (untagged)", name, rep.SwIfIndex)
		return name
	}
	tag, err := vpp.OwnerTag(owner, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
		t.Fatalf("sw_interface_tag_add_del %s: %v", name, err)
	}
	t.Logf("created %s sw_if_index %d tag %q", name, rep.SwIfIndex, tag)
	return name
}

// deleteOwned deletes everything the descriptors report as ours, in the order given (bindings
// before ACLs). Used before the test (leftovers of a failed run) and as the safety net after.
func deleteOwned(ctx context.Context, t *testing.T, descs ...scheduler.Descriptor) {
	t.Helper()
	for _, d := range descs {
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			t.Errorf("%s Retrieve: %v", d.Name(), err)
			continue
		}
		for _, kv := range kvs {
			t.Logf("deleting %s (meta %+v)", kv.Key, kv.Meta)
			if err := d.Delete(ctx, kv.Value, kv.Meta); err != nil {
				t.Errorf("delete %s: %v", kv.Key, err)
			}
		}
	}
}

func findKV(kvs []scheduler.KV, key scheduler.Key) (scheduler.KV, bool) {
	for _, kv := range kvs {
		if kv.Key == key {
			return kv, true
		}
	}
	return scheduler.KV{}, false
}

// assertRetrieved checks that d reports exactly our desired objects (and nothing else of ours).
func assertRetrieved(t *testing.T, d scheduler.Descriptor, desired ...scheduler.KV) []scheduler.KV {
	t.Helper()
	actual := mustRetrieve(t, d)
	if len(actual) != len(desired) {
		t.Fatalf("%s: Retrieve returned %d owned object(s), want %d: %+v", d.Name(), len(actual), len(desired), actual)
	}
	for _, want := range desired {
		got, ok := findKV(actual, want.Key)
		if !ok {
			t.Fatalf("%s: %s missing from Retrieve", d.Name(), want.Key)
		}
		if !proto.Equal(got.Value, want.Value) {
			t.Fatalf("%s: %s differs:\n got  %v\n want %v", d.Name(), want.Key, got.Value, want.Value)
		}
	}
	p := diffPlan(desired, actual)
	t.Logf("%s: Retrieve == desired for %d object(s); re-apply plan:\n%s", d.Name(), len(desired), planString(p))
	if !p.Empty() {
		t.Fatalf("%s: re-apply must plan nothing", d.Name())
	}
	return actual
}

func TestACLPluginOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c := connectHost(t)
	stats := connectStats(t)

	info, err := GetPluginInfo(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("host: %s", info)

	aclD := NewACL(c, owner)
	macipD := NewMacipACL(c, owner)
	bindD := NewInterfaceBinding(c, owner)
	etypeD := NewEtypeWhitelist(c, owner)
	mbindD := NewMacipBinding(c, owner)
	statsD := NewStatsEnable(c)
	reader := NewStatsReader(stats, c, owner)

	deleteOwned(ctx, t, bindD, etypeD, mbindD, aclD, macipD) // leftovers of an earlier failed run
	if os.Getenv("VRX_ACL_STATS_KEEP") != "1" {
		t.Cleanup(func() {
			if err := setCounters(context.Background(), c, false); err != nil {
				t.Errorf("restore counters flag: %v", err)
			} else {
				t.Log("restored acl stats counters flag to disabled (set VRX_ACL_STATS_KEEP=1 to keep it on)")
			}
		})
	} else {
		t.Log("VRX_ACL_STATS_KEEP=1: acl stats counters flag stays enabled after the test")
	}
	ifA := createLoopback(ctx, t, c, owner, 40, true)
	ifB := createLoopback(ctx, t, c, owner, 41, true)
	ifU := createLoopback(ctx, t, c, owner, 43, false) // untagged, like a physical port
	foreignOwner := owner + "f"                        // a second owner on the same VPP (tag "w10f:…")
	foreignACL := NewACL(c, foreignOwner)
	foreignBind := NewInterfaceBinding(c, foreignOwner)
	deleteOwned(ctx, t, foreignBind, foreignACL)
	t.Cleanup(func() { deleteOwned(context.Background(), t, bindD, etypeD, mbindD, aclD, macipD) })
	t.Cleanup(func() { deleteOwned(context.Background(), t, foreignBind, foreignACL) })

	lanIn := ACL{Name: "t-lan-in", Rules: sampleRules()}
	big := ACL{Name: "t-big50", Rules: manyRules(50)}
	var lanMeta, bigMeta any
	ok := t.Run("acl", func(t *testing.T) {
		var err error
		lanMeta, err = aclD.Create(ctx, lanIn.Proto())
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("created %s → %+v", KeyACL(lanIn.Name), lanMeta)
		assertRetrieved(t, aclD, kv(aclD, lanIn.Proto()))
		got, _ := findKV(mustRetrieve(t, aclD), KeyACL(lanIn.Name))
		if got.Meta != lanMeta {
			t.Fatalf("Retrieve meta %+v != Create meta %+v", got.Meta, lanMeta)
		}
		// update in place: reordered + one more rule, same index
		lanIn = ACL{Name: lanIn.Name, Rules: append([]Rule{lanIn.Rules[2], lanIn.Rules[0]}, lanIn.Rules[3:]...)}
		m2, err := aclD.Update(ctx, ACL{Name: lanIn.Name, Rules: sampleRules()}.Proto(), lanIn.Proto(), lanMeta)
		if err != nil || m2 != lanMeta {
			t.Fatalf("Update: %v (meta %+v → %+v)", err, lanMeta, m2)
		}
		assertRetrieved(t, aclD, kv(aclD, lanIn.Proto()))
		t.Logf("update kept acl_index %d", lanMeta.(Meta).ACLIndex)
	})
	if !ok {
		t.FailNow()
	}
	t.Run("acl-50-rules", func(t *testing.T) {
		var err error
		bigMeta, err = aclD.Create(ctx, big.Proto())
		if err != nil {
			t.Fatal(err)
		}
		actual := assertRetrieved(t, aclD, kv(aclD, lanIn.Proto()), kv(aclD, big.Proto()))
		got, _ := findKV(actual, KeyACL(big.Name))
		back, _ := FromProto(got.Value)
		t.Logf("50-rule ACL %+v: %d rules retrieved byte-identical", bigMeta, len(back.Rules))
	})
	if bigMeta == nil {
		t.FailNow()
	}
	binding := InterfaceBinding{Interface: ifA, Input: []string{lanIn.Name, big.Name}, Output: []string{lanIn.Name}}
	var bindMeta any
	t.Run("interface-binding", func(t *testing.T) {
		var err error
		bindMeta, err = bindD.Create(ctx, binding.Proto())
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("bound %v → %+v", binding, bindMeta)
		assertRetrieved(t, bindD, kv(bindD, binding.Proto()))
		reordered := InterfaceBinding{Interface: ifA, Input: []string{big.Name, lanIn.Name}, Output: []string{lanIn.Name}}
		if p := diffPlan([]scheduler.KV{kv(bindD, reordered.Proto())}, mustRetrieve(t, bindD)); len(p.Update) != 1 || len(p.Create) != 0 {
			t.Fatalf("reorder must plan an update, not a recreate:\n%s", planString(p))
		}
		if _, err := bindD.Update(ctx, binding.Proto(), reordered.Proto(), bindMeta); err != nil {
			t.Fatal(err)
		}
		binding = reordered
		assertRetrieved(t, bindD, kv(bindD, binding.Proto()))
		// update a bound ACL in place: index kept, binding unchanged
		updated := ACL{Name: lanIn.Name, Rules: append(append([]Rule{}, lanIn.Rules...), sampleRules()[0])}
		if m, err := aclD.Update(ctx, lanIn.Proto(), updated.Proto(), lanMeta); err != nil || m != lanMeta {
			t.Fatalf("Update of a bound ACL: %v (meta %+v)", err, m)
		}
		lanIn = updated
		assertRetrieved(t, aclD, kv(aclD, lanIn.Proto()), kv(aclD, big.Proto()))
		assertRetrieved(t, bindD, kv(bindD, binding.Proto()))
		t.Logf("bound ACL %s updated in place (%d rules), binding unchanged", lanIn.Name, len(lanIn.Rules))
		// the ACL cannot go while bound
		if err := aclD.Delete(ctx, lanIn.Proto(), lanMeta); err == nil {
			t.Fatal("acl_del of a bound ACL must fail (ACL_IN_USE)")
		} else {
			t.Logf("acl_del while bound refused as expected: %v", err)
		}
	})
	holdForEvidence(t)
	t.Run("etype-whitelist", func(t *testing.T) {
		w := EtypeWhitelist{Interface: ifA, Input: []uint16{0x0806, 0x88cc}, Output: []uint16{0x0806}}
		meta, err := etypeD.Create(ctx, w.Proto())
		if err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, etypeD, kv(etypeD, w.Proto()))
		w2 := EtypeWhitelist{Interface: ifA, Input: []uint16{0x88cc}, Output: []uint16{}}
		if _, err := etypeD.Update(ctx, w.Proto(), w2.Proto(), meta); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, etypeD, kv(etypeD, w2.Proto()))
		if err := etypeD.Delete(ctx, w2.Proto(), meta); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, etypeD)
	})
	t.Run("etype-whitelist-untagged", func(t *testing.T) {
		// review finding 2: an untagged interface (physical port) — visible, idempotent, deletable
		w := EtypeWhitelist{Interface: ifU, Input: []uint16{0x0806}, Output: []uint16{0x0806, 0x88cc}}
		meta, err := etypeD.Create(ctx, w.Proto())
		if err != nil {
			t.Fatal(err)
		}
		actual := assertRetrieved(t, etypeD, kv(etypeD, w.Proto()))
		if actual[0].Meta != meta {
			t.Fatalf("Retrieve meta %+v != Create meta %+v", actual[0].Meta, meta)
		}
		p := diffPlan(nil, actual)
		if len(p.Delete) != 1 {
			t.Fatalf("removing it from desired must plan a Delete:\n%s", planString(p))
		}
		if err := etypeD.Delete(ctx, p.Delete[0].Value, p.Delete[0].Meta); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, etypeD)
		t.Logf("untagged %s: whitelist created, retrieved, deleted", ifU)
	})
	t.Run("foreign-acl-preserved", func(t *testing.T) {
		// review finding 6: another owner's ACL on the same interface survives our Create/Delete
		fa := ACL{Name: "t-foreign", Rules: sampleRules()[:1]}
		faMeta, err := foreignACL.Create(ctx, fa.Proto())
		if err != nil {
			t.Fatal(err)
		}
		fb := InterfaceBinding{Interface: ifB, Input: []string{fa.Name}}
		if _, err := foreignBind.Create(ctx, fb.Proto()); err != nil {
			t.Fatal(err)
		}
		ours := InterfaceBinding{Interface: ifB, Input: []string{lanIn.Name}, Output: []string{big.Name}}
		oursMeta, err := bindD.Create(ctx, ours.Proto())
		if err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, bindD, kv(bindD, binding.Proto()), kv(bindD, ours.Proto()))
		assertRetrieved(t, foreignBind, kv(foreignBind, fb.Proto()))
		if err := bindD.Delete(ctx, ours.Proto(), oursMeta); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, bindD, kv(bindD, binding.Proto()))
		assertRetrieved(t, foreignBind, kv(foreignBind, fb.Proto()))
		t.Logf("foreign ACL %d (owner %s) kept on %s across our Create and Delete", faMeta.(Meta).ACLIndex, foreignOwner, ifB)
	})
	m1 := MacipACL{Name: "t-l2-guard", Rules: macipRules()}
	m2 := MacipACL{Name: "t-l2-alt", Rules: macipRules()[:1]}
	var m1Meta, m2Meta, mbMeta any
	t.Run("macip", func(t *testing.T) {
		var err error
		if m1Meta, err = macipD.Create(ctx, m1.Proto()); err != nil {
			t.Fatal(err)
		}
		if m2Meta, err = macipD.Create(ctx, m2.Proto()); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, macipD, kv(macipD, m1.Proto()), kv(macipD, m2.Proto()))
		mb := MacipBinding{Interface: ifB, ACL: m1.Name}
		if mbMeta, err = mbindD.Create(ctx, mb.Proto()); err != nil {
			t.Fatal(err)
		}
		t.Logf("macip acls %+v %+v; binding %+v", m1Meta, m2Meta, mbMeta)
		assertRetrieved(t, mbindD, kv(mbindD, mb.Proto()))
		mb2 := MacipBinding{Interface: ifB, ACL: m2.Name}
		if mbMeta, err = mbindD.Update(ctx, mb.Proto(), mb2.Proto(), mbMeta); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, mbindD, kv(mbindD, mb2.Proto()))
		holdForEvidence(t)
	})
	t.Run("stats", func(t *testing.T) {
		if _, err := statsD.Create(ctx, StatsEnable{Enabled: true}.Proto()); err != nil {
			t.Fatalf("enable counters (raw-stream reply handling): %v", err)
		}
		assertRetrieved(t, statsD, kv(statsD, StatsEnable{Enabled: true}.Proto()))
		id, err := bootid.Current(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if !id.Complete() {
			t.Fatalf("boot identity %v incomplete on the host", id)
		}
		t.Logf("acl.stats-enable applied to VPP boot identity (D-080 boot_id/vpe_pid/starttime) %s", id)
		paths, err := reader.ListPaths()
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range []any{lanMeta, bigMeta} {
			want := StatsPath(m.(Meta).ACLIndex)
			found := false
			for _, p := range paths {
				found = found || p == want
			}
			if !found {
				t.Fatalf("%s not in the stats segment: %v", want, paths)
			}
		}
		t.Logf("stats segment ACL vectors (all owners): %v", paths)
		raw, err := reader.ReadIndex(bigMeta.(Meta).ACLIndex)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("raw vector %s has %d slots (rules %d + VPP's spare)", StatsPath(bigMeta.(Meta).ACLIndex), len(raw), len(big.Rules))
		owned, err := reader.ReadOwned(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(owned) != 2 {
			t.Fatalf("ReadOwned = %d ACLs, want 2", len(owned))
		}
		for _, o := range owned {
			wantRules := len(lanIn.Rules)
			if o.Name == big.Name {
				wantRules = len(big.Rules)
			}
			if len(o.Rules) != wantRules {
				t.Fatalf("%s: %d counters, want %d", o.Name, len(o.Rules), wantRules)
			}
			for i, rc := range o.Rules {
				if rc != (RuleCounter{}) {
					t.Fatalf("%s rule %d: %+v, want zeros (no traffic on this host)", o.Name, i, rc)
				}
			}
			t.Logf("counters %s (acl %d): %d rules, all zero", o.Name, o.ACLIndex, len(o.Rules))
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := bindD.Delete(ctx, binding.Proto(), bindMeta); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, bindD)
		if mbMeta != nil {
			if err := mbindD.Delete(ctx, MacipBinding{Interface: ifB, ACL: m2.Name}.Proto(), mbMeta); err != nil {
				t.Fatal(err)
			}
			assertRetrieved(t, mbindD)
		}
		if err := aclD.Delete(ctx, lanIn.Proto(), lanMeta); err != nil {
			t.Fatal(err)
		}
		if err := aclD.Delete(ctx, big.Proto(), bigMeta); err != nil {
			t.Fatal(err)
		}
		assertRetrieved(t, aclD)
		for _, x := range []struct {
			a    MacipACL
			meta any
		}{{m1, m1Meta}, {m2, m2Meta}} {
			if x.meta == nil {
				continue
			}
			if err := macipD.Delete(ctx, x.a.Proto(), x.meta); err != nil {
				t.Fatal(err)
			}
		}
		assertRetrieved(t, macipD)
		assertRetrieved(t, etypeD)
		t.Log("after delete: Retrieve shows nothing of ours for acl, macip-acl, interface-binding, macip-interface-binding, etype-whitelist")
	})
	t.Run("macip-del-unbinds", func(t *testing.T) {
		// VPP semantics worth pinning: macip_acl_del of a bound MACIP ACL succeeds and unapplies it
		// (acl.c macip_acl_del_list), unlike acl_del which fails with ACL_IN_USE_*.
		m3 := MacipACL{Name: "t-l2-tmp", Rules: macipRules()}
		meta, err := macipD.Create(ctx, m3.Proto())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mbindD.Create(ctx, MacipBinding{Interface: ifB, ACL: m3.Name}.Proto()); err != nil {
			t.Fatal(err)
		}
		if err := macipD.Delete(ctx, m3.Proto(), meta); err != nil {
			t.Fatalf("macip_acl_del while bound: %v", err)
		}
		assertRetrieved(t, mbindD)
		assertRetrieved(t, macipD)
	})
}
