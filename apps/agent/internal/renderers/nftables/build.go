package nftables

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/objects"
)

// Input of Build: the `acl` domain (only host, host_attachments and host_settings are read), the objects
// document the rules reference and the FQDN answers of the running agent (nil: FQDN objects expand to
// nothing).
type Input struct {
	Acl     *vrxv1.AclConfig
	Objects *vrxv1.ObjectsConfig
	FQDN    objects.FQDNLookup
}

// Issue is a finding of Build at a JSON pointer of the document (the agent's DryRun reports it).
type Issue struct {
	Pointer string
	Rule    string
	Message string
	Warning bool
}

// Rule names of the issues (validation rules of the agent's DryRun).
const (
	RuleListName      = "acl.host-list-name"
	RuleRule          = "acl.host-rule"
	RuleObject        = "acl.host-object"
	RuleLimit         = "acl.host-expansion-limit"
	RuleFQDN          = "acl.host-fqdn-unresolved"
	RuleEmpty         = "acl.host-rule-empty"
	RuleAttachment    = "acl.host-attachment"
	RuleSettings      = "acl.host-settings"
	RuleAntiLockout   = "acl.host-anti-lockout"
	RuleLockoutShadow = "acl.host-anti-lockout-shadow"
)

// Rule kinds (Rule.kind, HostAclRuleState.kind).
const (
	KindEstablished = "established"
	KindLoopback    = "loopback"
	KindICMP        = "icmp"
	KindAntiLockout = "anti-lockout"
	KindRule        = "rule"
	KindUnknown     = "unknown"
)

// DefaultManagementPorts are the anti-lockout ports when none are configured: SSH and HTTPS.
var DefaultManagementPorts = []uint32{22, 443}

var (
	// objectNameRe is packages/schema objectName: list and object names become nft identifiers.
	objectNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	// ifnameRe is packages/schema linuxInterfaceName (IFNAMSIZ 15).
	ifnameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)
)

var hookOrder = map[string]int{"input": 0, "output": 1, "forward": 2}

var hookPrefix = map[string]string{"input": "in", "output": "out", "forward": "fwd"}

// Build turns the host firewall configuration into the descriptor value: the configuration it came from,
// the named sets and the base chains with their rules. It is pure (no I/O) and deterministic. It returns
// nil when the document configures no host firewall at all (no host list, attachment or settings). Errors
// and warnings carry the pointer of the offending leaf; with an error the value must not be applied.
func Build(in Input) (*HostTable, []Issue) {
	acl := in.Acl
	if len(acl.GetHost()) == 0 && len(acl.GetHostAttachments()) == 0 && acl.GetHostSettings() == nil {
		return nil, nil
	}
	b := &builder{in: in, sets: map[string]*Set{}, expanded: map[string]*expansion{}}
	b.settings()
	lists := b.lists()
	var chains []*chainPlan
	seen := map[string]int{}
	for i, a := range acl.GetHostAttachments() {
		pt := ptr("acl", "hostAttachments", strconv.Itoa(i))
		if a.Enabled != nil && !a.GetEnabled() {
			continue
		}
		hook := a.GetChain()
		if _, ok := hookOrder[hook]; !ok {
			b.errorf(pt+"/chain", RuleAttachment, "chain %q is not input, output or forward", hook)
			continue
		}
		if p := a.GetPriority(); p < -500 || p > 500 {
			b.errorf(pt+"/priority", RuleAttachment, "priority %d is outside -500…500", p)
			continue
		}
		rules, ok := lists[a.GetList()]
		if !ok {
			b.errorf(pt+"/list", RuleAttachment, "host access list %q does not exist", a.GetList())
			continue
		}
		key := hook + "|" + a.GetList()
		if first, dup := seen[key]; dup {
			b.errorf(pt+"/chain", RuleAttachment, "host access list %q is already attached to chain %q (attachment %d)", a.GetList(), hook, first)
			continue
		}
		seen[key] = i
		c := &chainPlan{name: hookPrefix[hook] + "_" + a.GetList(), hook: hook, priority: a.GetPriority(), policy: "accept", list: a.GetList()}
		if hook == "input" {
			c.policy = b.defaultInput
		}
		c.rules = append(c.rules, b.preamble(hook)...)
		for _, tmpl := range rules {
			nr := *tmpl
			nr.out = hook == "output" // the rule's interface is the outgoing one on the output hook
			c.rules = append(c.rules, &nr)
		}
		chains = append(chains, c)
	}
	sort.SliceStable(chains, func(i, j int) bool {
		a, c := chains[i], chains[j]
		if hookOrder[a.hook] != hookOrder[c.hook] {
			return hookOrder[a.hook] < hookOrder[c.hook]
		}
		if a.priority != c.priority {
			return a.priority < c.priority
		}
		return a.name < c.name
	})
	b.checkLockout(chains)

	v := &HostTable{Config: hostConfig(acl)}
	used := map[string]bool{}
	for _, c := range chains {
		ch := &Chain{Name: c.name, Hook: c.hook, Priority: c.priority, Policy: c.policy}
		for _, r := range c.rules {
			ch.Rules = append(ch.Rules, r.toRule())
			for _, s := range r.sets() {
				used[s] = true
			}
		}
		v.Chains = append(v.Chains, ch)
	}
	for _, name := range sortedKeys(b.sets) {
		if used[name] {
			v.Sets = append(v.Sets, b.sets[name])
		}
	}
	return v, b.issues
}

