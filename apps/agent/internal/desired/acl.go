package desired

// F-acl: builder and assembler of the `acl` domain (VPP acl plugin, DF-4 descriptors).
//
//	acl.lists.<name>             → acl.acl/<name>                     one VPP ACL; every enabled rule whose
//	                                                                    schedule is active expands (objects) to
//	                                                                    sources × destinations × services per family
//	acl.macip.<name>             → acl.macip-acl/<name>
//	acl.attachments[]            → acl.interface-binding/<ifname>     per interface (a zone → each member), in/out
//	                                                                    lists ordered by attachment sequence
//	acl.macipAttachments[]       → acl.macip-interface-binding/<ifname>
//	(globals owner only, D-071)  → acl.stats-enable/global            the per-rule hit counters
//
// Objects are expanded from the REQUEST's `objects` (docs/agent/objects.md, review F3); a rule that
// names an object in a transaction without `objects` is an error (acl.objects-required). FQDN answers
// come from the agent's resolver. Each list's expansion is recorded (internal/actions/acl Record) so
// VPP rule counters map back to configuration rules and Retrieve can name the configuration that
// produced what VPP holds. acl.host / acl.hostAttachments are F-host-acl-nftables' leaves: reported
// as agent.unsupported-field here.

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// MaxListRules is the most VPP rules one list may expand to (WBS D5.5: the editor's 100 000).
const MaxListRules = 100_000

// ACLEnv is what the acl projection needs beyond the document. subsystems/acl.go sets it when the
// acl family registers (one agent per process; questions Q5).
type ACLEnv struct {
	// Owner is the agent's owner (VPP tags "<owner>:<name>" must fit in 63 bytes).
	Owner string
	// FQDN answers FQDN address objects (nil: every FQDN object is unresolved).
	FQDN objects.FQDNLookup
	// GlobalsOwner (D-071) projects acl.stats-enable.
	GlobalsOwner bool
	// Now and Location evaluate schedules (Location: the box's zone, questions Q4).
	Now      func() time.Time
	Location *time.Location
	// Record receives every expansion (nil: aclstate.Default).
	Record *aclstate.Record
}

var aclEnv atomic.Pointer[ACLEnv]

// SetACLEnv installs the environment of the acl projection.
func SetACLEnv(e ACLEnv) {
	aclEnv.Store(withACLDefaults(e))
}

func withACLDefaults(e ACLEnv) *ACLEnv {
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.Location == nil {
		e.Location = time.Local
	}
	if e.Record == nil {
		e.Record = aclstate.Default
	}
	return &e
}

// CurrentACLEnv returns the installed environment (defaults when none).
func CurrentACLEnv() ACLEnv {
	if p := aclEnv.Load(); p != nil {
		return *p
	}
	return *withACLDefaults(ACLEnv{})
}

// ACL emits the acl.* objects of ds.acl; in says which domains the transaction carries.
func ACL(s Sink, ds *vrxv1.DesiredState, in map[string]bool) {
	env := CurrentACLEnv()
	if env.GlobalsOwner {
		s.Add(descacl.KeyStatsEnable, descacl.StatsEnable{Enabled: true}.Proto(), Ptr("acl"))
	}
	cfg := ds.GetAcl()
	if len(cfg.GetHost()) > 0 {
		s.Warnf(Ptr("acl", "host"), "agent.unsupported-field", "host ACLs are rendered by F-host-acl-nftables (nftables), not by this agent build")
	}
	if len(cfg.GetHostAttachments()) > 0 {
		s.Warnf(Ptr("acl", "hostAttachments"), "agent.unsupported-field", "host ACL attachments are rendered by F-host-acl-nftables (nftables), not by this agent build")
	}
	x := &aclExpander{
		env: env, s: s, objs: ds.GetObjects(), haveObjects: in["objects"],
		addrs: map[string]addrSide{}, svcs: map[string][]objects.PortSpec{}, fqdn: map[string][]string{},
	}
	for _, name := range sortedKeys(cfg.GetLists()) {
		x.list(name, cfg.GetLists()[name])
	}
	for _, name := range sortedKeys(cfg.GetMacip()) {
		x.macip(name, cfg.GetMacip()[name])
	}
	x.bindings(cfg)
}

