package ipsec

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
)

// Orphaned charon state after a charon restart (D-089, D-096; review DF-5 H2, H3, M2).
//
// With the kernel-vpp plugin (P11) charon installs its SPDs, policies and CHILD_SAs into VPP
// itself — untagged objects no DF-5 descriptor created. After a charon restart the previous
// instance's objects stay in VPP, and stock kernel-vpp allocates SPD/SA ids from 1 again. D-096
// fixes the procedure (P11 runs it when RF-2 reports State.Restarted):
//
//	stop charon → CharonSweeper.Sweep(ctx, restart, nil) → start charon → CharonSweeper.AckRestart(ctx, restart, renderer)
//
//   - Charon owns its own id range, validated at construction: it must be set, disjoint from the
//     descriptors' range (Config.IDs, which must then be explicit) and below bit 31 (the
//     VPP-native ikev2 plugin's SA ids). Overlap = refused.
//   - The sweeper needs the owner's PERSISTED record store (Config.Boot = *dfkit.FileBootStore or
//     a store reporting Persistent()): nothing with an ownership record — valid, pending or of an
//     earlier VPP instance — is ever touched; with an in-memory store a restarted agent would have
//     forgotten its own objects.
//   - With charon stopped (live == nil) every unrecorded SPD in the charon range is deleted first
//     (VPP drops its policies' SA locks and unbinds its interfaces), then every unrecorded SA in the
//     range. While charon runs, live reports its installed SPIs and only SAs are swept.
//   - VPP lock counting (ipsec_sa.c): ipsec_sad_entry_del is an UNLOCK; every protect policy and
//     every tunnel protection holds its own lock. So per SA: a tunnel protection using it → left in
//     place (InUse); the protect policies using it are deleted in EVERY SPD (not only charon's) —
//     except in an SPD with our ownership record (then InUse); then a fresh dump of all SPDs must
//     show no policy referencing it; only then the SA is unlocked, after re-reading it (same SPI,
//     still unrecorded, not live). Any failure stops the sweep FOR THAT SA — a referenced SA is
//     never unlocked, so it can never be freed under a policy (use-after-free on the packet path).
//   - AckRestart is refused unless a Sweep for the same restart token finished without error on the
//     running VPP instance (persisted marker); it then calls the RF-2 renderer's AckRestart and drops
//     the marker. restart is the token P11 uses for the restart being handled (RF-2's
//     State.DaemonStartedAt of the charon instance that went away).

// CharonSweeper sweeps charon's orphans; construct it with NewCharonSweeper.
type CharonSweeper struct {
	cfg Config
	ids vpn.IDRange
}

// Sweep errors.
var (
	ErrNoSweepRange     = errors.New("ipsec: charon sweep needs an explicit charon id range")
	ErrSweepRangeClash  = errors.New("ipsec: the charon id range must be disjoint from the descriptors' id range (D-096)")
	ErrSweepVolatile    = errors.New("ipsec: charon sweep needs the owner's persisted record store (WithBootStore)")
	ErrSweepNotComplete = errors.New("ipsec: no completed charon sweep for this restart; refusing AckRestart")
)

// ikev2SAIDBase is where the VPP-native ikev2 plugin allocates SA ids (ikev2.c: 0x80000000 | …).
const ikev2SAIDBase = 0x80000000

// PersistentStore is implemented by record stores that survive an agent restart.
type PersistentStore interface{ Persistent() bool }

func persistent(s dfkit.BootStore) bool {
	if _, ok := s.(*dfkit.FileBootStore); ok {
		return true
	}
	p, ok := s.(PersistentStore)
	return ok && p.Persistent()
}

