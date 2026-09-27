package nftables

// F-global-blocking: acl.globalBlocking lists with protectHost also drop traffic to the box itself.
//
//	chain in__gb { type filter hook input priority -300; policy accept;
//	    [anti-lockout accept rules, only when acl.hostSettings.antiLockout names its sources]
//	    ip saddr @b4_<list> counter [log prefix "vrx:gb:<list> "] drop    comment "vrx:@global-blocking/<n>:…"
//	    ip6 saddr @b6_<list> counter … drop }
//
// The chain runs before connection tracking, so a blocked source never creates a ct entry, and it drops
// established traffic too (a list is a block, not a policy). It applies to every host interface: which
// Linux interface carries traffic of a VPP interface is not known here (management included). Accept
// rules in this chain only end this chain; the host lists' chains still decide for everything else.

import (
	"net/netip"
	"sort"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// hostBlock is one block list that protects the box.
type hostBlock struct {
	name   string
	log    bool
	v4, v6 []netip.Prefix // sorted, overlaps removed
	bad    []int          // indexes of entries that are not canonical prefixes
}

// hostBlockLists returns the enabled block lists with protectHost and entries, by name.
func hostBlockLists(acl *vrxv1.AclConfig) []*hostBlock {
	lists := acl.GetGlobalBlocking().GetLists()
	names := make([]string, 0, len(lists))
	for n := range lists {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []*hostBlock
	for _, n := range names {
		l := lists[n]
		if (l.Enabled != nil && !l.GetEnabled()) || !l.GetProtectHost() || len(l.GetEntries()) == 0 {
			continue
		}
		hb := &hostBlock{name: n, log: l.GetLog()}
		for i, e := range l.GetEntries() {
			p, err := netip.ParsePrefix(e)
			if err != nil || p.Masked() != p {
				hb.bad = append(hb.bad, i)
				continue
			}
			if p.Addr().Is4() {
				hb.v4 = append(hb.v4, p)
			} else {
				hb.v6 = append(hb.v6, p)
			}
		}
		hb.v4, hb.v6 = outermost(hb.v4), outermost(hb.v6)
		out = append(out, hb)
	}
	return out
}

// outermost sorts ps and drops every prefix inside an earlier one (an interval set refuses overlaps).
func outermost(ps []netip.Prefix) []netip.Prefix {
	sort.Slice(ps, func(i, j int) bool { return prefixLess(ps[i], ps[j]) })
	var out []netip.Prefix
	for _, p := range ps {
		if n := len(out); n > 0 && out[n-1].Bits() <= p.Bits() && out[n-1].Contains(p.Addr()) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// blockSetName is the set of list's addresses of family (4 or 6).
func blockSetName(family int, list string) string {
	if family == 4 {
		return "b4_" + list
	}
	return "b6_" + list
}

// blockChain plans the block lists' chain and adds their sets (nil when no list protects the box).
func (b *builder) blockChain(lists []*hostBlock) *chainPlan {
	var rules []*nrule
	for _, l := range lists {
		pt := ptr("acl", "globalBlocking", "lists", l.name)
		if !objectNameRe.MatchString(l.name) {
			b.errorf(pt, RuleListName, "block list name %q is not a valid name", l.name)
			continue
		}
		if len(l.bad) > 0 {
			continue // the acl projection reports the entries (acl.global-blocking); the transaction fails there
		}
		for _, f := range []struct {
			family int
			ps     []netip.Prefix
			typ    string
		}{{4, l.v4, "ipv4_addr"}, {6, l.v6, "ipv6_addr"}} {
			if len(f.ps) == 0 {
				continue
			}
			name := blockSetName(f.family, l.name)
			b.sets[name] = &Set{Name: name, Type: f.typ, Elements: prefixStrings(f.ps)}
			rules = append(rules, &nrule{
				id: "@" + KindGlobalBlocking, n: len(rules), kind: KindGlobalBlocking, list: l.name, pointer: pt,
				family: f.family, blockSet: name, log: l.log, verdict: "drop",
			})
		}
	}
	if len(rules) == 0 {
		return nil
	}
	c := &chainPlan{name: BlockChain, hook: "input", priority: BlockPriority, policy: "accept"}
	if b.lockout.enabled && len(b.lockout.sources) > 0 {
		c.rules = append(c.rules, b.antiLockoutRules()...)
	}
	c.rules = append(c.rules, rules...)
	return c
}
