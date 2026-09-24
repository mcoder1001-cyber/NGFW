package ipsec_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/internal/descriptors/dfkit"
	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
)

type countAcker struct{ n int }

func (a *countAcker) AckRestart(context.Context) error { a.n++; return nil }

var charonIDs = vpn.IDRange{Lo: 4500, Hi: 4599}

// sweepCfg: descriptors own 4000–4499 with a persisted store; charon owns 4500–4599.
func sweepCfg(t *testing.T, v *fakeVPP) ipsecd.Config {
	t.Helper()
	store, err := dfkit.NewFileBootStore(filepath.Join(t.TempDir(), "records.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := newCfg(v)
	cfg.IDs, cfg.Boot = vpn.IDRange{Lo: 4000, Hi: 4499}, store
	return cfg
}

// charonSA installs an SA the way kernel-vpp would (no record, one lock).
func charonSA(v *fakeVPP, id uint32) {
	v.sas[id] = ipsec_types.IpsecSadEntryV4{SadID: id, Spi: 7000 + id, Protocol: ipsec_types.IPSEC_API_PROTO_ESP}
	v.saLocks[id] = 1
}

// charonPolicy installs a protect policy (holding an SA lock) into spd.
func charonPolicy(v *fakeVPP, spd uint32, prio int32, sa uint32) {
	if _, ok := v.spds[spd]; !ok {
		v.spds[spd] = 900 + spd
	}
	v.policies[spd] = append(v.policies[spd], ipsecSpdEntryV2Alias{SpdID: spd, Priority: prio, SaID: sa,
		Policy: ipsec_types.IPSEC_API_SPD_ACTION_PROTECT, Protocol: 255})
	v.saLocks[sa]++
}

func TestCharonSweeperValidation(t *testing.T) {
	v := newFakeVPP()
	good := sweepCfg(t, v)
	if _, err := ipsecd.NewCharonSweeper(good, charonIDs); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		mut  func(*ipsecd.Config)
		ids  vpn.IDRange
		want error
	}{
		"no charon range":     {func(*ipsecd.Config) {}, vpn.IDRange{}, ipsecd.ErrNoSweepRange},
		"ikev2 id space":      {func(*ipsecd.Config) {}, vpn.IDRange{Lo: 4500, Hi: 0x80000001}, ipsecd.ErrNoSweepRange},
		"overlap":             {func(*ipsecd.Config) {}, vpn.IDRange{Lo: 4400, Hi: 4599}, ipsecd.ErrSweepRangeClash},
		"descriptors own all": {func(c *ipsecd.Config) { c.IDs = vpn.IDRange{} }, charonIDs, ipsecd.ErrSweepRangeClash},
		"in-memory store":     {func(c *ipsecd.Config) { c.Boot = dfkit.NewMemoryBootStore() }, charonIDs, ipsecd.ErrSweepVolatile},
		"no keyer":            {func(c *ipsecd.Config) { c.Keys = nil }, charonIDs, vpn.ErrNoKeyer},
	} {
		cfg := good
		c.mut(&cfg)
		if _, err := ipsecd.NewCharonSweeper(cfg, c.ids); !errors.Is(err, c.want) {
			t.Fatalf("%s: %v, want %v", name, err, c.want)
		}
	}
}

func TestCharonSweepStopped(t *testing.T) {
	v := newFakeVPP()
	cfg := sweepCfg(t, v)
	// ours: SPD 4001 + SA 4001, and a record inside the charon range (never touched)
	if _, err := ipsecd.NewSpd(cfg).Create(ctx, &vpnpb.IpsecSpd{SpdId: 4001}); err != nil {
		t.Fatal(err)
	}
	if _, err := ipsecd.NewSa(cfg).Create(ctx, transportSA()); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Boot.Put(dfkit.BootRecord{Key: "ipsec.sa/4509", Identity: "old/1/1", Value: "x"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint32{4502, 4503, 4504, 4505, 4509, 3001} {
		charonSA(v, id)
	}
	charonPolicy(v, 4500, 1, 4502)  // charon SPD
	charonPolicy(v, 3002, 1, 4503)  // someone's unrecorded SPD outside the charon range
	charonPolicy(v, 4001, 99, 4505) // one of OUR SPDs → 4505 in use
	tun := v.addIface("ipip9", "w4:ipip9")
	v.tps[tun] = ipsec.IpsecTunnelProtect{SwIfIndex: interfaceIndex(tun), SaOut: 4504, NSaIn: 1, SaIn: []uint32{4504}}
	v.saLocks[4504] += 2

	sw, err := ipsecd.NewCharonSweeper(cfg, charonIDs)
	if err != nil {
		t.Fatal(err)
	}
	ack := &countAcker{}
	if err := sw.AckRestart(ctx, "t1", ack); !errors.Is(err, ipsecd.ErrSweepNotComplete) || ack.n != 0 {
		t.Fatalf("ack before the sweep: %v", err)
	}
	res, err := sw.Sweep(ctx, "t1", nil) // charon stopped
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.DeletedSPDs, []uint32{4500}) || !slices.Equal(res.DeletedSAs, []uint32{4502, 4503}) ||
		!slices.Equal(res.InUse, []uint32{4504, 4505}) || res.DeletedPolicies != 1 {
		t.Fatalf("sweep %+v", res)
	}
	for id, want := range map[uint32]bool{4001: true, 4502: false, 4503: false, 4504: true, 4505: true, 4509: true, 3001: true} {
		if _, ok := v.sas[id]; ok != want {
			t.Fatalf("sa %d present=%v, want %v", id, ok, want)
		}
	}
	if len(v.freedWhileReferenced) != 0 {
		t.Fatalf("an SA was freed while referenced: %v", v.freedWhileReferenced)
	}
	if _, ok := v.spds[4001]; !ok {
		t.Fatal("our SPD touched")
	}
	if err := sw.AckRestart(ctx, "t2", ack); !errors.Is(err, ipsecd.ErrSweepNotComplete) {
		t.Fatalf("ack for another restart: %v", err)
	}
	if err := sw.AckRestart(ctx, "t1", ack); err != nil || ack.n != 1 {
		t.Fatalf("ack: %v %d", err, ack.n)
	}
	if err := sw.AckRestart(ctx, "t1", ack); !errors.Is(err, ipsecd.ErrSweepNotComplete) {
		t.Fatalf("a second ack needs a new sweep: %v", err)
	}
}

// TestCharonSweepNeverFreesReferencedSA is the H3 regression: a failing policy delete stops the
// sweep for that SA; a second sweep must not unlock it either (VPP would free it under the policy).
func TestCharonSweepNeverFreesReferencedSA(t *testing.T) {
	v := newFakeVPP()
	cfg := sweepCfg(t, v)
	charonSA(v, 4510)
	charonPolicy(v, 3002, 1, 4510) // policy outside the charon range
	v.failPolicyDel = true
	sw, err := ipsecd.NewCharonSweeper(cfg, charonIDs)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := sw.Sweep(ctx, "t", func(uint32) bool { return false }); err == nil {
			t.Fatal("a failed policy delete must fail the sweep")
		}
		if _, ok := v.sas[4510]; !ok || v.saLocks[4510] != 2 || len(v.freedWhileReferenced) != 0 {
			t.Fatalf("sweep %d: SA unlocked while referenced (locks %d, uaf %v)", i, v.saLocks[4510], v.freedWhileReferenced)
		}
		if n := len(v.CallsNamed("ipsec_sad_entry_del")); n != 0 {
			t.Fatalf("sweep %d: %d unlocks", i, n)
		}
	}
	if err := sw.AckRestart(ctx, "t", &countAcker{}); !errors.Is(err, ipsecd.ErrSweepNotComplete) {
		t.Fatalf("ack after a failed sweep: %v", err)
	}
	v.failPolicyDel = false
	if res, err := sw.Sweep(ctx, "t", func(uint32) bool { return false }); err != nil || !slices.Equal(res.DeletedSAs, []uint32{4510}) {
		t.Fatalf("recovered sweep %+v %v", res, err)
	}
	if len(v.freedWhileReferenced) != 0 {
		t.Fatal("use-after-free")
	}
}

func TestCharonSweepLive(t *testing.T) {
	v := newFakeVPP()
	cfg := sweepCfg(t, v)
	charonSA(v, 4520)
	charonSA(v, 4521)
	charonPolicy(v, 4500, 1, 4521) // the running charon's SPD stays while charon runs
	sw, err := ipsecd.NewCharonSweeper(cfg, charonIDs)
	if err != nil {
		t.Fatal(err)
	}
	res, err := sw.Sweep(ctx, "t", func(spi uint32) bool { return spi == 7000+4521 })
	if err != nil || !slices.Equal(res.DeletedSAs, []uint32{4520}) || len(res.DeletedSPDs) != 0 {
		t.Fatalf("live sweep %+v %v", res, err)
	}
	if _, ok := v.sas[4521]; !ok {
		t.Fatal("live SA swept")
	}
}

// TestSaDeleteNeverUnlocksReferencedSA: Sa.Delete refuses while a policy still references the SA
// (a retried delete must not drop the policy's lock).
func TestSaDeleteNeverUnlocksReferencedSA(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	if _, err := ipsecd.NewSa(cfg).Create(ctx, transportSA()); err != nil {
		t.Fatal(err)
	}
	charonPolicy(v, 3002, 1, 4001)
	if err := ipsecd.NewSa(cfg).Delete(ctx, transportSA(), nil); err == nil {
		t.Fatal("delete of a referenced SA must be refused")
	}
	if _, ok := v.sas[4001]; !ok || len(v.freedWhileReferenced) != 0 || len(v.CallsNamed("ipsec_sad_entry_del")) != 0 {
		t.Fatal("referenced SA unlocked")
	}
}
