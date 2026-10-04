package ifsanitize

// Table-0 sentinel (INC-vpp-classify-crash mitigation M2, S-ipclassify-zerofill review F2, D-191).
//
// VPP 26.06 grows the per-interface ip4/ip6 classify vector with vec_validate, which zero-fills the
// new slots, and 0 is a valid classify table index. An interface whose slot was zero-filled and never
// reset explicitly gets a FIB_SOURCE_CLASSIFY /32 with a classify DPO to table 0 on its next address
// add. The DPO holds no reference on the table: when table 0 is free (never created, or freed by an
// ACL-plugin MACIP delete, a sanitizer probe, …) the first packet to that address crashes VPP in
// vnet_classify_find_entry (NULL buckets). M1 (ResetIPClassify at every create) closes the mechanism
// for compliant creators only; free pool indices and non-compliant creators (hand-made vppctl
// interfaces, old fixtures) are left. The sentinel covers them until the VPP fix (M3):
//
// The globals owner (D-071) creates one classify table at index 0 right after each VPP (re)connect and
// never deletes it. An armed classify /32 then looks up a live, empty table: every lookup misses, and
// ip4-classify / ip6-classify send the packet to miss_next_index = ~0, i.e. IP_LOOKUP_NEXT_DROP
// (ip_classify.c, "next0 = (t0->miss_next_index < n_next) ? … : next0"). The armed address drops
// instead of crashing VPP.
//
// Winning index 0: the classify table pool is a vppinfra pool, a create pops the most recently freed
// index (pool.h _pool_get: free_indices[n_free-1]). After a VPP start the pool is empty and the first
// create gets 0. When the pool already has freed indices (the agent restarted without VPP, or another
// client created and freed tables first), EnsureSentinel pops sentinel-signature tables until index 0
// comes back, keeps that one and deletes every other pop again in reverse creation order (identity
// re-verified right before each delete) — the free list is then exactly as before, minus index 0.
// A pop above every index seen re-reads classify_table_ids: when index 0 became a live table of
// another client meanwhile, the run stops (SentinelTaken). At most MaxSentinelPops tables are popped.
//
// When index 0 is a live table that is not the sentinel (another client won the race), nothing is
// created or deleted: that table protects the armed addresses while it lives, and the result says so.
// EnsureSentinel never deletes a table it did not create in the same run, and never deletes the
// sentinel.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/internal/vpp"
)

// SentinelIndex is the classify table index the sentinel must hold: the value a zero-filled
// per-interface slot names.
const SentinelIndex uint32 = 0

// sentinelMask is the signature of the sentinel (and of the pops that bring index 0 back): 16 bytes,
// match_n_vectors 1. It is distinct from placeholderMask, so Sanitize never deletes the sentinel and
// EnsureSentinel never deletes a sanitizer placeholder.
var sentinelMask = []byte("vrx-t0-sentinel!")

// sentinelMemory is the sentinel's session heap (it never gets a session; the sanitizer's
// placeholders use the same size).
const sentinelMemory = 64 << 10

// sentinelCleanupTimeout bounds the deletion of the pops after the run, on a context detached from
// the caller's (a cancelled or timed-out ctx must not leak the pops).
const sentinelCleanupTimeout = 10 * time.Second

// MaxSentinelPops bounds the tables one EnsureSentinel run may pop while waiting for index 0.
var MaxSentinelPops = 256

// SentinelState is the outcome of EnsureSentinel.
type SentinelState string

const (
	// SentinelCreated means this run created the sentinel at index 0.
	SentinelCreated SentinelState = "created"
	// SentinelPresent means the sentinel was already at index 0 (a reconnect without a VPP restart).
	SentinelPresent SentinelState = "present"
	// SentinelTaken means index 0 is a live table of another client; nothing was created.
	SentinelTaken SentinelState = "taken"
	// SentinelCapped means MaxSentinelPops pops did not bring index 0 back (ErrSentinelCapped).
	SentinelCapped SentinelState = "capped"
	// SentinelFailed means a VPP call failed (the error says which).
	SentinelFailed SentinelState = "failed"
)

// ErrSentinelCapped is returned with SentinelCapped.
var ErrSentinelCapped = errors.New("classify table sentinel: index 0 did not come back within the pop cap")

// SentinelReport describes one EnsureSentinel run.
type SentinelReport struct {
	State SentinelState
	// Pops are the indices this run popped other than 0, in creation order (deleted again).
	Pops []uint32
	// Left are pops that could not be deleted again (not ours any more, or the delete failed).
	Left []uint32
	// Foreign is index 0's table geometry when State is SentinelTaken.
	Foreign string
}

func (r SentinelReport) String() string {
	s := fmt.Sprintf("state=%s pops=%d", r.State, len(r.Pops))
	if len(r.Left) > 0 {
		s += fmt.Sprintf(" left=%v", r.Left)
	}
	if r.Foreign != "" {
		s += " foreign=" + r.Foreign
	}
	return s
}

// IsSentinel reports whether a classify_table_info reply describes the sentinel's signature.
func IsSentinel(info *classifyapi.ClassifyTableInfoReply) bool {
	return info != nil && info.MatchNVectors == 1 && info.SkipNVectors == 0 && info.NextTableIndex == NoIndex &&
		bytes.Equal(info.Mask, sentinelMask)
}

