package span

import (
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	iface "ngfw/agent/internal/descriptors/interface"
)

// failClaims is a claim store that cannot record (a full disk, a read-only state dir).
type failClaims struct{}

func (failClaims) Claim(string, string) error   { return errors.New("claim store down") }
func (failClaims) Release(string, string) error { return nil }
func (failClaims) Claimed(string, string) bool  { return false }

// Review M3 (TD-11b claim first): on an untagged source (eth0) the claim is recorded before the VPP write, so a claim
// that cannot be recorded leaves nothing in VPP (the old code enabled first and claimed after).
func TestCreateClaimsFirst(t *testing.T) {
	f, st := fakeSpan()
	iface.SetClaimStore(df7test.Owner, failClaims{})
	t.Cleanup(func() { iface.SetClaimStore(df7test.Owner, nil) })
	d := New(f, df7test.Owner)
	if _, err := d.Create(t.Context(), df7.Encode(Mirror{Source: "eth0", Destination: "loop0", State: StateBoth})); err == nil {
		t.Fatal("Create succeeded without its claim")
	}
	if n := len(f.CallsNamed("sw_interface_span_enable_disable")); n != 0 || len(st) != 0 {
		t.Fatalf("a mirror was written before the claim: %d calls, state %v", n, st)
	}
}

// A failed VPP write releases the claim this Create made.
func TestCreateReleasesClaimOnFailure(t *testing.T) {
	f, _ := fakeSpan()
	iface.SetClaimStore(df7test.Owner, nil)
	f.Fail("sw_interface_span_enable_disable", errors.New("vpp says no"))
	d := New(f, df7test.Owner)
	if _, err := d.Create(t.Context(), df7.Encode(Mirror{Source: "eth0", Destination: "loop0", State: StateBoth})); err == nil {
		t.Fatal("Create succeeded")
	}
	tg, err := d.Target(t.Context(), "eth0", string(Key("eth0", "loop0", false)))
	if err != nil {
		t.Fatal(err)
	}
	if tg.Claimed() {
		t.Fatal("the claim of a failed Create was kept")
	}
}
