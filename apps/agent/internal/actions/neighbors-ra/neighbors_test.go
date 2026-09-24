package neighborsra

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ethernet_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/ip_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/df2"
)

const owner = "w9"

// rig: two interfaces of ours (loop901 in table 9001, host-w9l0 in table 0), an untagged NIC (nameable), and another
// slot's interface (never listed, never flushed, never subscribed).
func rig(t *testing.T) (*coretest.VPP, *coretest.NeighborsRa) {
	t.Helper()
	v := coretest.New()
	ctx := context.Background()
	lo := v.AddInterface("loop901", "Loopback", "w9:loop901")
	host := v.AddInterface("host-w9l0", "af_packet", "w9:host-w9l0")
	v.AddInterface("GigabitEthernet0/8/0", "dpdk", "")
	v.AddInterface("loop301", "Loopback", "w3:loop301")
	svc := interfaces.NewServiceClient(v)
	for _, v6 := range []bool{false, true} {
		if _, err := ip.NewServiceClient(v).IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: ip.IPTable{TableID: 9001, IsIP6: v6, Name: "w9:red"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SwInterfaceSetTable(ctx, &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(lo), IsIPv6: v6, VrfID: 9001}); err != nil {
			t.Fatal(err)
		}
	}
	_ = host
	m := coretest.NeighborsRaOf(v)
	for _, n := range []struct{ ifn, ip, mac string }{
		{"loop901", "10.9.1.20", "02:00:00:00:09:20"},
		{"loop901", "10.9.1.3", "02:00:00:00:09:03"},
		{"loop901", "2001:db8:9:1::5", "02:00:00:00:09:05"},
		{"host-w9l0", "10.9.2.7", "02:00:00:00:92:07"},
		{"GigabitEthernet0/8/0", "192.0.2.9", "02:00:00:00:00:99"},
		{"loop301", "10.3.1.1", "02:00:00:00:03:01"},
	} {
		if !m.Learn(v, n.ifn, n.ip, n.mac, 12.5) {
			t.Fatalf("learn %v", n)
		}
	}
	addStatic(t, v, "loop901", "10.9.1.50", "02:00:00:00:09:50")
	return v, m
}

