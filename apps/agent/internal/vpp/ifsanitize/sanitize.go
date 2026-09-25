// Package ifsanitize clears the per-interface state VPP 26.06 keeps on a sw_if_index after the
// interface is deleted (docs/vpp-code-track.md V19, V21, V23; LOG D-095). VPP reuses a freed
// sw_if_index for the next interface of any type, and several per-index vectors are not reset
// by the sw-interface delete callbacks. Per kind, what Sanitize reads back and how it clears it:
//
//   - ip4/ip6 "ip classify table" (classify_set_interface_ip_table): read directly when an
//     address is added — the new interface gets a classify DPO to the old table on its /32.
//     If that table was deleted, the first packet to the address crashes VPP in
//     vnet_classify_find_entry (2026-09-24 04:50:27). No readback exists: reset blindly to ~0
//     (a reset names no table, so a deleted one does not matter).
//   - l2 input/output classify tables (classify_set_interface_l2_tables): no readback: reset
//     blindly (all three tables ~0 also disables the l2 feature bit).
//   - input ACL tables: read back exactly with classify_table_by_interface.
//   - policer classify and flow classify tables: read back exactly with policer_classify_dump /
//     flow_classify_dump with sw_if_index 0. In VPP 26.06 the handler walks
//     vec_len(&vector[sw_if_index]) (classify_api.c): ~0 returns nothing, any other index reads
//     out of bounds (see descriptors/policer/attach.go, D-063), but for 0 the pointer is the
//     vector itself, so the reply is every binding of every index — deleted ones included, like
//     `show classify policer` — and Sanitize keeps the rows of its own index. Never called with
//     another index.
//   - output ACL tables: no binary-API readback at all. Detected per slot with the run's own
//     probe table P (an empty placeholder): an unbind naming P succeeds only when the slot names
//     P's index; otherwise P is bound — VPP binds only an empty slot, an add on a bound slot
//     returns 0 and changes nothing (in_out_acl.c) — and unbound again: that unbind fails exactly
//     when the slot holds another table. Only such a slot is probed: an unbind through each
//     existing table, then through freed indices brought back one by one (below). P has no
//     sessions and misses to the node's default next, and it is bound only on an empty slot of
//     this interface for the two calls in between. This is the only kind that is probed.
//   - ADL: the adl-input feature on device-input — disabled blindly (a disable of a feature
//     that is not enabled is a no-op in vnet_feature_enable_disable). Not read back:
//     feature_is_enabled is unreliable in VPP 26.06 (the handler turns vnet_feature_is_enabled's
//     negative error codes — e.g. "sw_if_index beyond the arc's config vector", common for a
//     brand-new highest index — into is_enabled=true).
//   - vxlan bypass: a per-index bitmap that makes a later enable a silent no-op (V21); reset
//     blindly (disable is a no-op when the bit is clear).
//   - IPsec SPD binding (ipsec_interface_add_del_spd; DF-5 review M3): spd_index_by_sw_if_index
//     survives the interface, and VPP then refuses to bind an SPD to the new interface ("spd
//     already assigned"). Read with ipsec_spd_interface_dump; any binding on a new interface
//     is stale by definition (nothing of ours is bound yet) and is removed. The unbind needs an
//     existing spd_id but does not check it against the bound one (ipsec_set_interface_spd),
//     and the dump reports the SPD pool index, not its id, so the first spd_id of
//     ipsec_spds_dump is used (deleting an SPD clears every binding to it, so a stale binding
//     always refers to an existing SPD).
//
// Every interface is first put in L3 mode (sw_interface_set_l2_bridge enable=0 →
// set_int_l2_mode(MODE_L3)): that zeroes the l2-input/l2-output feature bitmaps, which carry the
// L2 input ACL, L2 output ACL and L2 policer classify bits — they are not vnet feature arcs, VPP
// resets them on delete only for interfaces that were bridged/xconnected, and l2-input-acl reads
// its table unconditionally (TD-3 review H1: a live crash vector once the index is bridged).
//
// A binding that names a deleted table cannot be removed while the table is gone (VPP checks
// pool_is_free_index before it compares the slot). The classify table pool is a vppinfra pool:
// a create pops the most recently freed index (pool.h, _pool_get pops free_indices[n_free-1]) and
// the vector never shrinks, so a freed index a binding names is on the free list and comes back
// after the indices freed after it. Sanitize resurrects ON DEMAND only (TD-25): when a binding
// names a table that is not there, it creates placeholder tables (a signature mask) until that
// index comes back — for an output ACL slot whose table is unknown, until an unbind through the
// latest placeholder succeeds — unbinds through it and deletes every placeholder again in reverse
// creation order (identity re-verified by geometry and mask right before each delete), which
// restores the free list exactly. It never pops to prove the free list empty: a freed index is
// on the free list, so a pop never grows the pool — except the probe table on a pool without any
// freed index (once per VPP instance: the index is then reused by every later run) and one pop
// after another client took the index being waited for (a pop above every index seen re-reads
// the table list). Before TD-25 every create-phase run popped 8 fresh indices "to prove the free
// list empty"; each became a permanent hole, the free list grew by 8 per create and every create
// failed closed at the 64-placeholder cap (the shared VPP, 2026-09-25 05:05, 122 freed indices).
//
// What still cannot be removed is Unclearable: in the create phase that is ErrUnclearable, and
// Acquire quarantines the index (see acquire.go) instead of reporting the interface created; a
// binding whose freed index is deeper in the free list than MaxPlaceholders pops is Unclearable
// as well (Report.Capped, ErrCapped). Before a delete (BeforeDelete) the same readbacks and
// on-demand resurrection run, and what remains is logged and counted, not an error.
//
// Sanitize uses only binapi messages and touches only the given sw_if_index (plus its own
// placeholder tables). Messages of a plugin that is not loaded are skipped. Every run is logged at
// info and counted per phase (create / delete, Metrics).
package ifsanitize

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	adlapi "ngfw/agent/binapi/adl"
	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/interface_types"
	ipsecapi "ngfw/agent/binapi/ipsec"
	l2api "ngfw/agent/binapi/l2"
	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/vpp"
)