type aclExpander struct {
	env         ACLEnv
	s           Sink
	objs        *vrxv1.ObjectsConfig
	haveObjects bool
	addrs       map[string]addrSide
	addrErr     map[string]error
	svcs        map[string][]objects.PortSpec
	svcErr      map[string]error
	fqdn        map[string][]string
}

// addrSide is one side (source or destination) of a rule, per family.
type addrSide struct {
	v4, v6     []string
	unresolved []string
	fqdn       []string
}

var anySide = addrSide{v4: []string{descacl.AnyV4}, v6: []string{descacl.AnyV6}}

var anyService = []objects.PortSpec{{Proto: objects.ProtoAny, SrcPortLast: 65535, DstPortLast: 65535}}

func (x *aclExpander) needObjects(pt, what string) bool {
	if x.haveObjects {
		return true
	}
	x.s.Errorf(pt, "acl.objects-required", "%s names an object, but this transaction carries no objects domain (send objects with acl)", what)
	return false
}

func prefixStrings(ps []netip.Prefix) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}

// fqdnRefs lists the FQDN address objects ref reaches (itself, or through groups), sorted.
func (x *aclExpander) fqdnRefs(ref string) []string {
	if v, ok := x.fqdn[ref]; ok {
		return v
	}
	seen := map[string]bool{}
	var out []string
	var walk func(string)
	walk = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true
		if a, ok := x.objs.GetAddresses()[n]; ok {
			if a.GetType() == "fqdn" {
				out = append(out, n)
			}
			return
		}
		for _, m := range x.objs.GetAddressGroups()[n].GetMembers() {
			walk(m)
		}
	}
	walk(ref)
	sort.Strings(out)
	x.fqdn[ref] = out
	return out
}

func (x *aclExpander) side(m *vrxv1.AddressMatch, pt string) (addrSide, bool) {
	switch m.GetKind() {
	case "", "any":
		return anySide, true
	case "prefix":
		p, err := netip.ParsePrefix(m.GetPrefix())
		if err != nil {
			x.s.Errorf(pt+"/prefix", "acl.rule-prefix", "%q is not a CIDR prefix: %v", m.GetPrefix(), err)
			return addrSide{}, false
		}
		c := p.Masked().String()
		if p.Addr().Is4() {
			return addrSide{v4: []string{c}}, true
		}
		return addrSide{v6: []string{c}}, true
	case "object":
		name := m.GetName()
		if !x.needObjects(pt+"/name", fmt.Sprintf("address object %q", name)) {
			return addrSide{}, false
		}
		if err, bad := x.addrErr[name]; bad {
			x.reportObjectErr(pt+"/name", err)
			return addrSide{}, false
		}
		if v, ok := x.addrs[name]; ok {
			return v, true
		}
		var opts []objects.Option
		if x.env.FQDN != nil {
			opts = append(opts, objects.WithFQDN(x.env.FQDN))
		}
		a, err := objects.Expand(x.objs, name, opts...)
		if err != nil {
			if x.addrErr == nil {
				x.addrErr = map[string]error{}
			}
			x.addrErr[name] = err
			x.reportObjectErr(pt+"/name", err)
			return addrSide{}, false
		}
		v := addrSide{v4: prefixStrings(a.V4), v6: prefixStrings(a.V6), unresolved: a.Unresolved, fqdn: x.fqdnRefs(name)}
		x.addrs[name] = v
		return v, true
	default:
		x.s.Errorf(pt+"/kind", "acl.rule-match", "unknown address match kind %q", m.GetKind())
		return addrSide{}, false
	}
}

func (x *aclExpander) reportObjectErr(pt string, err error) {
	var le *objects.LimitError
	if errors.As(err, &le) {
		x.s.Errorf(pt, "acl.expansion-limit", "%v", err)
		return
	}
	x.s.Errorf(pt, "acl.object", "%v", err)
}

