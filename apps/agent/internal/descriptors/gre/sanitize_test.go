package gre_test

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/gre"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// TestTunnelSanitizesReusedIndex (D-095 a) covers every DF-6 interface type (df6.IfDescriptor):
// inherited state is cleared before the tunnel is tagged; an unsafe tunnel is rolled back.
func TestTunnelSanitizesReusedIndex(t *testing.T) {
	ctx := context.Background()
	f := newFakeGRE()
	m := sanitizetest.NewModel()
	m.Install(f.Client)
	m.Poison(1, 2, 3, 4)
	d := gre.NewTunnel(f, "w11")
	meta, err := d.Create(ctx, &gre.Tunnel{Instance: 1100, Src: "10.11.1.1", Dst: "10.11.1.2", Type: gre.TunnelType_L3})
	if err != nil {
		t.Fatal(err)
	}
	if dirty := m.Dirty(meta.(df6.IfMeta).SwIfIndex); dirty != "" {
		t.Fatalf("new tunnel still has inherited %s", dirty)
	}
	if a, b := sanitizetest.Order(f.Client, "classify_table_ids", "sw_interface_tag_add_del"); a < 0 || a > b {
		t.Fatalf("sanitize (%d) must precede the tag (%d)", a, b)
	}
	f.Fail("classify_set_interface_ip_table", errRefused) // VPP refuses the reset: the interface must not be reported created
	if _, err := d.Create(ctx, &gre.Tunnel{Instance: 1101, Src: "10.11.1.1", Dst: "10.11.1.3", Type: gre.TunnelType_L3}); !errors.Is(err, errRefused) {
		t.Fatalf("err = %v", err)
	}
	if len(f.tunnels) != 1 {
		t.Fatalf("the unsafe tunnel was not rolled back: %d tunnels", len(f.tunnels))
	}
}

var errRefused = errors.New("vpp refused")