// NoIndex is VPP's "none" table index (~0).
const NoIndex = ^uint32(0)

// ErrNoSuchTable is VNET_API_ERROR_NO_SUCH_TABLE (vnet/error.h): the answer to an unbind that
// names a table which is not the one bound, or a table that does not exist.
const ErrNoSuchTable api.VPPApiError = -65

// ErrUnclearable means a binding of the new interface names a deleted classify table and could not
// be removed even through a resurrected placeholder: the index must not be used (quarantine).
var ErrUnclearable = errors.New("inherited binding to a deleted classify table cannot be removed (VPP V19)")

// Phases of a sanitize run.
const (
	PhaseCreate = "create" // a new sw_if_index, before the interface is reported created
	PhaseDelete = "delete" // right before an interface is deleted, while its tables still exist
)

// MaxPlaceholders bounds the placeholder tables of one run, the probe table included. A binding
// that names a freed index deeper in the classify pool's free list than that is Unclearable and
// the run Capped (create: ErrUnclearable and ErrCapped — Acquire quarantines the index). A run
// without such a binding makes one placeholder (the probe table).
var MaxPlaceholders = 64

// ErrCapped means MaxPlaceholders pops did not bring back the freed classify table index a
// binding of the new interface names: the binding is unclearable (the error also wraps
// ErrUnclearable) and the interface is not reported created (fail closed). It wraps
// ErrNoCleanIndex.
var ErrCapped = fmt.Errorf("%w: placeholder cap reached before the freed classify table index a binding names came back", ErrNoCleanIndex)

// noResurrect turns resurrection off (tests of the quarantine path only: a binding to a freed
// table is then unclearable; see export_test.go).
var noResurrect bool

// dumpEveryIndex is the sw_if_index policer/flow_classify_dump must be sent with in VPP 26.06:
// the only value for which the handler walks the whole per-index vector instead of nothing (~0)
// or memory out of bounds (any other index). See the package comment.
const dumpEveryIndex interface_types.InterfaceIndex = 0

// placeholderMask is the signature of Sanitize's own throwaway classify tables.
var placeholderMask = []byte("vrx-td3-v19-hold")

// State names used in reports, logs and the metric label.
const (
	StateIPClassify      = "ip-classify"      // classify_set_interface_ip_table
	StateL2Classify      = "l2-classify"      // classify_set_interface_l2_tables
	StateInputACL        = "input-acl"        // input_acl_set_interface
	StateOutputACL       = "output-acl"       // output_acl_set_interface
	StatePolicerClassify = "policer-classify" // policer_classify_set_interface
	StateFlowClassify    = "flow-classify"    // flow_classify_set_interface
	StateADL             = "adl"              // adl_interface_enable_disable
	StateVxlanBypass     = "vxlan-bypass"     //nolint:gosec // G101 false positive: a state label (sw_interface_set_vxlan_bypass)
	StateIPsecSPD        = "ipsec-spd"        // ipsec_interface_add_del_spd
	StateL2Mode          = "l2-mode"          // sw_interface_set_l2_bridge enable=0 (L3 mode)
)

