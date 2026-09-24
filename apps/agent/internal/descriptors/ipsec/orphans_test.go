package ipsec_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
)

type countAcker struct {
	n   int
	err error
}

func (a *countAcker) AckRestart(context.Context) error { a.n++; return a.err }

func TestSweepCharonOrphans(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	// our own SA inside the charon range (recorded by the descriptor): never an orphan
	own := transportSA()
	own.SadId, own.Spi = 4501, 1
	if _, err := ipsecd.NewSa(cfg).Create(ctx, own); err != nil {
		t.Fatal(err)
	}
	// charon's SAs: 4502 orphan (+ protect policy in charon SPD 4500), 4503 live, 4504 orphan but
	// used by a tunnel protection, 3001 outside the range
	for _, id := range []uint32{4502, 4503, 4504, 3001} {
		v.sas[id] = ipsec_types.IpsecSadEntryV4{SadID: id, Spi: 7000 + id, Protocol: ipsec_types.IPSEC_API_PROTO_ESP}
	}
	v.spds[4500] = 50
	v.policies[4500] = []ipsecSpdEntryV2Alias{
		{SpdID: 4500, Priority: 1, SaID: 4502, Policy: ipsec_types.IPSEC_API_SPD_ACTION_PROTECT, Protocol: 255},
		{SpdID: 4500, Priority: 2, Policy: ipsec_types.IPSEC_API_SPD_ACTION_BYPASS, Protocol: 255},
	}
	tun := v.addIface("ipip9", "w4:ipip9")
	v.tps[tun] = ipsec.IpsecTunnelProtect{SwIfIndex: interfaceIndex(tun), SaOut: 4504, NSaIn: 1, SaIn: []uint32{4503}}

	sw := ipsecd.CharonSweep{IDs: vpn.IDRange{Lo: 4500, Hi: 4599}, Live: func(spi uint32) bool { return spi == 7000+4503 }}
	if _, err := ipsecd.SweepCharonOrphans(ctx, cfg, ipsecd.CharonSweep{Live: sw.Live}); !errors.Is(err, ipsecd.ErrNoSweepRange) {
		t.Fatalf("zero range: %v", err)
	}
	ack := &countAcker{}
	res, err := ipsecd.SweepAndAck(ctx, cfg, sw, ack)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.DeletedSAs, []uint32{4502}) || res.DeletedPolicies != 1 || !slices.Equal(res.InUse, []uint32{4504}) || ack.n != 1 {
		t.Fatalf("sweep %+v ack %d", res, ack.n)
	}
	for id, want := range map[uint32]bool{4501: true, 4502: false, 4503: true, 4504: true, 3001: true} {
		if _, ok := v.sas[id]; ok != want {
			t.Fatalf("sa %d present=%v, want %v", id, ok, want)
		}
	}
	if len(v.policies[4500]) != 1 || v.policies[4500][0].Policy != ipsec_types.IPSEC_API_SPD_ACTION_BYPASS {
		t.Fatalf("charon SPD policies %+v (bypass must stay)", v.policies[4500])
	}
	// idempotent: nothing left to sweep
	if res, err := ipsecd.SweepCharonOrphans(ctx, cfg, sw); err != nil || len(res.DeletedSAs) != 0 {
		t.Fatalf("second sweep %+v %v", res, err)
	}
	// a failing sweep does not acknowledge the restart
	v.sas[4505] = ipsec_types.IpsecSadEntryV4{SadID: 4505, Spi: 1, Protocol: ipsec_types.IPSEC_API_PROTO_ESP}
	v.Fail("ipsec_sad_entry_del", errors.New("boom"))
	ack2 := &countAcker{}
	if _, err := ipsecd.SweepAndAck(ctx, cfg, sw, ack2); err == nil || ack2.n != 0 {
		t.Fatalf("failed sweep: %v, acks %d", err, ack2.n)
	}
}
