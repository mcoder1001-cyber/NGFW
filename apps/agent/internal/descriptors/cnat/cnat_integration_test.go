package cnat_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.fd.io/govpp/api"

	"google.golang.org/protobuf/types/known/structpb"

	cnatapi "ngfw/agent/binapi/cnat"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/cnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestCnatOnHost: one integration check per cnat object type on the host VPP. Every address
// is in the slot block (VIP 10.<N>.47.1, backends 10.<N>.48.x, SNAT 10.<N>.49.1, excluded
// 10.<N>.50.0/24); features only on this slot's loopbacks. The default SNAT entry and the
// policy are VPP globals (D-071): the slot only requires them; the entry is a test fixture.
func TestCnatOnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := cnat.New(c, vpptest.Prefix(t))

	// translations (no SNAT entry needed; the dependency is optional)
	tr := natcommon.MustEncode(&cnat.TranslationSpec{VIP: nattest.Addr4(t, 47, 1), Port: 80, Proto: "tcp",
		Paths: []cnat.PathSpec{{Dst: nattest.Addr4(t, 48, 1), DstPort: 8080}, {Dst: nattest.Addr4(t, 48, 2), DstPort: 8080}}})
	tr2 := natcommon.MustEncode(&cnat.TranslationSpec{VIP: nattest.Addr4(t, 47, 1), Port: 53, Proto: "udp", LBType: cnat.LBMaglev,
		Paths: []cnat.PathSpec{{Dst: nattest.Addr4(t, 48, 3), DstPort: 5353}}})
	nattest.CreateAll(ctx, t, p.Translation, tr, tr2)
	nattest.AssertPlan(t, p.Translation, tr, tr2)
	// backend change converges in place
	tr2b := natcommon.MustEncode(&cnat.TranslationSpec{VIP: nattest.Addr4(t, 47, 1), Port: 53, Proto: "udp", LBType: cnat.LBMaglev,
		Paths: []cnat.PathSpec{{Dst: nattest.Addr4(t, 48, 3), DstPort: 5353}, {Dst: nattest.Addr4(t, 48, 4), DstPort: 5353, NoNAT: true}}})
	if n := nattest.Apply(t, p.Translation, tr, tr2b); n != 1 {
		t.Fatalf("translation update planned %d ops", n)
	}
	nattest.AssertPlan(t, p.Translation, tr, tr2b)

	// per-interface cnat feature
	l0, _ := nattest.Loopback(t, c, 40)
	l1, _ := nattest.Loopback(t, c, 41)
	feat := natcommon.MustEncode(&cnat.InterfaceFeatureSpec{Interface: l0})
	nattest.CreateAll(ctx, t, p.InterfaceFeature, feat)
	nattest.AssertPlan(t, p.InterfaceFeature, feat)

	if sess, err := p.Sessions(ctx, 0, 100); err != nil {
		t.Fatalf("session dump: %v", err)
	} else {
		t.Logf("cnat sessions: %d (shape only)", len(sess))
	}

	// default SNAT entry and its dependents — only when nobody else holds it
	// default SNAT entry: a VPP global (D-071). The slot is not the globals owner, so the test
	// creates it as a FIXTURE (raw API, slot addresses, under the exclusive host-wide cnat lock)
	// only when none exists, and removes it again at the end (previous state: absent).
	api := cnatapi.NewServiceClient(c)
	if cur, err := api.CnatGetSnatAddresses(ctx, &cnatapi.CnatGetSnatAddresses{}); err == nil {
		t.Logf("cnat default SNAT entry held by another owner (%+v): skipping the snat-* checks", cur)
	} else {
		ip4, _ := natcommon.IP4(nattest.Addr4(t, 49, 1))
		ip6, _ := natcommon.IP6(nattest.Addr6(t, 0x4901))
		setSnat := func(add bool) error {
			unlock, err := natcommon.HostLock("/run/lock", "cnat", true)
			if err != nil {
				return err
			}
			defer unlock()
			req := &cnatapi.CnatSetSnatAddresses{SwIfIndex: ^interface_types.InterfaceIndex(0)}
			if add {
				req.SnatIP4, req.SnatIP6 = ip4, ip6
			}
			_, err = api.CnatSetSnatAddresses(context.Background(), req)
			return err
		}
		if err := setSnat(true); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			// remove the fixture only while it is still ours (same addresses)
			if cur, err := api.CnatGetSnatAddresses(context.Background(), &cnatapi.CnatGetSnatAddresses{}); err == nil && cur.SnatIP4 == ip4 {
				if err := setSnat(false); err != nil {
					t.Errorf("fixture: remove default SNAT entry: %v", err)
				}
			}
		})
		snat := natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: nattest.Addr4(t, 49, 1), IP6: nattest.Addr6(t, 0x4901)})
		if _, err := p.SnatAddresses.Create(ctx, snat); err != nil {
			t.Fatalf("snat-addresses requirement: %v", err)
		}
		if _, err := p.SnatAddresses.Create(ctx, natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: nattest.Addr4(t, 49, 2)})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
			t.Fatalf("snat-addresses requirement (other) must fail: %v", err)
		}
		if err := p.SnatAddresses.Delete(ctx, snat, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := api.CnatGetSnatAddresses(ctx, &cnatapi.CnatGetSnatAddresses{}); err != nil {
			t.Fatal("a non-owner's Delete removed the default SNAT entry")
		}
		nattest.AssertWriteOnly(t, p.SnatAddresses)
		pol := natcommon.MustEncode(&cnat.SnatPolicySpec{Policy: cnat.PolicyIfPfx})
		if _, err := p.SnatPolicy.Create(ctx, pol); err != nil { // unobservable: accepted, nothing sent
			t.Fatal(err)
		}

		// snat-interface / snat-exclude-prefix: per-object, write-only (D-063); re-applied
		// twice (D-076: the excluded-prefix add is skipped on the same VPP process)
		sif := natcommon.MustEncode(&cnat.SnatInterfaceSpec{Interface: l1, Table: cnat.TableIncludeV4})
		ex := natcommon.MustEncode(&cnat.SnatExcludePrefixSpec{Prefix: nattest.Addr4(t, 50, 0) + "/24"})
		for _, w := range []struct {
			d   scheduler.Descriptor
			obj *structpb.Struct
		}{{p.SnatInterface, sif}, {p.SnatExcludePfx, ex}} {
			nattest.CreateWriteOnly(ctx, t, w.d, w.obj)
			nattest.AssertWriteOnly(t, w.d)
		}

		nattest.Pause(t, "cnat") // evidence hook (VRX_EVIDENCE_DIR), no-op otherwise
		for _, w := range []struct {
			d   scheduler.Descriptor
			obj *structpb.Struct
		}{{p.SnatExcludePfx, ex}, {p.SnatInterface, sif}} {
			if err := w.d.Delete(ctx, w.obj, nil); err != nil {
				t.Fatalf("%s delete: %v", w.d.Name(), err)
			}
		}
	}

	nattest.DeleteAll(ctx, t, p.InterfaceFeature)
	nattest.DeleteAll(ctx, t, p.Translation)
}

