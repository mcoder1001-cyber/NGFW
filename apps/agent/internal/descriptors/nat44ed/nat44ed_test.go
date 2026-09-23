package nat44ed_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

// fakeNAT models the nat44-ed plugin state on a fake VPP shared with another owner (w3).
type fakeNAT struct {
	*fake.Client
	enabled    bool
	cfg        nat44_ed.Nat44EdPluginEnableDisable
	timeouts   nat_types.NatTimeouts
	forwarding bool
	ifaces     []*interfaces.SwInterfaceDetails
	features   map[uint32]nat_types.NatConfigFlags
	outputs    map[uint32]bool
	ifAddrs    map[uint32]nat_types.NatConfigFlags
	addrs      []*nat44_ed.Nat44AddressDetails
	statics    []*nat44_ed.Nat44StaticMappingDetails
	idents     []*nat44_ed.Nat44IdentityMappingDetails
	lbs        []*nat44_ed.Nat44LbStaticMappingDetails
	vrfTables  map[uint32][]uint32
}

func newFakeNAT() *fakeNAT {
	f := &fakeNAT{
		Client:   fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		features: map[uint32]nat_types.NatConfigFlags{}, outputs: map[uint32]bool{}, ifAddrs: map[uint32]nat_types.NatConfigFlags{},
		vrfTables: map[uint32][]uint32{},
		ifaces: []*interfaces.SwInterfaceDetails{
			{SwIfIndex: 0, InterfaceName: "local0"},
			{SwIfIndex: 1, InterfaceName: "loop900", Tag: "w9:loop900"},
			{SwIfIndex: 2, InterfaceName: "loop901", Tag: "w9:loop901"},
			{SwIfIndex: 3, InterfaceName: "loop300", Tag: "w3:loop300"},
		},
	}
	f.On("sw_interface_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceDump)
		var out []api.Message
		for _, i := range f.ifaces {
			if !r.NameFilterValid || len(i.InterfaceName) >= len(r.NameFilter) && i.InterfaceName[:len(r.NameFilter)] == r.NameFilter {
				out = append(out, i)
			}
		}
		return out, nil
	})
	f.On("nat44_show_running_config", func(api.Message) ([]api.Message, error) {
		rep := &nat44_ed.Nat44ShowRunningConfigReply{Flags: nat44_ed.NAT44_IS_ENDPOINT_DEPENDENT}
		if f.enabled {
			rep.Sessions, rep.InsideVrf, rep.OutsideVrf = f.cfg.Sessions, f.cfg.InsideVrf, f.cfg.OutsideVrf
			rep.Flags |= f.cfg.Flags
			rep.Timeouts, rep.ForwardingEnabled = f.timeouts, f.forwarding
		}
		return []api.Message{rep}, nil
	})
	f.On("nat44_ed_plugin_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44EdPluginEnableDisable)
		rep := &nat44_ed.Nat44EdPluginEnableDisableReply{}
		switch {
		case r.Enable && f.enabled:
			rep.Retval = int32(api.FEATURE_ALREADY_ENABLED)
		case !r.Enable && !f.enabled:
			rep.Retval = int32(api.FEATURE_ALREADY_DISABLED)
		case r.Enable:
			f.enabled, f.cfg = true, *r
			if f.cfg.Sessions == 0 {
				f.cfg.Sessions = 63 * 1024
			}
			f.timeouts = nat_types.NatTimeouts{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}
		default:
			f.enabled, f.forwarding = false, false
			f.timeouts = nat_types.NatTimeouts{}
		}
		return []api.Message{rep}, nil
	})
	f.On("nat_set_timeouts", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.NatSetTimeouts)
		f.timeouts = nat_types.NatTimeouts{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		return []api.Message{&nat44_ed.NatSetTimeoutsReply{}}, nil
	})
	f.On("nat44_forwarding_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.forwarding = req.(*nat44_ed.Nat44ForwardingEnableDisable).Enable
		return []api.Message{&nat44_ed.Nat44ForwardingEnableDisableReply{}}, nil
	})
	f.On("nat44_interface_add_del_feature", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44InterfaceAddDelFeature)
		idx := uint32(r.SwIfIndex)
		if r.IsAdd {
			f.features[idx] |= r.Flags
		} else {
			f.features[idx] &^= r.Flags
			if f.features[idx] == 0 {
				delete(f.features, idx)
			}
		}
		return []api.Message{&nat44_ed.Nat44InterfaceAddDelFeatureReply{}}, nil
	})
	f.On("nat44_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if fl, ok := f.features[idx]; ok {
				out = append(out, &nat44_ed.Nat44InterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: fl})
			}
		}
		return out, nil
	})
	f.On("nat44_ed_add_del_output_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44EdAddDelOutputInterface)
		if r.IsAdd {
			f.outputs[uint32(r.SwIfIndex)] = true
		} else {
			delete(f.outputs, uint32(r.SwIfIndex))
		}
		return []api.Message{&nat44_ed.Nat44EdAddDelOutputInterfaceReply{}}, nil
	})
	f.On("nat44_ed_output_interface_get", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if f.outputs[idx] {
				out = append(out, &nat44_ed.Nat44EdOutputInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return append(out, &nat44_ed.Nat44EdOutputInterfaceGetReply{Cursor: ^uint32(0)}), nil
	})
	f.On("nat44_add_del_interface_addr", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44AddDelInterfaceAddr)
		if r.IsAdd {
			f.ifAddrs[uint32(r.SwIfIndex)] = r.Flags
		} else {
			delete(f.ifAddrs, uint32(r.SwIfIndex))
		}
		return []api.Message{&nat44_ed.Nat44AddDelInterfaceAddrReply{}}, nil
	})
	f.On("nat44_interface_addr_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if fl, ok := f.ifAddrs[idx]; ok {
				out = append(out, &nat44_ed.Nat44InterfaceAddrDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: fl})
			}
		}
		return out, nil
	})
	f.On("nat44_add_del_address_range", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44AddDelAddressRange)
		first, last := r.FirstIPAddress, r.LastIPAddress
		for a := first; ; a[3]++ {
			if r.IsAdd {
				f.addrs = append(f.addrs, &nat44_ed.Nat44AddressDetails{IPAddress: a, Flags: r.Flags, VrfID: r.VrfID})
			} else {
				for i, d := range f.addrs {
					if d.IPAddress == a && d.Flags == r.Flags {
						f.addrs = append(f.addrs[:i], f.addrs[i+1:]...)
						break
					}
				}
			}
			if a == last {
				break
			}
		}
		return []api.Message{&nat44_ed.Nat44AddDelAddressRangeReply{}}, nil
	})
	f.On("nat44_address_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.addrs))
		for _, a := range f.addrs {
			out = append(out, a)
		}
		return out, nil
	})
	f.On("nat44_add_del_static_mapping_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44AddDelStaticMappingV2)
		if r.IsAdd {
			f.statics = append(f.statics, &nat44_ed.Nat44StaticMappingDetails{Flags: r.Flags, LocalIPAddress: r.LocalIPAddress, ExternalIPAddress: r.ExternalIPAddress,
				Protocol: r.Protocol, LocalPort: r.LocalPort, ExternalPort: r.ExternalPort, ExternalSwIfIndex: r.ExternalSwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		} else {
			for i, m := range f.statics {
				if m.Tag == r.Tag {
					f.statics = append(f.statics[:i], f.statics[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat44_ed.Nat44AddDelStaticMappingV2Reply{}}, nil
	})
	f.On("nat44_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.statics))
		for _, m := range f.statics {
			out = append(out, m)
		}
		return out, nil
	})
	f.On("nat44_add_del_identity_mapping", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44AddDelIdentityMapping)
		if r.IsAdd {
			f.idents = append(f.idents, &nat44_ed.Nat44IdentityMappingDetails{Flags: r.Flags, IPAddress: r.IPAddress, Protocol: r.Protocol, Port: r.Port, SwIfIndex: r.SwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		} else {
			for i, m := range f.idents {
				if m.Tag == r.Tag {
					f.idents = append(f.idents[:i], f.idents[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat44_ed.Nat44AddDelIdentityMappingReply{}}, nil
	})
	f.On("nat44_identity_mapping_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.idents))
		for _, m := range f.idents {
			out = append(out, m)
		}
		return out, nil
	})
	f.On("nat44_add_del_lb_static_mapping", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44AddDelLbStaticMapping)
		if r.IsAdd {
			f.lbs = append(f.lbs, &nat44_ed.Nat44LbStaticMappingDetails{ExternalAddr: r.ExternalAddr, ExternalPort: r.ExternalPort, Protocol: r.Protocol, Flags: r.Flags, Affinity: r.Affinity, Tag: r.Tag, Locals: append([]nat44_ed.Nat44LbAddrPort{}, r.Locals...)})
		} else {
			for i, m := range f.lbs {
				if m.Tag == r.Tag {
					f.lbs = append(f.lbs[:i], f.lbs[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat44_ed.Nat44AddDelLbStaticMappingReply{}}, nil
	})
	f.On("nat44_lb_static_mapping_add_del_local", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44LbStaticMappingAddDelLocal)
		for _, m := range f.lbs {
			if m.ExternalAddr != r.ExternalAddr || m.ExternalPort != r.ExternalPort || m.Protocol != r.Protocol {
				continue
			}
			if r.IsAdd {
				m.Locals = append(m.Locals, r.Local)
			} else {
				for i, l := range m.Locals {
					if l == r.Local {
						m.Locals = append(m.Locals[:i], m.Locals[i+1:]...)
						break
					}
				}
			}
			return []api.Message{&nat44_ed.Nat44LbStaticMappingAddDelLocalReply{}}, nil
		}
		return []api.Message{&nat44_ed.Nat44LbStaticMappingAddDelLocalReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
	})
	f.On("nat44_lb_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.lbs))
		for _, m := range f.lbs {
			out = append(out, m)
		}
		return out, nil
	})
	f.On("nat44_ed_add_del_vrf_table", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44EdAddDelVrfTable)
		if r.IsAdd {
			f.vrfTables[r.TableVrfID] = []uint32{}
		} else {
			delete(f.vrfTables, r.TableVrfID)
		}
		return []api.Message{&nat44_ed.Nat44EdAddDelVrfTableReply{}}, nil
	})
	f.On("nat44_ed_add_del_vrf_route", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ed.Nat44EdAddDelVrfRoute)
		routes, ok := f.vrfTables[r.TableVrfID]
		if !ok {
			return []api.Message{&nat44_ed.Nat44EdAddDelVrfRouteReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		if r.IsAdd {
			f.vrfTables[r.TableVrfID] = append(routes, r.VrfID)
		} else {
			for i, v := range routes {
				if v == r.VrfID {
					f.vrfTables[r.TableVrfID] = append(routes[:i], routes[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat44_ed.Nat44EdAddDelVrfRouteReply{}}, nil
	})
	f.On("nat44_ed_vrf_tables_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for id, routes := range f.vrfTables {
			out = append(out, &nat44_ed.Nat44EdVrfTablesV2Details{TableVrfID: id, NVrfIds: uint32(len(routes)), VrfIds: routes}) //nolint:gosec // test data
		}
		return out, nil
	})
	f.On("nat44_user_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{&nat44_ed.Nat44UserDetails{VrfID: 0, IPAddress: [4]uint8{192, 168, 9, 2}, Nsessions: 2}}, nil
	})
	f.On("nat44_user_session_v3_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{
			&nat44_ed.Nat44UserSessionV3Details{InsideIPAddress: [4]uint8{192, 168, 9, 2}, InsidePort: 1000, OutsideIPAddress: [4]uint8{10, 9, 0, 1}, OutsidePort: 2000, Protocol: 6, ExtHostAddress: [4]uint8{8, 8, 8, 8}, ExtHostPort: 443, TotalPkts: 3},
			&nat44_ed.Nat44UserSessionV3Details{InsideIPAddress: [4]uint8{192, 168, 9, 2}, InsidePort: 1001, OutsideIPAddress: [4]uint8{10, 9, 0, 1}, OutsidePort: 2001, Protocol: 17, Flags: nat_types.NAT_IS_STATIC},
		}, nil
	})
	f.Reply("nat44_del_session", &nat44_ed.Nat44DelSessionReply{})
	return f
}

// apply mimics one scheduler pass for a single descriptor: Create what is missing, Update
// what differs (recreate on ErrRecreate), Delete owned leftovers; returns the operation count.
func apply(t *testing.T, d scheduler.Descriptor, desired ...proto.Message) int {
	t.Helper()
	ctx := context.Background()
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s retrieve: %v", d.Name(), err)
	}
	have := map[scheduler.Key]scheduler.KV{}
	for _, kv := range actual {
		have[kv.Key] = kv
	}
	ops := 0
	seen := map[scheduler.Key]bool{}
	for _, obj := range desired {
		k := d.KeyOf(obj)
		seen[k] = true
		cur, ok := have[k]
		switch {
		case !ok:
			if _, err := d.Create(ctx, obj); err != nil {
				t.Fatalf("%s create %s: %v", d.Name(), k, err)
			}
			ops++
		case !proto.Equal(cur.Value, obj):
			if _, err := d.Update(ctx, cur.Value, obj, cur.Meta); err != nil {
				if !errors.Is(err, scheduler.ErrRecreate) {
					t.Fatalf("%s update %s: %v", d.Name(), k, err)
				}
				if err := d.Delete(ctx, cur.Value, cur.Meta); err != nil {
					t.Fatal(err)
				}
				if _, err := d.Create(ctx, obj); err != nil {
					t.Fatal(err)
				}
			}
			ops++
		}
	}
	for k, kv := range have {
		if !seen[k] {
			if err := d.Delete(ctx, kv.Value, kv.Meta); err != nil {
				t.Fatalf("%s delete %s: %v", d.Name(), k, err)
			}
			ops++
		}
	}
	return ops
}

func retrieveKeys(t *testing.T, d scheduler.Descriptor) []string {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		out = append(out, string(kv.Key))
	}
	return out
}

func TestRegisterAndNames(t *testing.T) {
	reg := scheduler.NewRegistry()
	p := nat44ed.Register(reg, newFakeNAT(), "w9")
	if reg.Len() != len(p.Descriptors()) || reg.Len() != 11 {
		t.Fatalf("registered %d descriptors, want 11", reg.Len())
	}
	for _, d := range p.Descriptors() {
		if !scheduler.ValidName(d.Name()) {
			t.Fatalf("invalid name %q", d.Name())
		}
	}
	if _, ok := reg.ForKey(nat44ed.EnableKey); !ok {
		t.Fatal("enable key not routed")
	}
}

func TestEnableSingletonSharedVPP(t *testing.T) {
	f := newFakeNAT()
	p := nat44ed.New(f, "w9")
	ctx := context.Background()
	desired := natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 1024, InsideVRF: 9001})
	if len(retrieveKeys(t, p.Enable)) != 0 {
		t.Fatal("disabled plugin must retrieve nothing")
	}
	if apply(t, p.Enable, desired) != 1 || apply(t, p.Enable, desired) != 0 {
		t.Fatal("create then idempotent re-apply")
	}
	if req := f.CallsNamed("nat44_ed_plugin_enable_disable"); len(req) != 1 || !req[0].(*nat44_ed.Nat44EdPluginEnableDisable).Enable || req[0].(*nat44_ed.Nat44EdPluginEnableDisable).Sessions != 1024 {
		t.Fatalf("enable request %+v", req)
	}
	// foreign owner enabled it with another config → Create refuses instead of flipping it
	other := nat44ed.New(f, "w3")
	if _, err := other.Enable.Create(ctx, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 4096})); !errors.Is(err, nat44ed.ErrForeignObjects) {
		t.Fatalf("foreign create: %v", err)
	}
	// compatible desired state from another owner is converged, no VPP call
	f.Reset()
	if _, err := other.Enable.Create(ctx, desired); err != nil || len(f.CallsNamed("nat44_ed_plugin_enable_disable")) != 0 {
		t.Fatalf("compatible create: %v", err)
	}
	if deps := p.Enable.Dependencies(desired); len(deps) != 1 || deps[0].Key != "vrf/9001" {
		t.Fatalf("deps %+v", deps)
	}
	// Update → recreate (no foreign objects); Delete disables
	if _, err := p.Enable.Update(ctx, desired, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 2048}), nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("update: %v", err)
	}
	// a foreign pool blocks disable
	f.addrs = append(f.addrs, &nat44_ed.Nat44AddressDetails{IPAddress: [4]uint8{10, 3, 0, 1}})
	if err := p.Enable.Delete(ctx, desired, nil); !errors.Is(err, nat44ed.ErrForeignObjects) {
		t.Fatalf("delete with foreign pool: %v", err)
	}
	f.addrs = nil
	if err := p.Enable.Delete(ctx, desired, nil); err != nil || f.enabled {
		t.Fatalf("delete: %v enabled=%v", err, f.enabled)
	}
	// production owner owns everything: the foreign check is skipped
	prod := nat44ed.New(f, "vrx")
	f.enabled = true
	f.addrs = append(f.addrs, &nat44_ed.Nat44AddressDetails{IPAddress: [4]uint8{10, 3, 0, 1}})
	if err := prod.Enable.Delete(ctx, desired, nil); err != nil {
		t.Fatal(err)
	}
	f.SetConnected(false)
	if _, err := p.Enable.Retrieve(ctx); err == nil {
		t.Fatal("disconnected must fail")
	}
}

