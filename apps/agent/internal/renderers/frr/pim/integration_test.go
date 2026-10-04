package pim_test

import (
	"context"
	"google.golang.org/protobuf/types/known/structpb"
	syncpim "ngfw/agent/internal/frrsync/pim"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/frrtest"
	"ngfw/agent/internal/renderers/frr/pim"
	"ngfw/agent/internal/vpp/vpptest"
	"strings"
	"testing"
)

// TestPimdScopedHarness verifies config and bounded state on a real child pimd.
// It deliberately does not claim VPP forwarding or neighbor topology acceptance.
func TestPimdScopedHarness(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix := vpptest.Prefix(t)
	host := prefix + "pim0"
	h := frrtest.Start(t, frrtest.Options{Prefix: prefix, Daemons: []string{"mgmtd", "zebra", "staticd", "pimd"}, Links: []frrtest.Link{{Name: host, Kind: "dummy", CIDR: "10.254.15.1/24"}}})
	r := h.Renderer(frr.WithInterfaceMapper(func(name string) (string, bool) { return host, name == "uplink" }))
	d, e := structpb.NewStruct(map[string]any{"routing": map[string]any{"multicast": map[string]any{"pim": map[string]any{"interfaces": []any{"uplink"}, "rp": []any{map[string]any{"address": "10.254.15.1", "groups": []any{"239.0.0.0/8"}}}}}}})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	files, e := r.Render(ctx, d)
	if e != nil {
		t.Fatal(e)
	}
	h.AssertScoped(t, files)
	if e = r.Validate(ctx, files); e != nil {
		t.Fatal(e)
	}
	if e = r.Apply(ctx, files); e != nil {
		t.Fatal(e)
	}
	raw, e := r.Show(ctx, frr.ShowRunningConfig)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(raw), "ip pim rp 10.254.15.1 239.0.0.0/8") || !strings.Contains(string(raw), "ip pim") {
		t.Fatalf("missing PIM config: %s", raw)
	}
	raw, e = r.Show(ctx, pim.ShowMroute)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = syncpim.Parse(raw); e != nil {
		t.Fatal(e)
	}
	empty, _ := structpb.NewStruct(map[string]any{})
	files, e = r.Render(ctx, empty)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Apply(ctx, files); e != nil {
		t.Fatal(e)
	}
	raw, e = r.Show(ctx, frr.ShowRunningConfig)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "ip pim") {
		t.Fatalf("PIM rollback left state: %s", raw)
	}
}