// Report is what one Sanitize run found and did.
type Report struct {
	SwIfIndex uint32
	Name      string
	Phase     string
	// Placeholders is how many throwaway tables the run made: the probe table of the output ACL
	// check, plus one per free-list pop that brought back a freed index a binding names.
	Placeholders int
	// Pops are the placeholder indices in creation order (they are deleted in reverse order, which
	// puts the classify pool's free list back as it was).
	Pops []uint32
	// Wanted is how many bindings named a table that was not there (a freed index to bring back).
	Wanted int
	// Capped reports that Cap (MaxPlaceholders) placeholders did not bring back every freed index
	// a binding names: those bindings are Unclearable.
	Capped bool
	Cap    int
	// Rereads counts classify_table_ids re-reads (after a pop above every index seen: a table
	// another client created meanwhile may be the one a binding names).
	Rereads int
	// Freed lists removed bindings that named a deleted table (removed through a placeholder).
	Freed []string
	// Reset lists the states reset blindly (no readback in VPP; the call is a no-op when unset).
	Reset []string
	// Cleared lists inherited state that was found and removed ("input-acl ip4 table 3").
	Cleared []string
	// Unclearable lists bindings to deleted tables that could not be removed.
	Unclearable []string
	// Skipped lists steps skipped because VPP does not know the message (plugin not loaded).
	Skipped []string
}

// Inherited reports whether anything inherited was found.
func (r Report) Inherited() bool {
	return len(r.Cleared) > 0 || len(r.Freed) > 0 || len(r.Unclearable) > 0
}

type sanitizer struct {
	ctx   context.Context
	c     vpp.Client
	cl    classifyapi.RPCService
	idx   interface_types.InterfaceIndex
	phase string
	live  map[uint32]bool // classify tables at the snapshot (minus tables found deleted meanwhile)
	taken map[uint32]bool // tables that appeared during the run (another client's, possibly on a freed index)
	top   int64           // the highest table index seen (live, named by a binding, popped)
	holds []uint32        // this run's placeholder tables in creation order; holds[0] is the probe table
	// pending are bindings whose table is not there (named, freed) or unknown (an output ACL slot
	// bound to a table no existing one matched): resurrect brings their index back.
	pending []*bound
	found   map[string]bool // readback kinds that had a binding: read back again at the end
	out     []*bound      // output ACL slots that were bound
	rep     *Report
}

// Sanitize clears inherited per-interface state on idx, a sw_if_index VPP has just returned for
// a new interface called name (used in logs and errors only). Call it after the create and
// before the interface is reported created; on error the caller removes the interface, and on
// ErrUnclearable it quarantines the index (Acquire does both).
func Sanitize(ctx context.Context, c vpp.Client, idx uint32, name string) (Report, error) {
	return run(ctx, c, idx, name, PhaseCreate)
}

// BeforeDelete clears every per-interface binding of an interface that is about to be deleted —
// the moment its tables normally all exist (VPP keeps the bindings on the freed index, V19; TD-3
// review H3). Every interface descriptor's Delete, the restart simulations and the test fixtures
// call it right before the VPP delete. A binding that cannot be removed is logged and counted,
// not an error (the delete must go on; the next creator's Sanitize handles the index).
func BeforeDelete(ctx context.Context, c vpp.Client, idx uint32, name string) error {
	_, err := run(ctx, c, idx, name, PhaseDelete)
	return err
}

func run(ctx context.Context, c vpp.Client, idx uint32, name, phase string) (Report, error) {
	rep := Report{SwIfIndex: idx, Name: name, Phase: phase, Cap: MaxPlaceholders}
	s := &sanitizer{ctx: ctx, c: c, cl: classifyapi.NewServiceClient(c), idx: interface_types.InterfaceIndex(idx), phase: phase,
		found: map[string]bool{}, rep: &rep}
	err := s.run()
	if derr := s.dropPlaceholders(); derr != nil && err == nil {
		err = derr
	}
	if err == nil && phase == PhaseCreate {
		switch {
		case len(rep.Unclearable) > 0 && rep.Capped:
			err = fmt.Errorf("%w: %v; %w (%d placeholders)", ErrUnclearable, rep.Unclearable, ErrCapped, rep.Placeholders)
		case len(rep.Unclearable) > 0:
			err = fmt.Errorf("%w: %v", ErrUnclearable, rep.Unclearable)
		case rep.Capped:
			err = fmt.Errorf("%w (%d placeholders)", ErrCapped, rep.Placeholders)
		}
	}
	record(rep, err)
	log := slog.Default().With("interface", name, "sw_if_index", idx, "phase", phase)
	switch {
	case err != nil:
		log.Error("interface sanitize failed (VPP V19)", "err", err, "cleared", rep.Cleared, "freed", rep.Freed, "unclearable", rep.Unclearable,
			"placeholders", rep.Placeholders, "pops", rep.Pops, "wanted", rep.Wanted, "capped", rep.Capped, "rereads", rep.Rereads)
	case len(rep.Unclearable) > 0:
		log.Warn("interface sanitized: bindings to deleted classify tables remain (VPP V19)",
			"cleared", rep.Cleared, "freed", rep.Freed, "unclearable", rep.Unclearable, "placeholders", rep.Placeholders, "pops", rep.Pops,
			"capped", rep.Capped, "reset", rep.Reset, "skipped", rep.Skipped)
	default:
		log.Info("interface sanitized (VPP V19/V21 inherited state)", "cleared", rep.Cleared, "freed", rep.Freed,
			"placeholders", rep.Placeholders, "pops", rep.Pops, "rereads", rep.Rereads, "reset", rep.Reset, "skipped", rep.Skipped)
	}
	if err != nil {
		return rep, fmt.Errorf("sanitize %s (sw_if_index %d, %s): %w", name, idx, phase, err)
	}
	return rep, nil
}