// NewCharonSweeper validates the configuration at startup (see the file comment).
func NewCharonSweeper(cfg Config, charon vpn.IDRange) (*CharonSweeper, error) {
	switch {
	case charon == (vpn.IDRange{}) || charon.Lo > charon.Hi:
		return nil, ErrNoSweepRange
	case charon.Hi >= ikev2SAIDBase:
		return nil, fmt.Errorf("%w: %d–%d reaches the ikev2 plugin's SA ids (≥ 0x80000000)", ErrNoSweepRange, charon.Lo, charon.Hi)
	case cfg.IDs == (vpn.IDRange{}):
		return nil, fmt.Errorf("%w: the descriptors own every id (Config.IDs unset)", ErrSweepRangeClash)
	case cfg.IDs.Lo <= charon.Hi && charon.Lo <= cfg.IDs.Hi:
		return nil, fmt.Errorf("%w: %d–%d overlaps %d–%d", ErrSweepRangeClash, charon.Lo, charon.Hi, cfg.IDs.Lo, cfg.IDs.Hi)
	case cfg.Boot == nil || !persistent(cfg.Boot):
		return nil, ErrSweepVolatile
	case cfg.Keys == nil:
		return nil, vpn.ErrNoKeyer
	}
	return &CharonSweeper{cfg: cfg, ids: charon}, nil
}

// SweepResult lists what a sweep did (ids only — never key material).
type SweepResult struct {
	DeletedSPDs     []uint32
	DeletedPolicies int
	DeletedSAs      []uint32
	InUse           []uint32 // orphans still referenced by a tunnel protection or one of our policies
}

const sweepMarkerKey = "ipsec.charon-sweep"

// recorded reports whether key has any record (valid, pending, or of an earlier VPP instance).
func (s *CharonSweeper) recorded(key string) bool {
	_, ok := s.cfg.Boot.Get(key)
	return ok
}

// Sweep removes charon's orphans (see the file comment). live == nil means charon is stopped.
func (s *CharonSweeper) Sweep(ctx context.Context, restart string, live func(spi uint32) bool) (SweepResult, error) {
	var res SweepResult
	if restart == "" {
		return res, errors.New("ipsec: charon sweep needs the restart token")
	}
	stopped := live == nil
	if stopped {
		live = func(uint32) bool { return false }
	}
	rec := s.cfg.records()
	bid, err := rec.Identity(ctx)
	if err != nil {
		return res, fmt.Errorf("charon sweep: %w", err)
	}
	svc := ipsec.NewServiceClient(s.cfg.Client)
	var errs []error

	// 1. charon stopped: its SPDs go first (drops the policies' SA locks, unbinds interfaces)
	if stopped {
		spds, err := dumpSpdIDs(ctx, s.cfg)
		if err != nil {
			return res, err
		}
		for _, id := range spds {
			if !s.ids.Contains(id) || s.recorded(spdRecordKey(id)) {
				continue
			}
			if _, err := svc.IpsecSpdAddDel(ctx, &ipsec.IpsecSpdAddDel{IsAdd: false, SpdID: id}); err != nil {
				errs = append(errs, fmt.Errorf("ipsec_spd_add_del (charon spd %d, del): %w", id, err))
				continue
			}
			res.DeletedSPDs = append(res.DeletedSPDs, id)
		}
	}

	// 2. orphan SAs: in the charon range, no record, not live
	all, err := dumpSAs(ctx, s.cfg, ^uint32(0))
	if err != nil {
		return res, errors.Join(append(errs, err)...)
	}
	orphans := map[uint32]uint32{} // sa id → spi
	for _, sa := range all {
		v := sa.value
		if s.ids.Contains(v.GetSadId()) && !live(v.GetSpi()) && !s.recorded(saRecordKey(v.GetSadId())) {
			orphans[v.GetSadId()] = v.GetSpi()
		}
	}
	ids := make([]uint32, 0, len(orphans))
	for id := range orphans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		inUse, deleted, err := s.releasePolicies(ctx, svc, id)
		res.DeletedPolicies += deleted
		if err != nil {
			errs = append(errs, err) // stop for this SA: never unlock a referenced SA
			continue
		}
		if inUse {
			res.InUse = append(res.InUse, id)
			continue
		}
		// re-read right before the unlock: same SA, still unrecorded, not live
		cur, err := dumpSAs(ctx, s.cfg, id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		same := false
		for _, sa := range cur {
			same = same || (sa.value.GetSadId() == id && sa.value.GetSpi() == orphans[id])
		}
		if !same || live(orphans[id]) || s.recorded(saRecordKey(id)) {
			continue
		}
		if _, err := svc.IpsecSadEntryDel(ctx, &ipsec.IpsecSadEntryDel{ID: id}); err != nil {
			errs = append(errs, fmt.Errorf("ipsec_sad_entry_del (orphan sa %d): %w", id, err))
			continue
		}
		res.DeletedSAs = append(res.DeletedSAs, id)
	}
	if err := errors.Join(errs...); err != nil {
		return res, err
	}
	if err := rec.Put(bid, sweepMarkerKey, restart); err != nil {
		return res, fmt.Errorf("charon sweep: marker: %w", err)
	}
	return res, nil
}

