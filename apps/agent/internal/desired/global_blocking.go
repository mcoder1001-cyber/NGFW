package desired

// F-global-blocking: acl.globalBlocking — IP block lists enforced ahead of the user ACLs (decision Q2:
// bucketed VPP ACLs of the acl plugin, the same family and attachment semantics as acl.lists).
//
//	acl.globalBlocking.lists.<name>  → acl.acl/_gb.<name>.i<nn>   deny src ∈ bucket nn (inbound)
//	                                   acl.acl/_gb.<name>.o<nn>   deny dst ∈ bucket nn (outbound)
//	                                   prepended to acl.interface-binding/<ifname> of every selected interface
//	(an interface with no user ACL in a direction) → acl.acl/_gb.pass  permit any, appended after the
//	                                   block lists (VPP denies a packet no ACL of a bound direction matches)
//
// Entries are spread over a stable number of buckets by a hash of the entry, so importing a changed list
// replaces only the buckets whose entries changed (a one-line change is one acl_add_replace of ≤ bucketSize
// rules, not the whole list). User list names cannot start with "_" (objectName), so the "_gb." names
// never collide with acl.lists. protectHost is rendered by the nftables host renderer (renderers/nftables).

import (
	"fmt"
	"hash/fnv"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/vpp"
)

// Block-list ACL naming and bucketing.
const (
	// GlobalBlockingPrefix starts the name of every VPP ACL the block lists produce.
	GlobalBlockingPrefix = "_gb."
	// GlobalBlockingPass is the permit-any ACL appended where no user ACL is bound in a direction.
	GlobalBlockingPass = GlobalBlockingPrefix + "pass"
	// gbBucketSize is the target number of entries per bucket ACL.
	gbBucketSize = 4096
	// gbMaxBuckets bounds the bucket ACLs per list and direction (200 000 entries → 64 buckets of ~3 125).
	gbMaxBuckets = 64
	// gbMaxBound is the most ACLs one interface takes over both directions (acl_interface_set_acl_list).
	gbMaxBound = 255
)

// gbBuckets is the stable bucket count for n entries: the next power of two of n/gbBucketSize, capped.
func gbBuckets(n int) int {
	want := (n + gbBucketSize - 1) / gbBucketSize
	b := 1
	for b < want && b < gbMaxBuckets {
		b *= 2
	}
	return b
}

func gbBucketOf(entry string, buckets int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(entry))
	return int(h.Sum32() % uint32(buckets)) //nolint:gosec // buckets ≤ gbMaxBuckets
}

// GlobalBlockingACLName is the VPP ACL name of bucket b of list in direction dir ('i' inbound, 'o' outbound).
func GlobalBlockingACLName(list string, dir byte, b int) string {
	return fmt.Sprintf("%s%s.%c%02d", GlobalBlockingPrefix, list, dir, b)
}

// parseGlobalBlockingACLName splits "_gb.<list>.<dir><nn>" (ok false for _gb.pass and foreign names).
func parseGlobalBlockingACLName(name string) (list string, dir byte, ok bool) {
	rest, found := strings.CutPrefix(name, GlobalBlockingPrefix)
	i := strings.LastIndexByte(rest, '.')
	if !found || i <= 0 || len(rest)-i < 3 {
		return "", 0, false
	}
	suf := rest[i+1:]
	if suf[0] != 'i' && suf[0] != 'o' {
		return "", 0, false
	}
	if _, err := strconv.Atoi(suf[1:]); err != nil {
		return "", 0, false
	}
	return rest[:i], suf[0], true
}

// gbPlan is what the block lists add to the interface bindings: per interface, the ACL names to put
// before the user's lists (in, out).
type gbPlan struct {
	in, out map[string][]string
	acls    map[string][]descacl.Rule // every emitted block-list ACL (for the record fingerprint)
	pointer map[string]string         // interface → the list that first selected it (object pointer)
}