func (s *sanitizer) run() error {
	for _, step := range []func() error{s.l3Mode, s.snapshot, s.ipClassify, s.l2Classify, s.probeTable,
		s.inputACL, s.outputACL, s.policerClassify, s.flowClassify, s.resurrect, s.verify,
		s.adl, s.vxlanBypass, s.ipsecSPD} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// l3Mode puts the interface in L3 mode: set_int_l2_mode(MODE_L3) zeroes the l2 feature bitmaps
// (L2 input/output ACL, L2 policer classify bits) whether or not their tables still exist. For
// an interface that is already L3 (every new interface) it changes nothing else (no L2 count
// change, the ethernet sub-interface flags of an L3 port stay).
func (s *sanitizer) l3Mode() error {
	_, err := l2api.NewServiceClient(s.c).SwInterfaceSetL2Bridge(s.ctx, &l2api.SwInterfaceSetL2Bridge{RxSwIfIndex: s.idx, Enable: false})
	if err != nil {
		return fmt.Errorf("sw_interface_set_l2_bridge (L3 mode): %w", err)
	}
	s.rep.Reset = append(s.rep.Reset, StateL2Mode+" l3")
	return nil
}

func (s *sanitizer) snapshot() error {
	ids, err := s.cl.ClassifyTableIds(s.ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return fmt.Errorf("classify_table_ids: %w", err)
	}
	s.live, s.taken, s.top = map[uint32]bool{}, map[uint32]bool{}, -1
	for _, id := range ids.Ids {
		s.live[id] = true
		s.see(id)
	}
	return nil
}

func (s *sanitizer) see(t uint32) {
	if int64(t) > s.top {
		s.top = int64(t)
	}
}

// probeTable creates the run's probe table (holds[0]) for the output ACL check. It pops the top
// of the classify pool's free list and goes back there when the run ends.
func (s *sanitizer) probeTable() error {
	_, err := s.createPlaceholder()
	return err
}

// bound is one per-interface classify binding of the interface.
type bound struct {
	state string
	sl    slot
	// table is the bound table; NoIndex for an output ACL slot proven bound to a table that no
	// readback names (found by trying tables).
	table  uint32
	unbind func(ip4, ip6, l2 uint32) error
	done   bool
}

func (b *bound) String() string {
	if b.table == NoIndex {
		return fmt.Sprintf("%s %s table ?", b.state, b.sl.name)
	}
	return fmt.Sprintf("%s %s table %d", b.state, b.sl.name, b.table)
}

// try unbinds table t from b's slot: true when t was the bound table (now removed), false on
// NO_SUCH_TABLE (t is not the bound table, or t does not exist).
func (b *bound) try(t uint32) (bool, error) {
	ip4, ip6, l2 := b.sl.set(t)
	err := b.unbind(ip4, ip6, l2)
	switch {
	case err == nil:
		return true, nil
	case isRetval(err, ErrNoSuchTable):
		return false, nil
	}
	return false, fmt.Errorf("%s: unbind %s table %d: %w", b.state, b.sl.name, t, err)
}

// usable reports whether an unbind through table t can work: t exists (live at the snapshot, or
// taken by another client meanwhile, or brought back by one of this run's placeholders).
func (s *sanitizer) usable(t uint32) bool {
	return s.live[t] || s.taken[t] || (!noResurrect && s.held(t))
}

func (s *sanitizer) held(table uint32) bool {
	for _, h := range s.holds {
		if h == table {
			return true
		}
	}
	return false
}

// clear records b as removed through table t.
func (s *sanitizer) clear(b *bound, t uint32) {
	b.table, b.done = t, true
	s.cleared(b.String(), t)
}

// named removes a binding a readback named: directly when its table exists, else it waits for
// resurrect.
func (s *sanitizer) named(b *bound) error {
	s.see(b.table)
	if s.usable(b.table) {
		ok, err := b.try(b.table)
		if err != nil {
			return err
		}
		if ok {
			s.clear(b, b.table)
			return nil
		}
		if s.held(b.table) {
			return nil // the slot no longer names it: the final readback shows what is left
		}
		// the table was deleted by another client after the snapshot: it is on the free list now
		delete(s.live, b.table)
		delete(s.taken, b.table)
	}
	s.pending = append(s.pending, b)
	return nil
}

func (s *sanitizer) createPlaceholder() (uint32, error) {
	rep, err := s.cl.ClassifyAddDelTable(s.ctx, &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: NoIndex,
		Nbuckets: 2, MemorySize: 64 << 10, MatchNVectors: 1, NextTableIndex: NoIndex, MissNextIndex: NoIndex,
		MaskLen: uint32(len(placeholderMask)), Mask: placeholderMask}) //nolint:gosec // 16
	if err != nil {
		return 0, fmt.Errorf("classify_add_del_table (placeholder): %w", err)
	}
	s.holds = append(s.holds, rep.NewTableIndex)
	s.rep.Placeholders++
	s.rep.Pops = append(s.rep.Pops, rep.NewTableIndex)
	return rep.NewTableIndex, nil
}

