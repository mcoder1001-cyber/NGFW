package agent

// F-lb end to end through the Service over the coretest model extended with the lb plugin (lbModel): projection of
// services.lb, write-only apply and removal ("removed" entries, V20), LbState, LbFlushVip, agent restart (re-applied
// without duplicates, intf-nat applied once per VPP instance) and — a slot agent — never a garbage collection.

import (
	"context"
	"math/bits"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/grpc/codes"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lb"
	"ngfw/agent/binapi/lb_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/vpp/bootid"
)

// lbModel is VPP 26.06's lb plugin as the agent sees it: VIPs keyed by (prefix, protocol, port) with the encap enums
// read without ntohl, deleted VIPs and ASes kept as "removed" until a garbage collection, dumps that report the VIP
// type as encap, protocol 0 and the target port byte-swapped.
type lbModel struct {
	mu      sync.Mutex
	vips    []*lbModelVIP
	nat     map[uint32]int // sw_if_index → in2out feature instances (stacks on every enable)
	flushed []*lb.LbFlushVip
	cli     []string
}

type lbModelVIP struct {
	pfx    ip_types.AddressWithPrefix
	proto  uint8
	port   uint16
	vt     lb_types.LbVipType
	target uint16
	dscp   uint8
	used   bool
	ases   []*lbModelAS
}

type lbModelAS struct {
	addr ip_types.Address
	used bool
}

// lbIdentity gives the fake VPP a complete D-080 boot identity (a main PID with a start time in a fake /proc): the
// lb ownership records are written only for a known VPP instance.
func lbIdentity(t *testing.T, v *coretest.VPP) {
	t.Helper()
	root := t.TempDir()
	if err := bootid.WriteFakeProc(root, "boot-lb", map[int]uint64{4731: 1000}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bootid.SetProcRoot(root))
	v.Reply("control_ping", &memclnt.ControlPingReply{VpePID: 4731})
}

