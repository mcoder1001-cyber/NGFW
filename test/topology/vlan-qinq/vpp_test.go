package vlanqinq

// Direct VPP access of the test (never of the product API): binary API dumps for the V19 guard, the tag stacks and the
// simulated loss, vppctl for the evidence the acceptance asks for (show int with its counters, show int address; no
// packet trace on the shared VPP, D-128). Copied from P08's test/topology/interfaces and extended with sub-interface rows
// and the sub-interface loss.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"

	"ngfw/agent/binapi/acl"
	afpapi "ngfw/agent/binapi/af_packet"
	"ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ipsec"
)

const apiSocket = "/run/vpp/api.sock"

const noIndex = ^uint32(0)

func connectVPP(t *testing.T) vppapi.Connection {
	t.Helper()
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatalf("govpp connect: %v", err)
	}
	t.Cleanup(conn.Disconnect)
	return conn
}

type vppIf struct {
	idx  uint32
	name string
	tag  string
	// sub-interfaces (sw_interface_dump): parent index, sub id, number of tags, outer/inner VLAN, sub_if_flags
	sub      bool
	sup      uint32
	subID    uint32
	nTags    uint8
	outer    uint16
	inner    uint16
	subFlags interface_types.SubIfFlags
}

// stack renders a sub-interface row the way VPP's API reports it.
func (i vppIf) stack() string {
	return fmt.Sprintf("sw_if_index=%d sup=%d sub_id=%d sub_number_of_tags=%d sub_outer_vlan_id=%d sub_inner_vlan_id=%d sub_if_flags=%s tag=%q",
		i.idx, i.sup, i.subID, i.nTags, i.outer, i.inner, subFlagNames(i.subFlags), i.tag)
}

// subFlagNames spells sub_if_flags with the binapi's own enum names.
func subFlagNames(f interface_types.SubIfFlags) string {
	var out []string
	for _, b := range []interface_types.SubIfFlags{
		interface_types.SUB_IF_API_FLAG_NO_TAGS, interface_types.SUB_IF_API_FLAG_ONE_TAG, interface_types.SUB_IF_API_FLAG_TWO_TAGS,
		interface_types.SUB_IF_API_FLAG_DOT1AD, interface_types.SUB_IF_API_FLAG_EXACT_MATCH, interface_types.SUB_IF_API_FLAG_DEFAULT,
		interface_types.SUB_IF_API_FLAG_OUTER_VLAN_ID_ANY, interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY, interface_types.SUB_IF_API_FLAG_DOT1AH,
	} {
		if f&b != 0 {
			out = append(out, strings.TrimPrefix(b.String(), "SUB_IF_API_FLAG_"))
		}
	}
	return strings.Join(out, "|")
}

func dumpIfs(t *testing.T, conn vppapi.Connection) map[string]vppIf {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := interfaces.NewServiceClient(conn).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]vppIf{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n := strings.TrimRight(d.InterfaceName, "\x00")
		out[n] = vppIf{
			idx: uint32(d.SwIfIndex), name: n, tag: strings.TrimRight(d.Tag, "\x00"),
			sub: d.Type == interface_types.IF_API_TYPE_SUB, sup: uint32(d.SupSwIfIndex), subID: d.SubID, nTags: d.SubNumberOfTags,
			outer: d.SubOuterVlanID, inner: d.SubInnerVlanID, subFlags: d.SubIfFlags,
		}
	}
	return out
}