// countingConn counts cnat_snat_policy_add_del_exclude_pfx adds sent through it.
type countingConn struct {
	*nattest.Conn
	mu   sync.Mutex
	adds int
}

func (c *countingConn) Invoke(ctx context.Context, req, reply api.Message) error {
	if r, ok := req.(*cnatapi.CnatSnatPolicyAddDelExcludePfx); ok && r.IsAdd == 1 {
		c.mu.Lock()
		c.adds++
		c.mu.Unlock()
	}
	return c.Conn.Invoke(ctx, req, reply)
}

func (c *countingConn) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.adds
}

// TestCnatExcludeReaddOnHost is the re-review N2 host regression: the excluded prefix is added
// once across resyncs; after the default SNAT entry is deleted and recreated on the SAME VPP
// process (VPP drops the prefixes with the entry) the next resync re-adds it exactly once.
// The entry is this test's own fixture (created only when none exists), recreated through the
// globals-owner descriptor of the same plugin instance — the only legitimate mutator (D-071) —
// and once more behind the agent's back with other addresses (observable fingerprint).
func TestCnatExcludeReaddOnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	api0 := cnatapi.NewServiceClient(c)
	if cur, err := api0.CnatGetSnatAddresses(ctx, &cnatapi.CnatGetSnatAddresses{}); err == nil {
		t.Skipf("cnat default SNAT entry held by another owner (%+v)", cur)
	}
	cc := &countingConn{Conn: c}
	// D-DF3-12-style exception: owner mode only for the entry this test creates as its fixture
	p := cnat.New(cc, vpptest.Prefix(t), natcommon.WithGlobalsOwner(true))
	entry := natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: nattest.Addr4(t, 49, 1)})
	if _, err := p.SnatAddresses.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.SnatAddresses.Delete(context.Background(), entry, nil) })
	ex := natcommon.MustEncode(&cnat.SnatExcludePrefixSpec{Prefix: nattest.Addr4(t, 50, 0) + "/24"})
	t.Cleanup(func() { _ = p.SnatExcludePfx.Delete(context.Background(), ex, nil) })
	resync := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			if _, err := p.SnatExcludePfx.Create(ctx, ex); err != nil {
				t.Fatal(err)
			}
		}
	}
	resync(3)
	if cc.count() != 1 {
		t.Fatalf("3 resyncs sent %d adds, want 1", cc.count())
	}
	t.Logf("N2: 3 resyncs → %d add", cc.count())
	// the reviewer's repro: delete + recreate the entry on the same VPP process
	if err := p.SnatAddresses.Delete(ctx, entry, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SnatAddresses.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	resync(3)
	if cc.count() != 2 {
		t.Fatalf("after the entry was recreated: %d adds, want 2 (re-added exactly once)", cc.count())
	}
	t.Logf("N2: entry deleted+recreated (same VPP) → 3 resyncs → re-added once (adds %d)", cc.count())
	// recreated behind the agent's back with other addresses (raw API, exclusive cnat lock)
	unlock, err := natcommon.HostLock("/run/lock", "cnat", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err1 := api0.CnatSetSnatAddresses(ctx, &cnatapi.CnatSetSnatAddresses{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	ip4, _ := natcommon.IP4(nattest.Addr4(t, 49, 2))
	_, err2 := api0.CnatSetSnatAddresses(ctx, &cnatapi.CnatSetSnatAddresses{SnatIP4: ip4, SwIfIndex: ^interface_types.InterfaceIndex(0)})
	unlock()
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	t.Cleanup(func() { // remove the raw entry if it is still the one we made
		if cur, err := api0.CnatGetSnatAddresses(context.Background(), &cnatapi.CnatGetSnatAddresses{}); err == nil && cur.SnatIP4 == ip4 {
			_, _ = api0.CnatSetSnatAddresses(context.Background(), &cnatapi.CnatSetSnatAddresses{SwIfIndex: ^interface_types.InterfaceIndex(0)})
		}
	})
	resync(2)
	if cc.count() != 3 {
		t.Fatalf("after an external recreate: %d adds, want 3", cc.count())
	}
	t.Logf("N2: entry recreated externally with other addresses → re-added once (adds %d)", cc.count())
	nattest.Pause(t, "cnat-n2") // evidence hook
}
