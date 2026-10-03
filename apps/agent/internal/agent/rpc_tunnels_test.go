package agent

// F-tunnels: tunnels.gre / .ipip / .vxlan through the agent against the unit-test VPP model
// (coretest/tunnels.go): apply → Retrieve == desired, idempotent re-apply, agent restart, rollback,
// the TD-8b id range and the agent-side checks.

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/subsystems"
)

const tunnelsDoc = `{
  "vrfs": {"red": {"id": 7100}},
  "interfaces": {"loop7001": {"ipv4": ["10.7.1.1/24"]}},
  "tunnels": {
    "gre": {"to-dc": {"instance": 7001, "description": "data centre", "src": "10.7.1.1", "dst": "10.7.1.2",
      "vrf": "red", "mtu": 1400, "ipv4": ["10.254.0.1/30"]}},
    "ipip": {"ipip-a": {"instance": 7002, "src": "10.7.1.1", "dst": "10.7.1.3", "dscp": 10, "ipv6": ["2001:db8:7::1/64"]}},
    "vxlan": {"vx-l3": {"instance": 7003, "src": "10.7.1.1", "dst": "10.7.1.4", "vni": 100, "decap": "ip6",
      "ipv6": ["2001:db8:9::1/64"]}}
  }
}`

// tunnelsRetrieved is tunnelsDoc's `tunnels` as Retrieve reports it (schema defaults filled in).
const tunnelsRetrieved = `{
  "gre": {"to-dc": {"enabled": true, "description": "data centre", "instance": 7001, "src": "10.7.1.1", "dst": "10.7.1.2",
    "underlayVrf": "default", "vrf": "red", "mtu": 1400, "ipv4": ["10.254.0.1/30"], "type": "l3"}},
  "ipip": {"ipip-a": {"enabled": true, "instance": 7002, "src": "10.7.1.1", "dst": "10.7.1.3", "underlayVrf": "default",
    "vrf": "default", "ipv6": ["2001:db8:7::1/64"], "mode": "p2p", "dscp": 10}},
  "vxlan": {"vx-l3": {"enabled": true, "instance": 7003, "src": "10.7.1.1", "dst": "10.7.1.4", "underlayVrf": "default",
    "vrf": "default", "ipv6": ["2001:db8:9::1/64"], "vni": 100, "srcPort": 4789, "dstPort": 4789, "decap": "ip6"}}
}`

