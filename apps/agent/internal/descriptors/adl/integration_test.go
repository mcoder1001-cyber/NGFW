package adl

import (
	"errors"
	"fmt"
	"testing"

	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestADLOnHost enables the interface feature and the allow-list, then disables both. The
// plugin has no dump, so the only Retrieve assertion is the typed ErrRetrieveUnsupported;
// `vppctl show features <loop>` during VRX_DF2_HOLD is the evidence.
func TestADLOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	table := vpptest.TableBase(t) + 3
	loop, idx := df2test.Loopback(t, c, 6)
	df2test.VRF(t, c, table, false, "adl")
	df2test.AddAddress(t, c, idx, fmt.Sprintf("10.%d.6.1/24", slot))

	ifd := NewInterface(c, owner)
	iface := &Interface{Interface: loop}
	imeta, err := ifd.Create(ctx, iface)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ifd.Delete(df2test.Ctx(t), iface, imeta) })
	ald := NewAllowlist(c, owner)
	allow := &Allowlist{Interface: loop, FibId: table, Ip4: true}
	ameta, err := ald.Create(ctx, allow)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ald.Delete(df2test.Ctx(t), allow, ameta) })
	if _, err := ifd.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("interface Retrieve: %v", err)
	}
	if _, err := ald.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("allowlist Retrieve: %v", err)
	}
	t.Logf("adl enabled on %s with allow-list table %d (write-only: no dump in the adl API)", loop, table)
	df2test.Hold(t)
	if err := ald.Delete(ctx, allow, ameta); err != nil {
		t.Fatal(err)
	}
	if err := ifd.Delete(ctx, iface, imeta); err != nil {
		t.Fatal(err)
	}
}