func TestTimeoutsAndForwarding(t *testing.T) {
	f := newFakeNAT()
	p := nat44ed.New(f, "w9")
	apply(t, p.Enable, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 1024}))
	tmo := natcommon.MustEncode(&nat44ed.TimeoutsSpec{UDP: 60, TCPEstablished: 600, TCPTransitory: 30, ICMP: 10})
	if apply(t, p.Timeouts, tmo) != 1 || apply(t, p.Timeouts, tmo) != 0 {
		t.Fatal("timeouts create/idempotent")
	}
	if f.timeouts.UDP != 60 || f.timeouts.TCPEstablished != 600 {
		t.Fatalf("timeouts not applied: %+v", f.timeouts)
	}
	tmo2 := natcommon.MustEncode(&nat44ed.TimeoutsSpec{UDP: 61, TCPEstablished: 600, TCPTransitory: 30, ICMP: 10})
	if apply(t, p.Timeouts, tmo2) != 1 || f.timeouts.UDP != 61 {
		t.Fatal("timeouts update in place")
	}
	if apply(t, p.Timeouts) != 1 || f.timeouts.UDP != nat44ed.DefaultUDPTimeout {
		t.Fatalf("delete restores defaults: %+v", f.timeouts)
	}
	if deps := p.Timeouts.Dependencies(tmo); len(deps) != 1 || deps[0].Key != nat44ed.EnableKey {
		t.Fatalf("deps %+v", deps)
	}
	fwd := natcommon.MustEncode(&nat44ed.ForwardingSpec{Enabled: true})
	if apply(t, p.Forwarding, fwd) != 1 || !f.forwarding || apply(t, p.Forwarding, fwd) != 0 {
		t.Fatal("forwarding")
	}
	if apply(t, p.Forwarding) != 1 || f.forwarding {
		t.Fatal("forwarding delete")
	}
}