func (x *aclExpander) service(m *vrxv1.ServiceMatch, pt string) ([]objects.PortSpec, bool) {
	switch m.GetKind() {
	case "", "any":
		return anyService, true
	case "inline":
		ps, err := objects.ExpandServiceSpec(m.GetSpec())
		if err != nil {
			x.s.Errorf(pt+"/spec", "acl.service", "%v", err)
			return nil, false
		}
		return ps, true
	case "object":
		name := m.GetName()
		if !x.needObjects(pt+"/name", fmt.Sprintf("service object %q", name)) {
			return nil, false
		}
		if err, bad := x.svcErr[name]; bad {
			x.reportObjectErr(pt+"/name", err)
			return nil, false
		}
		if v, ok := x.svcs[name]; ok {
			return v, true
		}
		ps, err := objects.ExpandService(x.objs, name)
		if err != nil {
			if x.svcErr == nil {
				x.svcErr = map[string]error{}
			}
			x.svcErr[name] = err
			x.reportObjectErr(pt+"/name", err)
			return nil, false
		}
		x.svcs[name] = ps
		return ps, true
	default:
		x.s.Errorf(pt+"/kind", "acl.rule-match", "unknown service match kind %q", m.GetKind())
		return nil, false
	}
}

// servicesFor keeps the port specs that exist in family v6 (ICMP only in IPv4, ICMPv6 only in IPv6).
func servicesFor(ps []objects.PortSpec, v6 bool) []objects.PortSpec {
	out := make([]objects.PortSpec, 0, len(ps))
	for _, p := range ps {
		if (p.Proto == objects.ProtoICMP && v6) || (p.Proto == objects.ProtoICMP6 && !v6) {
			continue
		}
		out = append(out, p)
	}
	return out
}

var aclActions = map[string]descacl.Action{"permit": descacl.ActionPermit, "deny": descacl.ActionDeny, "reflect": descacl.ActionReflect}

type indexedRule struct {
	i int
	r *vrxv1.AclRule
}