// hostConfig is the part of acl this renderer realises (the value's config, Retrieve's acl).
func hostConfig(acl *vrxv1.AclConfig) *vrxv1.AclConfig {
	src := &vrxv1.AclConfig{Host: acl.GetHost(), HostAttachments: acl.GetHostAttachments(), HostSettings: acl.GetHostSettings()}
	return proto.Clone(src).(*vrxv1.AclConfig)
}

type builder struct {
	in       Input
	issues   []Issue
	sets     map[string]*Set
	expanded map[string]*expansion

	defaultInput string
	allowICMP    bool
	lockout      lockoutPlan
}

func (b *builder) errorf(pointer, rule, format string, a ...any) {
	b.issues = append(b.issues, Issue{Pointer: pointer, Rule: rule, Message: fmt.Sprintf(format, a...)})
}

func (b *builder) warnf(pointer, rule, format string, a ...any) {
	b.issues = append(b.issues, Issue{Pointer: pointer, Rule: rule, Message: fmt.Sprintf(format, a...), Warning: true})
}

// lockoutPlan is the parsed acl.hostSettings.antiLockout.
type lockoutPlan struct {
	enabled    bool
	sources    []netip.Prefix // masked; empty = any
	interfaces []string       // empty = any
	ports      []uint16
}

// settings reads acl.hostSettings (defaults when unset).
func (b *builder) settings() {
	s := b.in.Acl.GetHostSettings()
	pt := ptr("acl", "hostSettings")
	b.defaultInput = "accept"
	switch d := s.GetDefaultInput(); d {
	case "", "accept", "drop":
		if d != "" {
			b.defaultInput = d
		}
	default:
		b.errorf(pt+"/defaultInput", RuleSettings, "default input policy %q is not accept or drop", d)
	}
	b.allowICMP = s == nil || s.AllowIcmp == nil || s.GetAllowIcmp()
	al := s.GetAntiLockout()
	b.lockout.enabled = al == nil || al.Enabled == nil || al.GetEnabled()
	for i, src := range al.GetSources() {
		p, err := netip.ParsePrefix(src)
		if err != nil {
			b.errorf(fmt.Sprintf("%s/antiLockout/sources/%d", pt, i), RuleSettings, "source %q is not a prefix", src)
			continue
		}
		b.lockout.sources = append(b.lockout.sources, p.Masked())
	}
	for i, ifn := range al.GetInterfaces() {
		if !ifnameRe.MatchString(ifn) {
			b.errorf(fmt.Sprintf("%s/antiLockout/interfaces/%d", pt, i), RuleSettings, "%q is not a Linux interface name", ifn)
			continue
		}
		b.lockout.interfaces = append(b.lockout.interfaces, ifn)
	}
	ports := al.GetPorts()
	if len(ports) == 0 {
		ports = DefaultManagementPorts
	}
	for i, p := range ports {
		if p < 1 || p > 65535 {
			b.errorf(fmt.Sprintf("%s/antiLockout/ports/%d", pt, i), RuleSettings, "port %d is outside 1–65535", p)
			continue
		}
		b.lockout.ports = append(b.lockout.ports, uint16(p))
	}
	sort.Slice(b.lockout.sources, func(i, j int) bool { return prefixLess(b.lockout.sources[i], b.lockout.sources[j]) })
	b.lockout.sources = uniqPrefixes(b.lockout.sources)
	sort.Strings(b.lockout.interfaces)
	b.lockout.interfaces = uniqStrings(b.lockout.interfaces)
	sort.Slice(b.lockout.ports, func(i, j int) bool { return b.lockout.ports[i] < b.lockout.ports[j] })
	b.lockout.ports = uniqPorts(b.lockout.ports)
}

