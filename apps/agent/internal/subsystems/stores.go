package subsystems

// Persisted record stores of the product agent (obligation from D-075/D-076/D-080/D-096: in-memory
// defaults are forbidden in the product agent). Every record carries the D-080 VPP boot identity it was
// written against; a record of another VPP instance never counts (claims expire, applied-once records
// re-apply once). Per-interface claims additionally carry the interface's sw_if_index, so an untagged
// interface that was deleted and re-created under the same name within one VPP instance is not
// silently adopted.
//
//	<state dir>/claims-iface-<owner>.json   iface.ClaimStore (DF-1 attributes on untagged interfaces,
//	                                        and every per-interface claim of df6/dfkit families)
//	<state dir>/claims-<family>-<owner>.json KeyedClaims for acl/df2 (acl.ClaimStore), natcommon;
//	                                        PairClaims for df6 keyed / per-boot claims (df6.ClaimStore)
//	<state dir>/boot-<owner>.json           dfkit.FileBootStore (df7.SetBootStore, pcap, …)
//	<state dir>/classify-<owner>.json       classify.FileStore (DF-2 classify tables, ipfix, redirect)
//
// Claim-first leftovers (TD-11b, review 3.3 / fix round 1 L5). Descriptors claim BEFORE the VPP write
// and release when the write fails; an agent crash between the claim and the write (or a failed
// release) leaves a claim on nothing. That never blocks a later Create — the claim is reused, the
// absent object is simply written — but within one VPP instance nothing removes it: the only
// cleanup is Prune on a VPP boot-identity change (Connected). While it stays, the claimed holder
// counts as ours on that untagged interface. For interface.admin-state that is visible: if someone
// else sets the NIC admin-up, Retrieve reports the admin state as ours, and a resync whose desired
// state does not name it Deletes it — sets the NIC admin DOWN, a state we never wrote. The cost is
// accepted (it replaces the worse orphan of write-then-claim: an object in VPP nobody owns);
// tech-debt: after the first successful resync, drop claims whose key is neither desired nor present.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// IdentitySource returns the boot identity of the VPP instance the agent is connected to (zero
// when not known yet). The agent updates it on every (re)connect before the resync.
type IdentitySource interface {
	Identity() bootid.Identity
}

// ErrClaimUnbound means an interface claim could not be bound to the interface's current sw_if_index
// (it is not in VPP's table right now, or the dump failed); nothing is recorded.
var ErrClaimUnbound = errors.New("claim store: interface has no sw_if_index in VPP")

// IndexResolver maps an untagged interface's VPP name to its current sw_if_index. Resolve may
// answer from a short-lived cache; a refresh is bounded by ctx — the caller's transaction, not a
// timeout of the store's own (TD-11b, review R2-stores). Invalidate forces the next Resolve to
// dump again.
type IndexResolver interface {
	Resolve(ctx context.Context, name string) (uint32, bool)
	Invalidate()
}

// legacyBound caps the sw_if_index lookup of the context-less ClaimStore methods (Claim, Claimed),
// which callers without a context use (iface.Table.Owns in Retrieve); callers with one use
// ClaimContext / ClaimedContext (iface.ContextClaimStore: the DF-1 attributes, dfkit.ClaimFirst).
const legacyBound = 5 * time.Second

// Identity is the agent's current VPP boot identity (IdentitySource), set on every VPP connect.
type Identity struct {
	mu sync.RWMutex
	id bootid.Identity
}

// Identity implements IdentitySource.
func (i *Identity) Identity() bootid.Identity {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.id
}

// Set records a new identity and reports whether it differs from the previous one.
func (i *Identity) Set(id bootid.Identity) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	changed := !i.id.Equal(id)
	i.id = id
	return changed
}

// claimRecord is one persisted claim.
type claimRecord struct {
	Key       string `json:"key"`                   // "<if name>|<holder>" for interface claims, the claim key otherwise
	Boot      string `json:"boot"`                  // bootid.Identity.String()
	SwIfIndex *int64 `json:"sw_if_index,omitempty"` // interface claims only
}

