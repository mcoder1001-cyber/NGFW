package desired

import (
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/lcp_osi"
	"sync/atomic"
)

var isisOSI atomic.Bool

// ConfigureIsisOSI opts the designated globals owner into irreversible enable.
func ConfigureIsisOSI(owner, optedIn bool) { isisOSI.Store(owner && optedIn) }

// IsisOSI projects the explicitly authorized global OSI punt setting.
func IsisOSI(s Sink, ds *ngfwv1.DesiredState, in map[string]bool) {
	if !in["routing"] || ds.GetRouting().GetIsis() == nil {
		return
	}
	if !isisOSI.Load() {
		s.Warnf("/routing/isis", "isis.osi-disabled", "IS-IS OSI punt is not managed: globals owner and explicit irreversible-enable opt-in required; VPP FIB acceptance unverified")
		return
	}
	s.Add(lcp_osi.Key, lcp_osi.Value(), "/routing/isis")
	s.Warnf("/routing/isis", "isis.osi-no-disable", "VPP OSI punt cannot be disabled by rollback or configuration removal; only VPP restart clears it")
}