// dropPlaceholders deletes this run's placeholder tables in reverse creation order — the free
// list gets them back in the order they were popped — each only after classify_table_info shows
// the placeholder's geometry and signature mask at that index (D-071).
func (s *sanitizer) dropPlaceholders() error {
	var errs []error
	for i := len(s.holds) - 1; i >= 0; i-- {
		idx := s.holds[i]
		info, err := s.cl.ClassifyTableInfo(s.ctx, &classifyapi.ClassifyTableInfo{TableID: idx})
		if err != nil || info.MatchNVectors != 1 || info.SkipNVectors != 0 || !bytes.Equal(info.Mask, placeholderMask) {
			errs = append(errs, fmt.Errorf("placeholder table %d is not ours any more (%v): left in VPP", idx, err))
			continue
		}
		if _, err := s.cl.ClassifyAddDelTable(s.ctx, &classifyapi.ClassifyAddDelTable{IsAdd: false, TableIndex: idx, Nbuckets: 2, MemorySize: 64 << 10,
			MatchNVectors: 1, MaskLen: uint32(len(placeholderMask)), Mask: placeholderMask, NextTableIndex: NoIndex, MissNextIndex: NoIndex}); err != nil { //nolint:gosec // 16
			errs = append(errs, fmt.Errorf("delete placeholder table %d: %w", idx, err))
		}
	}
	s.holds = nil
	return errors.Join(errs...)
}

// resurrect brings back, on demand, the freed indices pending bindings name: it pops the classify
// pool's free list (placeholder tables) until each named index came back, and for an output ACL
// slot bound to an unknown table it tries an unbind through every placeholder it pops. A freed
// index is on the free list, so this never pops a fresh index — unless another client took the
// index meanwhile: a pop above every index seen re-reads the table list, and the tables that
// appeared are tried like the placeholders. At most MaxPlaceholders placeholders (Capped).
func (s *sanitizer) resurrect() error {
	if len(s.pending) == 0 || noResurrect {
		return nil
	}
	want := map[uint32][]*bound{}
	var unknown []*bound
	for _, b := range s.pending {
		if b.table == NoIndex {
			unknown = append(unknown, b)
		} else {
			want[b.table] = append(want[b.table], b)
		}
	}
	s.rep.Wanted = len(s.pending)
	var through []uint32 // tables to try: the latest pop, and tables that appeared meanwhile (the probe table was tried already)
	for {
		for _, t := range through {
			for _, b := range want[t] {
				ok, err := b.try(t)
				if err != nil {
					return err
				}
				if ok {
					s.clear(b, t)
				}
			}
			delete(want, t)
			rest := unknown[:0]
			for _, b := range unknown {
				ok, err := b.try(t)
				if err != nil {
					return err
				}
				if ok {
					s.clear(b, t)
					continue
				}
				rest = append(rest, b)
			}
			unknown = rest
		}
		if len(want) == 0 && len(unknown) == 0 {
			return nil
		}
		if len(s.holds) >= MaxPlaceholders {
			s.rep.Capped = true
			left := make([]uint32, 0, len(want))
			for t := range want {
				left = append(left, t)
			}
			slog.Default().Warn("interface sanitize: placeholder cap reached before a freed classify table index a binding names came back (VPP V19)",
				"sw_if_index", uint32(s.idx), "phase", s.phase, "placeholders", len(s.holds), "cap", MaxPlaceholders,
				"waiting_for", left, "unknown_output_acl_slots", len(unknown), "pops", s.rep.Pops)
			return nil
		}
		q, err := s.createPlaceholder()
		if err != nil {
			return err
		}
		through = append(through[:0], q)
		if int64(q) > s.top {
			// above every index seen: a free-list entry never seen before or — only when another
			// client took the index being waited for — a fresh one; tables that appeared meanwhile
			// may be the ones the bindings name
			s.see(q)
			newly, err := s.reread()
			if err != nil {
				return err
			}
			through = append(through, newly...)
		}
	}
}

// reread reads classify_table_ids again and returns the tables that appeared during the run
// (another client's; their index may be a freed one a binding names): they become taken.
func (s *sanitizer) reread() ([]uint32, error) {
	ids, err := s.cl.ClassifyTableIds(s.ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return nil, fmt.Errorf("classify_table_ids (re-read): %w", err)
	}
	s.rep.Rereads++
	var newly []uint32
	for _, id := range ids.Ids {
		if s.live[id] || s.taken[id] || s.held(id) {
			continue
		}
		s.taken[id] = true
		s.see(id)
		newly = append(newly, id)
	}
	sort.Slice(newly, func(i, j int) bool { return newly[i] < newly[j] })
	return newly, nil
}