// configRule is one enabled rule of a list with its expansion.
type configRule struct {
	list    string
	index   int
	pointer string
	rule    *vrxv1.HostRule
}

// lists validates every host list (attached or not) and returns, per list, the nftables rules of its
// enabled rules in sequence order — templates that each attachment copies for its hook.
func (b *builder) lists() map[string][]*nrule {
	out := map[string][]*nrule{}
	for _, name := range sortedKeys(b.in.Acl.GetHost()) {
		pt := ptr("acl", "host", name)
		if !objectNameRe.MatchString(name) {
			b.errorf(pt, RuleListName, "host access list name %q: letters, digits, `_`, `.` and `-` only (max 63)", name)
			continue
		}
		var rules []*configRule
		seq := map[uint32]int{}
		for i, r := range b.in.Acl.GetHost()[name].GetRules() {
			rp := fmt.Sprintf("%s/rules/%d", pt, i)
			if r.GetSequence() == 0 {
				b.errorf(rp+"/sequence", RuleRule, "sequence must be 1 or more")
				continue
			}
			if first, dup := seq[r.GetSequence()]; dup {
				b.errorf(rp+"/sequence", RuleRule, "sequence %d is already used by rule %d", r.GetSequence(), first)
				continue
			}
			seq[r.GetSequence()] = i
			if r.Enabled != nil && !r.GetEnabled() {
				continue
			}
			rules = append(rules, &configRule{list: name, index: i, pointer: rp, rule: r})
		}
		sort.SliceStable(rules, func(i, j int) bool { return rules[i].rule.GetSequence() < rules[j].rule.GetSequence() })
		var templates []*nrule
		for _, cr := range rules {
			templates = append(templates, b.chainRules(cr)...)
		}
		out[name] = templates
	}
	return out
}

// ---- rules -------------------------------------------------------------------------------------

// addrMatch is a source or destination match; the zero value matches any address. With on set it is
// literal prefixes (object "") or an address object's named sets (object = its name).
type addrMatch struct {
	on     bool
	object string         // "" for literal prefixes
	p4, p6 []netip.Prefix // elements (literal: the prefixes)
}

func (m addrMatch) has(family int) bool {
	if !m.on {
		return true
	}
	if family == 4 {
		return len(m.p4) > 0
	}
	return len(m.p6) > 0
}

// svcGroup is one protocol clause: proto 0 = any protocol.
type svcGroup struct {
	proto                 uint8
	sport, dport          []span // nil = any
	flagsMask, flagsValue uint8
	icmpType, icmpCode    *span // icmp/icmpv6; nil = any
}

type span struct{ first, last uint16 }

