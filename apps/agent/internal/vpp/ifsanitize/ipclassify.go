package ifsanitize

import (
	"context"
	"fmt"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
)

// ResetIPClassify sets the ip4 and ip6 "ip classify table" of idx explicitly to ~0
// (classify_set_interface_ip_table, table_index ~0). Call it right after VPP returns idx for a new
// interface and before any address is added to it (INC-vpp-classify-crash, D-185, mitigation M1).
//
// Why: VPP 26.06 vnet_set_ip4_classify_intfc / vnet_set_ip6_classify_intfc grow
// classify_table_index_by_sw_if_index with vec_validate, which ZERO-fills the new slots, and 0 is a
// valid classify table index. A reset of a high sw_if_index therefore binds every lower index that
// was never set explicitly to classify table 0. The next address add on such an interface installs
// a FIB_SOURCE_CLASSIFY /32 with a classify DPO to table 0 (ip4_add_interface_routes); once table 0
// is freed (an ACL-plugin MACIP table, a sanitizer probe, …) the first packet to the address
// crashes VPP in vnet_classify_find_entry (2026-09-28 14:10:40 and 14:13:00). A slot that was set
// explicitly lies inside the vector and can never be zero-filled later: the fill touches only
// slots beyond the vector's current length.
//
// Sanitize (and so Acquire) does this as one of its steps; ResetIPClassify is the light form for
// test fixtures and helpers that create an interface directly and do not need the full sanitize.
// A reset names no table, so it cannot fail on a deleted one. When the interface already has an
// address, the reset also removes an armed classify /32 of its first address — so repair (reset)
// always goes BEFORE an address delete: after the address is gone the /32 stays in the FIB.
// Only the ip4 half repairs: VPP's ip6 reset removes the /128 through the interface's IPv4 FIB
// index (ip6_forward.c:3006, review F6), so the ip6 reset is meant for create time, before any
// IPv6 address.
func ResetIPClassify(ctx context.Context, c vpp.Client, idx uint32) error {
	svc := classifyapi.NewServiceClient(c)
	for _, v6 := range []bool{false, true} {
		req := &classifyapi.ClassifySetInterfaceIPTable{IsIPv6: v6, SwIfIndex: interface_types.InterfaceIndex(idx), TableIndex: NoIndex}
		if _, err := svc.ClassifySetInterfaceIPTable(ctx, req); err != nil {
			return fmt.Errorf("classify_set_interface_ip_table (reset ip%s, sw_if_index %d): %w", af(v6), idx, err)
		}
	}
	return nil
}