// releasePolicies removes every protect policy referencing SA id, in every SPD without our
// record, and verifies with a fresh dump that no policy references it any more. inUse: a tunnel
// protection or a policy in one of OUR SPDs references it (left alone).
func (s *CharonSweeper) releasePolicies(ctx context.Context, svc ipsec.RPCService, id uint32) (inUse bool, deleted int, err error) {
	if used, err := protectionUses(ctx, s.cfg, id); err != nil || used {
		return used, 0, err
	}
	refs, err := policiesUsing(ctx, s.cfg, id)
	if err != nil {
		return false, 0, err
	}
	for _, p := range refs {
		if s.recorded(spdRecordKey(p.GetSpdId())) {
			return true, 0, nil
		}
	}
	for _, p := range refs {
		if err := deletePolicy(ctx, svc, p); err != nil {
			return false, deleted, err
		}
		deleted++
	}
	left, err := policiesUsing(ctx, s.cfg, id)
	if err != nil {
		return false, deleted, err
	}
	if len(left) > 0 {
		return false, deleted, fmt.Errorf("ipsec: orphan sa %d still referenced by %d policies; not unlocked", id, len(left))
	}
	return false, deleted, nil
}

// protectionUses reports whether a tunnel protection references SA id.
func protectionUses(ctx context.Context, cfg Config, id uint32) (bool, error) {
	prots, err := NewTunnelProtect(cfg).dump(ctx, noInterface)
	if err != nil {
		return false, err
	}
	for _, p := range prots {
		for _, x := range append([]uint32{p.SaOut}, p.SaIn...) {
			if x == id {
				return true, nil
			}
		}
	}
	return false, nil
}

// policiesUsing dumps the protect policies of ALL SPDs that reference SA id.
func policiesUsing(ctx context.Context, cfg Config, id uint32) ([]*vpnpb.IpsecSpdEntry, error) {
	spds, err := dumpSpdIDs(ctx, cfg)
	if err != nil {
		return nil, err
	}
	entries := NewSpdEntry(cfg)
	var out []*vpnpb.IpsecSpdEntry
	for _, spd := range spds {
		pols, err := entries.dumpSpd(ctx, spd)
		if err != nil {
			return nil, err
		}
		for _, p := range pols {
			if p.GetAction() == actions.name(ipsec_types.IPSEC_API_SPD_ACTION_PROTECT) && p.GetSaId() == id {
				out = append(out, p)
			}
		}
	}
	return out, nil
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

// RestartAcker is the part of RF-2's strongswan.Renderer the procedure needs.
type RestartAcker interface {
	AckRestart(ctx context.Context) error
}

// AckRestart acknowledges the charon restart restart through ack — only when a Sweep for the
// same token completed on the running VPP instance (ErrSweepNotComplete otherwise).
func (s *CharonSweeper) AckRestart(ctx context.Context, restart string, ack RestartAcker) error {
	rec := s.cfg.records()
	bid, err := rec.Identity(ctx)
	if err != nil {
		return fmt.Errorf("charon ack: %w", err)
	}
	if v, ok := rec.Valid(bid, sweepMarkerKey); !ok || v != restart || restart == "" {
		return ErrSweepNotComplete
	}
	if err := ack.AckRestart(ctx); err != nil {
		return fmt.Errorf("charon ack: %w", err)
	}
	return rec.Drop(sweepMarkerKey)
}
