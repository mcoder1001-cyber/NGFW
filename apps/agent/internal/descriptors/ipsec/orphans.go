package ipsec

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
//   - **P11 precondition: live == nil asserts that charon is STOPPED** — the sweep then removes
//     every unrecorded SPD in the charon range, which would drop a running charon's tunnels.
//   - VPP lock counting (ipsec_sa.c, ipsec_spd.c:24-104, ipsec_spd_policy.c:308):
//     ipsec_sad_entry_del is an UNLOCK; every protect policy and every tunnel protection holds its
//     own lock, released ONLY by deleting that policy / protection — deleting an SPD frees its
//     policy vectors WITHOUT unlocking their SAs (fix round 2, N1, reproduced on the host). So:
//     1. (charon stopped) every policy of every unrecorded charon-range SPD is deleted one by one,
//        and a dump must show the SPD empty;
//     2. per orphan SA: a tunnel protection using it, or a policy in any SPD outside the charon
//        range or recorded by us, leaves it alone — reported "in use by <who>" (D-071: foreign
//        SPDs are never touched, fix round 2 N2); policies in unrecorded charon-range SPDs are
//        deleted; a fresh dump of all SPDs must show zero references; the SA is re-read (same
//        SPI, unrecorded, not live) and unlocked once; a re-dump must show it GONE, otherwise it
//        is reported NotSwept (a lock we cannot see is still held);
//     3. (charon stopped) the emptied charon SPDs are deleted.
//     Any failure stops the sweep for that SA or SPD; a sweep with an error or a NotSwept SA writes
//     no completion marker, so the ack for that restart is refused.
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
	// InUse: orphans left alone because something else references them ("spd <id>" of a foreign
	// or our SPD, "tunnel protection").
	InUse []InUseSA
	// NotSwept: orphans still present after their unlock (a lock is held elsewhere) — the sweep
	// reports an error and the restart cannot be acked.
	NotSwept []uint32
}

// InUseSA is an orphan SA left in place and what references it.
type InUseSA struct {
	SA uint32
	By string
}

const sweepMarkerKey = "ipsec.charon-sweep"

// recorded reports whether key has any record (valid, pending, or of an earlier VPP instance).
func (s *CharonSweeper) recorded(key string) bool {
	_, ok := s.cfg.Boot.Get(key)
	return ok
}

// Sweep removes charon's orphans (see the file comment). live == nil asserts charon is stopped.
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
	entries := NewSpdEntry(s.cfg)
	var errs []error

	// 1. charon stopped: empty its SPDs policy by policy (only a policy delete unlocks its SA)
	var emptied []uint32
	if stopped {
		spds, err := dumpSpdIDs(ctx, s.cfg)
		if err != nil {
			return res, err
		}
		for _, id := range spds {
			if !s.charonSPD(id) {
				continue
			}
			pols, err := entries.dumpSpd(ctx, id)
			if err != nil {
				return res, err
			}
			ok := true
			for _, p := range pols {
				if err := deletePolicy(ctx, svc, p); err != nil {
					errs = append(errs, err)
					ok = false
					break
				}
				res.DeletedPolicies++
			}
			if !ok {
				continue
			}
			if left, err := entries.dumpSpd(ctx, id); err != nil || len(left) > 0 {
				errs = append(errs, fmt.Errorf("ipsec: charon spd %d still holds %d policies (%v)", id, len(left), err))
				continue
			}
			emptied = append(emptied, id)
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
		by, deleted, err := s.releasePolicies(ctx, svc, id)
		res.DeletedPolicies += deleted
		if err != nil {
			errs = append(errs, err) // stop for this SA: never unlock a referenced SA
			continue
		}
		if by != "" {
			res.InUse = append(res.InUse, InUseSA{SA: id, By: by})
			continue
		}
		// re-read right before the unlock: same SA, still unrecorded, not live
		if !s.stillOrphan(ctx, id, orphans[id], live) {
			continue
		}
		if _, err := svc.IpsecSadEntryDel(ctx, &ipsec.IpsecSadEntryDel{ID: id}); err != nil {
			errs = append(errs, fmt.Errorf("ipsec_sad_entry_del (orphan sa %d): %w", id, err))
			continue
		}
		// VPP frees the SA only when its last lock goes: it must be gone now
		after, err := dumpSAs(ctx, s.cfg, id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if slices.ContainsFunc(after, func(sa dumpedSa) bool { return sa.value.GetSadId() == id }) {
			res.NotSwept = append(res.NotSwept, id)
			errs = append(errs, fmt.Errorf("ipsec: orphan sa %d still present after its unlock (a lock is held elsewhere)", id))
			continue
		}
		res.DeletedSAs = append(res.DeletedSAs, id)
	}

	// 3. charon stopped: delete the emptied charon SPDs (unbinds their interfaces)
	for _, id := range emptied {
		if _, err := svc.IpsecSpdAddDel(ctx, &ipsec.IpsecSpdAddDel{IsAdd: false, SpdID: id}); err != nil {
			errs = append(errs, fmt.Errorf("ipsec_spd_add_del (charon spd %d, del): %w", id, err))
			continue
		}
		res.DeletedSPDs = append(res.DeletedSPDs, id)
	}

	if err := errors.Join(errs...); err != nil {
		return res, err
	}
	if err := rec.Put(bid, sweepMarkerKey, restart); err != nil {
		return res, fmt.Errorf("charon sweep: marker: %w", err)
	}
	return res, nil
}

// charonSPD reports whether SPD id is charon's: inside the charon range and without any record of ours.
func (s *CharonSweeper) charonSPD(id uint32) bool {
	return s.ids.Contains(id) && !s.recorded(spdRecordKey(id))
}

// stillOrphan re-reads SA id right before its unlock.
func (s *CharonSweeper) stillOrphan(ctx context.Context, id, spi uint32, live func(uint32) bool) bool {
	cur, err := dumpSAs(ctx, s.cfg, id)
	if err != nil {
		return false
	}
	same := slices.ContainsFunc(cur, func(sa dumpedSa) bool { return sa.value.GetSadId() == id && sa.value.GetSpi() == spi })
	return same && !live(spi) && !s.recorded(saRecordKey(id))
}

// releasePolicies removes the protect policies referencing SA id in charon SPDs and verifies with a
// fresh dump that no policy references it any more. by != "": a tunnel protection or a policy in
// an SPD that is not charon's (ours, another owner's) references it — left alone (D-071).
func (s *CharonSweeper) releasePolicies(ctx context.Context, svc ipsec.RPCService, id uint32) (by string, deleted int, err error) {
	if used, err := protectionUses(ctx, s.cfg, id); err != nil || used {
		if used {
			return "tunnel protection", 0, nil
		}
		return "", 0, err
	}
	refs, err := policiesUsing(ctx, s.cfg, id)
	if err != nil {
		return "", 0, err
	}
	for _, p := range refs {
		if !s.charonSPD(p.GetSpdId()) {
			return fmt.Sprintf("spd %d", p.GetSpdId()), 0, nil
		}
	}
	for _, p := range refs {
		if err := deletePolicy(ctx, svc, p); err != nil {
			return "", deleted, err
		}
		deleted++
	}
	left, err := policiesUsing(ctx, s.cfg, id)
	if err != nil {
		return "", deleted, err
	}
	if len(left) > 0 {
		return "", deleted, fmt.Errorf("ipsec: orphan sa %d still referenced by %d policies; not unlocked", id, len(left))
	}
	return "", deleted, nil
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