// freed reports whether table was deleted before this run (it exists only as a placeholder).
func (s *sanitizer) freed(table uint32) bool { return !s.live[table] }

func (s *sanitizer) cleared(what string, table uint32) {
	if s.taken[table] {
		s.rep.Freed = append(s.rep.Freed, what+" (deleted table; its index was taken by another client during the run, removed through that table)")
		return
	}
	if s.freed(table) {
		s.rep.Freed = append(s.rep.Freed, what+" (deleted table, removed through a placeholder)")
		return
	}
	s.rep.Cleared = append(s.rep.Cleared, what)
}

// unknownMsg reports whether err says the VPP does not know the message (plugin not loaded).
func unknownMsg(err error) bool {
	var unknown *adapter.UnknownMsgError
	return errors.As(err, &unknown)
}

func isRetval(err error, want api.VPPApiError) bool {
	var apiErr api.VPPApiError
	return errors.As(err, &apiErr) && apiErr == want
}

// slot is one table kind of a three-table binding message.
type slot struct {
	name string
	set  func(table uint32) (ip4, ip6, l2 uint32)
}

var aclSlots = []slot{
	{"ip4", func(t uint32) (uint32, uint32, uint32) { return t, NoIndex, NoIndex }},
	{"ip6", func(t uint32) (uint32, uint32, uint32) { return NoIndex, t, NoIndex }},
	{"l2", func(t uint32) (uint32, uint32, uint32) { return NoIndex, NoIndex, t }},
}

func (s *sanitizer) inputUnbind(ip4, ip6, l2 uint32) error {
	_, err := s.cl.InputACLSetInterface(s.ctx, &classifyapi.InputACLSetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: l2, IsAdd: false})
	return err
}

func (s *sanitizer) outputSet(add bool) func(ip4, ip6, l2 uint32) error {
	return func(ip4, ip6, l2 uint32) error {
		_, err := s.cl.OutputACLSetInterface(s.ctx, &classifyapi.OutputACLSetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: l2, IsAdd: add})
		return err
	}
}

func (s *sanitizer) policerUnbind(ip4, ip6, l2 uint32) error {
	_, err := s.cl.PolicerClassifySetInterface(s.ctx, &classifyapi.PolicerClassifySetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: l2, IsAdd: false})
	return err
}

func (s *sanitizer) flowUnbind(ip4, ip6, _ uint32) error {
	_, err := s.cl.FlowClassifySetInterface(s.ctx, &classifyapi.FlowClassifySetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, IsAdd: false})
	return err
}

// inputTables reads the input ACL tables of the interface (classify_table_by_interface).
func (s *sanitizer) inputTables() ([]uint32, error) {
	cur, err := s.cl.ClassifyTableByInterface(s.ctx, &classifyapi.ClassifyTableByInterface{SwIfIndex: s.idx})
	if err != nil {
		return nil, fmt.Errorf("classify_table_by_interface: %w", err)
	}
	return []uint32{cur.IP4TableID, cur.IP6TableID, cur.L2TableID}, nil
}

// policerTables reads the policer classify tables of the interface, per slot, from
// policer_classify_dump(sw_if_index 0) — every index's bindings in VPP 26.06 (dumpEveryIndex).
func (s *sanitizer) policerTables() ([]uint32, error) {
	out := []uint32{NoIndex, NoIndex, NoIndex}
	for i := range out {
		stream, err := s.cl.PolicerClassifyDump(s.ctx, &classifyapi.PolicerClassifyDump{Type: classifyapi.PolicerClassifyTable(i), SwIfIndex: dumpEveryIndex}) //nolint:gosec // 0–2
		if err != nil {
			return nil, err
		}
		for {
			d, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if d.SwIfIndex == s.idx {
				out[i] = d.TableIndex
			}
		}
	}
	return out, nil
}

// flowTables is policerTables for flow_classify_dump (ip4, ip6).
func (s *sanitizer) flowTables() ([]uint32, error) {
	out := []uint32{NoIndex, NoIndex}
	for i := range out {
		stream, err := s.cl.FlowClassifyDump(s.ctx, &classifyapi.FlowClassifyDump{Type: classifyapi.FlowClassifyTable(i), SwIfIndex: dumpEveryIndex}) //nolint:gosec // 0–1
		if err != nil {
			return nil, err
		}
		for {
			d, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if d.SwIfIndex == s.idx {
				out[i] = d.TableIndex
			}
		}
	}
	return out, nil
}

// readback is one kind with an exact readback.
type readback struct {
	state  string
	msg    string
	tables func() ([]uint32, error)
	unbind func(ip4, ip6, l2 uint32) error
}