func TestInterfaceObjects(t *testing.T) {
	f := newFakeNAT()
	p := nat44ed.New(f, "w9")
	apply(t, p.Enable, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 1024}))
	// another owner's interface has everything: invisible to us
	f.features[3] = nat_types.NAT_IS_INSIDE | nat_types.NAT_IS_OUTSIDE
	f.outputs[3] = true
	f.ifAddrs[3] = 0
	in := natcommon.MustEncode(&nat44ed.InterfaceFeatureSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&nat44ed.InterfaceFeatureSpec{Interface: "loop901", Side: "outside"})
	if apply(t, p.InterfaceFeature, in, out) != 2 || apply(t, p.InterfaceFeature, in, out) != 0 {
		t.Fatal("features create/idempotent")
	}
	if f.features[1] != nat_types.NAT_IS_INSIDE || f.features[2] != nat_types.NAT_IS_OUTSIDE {
		t.Fatalf("features %+v", f.features)
	}
	keys := retrieveKeys(t, p.InterfaceFeature)
	if len(keys) != 2 || keys[0] != "nat44-ed.interface-feature/loop900/inside" {
		t.Fatalf("keys %v", keys)
	}
	if deps := p.InterfaceFeature.Dependencies(in); len(deps) != 2 || deps[0].Key != nat44ed.EnableKey || deps[1].Key != "interface/loop900" {
		t.Fatalf("deps %+v", deps)
	}
	if _, err := p.InterfaceFeature.Create(context.Background(), natcommon.MustEncode(&nat44ed.InterfaceFeatureSpec{Interface: "loop900", Side: "sideways"})); err == nil {
		t.Fatal("bad side must fail")
	}
	if _, err := p.InterfaceFeature.Create(context.Background(), natcommon.MustEncode(&nat44ed.InterfaceFeatureSpec{Interface: "nope", Side: "inside"})); !errors.Is(err, natcommon.ErrNoSuchInterface) {
		t.Fatalf("missing interface: %v", err)
	}
	if _, err := p.InterfaceFeature.Update(context.Background(), in, natcommon.MustEncode(&nat44ed.InterfaceFeatureSpec{Interface: "loop900", Side: "outside"}), nat44ed.IfMeta{SwIfIndex: 1}); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal("update must recreate")
	}
	if apply(t, p.InterfaceFeature, in) != 1 || len(retrieveKeys(t, p.InterfaceFeature)) != 1 {
		t.Fatal("leftover delete")
	}
	if f.features[3] != nat_types.NAT_IS_INSIDE|nat_types.NAT_IS_OUTSIDE {
		t.Fatal("foreign features touched")
	}

	of := natcommon.MustEncode(&nat44ed.OutputFeatureSpec{Interface: "loop900"})
	if apply(t, p.OutputFeature, of) != 1 || apply(t, p.OutputFeature, of) != 0 || !f.outputs[1] {
		t.Fatal("output feature")
	}
	if apply(t, p.OutputFeature) != 1 || f.outputs[1] || !f.outputs[3] {
		t.Fatal("output feature delete / foreign untouched")
	}

	ia := natcommon.MustEncode(&nat44ed.InterfaceAddressSpec{Interface: "loop901", TwiceNAT: true})
	if apply(t, p.InterfaceAddress, ia) != 1 || apply(t, p.InterfaceAddress, ia) != 0 || f.ifAddrs[2] != nat_types.NAT_IS_TWICE_NAT {
		t.Fatal("interface address")
	}
	ia2 := natcommon.MustEncode(&nat44ed.InterfaceAddressSpec{Interface: "loop901"})
	if apply(t, p.InterfaceAddress, ia2) != 1 || f.ifAddrs[2] != 0 {
		t.Fatal("twice-nat flag change recreates")
	}
	apply(t, p.InterfaceAddress)
	if _, ok := f.ifAddrs[3]; !ok {
		t.Fatal("foreign interface address touched")
	}
}

