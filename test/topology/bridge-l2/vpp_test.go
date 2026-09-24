package bridgel2

// Direct VPP access of the test (never of the product API): binary API dumps for the evidence and the simulated loss,
// vppctl for what the acceptance asks to see. Only objects of this slot (w<N>) are read or deleted.

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

	afpapi "ngfw/agent/binapi/af_packet"
	"ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	l3xcapi "ngfw/agent/binapi/l3xc"
	mactimeapi "ngfw/agent/binapi/mactime"
	"ngfw/agent/binapi/memclnt"
)

const apiSocket = "/run/vpp/api.sock"

const noIndex = ^uint32(0)

// vppConn is the test's binary-API connection.
type vppConn = vppapi.Connection

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
	vtr  uint32
}

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func dumpIfs(t *testing.T, conn vppapi.Connection) map[string]vppIf {
	t.Helper()
	ctx, cancel := ctx10()
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
		out[n] = vppIf{idx: uint32(d.SwIfIndex), name: n, tag: strings.TrimRight(d.Tag, "\x00"), vtr: d.VtrOp}
	}
	return out
}

// deleteBehindBack removes the rig's VPP host-interfaces (addresses first) so the AGENT creates them from the
// configuration. The veths are down (D-101: never delete an af_packet interface with its veth up).
func deleteBehindBack(t *testing.T, conn vppapi.Connection, netdevs ...string) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	ifs := dumpIfs(t, conn)
	var ev []string
	for _, nd := range netdevs {
		i, ok := ifs["host-"+nd]
		if !ok {
			continue
		}
		if _, err := interfaces.NewServiceClient(conn).SwInterfaceAddDelAddress(ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(i.idx), DelAll: true}); err != nil {
			t.Fatalf("sw_interface_add_del_address del_all %s: %v", i.name, err)
		}
		if _, err := afpapi.NewServiceClient(conn).AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: nd}); err != nil {
			t.Fatalf("af_packet_delete %s: %v", nd, err)
		}
		ev = append(ev, fmt.Sprintf("af_packet_delete host_if_name=%s (sw_if_index %d, tag %q) → ok", nd, i.idx, i.tag))
	}
	return ev
}

// ---- L2 state of this slot ----------------------------------------------------------------------

type bdState struct {
	id      uint32
	tag     string
	macAge  uint8
	bvi     uint32
	members map[uint32]uint8 // sw_if_index → shg
}

// ourBDs dumps the bridge domains whose tag carries the slot owner ("<prefix>:…").
func ourBDs(t *testing.T, conn vppapi.Connection, prefix string) map[uint32]bdState {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	stream, err := l2api.NewServiceClient(conn).BridgeDomainDump(ctx, &l2api.BridgeDomainDump{BdID: noIndex, SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint32]bdState{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		tag := strings.TrimRight(d.BdTag, "\x00")
		if !strings.HasPrefix(tag, prefix+":") {
			continue
		}
		b := bdState{id: d.BdID, tag: tag, macAge: d.MacAge, bvi: uint32(d.BviSwIfIndex), members: map[uint32]uint8{}}
		for _, sw := range d.SwIfDetails {
			b.members[uint32(sw.SwIfIndex)] = sw.Shg
		}
		out[d.BdID] = b
	}
	return out
}

// xconnects returns rx → tx sw_if_index of every L2 cross-connect.
func xconnects(t *testing.T, conn vppapi.Connection) map[uint32]uint32 {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	stream, err := l2api.NewServiceClient(conn).L2XconnectDump(ctx, &l2api.L2XconnectDump{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint32]uint32{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out[uint32(d.RxSwIfIndex)] = uint32(d.TxSwIfIndex)
	}
}

// l3xcs returns the rx sw_if_index (and family) of every L3 cross-connect.
func l3xcs(t *testing.T, conn vppapi.Connection) map[uint32]bool {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	stream, err := l3xcapi.NewServiceClient(conn).L3xcDump(ctx, &l3xcapi.L3xcDump{SwIfIndex: interface_types.InterfaceIndex(noIndex)})
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint32]bool{}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out[uint32(d.L3xc.SwIfIndex)] = d.L3xc.IsIP6
	}
}

// ourDevices returns this slot's mactime devices (name "<prefix>:…") by name. mactime_dump ends with its own
// mactime_dump_reply before the control-ping reply, so the stream is read here.
func ourDevices(t *testing.T, conn vppapi.Connection, prefix string) map[string]*mactimeapi.MactimeDetails {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	stream, err := conn.NewStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stream.Close() }()
	if err := stream.SendMsg(&mactimeapi.MactimeDump{}); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&memclnt.ControlPing{}); err != nil {
		t.Fatal(err)
	}
	out := map[string]*mactimeapi.MactimeDetails{}
	for {
		msg, err := stream.RecvMsg()
		if err != nil {
			t.Fatal(err)
		}
		switch m := msg.(type) {
		case *mactimeapi.MactimeDetails:
			if n := strings.TrimRight(m.DeviceName, "\x00"); strings.HasPrefix(n, prefix+":") {
				out[n] = m
			}
		case *mactimeapi.MactimeDumpReply:
		case *memclnt.ControlPingReply:
			return out
		}
	}
}

