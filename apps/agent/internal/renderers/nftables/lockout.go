package nftables

import (
	"fmt"
	"net/netip"
	"slices"
)

// Anti-lockout check (rule acl.host-anti-lockout). Management traffic is described by probes: a new TCP
// connection to each management port, from each configured source prefix (any IPv4 and any IPv6 source
// when none is configured), arriving on each configured interface (any interface when none is). Every
// probe is walked through the input chains as rendered, in hook-priority order, the way nftables evaluates
// base chains on one hook: in each chain the first rule that matches decides — an accept ends that chain
// and the packet moves on to the next one; a drop or reject is final; no match means the chain's policy.
//
// A probe is a set of packets, so a rule relates to it in one of three ways: it matches none of them,
// all of them (full) or some (partial). An accept only lets the probe through when it matches fully; a
// drop or reject that matches even partly is reported — the check is conservative and never misses a
// lockout (an accept that is split over several rules covering the source together is reported too, and
// the message says which rule to widen).
//
// The rendered anti-lockout rule accepts every probe first, so with it enabled the check can only warn:
// a rule that would drop management traffic without it is shadowed (acl.host-anti-lockout-shadow). With
// it disabled the rules themselves must accept every probe; otherwise the commit fails at the pointer of
// the first rule (or the default policy) that drops it.

type rel int

const (
	relNone rel = iota
	relPartial
	relFull
)

type probe struct {
	family int
	src    netip.Prefix
	iif    string // "" = any interface
	port   uint16
}

func (p probe) String() string {
	src := p.src.String()
	if p.src.Bits() == 0 {
		src = fmt.Sprintf("any IPv%d source", p.family)
	}
	on := "any interface"
	if p.iif != "" {
		on = p.iif
	}
	return fmt.Sprintf("management TCP %d from %s on %s", p.port, src, on)
}

func (b *builder) probes() []probe {
	srcs := b.lockout.sources
	if len(srcs) == 0 {
		srcs = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")}
	}
	ifs := b.lockout.interfaces
	if len(ifs) == 0 {
		ifs = []string{""}
	}
	var out []probe
	for _, s := range srcs {
		for _, i := range ifs {
			for _, port := range b.lockout.ports {
				out = append(out, probe{family: familyOf(s), src: s, iif: i, port: port})
			}
		}
	}
	return out
}

// checkLockout reports probes the rendered input chains would drop (errors) and, with the anti-lockout
// rule enabled, rules that would drop them without it (warnings). One finding per pointer.
func (b *builder) checkLockout(chains []*chainPlan) {
	reported := map[string]bool{}
	report := func(p probe, r *nrule, c *chainPlan, shadow bool) {
		pointer := ptr("acl", "hostSettings", "defaultInput")
		what := fmt.Sprintf("the default input policy (drop) of chain %s", c.name)
		if r != nil {
			pointer = r.pointer
			what = fmt.Sprintf("this rule (%s, chain %s)", r.verdict, c.name)
		}
		if reported[pointer] {
			return
		}
		reported[pointer] = true
		if shadow {
			b.warnf(pointer, RuleLockoutShadow, "%s would be dropped by %s, but the anti-lockout rule (acl.hostSettings.antiLockout) accepts it first", p, what)
			return
		}
		hint := ""
		if len(b.lockout.sources) == 0 {
			hint = "; with no acl.hostSettings.antiLockout.sources every source counts as management — set them to your management network"
		}
		b.errorf(pointer, RuleAntiLockout, "%s would be dropped by %s: the anti-lockout rule is off (acl.hostSettings.antiLockout.enabled), so the rules must accept management traffic before any drop — add an accept rule in front, narrow this one, or turn the anti-lockout rule on (matches through FQDN objects never count as protection)%s", p, what, hint)
	}
	for _, p := range b.probes() {
		if r, c := dropperOf(chains, p, false); c != nil {
			report(p, r, c, false)
			continue
		}
		if b.lockout.enabled {
			if r, c := dropperOf(chains, p, true); c != nil {
				report(p, r, c, true)
			}
		}
	}
}

// dropperOf walks p through the input chains and returns the rule (nil: the chain policy) and chain that
// drop it, or a nil chain when p is accepted. skipLockout ignores the anti-lockout rules.
func dropperOf(chains []*chainPlan, p probe, skipLockout bool) (*nrule, *chainPlan) {
	for _, c := range chains {
		if c.hook != "input" {
			continue
		}
		passed := false
		for _, r := range c.rules {
			if skipLockout && r.kind == KindAntiLockout {
				continue
			}
			x := relation(r, p)
			if x == relNone {
				continue
			}
			if r.verdict == "accept" {
				if x == relFull {
					passed = true
					break
				}
				continue
			}
			return r, c
		}
		if !passed && c.policy == "drop" {
			return nil, c
		}
	}
	return nil, nil
}

// relation of rule r to the packets of probe p (a new TCP connection).
func relation(r *nrule, p probe) rel {
	if r.ctEst || r.nd {
		return relNone // a new connection is not established/related, and not ICMPv6
	}
	if r.family != 0 && r.family != p.family {
		return relNone
	}
	res := relFull
	meet := func(x rel) {
		if x < res {
			res = x
		}
	}
	if r.loopback {
		meet(ifaceRel([]string{"lo"}, p.iif))
	}
	if len(r.ifaces) > 0 {
		meet(ifaceRel(r.ifaces, p.iif))
	}
	meet(srcRel(r.src, p.family, p.src))
	meet(dstRel(r.dst, p.family))
	meet(svcRel(r, p.port))
	return res
}

func ifaceRel(ifaces []string, iif string) rel {
	switch {
	case iif == "":
		return relPartial
	case slices.Contains(ifaces, iif):
		return relFull
	default:
		return relNone
	}
}

func familyList(m addrMatch, family int) []netip.Prefix {
	if family == 4 {
		return m.p4
	}
	return m.p6
}

func srcRel(m addrMatch, family int, src netip.Prefix) rel {
	if !m.on {
		return relFull
	}
	if m.dynamic {
		return relPartial // DNS answers: may or may not match, never protects (M2)
	}
	best := relNone
	for _, q := range familyList(m, family) {
		if q.Bits() <= src.Bits() && q.Contains(src.Addr()) {
			return relFull
		}
		if q.Overlaps(src) {
			best = relPartial
		}
	}
	return best
}

// dstRel: the probe goes to any address of the host, which the agent does not know here.
func dstRel(m addrMatch, family int) rel {
	if !m.on {
		return relFull
	}
	if m.dynamic {
		return relPartial
	}
	list := familyList(m, family)
	if len(list) == 0 {
		return relNone
	}
	for _, q := range list {
		if q.Bits() == 0 {
			return relFull
		}
	}
	return relPartial
}

func svcRel(r *nrule, port uint16) rel {
	if len(r.dports) > 0 {
		if slices.Contains(r.dports, port) {
			return relFull
		}
		return relNone
	}
	g := r.svc
	if g == nil || g.proto == 0 {
		return relFull
	}
	if g.proto != 6 { // only TCP clauses can match a TCP probe
		return relNone
	}
	if g.dport != nil && !slices.ContainsFunc(g.dport, func(s span) bool { return s.first <= port && port <= s.last }) {
		return relNone
	}
	if g.sport != nil || g.flagsMask != 0 {
		return relPartial
	}
	return relFull
}