// fileClaims is the persisted claim set shared by both claim store shapes.
type fileClaims struct {
	mu    sync.Mutex
	path  string
	id    IdentitySource
	index IndexResolver
	recs  map[string]claimRecord
	// batch: a transaction is open (Begin … Flush, TD-11c review 3.2): writes change recs in memory
	// and mark it dirty; Flush writes it once. Outside a batch every write goes to disk first.
	batch, dirty bool
	writes       int // atomic file writes so far (tests, diagnostics)
	// journal (KeyedClaims, TD-11c fix round 1, D-133): inside a batch every Claim/Release is appended
	// here with one write(2) BEFORE it returns — so before the descriptor writes VPP (TD-11b's
	// claim-first order) — and survives the death of the agent process in the page cache. No fsync:
	// what loses the page cache (kernel crash, power loss) restarts VPP too, which voids every claim
	// (D-080). Every snapshot write (Flush: one atomic, fsync'd write per transaction) empties it;
	// opening the store replays it over the snapshot. "" = no journal (IfaceClaims never batches).
	journal string
	jf      *os.File // open while the current batch appends
	jsize   int64    // bytes of whole lines in the journal (a failed append is cut back to it)
}

// journalLine is one journal record: a claim (Set) or a release (Del).
type journalLine struct {
	Set *claimRecord `json:"set,omitempty"`
	Del string       `json:"del,omitempty"`
}

func openClaims(path string, id IdentitySource, index IndexResolver) (*fileClaims, error) {
	c := &fileClaims{path: path, id: id, index: index, recs: map[string]claimRecord{}}
	raw, err := os.ReadFile(path) //nolint:gosec // the agent's own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
		return c, nil
	case err != nil:
		return nil, fmt.Errorf("claim store %s: %w", path, err)
	}
	var recs []claimRecord
	if err := json.Unmarshal(raw, &recs); err != nil {
		// fail closed: a corrupt claim file must not make the agent adopt or forget objects silently
		return nil, fmt.Errorf("claim store %s is corrupt (move it aside to drop every claim): %w", path, err)
	}
	for _, r := range recs {
		c.recs[r.Key] = r
	}
	return c, nil
}

func (c *fileClaims) current() (string, bool) {
	id := c.id.Identity()
	if id.IsZero() {
		return "", false
	}
	return id.String(), true
}

func (c *fileClaims) claim(ctx context.Context, key, ifName string) error {
	boot, ok := c.current()
	if !ok {
		return ErrNoIdentity
	}
	r := claimRecord{Key: key, Boot: boot}
	if ifName != "" && c.index != nil {
		c.index.Invalidate() // a claim binds to the index VPP has right now
		idx, ok := c.index.Resolve(ctx, ifName)
		if !ok {
			// fail closed like the loader (review N7): a claim without its sw_if_index would make
			// Claimed trust any interface of that name, e.g. one re-created by someone else
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("%w: %q (claim %s not recorded): %w", ErrClaimUnbound, ifName, key, err)
			}
			return fmt.Errorf("%w: %q (claim %s not recorded)", ErrClaimUnbound, ifName, key)
		}
		v := int64(idx)
		r.SwIfIndex = &v
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.setLocked(key, &r)
}

func (c *fileClaims) release(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.recs[key]; !ok {
		return nil
	}
	return c.setLocked(key, nil)
}

func (c *fileClaims) claimed(ctx context.Context, key, ifName string) bool {
	boot, ok := c.current()
	if !ok {
		return false
	}
	c.mu.Lock()
	r, found := c.recs[key]
	c.mu.Unlock()
	if !found || r.Boot != boot {
		return false
	}
	if r.SwIfIndex != nil && ifName != "" && c.index != nil {
		idx, ok := c.index.Resolve(ctx, ifName)
		return ok && int64(idx) == *r.SwIfIndex
	}
	return true
}

// Prune drops every record of another VPP instance (called after an identity change).
func (c *fileClaims) Prune() (int, error) {
	boot, ok := c.current()
	if !ok {
		return 0, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	next := map[string]claimRecord{}
	for k, r := range c.recs {
		if r.Boot == boot {
			next[k] = r
		}
	}
	n := len(c.recs) - len(next)
	if n == 0 {
		return 0, nil
	}
	if err := c.replaceLocked(next); err != nil {
		return 0, err
	}
	return n, nil
}

// Persistent marks the claim stores as surviving an agent restart (dfkit/persist, TD-11b).
func (*fileClaims) Persistent() bool { return true }

// Len returns the number of records (tests, diagnostics).
func (c *fileClaims) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.recs)
}

func (c *fileClaims) copyLocked() map[string]claimRecord {
	out := make(map[string]claimRecord, len(c.recs)+1)
	for k, v := range c.recs {
		out[k] = v
	}
	return out
}

