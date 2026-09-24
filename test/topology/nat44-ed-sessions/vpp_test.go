package nat44edsessions

// Direct VPP access of the test (never of the product API): binary API dumps for the V19 guard and the
// simulated loss, vppctl for the evidence the acceptance asks for (show int, trace).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
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
		out[n] = vppIf{idx: uint32(d.SwIfIndex), name: n, tag: strings.TrimRight(d.Tag, "\x00")}
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

var counterRe = regexp.MustCompile(`(?m)^\s*(?:(\S+)\s+\d+\s+(?:up|down)\s+\S+\s+)?(rx packets|tx packets)\s+(\d+)`)

// showIntCounters parses `vppctl show interface <names>` into name → {"rx packets": n, "tx packets": n}.
func showIntCounters(t *testing.T, names ...string) (map[string]map[string]uint64, string) {
	t.Helper()
	out := vppctl(t, append([]string{"show", "interface"}, names...)...)
	res := map[string]map[string]uint64{}
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && (f[2] == "up" || f[2] == "down") {
			cur = f[0]
			res[cur] = map[string]uint64{}
		}
		if m := counterRe.FindStringSubmatch(line); m != nil && cur != "" {
			n, _ := strconv.ParseUint(m[3], 10, 64)
			res[cur][m[2]] = n
		}
	}
	return res, out
}
