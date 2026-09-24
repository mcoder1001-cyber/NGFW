package afpacket_test

import (
	"errors"
	"testing"

	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// TestHostInterfaceSanitizesReusedIndex (D-095 a): the 04:50:27 crash was an af_packet interface
// on a reused sw_if_index with an inherited ip classify binding. Create must clear it before
// the interface is tagged; an interface that cannot be made safe is removed.
func TestHostInterfaceSanitizesReusedIndex(t *testing.T) {
	f := newFake()
	m := sanitizetest.NewModel()
	m.Install(f.Client)
	m.Poison(1, 2, 3, 4, 5)
	d := afpacket.New(f, owner)
	meta, err := d.Create(ctx, &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"})
	if err != nil {
		t.Fatal(err)
	}
	if dirty := m.Dirty(meta.(iface.Meta).SwIfIndex); dirty != "" {
		t.Fatalf("new host-interface still has inherited %s", dirty)
	}
	if a, b := sanitizetest.Order(f.Client, "classify_set_interface_ip_table", "sw_interface_tag_add_del"); a < 0 || a > b {
		t.Fatalf("ip classify reset (%d) must precede the tag (%d)", a, b)
	}
	f.Client.Fail("classify_set_interface_ip_table", errRefused) // VPP refuses the reset: the interface must not be reported created
	if _, err := d.Create(ctx, &afpacket.HostInterface{Name: "w2-w0", HostIfName: "w2-w0"}); !errors.Is(err, errRefused) {
		t.Fatalf("err = %v", err)
	}
	if len(f.CallsNamed("af_packet_delete")) != 1 {
		t.Fatal("the unsafe host-interface was not deleted")
	}
}

var errRefused = errors.New("vpp refused")