// setLocked records rec under key (nil: removes key): in a batch in memory only, otherwise through
// replaceLocked.
func (c *fileClaims) setLocked(key string, rec *claimRecord) error {
	if c.batch {
		if err := c.appendLocked(key, rec); err != nil {
			return err // nothing recorded: the caller must not write VPP
		}
		if rec == nil {
			delete(c.recs, key)
		} else {
			c.recs[key] = *rec
		}
		c.dirty = true
		return nil
	}
	next := c.copyLocked()
	if rec == nil {
		delete(next, key)
	} else {
		next[key] = *rec
	}
	return c.replaceLocked(next)
}

// replaceLocked makes next the record set: in a batch at once (Flush writes it), otherwise only once
// it is on disk, so outside a batch memory never runs ahead of the file.
func (c *fileClaims) replaceLocked(next map[string]claimRecord) error {
	if c.batch {
		c.recs, c.dirty = next, true
		return nil
	}
	if err := c.flushLocked(next); err != nil {
		return err
	}
	c.recs, c.dirty = next, false
	return nil
}

// begin opens a batch: until flush, Claim/Release/Prune change the in-memory set only.
func (c *fileClaims) begin() {
	c.mu.Lock()
	c.batch = true
	c.mu.Unlock()
}

// flush ends the batch and writes the set once when it changed. A failed write keeps the records in
// memory and dirty, and in the journal: the next write (the next flush, or any write outside a
// batch) persists them.
func (c *fileClaims) flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.closeJournalLocked()
	c.batch = false
	if !c.dirty {
		return nil
	}
	if err := c.flushLocked(c.recs); err != nil {
		return err
	}
	c.dirty = false
	return nil
}

func (c *fileClaims) flushLocked(m map[string]claimRecord) error {
	recs := make([]claimRecord, 0, len(m))
	for _, r := range m {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Key < recs[j].Key })
	raw, err := json.MarshalIndent(recs, "", " ")
	if err != nil {
		return err
	}
	if err := atomicWrite(c.path, raw); err != nil {
		return err
	}
	c.writes++
	c.truncateJournalLocked() // the snapshot is durable: the journal's records are in it
	return nil
}

// appendLocked appends key's claim (rec) or release (nil) to the journal with one write(2); a failed
// or short write is cut back so the journal only ever holds whole lines.
func (c *fileClaims) appendLocked(key string, rec *claimRecord) error {
	if c.journal == "" {
		return nil
	}
	line := journalLine{Del: key}
	if rec != nil {
		line = journalLine{Set: rec}
	}
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if c.jf == nil {
		f, err := os.OpenFile(c.journal, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // the agent's own state file
		if err != nil {
			return fmt.Errorf("claim journal %s: %w", filepath.Base(c.journal), err)
		}
		fi, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("claim journal %s: %w", filepath.Base(c.journal), err)
		}
		c.jf, c.jsize = f, fi.Size()
	}
	n, err := c.jf.Write(raw)
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_ = c.jf.Truncate(c.jsize)
		return fmt.Errorf("claim journal %s: %w", filepath.Base(c.journal), err)
	}
	c.jsize += int64(n)
	return nil
}

// truncateJournalLocked empties the journal after a durable snapshot. A failure is harmless: the
// replay of lines the snapshot already holds is idempotent.
func (c *fileClaims) truncateJournalLocked() {
	switch {
	case c.journal == "":
	case c.jf != nil:
		if c.jf.Truncate(0) == nil {
			c.jsize = 0
		}
	default:
		if err := os.Truncate(c.journal, 0); err == nil || errors.Is(err, os.ErrNotExist) {
			c.jsize = 0
		}
	}
}

func (c *fileClaims) closeJournalLocked() {
	if c.jf != nil {
		_ = c.jf.Close()
		c.jf = nil
	}
}

// splitLines splits raw at every newline (a final segment without one is the torn tail).
func splitLines(raw []byte) [][]byte {
	var out [][]byte
	for start, i := 0, 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == '\n' {
			out = append(out, raw[start:i])
			start = i + 1
		}
	}
	return out
}