// family of a service group: 4 (icmp), 6 (icmpv6) or 0.
func (g svcGroup) family() int {
	switch g.proto {
	case objects.ProtoICMP:
		return 4
	case objects.ProtoICMP6:
		return 6
	}
	return 0
}

// nrule is one planned nftables rule.
type nrule struct {
	id       string // "<list>:<sequence>" or "@<kind>"
	n        int
	kind     string
	list     string
	seq      uint32
	pointer  string
	family   int  // 0 = no family-specific match, 4, 6
	nfproto  bool // family pinned without an address match: render `meta nfproto`
	ctEst    bool
	loopback bool     // iif "lo" (input) / oif "lo" (output)
	out      bool     // output hook: interfaces are oifname
	ifaces   []string // iifname/oifname; empty = any
	src, dst addrMatch
	svc      *svcGroup // nil = any
	nd       bool      // icmpv6 neighbour/router discovery types
	dports   []uint16  // anti-lockout: tcp dport set
	log      bool
	verdict  string
}

func (r *nrule) sets() []string {
	var out []string
	for _, m := range []addrMatch{r.src, r.dst} {
		if m.on && m.object != "" {
			if r.family == 4 {
				out = append(out, setName(4, m.object))
			} else if r.family == 6 {
				out = append(out, setName(6, m.object))
			}
		}
	}
	return out
}

func setName(family int, object string) string {
	if family == 4 {
		return "a4_" + object
	}
	return "a6_" + object
}

// text renders the rule without its comment.
func (r *nrule) text() string {
	var p []string
	if r.ctEst {
		p = append(p, "ct state established,related")
	}
	if r.loopback {
		if r.out {
			p = append(p, `oif "lo"`)
		} else {
			p = append(p, `iif "lo"`)
		}
	}
	if len(r.ifaces) > 0 {
		kw := "iifname"
		if r.out {
			kw = "oifname"
		}
		p = append(p, kw+" "+quotedSet(r.ifaces))
	}
	if r.nfproto {
		p = append(p, "meta nfproto "+map[int]string{4: "ipv4", 6: "ipv6"}[r.family])
	}
	p = append(p, r.addr("saddr", r.src)...)
	p = append(p, r.addr("daddr", r.dst)...)
	if r.svc != nil {
		p = append(p, r.svc.text()...)
	}
	if r.nd {
		p = append(p, "icmpv6 type { nd-router-solicit, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert }")
	}
	if len(r.dports) > 0 {
		var ss []span
		for _, d := range r.dports {
			ss = append(ss, span{d, d})
		}
		p = append(p, "tcp dport "+spansText(ss))
	}
	p = append(p, "counter")
	if r.log {
		p = append(p, fmt.Sprintf(`log prefix "vrx:%s:%d "`, r.list, r.seq))
	}
	p = append(p, r.verdict)
	return strings.Join(p, " ")
}

func (r *nrule) addr(field string, m addrMatch) []string {
	if !m.on {
		return nil
	}
	proto, list := "ip", m.p4
	if r.family == 6 {
		proto, list = "ip6", m.p6
	}
	if m.object != "" {
		return []string{fmt.Sprintf("%s %s @%s", proto, field, setName(r.family, m.object))}
	}
	return []string{fmt.Sprintf("%s %s %s", proto, field, prefixesText(list))}
}

func (g *svcGroup) text() []string {
	switch g.proto {
	case objects.ProtoAny:
		return nil
	case objects.ProtoICMP, objects.ProtoICMP6:
		name, l4 := "icmp", "icmp"
		if g.proto == objects.ProtoICMP6 {
			name, l4 = "icmpv6", "ipv6-icmp"
		}
		if g.icmpType == nil {
			return []string{"meta l4proto " + l4}
		}
		p := []string{name + " type " + spanText(*g.icmpType)}
		if g.icmpCode != nil {
			p = append(p, name+" code "+spanText(*g.icmpCode))
		}
		return p
	case objects.ProtoTCP, objects.ProtoUDP, objects.ProtoSCTP:
		name := map[uint8]string{objects.ProtoTCP: "tcp", objects.ProtoUDP: "udp", objects.ProtoSCTP: "sctp"}[g.proto]
		var p []string
		if g.sport != nil {
			p = append(p, name+" sport "+spansText(g.sport))
		}
		if g.dport != nil {
			p = append(p, name+" dport "+spansText(g.dport))
		}
		if g.flagsMask != 0 {
			p = append(p, fmt.Sprintf("tcp flags & %#02x == %#02x", g.flagsMask, g.flagsValue))
		}
		if len(p) == 0 {
			p = append(p, "meta l4proto "+name)
		}
		return p
	default:
		return []string{"meta l4proto " + strconv.Itoa(int(g.proto))}
	}
}

