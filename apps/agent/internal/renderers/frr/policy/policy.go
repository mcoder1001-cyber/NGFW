// Package policy renders `routing.policy` (prefix lists and route maps, D-045/D-070) into FRR configuration: the
// `policy` section of the RF-1 framework (order OrderPolicy). BGP (P12) and the IGPs (F-ospf, F-isis-rip, …) refer to
// these objects by name; they never render their own filters. The canonical forms are FRR 10.7's `show running-config`
// (the framework's convergence check fails on anything else); see docs/agent/renderers/frr-bgp.md.
//
// Route-map `match community` and `match as-path` take a *list name* in FRR, while the document carries the value: the
// section generates one list per route-map entry (`bgp community-list standard vrx-<map>-<seq> seq 5 permit <c>`,
// `bgp as-path access-list vrx-<map>-<seq> seq 5 permit <regex>`), so a list never outlives its entry.
package policy

import (
	"cmp"
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
)

// OrderPolicy places the section after the routing protocols (FRR prints its filters after the `router` blocks).
const OrderPolicy = 850

// Name is the section name.
const Name = "policy"

func init() { frr.RegisterSection(Section{}) }

// Section is the frr.Section of `routing.policy`.
type Section struct{}

// Name implements frr.Section.
func (Section) Name() string { return Name }

// Order implements frr.Section.
func (Section) Order() int { return OrderPolicy }

// Render implements frr.Section.
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	return Render(rc.Desired.GetRouting().GetPolicy(), rc.MapInterface)
}

// nameRe is the document's objectName (primitives.ts) — also a safe FRR WORD.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// ObjectName validates a prefix-list / route-map name.
func ObjectName(kind, s string) (string, error) {
	if !nameRe.MatchString(s) {
		return "", fmt.Errorf("%w: %s name %q must match %s", renderers.ErrUnsafe, kind, s, nameRe)
	}
	return s, nil
}

var (
	communityRe = regexp.MustCompile(`^(?:(?:0|[1-9][0-9]{0,4}):(?:0|[1-9][0-9]{0,4})|internet|no-export|no-advertise|local-AS)$`)
	// asPathRe is the schema's alphabet (digits and regex metacharacters, D-049); "| " and a trailing "|" are refused by
	// AsPath because FRR's CLI pipe cuts the line there (RF-1 review H1).
	asPathRe = regexp.MustCompile(`^[0-9_.*+?^$()|\[\]{}, -]{1,255}$`)
)

// Community validates a standard community ("65000:100", each half 0–65535) or a well-known name.
func Community(s string) (string, error) {
	if !communityRe.MatchString(s) {
		return "", fmt.Errorf("%w: community %q", renderers.ErrUnsafe, s)
	}
	for _, part := range strings.Split(s, ":") {
		if n, err := strconv.Atoi(part); err == nil && n > 65535 {
			return "", fmt.Errorf("%w: community %q: numbers must be 0–65535", renderers.ErrUnsafe, s)
		}
	}
	return s, nil
}

// AsPath validates an as-path regular expression for `bgp as-path access-list … permit <regex>` (a LINE token).
func AsPath(s string) (string, error) {
	switch {
	case !asPathRe.MatchString(s):
		return "", fmt.Errorf("%w: as-path regex %q: digits and ^ $ _ . * + ? ( ) [ ] { } | , - and space only", renderers.ErrUnsafe, s)
	case strings.Contains(s, "| ") || strings.HasSuffix(s, "|"):
		return "", fmt.Errorf("%w: as-path regex %q: \"| \" or a trailing '|' is cut by FRR's CLI pipe; write the alternation without blanks", renderers.ErrUnsafe, s)
	case strings.TrimSpace(s) != s || strings.Contains(s, "  "):
		return "", fmt.Errorf("%w: as-path regex %q: no leading, trailing or doubled blanks (FRR re-joins the tokens)", renderers.ErrUnsafe, s)
	}
	return s, nil
}