func (s *sanitizer) readbacks() []readback {
	return []readback{
		{StateInputACL, "classify_table_by_interface", s.inputTables, s.inputUnbind},
		{StatePolicerClassify, "policer_classify_dump", s.policerTables, s.policerUnbind},
		{StateFlowClassify, "flow_classify_dump", s.flowTables, s.flowUnbind},
	}
}

// removeRead reads the bindings of one readback kind and removes them (named).
func (s *sanitizer) removeRead(k readback) error {
	tables, err := k.tables()
	if unknownMsg(err) {
		s.rep.Skipped = append(s.rep.Skipped, k.state)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", k.msg, err)
	}
	for i, t := range tables {
		if t == NoIndex {
			continue
		}
		s.found[k.state] = true
		if err := s.named(&bound{state: k.state, sl: aclSlots[i], table: t, unbind: k.unbind}); err != nil {
			return err
		}
	}
	return nil
}

func (s *sanitizer) inputACL() error        { return s.removeRead(s.readbacks()[0]) }
func (s *sanitizer) policerClassify() error { return s.removeRead(s.readbacks()[1]) }
func (s *sanitizer) flowClassify() error    { return s.removeRead(s.readbacks()[2]) }

// outputACL checks the three output ACL slots with the probe table P (no readback exists):
//  1. an unbind of P succeeds only when the slot names P's index — a freed index the probe
//     table brought back: removed;
//  2. otherwise P is bound and unbound again: VPP binds only an empty slot (an add on a bound
//     slot returns 0 unchanged, in_out_acl.c), so this unbind succeeds exactly when the slot was
//     empty — and leaves it empty;
//  3. otherwise the slot holds another table: an unbind through each existing table (the only
//     probe left), and when none matches, resurrect pops freed indices until one does.
func (s *sanitizer) outputACL() error {
	unbind, bind := s.outputSet(false), s.outputSet(true)
	p := s.holds[0]
	for _, sl := range aclSlots {
		b := &bound{state: StateOutputACL, sl: sl, table: NoIndex, unbind: unbind}
		ok, err := b.try(p)
		if unknownMsg(err) {
			s.rep.Skipped = append(s.rep.Skipped, StateOutputACL)
			return nil
		}
		if err != nil {
			return err
		}
		if ok {
			s.out = append(s.out, b)
			s.clear(b, p)
			continue
		}
		if err := bind(sl.set(p)); err != nil {
			return fmt.Errorf("output_acl_set_interface (probe %s slot with placeholder %d): %w", sl.name, p, err)
		}
		if ok, err = b.try(p); err != nil {
			return err
		}
		if ok {
			continue // the slot was empty and is empty again
		}
		s.out = append(s.out, b)
		for _, t := range s.existing() {
			if ok, err = b.try(t); err != nil {
				return err
			}
			if ok {
				s.clear(b, t)
				break
			}
		}
		if !b.done {
			s.pending = append(s.pending, b)
		}
	}
	return nil
}

