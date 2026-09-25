package dfkit_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
)

// failClaims is an iface.ClaimStore that can refuse to record.
type failClaims struct {
	mu   sync.Mutex
	m    map[[2]string]bool
	fail error
}

func (c *failClaims) Claim(n, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.m[[2]string{n, h}] = true
	return nil
}

func (c *failClaims) Release(n, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, [2]string{n, h})
	return nil
}

func (c *failClaims) Claimed(n, h string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[[2]string{n, h}]
}

type persistedClaims struct{ *failClaims }

func (persistedClaims) Persistent() bool { return true }

// TestClaimFirst (TD-11b, review 3.3): the claim precedes the add; a refused claim fails before
// anything is written; Undo releases only a claim this Create made; Adopt accepts an existing
// object only on a claim that existed before this Create.
func TestClaimFirst(t *testing.T) {
	const owner = "w5f"
	s := &failClaims{m: map[[2]string]bool{}}
	iface.SetClaimStore(owner, s)
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	f := dfkittest.NewFake(dfkittest.Iface{Index: 1, Name: "loop501", Tag: owner + ":loop501"}, dfkittest.Iface{Index: 9, Name: "ens192"})
	ctx := context.Background()

	tagged, err := dfkit.ResolveTarget(ctx, f, "loop501", owner, "x.y")
	if err != nil {
		t.Fatal(err)
	}
	s.fail = errors.New("claim store: no identity")
	if c, err := tagged.ClaimFirst(ctx); err != nil || c.Adopt() != nil {
		t.Fatalf("tagged interfaces need no claim: %v", err)
	}
	u, err := dfkit.ResolveTarget(ctx, f, "ens192", owner, "x.y")
	if err != nil || !u.Untagged {
		t.Fatal(err)
	}
	if _, err := u.ClaimFirst(ctx); !errors.Is(err, s.fail) {
		t.Fatalf("refused claim: %v", err)
	}
	s.fail = nil

	c, err := u.ClaimFirst(ctx)
	if err != nil || !u.Claimed() {
		t.Fatalf("claim first: %v", err)
	}
	addErr := errors.New("vpp: add refused")
	if err := c.Undo(addErr); !errors.Is(err, addErr) || u.Claimed() {
		t.Fatalf("Undo: %v, claimed %v", err, u.Claimed())
	}
	// "already exists" on an object nobody of ours claimed before: not adopted, claim dropped
	c, _ = u.ClaimFirst(ctx)
	if err := c.Adopt(); !errors.Is(err, dfkit.ErrNotOurs) || u.Claimed() {
		t.Fatalf("Adopt of a foreign object: %v, claimed %v", err, u.Claimed())
	}
	// a claim of an earlier successful Create: adopted, and kept by Undo
	if err := u.Claim(); err != nil {
		t.Fatal(err)
	}
	c, _ = u.ClaimFirst(ctx)
	if err := c.Adopt(); err != nil {
		t.Fatalf("Adopt of our own object: %v", err)
	}
	if err := c.Undo(addErr); !errors.Is(err, addErr) || !u.Claimed() {
		t.Fatal("Undo released a claim that existed before the Create")
	}
}

// TestCheckPersistent (TD-11b, review 3.2): the DF-8 checks fail on the in-memory defaults and pass
// on the persisted stores.
func TestCheckPersistent(t *testing.T) {
	const owner = "w5g"
	iface.SetClaimStore(owner, nil)
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	if err := dfkit.CheckClaims("dhcp.client", owner); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("in-memory claims: %v", err)
	}
	iface.SetClaimStore(owner, persistedClaims{&failClaims{m: map[[2]string]bool{}}})
	if err := dfkit.CheckClaims("dhcp.client", owner); err != nil {
		t.Fatal(err)
	}
	if err := dfkit.CheckBoot("pcap.capture", dfkit.NewMemoryBootStore()); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("in-memory boot store: %v", err)
	}
	fb, err := dfkit.NewFileBootStore(filepath.Join(t.TempDir(), "boot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := dfkit.CheckBoot("pcap.capture", fb); err != nil {
		t.Fatal(err)
	}
	if err := dfkit.CheckBoot("pcap.capture", nil); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("nil boot store: %v", err)
	}
}