func TestAddressPool(t *testing.T) {
	f := newFakeNAT()
	p := nat44ed.New(f, "w9")
	apply(t, p.Enable, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 1024}))
	f.addrs = append(f.addrs, &nat44_ed.Nat44AddressDetails{IPAddress: [4]uint8{10, 3, 0, 1}}) // w3's pool
	pool := natcommon.MustEncode(&nat44ed.AddressPoolSpec{First: "10.9.0.10", Last: "10.9.0.1"}) // reversed: normalised
	twice := natcommon.MustEncode(&nat44ed.AddressPoolSpec{First: "10.9.1.1", Last: "10.9.1.1", VRF: 9001, TwiceNAT: true})
	if apply(t, p.AddressPool, pool, twice) != 2 || apply(t, p.AddressPool, pool, twice) != 0 {
		t.Fatal("pools create/idempotent")
	}
	keys := retrieveKeys(t, p.AddressPool)
	if len(keys) != 2 || keys[0] != "nat44-ed.address-pool/10.9.0.1-10.9.0.10/0" || keys[1] != "nat44-ed.address-pool/10.9.1.1-10.9.1.1/9001/twice-nat" {
		t.Fatalf("keys %v", keys)
	}
	req := f.CallsNamed("nat44_add_del_address_range")[1].(*nat44_ed.Nat44AddDelAddressRange)
	if req.Flags != nat_types.NAT_IS_TWICE_NAT || req.VrfID != 9001 || !req.IsAdd {
		t.Fatalf("twice-nat request %+v", req)
	}
	if deps := p.AddressPool.Dependencies(twice); len(deps) != 2 || deps[1].Key != "vrf/9001" || !deps[1].Optional {
		t.Fatalf("deps %+v", deps)
	}
	if apply(t, p.AddressPool, twice) != 1 || len(f.addrs) != 2 {
		t.Fatalf("delete range: %d addresses left", len(f.addrs))
	}
	if f.addrs[0].IPAddress != [4]uint8{10, 3, 0, 1} {
		t.Fatal("foreign pool touched")
	}
	if _, err := p.AddressPool.Create(context.Background(), natcommon.MustEncode(&nat44ed.AddressPoolSpec{First: "bad"})); err == nil {
		t.Fatal("bad address must fail")
	}
}