func (x *aclExpander) list(name string, l *vrxv1.AclList) {
	lp := Ptr("acl", "lists", name)
	if _, err := vpp.OwnerTag(x.env.Owner, name); x.env.Owner != "" && err != nil {
		x.s.Errorf(lp, "acl.name", "list name %q: %v", name, err)
		return
	}
	rules := make([]indexedRule, len(l.GetRules()))
	for i, r := range l.GetRules() {
		rules[i] = indexedRule{i, r}
	}
	sort.SliceStable(rules, func(a, b int) bool { return rules[a].r.GetSequence() < rules[b].r.GetSequence() })
	exp := &aclstate.Expansion{Name: name, Rules: make([]aclstate.RuleInfo, 0, len(rules))}
	rulesPtr := Ptr("acl", "lists", name, "rules") + "/" // Ptr builds a replacer per call: once per list
	var out []descacl.Rule
	var logs int
	var logPtr string
	failed, tooMany := false, false
	for k, ir := range rules {
		r := ir.r
		rp := rulesPtr + strconv.Itoa(ir.i)
		if k > 0 && rules[k-1].r.GetSequence() == r.GetSequence() {
			x.s.Errorf(rp+"/sequence", "acl.rule-sequences-unique", "sequence %d is used twice", r.GetSequence())
			failed = true
			continue
		}
		info := aclstate.RuleInfo{Sequence: r.GetSequence(), Schedule: r.GetSchedule(), First: uint32(min(len(out), MaxListRules))} //nolint:gosec // bounded
		if r.Enabled != nil && !r.GetEnabled() {
			info.Status = vrxv1.AclRuleStatus_ACL_RULE_STATUS_DISABLED
			exp.Rules = append(exp.Rules, info)
			continue
		}
		if r.GetLog() {
			if logs == 0 {
				logPtr = rp + "/log"
			}
			logs++
		}
		action, ok := aclActions[r.GetAction()]
		if !ok {
			x.s.Errorf(rp+"/action", "acl.rule-action", "unknown action %q", r.GetAction())
			failed = true
			continue
		}
		if sname := r.GetSchedule(); sname != "" {
			if !x.needObjects(rp+"/schedule", fmt.Sprintf("schedule %q", sname)) {
				failed = true
				continue
			}
			def, ok := x.objs.GetSchedules()[sname]
			if !ok {
				x.s.Errorf(rp+"/schedule", "acl.rule-references", "schedule %q does not exist in objects.schedules", sname)
				failed = true
				continue
			}
			if exp.Schedules == nil {
				exp.Schedules = map[string]*vrxv1.Schedule{}
			}
			exp.Schedules[sname] = def
			on, err := objects.Active(def, x.env.Now(), x.env.Location)
			if err != nil {
				x.s.Errorf(rp+"/schedule", "acl.schedule", "schedule %q: %v", sname, err)
				failed = true
				continue
			}
			if !on {
				info.Status = vrxv1.AclRuleStatus_ACL_RULE_STATUS_SCHEDULE_INACTIVE
				exp.Rules = append(exp.Rules, info)
				continue
			}
		}
		src, ok1 := x.side(r.GetSource(), rp+"/source")
		dst, ok2 := x.side(r.GetDestination(), rp+"/destination")
		svc, ok3 := x.service(r.GetService(), rp+"/service")
		if !ok1 || !ok2 || !ok3 {
			failed = true
			continue
		}
		v4, v6 := true, true
		switch r.GetIpVersion() {
		case "ipv4":
			v6 = false
		case "ipv6":
			v4 = false
		}
		type fam struct {
			src, dst []string
			svc      []objects.PortSpec
		}
		var fams []fam
		if v4 {
			fams = append(fams, fam{src.v4, dst.v4, servicesFor(svc, false)})
		}
		if v6 {
			fams = append(fams, fam{src.v6, dst.v6, servicesFor(svc, true)})
		}
		n := 0
		for _, f := range fams {
			n += len(f.src) * len(f.dst) * len(f.svc)
		}
		if err := objects.CheckLimit(rp, n); err != nil {
			x.s.Errorf(rp, "acl.expansion-limit", "rule %d expands to %d VPP rules, more than %d (sources × destinations × services)", r.GetSequence(), n, objects.MaxEntries)
			failed = true
			continue
		}
		if len(out)+n > MaxListRules {
			tooMany = true
		}
		if !tooMany {
			for _, f := range fams {
				for _, s := range f.src {
					for _, d := range f.dst {
						for _, p := range f.svc {
							out = append(out, descacl.Rule{
								Action: action, Src: s, Dst: d, Proto: p.Proto,
								SrcPortFirst: p.SrcPortFirst, SrcPortLast: p.SrcPortLast,
								DstPortFirst: p.DstPortFirst, DstPortLast: p.DstPortLast,
								TCPFlagsMask: p.TCPFlagsMask, TCPFlagsValue: p.TCPFlagsValue,
							})
						}
					}
				}
			}
		}
		info.Count = uint32(n) //nolint:gosec // ≤ objects.MaxEntries
		info.Status = vrxv1.AclRuleStatus_ACL_RULE_STATUS_APPLIED
		if n == 0 {
			info.Status = vrxv1.AclRuleStatus_ACL_RULE_STATUS_EMPTY
			info.Count = 0
		}
		info.FQDN = unionSorted(src.fqdn, dst.fqdn)
		if un := unionSorted(src.unresolved, dst.unresolved); len(un) > 0 {
			x.s.Warnf(rp, "acl.fqdn-unresolved", "FQDN object(s) %v have no address yet; they match nothing in rule %d", un, r.GetSequence())
		}
		exp.Rules = append(exp.Rules, info)
	}
	if logs > 0 {
		x.s.Warnf(logPtr, "acl.log-unsupported", "%d rule(s) of list %q ask for logging; VPP's acl plugin cannot log per rule — matches are counted (hit counters), not logged", logs, name)
	}
	if tooMany {
		total := 0
		for _, ri := range exp.Rules {
			total += int(ri.Count)
		}
		x.s.Errorf(lp, "acl.list-limit", "list %q expands to %d VPP rules, more than %d", name, total, MaxListRules)
		return
	}
	if failed {
		return
	}
	exp.VPPRules = len(out)
	exp.Fingerprint = aclstate.Fingerprint(out)
	exp.ConfigHash = aclstate.ConfigHash(l)
	x.env.Record.PutACL(exp) // every projection records; readers use the APPLIED configuration's entry (H1)
	x.s.Add(descacl.KeyACL(name), descacl.ACL{Name: name, Rules: out}.Proto(), lp)
	x.s.Add(aclstate.KeyConfigList(name), aclstate.ConfigList(name, l), lp)
}

