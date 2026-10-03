package agent

// S-tunnels-contract (T1): the kinds VPP names itself (vxlan-gpe, gtpu, l2tpv3, pppoe) and IPIP 6RD
// through the projection, a VXLAN-GPE tunnel through the unit-test VPP model (apply → Retrieve ==
// desired, restart, rollback) and the TunnelState RPC over tagged interfaces.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	ipipapi "ngfw/agent/binapi/ipip"
	gpeapi "ngfw/agent/binapi/vxlan_gpe"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/subsystems"
)

func TestTunnelsT1Projection(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	js := `{"tunnels": {
	  "ipip": {"rd": {"src": "10.0.0.1", "sixrd": {"ip6Prefix": "2001:db8:6::/48", "ip4Prefix": "10.0.0.0/8", "securityCheck": true}, "ipv6": ["2001:db8:6::1/64"]}},
	  "vxlanGpe": {"gpe-a": {"src": "10.0.0.1", "dst": "10.0.0.2", "vni": 300, "protocol": "ip4", "ipv4": ["10.254.3.1/30"]}},
	  "gtpu": {"upf": {"src": "10.0.0.1", "dst": "10.0.0.3", "teid": 7, "decap": "l2", "bridgeDomain": 5}},
	  "l2tpv3": {"l2": {"src": "2001:db8::1", "dst": "2001:db8::2", "localSessionId": 1, "remoteSessionId": 2, "bridgeDomain": 5}},
	  "pppoe": {"cpe": {"sessionId": 9, "clientMac": "02:00:00:00:00:01", "clientIp": "10.99.0.2", "mtu": 1492}}
	}}`
	p := project(doc(t, js), []string{"interfaces", "tunnels"}, nil, nil)
	var keys []string
	for _, kv := range p.kvs {
		keys = append(keys, string(kv.Key))
	}
	joined := strings.Join(keys, " ")
	for _, k := range []string{
		"ipip.sixrd/rd", "tunnels.meta/rd", "interface/rd", "interface-ip/rd/2001:db8:6::1/64",
		"vxlan-gpe.tunnel/gpe-a", "tunnels.meta/gpe-a", "interface/gpe-a", "interface-ip/gpe-a/10.254.3.1/30",
		"gtpu.tunnel/upf", "l2.bridge-domain-member/5/upf",
		"l2tp.tunnel/l2", "l2.bridge-domain-member/5/l2",
		"pppoe.session/02:00:00:00:00:01/9", "tunnels.meta/02:00:00:00:00:01/9", "interface.mtu/02:00:00:00:00:01/9",
	} {
		if !strings.Contains(joined, k) {
			t.Errorf("missing %s in %s (issues %+v)", k, joined, p.issues)
		}
	}
	var noDelete bool
	for _, is := range p.issues {
		if is.rule == "tunnels.l2tpv3-no-delete" && is.pointer == "/tunnels/l2tpv3/l2" && is.severity == vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING {
			noDelete = true
		}
		if is.severity == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			t.Errorf("unexpected error %+v", is)
		}
	}
	if !noDelete {
		t.Errorf("no tunnels.l2tpv3-no-delete warning: %+v", p.issues)
	}
	// value checks: a non-default underlay refuses L2TPv3; unknown protocol / decap
	for _, c := range []struct{ js, ptr, rule string }{
		{`{"vrfs": {"red": {"id": 7}}, "tunnels": {"l2tpv3": {"l": {"src": "2001:db8::1", "dst": "2001:db8::2", "underlayVrf": "red"}}}}`, "/tunnels/l2tpv3/l/underlayVrf", "tunnels.value"},
		{`{"tunnels": {"vxlanGpe": {"g": {"src": "10.0.0.1", "dst": "10.0.0.2", "protocol": "sctp"}}}}`, "/tunnels/vxlanGpe/g/protocol", "tunnels.value"},
		{`{"tunnels": {"gtpu": {"g": {"src": "10.0.0.1", "dst": "10.0.0.2", "vrf": "nope"}}}}`, "/tunnels/gtpu/g/vrf", "tunnels.vrf-exists"},
	} {
		p := project(doc(t, c.js), []string{"interfaces", "tunnels"}, nil, nil)
		var found bool
		for _, is := range p.issues {
			found = found || (is.pointer == c.ptr && is.rule == c.rule)
		}
		if !found {
			t.Errorf("want %s %s, issues %+v", c.ptr, c.rule, p.issues)
		}
	}
}