// prefixesText is one prefix, or an anonymous set `{ a, b }`.
func prefixesText(ps []netip.Prefix) string {
	if len(ps) == 1 {
		return ps[0].String()
	}
	return "{ " + strings.Join(prefixStrings(ps), ", ") + " }"
}

func spanText(s span) string {
	if s.first == s.last {
		return strconv.Itoa(int(s.first))
	}
	return fmt.Sprintf("%d-%d", s.first, s.last)
}

func spansText(ss []span) string {
	if len(ss) == 1 {
		return spanText(ss[0])
	}
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = spanText(s)
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func quotedSet(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = `"` + n + `"`
	}
	if len(q) == 1 {
		return q[0]
	}
	return "{ " + strings.Join(q, ", ") + " }"
}

// toRule is the value's Rule: its text, identity comment and annotations.
func (r *nrule) toRule() *Rule {
	t := r.text()
	sum := sha256.Sum256([]byte(t))
	return &Rule{
		Comment: fmt.Sprintf("vrx:%s/%d:%s", r.id, r.n, hex.EncodeToString(sum[:4])),
		Text:    t, Kind: r.kind, List: r.list, Sequence: r.seq, Pointer: r.pointer, Verdict: r.verdict,
	}
}

// preamble are the fixed rules at the top of every chain of hook: established/related, loopback, ICMP
// (input) and the anti-lockout rule (input).
func (b *builder) preamble(hook string) []*nrule {
	out := hook == "output"
	rules := []*nrule{{id: "@" + KindEstablished, kind: KindEstablished, ctEst: true, verdict: "accept", out: out}}
	if hook == "forward" {
		return rules
	}
	rules = append(rules, &nrule{id: "@" + KindLoopback, kind: KindLoopback, loopback: true, verdict: "accept", out: out})
	if hook != "input" {
		return rules
	}
	if b.allowICMP {
		rules = append(rules,
			&nrule{id: "@" + KindICMP, kind: KindICMP, svc: &svcGroup{proto: objects.ProtoICMP}, verdict: "accept"},
			&nrule{id: "@" + KindICMP, n: 1, kind: KindICMP, svc: &svcGroup{proto: objects.ProtoICMP6}, verdict: "accept"})
	} else {
		rules = append(rules, &nrule{id: "@" + KindICMP, kind: KindICMP, nd: true, verdict: "accept"})
	}
	return append(rules, b.antiLockoutRules()...)
}

// antiLockoutRules accept management TCP ports from the configured sources on the configured interfaces:
// one rule per source family (one rule without a source match when no source is configured).
func (b *builder) antiLockoutRules() []*nrule {
	if !b.lockout.enabled || len(b.lockout.ports) == 0 {
		return nil
	}
	mk := func(n, family int, src addrMatch) *nrule {
		return &nrule{id: "@" + KindAntiLockout, n: n, kind: KindAntiLockout, family: family, ifaces: b.lockout.interfaces, src: src, dports: b.lockout.ports, verdict: "accept"}
	}
	if len(b.lockout.sources) == 0 {
		return []*nrule{mk(0, 0, addrMatch{})}
	}
	var rules []*nrule
	for _, fam := range []int{4, 6} {
		var ps []netip.Prefix
		for _, p := range b.lockout.sources {
			if familyOf(p) == fam {
				ps = append(ps, p)
			}
		}
		if len(ps) == 0 {
			continue
		}
		m := addrMatch{on: true}
		if fam == 4 {
			m.p4 = ps
		} else {
			m.p6 = ps
		}
		rules = append(rules, mk(len(rules), fam, m))
	}
	return rules
}