func unionSorted(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range [][]string{a, b} {
		for _, s := range l {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

func canonMAC(s string) (string, error) {
	hw, err := net.ParseMAC(s)
	if err != nil {
		return "", err
	}
	if len(hw) != 6 {
		return "", fmt.Errorf("%q is not a 48-bit MAC", s)
	}
	return hw.String(), nil
}

func (x *aclExpander) macip(name string, l *vrxv1.MacipList) {
	lp := Ptr("acl", "macip", name)
	if _, err := vpp.OwnerTag(x.env.Owner, name); x.env.Owner != "" && err != nil {
		x.s.Errorf(lp, "acl.name", "MACIP list name %q: %v", name, err)
		return
	}
	idx := make([]int, len(l.GetRules()))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return l.GetRules()[idx[a]].GetSequence() < l.GetRules()[idx[b]].GetSequence() })
	var out []descacl.MacipRule
	failed := false
	for _, i := range idx {
		r := l.GetRules()[i]
		rp := Ptr("acl", "macip", name, "rules", strconv.Itoa(i))
		var action descacl.Action
		switch r.GetAction() {
		case "permit":
			action = descacl.ActionPermit
		case "deny":
			action = descacl.ActionDeny
		default:
			x.s.Errorf(rp+"/action", "acl.macip-rules", "unknown MACIP action %q", r.GetAction())
			failed = true
			continue
		}
		mac, err := canonMAC(r.GetSourceMac())
		if err != nil {
			x.s.Errorf(rp+"/sourceMac", "acl.macip-rules", "%v", err)
			failed = true
			continue
		}
		maskText := r.GetSourceMacMask()
		if maskText == "" {
			maskText = "ff:ff:ff:ff:ff:ff"
		}
		mask, err := canonMAC(maskText)
		if err != nil {
			x.s.Errorf(rp+"/sourceMacMask", "acl.macip-rules", "%v", err)
			failed = true
			continue
		}
		prefixes := []string{descacl.AnyV4, descacl.AnyV6} // no source prefix = any address of either family
		if r.SourcePrefix != nil {
			p, err := netip.ParsePrefix(r.GetSourcePrefix())
			if err != nil {
				x.s.Errorf(rp+"/sourcePrefix", "acl.macip-rules", "%q is not a CIDR prefix: %v", r.GetSourcePrefix(), err)
				failed = true
				continue
			}
			prefixes = []string{p.Masked().String()}
		}
		for _, p := range prefixes {
			out = append(out, descacl.MacipRule{Action: action, SrcMac: mac, SrcMacMask: mask, SrcPrefix: p})
		}
	}
	if failed {
		return
	}
	x.env.Record.PutMacip(&aclstate.MacipExpansion{Name: name, Fingerprint: aclstate.MacipFingerprint(out), ConfigHash: aclstate.ConfigHash(l)})
	x.s.Add(descacl.KeyMacipACL(name), descacl.MacipACL{Name: name, Rules: out}.Proto(), lp)
	x.s.Add(aclstate.KeyConfigMacip(name), aclstate.ConfigMacip(name, l), lp)
}

type bindEntry struct {
	seq  uint32
	idx  int
	list string
}

type ifBinding struct {
	pointer string
	in, out []bindEntry
}

func (x *aclExpander) bindings(cfg *vrxv1.AclConfig) {
	binds := map[string]*ifBinding{}
	for i, a := range cfg.GetAttachments() {
		ap := Ptr("acl", "attachments", strconv.Itoa(i))
		if a.Enabled != nil && !a.GetEnabled() {
			continue
		}
		list := a.GetList()
		if _, ok := cfg.GetLists()[list]; !ok {
			x.s.Errorf(ap+"/list", "acl.attachments", "access list %q does not exist", list)
			continue
		}
		var ifs []string
		switch t := a.GetTarget(); t.GetKind() {
		case "interface":
			ifs = []string{t.GetInterface()}
		case "zone":
			if !x.needObjects(ap+"/target/zone", fmt.Sprintf("zone %q", t.GetZone())) {
				continue
			}
			z, err := objects.ZoneInterfaces(x.objs, t.GetZone())
			if err != nil {
				x.s.Errorf(ap+"/target/zone", "acl.attachments", "%v", err)
				continue
			}
			ifs = z
		default:
			x.s.Errorf(ap+"/target/kind", "acl.attachments", "unknown attachment target kind %q", t.GetKind())
			continue
		}
		in := true
		switch a.GetDirection() {
		case "", "in":
		case "out":
			in = false
		default:
			x.s.Errorf(ap+"/direction", "acl.attachments", "unknown direction %q", a.GetDirection())
			continue
		}
		for _, ifn := range ifs {
			b, ok := binds[ifn]
			if !ok {
				b = &ifBinding{pointer: ap}
				binds[ifn] = b
			}
			e := bindEntry{seq: a.GetSequence(), idx: i, list: list}
			if in {
				b.in = append(b.in, e)
			} else {
				b.out = append(b.out, e)
			}
		}
	}
	var all []descacl.InterfaceBinding
	for _, ifn := range sortedKeys(binds) {
		b := binds[ifn]
		v := descacl.InterfaceBinding{Interface: ifn}
		ok := true
		v.Input, ok = x.ordered(b.in, ifn, "in", ok)
		v.Output, ok = x.ordered(b.out, ifn, "out", ok)
		if !ok {
			continue
		}
		all = append(all, v)
		x.s.Add(descacl.KeyInterfaceBinding(ifn), v.Proto(), b.pointer)
	}
	var macips []descacl.MacipBinding
	seen := map[string]int{}
	for i, a := range cfg.GetMacipAttachments() {
		ap := Ptr("acl", "macipAttachments", strconv.Itoa(i))
		if a.Enabled != nil && !a.GetEnabled() {
			continue
		}
		if _, ok := cfg.GetMacip()[a.GetList()]; !ok {
			x.s.Errorf(ap+"/list", "acl.macip-attachments", "MACIP access list %q does not exist", a.GetList())
			continue
		}
		if first, dup := seen[a.GetInterface()]; dup {
			x.s.Errorf(ap+"/interface", "acl.macip-attachments", "interface %q already has a MACIP access list (attachment %d; VPP allows one per interface)", a.GetInterface(), first)
			continue
		}
		seen[a.GetInterface()] = i
		v := descacl.MacipBinding{Interface: a.GetInterface(), ACL: a.GetList()}
		macips = append(macips, v)
		x.s.Add(descacl.KeyMacipBinding(a.GetInterface()), v.Proto(), ap)
	}
	if len(cfg.GetAttachments())+len(cfg.GetMacipAttachments()) == 0 {
		return
	}
	att := aclstate.ConfigAttachments(cfg.GetAttachments(), cfg.GetMacipAttachments())
	x.env.Record.PutAttachments(&aclstate.Attachments{Fingerprint: aclstate.BindingsFingerprint(all, macips), ConfigHash: aclstate.ConfigHash(att)})
	x.s.Add(aclstate.KeyConfigAttachments, att, Ptr("acl", "attachments"))
}

// ordered sorts a direction's entries by attachment sequence (then position) and rejects a list
// named twice (VPP: ENTRY_ALREADY_EXISTS).
func (x *aclExpander) ordered(es []bindEntry, ifn, dir string, ok bool) ([]string, bool) {
	sort.SliceStable(es, func(a, b int) bool {
		if es[a].seq != es[b].seq {
			return es[a].seq < es[b].seq
		}
		return es[a].idx < es[b].idx
	})
	var names []string
	seen := map[string]bool{}
	for _, e := range es {
		if seen[e.list] {
			x.s.Errorf(Ptr("acl", "attachments", strconv.Itoa(e.idx), "list"), "acl.attachments", "access list %q is attached twice to interface %q in direction %q", e.list, ifn, dir)
			ok = false
			continue
		}
		seen[e.list] = true
		names = append(names, e.list)
	}
	return names, ok
}

// ---- assemble ---------------------------------------------------------------------------------------

// AssembleACL builds the `acl` domain from retrieved objects (nil when this owner has none). A list
// whose VPP rules are exactly what the APPLIED configuration of that list produced (acl.config, the
// record entry of its fingerprint and configuration hash) is reported as that configuration;
// anything else is reconstructed from the VPP rules (one rule per VPP rule, sequences 10, 20, …), so a
// difference shows as drift. A DryRun never changes the result (review H1).
func AssembleACL(kvs []scheduler.KV) *vrxv1.AclConfig {
	env := CurrentACLEnv()
	out := &vrxv1.AclConfig{}
	applied := map[scheduler.Key]*vrxv1.AclConfig{}
	for _, kv := range kvs {
		if kv.Key.Descriptor() == aclstate.NameConfig {
			if c, ok := kv.Value.(*vrxv1.AclConfig); ok {
				applied[kv.Key] = c
			}
		}
	}
	var binds []descacl.InterfaceBinding
	var macips []descacl.MacipBinding
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case descacl.NameACL:
			a, err := descacl.FromProto(kv.Value)
			if err != nil || isDupName(a.Name) {
				continue
			}
			if out.Lists == nil {
				out.Lists = map[string]*vrxv1.AclList{}
			}
			cfg := applied[aclstate.KeyConfigList(a.Name)].GetLists()[a.Name]
			if cfg != nil {
				if _, ok := env.Record.ACL(a.Name, aclstate.Fingerprint(a.Rules), aclstate.ConfigHash(cfg)); ok {
					out.Lists[a.Name] = proto.Clone(cfg).(*vrxv1.AclList)
					continue
				}
			}
			out.Lists[a.Name] = reconstructList(a)
		case descacl.NameMacipACL:
			a, err := descacl.MacipACLFromProto(kv.Value)
			if err != nil || isDupName(a.Name) {
				continue
			}
			if out.Macip == nil {
				out.Macip = map[string]*vrxv1.MacipList{}
			}
			cfg := applied[aclstate.KeyConfigMacip(a.Name)].GetMacip()[a.Name]
			if cfg != nil {
				if _, ok := env.Record.Macip(a.Name, aclstate.MacipFingerprint(a.Rules), aclstate.ConfigHash(cfg)); ok {
					out.Macip[a.Name] = proto.Clone(cfg).(*vrxv1.MacipList)
					continue
				}
			}
			out.Macip[a.Name] = reconstructMacip(a)
		case descacl.NameInterfaceBinding:
			if b, err := descacl.InterfaceBindingFromProto(kv.Value); err == nil {
				binds = append(binds, b)
			}
		case descacl.NameMacipInterfaceBinding:
			if b, err := descacl.MacipBindingFromProto(kv.Value); err == nil {
				macips = append(macips, b)
			}
		}
	}
	att := applied[aclstate.KeyConfigAttachments]
	if att != nil && env.Record.Attachments(aclstate.BindingsFingerprint(binds, macips), aclstate.ConfigHash(att)) {
		for _, a := range att.GetAttachments() {
			out.Attachments = append(out.Attachments, proto.Clone(a).(*vrxv1.AclAttachment))
		}
		for _, a := range att.GetMacipAttachments() {
			out.MacipAttachments = append(out.MacipAttachments, proto.Clone(a).(*vrxv1.MacipAttachment))
		}
	} else {
		out.Attachments, out.MacipAttachments = reconstructAttachments(binds, macips)
	}
	if proto.Size(out) == 0 {
		return nil
	}
	return out
}