func TestMappings(t *testing.T) {
	f := newFakeNAT()
	p := nat44ed.New(f, "w9")
	ctx := context.Background()
	apply(t, p.Enable, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 1024}))
	f.statics = append(f.statics, &nat44_ed.Nat44StaticMappingDetails{Tag: "w3:theirs", ExternalSwIfIndex: ^interface_types.InterfaceIndex(0)})
	pf := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "web", Local: nat44ed.Endpoint{IP: "192.168.9.10", Port: 80}, External: nat44ed.Endpoint{IP: "10.9.0.1", Port: 8080}, Protocol: "6", TwiceNAT: true})
	one2one := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "srv", Local: nat44ed.Endpoint{IP: "192.168.9.11", Port: 99}, External: nat44ed.Endpoint{Interface: "loop901", IP: "1.2.3.4"}, AddrOnly: true, Out2InOnly: true})
	if apply(t, p.StaticMapping, pf, one2one) != 2 || apply(t, p.StaticMapping, pf, one2one) != 0 {
		t.Fatal("static mappings create/idempotent")
	}
	reqs := f.CallsNamed("nat44_add_del_static_mapping_v2")
	r0 := reqs[0].(*nat44_ed.Nat44AddDelStaticMappingV2)
	if r0.Tag != "w9:web" || r0.Protocol != 6 || r0.LocalPort != 80 || r0.ExternalPort != 8080 || r0.Flags != nat_types.NAT_IS_TWICE_NAT || r0.ExternalSwIfIndex != ^interface_types.InterfaceIndex(0) {
		t.Fatalf("port-forward request %+v", r0)
	}
	r1 := reqs[1].(*nat44_ed.Nat44AddDelStaticMappingV2)
	if r1.ExternalSwIfIndex != 2 || r1.Flags != nat_types.NAT_IS_ADDR_ONLY|nat_types.NAT_IS_OUT2IN_ONLY || r1.LocalPort != 0 || r1.Protocol != 0 || r1.ExternalIPAddress != [4]uint8{} {
		t.Fatalf("addr-only request %+v", r1)
	}
	keys := retrieveKeys(t, p.StaticMapping)
	if len(keys) != 2 || keys[0] != "nat44-ed.static-mapping/web" || keys[1] != "nat44-ed.static-mapping/srv" {
		t.Fatalf("keys %v (foreign filtered)", keys)
	}
	if deps := p.StaticMapping.Dependencies(one2one); len(deps) != 2 || deps[1].Key != "interface/loop901" {
		t.Fatalf("deps %+v", deps)
	}
	pf2 := natcommon.MustEncode(&nat44ed.StaticMappingSpec{Name: "web", Local: nat44ed.Endpoint{IP: "192.168.9.10", Port: 80}, External: nat44ed.Endpoint{IP: "10.9.0.1", Port: 8081}, Protocol: "tcp", TwiceNAT: true})
	if apply(t, p.StaticMapping, pf2, one2one) != 1 || f.statics[2].ExternalPort != 8081 {
		t.Fatal("recreate on change")
	}
	if apply(t, p.StaticMapping) != 2 || len(f.statics) != 1 || f.statics[0].Tag != "w3:theirs" {
		t.Fatal("delete leftovers, foreign untouched")
	}

	id := natcommon.MustEncode(&nat44ed.IdentityMappingSpec{Name: "self", Interface: "loop900", Protocol: "udp", Port: 500})
	id2 := natcommon.MustEncode(&nat44ed.IdentityMappingSpec{Name: "self2", IP: "10.9.2.1", AddrOnly: true, VRF: 9002})
	if apply(t, p.IdentityMapping, id, id2) != 2 || apply(t, p.IdentityMapping, id, id2) != 0 {
		t.Fatal("identity mappings")
	}
	if deps := p.IdentityMapping.Dependencies(id2); len(deps) != 2 || deps[1].Key != "vrf/9002" {
		t.Fatalf("deps %+v", deps)
	}
	if apply(t, p.IdentityMapping) != 2 || len(f.idents) != 0 {
		t.Fatal("identity delete")
	}

	lb := natcommon.MustEncode(&nat44ed.LBStaticMappingSpec{Name: "lb", External: nat44ed.Endpoint{IP: "10.9.0.5", Port: 80}, Protocol: "tcp", Affinity: 10,
		Locals: []nat44ed.LBLocal{{IP: "192.168.9.2", Port: 8080, Probability: 50}, {IP: "192.168.9.1", Port: 8080, Probability: 50}}})
	if apply(t, p.LBStaticMapping, lb) != 1 || apply(t, p.LBStaticMapping, lb) != 0 {
		t.Fatal("lb mapping")
	}
	lb2 := natcommon.MustEncode(&nat44ed.LBStaticMappingSpec{Name: "lb", External: nat44ed.Endpoint{IP: "10.9.0.5", Port: 80}, Protocol: "tcp", Affinity: 10,
		Locals: []nat44ed.LBLocal{{IP: "192.168.9.1", Port: 8080, Probability: 50}, {IP: "192.168.9.3", Port: 8080, Probability: 50}}})
	if apply(t, p.LBStaticMapping, lb2) != 1 {
		t.Fatal("lb locals update")
	}
	if calls := f.CallsNamed("nat44_lb_static_mapping_add_del_local"); len(calls) != 2 || !calls[0].(*nat44_ed.Nat44LbStaticMappingAddDelLocal).IsAdd || calls[1].(*nat44_ed.Nat44LbStaticMappingAddDelLocal).IsAdd {
		t.Fatalf("locals diff: %+v", calls)
	}
	if apply(t, p.LBStaticMapping, lb2) != 0 || len(f.CallsNamed("nat44_add_del_lb_static_mapping")) != 1 {
		t.Fatal("locals update must not recreate")
	}
	lb3 := natcommon.MustEncode(&nat44ed.LBStaticMappingSpec{Name: "lb", External: nat44ed.Endpoint{IP: "10.9.0.5", Port: 81}, Protocol: "tcp", Locals: []nat44ed.LBLocal{}})
	if _, err := p.LBStaticMapping.Update(ctx, lb2, lb3, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal("external change recreates")
	}
	if apply(t, p.LBStaticMapping) != 1 || len(f.lbs) != 0 {
		t.Fatal("lb delete")
	}
}

