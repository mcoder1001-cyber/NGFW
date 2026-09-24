package ipsec_test

import (
	"errors"
	"slices"
	"testing"

	"go.fd.io/govpp/api"

	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// TestItfSanitizesReusedIndex is TD-3 re-review H1: an ipsec interface on a reused sw_if_index
// carrying a previous holder's bindings (ip classify — the 04:50 crash path once the interface
// has an address —, input/output ACL, vxlan bypass, SPD) is created clean through
// ifsanitize.Acquire; Delete clears the bindings before ipsec_itf_delete; a sanitize failure
// removes the interface and fails the Create.
func TestItfSanitizesReusedIndex(t *testing.T) {
	v := newFakeVPP()
	m := sanitizetest.NewModel()
	m.Install(v.Client)
	m.Poison(1, 2, 3)
	d := ipsecd.NewItf(newCfg(v))
	obj := &vpnpb.IpsecItf{Instance: 4002, Mode: "p2p"}
	meta, err := d.Create(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}
	idx := meta.(ipsecd.ItfMeta).SwIfIndex
	if dirty := m.Dirty(idx); dirty != "" {
		t.Fatalf("new ipsec interface %d still has inherited %s", idx, dirty)
	}
	if v.ifaces[idx].Tag != "w4:ipsec4002" {
		t.Fatalf("interface %+v", v.ifaces[idx])
	}

	// Delete: bindings made meanwhile go before the interface (the table still exists)
	m.If(idx).InACL = [3]uint32{sanitizetest.PoisonTable, sanitizetest.PoisonTable, sanitizetest.PoisonTable}
	if err := d.Delete(ctx, obj, meta); err != nil {
		t.Fatal(err)
	}
	if dirty := m.Dirty(idx); dirty != "" {
		t.Fatalf("deleted ipsec interface %d left %s on its index", idx, dirty)
	}
	names := callNames(v.Calls())
	unbind, del := slices.Index(names, "input_acl_set_interface"), slices.Index(names, "ipsec_itf_delete")
	if unbind < 0 || del < 0 || unbind > del {
		t.Fatalf("input ACL unbind at %d, ipsec_itf_delete at %d", unbind, del)
	}

	// VPP refuses the reset: the interface is removed and not reported created
	v.Fail("classify_set_interface_ip_table", errRefused)
	if _, err := d.Create(ctx, &vpnpb.IpsecItf{Instance: 4003, Mode: "p2p"}); !errors.Is(err, errRefused) {
		t.Fatalf("err = %v", err)
	}
	for _, itf := range v.itfs {
		if itf.UserInstance == 4003 {
			t.Fatal("ipsec4003 left behind")
		}
	}
	if n := len(v.CallsNamed("ipsec_itf_delete")); n != 2 {
		t.Fatalf("ipsec_itf_delete sent %d times", n)
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
