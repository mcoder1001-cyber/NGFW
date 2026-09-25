package df2_test

// TD-11b: DF-2 claim hygiene in claims.go (N5) and the product agent's persistence guard (review 3.2).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/dfkit/persist"
)

// TestFileClaimStoreWriteFailure (N5): a claim whose file write failed is not kept in memory (the
// retry would report success and the claim would be lost on restart), and a release whose write
// failed is not applied in memory.
func TestFileClaimStoreWriteFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.json")
	s, err := df2.OpenFileClaimStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Claim("urpf.interface/ens224/ipv4/rx"); err != nil {
		t.Fatal(err)
	}
	// the next rename onto the store file fails (even for root): the path is now a non-empty directory
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim("urpf.interface/ens224/ipv6/rx"); err == nil {
		t.Fatal("Claim succeeded although the write failed")
	}
	if s.Claimed("urpf.interface/ens224/ipv6/rx") {
		t.Fatal("claim kept in memory although it was never written")
	}
	if err := s.Release("urpf.interface/ens224/ipv4/rx"); err == nil {
		t.Fatal("Release succeeded although the write failed")
	}
	if !s.Claimed("urpf.interface/ens224/ipv4/rx") {
		t.Fatal("release applied in memory although it was never written")
	}
}

// TestClaimFirst: the claim precedes the VPP call; a refused claim fails first; undo releases only
// a claim this call made; tagged interfaces need no claim.
func TestClaimFirst(t *testing.T) {
	c := acl.NewMemoryClaimStore()
	undo, err := df2.ClaimFirst(c, true, "urpf.interface/ens224/ipv4/rx")
	if err != nil || !c.Claimed("urpf.interface/ens224/ipv4/rx") {
		t.Fatalf("claim first: %v", err)
	}
	undo()
	if c.Claimed("urpf.interface/ens224/ipv4/rx") {
		t.Fatal("undo kept the claim this call made")
	}
	_ = c.Claim("urpf.interface/ens224/ipv4/rx")
	undo, _ = df2.ClaimFirst(c, true, "urpf.interface/ens224/ipv4/rx")
	undo()
	if !c.Claimed("urpf.interface/ens224/ipv4/rx") {
		t.Fatal("undo released a claim that existed before")
	}
	if undo, err := df2.ClaimFirst(c, false, "urpf.interface/loop1/ipv4/rx"); err != nil || c.Claimed("urpf.interface/loop1/ipv4/rx") {
		t.Fatalf("tagged interface claimed: %v", err)
	} else {
		undo()
	}
	refuse := refusing{errors.New("claim store: no identity")}
	if _, err := df2.ClaimFirst(refuse, true, "k"); !errors.Is(err, refuse.err) {
		t.Fatalf("refused claim: %v", err)
	}
}

type refusing struct{ err error }

func (r refusing) Claim(string) error   { return r.err }
func (r refusing) Release(string) error { return nil }
func (r refusing) Claimed(string) bool  { return false }

// TestCheckPersistent (review 3.2): Options with the in-memory default fail the product agent's
// guard; with a FileClaimStore (or any store reporting Persistent) they pass.
func TestCheckPersistent(t *testing.T) {
	if err := df2.BuildOptions().CheckPersistent("urpf.interface"); !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("in-memory default: %v", err)
	}
	fs, err := df2.OpenFileClaimStore(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := df2.BuildOptions(df2.WithClaims(fs)).CheckPersistent("urpf.interface"); err != nil {
		t.Fatal(err)
	}
}
