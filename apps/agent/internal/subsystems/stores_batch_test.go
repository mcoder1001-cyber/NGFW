package subsystems

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// TD-11c (review 3.2): the keyed claim stores (acl/df2, natcommon) write once per transaction — the
// agent brackets every transaction with Wiring.ClaimsTxn — instead of rewriting the whole file with
// two fsyncs on every Claim/Release (O(n²) at NAT static-mapping or ACL scale). The one write is the
// same atomic, fsync'd replace as before; a transaction sees its own claims; outside a transaction
// every write is still immediate.
func TestKeyedClaimsFlushOncePerTxn(t *testing.T) {
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "w1")
	if err != nil {
		t.Fatal(err)
	}
	w, err := Register(scheduler.NewRegistry(), Env{Client: coretest.New(), Owner: "w1", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	w.identity.Set(bootid.Identity{BootID: "b", PID: 1, StartTime: 1})
	nat, err := w.KeyedClaims("nat")
	if err != nil {
		t.Fatal(err)
	}
	acl, err := w.KeyedClaims("acl")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "claims-nat-w1.json")
	reopen := func() *KeyedClaims {
		t.Helper()
		k, err := OpenKeyedClaims(dir, "nat", "w1", w.identity)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	key := func(i int) string { return fmt.Sprintf("static/10.1.%d.%d:%d", i/250, i%250, 1000+i) }

	// One transaction: 2000 claims and one release → nothing on disk until the end, then one write.
	flush := w.ClaimsTxn()
	const n = 2000
	for i := 0; i < n; i++ {
		if err := nat.Claim(key(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := nat.Release(key(7)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("claims written before the end of the transaction: %v", err)
	}
	if !nat.Claimed(key(0)) || nat.Claimed(key(7)) {
		t.Fatal("the transaction does not see its own claims")
	}
	if err := flush(); err != nil {
		t.Fatal(err)
	}
	if nat.writes != 1 || acl.writes != 0 {
		t.Fatalf("writes nat=%d acl=%d, want 1 and 0 (an unchanged store is not rewritten)", nat.writes, acl.writes)
	}
	if k := reopen(); k.Len() != n-1 || !k.Claimed(key(n-1)) || k.Claimed(key(7)) {
		t.Fatalf("after the transaction the file has %d records", k.Len())
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode %v %v", fi, err)
	}

	// Outside a transaction: immediate, as before.
	if err := nat.Claim("outside"); err != nil {
		t.Fatal(err)
	}
	if nat.writes != 2 || !reopen().Claimed("outside") {
		t.Fatalf("an out-of-transaction claim is not written at once (writes %d)", nat.writes)
	}
	// An empty transaction writes nothing.
	if err := w.ClaimsTxn()(); err != nil || nat.writes != 2 {
		t.Fatalf("empty transaction: %v, writes %d", err, nat.writes)
	}

	// A failed snapshot write keeps the records in memory, dirty and in the journal; the next
	// transaction end writes them.
	flush = w.ClaimsTxn()
	if err := nat.Claim("late"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0o700); err != nil { // the atomic write's temp file cannot be created
		t.Fatal(err)
	}
	if err := flush(); err == nil {
		t.Fatal("a failed write was not reported")
	}
	if err := os.Remove(path + ".tmp"); err != nil {
		t.Fatal(err)
	}
	if !nat.Claimed("late") {
		t.Fatal("the failed flush dropped the claim from memory")
	}
	if !reopen().Claimed("late") { // fix round 1 (D-133): the journal holds it although the snapshot failed
		t.Fatal("the claim of a transaction whose snapshot failed is not durable (journal)")
	}
	if err := w.ClaimsTxn()(); err != nil {
		t.Fatal(err)
	}
	if !reopen().Claimed("late") {
		t.Fatal("the pending claim was not written by the next transaction end")
	}
}

// A Prune (VPP identity change) inside a transaction is written with it, once.
func TestKeyedClaimsPruneInTxn(t *testing.T) {
	dir := t.TempDir()
	id := &Identity{}
	id.Set(bootid.Identity{BootID: "b", PID: 1, StartTime: 1})
	k, err := OpenKeyedClaims(dir, "acl", "w1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Claim("etype/lan"); err != nil {
		t.Fatal(err)
	}
	k.Begin()
	id.Set(bootid.Identity{BootID: "b", PID: 2, StartTime: 2})
	if n, err := k.Prune(); err != nil || n != 1 {
		t.Fatalf("prune %d %v", n, err)
	}
	if err := k.Claim("etype/wan"); err != nil {
		t.Fatal(err)
	}
	if err := k.Flush(); err != nil {
		t.Fatal(err)
	}
	k2, _ := OpenKeyedClaims(dir, "acl", "w1", id)
	if k.writes != 2 || k2.Len() != 1 || !k2.Claimed("etype/wan") {
		t.Fatalf("writes %d, records %d", k.writes, k2.Len())
	}
}