// v19Guard (D-095, shared-host rule for this task): before any packet crosses the rig, prove that no
// classify / ACL / SPD binding sits on the rig interfaces' sw_if_index. Dumpable bindings are asserted
// absent; the write-only ip/l2 classify bindings (no dump in VPP 26.06, D-063) are reset to "none" on our
// own freshly created interfaces — exactly what DF-2's delete path sends — so a binding inherited from a
// deleted interface with the same index cannot survive (the V19 crash vector).
func v19Guard(t *testing.T, conn vppapi.Connection, ifs map[string]uint32) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var evidence []string
	cls := classify.NewServiceClient(conn)
	for name, idx := range ifs {
		r, err := cls.ClassifyTableByInterface(ctx, &classify.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			t.Fatalf("V19: classify_table_by_interface %s: %v", name, err)
		}
		if r.L2TableID != noIndex || r.IP4TableID != noIndex || r.IP6TableID != noIndex {
			t.Fatalf("V19: %s (sw_if_index %d) carries an input classify binding l2=%d ip4=%d ip6=%d — refusing to send packets", name, idx, r.L2TableID, r.IP4TableID, r.IP6TableID)
		}
		evidence = append(evidence, fmt.Sprintf("classify_table_by_interface %s sw_if_index=%d l2=~0 ip4=~0 ip6=~0", name, idx))

		st, err := acl.NewServiceClient(conn).ACLInterfaceListDump(ctx, &acl.ACLInterfaceListDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err != nil {
			t.Fatalf("V19: acl_interface_list_dump %s: %v", name, err)
		}
		n := 0
		for {
			d, err := st.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("V19: acl_interface_list_dump %s: %v", name, err)
			}
			if uint32(d.SwIfIndex) == idx {
				n += len(d.Acls)
			}
		}
		if n != 0 {
			t.Fatalf("V19: %s (sw_if_index %d) has %d ACL bindings — refusing to send packets", name, idx, n)
		}
		evidence = append(evidence, fmt.Sprintf("acl_interface_list_dump %s: 0 ACLs", name))
	}
	// SPD bindings (all SPDs in one dump)
	sp, err := ipsec.NewServiceClient(conn).IpsecSpdInterfaceDump(ctx, &ipsec.IpsecSpdInterfaceDump{})
	if err != nil {
		evidence = append(evidence, "ipsec_spd_interface_dump: "+err.Error()+" (ipsec not loaded)")
	} else {
		for {
			d, err := sp.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("V19: ipsec_spd_interface_dump: %v", err)
			}
			for name, idx := range ifs {
				if uint32(d.SwIfIndex) == idx {
					t.Fatalf("V19: %s (sw_if_index %d) is bound to SPD %d — refusing to send packets", name, idx, d.SpdIndex)
				}
			}
		}
		evidence = append(evidence, "ipsec_spd_interface_dump: no SPD on the rig interfaces")
	}
	// write-only bindings: reset to none on our own interfaces (no dump exists)
	for name, idx := range ifs {
		for _, v6 := range []bool{false, true} {
			if _, err := cls.ClassifySetInterfaceIPTable(ctx, &classify.ClassifySetInterfaceIPTable{IsIPv6: v6, SwIfIndex: interface_types.InterfaceIndex(idx), TableIndex: noIndex}); err != nil {
				t.Fatalf("V19: classify_set_interface_ip_table %s ~0: %v", name, err)
			}
		}
		for _, in := range []bool{true, false} {
			if _, err := cls.ClassifySetInterfaceL2Tables(ctx, &classify.ClassifySetInterfaceL2Tables{SwIfIndex: interface_types.InterfaceIndex(idx), IP4TableIndex: noIndex, IP6TableIndex: noIndex, OtherTableIndex: noIndex, IsInput: in}); err != nil {
				t.Fatalf("V19: classify_set_interface_l2_tables %s ~0: %v", name, err)
			}
		}
		evidence = append(evidence, fmt.Sprintf("reset write-only ip4/ip6 classify table and l2 in/out tables of %s to ~0", name))
	}
	return evidence
}

// deleteBehindBack simulates the loss: addresses first (dependents first, D-095 c), then the af_packet
// interfaces themselves — through the binary API, never through the agent.
func deleteBehindBack(t *testing.T, conn vppapi.Connection, netdevs ...string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ifs := dumpIfs(t, conn)
	var ev []string
	for _, nd := range netdevs {
		i, ok := ifs["host-"+nd]
		if !ok {
			t.Fatalf("loss: host-%s is not in VPP", nd)
		}
		if _, err := interfaces.NewServiceClient(conn).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(i.idx), DelAll: true}); err != nil {
			t.Fatalf("loss: sw_interface_add_del_address del_all %s: %v", i.name, err)
		}
		ev = append(ev, fmt.Sprintf("sw_interface_add_del_address sw_if_index=%d (%s) del_all=true → ok", i.idx, i.name))
		if _, err := afpapi.NewServiceClient(conn).AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: nd}); err != nil {
			t.Fatalf("loss: af_packet_delete %s: %v", nd, err)
		}
		ev = append(ev, fmt.Sprintf("af_packet_delete host_if_name=%s (tag %q) → ok", nd, i.tag))
	}
	return ev
}

// deleteSubsBehindBack simulates the loss of sub-interfaces: their addresses first (dependents first, D-095 c), then the
// sub-interfaces — through the binary API, never through the agent. No af_packet interface is deleted (the parent stays),
// so no veth has to be quiesced for this loss (D-101 concerns af_packet deletes only).
func deleteSubsBehindBack(t *testing.T, conn vppapi.Connection, names ...string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ifs := dumpIfs(t, conn)
	svc := interfaces.NewServiceClient(conn)
	var ev []string
	for _, n := range names {
		i, ok := ifs[n]
		if !ok || !i.sub {
			t.Fatalf("loss: %s is not a sub-interface in VPP", n)
		}
		if _, err := svc.SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(i.idx), DelAll: true}); err != nil {
			t.Fatalf("loss: sw_interface_add_del_address del_all %s: %v", n, err)
		}
		ev = append(ev, fmt.Sprintf("sw_interface_add_del_address sw_if_index=%d (%s) del_all=true → ok", i.idx, n))
	}
	for _, n := range names {
		i := ifs[n]
		if _, err := svc.DeleteSubif(ctx, &interfaces.DeleteSubif{SwIfIndex: interface_types.InterfaceIndex(i.idx)}); err != nil {
			t.Fatalf("loss: delete_subif %s: %v", n, err)
		}
		ev = append(ev, fmt.Sprintf("delete_subif sw_if_index=%d (%s, tag %q) → ok", i.idx, n, i.tag))
	}
	return ev
}

// ipv4Of returns the IPv4 prefixes VPP has on an interface (ip_address_dump).
func ipv4Of(t *testing.T, conn vppapi.Connection, idx uint32) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := ip.NewServiceClient(conn).IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatalf("ip_address_dump %d: %v", idx, err)
	}
	var out []string
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("ip_address_dump %d: %v", idx, err)
		}
		out = append(out, d.Prefix.String())
	}
}

// ---- vppctl (evidence only; fixed commands, no user input) ----------------------------------------

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vppctl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("vppctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
