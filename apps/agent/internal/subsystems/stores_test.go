package subsystems

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/vpp/bootid"
)

type fixedIndex map[string]uint32

func (f fixedIndex) Resolve(_ context.Context, name string) (uint32, bool) {
	i, ok := f[name]
	return i, ok
}
func (fixedIndex) Invalidate() {}

func TestIfaceClaimsPersistAndExpire(t *testing.T) {
	dir := t.TempDir()
	id := &Identity{}
	idx := fixedIndex{"lan": 5}
	c, err := OpenIfaceClaims(dir, "w1", id, idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Claim("lan", "interface.mtu"); err != ErrNoIdentity {
		t.Fatalf("claim before the first connect: %v", err)
	}
	boot1 := bootid.Identity{BootID: "b", PID: 10, StartTime: 100}
	id.Set(boot1)
	if err := c.Claim("lan", "interface.mtu"); err != nil {
		t.Fatal(err)
	}
	if !c.Claimed("lan", "interface.mtu") || c.Claimed("lan", "interface.admin-state") {
		t.Fatal("claim not recorded per holder")
	}
	// survives an agent restart (new store over the same file)
	c2, err := OpenIfaceClaims(dir, "w1", id, idx)
	if err != nil || !c2.Claimed("lan", "interface.mtu") {
		t.Fatalf("claim lost across reopen: %v", err)
	}
	// the interface was re-created under the same name (new sw_if_index): not ours any more
	idx["lan"] = 9
	if c2.Claimed("lan", "interface.mtu") {
		t.Fatal("claim survived a re-created interface")
	}
	idx["lan"] = 5
	// VPP restarted: the claim expires and Prune drops it from disk
	id.Set(bootid.Identity{BootID: "b", PID: 11, StartTime: 200})
	if c2.Claimed("lan", "interface.mtu") {
		t.Fatal("claim survived a VPP restart")
	}
	if n, err := c2.Prune(); err != nil || n != 1 || c2.Len() != 0 {
		t.Fatalf("prune %d %v", n, err)
	}
	// release is persisted as well
	if err := c2.Claim("lan", "interface.mtu"); err != nil {
		t.Fatal(err)
	}
	if err := c2.Release("lan", "interface.mtu"); err != nil {
		t.Fatal(err)
	}
	c3, _ := OpenIfaceClaims(dir, "w1", id, idx)
	if c3.Len() != 0 {
		t.Fatal("release not persisted")
	}
	// review N7: an interface that cannot be bound to a sw_if_index is not claimed (fail closed)
	if err := c3.Claim("wan", "interface.mtu"); !errors.Is(err, ErrClaimUnbound) || c3.Len() != 0 || c3.Claimed("wan", "interface.mtu") {
		t.Fatalf("unbound claim: err %v, len %d", err, c3.Len())
	}
	if fi, err := os.Stat(filepath.Join(dir, "claims-iface-w1.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode %v %v", fi, err)
	}
}

func TestKeyedClaimsAndCorruptFile(t *testing.T) {
	dir := t.TempDir()
	id := &Identity{}
	id.Set(bootid.Identity{BootID: "b", PID: 1, StartTime: 1})
	k, err := OpenKeyedClaims(dir, "acl", "w1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Claim("etype/lan"); err != nil || !k.Claimed("etype/lan") {
		t.Fatalf("keyed claim %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "claims-nat-w1.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenKeyedClaims(dir, "nat", "w1", id); err == nil {
		t.Fatal("a corrupt claim file must fail closed")
	}
}
