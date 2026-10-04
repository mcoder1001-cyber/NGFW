package pppoe_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	mssclamp "ngfw/agent/binapi/mss_clamp"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/pppoe"
)

// recordMirror wires a fake VPP that records the address/route/mss requests the ClientMirror sends and answers
// sw_interface_get_table with a configurable VRF.
type recordMirror struct {
	*df6test.FakeVPP
	table  uint32
	addrs  []*interfaces.SwInterfaceAddDelAddress
	routes []*ip.IPRouteAddDel
	mss    []*mssclamp.MssClampEnableDisable
}

func newRecordMirror(table uint32) *recordMirror {
	f := &recordMirror{FakeVPP: df6test.NewFakeVPP(), table: table}
	f.On("ip_address_dump", func(api.Message) ([]api.Message, error) { return nil, nil })
	f.On("sw_interface_add_del_address", func(req api.Message) ([]api.Message, error) {
		f.addrs = append(f.addrs, req.(*interfaces.SwInterfaceAddDelAddress))
		return []api.Message{&interfaces.SwInterfaceAddDelAddressReply{}}, nil
	})
	f.On("sw_interface_get_table", func(api.Message) ([]api.Message, error) {
		return []api.Message{&interfaces.SwInterfaceGetTableReply{VrfID: f.table}}, nil
	})
	f.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
		f.routes = append(f.routes, req.(*ip.IPRouteAddDel))
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	f.On("mss_clamp_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.mss = append(f.mss, req.(*mssclamp.MssClampEnableDisable))
		return []api.Message{&mssclamp.MssClampEnableDisableReply{}}, nil
	})
	return f
}

func allowAll(uint32) bool { return true }

func TestClientMirrorApplyUp(t *testing.T) {
	f := newRecordMirror(0)
	wan := f.AddInterface("wan0", "")
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	mir := pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", PeerIPv4: "203.0.113.1", DefaultRoute: true, MSSClamp: true, MTU: 1492}
	if err := m.Apply(context.Background(), mir, true); err != nil {
		t.Fatalf("Apply up: %v", err)
	}
	if len(f.addrs) != 1 || !f.addrs[0].IsAdd || uint32(f.addrs[0].SwIfIndex) != wan {
		t.Fatalf("address add on the WAN expected, got %+v", f.addrs)
	}
	if got := f.addrs[0].Prefix.String(); got != "203.0.113.7/32" {
		t.Fatalf("address prefix = %q, want 203.0.113.7/32", got)
	}
	if len(f.routes) != 1 || !f.routes[0].IsAdd || !f.routes[0].IsMultipath {
		t.Fatalf("default route add must be IsMultipath, got %+v", f.routes)
	}
	if got := f.routes[0].Route.Prefix.String(); got != "0.0.0.0/0" {
		t.Fatalf("route prefix = %q, want default", got)
	}
	if n := len(f.routes[0].Route.Paths); n != 1 || uint32(f.routes[0].Route.Paths[0].SwIfIndex) != wan {
		t.Fatalf("route must carry exactly this session's single path egressing the WAN, got %+v", f.routes[0].Route.Paths)
	}
	if len(f.mss) != 1 || uint32(f.mss[0].SwIfIndex) != wan || f.mss[0].IPv4Mss != 1452 {
		t.Fatalf("mss clamp on the WAN with MSS 1452 (1492-40) expected, got %+v", f.mss)
	}
	if f.mss[0].IPv4Direction != (mssclamp.MSS_CLAMP_DIR_RX | mssclamp.MSS_CLAMP_DIR_TX) {
		t.Fatalf("mss clamp should be RX+TX, got %v", f.mss[0].IPv4Direction)
	}
}

// The default route uses the interface's own FIB table (sw_interface_get_table), not a hard-coded 0.
func TestClientMirrorUsesInterfaceTable(t *testing.T) {
	f := newRecordMirror(9007) // the WAN is bound to VRF 9007 (a slot table)
	wan := f.AddInterface("wan0", "")
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	mir := pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", PeerIPv4: "203.0.113.1", DefaultRoute: true, MTU: 1492}
	if err := m.Apply(context.Background(), mir, true); err != nil {
		t.Fatalf("Apply up: %v", err)
	}
	if len(f.routes) != 1 || f.routes[0].Route.TableID != 9007 || f.routes[0].Route.Paths[0].TableID != 9007 {
		t.Fatalf("route and path must use table 9007, got %+v", f.routes)
	}
	_ = wan
}

// Withdraw removes only THIS session's path (IsMultipath=true, single path) — a static/ECMP/second-session default
// path in the same 0.0.0.0/0 entry survives (the delete never wipes the whole entry).
func TestClientMirrorApplyDownWithdrawsOnlyItsPath(t *testing.T) {
	f := newRecordMirror(0)
	wan := f.AddInterface("wan0", "")
	prefix, _ := ip_types.ParseAddressWithPrefix("203.0.113.7/32")
	f.On("ip_address_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ip.IPAddressDetails{Prefix: prefix}}, nil
	})
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	mir := pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", PeerIPv4: "203.0.113.1", DefaultRoute: true, MSSClamp: true, MTU: 1492}
	if err := m.Apply(context.Background(), mir, false); err != nil {
		t.Fatalf("Apply down: %v", err)
	}
	if len(f.addrs) != 1 || f.addrs[0].IsAdd {
		t.Fatalf("address delete expected, got %+v", f.addrs)
	}
	if len(f.routes) != 1 || f.routes[0].IsAdd || !f.routes[0].IsMultipath {
		t.Fatalf("default route delete must be IsMultipath (path-only), got %+v", f.routes)
	}
	if n := len(f.routes[0].Route.Paths); n != 1 || uint32(f.routes[0].Route.Paths[0].SwIfIndex) != wan {
		t.Fatalf("delete must carry exactly this session's single path (not a whole-entry delete), got %+v", f.routes[0].Route.Paths)
	}
	if len(f.mss) != 1 || f.mss[0].IPv4Direction != mssclamp.MSS_CLAMP_DIR_NONE || f.mss[0].IPv4Mss != 0 {
		t.Fatalf("mss clamp disable (dir NONE, mss 0) expected, got %+v", f.mss)
	}
}