func addStatic(t *testing.T, v *coretest.VPP, ifName, addr, mac string) {
	t.Helper()
	i, ok := v.InterfaceByName(ifName)
	if !ok {
		t.Fatal(ifName)
	}
	a, _ := df2.ParseAddr(addr)
	hw, _ := ethernet_types.ParseMacAddress(mac)
	_, err := ip_neighbor.NewServiceClient(v).IPNeighborAddDel(context.Background(), &ip_neighbor.IPNeighborAddDel{IsAdd: true, Neighbor: ip_neighbor.IPNeighbor{
		SwIfIndex: interface_types.InterfaceIndex(i.Index), Flags: ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC, MacAddress: hw, IPAddress: df2.ToAddress(a),
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func vrfName(id uint32) string {
	switch id {
	case 0:
		return "default"
	case 9001:
		return "red"
	}
	return fmt.Sprint(id)
}

func rows(p Page) []string {
	var out []string
	for _, e := range p.Entries {
		out = append(out, fmt.Sprintf("%s %s %s %s %s %s", e.Interface, e.IP, e.MAC, e.Family, e.State(), e.VRF))
	}
	return out
}

func TestListFiltersSortsPagesAndNeverDumpsEverything(t *testing.T) {
	v, m := rig(t)
	ctx := context.Background()
	p, err := List(ctx, v, owner, Query{}, vrfName)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GigabitEthernet0/8/0 192.0.2.9 02:00:00:00:00:99 ipv4 dynamic default",
		"host-w9l0 10.9.2.7 02:00:00:00:92:07 ipv4 dynamic default",
		"loop901 10.9.1.3 02:00:00:00:09:03 ipv4 dynamic red",
		"loop901 10.9.1.20 02:00:00:00:09:20 ipv4 dynamic red",
		"loop901 10.9.1.50 02:00:00:00:09:50 ipv4 static red",
		"loop901 2001:db8:9:1::5 02:00:00:00:09:05 ipv6 dynamic red",
	}
	if got := rows(p); p.Total != 6 || strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("total %d rows\n%s", p.Total, strings.Join(got, "\n"))
	}
	if p.Entries[0].Age != 12.5 || p.Entries[4].NoFibEntry || p.Entries[4].Age != 0 { // static: the model reports age 3 like VPP; the lister says 0
		t.Fatalf("age/flags %+v", p.Entries)
	}
	for _, c := range []struct {
		q    Query
		want []string
	}{
		{Query{VRF: "red", Family: "ipv4", State: "dynamic"}, []string{want[2], want[3]}},
		{Query{Interface: "host-w9l0"}, []string{want[1]}},
		{Query{State: "static"}, []string{want[4]}},
		{Query{Search: ":09:0"}, []string{want[2], want[5]}},
		{Query{Search: "LOOP901", Family: "ipv6"}, []string{want[5]}},
		{Query{Sort: "ip", Limit: 2}, []string{want[2], want[3]}},
		{Query{Sort: "ip", Descending: true, Limit: 1}, []string{want[5]}},
		{Query{Sort: "mac", Offset: 5}, []string{want[1]}},
		{Query{Interface: "loop301"}, nil}, // another owner's interface: not nameable
		{Query{Interface: "local0"}, nil},  // never
		{Query{Offset: 100}, nil},          // past the end: empty page, total kept
		{Query{VRF: "nosuch"}, nil},        // unknown VRF name: no rows
		{Query{Interface: "GigabitEthernet0/8/0", Family: "ipv6"}, nil},
	} {
		p, err := List(ctx, v, owner, c.q, vrfName)
		if err != nil {
			t.Fatalf("%+v: %v", c.q, err)
		}
		if got := rows(p); strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%+v:\n%s\nwant\n%s", c.q, strings.Join(got, "\n"), strings.Join(c.want, "\n"))
		}
	}
	if m.AllCalls() != 0 {
		t.Fatalf("%d dumps named every interface (sw_if_index ~0)", m.AllCalls())
	}
	for _, c := range v.CallsNamed("ip_neighbor_dump") {
		if idx := uint32(c.(*ip_neighbor.IPNeighborDump).SwIfIndex); idx == 0 || idx == ^uint32(0) {
			t.Fatalf("ip_neighbor_dump with sw_if_index %d", idx)
		}
	}
}

func TestListRejectsBadQueries(t *testing.T) {
	v, _ := rig(t)
	for _, q := range []Query{{Family: "ipx"}, {State: "stale"}, {Sort: "speed"}, {Limit: MaxLimit + 1}} {
		if _, err := List(context.Background(), v, owner, q, vrfName); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", q, err)
		}
	}
}

func TestFlushDeletesLearnedEntriesOnly(t *testing.T) {
	v, m := rig(t)
	ctx := context.Background()
	var lines []string
	res, err := Flush(ctx, v, owner, []string{"loop901"}, Both, func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 3 || res.Interfaces != 1 {
		t.Fatalf("result %+v", res)
	}
	if got := strings.Join(lines, "|"); got != "loop901 ipv4: deleted 2 learned entries|loop901 ipv6: deleted 1 learned entries" {
		t.Fatalf("lines %s", got)
	}
	if got := m.Neighbors(v, "loop901"); len(got) != 1 || got[0] != "10.9.1.50 02:00:00:00:09:50 static" {
		t.Fatalf("static entry must stay: %v", got)
	}
	if got := m.Neighbors(v, "host-w9l0"); len(got) != 1 {
		t.Fatalf("other interfaces untouched: %v", got)
	}
	if len(v.CallsNamed("ip_neighbor_flush")) != 0 || m.AllCalls() != 0 {
		t.Fatal("flush must not use ip_neighbor_flush (static entries) or ~0")
	}
	// family filter
	m.Learn(v, "host-w9l0", "2001:db8:9:2::7", "02:00:00:00:92:08", 1)
	if res, err := Flush(ctx, v, owner, []string{"host-w9l0"}, IPv6, nil); err != nil || res.Deleted != 1 {
		t.Fatalf("ipv6 flush %+v %v", res, err)
	}
	if got := m.Neighbors(v, "host-w9l0"); len(got) != 1 || !strings.HasPrefix(got[0], "10.9.2.7 ") {
		t.Fatalf("ipv4 entry must stay: %v", got)
	}
}

func TestFlushRefusesForeignAndUnknownInterfaces(t *testing.T) {
	v, m := rig(t)
	for _, name := range []string{"loop301", "local0", "nosuch"} {
		if _, err := Flush(context.Background(), v, owner, []string{name}, Both, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got := m.Neighbors(v, "loop301"); len(got) != 1 {
		t.Fatalf("another owner's neighbours were touched: %v", got)
	}
	if n := len(v.CallsNamed("ip_neighbor_add_del")); n != 1 { // only rig()'s static add
		t.Fatalf("%d ip_neighbor_add_del calls", n)
	}
}

// An entry that ages out between the dump and the delete is not an error.
func TestFlushToleratesVanishedEntries(t *testing.T) {
	v, _ := rig(t)
	v.On("ip_neighbor_add_del", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ip_neighbor.IPNeighborAddDelReply{Retval: coretest.RetvalNoSuchEntry}}, nil
	})
	res, err := Flush(context.Background(), v, owner, []string{"host-w9l0"}, IPv4, nil)
	if err != nil || res.Deleted != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	v.On("ip_neighbor_add_del", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ip_neighbor.IPNeighborAddDelReply{Retval: -1}}, nil
	})
	if _, err := Flush(context.Background(), v, owner, []string{"loop901"}, IPv4, nil); err == nil || errors.Is(err, ErrInvalid) {
		t.Fatalf("a real VPP failure must surface: %v", err)
	}
}