// globalBlocking emits the block-list ACLs of ds.acl.globalBlocking and returns what they add to the
// bindings. A list with an error emits nothing (the transaction fails on the error anyway).
func (x *aclExpander) globalBlocking(ds *ngfwv1.DesiredState) *gbPlan {
	g := ds.GetAcl().GetGlobalBlocking()
	plan := &gbPlan{in: map[string][]string{}, out: map[string][]string{}, acls: map[string][]descacl.Rule{}, pointer: map[string]string{}}
	if g == nil {
		return plan
	}
	allIfs := sortedKeys(ds.GetInterfaces())
	for _, name := range sortedKeys(g.GetLists()) {
		l := g.GetLists()[name]
		lp := Ptr("acl", "globalBlocking", "lists", name)
		if l.Enabled != nil && !l.GetEnabled() {
			continue
		}
		if l.GetLog() {
			x.s.Warnf(lp+"/log", "acl.log-unsupported", "block list %q asks for logging; VPP's acl plugin cannot log — data-plane drops are counted (hit counters), drops to the box itself are logged", name)
		}
		if len(l.GetEntries()) == 0 {
			continue
		}
		longest := GlobalBlockingACLName(name, 'o', gbMaxBuckets-1)
		if _, err := vpp.OwnerTag(x.env.Owner, longest); x.env.Owner != "" && err != nil {
			x.s.Errorf(lp, "acl.name", "block list name %q is too long for this agent's VPP tags (%q): %v", name, longest, err)
			continue
		}
		in, out := true, true
		switch l.GetDirection() {
		case "", "both":
		case "inbound":
			out = false
		case "outbound":
			in = false
		default:
			x.s.Errorf(lp+"/direction", "acl.global-blocking", "unknown direction %q", l.GetDirection())
			continue
		}
		ifs := l.GetInterfaces()
		if l.GetAllInterfaces() {
			ifs = allIfs
		}
		if len(ifs) == 0 {
			x.s.Warnf(lp+"/interfaces", "acl.global-blocking", "block list %q selects no interface: it is not enforced in the data plane", name)
			continue
		}
		prefixes, ok := x.gbEntries(lp, l.GetEntries())
		if !ok {
			continue
		}
		nb := gbBuckets(len(prefixes))
		buckets := make([][]netip.Prefix, nb)
		for _, p := range prefixes {
			b := gbBucketOf(p.String(), nb)
			buckets[b] = append(buckets[b], p)
		}
		var inNames, outNames []string
		for b, ps := range buckets {
			if len(ps) == 0 {
				continue
			}
			for _, d := range []struct {
				on  bool
				dir byte
				to  *[]string
			}{{in, 'i', &inNames}, {out, 'o', &outNames}} {
				if !d.on {
					continue
				}
				rules := make([]descacl.Rule, 0, len(ps))
				for _, p := range ps {
					anyAddr := descacl.AnyV4
					if p.Addr().Is6() {
						anyAddr = descacl.AnyV6
					}
					r := descacl.Rule{Action: descacl.ActionDeny, Proto: objects.ProtoAny, SrcPortLast: 65535, DstPortLast: 65535, Src: p.String(), Dst: anyAddr}
					if d.dir == 'o' {
						r.Src, r.Dst = anyAddr, p.String()
					}
					rules = append(rules, r)
				}
				an := GlobalBlockingACLName(name, d.dir, b)
				plan.acls[an] = rules
				*d.to = append(*d.to, an)
				x.s.Add(descacl.KeyACL(an), descacl.ACL{Name: an, Rules: rules}.Proto(), lp)
			}
		}
		for _, ifn := range ifs {
			if _, seen := plan.pointer[ifn]; !seen {
				plan.pointer[ifn] = lp
			}
			plan.in[ifn] = append(plan.in[ifn], inNames...)
			plan.out[ifn] = append(plan.out[ifn], outNames...)
		}
	}
	return plan
}

// gbEntries parses a list's entries (canonical prefixes, the schema's form), sorted and deduplicated.
func (x *aclExpander) gbEntries(lp string, entries []string) ([]netip.Prefix, bool) {
	out := make([]netip.Prefix, 0, len(entries))
	for i, e := range entries {
		p, err := netip.ParsePrefix(e)
		if err != nil || p.Masked() != p {
			x.s.Errorf(lp+"/entries/"+strconv.Itoa(i), "acl.global-blocking", "%q is not a canonical IPv4/IPv6 prefix", e)
			return nil, false
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Addr().Compare(out[j].Addr()); c != 0 {
			return c < 0
		}
		return out[i].Bits() < out[j].Bits()
	})
	n := 0
	for i, p := range out {
		if i > 0 && p == out[n-1] {
			continue
		}
		out[n] = p
		n++
	}
	return out[:n], true
}

// gbMerge returns the binding of ifn with the block lists put first and, in a direction the block lists
// use and no user ACL does, the pass ACL last; gbPart is the block-list part alone (record fingerprint).
func gbMerge(user descacl.InterfaceBinding, plan *gbPlan) (merged, gbPart descacl.InterfaceBinding) {
	ifn := user.Interface
	gbPart = descacl.InterfaceBinding{Interface: ifn}
	gbPart.Input = append(gbPart.Input, plan.in[ifn]...)
	gbPart.Output = append(gbPart.Output, plan.out[ifn]...)
	if len(gbPart.Input) > 0 && len(user.Input) == 0 {
		gbPart.Input = append(gbPart.Input, GlobalBlockingPass)
	}
	if len(gbPart.Output) > 0 && len(user.Output) == 0 {
		gbPart.Output = append(gbPart.Output, GlobalBlockingPass)
	}
	merged = descacl.InterfaceBinding{Interface: ifn}
	merged.Input = append(merged.Input, gbPart.Input...)
	merged.Input = append(merged.Input, user.Input...)
	merged.Output = append(merged.Output, gbPart.Output...)
	merged.Output = append(merged.Output, user.Output...)
	// a pass ACL only after the user lists never happens: it is added only where there are none
	return merged, gbPart
}

