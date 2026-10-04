package pim

import (
	"context"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"strings"
	"testing"
)

func str(s string) *string { return &s }
func TestRenderFramework(t *testing.T) {
	doc := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{"wan": {Lcp: &ngfwv1.InterfaceLcp{HostIfName: str("w15in")}}}, Routing: &ngfwv1.RoutingConfig{Multicast: &ngfwv1.MulticastConfig{Pim: &ngfwv1.PimConfig{Interfaces: []string{"wan"}, Rp: []*ngfwv1.PimRp{{Address: str("10.0.0.1"), Groups: []string{"239.0.0.0/8"}}}}}}}
	mapper := &lcpmap.Mapper{}
	mapper.Set(lcpmap.FromDesired(doc))
	runner := renderers.NewRecordingRunner()
	r := frr.New(runner, frr.WithInterfaceMapper(mapper.Map), frr.WithSections(Section{}), frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: Name, Fn: InterfaceLines}))
	files, e := r.Render(context.Background(), doc)
	if e != nil {
		t.Fatal(e)
	}
	got := string(files[r.Paths().ConfFile()].Content)
	if strings.Count(got, "interface w15in\n") != 1 || !strings.Contains(got, " ip pim\n") || !strings.Contains(got, "ip pim rp 10.0.0.1 239.0.0.0/8\n") {
		t.Fatalf("unexpected config: %s", got)
	}
}
func TestRejectRP(t *testing.T) {
	for _, rp := range []*ngfwv1.PimRp{{Address: str("10.0.0.1\nend")}, {Address: str("239.0.0.1")}, {Address: str("10.0.0.1"), Groups: []string{"10.0.0.0/8"}}, {Address: str("10.0.0.1"), Groups: []string{"239.0.0.1/8"}}} {
		if _, e := Render(&ngfwv1.PimConfig{Rp: []*ngfwv1.PimRp{rp}}); e == nil {
			t.Fatalf("accepted %v", rp)
		}
	}
}
func TestRejectInterface(t *testing.T) {
	for _, mapping := range []frr.InterfaceMapper{func(string) (string, bool) { return "", false }, func(string) (string, bool) { return "bad\nend", true }, func(string) (string, bool) { return "w15in", true }} {
		doc := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Multicast: &ngfwv1.MulticastConfig{Pim: &ngfwv1.PimConfig{Interfaces: []string{"a", "b"}}}}}
		r := frr.New(renderers.NewRecordingRunner(), frr.WithInterfaceMapper(mapping), frr.WithSections(Section{}), frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: Name, Fn: InterfaceLines}))
		if _, e := r.Render(context.Background(), doc); e == nil {
			t.Fatal("accepted missing, unsafe, or duplicate mapping")
		}
	}
}