// chainRules expands one configured rule into its nftables rules (for an input/forward chain; output
// chains flip `out`).
func (b *builder) chainRules(cr *configRule) []*nrule {
	r := cr.rule
	pt := cr.pointer
	verdict := r.GetAction()
	switch verdict {
	case "accept", "drop", "reject":
	default:
		b.errorf(pt+"/action", RuleRule, "action %q is not accept, drop or reject", verdict)
		return nil
	}
	var ifaces []string
	if r.Interface != nil {
		if !ifnameRe.MatchString(r.GetInterface()) {
			b.errorf(pt+"/interface", RuleRule, "%q is not a Linux interface name", r.GetInterface())
			return nil
		}
		ifaces = []string{r.GetInterface()}
	}
	fams := map[string][]int{"": {4, 6}, "any": {4, 6}, "ipv4": {4}, "ipv6": {6}}[r.GetIpVersion()]
	if fams == nil {
		b.errorf(pt+"/ipVersion", RuleRule, "ipVersion %q is not ipv4, ipv6 or any", r.GetIpVersion())
		return nil
	}
	src, ok1 := b.addr(r.GetSource(), pt+"/source")
	dst, ok2 := b.addr(r.GetDestination(), pt+"/destination")
	groups, ok3 := b.service(r.GetService(), pt+"/service")
	if !ok1 || !ok2 || !ok3 {
		return nil
	}
	base := nrule{id: fmt.Sprintf("%s:%d", cr.list, r.GetSequence()), kind: KindRule, list: cr.list, seq: r.GetSequence(),
		pointer: pt, ifaces: ifaces, src: src, dst: dst, log: r.GetLog(), verdict: verdict}
	var rules []*nrule
	add := func(family int, nfproto bool, g svcGroup) {
		nr := base
		nr.family, nr.nfproto, nr.n = family, nfproto, len(rules)
		if g.proto != objects.ProtoAny {
			gg := g
			nr.svc = &gg
		}
		rules = append(rules, &nr)
	}
	if !src.on && !dst.on {
		// No address match: one rule for both families unless ipVersion pins one.
		family, nfproto := 0, false
		if len(fams) == 1 {
			family, nfproto = fams[0], true
		}
		for _, g := range groups {
			if gf := g.family(); family != 0 && gf != 0 && gf != family {
				continue
			}
			add(family, nfproto, g)
		}
	} else {
		for _, f := range fams {
			if !src.has(f) || !dst.has(f) {
				continue
			}
			for _, g := range groups {
				if gf := g.family(); gf != 0 && gf != f {
					continue
				}
				add(f, false, g)
			}
		}
	}
	if len(rules) == 0 {
		b.warnf(pt, RuleEmpty, "the rule matches no packet (address families of its source, destination and service do not meet) and is not rendered")
	}
	return rules
}

// expansion is one address object expanded once per Build.
type expansion struct {
	v4, v6 []netip.Prefix
	err    error
}

func (b *builder) addr(m *vrxv1.AddressMatch, pt string) (addrMatch, bool) {
	switch m.GetKind() {
	case "", "any":
		return addrMatch{}, true
	case "prefix":
		p, err := netip.ParsePrefix(m.GetPrefix())
		if err != nil {
			b.errorf(pt+"/prefix", RuleRule, "%q is not a prefix", m.GetPrefix())
			return addrMatch{}, false
		}
		p = p.Masked()
		if p.Addr().Is4() {
			return addrMatch{on: true, p4: []netip.Prefix{p}}, true
		}
		return addrMatch{on: true, p6: []netip.Prefix{p}}, true
	case "object":
		name := m.GetName()
		if !objectNameRe.MatchString(name) {
			b.errorf(pt+"/name", RuleObject, "object name %q: letters, digits, `_`, `.` and `-` only", name)
			return addrMatch{}, false
		}
		e := b.expand(name, pt+"/name")
		if e.err != nil {
			return addrMatch{}, false
		}
		return addrMatch{on: true, object: name, p4: e.v4, p6: e.v6}, true
	default:
		b.errorf(pt+"/kind", RuleRule, "address match kind %q is not any, prefix or object", m.GetKind())
		return addrMatch{}, false
	}
}