func installLbModel(t *testing.T, v *coretest.VPP) *lbModel {
	lbIdentity(t, v)
	m := &lbModel{nat: map[uint32]int{}}
	find := func(p ip_types.AddressWithPrefix, proto uint8, port uint16) *lbModelVIP {
		if port == 0 {
			proto = 255
		}
		for _, x := range m.vips {
			if x.used && x.pfx == p && x.proto == proto && x.port == port {
				return x
			}
		}
		return nil
	}
	v.On("lb_add_del_vip_v2", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*lb.LbAddDelVipV2)
		m.mu.Lock()
		defer m.mu.Unlock()
		rv := func(e api.VPPApiError) []api.Message { return []api.Message{&lb.LbAddDelVipV2Reply{Retval: int32(e)}} }
		x := find(r.Pfx, r.Protocol, r.Port)
		if r.IsDel {
			if x == nil {
				return rv(api.NO_SUCH_ENTRY), nil
			}
			x.used = false
			for _, a := range x.ases {
				a.used = false
			}
			return rv(0), nil
		}
		if x != nil {
			return rv(api.VALUE_EXIST), nil
		}
		v4 := r.Pfx.Address.Af == ip_types.ADDRESS_IP4
		var vt map[uint32]lb_types.LbVipType
		if v4 {
			vt = map[uint32]lb_types.LbVipType{0: lb_types.LB_API_VIP_TYPE_IP4_GRE4, 1: lb_types.LB_API_VIP_TYPE_IP4_GRE6, 2: lb_types.LB_API_VIP_TYPE_IP4_L3DSR, 3: lb_types.LB_API_VIP_TYPE_IP4_NAT4}
		} else {
			vt = map[uint32]lb_types.LbVipType{0: lb_types.LB_API_VIP_TYPE_IP6_GRE4, 1: lb_types.LB_API_VIP_TYPE_IP6_GRE6, 4: lb_types.LB_API_VIP_TYPE_IP6_NAT6}
		}
		t, ok := vt[bits.ReverseBytes32(uint32(r.Encap))] // VPP 26.06: no ntohl
		if !ok {
			return rv(api.INVALID_ADDRESS_FAMILY), nil
		}
		proto := r.Protocol
		if r.Port == 0 {
			proto = 255
		}
		m.vips = append(m.vips, &lbModelVIP{pfx: r.Pfx, proto: proto, port: r.Port, vt: t, target: bits.ReverseBytes16(r.TargetPort), dscp: r.Dscp, used: true})
		return rv(0), nil
	})
	v.On("lb_add_del_as", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*lb.LbAddDelAs)
		m.mu.Lock()
		defer m.mu.Unlock()
		rv := func(e api.VPPApiError) []api.Message { return []api.Message{&lb.LbAddDelAsReply{Retval: int32(e)}} }
		x := find(r.Pfx, r.Protocol, r.Port)
		if x == nil {
			return rv(api.NO_SUCH_ENTRY), nil
		}
		for _, a := range x.ases {
			if a.addr == r.AsAddress {
				switch {
				case r.IsDel && a.used:
					a.used = false
					return rv(0), nil
				case r.IsDel:
					return rv(api.NO_SUCH_ENTRY), nil
				case a.used:
					return rv(api.VALUE_EXIST), nil
				}
				a.used = true
				return rv(0), nil
			}
		}
		if r.IsDel {
			return rv(api.NO_SUCH_ENTRY), nil
		}
		x.ases = append(x.ases, &lbModelAS{addr: r.AsAddress, used: true})
		return rv(0), nil
	})
	v.On("lb_vip_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, x := range m.vips {
			out = append(out, &lb.LbVipDetails{Vip: lb_types.LbVip{Pfx: x.pfx, Port: x.port}, Encap: lb_types.LbEncapType(x.vt), Dscp: ip_types.IPDscp(x.dscp), TargetPort: x.target})
		}
		return out, nil
	})
	v.On("lb_as_dump", func(api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var out []api.Message
		for _, x := range m.vips {
			for _, a := range x.ases {
				var f uint8
				if a.used {
					f = 1
				}
				out = append(out, &lb.LbAsDetails{Vip: lb_types.LbVip{Pfx: x.pfx, Port: x.port}, AppSrv: a.addr, Flags: f, InUseSince: 42})
			}
		}
		return out, nil
	})
	v.On("lb_flush_vip", func(msg api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.flushed = append(m.flushed, msg.(*lb.LbFlushVip))
		return []api.Message{&lb.LbFlushVipReply{}}, nil
	})
	v.On("lb_add_del_intf_nat4", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*lb.LbAddDelIntfNat4)
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.IsAdd {
			m.nat[uint32(r.SwIfIndex)]++
		} else if m.nat[uint32(r.SwIfIndex)] > 0 {
			m.nat[uint32(r.SwIfIndex)]--
		}
		return []api.Message{&lb.LbAddDelIntfNat4Reply{}}, nil
	})
	v.On("cli_inband", func(msg api.Message) ([]api.Message, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cli = append(m.cli, msg.(*vlib.CliInband).Cmd)
		return []api.Message{&vlib.CliInbandReply{Retval: -1, Reply: "lb_vip_find_index error -6"}}, nil
	})
	return m
}

func (m *lbModel) counts() (vips, used, ases, usedAses int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.vips {
		vips++
		if x.used {
			used++
		}
		for _, a := range x.ases {
			ases++
			if a.used {
				usedAses++
			}
		}
	}
	return
}

func (m *lbModel) natTotal() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.nat {
		n += c
	}
	return n
}

const lbDoc = `{
  "interfaces": {"loop731": {"ipv4": ["10.7.31.1/24"]}},
  "services": {"lb": {
    "settings": {"ip4Source": "10.7.31.1"},
    "vips": {
      "web": {"prefix": "10.2.250.1/32", "protocol": "tcp", "port": 80, "encap": "gre4", "newFlowsTableLength": 1024, "srcIpSticky": false,
              "servers": [{"address": "10.2.2.10", "flushOnDelete": false}, {"address": "10.2.2.11", "flushOnDelete": true}]},
      "dsr": {"prefix": "10.2.250.2/32", "protocol": "any", "encap": "l3dsr", "dscp": 10, "newFlowsTableLength": 256, "srcIpSticky": true,
              "servers": [{"address": "10.2.2.12", "flushOnDelete": false}]},
      "dns": {"prefix": "10.2.250.3/32", "protocol": "udp", "port": 53, "encap": "nat4", "srvType": "clusterip", "targetPort": 5353,
              "newFlowsTableLength": 1024, "srcIpSticky": false, "servers": [{"address": "10.2.2.13", "flushOnDelete": false}]}
    },
    "natInterfaces": [{"interface": "loop731", "family": "ip4"}]
  }}
}`