// installGPE models the vxlan-gpe plugin on the unit-test VPP: add/del creating a modelled interface
// named vxlan_gpe_tunnel<n> with VPP 26.06's device class, and the v2 dump.
func installGPE(v *coretest.VPP) {
	var mu sync.Mutex
	tunnels := map[uint32]*gpeapi.VxlanGpeTunnelV2Details{}
	n := 0
	v.On("vxlan_gpe_add_del_tunnel_v2", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*gpeapi.VxlanGpeAddDelTunnelV2)
		mu.Lock()
		defer mu.Unlock()
		if r.IsAdd {
			idx := v.AddInterface(fmt.Sprintf("vxlan_gpe_tunnel%d", n), "VXLAN_GPE", "")
			n++
			tunnels[idx] = &gpeapi.VxlanGpeTunnelV2Details{SwIfIndex: interface_types.InterfaceIndex(idx), Local: r.Local, Remote: r.Remote,
				LocalPort: r.LocalPort, RemotePort: r.RemotePort, Vni: r.Vni, Protocol: r.Protocol, McastSwIfIndex: r.McastSwIfIndex,
				EncapVrfID: r.EncapVrfID, DecapVrfID: r.DecapVrfID}
			return []api.Message{&gpeapi.VxlanGpeAddDelTunnelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
		}
		for idx, d := range tunnels {
			if d.Local == r.Local && d.Remote == r.Remote && d.Vni == r.Vni {
				delete(tunnels, idx)
				v.DeleteInterface(fmt.Sprintf("vxlan_gpe_tunnel%d", idxName(v, idx)))
				return []api.Message{&gpeapi.VxlanGpeAddDelTunnelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
		return []api.Message{&gpeapi.VxlanGpeAddDelTunnelV2Reply{Retval: coretest.RetvalNoSuchEntry}}, nil
	})
	v.On("vxlan_gpe_tunnel_v2_dump", func(api.Message) ([]api.Message, error) {
		mu.Lock()
		defer mu.Unlock()
		var out []api.Message
		for idx, d := range tunnels {
			if _, ok := v.InterfaceByName(fmt.Sprintf("vxlan_gpe_tunnel%d", idxName(v, idx))); !ok {
				continue // removed behind the agent's back
			}
			out = append(out, d)
		}
		return out, nil
	})
}

// idxName finds the model's name suffix of a tunnel by its sw_if_index (the model names by creation order).
func idxName(v *coretest.VPP, idx uint32) uint32 {
	for i := uint32(0); i < 64; i++ {
		if it, ok := v.InterfaceByName(fmt.Sprintf("vxlan_gpe_tunnel%d", i)); ok && it.Index == idx {
			return i
		}
	}
	return ^uint32(0)
}

const gpeDoc = `{
  "interfaces": {"loop7001": {"ipv4": ["10.7.1.1/24"]}},
  "tunnels": {"vxlanGpe": {"gpe-a": {"description": "to dc", "src": "10.7.1.1", "dst": "10.7.1.9", "vni": 300, "protocol": "ip4", "mtu": 1400, "ipv4": ["10.254.3.1/30"]}}}
}`

const gpeRetrieved = `{"vxlanGpe": {"gpe-a": {"enabled": true, "description": "to dc", "src": "10.7.1.1", "dst": "10.7.1.9", "underlayVrf": "default",
  "vrf": "default", "mtu": 1400, "ipv4": ["10.254.3.1/30"], "vni": 300, "srcPort": 4790, "dstPort": 4790, "protocol": "ip4"}}}`

func TestTunnelsT1VxlanGpeApplyRetrieveRestartRollback(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	v := coretest.New()
	installGPE(v)
	dir := t.TempDir()
	s := newLispSvc(t, v, dir, false)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "g1", DesiredState: doc(t, gpeDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	want := &vrxv1.TunnelsConfig{}
	if err := protojson.Unmarshal([]byte(gpeRetrieved), want); err != nil {
		t.Fatal(err)
	}
	ds := tunnelsRetrieve(t, s)
	if !proto.Equal(ds.GetTunnels(), want) {
		t.Fatalf("Retrieve tunnels:\n got %s\nwant %s", protojson.Format(ds.GetTunnels()), protojson.Format(want))
	}
	if _, ok := ds.GetInterfaces()["gpe-a"]; ok {
		t.Error("interfaces.gpe-a reported: the tunnel interface belongs to tunnels.vxlanGpe")
	}
	i, ok := v.InterfaceByName("vxlan_gpe_tunnel0")
	if !ok || !i.AdminUp || i.Tag != testOwner+":gpe-a" || !i.Addrs["10.254.3.1/30"] {
		t.Fatalf("vxlan_gpe_tunnel0 in VPP: %+v (want up, tag %s:gpe-a, 10.254.3.1/30)", i, testOwner)
	}
	// TunnelState names it from the tag and sees the F-tunnels kinds too
	v.AddInterface("gre7009", "GRE tunnel device", "w9:gre7009") // another owner's: never listed
	st, err := s.TunnelState(context.Background(), &vrxv1.TunnelStateRequest{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.GetTunnels()) != 1 || st.GetTunnels()[0].GetName() != "gpe-a" || st.GetTunnels()[0].GetKind() != "vxlanGpe" ||
		st.GetTunnels()[0].GetInterface() != "vxlan_gpe_tunnel0" || !st.GetTunnels()[0].GetAdminUp() || st.GetTunnels()[0].GetDeviceClass() != "VXLAN_GPE" {
		t.Fatalf("TunnelState: %s", protojson.Format(st))
	}
	if _, err := s.TunnelState(context.Background(), &vrxv1.TunnelStateRequest{Owner: "w9"}); err == nil {
		t.Fatal("TunnelState for another owner accepted")
	}
	// idempotent, restart
	if r := apply(t, s, &vrxv1.ApplyRequest{TxnId: "g2", DesiredState: doc(t, gpeDoc)}); changes(r) != 0 {
		t.Fatalf("re-apply changed %d objects: %s", changes(r), protojson.Format(r))
	}
	s.Close()
	s2 := newLispSvc(t, v, dir, false)
	if r := apply(t, s2, &vrxv1.ApplyRequest{TxnId: "g3", DesiredState: doc(t, gpeDoc)}); changes(r) != 0 {
		t.Fatalf("apply after restart changed %d objects: %s", changes(r), protojson.Format(r))
	}
	// rollback: the tunnel and its attributes go, nothing is left
	rb := apply(t, s2, &vrxv1.ApplyRequest{TxnId: "g4", DesiredState: doc(t, `{"interfaces": {"loop7001": {"ipv4": ["10.7.1.1/24"]}}, "tunnels": {}}`)})
	mustStatus(t, rb, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("vxlan_gpe_tunnel0"); ok {
		t.Fatal("vxlan_gpe_tunnel0 left after the rollback")
	}
	if got := tunnelsRetrieve(t, s2).GetTunnels(); len(got.GetVxlanGpe()) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	st, _ = s2.TunnelState(context.Background(), &vrxv1.TunnelStateRequest{Owner: testOwner})
	if len(st.GetTunnels()) != 0 {
		t.Fatalf("TunnelState after rollback: %s", protojson.Format(st))
	}
}

func TestTunnelsT1SixrdRoundTrip(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	v := coretest.New()
	v.On("ipip_6rd_add_tunnel", func(msg api.Message) ([]api.Message, error) {
		idx := v.AddInterface("ipip0", "ip6ip-6rd", "")
		return []api.Message{&ipipapi.Ipip6rdAddTunnelReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	v.On("ipip_6rd_del_tunnel", func(msg api.Message) ([]api.Message, error) {
		v.DeleteInterface("ipip0")
		return []api.Message{&ipipapi.Ipip6rdDelTunnelReply{}}, nil
	})
	s := newLispSvc(t, v, t.TempDir(), false)
	js := `{"interfaces": {"loop7001": {"ipv4": ["10.7.1.1/24"]}},
	  "tunnels": {"ipip": {"rd": {"src": "10.7.1.1", "sixrd": {"ip6Prefix": "2001:db8:6::/48", "ip4Prefix": "10.0.0.0/8", "securityCheck": true, "tcTos": 8}}}}}`
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "r1", DesiredState: doc(t, js)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got := tunnelsRetrieve(t, s).GetTunnels().GetIpip()["rd"]
	want := &vrxv1.IpipTunnel{}
	if err := protojson.Unmarshal([]byte(`{"enabled": true, "src": "10.7.1.1", "underlayVrf": "default", "vrf": "default", "mode": "p2p", "sixrd": {"ip6Prefix": "2001:db8:6::/48", "ip4Prefix": "10.0.0.0/8", "securityCheck": true, "tcTos": 8}}`), want); err != nil {
		t.Fatal(err)
	}
	// VPP cannot dump 6RD parameters; the applied tunnel metadata preserves source/underlay.
	if !proto.Equal(got, want) {
		t.Fatalf("Retrieve 6RD:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if i, ok := v.InterfaceByName("ipip0"); !ok || i.Tag != testOwner+":rd" {
		t.Fatalf("ipip0: %+v", i)
	}
	rb := apply(t, s, &vrxv1.ApplyRequest{TxnId: "r2", DesiredState: doc(t, `{"interfaces": {"loop7001": {"ipv4": ["10.7.1.1/24"]}}, "tunnels": {}}`)})
	mustStatus(t, rb, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, ok := v.InterfaceByName("ipip0"); ok {
		t.Fatal("ipip0 left after the rollback")
	}
}