func (b *builder) expand(name, pt string) *expansion {
	if e, ok := b.expanded[name]; ok {
		if e.err != nil {
			b.errorf(pt, RuleObject, "%v", e.err)
		}
		return e
	}
	e := &expansion{}
	b.expanded[name] = e
	if b.in.Objects == nil {
		e.err = fmt.Errorf("%w: %q (no objects document)", objects.ErrUnknownObject, name)
		b.errorf(pt, RuleObject, "%v", e.err)
		return e
	}
	var opts []objects.Option
	if b.in.FQDN != nil {
		opts = append(opts, objects.WithFQDN(b.in.FQDN))
	}
	a, err := objects.Expand(b.in.Objects, name, opts...)
	var le *objects.LimitError
	switch {
	case errors.As(err, &le):
		e.err = err
		b.errorf(pt, RuleLimit, "%v", err)
		return e
	case err != nil:
		e.err = err
		b.errorf(pt, RuleObject, "%v", err)
		return e
	}
	for _, u := range a.Unresolved {
		b.warnf(pt, RuleFQDN, "FQDN object %q has no address yet; it matches nothing until it resolves", u)
	}
	e.v4, e.v6 = sortedPrefixes(a.V4), sortedPrefixes(a.V6)
	b.sets[setName(4, name)] = &Set{Name: setName(4, name), Type: "ipv4_addr", Elements: prefixStrings(e.v4)}
	b.sets[setName(6, name)] = &Set{Name: setName(6, name), Type: "ipv6_addr", Elements: prefixStrings(e.v6)}
	return e
}

func (b *builder) service(m *vrxv1.ServiceMatch, pt string) ([]svcGroup, bool) {
	var specs []objects.PortSpec
	var err error
	switch m.GetKind() {
	case "", "any":
		return []svcGroup{{}}, true
	case "object":
		name := m.GetName()
		if !objectNameRe.MatchString(name) {
			b.errorf(pt+"/name", RuleObject, "object name %q: letters, digits, `_`, `.` and `-` only", name)
			return nil, false
		}
		if b.in.Objects == nil {
			b.errorf(pt+"/name", RuleObject, "%v: %q (no objects document)", objects.ErrUnknownObject, name)
			return nil, false
		}
		specs, err = objects.ExpandService(b.in.Objects, name)
		pt += "/name"
	case "inline":
		specs, err = objects.ExpandServiceSpec(m.GetSpec())
		pt += "/spec"
	default:
		b.errorf(pt+"/kind", RuleRule, "service match kind %q is not any, object or inline", m.GetKind())
		return nil, false
	}
	var le *objects.LimitError
	switch {
	case errors.As(err, &le):
		b.errorf(pt, RuleLimit, "%v", err)
		return nil, false
	case err != nil:
		b.errorf(pt, RuleObject, "%v", err)
		return nil, false
	}
	return groupSpecs(specs), true
}

