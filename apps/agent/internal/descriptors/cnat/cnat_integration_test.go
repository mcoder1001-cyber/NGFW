package cnat_test

import (
	"context"
	"errors"
	"testing"

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
