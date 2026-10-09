package desired

import (
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/l2"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/scheduler"
)

// PppoeCarrierVLAN emits only the selected raw child's ingress pop. VPP derives
// the inverse output push from the subinterface's authoritative tag definition.
// Call only after validating all carriers, so invalid plans emit no partial graph.
func PppoeCarrierVLAN(s Sink, parent desc.CarrierParent, pointer string) {
	if parent.Rewrite != nil {
		s.Add(scheduler.Join(l2.VlanTagRewriteName, parent.Name), parent.Rewrite, pointer)
	}
}

func PppoeCarrierParent(ifs map[string]*ngfwv1.Interface, name string) (desc.CarrierParent, error) {
	return desc.ResolveCarrierParent(ifs, name)
}
