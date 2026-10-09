package subsystems

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/multiwan"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

// These immutable nodes stand in only for already provisioned native topology.
// The scheduler, PPP descriptor/runtime and WAN route descriptor are product code.
type wanCarrierTopology struct {
	name string
	rows []scheduler.KV
}

func (d *wanCarrierTopology) Name() string { return d.name }
func (d *wanCarrierTopology) KeyOf(v proto.Message) scheduler.Key {
	return scheduler.Key(v.(*structpb.Struct).Fields["key"].GetStringValue())
}
func (d *wanCarrierTopology) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (d *wanCarrierTopology) Retrieve(context.Context) ([]scheduler.KV, error)  { return d.rows, nil }
func (d *wanCarrierTopology) Create(context.Context, proto.Message) (any, error) {
	return nil, errors.New("unexpected topology mutation")
}
func (d *wanCarrierTopology) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, errors.New("unexpected topology mutation")
}
func (d *wanCarrierTopology) Delete(context.Context, proto.Message, any) error {
	return errors.New("unexpected topology mutation")
}

func TestCarrierWANTransactionOrdersRoutesAndRollsBackFailure(t *testing.T) {
	rt, _, fake := newTestRuntime(t)
	rt.carrierMode, rt.carrierRoot, rt.carrierHooks = true, t.TempDir(), t.TempDir()
	for _, kind := range []string{"ip-up", "ip-down", "ipv6-up", "ipv6-down"} {
		if err := os.WriteFile(filepath.Join(rt.carrierHooks, kind), []byte("#!/usr/bin/python3\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	spec, _ := pppoe.NewCarrierSpec(rt.owner, "pppwan", "wanraw", 1492)
	fake.AddInterface("wanraw", "")
	fake.AddInterface("pppwan", "w9:pppwan")
	fake.Reply("sw_interface_get_table", &interfaces.SwInterfaceGetTableReply{VrfID: 9000})
	rt.runner = &carrierTestRunner{lease: namespaceLeaseForTest(spec)}
	client := desc.NewClientConfig(rt, rt.renderer, filepath.Join(t.TempDir(), "applied.pb"))
	client.SetCarrierOwner(rt.owner)
	client.SetSecretSource(func(string) ([]byte, error) { return []byte("NGFW_TEST_PSK_F-pppoe-client-wiring"), nil })
	standalone := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{
		"wanraw": {Enabled: proto.Bool(true)},
		"pppwan": {Pppoe: &ngfwv1.Pppoe{Parent: proto.String("wanraw"), Username: proto.String("test"), PasswordRef: proto.String("password/test"), Ipv6: proto.String("off"), MssClamp: proto.Bool(false), DefaultRoute: proto.Bool(true)}},
	}}
	if _, err := client.Create(t.Context(), standalone); err != nil {
		t.Fatal(err)
	}
	joined := proto.Clone(standalone).(*ngfwv1.DesiredState)
	joined.Routing = &ngfwv1.RoutingConfig{WanGroups: []*ngfwv1.WanGroup{{Name: proto.String("internet"), Members: []*ngfwv1.WanMember{{Interface: proto.String("pppwan"), NextHop: proto.String("pppoe")}}}}}
	owned, err := ownertable.Open(t.TempDir(), rt.owner)
	if err != nil {
		t.Fatal(err)
	}
	routes := &multiwan.RouteDescriptor{RouteDescriptor: &core.RouteDescriptor{Env: core.Env{Client: fake, Owner: rt.owner, Owned: owned, RouteInstance: multiwan.RouteName}}}
	reg := scheduler.NewRegistry()
	reg.Register(client)
	reg.Register(routes)
	topology := map[string]*wanCarrierTopology{}
	var base []scheduler.KV
	for _, dep := range client.Dependencies(standalone) {
		node := topology[dep.Key.Descriptor()]
		if node == nil {
			node = &wanCarrierTopology{name: dep.Key.Descriptor()}
			topology[node.name] = node
			reg.Register(node)
		}
		value, _ := structpb.NewStruct(map[string]any{"key": string(dep.Key)})
		row := scheduler.KV{Key: dep.Key, Value: value}
		node.rows = append(node.rows, row)
		base = append(base, row)
	}
	sched := scheduler.New(reg, nil)
	route := &core.Route{Prefix: "0.0.0.0/0", Paths: []*core.RoutePath{{Interface: "pppwan", Address: strings.Split(spec.Host4, "/")[0], Weight: 1}}}
	plan := func(doc *ngfwv1.DesiredState, wan bool) []scheduler.KV {
		out := append([]scheduler.KV{}, base...)
		out = append(out, scheduler.KV{Key: desc.ClientConfigKey, Value: doc})
		if wan {
			out = append(out, scheduler.KV{Key: routes.KeyOf(route), Value: route})
		}
		return out
	}
	scope := scheduler.All
	withdrawn, installed, failNext := false, false, true
	var installedRoute ip.IPRoute
	fake.On("ip_route_v2_dump", func(api.Message) ([]api.Message, error) {
		if !installed {
			return nil, nil
		}
		return []api.Message{&ip.IPRouteV2Details{Route: ip.IPRouteV2{TableID: installedRoute.TableID, Prefix: installedRoute.Prefix, NPaths: installedRoute.NPaths, Paths: installedRoute.Paths, Src: 8}}}, nil
	})
	fake.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
		write := req.(*ip.IPRouteAddDel)
		if write.IsMultipath && !write.IsAdd {
			withdrawn = true
			return []api.Message{&ip.IPRouteAddDelReply{}}, nil
		}
		if write.IsAdd {
			if !withdrawn || rt.applied["pppwan"].DefaultRoute || len(rt.carrierReady) != 0 {
				t.Error("WAN route written before PPP default/readiness withdrawal")
			}
			if failNext {
				failNext = false
				return nil, errors.New("injected WAN route failure")
			}
			installed = true
			installedRoute = write.Route
		} else {
			installed = false
		}
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	rt.mirrored = map[string]desc.Mirror{"pppwan": {Interface: "pppwan", LocalIPv4: "192.0.2.7/32", PeerIPv4: strings.Split(spec.Host4, "/")[0], DefaultRoute: true}}
	rt.carrierReady["pppwan"] = carrierForwarding{epoch: "old", until: time.Now().Add(time.Minute)}
	failed := sched.Apply(t.Context(), plan(joined, true), scope)
	if failed.Outcome == scheduler.OutcomeApplied || !withdrawn || installed {
		t.Fatalf("failure ignored or unsafe transition: %+v", failed)
	}
	if !rt.applied["pppwan"].DefaultRoute {
		t.Fatal("failed transaction did not restore standalone policy")
	}
	result := sched.Apply(t.Context(), plan(joined, true), scope)
	if result.Outcome != scheduler.OutcomeApplied || !installed || rt.applied["pppwan"].DefaultRoute {
		t.Fatalf("join failed: %+v", result)
	}
	// An all-down health projection contains no WAN route. PPP must remain suppressed.
	result = sched.Apply(t.Context(), plan(joined, false), scope)
	if result.Outcome != scheduler.OutcomeApplied || installed || rt.applied["pppwan"].DefaultRoute {
		t.Fatalf("health-down restored bypass: %+v", result)
	}
	result = sched.Apply(t.Context(), plan(standalone, false), scope)
	if result.Outcome != scheduler.OutcomeApplied || !rt.applied["pppwan"].DefaultRoute {
		t.Fatalf("leave failed: %+v", result)
	}
}