func isDupName(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] == '#' {
			return true
		}
	}
	return false
}

func addressMatch(prefix string) *vrxv1.AddressMatch {
	if prefix == descacl.AnyV4 || prefix == descacl.AnyV6 {
		return &vrxv1.AddressMatch{Kind: proto.String("any")}
	}
	return &vrxv1.AddressMatch{Kind: proto.String("prefix"), Prefix: proto.String(prefix)}
}

func portRange(first, last uint16) []string {
	if first == 0 && last == 65535 {
		return nil
	}
	if first == last {
		return []string{strconv.Itoa(int(first))}
	}
	return []string{fmt.Sprintf("%d-%d", first, last)}
}

func serviceMatch(r descacl.Rule) *vrxv1.ServiceMatch {
	spec := &vrxv1.ServiceSpec{}
	switch r.Proto {
	case objects.ProtoAny:
		return &vrxv1.ServiceMatch{Kind: proto.String("any")}
	case objects.ProtoICMP, objects.ProtoICMP6:
		spec.Protocol = proto.String("icmp")
		if r.Proto == objects.ProtoICMP6 {
			spec.Protocol = proto.String("icmp6")
		}
		if r.SrcPortFirst == r.SrcPortLast {
			spec.Type = proto.Uint32(uint32(r.SrcPortFirst))
			if r.DstPortFirst == r.DstPortLast {
				spec.Code = proto.Uint32(uint32(r.DstPortFirst))
			}
		}
	case objects.ProtoTCP, objects.ProtoUDP, objects.ProtoSCTP:
		spec.Protocol = proto.String(map[uint8]string{objects.ProtoTCP: "tcp", objects.ProtoUDP: "udp", objects.ProtoSCTP: "sctp"}[r.Proto])
		spec.DestinationPorts = portRange(r.DstPortFirst, r.DstPortLast)
		spec.SourcePorts = portRange(r.SrcPortFirst, r.SrcPortLast)
		if r.TCPFlagsMask != 0 {
			spec.TcpFlags = &vrxv1.TcpFlags{Mask: proto.Uint32(uint32(r.TCPFlagsMask)), Value: proto.Uint32(uint32(r.TCPFlagsValue))}
		}
	default:
		spec.Protocol = proto.String("other")
		spec.Number = proto.Uint32(uint32(r.Proto))
	}
	return &vrxv1.ServiceMatch{Kind: proto.String("inline"), Spec: spec}
}