func tunnelsRetrieve(t *testing.T, s *Service) *ngfwv1.DesiredState {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"interfaces", "tunnels"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

func TestTunnelsApplyRetrieveRestartRollback(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	v := coretest.New()
	dir := t.TempDir()
	s := newLispSvc(t, v, dir, false)

	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "t1", DesiredState: doc(t, tunnelsDoc)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := v.TunnelCount(); n != 3 {
		t.Fatalf("VPP holds %d tunnels, want 3", n)
	}
	want := &ngfwv1.TunnelsConfig{}
	if err := protojson.Unmarshal([]byte(tunnelsRetrieved), want); err != nil {
		t.Fatal(err)
	}
	ds := tunnelsRetrieve(t, s)
	if !proto.Equal(ds.GetTunnels(), want) {
		t.Fatalf("Retrieve tunnels:\n got %s\nwant %s", protojson.Format(ds.GetTunnels()), protojson.Format(want))
	}
	for _, n := range []string{"gre7001", "ipip7002", "vxlan_tunnel7003"} {
		if _, ok := ds.GetInterfaces()[n]; ok {
			t.Errorf("interfaces.%s reported: the tunnel interface belongs to tunnels.*", n)
		}
	}
	if i, ok := v.InterfaceByName("gre7001"); !ok || !i.AdminUp || i.Table4 != 7100 || !i.Addrs["10.254.0.1/30"] {
		t.Fatalf("gre7001 in VPP: %+v (want up, table 7100, 10.254.0.1/30)", i)
	}
	if i, _ := v.InterfaceByName("vxlan_tunnel7003"); i.L2 != [6]uint8{} {
		t.Fatalf("decap ip6 created an L2 VXLAN tunnel: %+v", i)
	}

	// idempotent re-apply, then an agent restart (same state dir, same VPP): nothing to do, names kept
	if r := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "t2", DesiredState: doc(t, tunnelsDoc)}); changes(r) != 0 {
		t.Fatalf("re-apply changed %d objects", changes(r))
	}
	s.Close()
	s2 := newLispSvc(t, v, dir, false)
	if r := apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "t3", DesiredState: doc(t, tunnelsDoc)}); changes(r) != 0 {
		t.Fatalf("apply after restart changed %d objects: %s", changes(r), protojson.Format(r))
	}
	if got := tunnelsRetrieve(t, s2).GetTunnels(); !proto.Equal(got, want) {
		t.Fatalf("Retrieve after restart: %s", protojson.Format(got))
	}

	// a tunnel lost behind the agent's back is re-created by the next apply
	v.DeleteInterface("ipip7002")
	mustStatus(t, apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "t4", DesiredState: doc(t, tunnelsDoc)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("ipip7002"); !ok {
		t.Fatal("ipip7002 not re-created")
	}

	// rollback to no tunnels: every tunnel and its attributes go, the loopback stays
	rb := apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "t5", DesiredState: doc(t, `{"vrfs": {"red": {"id": 7100}}, "interfaces": {"loop7001": {"ipv4": ["10.7.1.1/24"]}}, "tunnels": {}}`)})
	mustStatus(t, rb, ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var dels []string
	tunnelAt, lastAttr := map[string]int{}, map[string]int{}
	for _, r := range rb.GetResults() {
		if r.GetOp() != ngfwv1.ApplyOperation_APPLY_OPERATION_DELETE {
			continue
		}
		parts := strings.SplitN(r.GetKey(), "/", 3)
		switch parts[0] {
		case "gre.tunnel", "ipip.tunnel", "vxlan.tunnel":
			tunnelAt[parts[1]] = len(dels)
		case "interface-ip", "interface-ip.table", "interface.mtu", "interface.admin-state":
			lastAttr[parts[1]] = len(dels)
		}
		dels = append(dels, r.GetKey())
	}
	t.Logf("rollback deletes: %s", strings.Join(dels, " → "))
	for ifn, at := range lastAttr {
		if tat, ok := tunnelAt[ifn]; !ok || tat < at {
			t.Fatalf("%s must be deleted after its attributes: %v", ifn, dels)
		}
	}
	if n := v.TunnelCount(); n != 0 {
		t.Fatalf("%d tunnels left after the rollback", n)
	}
	if got := tunnelsRetrieve(t, s2); len(got.GetTunnels().GetGre())+len(got.GetTunnels().GetIpip())+len(got.GetTunnels().GetVxlan()) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got.GetTunnels()))
	}
	if _, ok := v.InterfaceByName("loop7001"); !ok {
		t.Fatal("the rollback removed the loopback")
	}
}

func TestTunnelsNeverTakeOverForeignTunnel(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	v := coretest.New()
	v.AddInterface("gre7001", "GRE tunnel device", "w9:gre7001") // another owner's
	s := newLispSvc(t, v, t.TempDir(), false)
	if got := tunnelsRetrieve(t, s).GetTunnels(); len(got.GetGre()) != 0 {
		t.Fatalf("another owner's tunnel reported: %s", protojson.Format(got))
	}
	r := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "f1", DesiredState: doc(t, tunnelsDoc)})
	if r.GetStatus() == ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatal("applied over another owner's gre7001")
	}
	if i, _ := v.InterfaceByName("gre7001"); i.Tag != "w9:gre7001" {
		t.Fatalf("foreign tunnel touched: %+v", i)
	}
}