func TestLbApplyStateFlushRemove(t *testing.T) {
	v := coretest.New()
	m := installLbModel(t, v)
	s := newSvc(t, v, t.TempDir()) // a slot agent: VRX_GLOBALS_OWNER=0
	ctx := context.Background()

	rep, err := s.DryRun(ctx, &vrxv1.DryRunRequest{DesiredState: doc(t, lbDoc)})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run %v %v", err, rep)
	}
	notes := map[string]string{}
	for _, e := range rep.GetErrors() {
		notes[e.GetPointer()] = e.GetRule()
	}
	if notes["/services/lb"] != "agent.write-only" || notes["/services/lb/settings"] != "agent.unsupported-field" {
		t.Fatalf("notes %v", notes)
	}

	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "lb1", DesiredState: doc(t, lbDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if vips, used, ases, usedAses := m.counts(); vips != 3 || used != 3 || ases != 4 || usedAses != 4 || m.natTotal() != 1 {
		t.Fatalf("model after apply: vips %d/%d ases %d/%d nat %d", used, vips, usedAses, ases, m.natTotal())
	}
	if n := len(v.CallsNamed("lb_conf")); n != 0 {
		t.Fatalf("a slot agent sent lb_conf %d times (D-071)", n)
	}
	// write-only: Retrieve never reports services.lb
	got, err := s.Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"services"}})
	if err != nil || got.GetDesiredState().GetServices().GetLb() != nil {
		t.Fatalf("retrieve %v %v", err, got)
	}

	st, err := s.LbState(ctx, &vrxv1.LbStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.GetVips()) != 3 || st.GetTotalVppVips() != 3 {
		t.Fatalf("state %v", st)
	}
	dns, dsr, web := st.GetVips()[0], st.GetVips()[1], st.GetVips()[2]
	if dns.GetName() != "dns" || dns.GetEncap() != "nat4" || dns.GetTargetPort() != 5353 || dns.GetProtocol() != "udp" || dns.GetPort() != 53 {
		t.Fatalf("dns %v", dns)
	}
	if web.GetName() != "web" || !web.GetApplied() || web.GetVppEntries() != 1 || web.GetEncap() != "gre4" || len(web.GetServers()) != 2 || !web.GetServers()[0].GetInUse() {
		t.Fatalf("web %v", web)
	}
	if dsr.GetName() != "dsr" || dsr.GetEncap() != "l3dsr" || dsr.GetDscp() != 10 || dsr.GetPort() != 0 {
		t.Fatalf("dsr %v", dsr)
	}
	if st, err := s.LbState(ctx, &vrxv1.LbStateRequest{Names: []string{"dsr"}}); err != nil || len(st.GetVips()) != 1 {
		t.Fatalf("filter %v %v", err, st)
	}

	// flush: the ip46 layout for an IPv4 VIP, protocol and port of the VIP
	fr, err := s.LbFlushVip(ctx, &vrxv1.LbFlushVipRequest{Name: "web"})
	if err != nil || fr.GetVip() != "lb.vip/10.2.250.1/32/tcp/80" || len(m.flushed) != 1 {
		t.Fatalf("flush %v %v", err, fr)
	}
	if f := m.flushed[0]; [16]byte(f.Pfx.Address.Un.GetIP6()) != [16]byte{12: 10, 13: 2, 14: 250, 15: 1} || f.Pfx.Len != 128 || f.Protocol != 6 || f.Port != 80 {
		t.Fatalf("flush request %+v", f)
	}
	for name, want := range map[string]codes.Code{"nope": codes.NotFound, "": codes.InvalidArgument} {
		if _, err := s.LbFlushVip(ctx, &vrxv1.LbFlushVipRequest{Name: name}); grpcCode(err) != want {
			t.Fatalf("flush %q: %v", name, err)
		}
	}
	if _, err := s.LbState(ctx, &vrxv1.LbStateRequest{Owner: "other"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatal(err)
	}

	// a VIP change is delete + add: VPP keeps the old entry as "removed" (V20)
	changed := strings.Replace(lbDoc, `"port": 80, "encap": "gre4", "newFlowsTableLength": 1024`, `"port": 80, "encap": "gre4", "newFlowsTableLength": 2048`, 1)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "lb2", DesiredState: doc(t, changed)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	st, _ = s.LbState(ctx, &vrxv1.LbStateRequest{Names: []string{"web"}})
	if w := st.GetVips()[0]; w.GetVppEntries() != 2 || len(w.GetServers()) != 4 || !w.GetServers()[0].GetInUse() || w.GetServers()[3].GetInUse() {
		t.Fatalf("changed web %v", w)
	}

	// removal: delete messages sent, VPP lists the VIPs as removed, the NAT feature is disabled
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "lb3", DesiredState: doc(t, `{"interfaces": {"loop731": {"ipv4": ["10.7.31.1/24"]}}, "services": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if vips, used, _, usedAses := m.counts(); vips != 4 || used != 0 || usedAses != 0 || m.natTotal() != 0 {
		t.Fatalf("after removal: %d vips, %d used, %d ASes in use, nat %d", vips, used, usedAses, m.natTotal())
	}
	if st, err := s.LbState(ctx, &vrxv1.LbStateRequest{}); err != nil || len(st.GetVips()) != 0 || st.GetTotalVppVips() != 4 {
		t.Fatalf("state after removal %v %v", err, st)
	}
	if len(m.cli) != 0 {
		t.Fatalf("a slot agent ran the lb garbage collection: %v", m.cli)
	}
}

// Agent-restart simulation: the VIPs, ASes and the NAT feature are re-applied from the stored desired state without
// duplicates (ownership records in the persisted DF-7 BootStore); a VIP lost behind the agent's back is re-created.
func TestLbRestartReappliesWithoutDuplicates(t *testing.T) {
	v := coretest.New()
	m := installLbModel(t, v)
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "lb1", DesiredState: doc(t, lbDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	s.Close()

	s2 := newSvc(t, v, dir)
	mustStatus(t, s2.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if vips, used, ases, _ := m.counts(); vips != 3 || used != 3 || ases != 4 || m.natTotal() != 1 {
		t.Fatalf("resync duplicated: vips %d/%d ases %d nat %d", used, vips, ases, m.natTotal())
	}
	st, err := s2.LbState(context.Background(), &vrxv1.LbStateRequest{})
	if err != nil || len(st.GetVips()) != 3 || !st.GetVips()[0].GetApplied() || !st.GetVips()[2].GetApplied() {
		t.Fatalf("records lost across the restart: %v %v", err, st)
	}
	s2.Close()

	// simulated loss of one VIP (and its ASes) while the agent is down
	m.mu.Lock()
	for _, x := range m.vips {
		if netip.AddrFrom4(x.pfx.Address.Un.GetIP4()).String() == "10.2.250.1" {
			x.used = false
			for _, a := range x.ases {
				a.used = false
			}
		}
	}
	m.mu.Unlock()
	s3 := newSvc(t, v, dir)
	mustStatus(t, s3.Resync(context.Background()), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if _, used, _, usedAses := m.counts(); used != 3 || usedAses != 4 {
		t.Fatalf("lost VIP not re-created: %d VIPs, %d ASes in use", used, usedAses)
	}
}

// The projection refuses the garbage-collection sentinel range (0.0.0.0/8) whatever the API let through.
func TestLbProjectionRefusesSentinel(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	bad := strings.Replace(lbDoc, "10.2.250.1/32", "0.0.0.0/32", 1)
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: doc(t, bad)})
	if err != nil || rep.GetOk() {
		t.Fatalf("%v %v", err, rep)
	}
	found := false
	for _, e := range rep.GetErrors() {
		found = found || (e.GetPointer() == "/services/lb/vips/web/prefix" && e.GetRule() == "services.lb-vip-reserved")
	}
	if !found {
		t.Fatalf("errors %v", rep.GetErrors())
	}
}