// A default route in a table the policy does not allow is refused (fail closed) and nothing reaches the FIB.
func TestClientMirrorDefaultRouteRefusedByPolicy(t *testing.T) {
	f := newRecordMirror(500) // a table outside this agent's range
	f.AddInterface("wan0", "")
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(func(table uint32) bool { return table == 9000 }))
	mir := pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", PeerIPv4: "203.0.113.1", DefaultRoute: true, MTU: 1492}
	err := m.Apply(context.Background(), mir, true)
	if !errors.Is(err, pppoe.ErrNotGlobalsOwner) {
		t.Fatalf("default route in a disallowed table must be refused, got %v", err)
	}
	if len(f.routes) != 0 {
		t.Fatalf("no route may be sent when refused, got %+v", f.routes)
	}
	// the policy is checked before anything is sent: a refusal is permanent, so no address may be left behind.
	if len(f.addrs) != 0 || len(f.mss) != 0 {
		t.Fatalf("a refused up must send nothing, got addrs=%+v mss=%+v", f.addrs, f.mss)
	}
}

// Without a policy, NewClientMirror refuses every default route (fail closed).
func TestClientMirrorDefaultRouteFailClosed(t *testing.T) {
	f := newRecordMirror(0)
	f.AddInterface("wan0", "")
	m := pppoe.NewClientMirror(f, "w9", nil) // no WithRouteTablePolicy
	err := m.Apply(context.Background(), pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", DefaultRoute: true}, true)
	if !errors.Is(err, pppoe.ErrNotGlobalsOwner) {
		t.Fatalf("a mirror with no route policy must refuse the default route, got %v", err)
	}
	if len(f.addrs)+len(f.routes) != 0 {
		t.Fatalf("a refused up must send nothing, got addrs=%+v routes=%+v", f.addrs, f.routes)
	}
}

func TestClientMirrorWithdrawMissingInterfaceIsNoop(t *testing.T) {
	f := newRecordMirror(0) // no wan0 added
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	if err := m.Apply(context.Background(), pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32"}, false); err != nil {
		t.Fatalf("withdraw of a gone interface should be a no-op, got %v", err)
	}
	if len(f.addrs)+len(f.routes)+len(f.mss) != 0 {
		t.Fatalf("no VPP calls expected for a gone interface, got %d/%d/%d", len(f.addrs), len(f.routes), len(f.mss))
	}
}

func TestClientMirrorAddMissingInterfaceErrors(t *testing.T) {
	f := newRecordMirror(0)
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	if err := m.Apply(context.Background(), pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32"}, true); err == nil {
		t.Fatal("adding a mirror for a missing interface must error")
	}
}

func TestClientMirrorWithdrawalRetrySkipsAlreadyRemovedAddress(t *testing.T) {
	f := newRecordMirror(0)
	f.AddInterface("wan0", "")
	prefix, _ := ip_types.ParseAddressWithPrefix("203.0.113.7/32")
	present := true
	deletes := 0
	routeDeletes := 0
	f.On("ip_address_dump", func(api.Message) ([]api.Message, error) {
		if present {
			return []api.Message{&ip.IPAddressDetails{Prefix: prefix}}, nil
		}
		return nil, nil
	})
	f.On("sw_interface_add_del_address", func(_ api.Message) ([]api.Message, error) {
		if !present {
			return nil, errors.New("absent address delete")
		}
		present = false
		deletes++
		return []api.Message{&interfaces.SwInterfaceAddDelAddressReply{}}, nil
	})
	f.On("ip_route_add_del", func(api.Message) ([]api.Message, error) {
		routeDeletes++
		if routeDeletes == 1 {
			return nil, errors.New("temporary route failure")
		}
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	mirror := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	record := pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", DefaultRoute: true}
	if err := mirror.Apply(context.Background(), record, false); err == nil {
		t.Fatal("expected partial withdrawal")
	}
	if err := mirror.Apply(context.Background(), record, false); err != nil {
		t.Fatal("retry failed after address already removed", err)
	}
	if deletes != 1 || routeDeletes != 2 {
		t.Fatal(deletes, routeDeletes)
	}
}
func TestClientMirrorFailedAddressAddDownDoesNotDeleteAbsentAddress(t *testing.T) {
	f := newRecordMirror(0)
	f.AddInterface("wan0", "")
	writes := 0
	f.On("sw_interface_add_del_address", func(req api.Message) ([]api.Message, error) {
		writes++
		if !req.(*interfaces.SwInterfaceAddDelAddress).IsAdd {
			t.Fatal("deleting an absent address")
		}
		return nil, errors.New("address add rejected")
	})
	f.On("ip_route_add_del", func(api.Message) ([]api.Message, error) { return nil, api.NO_SUCH_ENTRY })
	mirror := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	record := pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", DefaultRoute: true}
	if err := mirror.Apply(context.Background(), record, true); err == nil {
		t.Fatal("expected address rejection")
	}
	if err := mirror.Apply(context.Background(), record, false); err != nil {
		t.Fatal("failed add cleanup refused", err)
	}
	if writes != 1 {
		t.Fatal(writes)
	}
}