func mactimeOn(t *testing.T, conn vppapi.Connection, idx uint32) bool {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	r, err := feature.NewServiceClient(conn).FeatureIsEnabled(ctx, &feature.FeatureIsEnabled{ArcName: "device-input", FeatureName: "mactime", SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatal(err)
	}
	return r.IsEnabled
}

// l2Loss deletes this slot's L2 objects behind the agent's back, dependents first (D-095c): the MAC filter enable and
// devices, tag rewrites, memberships, cross-connects, L3 cross-connects, static MACs, then the bridge domains. The
// interfaces themselves stay (their veths are down; nothing crosses them).
func l2Loss(t *testing.T, conn vppapi.Connection, prefix string, mactimeIfs, vtrIfs []string) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	ifs := dumpIfs(t, conn)
	l2c := l2api.NewServiceClient(conn)
	var ev []string
	for _, n := range mactimeIfs {
		if _, err := mactimeapi.NewServiceClient(conn).MactimeEnableDisable(ctx, &mactimeapi.MactimeEnableDisable{EnableDisable: false, SwIfIndex: interface_types.InterfaceIndex(ifs[n].idx)}); err != nil {
			t.Fatalf("mactime_enable_disable off %s: %v", n, err)
		}
		ev = append(ev, "mactime_enable_disable enable_disable=false "+n+" → ok")
	}
	for name, d := range ourDevices(t, conn, prefix) {
		if _, err := mactimeapi.NewServiceClient(conn).MactimeAddDelRange(ctx, &mactimeapi.MactimeAddDelRange{IsAdd: false, MacAddress: d.MacAddress}); err != nil {
			t.Fatalf("mactime_add_del_range del %s: %v", name, err)
		}
		ev = append(ev, "mactime_add_del_range is_add=false "+name+" → ok")
	}
	for _, n := range vtrIfs {
		if _, err := l2c.L2InterfaceVlanTagRewrite(ctx, &l2api.L2InterfaceVlanTagRewrite{SwIfIndex: interface_types.InterfaceIndex(ifs[n].idx), VtrOp: 0}); err != nil {
			t.Fatalf("l2_interface_vlan_tag_rewrite disable %s: %v", n, err)
		}
		ev = append(ev, "l2_interface_vlan_tag_rewrite vtr_op=0 "+n+" → ok")
	}
	byIdx := map[uint32]string{}
	for n, i := range ifs {
		byIdx[i.idx] = n
	}
	bds := ourBDs(t, conn, prefix)
	for id, b := range bds {
		for sw := range b.members {
			if _, err := l2c.SwInterfaceSetL2Bridge(ctx, &l2api.SwInterfaceSetL2Bridge{RxSwIfIndex: interface_types.InterfaceIndex(sw), BdID: id, Enable: false}); err != nil {
				t.Fatalf("sw_interface_set_l2_bridge off %d: %v", sw, err)
			}
			ev = append(ev, fmt.Sprintf("sw_interface_set_l2_bridge enable=false %s (bd %d) → ok", byIdx[sw], id))
		}
	}
	for rx, tx := range xconnects(t, conn) {
		if n := byIdx[rx]; !strings.HasPrefix(n, "host-"+prefix) {
			continue
		}
		if _, err := l2c.SwInterfaceSetL2Xconnect(ctx, &l2api.SwInterfaceSetL2Xconnect{RxSwIfIndex: interface_types.InterfaceIndex(rx), TxSwIfIndex: interface_types.InterfaceIndex(tx), Enable: false}); err != nil {
			t.Fatalf("sw_interface_set_l2_xconnect off %d: %v", rx, err)
		}
		ev = append(ev, "sw_interface_set_l2_xconnect enable=false "+byIdx[rx]+" → ok")
	}
	for rx, v6 := range l3xcs(t, conn) {
		if n := byIdx[rx]; !strings.HasPrefix(ifs[n].tag, prefix+":") {
			continue
		}
		if _, err := l3xcapi.NewServiceClient(conn).L3xcDel(ctx, &l3xcapi.L3xcDel{SwIfIndex: interface_types.InterfaceIndex(rx), IsIP6: v6}); err != nil {
			t.Fatalf("l3xc_del %d: %v", rx, err)
		}
		ev = append(ev, "l3xc_del "+byIdx[rx]+" → ok")
	}
	for id := range bds {
		// static entries go with the bridge domain (l2fib flush on delete); delete explicitly first anyway
		if _, err := l2c.BridgeDomainAddDelV2(ctx, &l2api.BridgeDomainAddDelV2{BdID: id, IsAdd: false}); err != nil {
			t.Fatalf("bridge_domain_add_del_v2 del %d: %v", id, err)
		}
		ev = append(ev, fmt.Sprintf("bridge_domain_add_del_v2 is_add=false bd_id=%d → ok", id))
	}
	return ev
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
