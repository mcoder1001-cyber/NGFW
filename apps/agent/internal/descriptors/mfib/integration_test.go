package mfib

import (
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	"strconv"
	"testing"
)

// Static routes do not transmit IGMP packets, so this host test needs the normal lab integration gate only.
func TestStaticMulticastOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	table := h.TableB + 83
	h.IPTable(table)
	t.Cleanup(func() { h.NoLeftovers(table) })
	in, inIdx := h.Loopback(83, true, true)
	out, outIdx := h.Loopback(84, true, true)
	h.BindTable(inIdx, table)
	h.BindTable(outIdx, table)
	old := dfkit.IdentitySource
	dfkit.IdentitySource = dfkit.BootIdentity
	t.Cleanup(func() { dfkit.IdentitySource = old })
	d := New(h.C, h.Owner, dfkit.NewMemoryBootStore(), df7.WithIDRange(h.TableB, h.TableB+999))
	h.CleanupOwned(d)
	r := Route{Table: table, Group: "239." + strconv.Itoa(h.Slot) + ".0.1", Source: h.Addr(83, 1), Paths: []Path{{in, "accept"}, {out, "forward"}}}
	want := df7test.Desired(d, df7.Encode(r))
	h.Apply(d, want)
	h.ExpectRetrieved(d, want)
	h.Hold("show ip mfib: static multicast source/group accept/forward")
	if e := d.Delete(h.Ctx, want.Value, nil); e != nil {
		t.Fatal(e)
	}
	h.ExpectRetrieved(d)
	h.Apply(d, want)
	h.ExpectRetrieved(d, want)
	t.Log("static mFIB create/retrieve/rollback/recreate verified; no VPP restart or IGMP transmission")
}
