package multiwan

import (
	"net/netip"
	"reflect"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// LearnedGateway is ephemeral, owned runtime state, never persisted configuration.
// Source is dhcp or pppoe; an absent observation withdraws the forwarding path.
type LearnedGateway struct {
	Source     string
	Generation string
	Address    netip.Prefix
	Gateway    netip.Addr
}

func (g LearnedGateway) valid() bool {
	return (g.Source == "dhcp" || (g.Source == "pppoe" && g.Generation != "")) && g.Address.IsValid() && g.Address.Addr().Is4() &&
		!g.Address.Addr().IsUnspecified() && !g.Address.Addr().IsMulticast() && g.Gateway.Is4() &&
		!g.Gateway.IsUnspecified() && !g.Gateway.IsMulticast()
}

// SetGateways replaces the complete observation, including withdrawals/errors.
// Callers serialize acquisition with committed configuration to reject stale reads.
func (r *Runtime) SetGateways(groups []*ngfwv1.WanGroup, identity string, observed map[string]LearnedGateway) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	values := make(map[string]LearnedGateway)
	for name, value := range observed {
		if value.valid() {
			values[name] = value
		}
	}
	now := time.Now()
	expired := !now.Before(r.gatewayUntil)
	// An empty observation has no lease that can expire. Resyncing it after a
	// slow transaction would keep requesting another equally slow full resync.
	changed := (expired && (len(r.gateways) != 0 || len(values) != 0)) || identity != r.gatewayIdentity || !equalGroups(groups, r.gatewayGroups) || !reflect.DeepEqual(values, r.gateways)
	// In-flight probes belong to the old learned egress. Reset hysteresis and
	// fence their completion whenever a lease/carrier generation changes.
	for _, group := range r.groups {
		for name, member := range group.members {
			before, had := r.gateways[name]
			after, has := values[name]
			if before != after || had != has || (expired && has) {
				member.egressEpoch++
				member.up = false
				member.since = now
				for i := range member.samples {
					member.samples[i] = sample{}
				}
			}
		}
	}
	r.gatewayIdentity = identity
	r.gatewayGroups = make([]*ngfwv1.WanGroup, len(groups))
	for i, g := range groups {
		r.gatewayGroups[i] = proto.Clone(g).(*ngfwv1.WanGroup)
	}
	r.gateways = values
	r.gatewayUntil = now.Add(5 * time.Second)
	return changed
}

func equalGroups(a, b []*ngfwv1.WanGroup) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !proto.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// ResolveGateways returns a detached projection. Learned addresses are included
// only when explicitly requested by the WAN-owned NAT projection; generic desired
// interface projection must never turn a lease into a configured static address.
func (r *Runtime) ResolveGateways(doc *ngfwv1.DesiredState, identity string, addresses bool) *ngfwv1.DesiredState {
	out := proto.Clone(doc).(*ngfwv1.DesiredState)
	r.mu.Lock()
	defer r.mu.Unlock()
	current := identity == r.gatewayIdentity && equalGroups(doc.GetRouting().GetWanGroups(), r.gatewayGroups) && time.Now().Before(r.gatewayUntil)
	for _, g := range out.GetRouting().GetWanGroups() {
		for _, m := range g.GetMembers() {
			if m.GetNextHop() != "dhcp" && m.GetNextHop() != "pppoe" {
				continue
			}
			m.Gateway = nil
			value, ok := r.gateways[m.GetInterface()]
			if !current || !ok || value.Source != m.GetNextHop() {
				continue
			}
			m.NextHop = proto.String("gateway")
			m.Gateway = proto.String(value.Gateway.String())
			if addresses {
				if iface := out.GetInterfaces()[m.GetInterface()]; iface != nil {
					iface.Ipv4 = []string{value.Address.String()}
				}
			}
		}
	}
	return out
}

// RetiredAddresses clears sessions after a lease is withdrawn or renumbered,
// even if probe health has not changed yet. Explicit NAT-disabled/non-sticky
// configuration is respected by DeadAddresses.
func RetiredAddresses(before, after *ngfwv1.DesiredState) map[string]bool {
	healthBefore, healthAfter := []*ngfwv1.WanGroupState{}, []*ngfwv1.WanGroupState{}
	for _, g := range before.GetRouting().GetWanGroups() {
		old := &ngfwv1.WanGroupState{Name: g.GetName()}
		next := &ngfwv1.WanGroupState{Name: g.GetName()}
		for _, m := range g.GetMembers() {
			old.Members = append(old.Members, &ngfwv1.WanMemberState{Interface: m.GetInterface(), Up: true})
			equal := reflect.DeepEqual(before.GetInterfaces()[m.GetInterface()].GetIpv4(), after.GetInterfaces()[m.GetInterface()].GetIpv4())
			next.Members = append(next.Members, &ngfwv1.WanMemberState{Interface: m.GetInterface(), Up: equal})
		}
		healthBefore = append(healthBefore, old)
		healthAfter = append(healthAfter, next)
	}
	return DeadAddresses(before, healthBefore, healthAfter)
}

// PruneLiveCleanup cancels queued deletion when an outside address becomes live
// again before a retry completes. NAT sessions contain no lease-generation ID,
// so deleting by a retired address after reuse would delete new sessions too.
// Protect every healthy member, including sticky flows on a non-selected backup.
// Reassigned retired addresses are protected even before health recovers.
func PruneLiveCleanup(pending, retired map[string]bool, doc *ngfwv1.DesiredState, health []*ngfwv1.WanGroupState) bool {
	changed := false
	healthy := map[string]bool{}
	for _, group := range health {
		for _, member := range group.GetMembers() {
			if member.GetUp() {
				healthy[member.GetInterface()] = true
			}
		}
	}
	for _, group := range doc.GetRouting().GetWanGroups() {
		for _, member := range group.GetMembers() {
			if member.GetNextHop() != "gateway" {
				continue
			}
			for _, raw := range doc.GetInterfaces()[member.GetInterface()].GetIpv4() {
				prefix, err := netip.ParsePrefix(raw)
				if err != nil || !prefix.Addr().Is4() {
					continue
				}
				address := prefix.Addr().String()
				if pending[address] && (healthy[member.GetInterface()] || retired[address]) {
					delete(pending, address)
					delete(retired, address)
					changed = true
				}
			}
		}
	}
	return changed
}
