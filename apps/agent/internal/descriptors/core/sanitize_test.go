package core_test

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

// TestLoopbackSanitizesReusedIndex (D-095 a): a loopback that gets a sw_if_index carrying a
// previous interface's classify bindings / SPD is cleaned before it is tagged and reported
// created; one that cannot be made safe is removed again.
func TestLoopbackSanitizesReusedIndex(t *testing.T) {
	ctx := context.Background()
	v := coretest.New()
	m := sanitizetest.NewModel()
	m.Install(v.Client)
	m.Poison(1, 2, 3)
	d := &core.LoopbackDescriptor{Env: core.Env{Client: v, Owner: "w2"}}
	meta, err := d.Create(ctx, &core.Loopback{Name: "loop201", Instance: 201})
	if err != nil {
		t.Fatal(err)
	}
	idx := meta.(core.IfMeta).SwIfIndex
	if dirty := m.Dirty(idx); dirty != "" {
		t.Fatalf("sw_if_index %d still has inherited %s", idx, dirty)
	}
	if a, b := sanitizetest.Order(v.Client, "classify_table_ids", "sw_interface_tag_add_del"); a < 0 || b < 0 || a > b {
		t.Fatalf("sanitize (%d) must precede the tag (%d)", a, b)
	}

	v.Client.Fail("classify_set_interface_ip_table", errRefused) // VPP refuses the reset: the interface must not be reported created
	_, err = d.Create(ctx, &core.Loopback{Name: "loop202", Instance: 202})
	if !errors.Is(err, errRefused) {
		t.Fatalf("err = %v, want the sanitize error", err)
	}
	for _, in := range v.Ifaces {
		if in.Name == "loop202" {
			t.Fatal("a loopback that could not be sanitized was left in VPP")
		}
	}
}

var errRefused = errors.New("vpp refused")
