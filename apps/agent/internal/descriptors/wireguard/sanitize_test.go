package wireguard_test

import (
	"errors"
	"slices"
	"testing"

	"go.fd.io/govpp/api"

	wgd "ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// TestInterfaceSanitizesReusedIndex is TD-3 re-review H1: a WireGuard interface on a reused
// sw_if_index carrying a previous holder's bindings (ip classify — the 04:50 crash path once the
// interface has an address —, input/output ACL, vxlan bypass, SPD) is created clean through
// ifsanitize.Acquire; Delete clears the bindings before wireguard_interface_delete; a sanitize
// failure removes the interface and fails the Create.
func TestInterfaceSanitizesReusedIndex(t *testing.T) {
	e := newEnv(t)
	m := sanitizetest.NewModel()
	m.Install(e.v.Client)
	m.Poison(1, 2, 3)
	obj := e.itfV()
	meta, err := e.itf.Create(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}
	idx := meta.(wgd.InterfaceMeta).SwIfIndex
	if dirty := m.Dirty(idx); dirty != "" {
		t.Fatalf("new wg interface %d still has inherited %s", idx, dirty)
	}
	if e.v.ifaces[idx].Tag != owner+":wg4001" {
		t.Fatalf("interface %+v", e.v.ifaces[idx])
	}

	// Delete: bindings made meanwhile go before the interface (the table still exists)
	m.If(idx).OutACL = [3]uint32{sanitizetest.PoisonTable, sanitizetest.PoisonTable, sanitizetest.PoisonTable}
	if err := e.itf.Delete(ctx, obj, meta); err != nil {
		t.Fatal(err)
	}
	if dirty := m.Dirty(idx); dirty != "" {
		t.Fatalf("deleted wg interface %d left %s on its index", idx, dirty)
	}
	names := callNames(e.v.Calls())
	del := slices.Index(names, "wireguard_interface_delete")
	last := -1
	for i, n := range names {
		if n == "output_acl_set_interface" && i < del {
			last = i
		}
	}
	if last < 0 || del < 0 {
		t.Fatalf("no output ACL unbind before wireguard_interface_delete (%d, %d)", last, del)
	}

	// VPP refuses the reset: the interface is removed and not reported created
	e.v.Fail("classify_set_interface_ip_table", errRefused)
	if _, err := e.itf.Create(ctx, obj); !errors.Is(err, errRefused) {
		t.Fatalf("err = %v", err)
	}
	if len(e.v.wgs) != 0 {
		t.Fatalf("wg interface left behind: %v", e.v.wgs)
	}
	if n := len(e.v.CallsNamed("wireguard_interface_delete")); n != 2 {
		t.Fatalf("wireguard_interface_delete sent %d times", n)
	}
}

var errRefused = errors.New("vpp refused")

func callNames(msgs []api.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.GetMessageName()
	}
	return out
}