func TestTunnelsProjection(t *testing.T) {
	cases := []struct {
		name, env, js, ptr, rule string
	}{
		{"no id range (TD-8b fail closed)", "", `{"tunnels": {"gre": {"g": {"instance": 7001, "src": "10.0.0.1", "dst": "10.0.0.2"}}}}`,
			"/tunnels/gre/g/instance", "tunnels.instance-range"},
		{"outside the slot range", "7000", `{"tunnels": {"vxlan": {"x": {"instance": 5, "src": "10.0.0.1", "dst": "10.0.0.2", "vni": 1}}}}`,
			"/tunnels/vxlan/x/instance", "tunnels.instance-range"},
		{"no instance", "all", `{"tunnels": {"ipip": {"i": {"src": "10.0.0.1", "dst": "10.0.0.2"}}}}`,
			"/tunnels/ipip/i/instance", "tunnels.instance-required"},
		{"duplicate instance", "all", `{"tunnels": {"gre": {"a": {"instance": 1, "src": "10.0.0.1", "dst": "10.0.0.2"}, "b": {"instance": 1, "src": "10.0.0.1", "dst": "10.0.0.3"}}}}`,
			"/tunnels/gre/b/instance", "tunnels.instance-unique"},
		{"unknown underlay VRF", "all", `{"tunnels": {"gre": {"a": {"instance": 1, "src": "10.0.0.1", "dst": "10.0.0.2", "underlayVrf": "nope"}}}}`,
			"/tunnels/gre/a/underlayVrf", "tunnels.vrf-exists"},
		{"bridge domain on an L3 tunnel", "all", `{"tunnels": {"vxlan": {"x": {"instance": 1, "src": "10.0.0.1", "dst": "10.0.0.2", "vni": 1, "decap": "ip4", "bridgeDomain": 7}}}}`,
			"/tunnels/vxlan/x/bridgeDomain", "tunnels.value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(subsystems.EnvIDRange, "")
			t.Setenv(subsystems.EnvTableBase, "")
			switch c.env {
			case "all":
				t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
			case "":
			default:
				t.Setenv(subsystems.EnvTableBase, c.env)
			}
			p := project(doc(t, c.js), []string{"interfaces", "tunnels"}, nil, nil)
			var found bool
			for _, is := range p.issues {
				found = found || (is.pointer == c.ptr && is.rule == c.rule && is.severity == ngfwv1.IssueSeverity_ISSUE_SEVERITY_ERROR)
			}
			if !found {
				t.Fatalf("want %s %s, issues %+v", c.ptr, c.rule, p.issues)
			}
		})
	}
	// in range with a slot base: accepted; an L2 VXLAN goes into its bridge domain
	t.Setenv(subsystems.EnvIDRange, "")
	t.Setenv(subsystems.EnvTableBase, "7000")
	p := project(doc(t, `{"tunnels": {"vxlan": {"x": {"instance": 7005, "src": "10.0.0.1", "dst": "10.0.0.2", "vni": 1, "bridgeDomain": 7010}}}}`), []string{"interfaces", "tunnels"}, nil, nil)
	var keys []string
	for _, kv := range p.kvs {
		keys = append(keys, string(kv.Key))
	}
	joined := strings.Join(keys, " ")
	for _, k := range []string{"vxlan.tunnel/vxlan_tunnel7005", "tunnels.meta/vxlan_tunnel7005", "interface/vxlan_tunnel7005", "l2.bridge-domain-member/7010/vxlan_tunnel7005"} {
		if !strings.Contains(joined, k) {
			t.Errorf("missing %s in %s (issues %+v)", k, joined, p.issues)
		}
	}
	// only `tunnels` in the transaction: the tunnel, no interface attribute, a warning
	p = project(doc(t, `{"tunnels": {"gre": {"g": {"instance": 7001, "src": "10.0.0.1", "dst": "10.0.0.2", "ipv4": ["10.1.0.1/30"]}}}}`), []string{"tunnels"}, nil, nil)
	if len(p.kvs) != 2 || len(p.issues) != 1 || p.issues[0].rule != "tunnels.interfaces-domain" {
		t.Fatalf("kvs %v issues %+v", p.kvs, p.issues)
	}
}

func TestTunnelsL2VxlanInBridgeDomain(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	v := coretest.New()
	s := newLispSvc(t, v, t.TempDir(), false)
	js := `{"interfaces": {"loop7001": {"ipv4": ["10.7.1.1/24"]}},
	  "routing": {"l2": {"bridgeDomains": {"lan": {"id": 7010}}}},
	  "tunnels": {"vxlan": {"vx": {"instance": 7004, "src": "10.7.1.1", "dst": "10.7.1.9", "vni": 7004, "bridgeDomain": 7010}}}}`
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "b1", DesiredState: doc(t, js)}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"interfaces", "routing", "tunnels"}})
	if err != nil {
		t.Fatal(err)
	}
	ds := got.GetDesiredState()
	x := ds.GetTunnels().GetVxlan()["vx"]
	if x.GetBridgeDomain() != 7010 || x.GetDecap() != "l2" {
		t.Fatalf("vxlan: %s", protojson.Format(x))
	}
	if _, ok := ds.GetInterfaces()["vxlan_tunnel7004"]; ok {
		t.Fatalf("the bridge membership was reported again as interfaces.vxlan_tunnel7004: %s", protojson.Format(ds.GetInterfaces()["vxlan_tunnel7004"]))
	}
	if r := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "b2", DesiredState: doc(t, js)}); changes(r) != 0 {
		t.Fatalf("re-apply changed %d objects: %s", changes(r), protojson.Format(r))
	}
}