// reconstructList turns VPP rules into configuration rules (one each), for an ACL no recorded
// projection explains.
func reconstructList(a descacl.ACL) *vrxv1.AclList {
	l := &vrxv1.AclList{}
	for i, r := range a.Rules {
		ver := "ipv4"
		if p, err := netip.ParsePrefix(r.Src); err == nil && p.Addr().Is6() {
			ver = "ipv6"
		}
		l.Rules = append(l.Rules, &vrxv1.AclRule{
			Sequence: proto.Uint32(uint32(min(i+1, 214748364)) * 10), //nolint:gosec // bounded
			Enabled:  proto.Bool(true), Action: proto.String(string(r.Action)), IpVersion: proto.String(ver),
			Source: addressMatch(r.Src), Destination: addressMatch(r.Dst), Service: serviceMatch(r), Log: proto.Bool(false),
		})
	}
	return l
}

func reconstructMacip(a descacl.MacipACL) *vrxv1.MacipList {
	l := &vrxv1.MacipList{}
	for i, r := range a.Rules {
		mr := &vrxv1.MacipRule{
			Sequence: proto.Uint32(uint32(min(i+1, 214748364)) * 10), //nolint:gosec // bounded
			Action:   proto.String(string(r.Action)), SourceMac: proto.String(r.SrcMac), SourceMacMask: proto.String(r.SrcMacMask),
		}
		if r.SrcPrefix != descacl.AnyV4 && r.SrcPrefix != descacl.AnyV6 {
			mr.SourcePrefix = proto.String(r.SrcPrefix)
		}
		l.Rules = append(l.Rules, mr)
	}
	return l
}

