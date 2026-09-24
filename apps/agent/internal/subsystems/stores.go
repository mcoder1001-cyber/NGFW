package subsystems

// Persisted record stores of the product agent (obligation from D-075/D-076/D-080/D-096: in-memory
// defaults are forbidden in the product agent). Every record carries the D-080 VPP boot identity it was
// written against; a record of another VPP instance never counts (claims expire, applied-once records
// re-apply once). Per-interface claims additionally carry the interface's sw_if_index, so an untagged
// interface that was deleted and re-created under the same name within one VPP instance is not
// silently adopted.
//
//	<state dir>/claims-iface-<owner>.json   iface.ClaimStore (DF-1 attributes on untagged interfaces;
//	                                        df6.ClaimStore is the same type)
//	<state dir>/claims-<family>-<owner>.json KeyedClaims for acl/df2 (acl.ClaimStore), natcommon;
//	                                        written once per transaction (Wiring.ClaimsTxn, TD-11c)
//	<state dir>/boot-<owner>.json           dfkit.FileBootStore (df7.SetBootStore, pcap, …)
//	<state dir>/classify-<owner>.json       classify.FileStore (DF-2 classify tables, ipfix, redirect)

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

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
// answer from a short-lived cache; Invalidate forces the next Resolve to dump again.
type IndexResolver interface {
	Resolve(name string) (uint32, bool)
	Invalidate()
}

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
	// only and mark it dirty; Flush writes it once. Outside a batch every write goes to disk first.
	batch, dirty bool
	writes       int // atomic file writes so far (tests, diagnostics)
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

func (c *fileClaims) claim(key, ifName string) error {
	boot, ok := c.current()
	if !ok {
		return ErrNoIdentity
	}
	r := claimRecord{Key: key, Boot: boot}
	if ifName != "" && c.index != nil {
		c.index.Invalidate() // a claim binds to the index VPP has right now
		idx, ok := c.index.Resolve(ifName)
		if !ok {
			// fail closed like the loader (review N7): a claim without its sw_if_index would make
			// Claimed trust any interface of that name, e.g. one re-created by someone else
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

func (c *fileClaims) claimed(key, ifName string) bool {
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
		idx, ok := c.index.Resolve(ifName)
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
// memory and dirty: the next write (the next flush, or any write outside a batch) persists them.
func (c *fileClaims) flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
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

// Claim implements iface.ClaimStore.
func (c *IfaceClaims) Claim(ifName, holder string) error {
	return c.claim(ifaceKey(ifName, holder), ifName)
}

// Release implements iface.ClaimStore.
func (c *IfaceClaims) Release(ifName, holder string) error {
	return c.release(ifaceKey(ifName, holder))
}

// Claimed implements iface.ClaimStore.
func (c *IfaceClaims) Claimed(ifName, holder string) bool {
	return c.claimed(ifaceKey(ifName, holder), ifName)
}

// KeyedClaims is the persisted single-key claim store (acl.ClaimStore = df2.ClaimStore,
// natcommon.ClaimStore), bound to the VPP boot identity.
type KeyedClaims struct{ *fileClaims }

// OpenKeyedClaims opens <dir>/claims-<family>-<owner>.json.
func OpenKeyedClaims(dir, family, owner string, id IdentitySource) (*KeyedClaims, error) {
	c, err := openClaims(filepath.Join(dir, "claims-"+family+"-"+owner+".json"), id, nil)
	if err != nil {
		return nil, err
	}
	return &KeyedClaims{c}, nil
}

// Claim implements the single-key claim stores.
func (c *KeyedClaims) Claim(key string) error { return c.claim(key, "") }

// Release implements the single-key claim stores.
func (c *KeyedClaims) Release(key string) error { return c.release(key) }

// Claimed implements the single-key claim stores.
func (c *KeyedClaims) Claimed(key string) bool { return c.claimed(key, "") }

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
	refresh func() (map[string]uint32, error)
	now     func() time.Time
}

// NewIndexCache returns a cache that calls refresh at most every ttl.
func NewIndexCache(ttl time.Duration, refresh func() (map[string]uint32, error)) *IndexCache {
	return &IndexCache{ttl: ttl, refresh: refresh, now: time.Now}
}

// Resolve implements IndexResolver.
func (c *IndexCache) Resolve(name string) (uint32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || c.now().Sub(c.at) > c.ttl {
		m, err := c.refresh()
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