// SentinelTable is the classify_add_del_table create of the sentinel (exported for the host check and
// the tests): one bucket, no next table, miss → drop.
func SentinelTable() *classifyapi.ClassifyAddDelTable {
	return &classifyapi.ClassifyAddDelTable{IsAdd: true, TableIndex: NoIndex, Nbuckets: 1, MemorySize: sentinelMemory,
		MatchNVectors: 1, NextTableIndex: NoIndex, MissNextIndex: NoIndex,
		MaskLen: uint32(len(sentinelMask)), Mask: append([]byte(nil), sentinelMask...)} //nolint:gosec // 16
}

// EnsureSentinel makes sure the classify table at index 0 exists, creating the sentinel there when
// index 0 is free (see the file comment). Globals owner only (D-071): the table is VPP-global state.
// It is idempotent: with the sentinel present it only reads.
func EnsureSentinel(ctx context.Context, c vpp.Client) (SentinelReport, error) {
	cl := classifyapi.NewServiceClient(c)
	rep := SentinelReport{}
	live, err := tableIDs(ctx, cl)
	if err != nil {
		rep.State = SentinelFailed
		return rep, err
	}
	if live[SentinelIndex] {
		st, foreign, err := inspectZero(ctx, cl)
		if err != nil {
			rep.State = SentinelFailed
			return rep, err
		}
		if st != "" {
			rep.State, rep.Foreign = st, foreign
			return rep, nil
		}
		delete(live, SentinelIndex) // deleted between the two reads: pop it back
	}
	maxSeen := int64(-1)
	for id := range live {
		maxSeen = max(maxSeen, int64(id))
	}
	var runErr error
	for {
		if len(rep.Pops) >= MaxSentinelPops {
			rep.State, runErr = SentinelCapped, ErrSentinelCapped
			break
		}
		r, err := cl.ClassifyAddDelTable(ctx, SentinelTable())
		if err != nil {
			rep.State, runErr = SentinelFailed, fmt.Errorf("classify_add_del_table (sentinel): %w", err)
			break
		}
		if r.NewTableIndex == SentinelIndex {
			rep.State = SentinelCreated
			break
		}
		rep.Pops = append(rep.Pops, r.NewTableIndex)
		if int64(r.NewTableIndex) <= maxSeen {
			continue
		}
		// above every index seen: the free list may be empty (the pool grew), so index 0 may have been
		// taken by another client since the first read
		maxSeen = int64(r.NewTableIndex)
		again, err := tableIDs(ctx, cl)
		if err != nil {
			rep.State, runErr = SentinelFailed, err
			break
		}
		if again[SentinelIndex] {
			st, foreign, err := inspectZero(ctx, cl)
			if err != nil {
				rep.State, runErr = SentinelFailed, err
				break
			}
			if st != "" { // "present" here means another agent placed a sentinel meanwhile
				rep.State, rep.Foreign = st, foreign
				break
			}
		}
	}
	// Cleanup runs on its own deadline, detached from ctx (review R14 #1 / R78 #1): a timeout or a
	// shutdown mid-run must still delete the pops, or up to MaxSentinelPops tables (~16 MiB) stay in
	// VPP for its lifetime.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sentinelCleanupTimeout)
	defer cancel()
	if err := dropSentinelPops(cctx, cl, &rep); err != nil {
		runErr = errors.Join(runErr, err)
	}
	return rep, runErr
}

// inspectZero reads table 0: SentinelPresent or SentinelTaken (with its geometry), or "" when the
// table no longer exists.
func inspectZero(ctx context.Context, cl classifyapi.RPCService) (SentinelState, string, error) {
	info, err := cl.ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: SentinelIndex})
	var apiErr api.VPPApiError
	switch {
	case errors.As(err, &apiErr):
		return "", "", nil // freed after classify_table_ids
	case err != nil:
		return "", "", fmt.Errorf("classify_table_info 0: %w", err)
	case IsSentinel(info):
		return SentinelPresent, "", nil
	}
	return SentinelTaken, fmt.Sprintf("nbuckets=%d match=%d skip=%d next=%d miss=%d sessions=%d",
		info.Nbuckets, info.MatchNVectors, info.SkipNVectors, int32(info.NextTableIndex), int32(info.MissNextIndex), info.ActiveSessions), nil //nolint:gosec // ~0 printed as -1
}

// dropSentinelPops deletes the run's pops in reverse creation order (the free list gets them back as it
// was), each only after classify_table_info shows the sentinel signature at that index.
func dropSentinelPops(ctx context.Context, cl classifyapi.RPCService, rep *SentinelReport) error {
	var errs []error
	for i := len(rep.Pops) - 1; i >= 0; i-- {
		idx := rep.Pops[i]
		info, err := cl.ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: idx})
		if err != nil || !IsSentinel(info) {
			rep.Left = append(rep.Left, idx)
			errs = append(errs, fmt.Errorf("sentinel pop %d is not ours any more (%v): left in VPP", idx, err))
			continue
		}
		del := SentinelTable()
		del.IsAdd, del.TableIndex = false, idx
		if _, err := cl.ClassifyAddDelTable(ctx, del); err != nil {
			rep.Left = append(rep.Left, idx)
			errs = append(errs, fmt.Errorf("delete sentinel pop %d: %w", idx, err))
		}
	}
	return errors.Join(errs...)
}

func tableIDs(ctx context.Context, cl classifyapi.RPCService) (map[uint32]bool, error) {
	rep, err := cl.ClassifyTableIds(ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return nil, fmt.Errorf("classify_table_ids: %w", err)
	}
	m := make(map[uint32]bool, len(rep.Ids))
	for _, id := range rep.Ids {
		m[id] = true
	}
	return m, nil
}