var gbPassRules = []descacl.Rule{
	{Action: descacl.ActionPermit, Src: descacl.AnyV4, Dst: descacl.AnyV4, Proto: objects.ProtoAny, SrcPortLast: 65535, DstPortLast: 65535},
	{Action: descacl.ActionPermit, Src: descacl.AnyV6, Dst: descacl.AnyV6, Proto: objects.ProtoAny, SrcPortLast: 65535, DstPortLast: 65535},
}

// splitGlobalBlocking separates the block-list names of a retrieved binding from the user's.
func splitGlobalBlocking(b descacl.InterfaceBinding) (user, gbPart descacl.InterfaceBinding) {
	user = descacl.InterfaceBinding{Interface: b.Interface}
	gbPart = descacl.InterfaceBinding{Interface: b.Interface}
	for _, n := range b.Input {
		if strings.HasPrefix(n, GlobalBlockingPrefix) {
			gbPart.Input = append(gbPart.Input, n)
		} else {
			user.Input = append(user.Input, n)
		}
	}
	for _, n := range b.Output {
		if strings.HasPrefix(n, GlobalBlockingPrefix) {
			gbPart.Output = append(gbPart.Output, n)
		} else {
			user.Output = append(user.Output, n)
		}
	}
	return user, gbPart
}

// reconstructGlobalBlocking turns block-list ACLs and bindings no recorded projection explains into
// block lists (entries from the rules, interfaces from the bindings), so the difference shows as drift.
func reconstructGlobalBlocking(acls map[string][]descacl.Rule, parts []descacl.InterfaceBinding) *ngfwv1.GlobalBlocking {
	type acc struct {
		entries map[string]bool
		in, out bool
		ifs     map[string]bool
	}
	lists := map[string]*acc{}
	get := func(n string) *acc {
		a, ok := lists[n]
		if !ok {
			a = &acc{entries: map[string]bool{}, ifs: map[string]bool{}}
			lists[n] = a
		}
		return a
	}
	for name, rules := range acls {
		list, dir, ok := parseGlobalBlockingACLName(name)
		if !ok {
			continue
		}
		a := get(list)
		for _, r := range rules {
			if dir == 'i' {
				a.in = true
				a.entries[r.Src] = true
			} else {
				a.out = true
				a.entries[r.Dst] = true
			}
		}
	}
	for _, b := range parts {
		for _, n := range append(append([]string{}, b.Input...), b.Output...) {
			if list, _, ok := parseGlobalBlockingACLName(n); ok {
				get(list).ifs[b.Interface] = true
			}
		}
	}
	if len(lists) == 0 {
		return nil
	}
	g := &ngfwv1.GlobalBlocking{Lists: map[string]*ngfwv1.GlobalBlockingList{}}
	for name, a := range lists {
		dir := "both"
		switch {
		case a.in && !a.out:
			dir = "inbound"
		case a.out && !a.in:
			dir = "outbound"
		}
		entries := make([]string, 0, len(a.entries))
		for e := range a.entries {
			entries = append(entries, e)
		}
		sort.Strings(entries)
		ifs := make([]string, 0, len(a.ifs))
		for i := range a.ifs {
			ifs = append(ifs, i)
		}
		sort.Strings(ifs)
		t, f := true, false
		g.Lists[name] = &ngfwv1.GlobalBlockingList{
			Enabled: &t, Source: &ngfwv1.GlobalBlockingSource{Kind: strPtr("upload")}, AllInterfaces: &f, Interfaces: ifs,
			Direction: &dir, ProtectHost: &f, Log: &f, Entries: entries,
		}
	}
	return g
}

// recordGlobalBlocking records the block-list set and emits the applied-configuration object.
func (x *aclExpander) recordGlobalBlocking(g *ngfwv1.GlobalBlocking, plan *gbPlan, parts []descacl.InterfaceBinding) {
	if g == nil {
		return
	}
	fp := aclstate.GlobalBlockingFingerprint(plan.acls, parts)
	x.env.Record.PutGlobalBlocking(&aclstate.Attachments{Fingerprint: fp, ConfigHash: aclstate.ConfigHash(g)})
	x.s.Add(aclstate.KeyConfigGlobalBlocking, aclstate.ConfigGlobalBlocking(g), Ptr("acl", "globalBlocking"))
}
