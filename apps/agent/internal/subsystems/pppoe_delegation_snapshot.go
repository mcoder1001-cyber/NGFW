package subsystems

import (
	"sort"

	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers/pppoe"
)

// carrierDelegationReadiness is implemented by carrier runtime integration. Its
// method verifies a current Carrier session, ownership/readback and exact admission
// identity. Without that integration PD remains closed, even when a hook says up.
type carrierDelegationReadiness interface {
	CarrierDelegationReady(logical, admission string) bool
}

// DelegationSnapshot performs host observation outside the scheduler transaction.
// The dynamic source caches this result before calling sync; Desired never does I/O.
func (rt *PppoeRuntime) DelegationSnapshot() []desired.PppoeDelegationLease {
	ready, supported := any(rt).(carrierDelegationReadiness)
	if !supported || rt.renderer == nil {
		return nil
	}
	rt.mu.Lock()
	sessions := make(map[string]pppoe.Session, len(rt.applied))
	for name, session := range rt.applied {
		sessions[name] = session
	}
	rt.mu.Unlock()
	var leases []desired.PppoeDelegationLease
	for logical, session := range sessions {
		if session.IPv6 != "dhcpv6" {
			continue
		}
		state, err := rt.renderer.ReadIPv6(session.HostIf)
		if err != nil || !state.Up || state.Failure != "" || !state.Delegated.IsValid() ||
			!ready.CarrierDelegationReady(logical, state.PDGeneration) {
			continue
		}
		leases = append(leases, desired.PppoeDelegationLease{
			Logical: logical, Generation: state.PDGeneration, Delegated: state.Delegated,
			ValidUntil: state.PDValidUntil, PreferredUntil: state.PDPreferredUntil, Ready: true,
		})
	}
	sort.Slice(leases, func(i, j int) bool { return leases[i].Logical < leases[j].Logical })
	return leases
}
