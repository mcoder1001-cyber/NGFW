package scheduler

import (
	"context"
	"strings"
	"testing"
)

// TD-11c (review 3.1c): the topological order follows a dependency THROUGH an object that is not a
// node of the plan — the observe-only "interface/<name>" alias (D-065) of an interface the
// transaction deletes — to the creator behind it, so deletes run dependents-first for every
// interface kind, whatever the descriptor registration order. Before TD-11c the edge was dropped
// (the alias is never a plan node once it leaves the desired state) and the order fell back to the
// registration-order tie breaker: a creator ranked after its attributes (af_packet after core's
// interface-ip, F-bonding's bond after DF-1) was deleted first, and a sub-interface (ordered after
// its attributes through its dependency on the parent's alias) too (F-vlan-qinq Q1).

// aliasFixture registers the product's descriptor names in the product's registration order
// (subsystems.Register: core, af_packet, DF-1 sub-interface and attributes, alias) plus a feature
// family registered after them (bond, bridge), with the alias observe-only as DF-1's is.
func aliasFixture(t *testing.T, extra ...string) (*Scheduler, *store) {
	t.Helper()
	st := newStore()
	reg := NewRegistry()
	for _, n := range []string{
		"vrf", "interface.loopback", "interface-ip.table", "interface-ip", "ip.route",
		"af-packet.host-interface", "interface.subinterface", "interface.admin-state", "interface.mtu",
	} {
		reg.Register(&mem{name: n, st: st})
	}
	for _, n := range extra {
		reg.Register(&mem{name: n, st: st})
	}
	reg.Register(&observe{mem{name: "interface", st: st}})
	s := New(reg, nil)
	s.VerifyRetries = 0
	return s, st
}

// before reports whether op a precedes op b in ops (both must be present).
func before(t *testing.T, ops []string, a, b string) bool {
	t.Helper()
	ia, ib := -1, -1
	for i, o := range ops {
		switch o {
		case a:
			ia = i
		case b:
			ib = i
		}
	}
	if ia < 0 || ib < 0 {
		t.Fatalf("ops %v: missing %q or %q", ops, a, b)
	}
	return ia < ib
}

// alias is the desired "interface/<name>" object of an interface (creator "" = physical NIC).
func alias(name, creator string) KV {
	if creator == "" {
		return kv("interface", obj(name, "1"))
	}
	return kv("interface", obj(name, "1", creator))
}

