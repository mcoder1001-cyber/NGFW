package ravpn

import (
	"ngfw/agent/internal/vpp/bootid"
)

// Handoff records observed VPP indices only after tag/kind/address/VRF/ACL
// readback. It is root-private runtime metadata, never desired configuration.
type Handoff struct {
	Format         uint32          `json:"format"`
	Instance       string          `json:"instance"`
	NamespaceInode uint64          `json:"namespaceInode"`
	VPPBoot        bootid.Identity `json:"vppBoot"`
	OuterIndex     uint32          `json:"outerIndex"`
	InnerIndex     uint32          `json:"innerIndex"`
	OuterName      string          `json:"outerName"`
	InnerName      string          `json:"innerName"`
}

func LinkName(instance string, outer bool) string {
	if !ValidInstance(instance) {
		return ""
	}
	suffix := "i"
	if outer {
		suffix = "o"
	}
	return "ra_" + instance[:24] + suffix
}

// ValidateHandoff never trusts a persisted sw_if_index alone. Caller supplies
// freshly observed, owner-tagged TAP indices and current complete D-080 triple.
func ValidateHandoff(plan *NetworkPlan, receipt Handoff, current bootid.Identity, outer, inner uint32) error {
	if plan.Validate() != nil || plan.NamespaceInode == 0 || receipt.Format != 1 || receipt.Instance != plan.Instance || receipt.NamespaceInode != plan.NamespaceInode || !current.Complete() || !receipt.VPPBoot.Complete() || !current.Equal(receipt.VPPBoot) {
		return ErrBoundary
	}
	if receipt.OuterName != LinkName(plan.Instance, true) || receipt.InnerName != LinkName(plan.Instance, false) || receipt.OuterIndex != outer || receipt.InnerIndex != inner || outer == inner || outer == ^uint32(0) || inner == ^uint32(0) {
		return ErrBoundary
	}
	return nil
}