func reconstructAttachments(binds []descacl.InterfaceBinding, macips []descacl.MacipBinding) ([]*vrxv1.AclAttachment, []*vrxv1.MacipAttachment) {
	sort.Slice(binds, func(i, j int) bool { return binds[i].Interface < binds[j].Interface })
	var out []*vrxv1.AclAttachment
	for _, b := range binds {
		for _, d := range []struct {
			dir   string
			lists []string
		}{{"in", b.Input}, {"out", b.Output}} {
			for i, name := range d.lists {
				out = append(out, &vrxv1.AclAttachment{
					List:      proto.String(name),
					Target:    &vrxv1.AttachmentTarget{Kind: proto.String("interface"), Interface: proto.String(b.Interface)},
					Direction: proto.String(d.dir), Sequence: proto.Uint32(uint32(i+1) * 10), Enabled: proto.Bool(true), //nolint:gosec // ≤ 255 entries
				})
			}
		}
	}
	sort.Slice(macips, func(i, j int) bool { return macips[i].Interface < macips[j].Interface })
	var mout []*vrxv1.MacipAttachment
	for _, m := range macips {
		mout = append(mout, &vrxv1.MacipAttachment{List: proto.String(m.ACL), Interface: proto.String(m.Interface), Enabled: proto.Bool(true)})
	}
	return out, mout
}