// replayJournal applies the journal at path over the loaded snapshot (the agent died inside a
// transaction) and compacts it into the snapshot. A torn last line — the process died inside
// write(2) — is dropped; a bad line before it fails closed like a corrupt snapshot.
func (c *fileClaims) replayJournal(path string) error {
	c.journal = path
	raw, err := os.ReadFile(path) //nolint:gosec // the agent's own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("claim journal %s: %w", path, err)
	}
	lines, whole := splitLines(raw), 0
	for i, l := range lines {
		if len(l) == 0 {
			continue
		}
		var jl journalLine
		if err := json.Unmarshal(l, &jl); err != nil || (jl.Set == nil) == (jl.Del == "") {
			if i == len(lines)-1 { // torn tail (no trailing newline): never completed
				break
			}
			return fmt.Errorf("claim journal %s is corrupt at line %d (move it aside to drop the claims of the interrupted transaction): %v", path, i+1, err)
		}
		if jl.Set != nil {
			c.recs[jl.Set.Key] = *jl.Set
		} else {
			delete(c.recs, jl.Del)
		}
		whole += len(l) + 1
	}
	if len(raw) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.flushLocked(c.recs); err != nil {
		// keep the journal (its whole lines hold the records) and write the snapshot at the next flush
		if whole < len(raw) {
			_ = os.Truncate(path, int64(whole))
		}
		c.dirty = true
	}
	return nil
}