// existing returns the tables that exist and are not this run's (live at the snapshot or taken),
// ascending.
func (s *sanitizer) existing() []uint32 {
	out := make([]uint32, 0, len(s.live)+len(s.taken))
	for t := range s.live {
		out = append(out, t)
	}
	for t := range s.taken {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// verify reads back what the run removed: the input ACL always (L2 input ACL is a crash vector),
// policer and flow classify when they had a binding, and every output ACL slot that was bound
// (P is bound and unbound again: that works only on an empty slot). What is left is Unclearable.
func (s *sanitizer) verify() error {
	for _, k := range s.readbacks() {
		if k.state != StateInputACL && !s.found[k.state] {
			continue
		}
		tables, err := k.tables()
		if err != nil {
			return fmt.Errorf("%s (verify): %w", k.msg, err)
		}
		for i, t := range tables {
			if t != NoIndex {
				s.rep.Unclearable = append(s.rep.Unclearable, fmt.Sprintf("%s %s table %d", k.state, aclSlots[i].name, t))
			}
		}
	}
	if len(s.out) == 0 {
		return nil
	}
	bind, p := s.outputSet(true), s.holds[0]
	for _, b := range s.out {
		if !b.done {
			s.rep.Unclearable = append(s.rep.Unclearable, b.String()+" (a deleted table the run did not bring back)")
			continue
		}
		if err := bind(b.sl.set(p)); err != nil {
			return fmt.Errorf("output_acl_set_interface (verify %s slot): %w", b.sl.name, err)
		}
		probe := &bound{state: b.state, sl: b.sl, unbind: b.unbind}
		ok, err := probe.try(p)
		if err != nil {
			return err
		}
		if !ok {
			s.rep.Unclearable = append(s.rep.Unclearable, b.String()+" (still bound after the unbind)")
		}
	}
	return nil
}

func (s *sanitizer) ipClassify() error {
	svc := classifyapi.NewServiceClient(s.c)
	for _, v6 := range []bool{false, true} {
		if _, err := svc.ClassifySetInterfaceIPTable(s.ctx, &classifyapi.ClassifySetInterfaceIPTable{IsIPv6: v6, SwIfIndex: s.idx, TableIndex: NoIndex}); err != nil {
			return fmt.Errorf("classify_set_interface_ip_table (reset ip%s): %w", af(v6), err)
		}
		s.rep.Reset = append(s.rep.Reset, StateIPClassify+" ip"+af(v6))
	}
	return nil
}

func af(v6 bool) string {
	if v6 {
		return "6"
	}
	return "4"
}

func (s *sanitizer) l2Classify() error {
	svc := classifyapi.NewServiceClient(s.c)
	for _, in := range []bool{true, false} {
		req := &classifyapi.ClassifySetInterfaceL2Tables{SwIfIndex: s.idx, IP4TableIndex: NoIndex, IP6TableIndex: NoIndex, OtherTableIndex: NoIndex, IsInput: in}
		if _, err := svc.ClassifySetInterfaceL2Tables(s.ctx, req); err != nil {
			return fmt.Errorf("classify_set_interface_l2_tables (reset %s): %w", dir(in), err)
		}
		s.rep.Reset = append(s.rep.Reset, StateL2Classify+" "+dir(in))
	}
	return nil
}

func dir(in bool) string {
	if in {
		return "input"
	}
	return "output"
}

func (s *sanitizer) adl() error {
	_, err := adlapi.NewServiceClient(s.c).AdlInterfaceEnableDisable(s.ctx, &adlapi.AdlInterfaceEnableDisable{SwIfIndex: s.idx, EnableDisable: false})
	if unknownMsg(err) {
		s.rep.Skipped = append(s.rep.Skipped, StateADL)
		return nil
	}
	if err != nil {
		return fmt.Errorf("adl_interface_enable_disable (reset): %w", err)
	}
	s.rep.Reset = append(s.rep.Reset, StateADL+" adl-input")
	return nil
}

func (s *sanitizer) vxlanBypass() error {
	svc := vxlanapi.NewServiceClient(s.c)
	for _, v6 := range []bool{false, true} {
		_, err := svc.SwInterfaceSetVxlanBypass(s.ctx, &vxlanapi.SwInterfaceSetVxlanBypass{SwIfIndex: s.idx, IsIPv6: v6, Enable: false})
		if unknownMsg(err) {
			s.rep.Skipped = append(s.rep.Skipped, StateVxlanBypass)
			return nil
		}
		if err != nil {
			return fmt.Errorf("sw_interface_set_vxlan_bypass (reset ip%s): %w", af(v6), err)
		}
		s.rep.Reset = append(s.rep.Reset, StateVxlanBypass+" ip"+af(v6))
	}
	return nil
}

func (s *sanitizer) ipsecSPD() error {
	svc := ipsecapi.NewServiceClient(s.c)
	stream, err := svc.IpsecSpdInterfaceDump(s.ctx, &ipsecapi.IpsecSpdInterfaceDump{})
	if unknownMsg(err) {
		s.rep.Skipped = append(s.rep.Skipped, StateIPsecSPD)
		return nil
	}
	if err != nil {
		return fmt.Errorf("ipsec_spd_interface_dump: %w", err)
	}
	var bound []uint32
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if unknownMsg(err) {
			s.rep.Skipped = append(s.rep.Skipped, StateIPsecSPD)
			return nil
		}
		if err != nil {
			return fmt.Errorf("ipsec_spd_interface_dump: %w", err)
		}
		if d.SwIfIndex == s.idx {
			bound = append(bound, d.SpdIndex)
		}
	}
	if len(bound) == 0 {
		return nil
	}
	spds, err := svc.IpsecSpdsDump(s.ctx, &ipsecapi.IpsecSpdsDump{})
	if err != nil {
		return fmt.Errorf("ipsec_spds_dump: %w", err)
	}
	spdID, found := uint32(0), false
	for {
		d, err := spds.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("ipsec_spds_dump: %w", err)
		}
		if !found {
			spdID, found = d.SpdID, true
		}
	}
	what := fmt.Sprintf("%s spd-index %d", StateIPsecSPD, bound[0])
	if !found {
		s.rep.Unclearable = append(s.rep.Unclearable, what+" (no SPD exists to name in the unbind)")
		return nil
	}
	if _, err := svc.IpsecInterfaceAddDelSpd(s.ctx, &ipsecapi.IpsecInterfaceAddDelSpd{IsAdd: false, SwIfIndex: s.idx, SpdID: spdID}); err != nil {
		return fmt.Errorf("ipsec_interface_add_del_spd (unbind stale %s): %w", what, err)
	}
	s.rep.Cleared = append(s.rep.Cleared, what)
	return nil
}