func TestVRFTableAndSessions(t *testing.T) {
	f := newFakeNAT()
	p := nat44ed.New(f, "w9")
	apply(t, p.Enable, natcommon.MustEncode(&nat44ed.EnableSpec{Sessions: 1024}))
	f.vrfTables[3000] = []uint32{0} // w3's table: outside our range
	tbl := natcommon.MustEncode(&nat44ed.VRFTableSpec{Table: 9001, Routes: []uint32{9003, 9002, 9002}})
	if apply(t, p.VRFTable, tbl) != 1 || apply(t, p.VRFTable, tbl) != 0 {
		t.Fatal("vrf table")
	}
	if keys := retrieveKeys(t, p.VRFTable); len(keys) != 1 || keys[0] != "nat44-ed.vrf-table/9001" {
		t.Fatalf("keys %v", keys)
	}
	if deps := p.VRFTable.Dependencies(tbl); len(deps) != 4 {
		t.Fatalf("deps %+v", deps)
	}
	tbl2 := natcommon.MustEncode(&nat44ed.VRFTableSpec{Table: 9001, Routes: []uint32{9002, 9004}})
	if apply(t, p.VRFTable, tbl2) != 1 || fmt.Sprint(f.vrfTables[9001]) != "[9002 9004]" {
		t.Fatalf("routes update in place: %v", f.vrfTables[9001])
	}
	if apply(t, p.VRFTable) != 1 || len(f.vrfTables) != 1 {
		t.Fatal("delete, foreign kept")
	}

	users, err := p.Users(context.Background())
	if err != nil || len(users) != 1 || users[0].IP != "192.168.9.2" {
		t.Fatalf("users %+v %v", users, err)
	}
	page, err := p.UserSessions(context.Background(), users[0], 1, 1)
	if err != nil || len(page) != 1 || page[0].Protocol != "udp" || !page[0].Static {
		t.Fatalf("page %+v %v", page, err)
	}
	if err := p.DeleteSession(context.Background(), nat44ed.Endpoint{IP: "192.168.9.2", Port: 1000}, "tcp", 0, nat44ed.Endpoint{IP: "8.8.8.8", Port: 443}); err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("nat44_del_session")[0].(*nat44_ed.Nat44DelSession)
	if req.Flags != nat_types.NAT_IS_INSIDE|nat_types.NAT_IS_EXT_HOST_VALID || req.ExtHostPort != 443 || req.Protocol != 6 {
		t.Fatalf("del session %+v", req)
	}
}