// atomicWrite writes raw to path via a fsynced temp file and a rename (0600).
func atomicWrite(path string, raw []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // agent state file
	if err != nil {
		return fmt.Errorf("state %s: %w", filepath.Base(path), err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return fmt.Errorf("state %s: %w", filepath.Base(path), err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("state %s: %w", filepath.Base(path), err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("state %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// the rename is durable only once the directory entry is (review N7)
	d, err := os.Open(filepath.Dir(path)) //nolint:gosec // the agent's state dir
	if err != nil {
		return fmt.Errorf("state %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("state %s: fsync dir: %w", filepath.Base(path), err)
	}
	return nil
}

// IfaceClaims is the persisted iface.ClaimStore: (interface VPP name, holder descriptor) pairs,
// bound to the VPP boot identity and the interface's sw_if_index.
type IfaceClaims struct{ *fileClaims }

// OpenIfaceClaims opens <dir>/claims-iface-<owner>.json.
func OpenIfaceClaims(dir, owner string, id IdentitySource, index IndexResolver) (*IfaceClaims, error) {
	c, err := openClaims(filepath.Join(dir, "claims-iface-"+owner+".json"), id, index)
	if err != nil {
		return nil, err
	}
	return &IfaceClaims{c}, nil
}

func ifaceKey(ifName, holder string) string { return ifName + "|" + holder }

// Claim implements iface.ClaimStore (for callers without a context: the lookup is capped at
// legacyBound).
func (c *IfaceClaims) Claim(ifName, holder string) error {
	ctx, cancel := context.WithTimeout(context.Background(), legacyBound)
	defer cancel()
	return c.ClaimContext(ctx, ifName, holder)
}

// ClaimContext implements iface.ContextClaimStore: the claim binds to the interface's current
// sw_if_index, looked up within ctx (R2-stores).
func (c *IfaceClaims) ClaimContext(ctx context.Context, ifName, holder string) error {
	return c.claim(ctx, ifaceKey(ifName, holder), ifName)
}

// Release implements iface.ClaimStore.
func (c *IfaceClaims) Release(ifName, holder string) error {
	return c.release(ifaceKey(ifName, holder))
}

// Claimed implements iface.ClaimStore (for callers without a context, as Claim).
func (c *IfaceClaims) Claimed(ifName, holder string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), legacyBound)
	defer cancel()
	return c.ClaimedContext(ctx, ifName, holder)
}

// ClaimedContext implements iface.ContextClaimStore.
func (c *IfaceClaims) ClaimedContext(ctx context.Context, ifName, holder string) bool {
	return c.claimed(ctx, ifaceKey(ifName, holder), ifName)
}

// BindsInterfaceIndex reports that every claim of this store is bound to an interface's
// sw_if_index: ids that are not interface names cannot be claimed here (df6 checks it, TD-11b).
func (*IfaceClaims) BindsInterfaceIndex() bool { return true }

// KeyedClaims is the persisted single-key claim store (acl.ClaimStore = df2.ClaimStore,
// natcommon.ClaimStore), bound to the VPP boot identity.
type KeyedClaims struct{ *fileClaims }

// OpenKeyedClaims opens <dir>/claims-<family>-<owner>.json.
func OpenKeyedClaims(dir, family, owner string, id IdentitySource) (*KeyedClaims, error) {
	c, err := openClaims(filepath.Join(dir, "claims-"+family+"-"+owner+".json"), id, nil)
	if err != nil {
		return nil, err
	}
	if err := c.replayJournal(filepath.Join(dir, "claims-"+family+"-"+owner+".journal")); err != nil {
		return nil, err
	}
	return &KeyedClaims{c}, nil
}

// Claim implements the single-key claim stores.
func (c *KeyedClaims) Claim(key string) error { return c.claim(context.Background(), key, "") }

// Release implements the single-key claim stores.
func (c *KeyedClaims) Release(key string) error { return c.release(key) }

// Claimed implements the single-key claim stores.
func (c *KeyedClaims) Claimed(key string) bool { return c.claimed(context.Background(), key, "") }

// Pairs returns the (id, holder) view of this store (PairClaims), sharing its file.
func (c *KeyedClaims) Pairs() *PairClaims { return &PairClaims{c.fileClaims} }

// PairClaims is the persisted (id, holder) claim store for claims whose id is NOT an interface
// name — df6's keyed and per-boot claims (df6.ClaimStore, df6.WithClaims(Wiring.PairClaims("df6"))).
// It is bound to the VPP boot identity like KeyedClaims, never to an interface index: through
// IfaceClaims every such claim failed with ErrClaimUnbound (TD-11b).
type PairClaims struct{ *fileClaims }

func pairKey(id, holder string) string { return id + "|" + holder }

// Claim implements df6.ClaimStore.
func (c *PairClaims) Claim(id, holder string) error {
	return c.claim(context.Background(), pairKey(id, holder), "")
}

// Release implements df6.ClaimStore.
func (c *PairClaims) Release(id, holder string) error { return c.release(pairKey(id, holder)) }

// Claimed implements df6.ClaimStore.
func (c *PairClaims) Claimed(id, holder string) bool {
	return c.claimed(context.Background(), pairKey(id, holder), "")
}

// Begin opens a transaction on the store (TD-11c, review 3.2): Claim, Release and Prune change only
// the in-memory set — Claimed reads it, so the transaction sees its own claims — until Flush writes
// it once (the same atomic, fsync'd replace as an immediate write). The agent brackets every
// transaction with Wiring.ClaimsTxn. Trade-off: an agent process that dies inside a transaction loses
// that transaction's keyed claims (an untagged object it created stays in VPP unclaimed, invisible,
// until VPP restarts); before, it paid a whole-file rewrite with two fsyncs per claim (O(n²)).
func (c *KeyedClaims) Begin() { c.begin() }

// Flush ends the transaction Begin opened: one write when the set changed, none otherwise. A failed
// write keeps the records in memory and dirty; the next write persists them.
func (c *KeyedClaims) Flush() error { return c.flush() }

// ClaimsTxn opens one transaction on every keyed claim store opened so far and returns the function
// that ends it: each store that changed is written once (KeyedClaims.Begin/Flush). The agent calls it
// around every transaction (Service: ClaimsTxn); a store opened later writes immediately.
func (w *Wiring) ClaimsTxn() (flush func() error) {
	w.storesMu.Lock()
	open := make([]*KeyedClaims, 0, len(w.keyed))
	for _, k := range w.keyed {
		k.Begin()
		open = append(open, k)
	}
	w.storesMu.Unlock()
	return func() error {
		var errs []error
		for _, k := range open {
			if err := k.Flush(); err != nil {
				errs = append(errs, fmt.Errorf("claim store %s: %w", filepath.Base(k.path), err))
			}
		}
		return errors.Join(errs...)
	}
}

// IndexCache is an IndexResolver over a cached name → sw_if_index map refreshed at most every ttl.
type IndexCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	at      time.Time
	m       map[string]uint32
	refresh func(ctx context.Context) (map[string]uint32, error)
	now     func() time.Time
}

// NewIndexCache returns a cache that calls refresh at most every ttl; refresh gets the context of
// the Resolve that triggered it (the caller's deadline, R2-stores).
func NewIndexCache(ttl time.Duration, refresh func(ctx context.Context) (map[string]uint32, error)) *IndexCache {
	return &IndexCache{ttl: ttl, refresh: refresh, now: time.Now}
}

// Resolve implements IndexResolver. A refresh runs within ctx's deadline; a ctx without one (the
// resync on a VPP connect runs on the agent's run context, and govpp's reply timeout may be 0) is
// capped at legacyBound, so a stalled VPP never holds the cache mutex — and every Claimed waiting
// on it — forever (TD-11b fix round 1, review L1; TD-9's global reply timeout does not replace it).
func (c *IndexCache) Resolve(ctx context.Context, name string) (uint32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || c.now().Sub(c.at) > c.ttl {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, legacyBound)
			defer cancel()
		}
		m, err := c.refresh(ctx)
		if err != nil {
			return 0, false
		}
		c.m, c.at = m, c.now()
	}
	idx, ok := c.m[name]
	return idx, ok
}

// Invalidate implements IndexResolver.
func (c *IndexCache) Invalidate() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

// PairClaims opens (once per family, sharing KeyedClaims' file) the persisted (id, holder) claim
// store of a descriptor family whose claim ids are not interface names: "df6" (df6.WithClaims).
func (w *Wiring) PairClaims(family string) (*PairClaims, error) {
	k, err := w.KeyedClaims(family)
	if err != nil {
		return nil, err
	}
	return k.Pairs(), nil
}

// Register builds every store (persisted in env.StateDir), installs the process-wide ones for
// env.Owner, and registers the descriptors of this build with r in dependency-friendly order (the
// scheduler's tie breaker): VRFs, interface creators, alias, attributes, addresses, routes.
//
// It refuses to start the agent when a registered descriptor does not declare how it records
// ownership (CheckPersistent or RecordsNoOwnership, TD-11b fix round 1) or records ownership claims
// or applied-once records in a store that does not survive an agent restart (TD-11b, review 3.2;
// D-075): the in-memory defaults of the descriptor families are for unit tests, and in the product
// agent they forget, on every agent restart, which objects on untagged interfaces are ours — which
// are then neither reported nor ever deleted. Every descriptor that implements CheckPersistent
// (dfkit/persist) is checked, through wrappers.
func Register(r scheduler.Registry, env Env) (*Wiring, error) {
	g := &guardRegistry{Registry: r}
	w, err := register(g, env)
	if err != nil {
		return nil, err
	}
	if err := RequirePersistent(g.ds); err != nil {
		return nil, err
	}
	return w, nil
}

// guardRegistry records every descriptor registered through it.
type guardRegistry struct {
	scheduler.Registry
	ds []scheduler.Descriptor
}

// Register implements scheduler.Registry.
func (g *guardRegistry) Register(d scheduler.Descriptor) {
	g.Registry.Register(d)
	g.ds = append(g.ds, d)
}

// ErrVolatileStores is returned (wrapping every persist.ErrVolatile / df6.ErrClaimStoreKind finding)
// when the product wiring registered a descriptor with an in-memory ownership store.
var ErrVolatileStores = errors.New("subsystems: refusing to start: a descriptor records ownership in a store that does not survive an agent restart")

// ErrUndeclaredDescriptors is returned (wrapping persist.ErrUndeclared / ErrConflictingDeclaration)
// when the product wiring registered a descriptor that does not declare how it records ownership
// (TD-11b fix round 1, review M1): a feature row registering a family under its anchor must give
// every descriptor CheckPersistent or RecordsNoOwnership.
var ErrUndeclaredDescriptors = errors.New("subsystems: refusing to start: a descriptor does not declare how it records ownership (CheckPersistent or RecordsNoOwnership)")

// RequirePersistent runs the completeness and persistence checks (dfkit/persist) of every
// descriptor in ds: each must declare how it records ownership, and every store it records in
// must survive an agent restart.
func RequirePersistent(ds []scheduler.Descriptor) error {
	var undeclared, volatile []error
	for _, d := range ds {
		if err := persist.Declared(d); err != nil {
			undeclared = append(undeclared, fmt.Errorf("%s (%T): %w", d.Name(), d, err))
			continue
		}
		if err := persist.Check(d); err != nil {
			volatile = append(volatile, err)
		}
	}
	var errs []error
	if len(undeclared) > 0 {
		errs = append(errs, fmt.Errorf("%w: %w", ErrUndeclaredDescriptors, errors.Join(undeclared...)))
	}
	if len(volatile) > 0 {
		errs = append(errs, fmt.Errorf("%w: %w", ErrVolatileStores, errors.Join(volatile...)))
	}
	return errors.Join(errs...)
}
