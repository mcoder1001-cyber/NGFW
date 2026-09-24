package ipneighbor

import (
	"context"
	"testing"

	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/internal/descriptors/df2"
)

// TestRetrieveNeverDumpsAllInterfaces (F-neighbors-ra gap fix, TASK ENVELOPE shared-VPP rule): Retrieve asks VPP for
// one candidate interface at a time — this owner's tagged interfaces and untagged ones — and never with
// sw_if_index ~0, which on the shared host reads every slot's neighbour table. Before the fix every Retrieve sent
// ip_neighbor_dump{sw_if_index: 4294967295} for both families.
func TestRetrieveNeverDumpsAllInterfaces(t *testing.T) {
	v := newFakeVPP() // local0 (0), loop300 (5, w3), loop200 (7, w2)
	v.neighbors = []ip_neighbor.IPNeighbor{
		{SwIfIndex: 5, Flags: ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC, IPAddress: addr("10.3.1.10"), MacAddress: mac("aa:bb:cc:00:11:22")},
		{SwIfIndex: 7, Flags: ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC, IPAddress: addr("10.2.1.1"), MacAddress: mac("00:00:00:00:00:02")},
	}
	kvs, err := NewNeighbor(v, "w3").Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(kvs) != 1 || kvs[0].Key != "ip-neighbor.neighbor/loop300/10.3.1.10" {
		t.Fatalf("Retrieve = %+v", kvs)
	}
	calls := v.CallsNamed("ip_neighbor_dump")
	if len(calls) != 2 { // loop300 × {ipv4, ipv6}; loop200 is another owner's, local0 is never a candidate
		t.Fatalf("%d dumps", len(calls))
	}
	for _, c := range calls {
		if idx := uint32(c.(*ip_neighbor.IPNeighborDump).SwIfIndex); idx != 5 {
			t.Fatalf("ip_neighbor_dump for sw_if_index %d (all = %d)", idx, df2.NoInterface)
		}
	}
}
