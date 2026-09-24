package df7test

import (
	"context"
	"strconv"
	"strings"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// HwIndex returns the hardware interface index of name. The binary API exposes no
// hw_if_index for an interface, so this TEST-ONLY guard reads `show hardware-interfaces
// <name>` through cli_inband (no vppctl, no shell). It exists for the LLDP host test: VPP
// 26.06 sw_interface_set_lldp uses the sw_if_index as a hw_if_index, and on the shared host a
// mismatch would put LLDP on another worker's interface.
func (h *Host) HwIndex(name string) (uint32, bool) {
	h.T.Helper()
	rep, err := vlib.NewServiceClient(h.C).CliInband(h.Ctx, &vlib.CliInband{Cmd: "show hardware-interfaces brief " + name})
	if err != nil {
		h.T.Fatalf("cli_inband: %v", err)
	}
	for _, line := range strings.Split(rep.Reply, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == name {
			n, err := strconv.ParseUint(f[1], 10, 32)
			if err == nil {
				return uint32(n), true
			}
		}
	}
	return 0, false
}

// AlignedLoopback returns one of this slot's loopbacks (index first, first+1, …) whose
// sw_if_index equals its hw_if_index. VPP allocates software and hardware indexes from two
// pools; after each miss a VLAN sub-interface (which takes a software index only) nudges the
// software pool towards the hardware one. Everything is removed in Cleanup. ok is false when
// no aligned loopback was found within the slot's budget.
func (h *Host) AlignedLoopback(first, tries int) (name string, swIfIndex uint32, ok bool) {
	h.T.Helper()
	svc := interfaces.NewServiceClient(h.C)
	for i := 0; i < tries; i++ {
		n, idx := h.Loopback(first+i, true, true)
		hw, found := h.HwIndex(n)
		h.T.Logf("%s: sw_if_index %d hw_if_index %d (found %v)", n, idx, hw, found)
		if found && hw == idx {
			return n, idx, true
		}
		if !found || hw < idx {
			continue // software pool ahead: the hardware pool may catch up over its holes
		}
		rep, err := svc.CreateSubif(h.Ctx, &interfaces.CreateSubif{SwIfIndex: interface_types.InterfaceIndex(idx), SubID: 1, OuterVlanID: 1,
			SubIfFlags: interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_EXACT_MATCH})
		if err != nil {
			h.T.Fatalf("create_subif on %s: %v", n, err)
		}
		sub := rep.SwIfIndex
		h.T.Cleanup(func() {
			_ = ifsanitize.BeforeDelete(context.Background(), h.C, uint32(sub), "test cleanup") // D-095 c: bindings go before the interface (V19)
			if _, err := svc.DeleteSubif(context.Background(), &interfaces.DeleteSubif{SwIfIndex: sub}); err != nil {
				h.T.Errorf("cleanup delete_subif %d: %v", sub, err)
			}
		})
	}
	return "", 0, false
}
