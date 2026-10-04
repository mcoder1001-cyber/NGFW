package subsystems

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mpls"
	"ngfw/agent/internal/desired"
	ldp "ngfw/agent/internal/frrsync/ldp"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/scheduler"
)

func TestLdpRegistrationAndConfigDependencies(t *testing.T) {
	owner := "ldptest"
	rt := newFRRAt(Env{Owner: owner}, renderers.NewRecordingRunner(), frr.ProductPaths(), true)
	frrRuntimes.Store(owner, rt)
	t.Cleanup(func() { frrRuntimes.Delete(owner); ldpCaches.Delete(owner); ldpStateReaders.Delete(owner) })
	w := &Wiring{env: Env{Owner: owner, IDs: IDScope{All: true}}}
	reg := scheduler.NewRegistry()
	if err := registerMplsLdpFor(reg, w, 7000); err != nil {
		t.Fatal(err)
	}
	sources := w.DynamicSources()
	if len(sources) != 1 || DomainOf(ldp.RouteName) != "" {
		t.Fatal("source scope is not isolated")
	}
	cache, _ := ldpCaches.Load(owner)
	cache.(*ldp.Cache).Update(time.Now(), ldp.Observation{Bindings: []ldp.Binding{{LocalLabel: 16000, NextHop: "192.0.2.1", LinuxInterface: "tap"}}}, []mpls.Route{{Table: 7000, Label: 16000, EOS: true, EOSProto: mpls.PayloadIP4, Paths: []df7.Path{{Interface: "wan", NextHop: "192.0.2.1"}}}}, nil)
	doc := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{"wan": {Lcp: &ngfwv1.InterfaceLcp{HostIfName: proto.String("tap")}}}, Routing: &ngfwv1.RoutingConfig{Mpls: &ngfwv1.MplsConfig{Ldp: &ngfwv1.MplsLdp{Interfaces: []string{"wan"}}}}}
	if got := sources[0].Desired(doc); len(got) != 1 || got[0].Key != "mpls-route.ldp/7000/16000/eos" {
		t.Fatalf("%+v", got)
	}
	doc.Routing.Mpls.Ldp.LabelRange = &ngfwv1.LdpLabelRange{Min: proto.Uint32(17000), Max: proto.Uint32(18000)}
	if got := sources[0].Desired(doc); len(got) != 0 {
		t.Fatal("old range label retained")
	}
	doc.Routing.Mpls.Ldp.LabelRange = nil
	doc.Interfaces["wan"].Lcp.HostIfName = proto.String("other")
	if got := sources[0].Desired(doc); len(got) != 0 {
		t.Fatal("stale remapped path retained")
	}
	doc.Interfaces["wan"].Lcp.HostIfName = proto.String("tap")
	delete(doc.Interfaces, "wan")
	if got := sources[0].Desired(doc); len(got) != 0 {
		t.Fatal("deleted interface retained")
	}
	doc.Routing.Mpls.Ldp = nil
	if got := sources[0].Desired(doc); len(got) != 0 {
		t.Fatal("disabled LDP retained")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sources[0].Run(ctx, func(context.Context) error { return nil })
}
func TestLdpFRRProjection(t *testing.T) {
	doc := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Mpls: &ngfwv1.MplsConfig{Ldp: &ngfwv1.MplsLdp{RouterId: proto.String("192.0.2.1")}}}}
	out := desired.FRRDoc(doc, nil)
	if out.GetRouting().GetMpls().GetLdp().GetRouterId() != "192.0.2.1" {
		t.Fatal("LDP lost from FRR projection")
	}
}

func TestLdpInstalledCountUsesRetrievedOwnedRoutes(t *testing.T) {
	owner := "ldp-count"
	cache := &ldp.Cache{}
	ldpCaches.Store(owner, cache)
	t.Cleanup(func() { ldpCaches.Delete(owner); ldpStateReaders.Delete(owner) })
	cache.Update(time.Now(), ldp.Observation{}, []mpls.Route{{Label: 16000}}, nil)
	cache.Applied(time.Now(), nil)
	installed := []scheduler.KV{{Key: "mpls-route.ldp/0/16000/eos"}}
	ldpStateReaders.Store(owner, func(context.Context) ([]scheduler.KV, error) { return installed, nil })
	out, err := MplsLdpState(context.Background(), owner)
	if err != nil || out.Sync.Installed != 1 {
		t.Fatalf("%+v %v", out, err)
	}
	// A configuration transaction suppresses the route while the observation remains cached.
	installed = nil
	out, err = MplsLdpState(context.Background(), owner)
	if err != nil || out.Sync.Installed != 0 {
		t.Fatalf("suppressed route counted: %+v %v", out, err)
	}
	ldpStateReaders.Store(owner, func(context.Context) ([]scheduler.KV, error) { return nil, fmt.Errorf("retrieve failed") })
	if out, err := MplsLdpState(context.Background(), owner); err == nil || out != nil {
		t.Fatalf("failed Retrieve returned stale state: %+v %v", out, err)
	}
	ldpStateReaders.Delete(owner)
	if out, err := MplsLdpState(context.Background(), owner); err == nil || out != nil {
		t.Fatalf("missing reader returned stale state: %+v %v", out, err)
	}
	ldpStateReaders.Store(owner, func(context.Context) ([]scheduler.KV, error) { return installed, nil })
	// A failed subsequent apply must not overwrite the count of actual surviving objects.
	cache.Applied(time.Now(), fmt.Errorf("apply failed"))
	installed = []scheduler.KV{{Key: "mpls-route.ldp/0/16000/eos"}}
	out, err = MplsLdpState(context.Background(), owner)
	if err != nil || out.Sync.Installed != 1 {
		t.Fatalf("failed apply count: %+v %v", out, err)
	}
}
