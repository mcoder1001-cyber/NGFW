package lldp

import (
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	iface "ngfw/agent/internal/descriptors/interface"
)

type failClaims struct{}

func (failClaims) Claim(string, string) error   { return errors.New("claim store down") }
func (failClaims) Release(string, string) error { return nil }
func (failClaims) Claimed(string, string) bool  { return false }

// Review M3 (TD-11b claim first): LLDP on an untagged interface (eth0) is claimed before sw_interface_set_lldp; a claim
// that cannot be recorded sends nothing (the old code enabled first and claimed after).
func TestInterfaceClaimsFirst(t *testing.T) {
	f, on := fakeLLDP(nil)
	iface.SetClaimStore(df7test.Owner, failClaims{})
	t.Cleanup(func() { iface.SetClaimStore(df7test.Owner, nil) })
	d := NewInterface(f, df7test.Owner)
	if _, err := d.Create(t.Context(), df7.Encode(Interface{Interface: "eth0"})); err == nil {
		t.Fatal("Create succeeded without its claim")
	}
	if n := len(f.CallsNamed("sw_interface_set_lldp")); n != 0 || len(on) != 0 {
		t.Fatalf("LLDP enabled before the claim: %d calls, %v", n, on)
	}
}

// V20 mismatch: the stray enable is not ours on our index, so the claim this Create made is released.
func TestInterfaceMismatchReleasesClaim(t *testing.T) {
	f, _ := fakeLLDP(map[uint32]uint32{4: 2}) // VPP enables LLDP on sw 2 when asked for eth0 (4)
	iface.SetClaimStore(df7test.Owner, nil)
	d := NewInterface(f, df7test.Owner)
	if _, err := d.Create(t.Context(), df7.Encode(Interface{Interface: "eth0"})); !errors.Is(err, ErrIndexMismatch) {
		t.Fatalf("want ErrIndexMismatch: %v", err)
	}
	tg, err := d.Target(t.Context(), "eth0", string(KeyInterface("eth0")))
	if err != nil {
		t.Fatal(err)
	}
	if tg.Claimed() {
		t.Fatal("the claim was kept after the mismatch")
	}
}