func TestSubscribeRefusesAllInterfaces(t *testing.T) {
	v, m := rig(t)
	for _, idx := range []uint32{0, ^uint32(0)} {
		if err := Subscribe(context.Background(), v, idx, 1, true); err == nil {
			t.Fatalf("subscribe %d accepted", idx)
		}
	}
	if m.AllCalls() != 0 || len(v.CallsNamed("want_ip_neighbor_events_v2")) != 0 {
		t.Fatal("nothing may be sent for 0 / ~0")
	}
	i, _ := v.InterfaceByName("loop901")
	if err := Subscribe(context.Background(), v, i.Index, 7, true); err != nil {
		t.Fatal(err)
	}
	if w := m.Watched(); len(w) != 1 || w[0] != i.Index {
		t.Fatalf("watched %v", w)
	}
}

func TestDecodeEvent(t *testing.T) {
	a, _ := df2.ParseAddr("2001:db8::7")
	hw, _ := ethernet_types.ParseMacAddress("02:AA:00:00:00:07")
	ch, ok := DecodeEvent(&ip_neighbor.IPNeighborEventV2{Flags: ip_neighbor.IP_NEIGHBOR_API_EVENT_FLAG_REMOVED, Neighbor: ip_neighbor.IPNeighbor{
		SwIfIndex: 4, MacAddress: hw, IPAddress: df2.ToAddress(a),
	}})
	if !ok || ch != (Change{SwIfIndex: 4, Removed: true, IP: "2001:db8::7", MAC: "02:aa:00:00:00:07"}) {
		t.Fatalf("%+v %v", ch, ok)
	}
	if _, ok := DecodeEvent(&ip_neighbor.IPNeighborEvent{}); ok {
		t.Fatal("v1 event decoded")
	}
	_ = ip_types.ADDRESS_IP6
}

func TestCoalescer(t *testing.T) {
	var c Coalescer
	if c.Pending() || c.Flush() != nil {
		t.Fatal("empty")
	}
	for i := 0; i < 50; i++ {
		c.Add("loop901", Change{Added: true})
	}
	c.Add("loop901", Change{Removed: true})
	c.Add("host-w9l0", Change{})
	evs := c.Flush()
	if len(evs) != 2 || c.Pending() {
		t.Fatalf("%d events", len(evs))
	}
	if evs[0].GetInterface() != "host-w9l0" || evs[0].GetAttributes()["updated"] != "1" || evs[0].GetKind() != vrxv1.EventKind_EVENT_KIND_NEIGHBOR_CHANGED {
		t.Fatalf("%v", evs[0])
	}
	if evs[1].GetInterface() != "loop901" || evs[1].GetAttributes()["added"] != "50" || evs[1].GetAttributes()["removed"] != "1" ||
		evs[1].GetMessage() != "neighbours on loop901: 50 added, 1 removed" {
		t.Fatalf("%v", evs[1])
	}
	for i := 0; i <= MaxInterfacesPerEvent; i++ {
		c.Add(fmt.Sprintf("if%d", i), Change{Added: true})
	}
	evs = c.Flush()
	if len(evs) != 1 || evs[0].Interface != nil || evs[0].GetAttributes()["interfaces"] != fmt.Sprint(MaxInterfacesPerEvent+1) {
		t.Fatalf("aggregate %v", evs)
	}
}