func TestDeleteOrderFollowsAliasToCreator(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		extra []string // feature descriptors registered after DF-1
		base  []KV     // applied first
		keep  []KV     // the desired state that removes the rest
		// every pair {dependent, creator}: the dependent's delete must run before the creator's
		order [][2]string
	}{{
		// F-vlan-qinq Q1: an enabled sub-interface with an address is removed while its parent stays.
		name: "sub-interface removed, parent stays",
		base: []KV{
			kv("af-packet.host-interface", obj("host-p0", "1")), alias("host-p0", "af-packet.host-interface/host-p0"),
			kv("interface.subinterface", obj("host-p0.100", "1", "interface/host-p0")), alias("host-p0.100", "interface.subinterface/host-p0.100"),
			kv("interface.admin-state", obj("host-p0.100", "1", "interface/host-p0.100")),
			kv("interface-ip", obj("host-p0.100", "10.0.100.1/24", "interface/host-p0.100", "?interface-ip.table/host-p0.100")),
		},
		keep: []KV{kv("af-packet.host-interface", obj("host-p0", "1")), alias("host-p0", "af-packet.host-interface/host-p0")},
		order: [][2]string{
			{"delete interface.admin-state/host-p0.100", "delete interface.subinterface/host-p0.100"},
			{"delete interface-ip/host-p0.100", "delete interface.subinterface/host-p0.100"},
		},
	}, {
		// af_packet is registered after core: its address, VRF binding and MTU go first.
		name: "af_packet interface removed with address, VRF binding, admin state, MTU",
		base: []KV{
			kv("vrf", obj("10001", "1")),
			kv("af-packet.host-interface", obj("host-p1", "1")), alias("host-p1", "af-packet.host-interface/host-p1"),
			kv("interface-ip.table", obj("host-p1", "10001", "interface/host-p1", "vrf/10001")),
			kv("interface-ip", obj("host-p1", "10.0.1.1/24", "interface/host-p1", "?interface-ip.table/host-p1")),
			kv("interface.admin-state", obj("host-p1", "1", "interface/host-p1")),
			kv("interface.mtu", obj("host-p1", "1400", "interface/host-p1")),
		},
		keep: []KV{kv("vrf", obj("10001", "1"))},
		order: [][2]string{
			{"delete interface-ip/host-p1", "delete af-packet.host-interface/host-p1"},
			{"delete interface-ip.table/host-p1", "delete af-packet.host-interface/host-p1"},
			{"delete interface-ip/host-p1", "delete interface-ip.table/host-p1"},
			{"delete interface.admin-state/host-p1", "delete af-packet.host-interface/host-p1"},
			{"delete interface.mtu/host-p1", "delete af-packet.host-interface/host-p1"},
		},
	}, {
		// F-bonding shape: bond.bond registered after DF-1; a member NIC (untagged, stays) and the
		// bond's own attributes hang off the bond's alias.
		name:  "bond removed with member and address",
		extra: []string{"bond.member", "bond.bond"},
		base: []KV{
			alias("lan", ""),
			kv("bond.bond", obj("BondEthernet0", "1")), alias("BondEthernet0", "bond.bond/BondEthernet0"),
			kv("bond.member", obj("lan", "1", "interface/BondEthernet0", "interface/lan")),
			kv("interface.admin-state", obj("BondEthernet0", "1", "interface/BondEthernet0")),
			kv("interface-ip", obj("BondEthernet0", "10.0.2.1/24", "interface/BondEthernet0", "?interface-ip.table/BondEthernet0")),
		},
		keep: []KV{alias("lan", "")},
		order: [][2]string{
			{"delete bond.member/lan", "delete bond.bond/BondEthernet0"},
			{"delete interface.admin-state/BondEthernet0", "delete bond.bond/BondEthernet0"},
			{"delete interface-ip/BondEthernet0", "delete bond.bond/BondEthernet0"},
		},
	}, {
		// F-bridge-l2 shape: a BVI creator registered after the members; the BVI membership and the
		// BVI address hang off the BVI's alias, the NIC member off the NIC's (which stays).
		name:  "bridge with BVI removed",
		extra: []string{"l2.bridge-domain", "l2.bridge-member", "bvi.interface"},
		base: []KV{
			alias("lan", ""),
			kv("l2.bridge-domain", obj("10", "1")),
			kv("bvi.interface", obj("bvi10", "1")), alias("bvi10", "bvi.interface/bvi10"),
			kv("l2.bridge-member", obj("10-bvi10", "bvi", "l2.bridge-domain/10", "interface/bvi10")),
			kv("l2.bridge-member", obj("10-lan", "normal", "l2.bridge-domain/10", "interface/lan")),
			kv("interface-ip", obj("bvi10", "10.0.10.1/24", "interface/bvi10", "?interface-ip.table/bvi10")),
		},
		keep: []KV{alias("lan", "")},
		order: [][2]string{
			{"delete l2.bridge-member/10-bvi10", "delete bvi.interface/bvi10"},
			{"delete interface-ip/bvi10", "delete bvi.interface/bvi10"},
			{"delete l2.bridge-member/10-bvi10", "delete l2.bridge-domain/10"},
			{"delete l2.bridge-member/10-lan", "delete l2.bridge-domain/10"},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			s, st := aliasFixture(t, tc.extra...)
			mustApplied(t, s.Apply(ctx, tc.base, nil))
			st.reset()
			r := s.Apply(ctx, tc.keep, nil)
			mustApplied(t, r)
			ops := st.ops()
			for _, p := range tc.order {
				if !before(t, ops, p[0], p[1]) {
					t.Errorf("%s must run before %s; ops:\n  %s", p[0], p[1], strings.Join(ops, "\n  "))
				}
			}
			// the aliases are observe-only: never deleted
			for _, o := range ops {
				if strings.HasPrefix(o, "delete interface/") {
					t.Errorf("observe-only alias deleted: %s", o)
				}
			}
		})
	}
}