// groupSpecs folds PortSpecs into protocol clauses: TCP/UDP/SCTP specs sharing protocol, source range and
// TCP flags become one clause with a destination-port set; ICMP specs stay one clause each; protocol
// "any" subsumes everything.
func groupSpecs(specs []objects.PortSpec) []svcGroup {
	type key struct {
		proto                 uint8
		sFirst, sLast         uint16
		flagsMask, flagsValue uint8
	}
	var order []key
	byKey := map[key]*svcGroup{}
	var out []svcGroup
	full := func(s span, max uint16) bool { return s.first == 0 && s.last == max }
	for _, p := range specs {
		switch p.Proto {
		case objects.ProtoAny:
			return []svcGroup{{}}
		case objects.ProtoICMP, objects.ProtoICMP6:
			g := svcGroup{proto: p.Proto}
			if t := (span{p.SrcPortFirst, p.SrcPortLast}); !full(t, 255) {
				g.icmpType = &t
				if c := (span{p.DstPortFirst, p.DstPortLast}); !full(c, 255) {
					g.icmpCode = &c
				}
			}
			out = append(out, g)
		case objects.ProtoTCP, objects.ProtoUDP, objects.ProtoSCTP:
			k := key{p.Proto, p.SrcPortFirst, p.SrcPortLast, p.TCPFlagsMask, p.TCPFlagsValue}
			g, ok := byKey[k]
			if !ok {
				g = &svcGroup{proto: p.Proto, flagsMask: p.TCPFlagsMask, flagsValue: p.TCPFlagsValue}
				if s := (span{p.SrcPortFirst, p.SrcPortLast}); !full(s, 65535) {
					g.sport = []span{s}
				}
				byKey[k] = g
				order = append(order, k)
			}
			d := span{p.DstPortFirst, p.DstPortLast}
			if full(d, 65535) {
				g.dport = []span{d} // marker: any; normalised below
			} else {
				g.dport = append(g.dport, d)
			}
		default:
			out = append(out, svcGroup{proto: p.Proto})
		}
	}
	for _, k := range order {
		g := byKey[k]
		g.dport = mergeSpans(g.dport)
		if len(g.dport) == 1 && full(g.dport[0], 65535) {
			g.dport = nil
		}
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].proto < out[j].proto })
	return out
}

// mergeSpans sorts and merges overlapping or adjacent spans.
func mergeSpans(ss []span) []span {
	sort.Slice(ss, func(i, j int) bool { return ss[i].first < ss[j].first || (ss[i].first == ss[j].first && ss[i].last < ss[j].last) })
	var out []span
	for _, s := range ss {
		if n := len(out); n > 0 && int(s.first) <= int(out[n-1].last)+1 {
			if s.last > out[n-1].last {
				out[n-1].last = s.last
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// ---- helpers -----------------------------------------------------------------------------------

// chainPlan is one base chain being built.
type chainPlan struct {
	name, hook, policy, list string
	priority                 int32
	rules                    []*nrule
}

func ptr(segs ...string) string {
	var sb strings.Builder
	for _, s := range segs {
		sb.WriteByte('/')
		sb.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(s))
	}
	return sb.String()
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func familyOf(p netip.Prefix) int {
	if p.Addr().Is4() {
		return 4
	}
	return 6
}

func prefixLess(a, b netip.Prefix) bool {
	if c := a.Addr().Compare(b.Addr()); c != 0 {
		return c < 0
	}
	return a.Bits() < b.Bits()
}

func sortedPrefixes(ps []netip.Prefix) []netip.Prefix {
	out := append([]netip.Prefix(nil), ps...)
	sort.Slice(out, func(i, j int) bool { return prefixLess(out[i], out[j]) })
	return uniqPrefixes(out)
}

func prefixStrings(ps []netip.Prefix) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}

func uniqPrefixes(ps []netip.Prefix) []netip.Prefix {
	var out []netip.Prefix
	for i, p := range ps {
		if i == 0 || p != ps[i-1] {
			out = append(out, p)
		}
	}
	return out
}

func uniqStrings(ss []string) []string {
	var out []string
	for i, s := range ss {
		if i == 0 || s != ss[i-1] {
			out = append(out, s)
		}
	}
	return out
}

func uniqPorts(ps []uint16) []uint16 {
	var out []uint16
	for i, p := range ps {
		if i == 0 || p != ps[i-1] {
			out = append(out, p)
		}
	}
	return out
}
