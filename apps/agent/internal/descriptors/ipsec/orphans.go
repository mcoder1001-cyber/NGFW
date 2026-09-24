package ipsec

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
)

// Orphaned SAs after a charon restart (D-089, RF-2 review M3).
//
// With the kernel-vpp plugin (P11) charon installs its CHILD_SAs into VPP itself — SAs and
// "protect" SPD policies that no DF-5 descriptor created and that carry no tag. When charon
// restarts, the SAs of the previous charon stay in VPP: nobody will rekey or delete them. RF-2
// reports such a restart (strongswan.State.Restarted); P11 then calls SweepCharonOrphans with the
// SPIs of the CHILD_SAs the new charon has installed, and acknowledges the restart
// (strongswan.Renderer.AckRestart) only when the sweep succeeded — SweepAndAck does both.
//
// What is an orphan: an SA whose id is inside the charon id range, that has no ownership record of
// the ipsec.sa descriptor (our own SAs are never touched), and whose SPI is not live. The sweep
// first removes the SPD policies (in SPDs of the charon range without our record) that protect
// with an orphan, then the SA; each deletion re-reads the SA / policy right before deleting it
// (D-071/D-074). SAs still referenced by a tunnel protection are reported (InUse) and left alone:
// removing the protection would change a tunnel interface, which is P11's decision.
//
// Not swept: charon's SPDs and bypass policies (a fresh charon's SPD holds only bypass policies
// until its first CHILD_SA and cannot be told apart from a stale one), see
// docs/agent/descriptors/ipsec.md.

// CharonSweep configures SweepCharonOrphans.
type CharonSweep struct {
	// IDs is the SA/SPD id range charon's kernel-vpp plugin allocates from. It must be set: the
	// zero range (every id) is refused, the sweep never scans the whole SAD.
	IDs vpn.IDRange
	// Live reports whether spi belongs to a CHILD_SA the running charon has installed.
	Live func(spi uint32) bool
}

// SweepResult lists what a sweep did (ids only — never key material).
type SweepResult struct {
	DeletedSAs      []uint32
	DeletedPolicies int
	InUse           []uint32 // orphans still referenced by a tunnel protection (left in place)
}

// ErrNoSweepRange is returned when CharonSweep.IDs is the zero range.
var ErrNoSweepRange = errors.New("ipsec: charon sweep needs an explicit SA id range")

// SweepCharonOrphans removes the SAs (and their protect policies) a previous charon left in VPP.
// cfg is the ipsec package Config of the owner (client, record store, owned id range of the
// descriptors).
func SweepCharonOrphans(ctx context.Context, cfg Config, sw CharonSweep) (SweepResult, error) {
	var res SweepResult
	if sw.IDs == (vpn.IDRange{}) {
		return res, ErrNoSweepRange
	}
	if sw.Live == nil {
		return res, errors.New("ipsec: charon sweep needs the live SPI set")
	}
	rec := cfg.records()
	bid, err := rec.Identity(ctx)
	if err != nil {
		return res, fmt.Errorf("charon sweep: %w", err)
	}
	all, err := dumpSAs(ctx, cfg, ^uint32(0))
	if err != nil {
		return res, err
	}
	orphans := map[uint32]uint32{} // sa id → spi
	for _, sa := range all {
		v := sa.value
		id := v.GetSadId()
		if !sw.IDs.Contains(id) || sw.Live(v.GetSpi()) {
			continue
		}
		if _, ours := rec.Valid(bid, saRecordKey(id)); ours {
			continue // created by our ipsec.sa descriptor
		}
		orphans[id] = v.GetSpi()
	}
	if len(orphans) == 0 {
		return res, nil
	}

	// tunnel protections that still use an orphan: leave those SAs alone
	tp := NewTunnelProtect(cfg)
	prots, err := tp.dump(ctx, noInterface)
	if err != nil {
		return res, err
	}
	for _, p := range prots {
		for _, id := range append([]uint32{p.SaOut}, p.SaIn...) {
			if _, ok := orphans[id]; ok {
				res.InUse = append(res.InUse, id)
				delete(orphans, id)
			}
		}
	}

	// protect policies in charon SPDs (never in SPDs our descriptors own)
	spds, err := dumpSpdIDs(ctx, cfg)
	if err != nil {
		return res, err
	}
	entries := NewSpdEntry(cfg)
	svc := ipsec.NewServiceClient(cfg.Client)
	var errs []error
	for _, spd := range spds {
		if !sw.IDs.Contains(spd) {
			continue
		}
		if _, ours := rec.Valid(bid, spdRecordKey(spd)); ours {
			continue
		}
		pols, err := entries.dumpSpd(ctx, spd)
		if err != nil {
			return res, err
		}
		for _, p := range pols {
			if p.GetAction() != actions.name(ipsec_types.IPSEC_API_SPD_ACTION_PROTECT) {
				continue
			}
			if _, orphan := orphans[p.GetSaId()]; !orphan {
				continue
			}
			if err := deletePolicy(ctx, svc, p); err != nil {
				errs = append(errs, err)
				continue
			}
			res.DeletedPolicies++
		}
	}

	ids := make([]uint32, 0, len(orphans))
	for id := range orphans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		// re-read right before the delete: the same SA (id and SPI), still not ours, not live
		cur, err := dumpSAs(ctx, cfg, id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		still := false
		for _, sa := range cur {
			if sa.value.GetSadId() == id && sa.value.GetSpi() == orphans[id] && !sw.Live(orphans[id]) {
				still = true
			}
		}
		if _, ours := rec.Valid(bid, saRecordKey(id)); !still || ours {
			continue
		}
		if _, err := svc.IpsecSadEntryDel(ctx, &ipsec.IpsecSadEntryDel{ID: id}); err != nil {
			errs = append(errs, fmt.Errorf("ipsec_sad_entry_del (orphan sa %d): %w", id, err))
			continue
		}
		res.DeletedSAs = append(res.DeletedSAs, id)
	}
	sort.Slice(res.InUse, func(i, j int) bool { return res.InUse[i] < res.InUse[j] })
	return res, errors.Join(errs...)
}

// deletePolicy removes one dumped policy (re-encoded from its decoded form).
func deletePolicy(ctx context.Context, svc ipsec.RPCService, p *vpnpb.IpsecSpdEntry) error {
	e, err := encodeSpdEntry(p)
	if err != nil {
		return err
	}
	if _, err := svc.IpsecSpdEntryAddDelV2(ctx, &ipsec.IpsecSpdEntryAddDelV2{IsAdd: false, Entry: e}); err != nil {
		return fmt.Errorf("ipsec_spd_entry_add_del_v2 (orphan policy spd %d sa %d): %w", p.GetSpdId(), p.GetSaId(), err)
	}
	return nil
}

// RestartAcker is the part of RF-2's strongswan.Renderer the sweep needs.
type RestartAcker interface {
	AckRestart(ctx context.Context) error
}

// SweepAndAck runs SweepCharonOrphans and acknowledges the charon restart only when the sweep
// finished without errors (orphans still in use by a tunnel protection do not block the ack:
// they are reported for P11).
func SweepAndAck(ctx context.Context, cfg Config, sw CharonSweep, ack RestartAcker) (SweepResult, error) {
	res, err := SweepCharonOrphans(ctx, cfg, sw)
	if err != nil {
		return res, err
	}
	if err := ack.AckRestart(ctx); err != nil {
		return res, fmt.Errorf("charon sweep: ack restart: %w", err)
	}
	return res, nil
}