// family is "ip" or "ipv6" (the FRR keyword) of a prefix list.
func family(pl *vrxv1.PrefixList) string {
	if pl.GetFamily() == "ipv6" {
		return "ipv6"
	}
	return "ip"
}

// Render returns the section's lines for pol (nil = nothing). mapIf maps `match interface` names (VPP → Linux).
func Render(pol *vrxv1.RoutingPolicy, mapIf frr.InterfaceMapper) ([]string, error) {
	if pol == nil {
		return nil, nil
	}
	var out []string
	pls := pol.GetPrefixLists()
	for _, name := range slices.Sorted(maps.Keys(pls)) {
		lines, err := prefixList(name, pls[name])
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	rms := pol.GetRouteMaps()
	var lists, blocks []string
	for _, name := range slices.Sorted(maps.Keys(rms)) {
		l, m, err := routeMap(name, rms[name], pls, mapIf)
		if err != nil {
			return nil, err
		}
		lists = append(lists, l...)
		blocks = append(blocks, m...)
	}
	out = append(out, lists...)
	return append(out, blocks...), nil
}

func prefixList(name string, pl *vrxv1.PrefixList) ([]string, error) {
	path := P("routing", "policy", "prefixLists", name)
	if _, err := ObjectName("prefix list", name); err != nil {
		return nil, Wrap(path, err)
	}
	fam := family(pl)
	var out []string
	if d := pl.GetDescription(); d != "" {
		desc, err := frr.Description(d)
		if err != nil {
			return nil, Wrap(path.At("description"), err)
		}
		out = append(out, fmt.Sprintf("%s prefix-list %s description %s", fam, name, desc))
	}
	rules := pl.GetRules()
	for _, i := range bySeq(len(rules), func(i int) uint32 { return rules[i].GetSeq() }) {
		line, err := prefixListRule(fam, name, rules[i])
		if err != nil {
			return nil, Wrap(path.At("rules").Index(i), err) // the document's index, not the sorted one
		}
		out = append(out, line)
	}
	return out, nil
}

func action(a string) (string, error) {
	if a != "permit" && a != "deny" {
		return "", fmt.Errorf("%w: action %q is not permit or deny", renderers.ErrUnsafe, a)
	}
	return a, nil
}

func prefixListRule(fam, name string, r *vrxv1.PrefixListRule) (string, error) {
	if r.GetSeq() == 0 {
		return "", fmt.Errorf("%w: seq must be 1–4294967295", frr.ErrInput)
	}
	act, err := action(r.GetAction())
	if err != nil {
		return "", err
	}
	p, err := netip.ParsePrefix(r.GetPrefix())
	if err != nil || p.Masked() != p {
		return "", fmt.Errorf("%w: prefix %q is not a network prefix", renderers.ErrUnsafe, r.GetPrefix())
	}
	if (fam == "ipv6") != p.Addr().Is6() {
		return "", fmt.Errorf("%w: prefix %s is not in the family of the list", frr.ErrInput, p)
	}
	maxLen := uint32(32) // prefix lengths are 0–128: no conversion can overflow
	if p.Addr().Is6() {
		maxLen = 128
	}
	bits := uint32(p.Bits()) //nolint:gosec // G115: 0–128
	line := fmt.Sprintf("%s prefix-list %s seq %d %s %s", fam, name, r.GetSeq(), act, p)
	if r.Ge != nil {
		if r.GetGe() <= bits || r.GetGe() > maxLen {
			return "", fmt.Errorf("%w: ge %d must be > %d and ≤ %d", frr.ErrInput, r.GetGe(), bits, maxLen)
		}
		line += fmt.Sprintf(" ge %d", r.GetGe())
	}
	if r.Le != nil {
		if r.GetLe() < bits || r.GetLe() > maxLen || (r.Ge != nil && r.GetGe() > r.GetLe()) {
			return "", fmt.Errorf("%w: le %d must be in %d..%d and ≥ ge", frr.ErrInput, r.GetLe(), bits, maxLen)
		}
		line += fmt.Sprintf(" le %d", r.GetLe())
	}
	return line, nil
}

// routeMap returns the generated community / as-path lists and the route-map blocks of one route map.
func routeMap(name string, rm *vrxv1.RouteMap, pls map[string]*vrxv1.PrefixList, mapIf frr.InterfaceMapper) (lists, blocks []string, err error) {
	path := P("routing", "policy", "routeMaps", name)
	if _, err := ObjectName("route map", name); err != nil {
		return nil, nil, Wrap(path, err)
	}
	entries := rm.GetEntries()
	order := bySeq(len(entries), func(i int) uint32 { return entries[i].GetSeq() })
	for k := 1; k < len(order); k++ {
		if entries[order[k]].GetSeq() == entries[order[k-1]].GetSeq() {
			return nil, nil, Errf(path.At("entries").Index(order[k]).At("seq"), "sequence %d is used twice", entries[order[k]].GetSeq())
		}
	}
	// The route map's description (FRR has none per map, only per entry) is not rendered.
	for _, i := range order {
		l, b, err := routeMapEntry(name, entries[i], pls, mapIf, path.At("entries").Index(i))
		if err != nil {
			return nil, nil, err
		}
		lists = append(lists, l...)
		blocks = append(blocks, b...)
	}
	return lists, blocks, nil
}

// GeneratedListName is the community-list / as-path access-list name generated for a route-map entry.
func GeneratedListName(routeMap string, seq uint32) string {
	return fmt.Sprintf("vrx-%s-%d", routeMap, seq)
}

func routeMapEntry(name string, e *vrxv1.RouteMapEntry, pls map[string]*vrxv1.PrefixList, mapIf frr.InterfaceMapper, ep Path) (lists, block []string, err error) {
	if e.GetSeq() == 0 || e.GetSeq() > 65535 {
		// FRR's route-map sequence is 1–65535 (the schema allows a uint32; routing.bgp-route-map-seq says so first)
		return nil, nil, Errf(ep.At("seq"), "seq %d: FRR route-map sequence numbers are 1–65535", e.GetSeq())
	}
	act, err := action(e.GetAction())
	if err != nil {
		return nil, nil, Wrap(ep.At("action"), err)
	}
	block = append(block, fmt.Sprintf("route-map %s %s %d", name, act, e.GetSeq()))
	if d := e.GetDescription(); d != "" {
		desc, err := frr.Description(d)
		if err != nil {
			return nil, nil, Wrap(ep.At("description"), err)
		}
		block = append(block, " description "+desc)
	}
	gen := GeneratedListName(name, e.GetSeq())
	m := e.GetMatch()
	if m == nil {
		m = &vrxv1.RouteMapMatch{}
	}
	plFamily := func(pl string) (string, error) {
		if _, err := ObjectName("prefix list", pl); err != nil {
			return "", err
		}
		if l, ok := pls[pl]; ok {
			return family(l), nil
		}
		return "", fmt.Errorf("%w: prefix list %q does not exist", frr.ErrInput, pl)
	}
	var matches []string
	if pl := m.GetPrefixList(); pl != "" {
		fam, err := plFamily(pl)
		if err != nil {
			return nil, nil, Wrap(ep.At("match", "prefixList"), err)
		}
		matches = append(matches, fmt.Sprintf(" match %s address prefix-list %s", fam, pl))
	}
	if pl := m.GetNextHopPrefixList(); pl != "" {
		fam, err := plFamily(pl)
		if err != nil {
			return nil, nil, Wrap(ep.At("match", "nextHopPrefixList"), err)
		}
		matches = append(matches, fmt.Sprintf(" match %s next-hop prefix-list %s", fam, pl))
	}
	if itf := m.GetInterface(); itf != "" {
		if mapIf == nil {
			mapIf = frr.NoMapper
		}
		linux, ok := mapIf(itf)
		if !ok {
			return nil, nil, Errf(ep.At("match", "interface"), "interface %q has no Linux interface for FRR (interfaces.%s.lcp)", itf, itf)
		}
		n, err := frr.IfName(linux)
		if err != nil {
			return nil, nil, Wrap(ep.At("match", "interface"), err)
		}
		matches = append(matches, " match interface "+n)
	}
	if c := m.GetCommunity(); c != "" {
		v, err := Community(c)
		if err != nil {
			return nil, nil, Wrap(ep.At("match", "community"), err)
		}
		lists = append(lists, fmt.Sprintf("bgp community-list standard %s seq 5 permit %s", gen, v))
		matches = append(matches, " match community "+gen)
	}
	if ap := m.GetAsPath(); ap != "" {
		v, err := AsPath(ap)
		if err != nil {
			return nil, nil, Wrap(ep.At("match", "asPath"), err)
		}
		lists = append(lists, fmt.Sprintf("bgp as-path access-list %s seq 5 permit %s", gen, v))
		matches = append(matches, " match as-path "+gen)
	}
	if m.Metric != nil {
		matches = append(matches, fmt.Sprintf(" match metric %d", m.GetMetric()))
	}
	if m.Tag != nil {
		matches = append(matches, fmt.Sprintf(" match tag %d", m.GetTag()))
	}
	sets, err := setLines(e.GetSet())
	if err != nil {
		return nil, nil, Wrap(ep.At("set"), err)
	}
	block = append(block, matches...)
	block = append(block, sets...)
	return lists, append(block, "exit"), nil
}

func setLines(s *vrxv1.RouteMapSet) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	var out []string
	if len(s.GetAsPathPrepend()) > 0 {
		asns := make([]string, 0, len(s.GetAsPathPrepend()))
		for _, a := range s.GetAsPathPrepend() {
			if a == 0 {
				return nil, fmt.Errorf("%w: set.asPathPrepend: AS 0 is reserved", frr.ErrInput)
			}
			asns = append(asns, strconv.FormatUint(uint64(a), 10))
		}
		out = append(out, " set as-path prepend "+strings.Join(asns, " "))
	}
	if cs := s.GetCommunity(); len(cs) > 0 {
		vals := make([]string, 0, len(cs))
		for _, c := range cs {
			v, err := Community(c)
			if err != nil {
				return nil, fmt.Errorf("set.community: %w", err)
			}
			vals = append(vals, v)
		}
		line := " set community " + strings.Join(vals, " ")
		if s.GetCommunityAdditive() {
			line += " additive"
		}
		out = append(out, line)
	}
	if nh := s.GetNextHop(); nh != "" {
		a, err := netip.ParseAddr(nh)
		if err != nil || a.Zone() != "" || a.IsUnspecified() || a.IsMulticast() {
			return nil, fmt.Errorf("%w: set.nextHop %q is not a unicast address", renderers.ErrUnsafe, nh)
		}
		if a.Is4() {
			out = append(out, " set ip next-hop "+a.String())
		} else {
			out = append(out, " set ipv6 next-hop global "+a.String())
		}
	}
	if s.LocalPref != nil {
		out = append(out, fmt.Sprintf(" set local-preference %d", s.GetLocalPref()))
	}
	if s.Med != nil {
		out = append(out, fmt.Sprintf(" set metric %d", s.GetMed()))
	}
	if s.Tag != nil {
		out = append(out, fmt.Sprintf(" set tag %d", s.GetTag()))
	}
	if s.Weight != nil {
		if s.GetWeight() > 65535 {
			return nil, fmt.Errorf("%w: set.weight %d not in 0–65535", frr.ErrInput, s.GetWeight())
		}
		out = append(out, fmt.Sprintf(" set weight %d", s.GetWeight()))
	}
	return out, nil
}

// bySeq returns the indexes 0..n-1 sorted by seq (stable): render order, while errors keep the document's index.
func bySeq(n int, seq func(int) uint32) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(seq(a), seq(b)) })
	return idx
}
