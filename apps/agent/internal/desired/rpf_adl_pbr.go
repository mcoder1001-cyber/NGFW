package desired

// F-rpf-adl-pbr: the projection of uRPF, ADL, ABF policy-based routing and Auto-SDL onto the DF-2
// descriptors (urpf, adl, abf) and descriptors/auto_sdl, and the assembly of their Retrieve
// results back into the document (wave-A-hotspots A2: projection.go calls RpfAdlPbr in project()
// and RpfAdlPbrAssemble in assemble()).
//
//	interfaces.<if>.urpf.{ipv4,ipv6}   → urpf.interface/<if>/<af>/<dir>   (table = the interface's VRF)
//	interfaces.<if>.adl (ipv4|ipv6)    → adl.allowlist/<if> (write-only)  + adl.interface/<if>
//	routing.pbr.policies.<name>        → abf.policy/<id>                   + pbr.policy/<name> (name record)
//	routing.pbr.attachments[]          → abf.attach/<id>/<if>/<af>         (priority = the policy's)
//	services.autoSdl (enabled)         → auto-sdl.config/global (write-only; globals owner only, D-071)
//
// ABF policies are numbered in VPP and carry no tag or name. The policy id is derived from the
// name (FNV-1a into the agent's id range, linear probing in name order — stable unless two names
// collide), and the agent-local pbr.policy/<name> record (persisted by subsystems) keeps the
// name ↔ id mapping and the policy's priority, so Retrieve can name what it finds.
//
// Write-only leaves (the ADL allow-list binding, Auto-SDL) cannot be reported by Retrieve (VPP has
// no dump or getter, D-063); DryRun marks them `agent.write-only` so the drift view can tell them
// from real drift.

import (
	"hash/fnv"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/abf"
	"ngfw/agent/internal/descriptors/adl"
	autosdl "ngfw/agent/internal/descriptors/auto_sdl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/urpf"
	"ngfw/agent/internal/scheduler"
)

// PbrPolicyName is the agent-local descriptor of PBR policy names (subsystems registers it): keys
// "pbr.policy/<name>", value the dfkit encoding of PbrPolicyRecord.
const PbrPolicyName = "pbr.policy"

// PbrPolicyRecord is the value of pbr.policy/<name>: the ABF policy id the name was projected to
// and the policy's priority (VPP keeps priorities only on attachments).
type PbrPolicyRecord struct {
	Name     string `json:"name"`
	PolicyID uint32 `json:"policy_id"`
	Priority uint32 `json:"priority"`
}

// PbrPolicyKey is the key of a policy's name record.
func PbrPolicyKey(name string) scheduler.Key { return scheduler.Join(PbrPolicyName, name) }

// DecodePbrPolicyRecord decodes a pbr.policy value.
func DecodePbrPolicyRecord(v proto.Message) (PbrPolicyRecord, error) {
	var r PbrPolicyRecord
	err := dfkit.Decode(v, &r)
	return r, err
}

// AbfPolicyKey is DF-2's abf.policy key of a policy id.
func AbfPolicyKey(id uint32) scheduler.Key {
	return (*abf.PolicyDescriptor)(nil).KeyOf(&abf.Policy{PolicyId: id})
}

// RpfAdlPbrEnv is what the projection needs besides the document.
type RpfAdlPbrEnv struct {
	// PolicyIDs is the agent's ABF policy id range (nil = every id: the product agent).
	PolicyIDs *df2.IDRange
	// RecordedIDs are the policy ids the agent applied, by name (the pbr.policy store): kept (sticky).
	RecordedIDs map[string]uint32
	// AutoSdl reports whether auto-sdl.config is registered (the D-071 globals owner only).
	AutoSdl bool
}

// Rule ids of the projection's findings.
const (
	ruleUrpfMode      = "interfaces.rpf-adl-pbr-urpf-mode"
	ruleUrpfECMP      = "interfaces.rpf-adl-pbr-urpf-strict-ecmp"
	ruleAdlNonIP      = "interfaces.rpf-adl-pbr-adl-non-ip"
	ruleAdlVRF        = "interfaces.rpf-adl-pbr-adl-vrf"
	rulePbrPath       = "routing.rpf-adl-pbr-path"
	rulePbrPolicy     = "routing.rpf-adl-pbr-policy"
	rulePbrAttachment = "routing.rpf-adl-pbr-attachment"
	ruleWriteOnly     = "agent.write-only"
	ruleUnsupported   = "agent.unsupported-field"
)

// PolicyIDs assigns every policy name an ABF policy id inside ids (nil: 1..2^31-1). A name that has a
// recorded id (the pbr.policy store: what the agent applied) keeps it while it is inside the range and not
// taken — ids are sticky, so adding or removing other policies never moves an existing one (which would
// recreate its attachments and briefly let its traffic follow the FIB). A new name gets FNV-1a of the name,
// linear probing over the free ids in sorted name order.
func PolicyIDs(names []string, ids *df2.IDRange, recorded map[string]uint32) map[string]uint32 {
	lo, size := uint64(1), uint64(1)<<31-1
	if ids != nil {
		lo, size = uint64(ids.Lo), uint64(ids.Hi)-uint64(ids.Lo)+1
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	used := map[uint64]bool{}
	out := make(map[string]uint32, len(sorted))
	for _, n := range sorted { // recorded ids first
		id, ok := recorded[n]
		if !ok || uint64(id) < lo || uint64(id)-lo >= size || used[uint64(id)-lo] {
			continue
		}
		used[uint64(id)-lo] = true
		out[n] = id
	}
	for _, n := range sorted {
		if _, done := out[n]; done {
			continue
		}
		if uint64(len(used)) >= size {
			break // more policies than ids: the rest stay unassigned (reported by the caller)
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(n))
		slot := uint64(h.Sum32()) % size
		for used[slot] {
			slot = (slot + 1) % size
		}
		used[slot] = true
		out[n] = uint32(lo + slot) //nolint:gosec // lo+slot ≤ Hi (a uint32) by construction
	}
	return out
}

// RpfAdlPbr emits the objects of the feature for the domains in `in`. vrfID maps a VRF name to its
// table id (false: unknown VRF).
func RpfAdlPbr(s Sink, ds *vrxv1.DesiredState, in map[string]bool, vrfID func(string) (uint32, bool), env RpfAdlPbrEnv) {
	if in["interfaces"] {
		for _, name := range sortedKeys(ds.GetInterfaces()) {
			itf := ds.GetInterfaces()[name]
			projectUrpf(s, name, itf, ds.GetRouting(), vrfID)
			projectAdl(s, name, itf.GetAdl(), vrfID)
		}
	}
	if in["routing"] && ds.GetRouting().GetPbr() != nil {
		projectPbr(s, ds.GetRouting().GetPbr(), vrfID, env.PolicyIDs, env.RecordedIDs)
	}
	if in["services"] {
		projectServices(s, ds.GetServices(), env.AutoSdl)
	}
}

var urpfModes = map[string]urpf.Interface_Mode{"loose": urpf.Interface_LOOSE, "strict": urpf.Interface_STRICT}

func projectUrpf(s Sink, name string, itf *vrxv1.Interface, routing *vrxv1.RoutingConfig, vrfID func(string) (uint32, bool)) {
	u := itf.GetUrpf()
	if u == nil {
		return
	}
	dir := urpf.Interface_RX
	switch u.GetDirection() {
	case "", "rx":
	case "tx":
		dir = urpf.Interface_TX
	default:
		s.Errorf(Ptr("interfaces", name, "urpf", "direction"), ruleUrpfMode, "uRPF direction %q is not rx or tx", u.GetDirection())
		return
	}
	table, ok := vrfID(itf.GetVrf())
	if !ok {
		return // interfaces.vrf-exists (P08) reports it
	}
	for _, fam := range []struct {
		key  string
		mode *string
		af   df2.AddressFamily
	}{{"ipv4", u.Ipv4, df2.AddressFamily_IPV4}, {"ipv6", u.Ipv6, df2.AddressFamily_IPV6}} {
		if fam.mode == nil {
			continue
		}
		pt := Ptr("interfaces", name, "urpf", fam.key)
		mode, ok := urpfModes[*fam.mode]
		if !ok {
			s.Errorf(pt, ruleUrpfMode, "uRPF mode %q is not loose or strict", *fam.mode)
			continue
		}
		v := &urpf.Interface{Interface: name, Af: fam.af, Direction: dir, Mode: mode, TableId: table}
		s.Add((*urpf.Descriptor)(nil).KeyOf(v), v, pt)
		if mode == urpf.Interface_STRICT && dir == urpf.Interface_RX {
			if prefix, ok := ecmpEgress(routing, name, fam.af == df2.AddressFamily_IPV6); ok {
				s.Warnf(pt, ruleUrpfECMP, "strict uRPF on %s: it is one of several ECMP egress interfaces of %s, so replies from there may arrive on another member and be dropped; loose mode is the usual choice on ECMP uplinks", name, prefix)
			}
		}
	}
}

// ecmpEgress returns a static route of the family that has several next hops, one of them out of
// iface.
func ecmpEgress(routing *vrxv1.RoutingConfig, iface string, v6 bool) (string, bool) {
	for _, r := range routing.GetStatic() {
		if len(r.GetNextHops()) < 2 {
			continue
		}
		p, err := netip.ParsePrefix(r.GetPrefix())
		if err != nil || p.Addr().Is6() != v6 {
			continue
		}
		for _, nh := range r.GetNextHops() {
			if nh.GetInterface() == iface {
				return r.GetPrefix(), true
			}
		}
	}
	return "", false
}

func projectAdl(s Sink, name string, a *vrxv1.AdlConfig, vrfID func(string) (uint32, bool)) {
	if a == nil || (!a.GetIpv4() && !a.GetIpv6()) {
		return // absent or nothing checked: ADL off
	}
	pt := Ptr("interfaces", name, "adl")
	if a.DefaultAllow != nil && !a.GetDefaultAllow() {
		s.Errorf(pt+"/defaultAllow", ruleAdlNonIP, "VPP 26.06 cannot filter non-IP frames with ADL (its default allow-list node is a stub that leaks buffers): defaultAllow must stay true")
		return
	}
	if a.AllowVrf == nil {
		s.Errorf(pt+"/allowVrf", ruleAdlVRF, "allowVrf is required when ipv4 or ipv6 is checked")
		return
	}
	fib, ok := vrfID(a.GetAllowVrf())
	if !ok {
		s.Errorf(pt+"/allowVrf", ruleAdlVRF, "VRF %q does not exist", a.GetAllowVrf())
		return
	}
	allow := &adl.Allowlist{Interface: name, FibId: fib, Ip4: a.GetIpv4(), Ip6: a.GetIpv6()}
	s.Add((*adl.AllowlistDescriptor)(nil).KeyOf(allow), allow, pt)
	on := &adl.Interface{Interface: name}
	s.Add((*adl.InterfaceDescriptor)(nil).KeyOf(on), on, pt)
	for _, leaf := range []string{"ipv4", "ipv6", "allowVrf", "defaultAllow"} {
		s.Warnf(pt+"/"+leaf, ruleWriteOnly, "%s/%s is applied but VPP cannot report it (adl allow-list binding: no dump, D-063); Retrieve shows only that ADL is on", pt, leaf)
	}
}

// pathProto is the FIB next-hop protocol of an address-less path: the family its policy is
// attached for (IPv4 when it is not attached, or attached for both — then an error).
func pathProto(p *vrxv1.PbrConfig, policy string) (df2.FibPath_Proto, bool) {
	var v4, v6 bool
	for _, a := range p.GetAttachments() {
		if a.GetPolicy() == policy {
			if a.GetFamily() == "ipv6" {
				v6 = true
			} else {
				v4 = true
			}
		}
	}
	if v6 && !v4 {
		return df2.FibPath_IP6, true
	}
	return df2.FibPath_IP4, !v4 || !v6
}

func projectPbr(s Sink, p *vrxv1.PbrConfig, vrfID func(string) (uint32, bool), ids *df2.IDRange, recorded map[string]uint32) {
	names := sortedKeys(p.GetPolicies())
	idOf := PolicyIDs(names, ids, recorded)
	priority := map[string]uint32{}
	for _, name := range names {
		pol := p.GetPolicies()[name]
		pt := Ptr("routing", "pbr", "policies", name)
		id, ok := idOf[name]
		if !ok {
			s.Errorf(pt, rulePbrPolicy, "no free ABF policy id left in this agent's range for policy %q", name)
			continue
		}
		if strings.Contains(name, "#") || len(name) > 63 || name == "" {
			s.Errorf(pt, rulePbrPolicy, "policy name %q: at most 63 characters, no '#' (D-066)", name)
			continue
		}
		if pol.GetAcl() == "" {
			s.Errorf(pt+"/acl", rulePbrPolicy, "policy %q needs an ACL", name)
			continue
		}
		paths, ok := pbrPaths(s, p, name, pol, vrfID)
		if !ok {
			continue
		}
		v, err := abf.NormalizePolicy(&abf.Policy{PolicyId: id, Acl: pol.GetAcl(), Paths: paths})
		if err != nil {
			s.Errorf(pt+"/paths", rulePbrPath, "%v", err)
			continue
		}
		prio := uint32(100)
		if pol.Priority != nil {
			prio = pol.GetPriority()
		}
		priority[name] = prio
		s.Add(AbfPolicyKey(id), v, pt)
		s.Add(PbrPolicyKey(name), dfkit.Encode(PbrPolicyRecord{Name: name, PolicyID: id, Priority: prio}), pt)
	}
	for i, a := range p.GetAttachments() {
		pt := Ptr("routing", "pbr", "attachments", strconv.Itoa(i))
		prio, ok := priority[a.GetPolicy()]
		if !ok {
			if _, declared := p.GetPolicies()[a.GetPolicy()]; !declared {
				s.Errorf(pt+"/policy", rulePbrAttachment, "PBR policy %q does not exist", a.GetPolicy())
			}
			continue // an invalid policy was reported above
		}
		v6 := false
		switch a.GetFamily() {
		case "", "ipv4":
		case "ipv6":
			v6 = true
		default:
			s.Errorf(pt+"/family", rulePbrAttachment, "family %q is not ipv4 or ipv6", a.GetFamily())
			continue
		}
		if a.GetInterface() == "" {
			s.Errorf(pt+"/interface", rulePbrAttachment, "an attachment needs an interface")
			continue
		}
		v := &abf.Attach{PolicyId: idOf[a.GetPolicy()], Interface: a.GetInterface(), Priority: prio, Ipv6: v6}
		s.Add((*abf.AttachDescriptor)(nil).KeyOf(v), v, pt)
	}
}

func pbrPaths(s Sink, p *vrxv1.PbrConfig, name string, pol *vrxv1.PbrPolicy, vrfID func(string) (uint32, bool)) ([]*df2.FibPath, bool) {
	if len(pol.GetPaths()) == 0 || len(pol.GetPaths()) > 255 {
		s.Errorf(Ptr("routing", "pbr", "policies", name, "paths"), rulePbrPath, "a policy needs 1–255 paths")
		return nil, false
	}
	var out []*df2.FibPath
	ok := true
	for j, path := range pol.GetPaths() {
		pt := Ptr("routing", "pbr", "policies", name, "paths", strconv.Itoa(j))
		fp := &df2.FibPath{Interface: path.GetInterface(), Weight: path.GetWeight()}
		if fp.Weight == 0 {
			fp.Weight = 1
		}
		if fp.Weight > 255 {
			s.Errorf(pt+"/weight", rulePbrPath, "weight %d exceeds 255", fp.Weight)
			ok = false
			continue
		}
		if path.Address != nil {
			addr, err := netip.ParseAddr(path.GetAddress())
			if err != nil {
				s.Errorf(pt+"/address", rulePbrPath, "next-hop %q: %v", path.GetAddress(), err)
				ok = false
				continue
			}
			fp.NextHop = addr.Unmap().String()
		} else {
			proto, single := pathProto(p, name)
			if !single {
				s.Errorf(pt, rulePbrPath, "a path without a next-hop address serves one address family, but policy %q is attached for IPv4 and IPv6", name)
				ok = false
				continue
			}
			fp.Proto = proto
		}
		vrf := path.GetVrf()
		if vrf == "" {
			vrf = "default"
		}
		table, found := vrfID(vrf)
		if !found {
			s.Errorf(pt+"/vrf", rulePbrPath, "VRF %q does not exist", vrf)
			ok = false
			continue
		}
		if fp.Interface != "" && table != 0 {
			s.Errorf(pt+"/vrf", rulePbrPath, "a path with an interface is resolved on that interface; vrf applies only to paths without one")
			ok = false
			continue
		}
		fp.TableId = table
		out = append(out, fp)
	}
	return out, ok
}

// ServicesMembers are the `services.*` members (JSON names) this agent build implements. Append-only: a feature
// that implements a member adds it from an init() in its own file (`func init() { ServicesMembers["nsim"] = true }`);
// every other non-empty member is reported as agent.unsupported-field.
var ServicesMembers = map[string]bool{}

func init() { ServicesMembers["autoSdl"] = true }

// projectServices projects services.autoSdl (globals owner only) and reports the services members no feature
// implements. Nothing is noted at domain level: `services` is an implemented domain (Health.subsystems).
func projectServices(s Sink, svc *vrxv1.ServicesConfig, autoSdl bool) {
	if a := svc.GetAutoSdl(); a != nil {
		pt := Ptr("services", "autoSdl")
		switch {
		case !autoSdl:
			s.Warnf(pt, ruleUnsupported, "services.autoSdl is a VPP-global setting (auto_sdl_config) that only the globals owner applies (D-071); this agent is not the globals owner")
		case a.GetEnabled():
			c := autosdl.Config{Enable: true, Threshold: autosdl.DefaultThreshold, RemoveTimeout: autosdl.DefaultRemoveTimeout}
			if a.Threshold != nil {
				c.Threshold = a.GetThreshold()
			}
			if a.RemoveTimeoutSec != nil {
				c.RemoveTimeout = a.GetRemoveTimeoutSec()
			}
			if c.Threshold == 0 || c.RemoveTimeout == 0 {
				s.Errorf(pt, ruleUnsupported, "threshold and removeTimeoutSec must be > 0")
				break
			}
			s.Add(autosdl.Key, c.Proto(), pt)
			fallthrough
		default:
			s.Warnf(pt, ruleWriteOnly, "services.autoSdl is applied but VPP has no getter for it (auto_sdl_config, D-063); Retrieve cannot report it")
		}
	}
	svc.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if ServicesMembers[fd.JSONName()] || fd.Message() == nil || proto.Size(v.Message().Interface()) == 0 {
			return true
		}
		s.Warnf(Ptr("services", fd.JSONName()), ruleUnsupported, "services.%s is not implemented by this agent build", fd.JSONName())
		return true
	})
}

// RpfAdlPbrAssemble adds the feature's leaves to ds (after Assemble built `interfaces` and the
// routes): uRPF and ADL on the interfaces, routing.pbr from the policy name records, the ABF
// policies and attachments. tableName names a FIB table; stored is the stored `interfaces`
// document (an off-equivalent urpf/adl object the operator wrote is reported as written when VPP
// has nothing on the interface — it is the canonical form of "off").
func RpfAdlPbrAssemble(ds *vrxv1.DesiredState, kvs []scheduler.KV, in map[string]bool, tableName func(uint32) string, stored map[string]*vrxv1.Interface) {
	if in["interfaces"] {
		assembleInterfaces(ds, kvs, tableName, stored)
	}
	if in["routing"] {
		if pbr := assemblePbr(kvs, tableName); pbr != nil {
			if ds.Routing == nil {
				ds.Routing = &vrxv1.RoutingConfig{}
			}
			ds.Routing.Pbr = pbr
		}
	}
	// services: present when requested (an implemented domain); no member is retrievable here (autoSdl is
	// write-only, D-063), so the drift view skips the members by their field-level DryRun notes
	if in["services"] && ds.Services == nil {
		ds.Services = &vrxv1.ServicesConfig{}
	}
}

func assembleInterfaces(ds *vrxv1.DesiredState, kvs []scheduler.KV, tableName func(uint32) string, stored map[string]*vrxv1.Interface) {
	get := func(name string) *vrxv1.Interface {
		if ds.Interfaces == nil {
			ds.Interfaces = map[string]*vrxv1.Interface{}
		}
		itf, ok := ds.Interfaces[name]
		if !ok {
			// only our objects on it (e.g. a claimed port the stored document no longer names)
			itf = &vrxv1.Interface{Enabled: proto.Bool(false), Promiscuous: proto.Bool(false), Vrf: proto.String(tableName(0))}
			ds.Interfaces[name] = itf
		}
		return itf
	}
	modes := map[urpf.Interface_Mode]string{urpf.Interface_LOOSE: "loose", urpf.Interface_STRICT: "strict"}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *urpf.Interface:
			m, ok := modes[v.GetMode()]
			if !ok {
				continue
			}
			itf := get(v.GetInterface())
			if itf.Urpf == nil {
				itf.Urpf = &vrxv1.UrpfConfig{Direction: proto.String("rx")}
			}
			if v.GetDirection() == urpf.Interface_TX {
				itf.Urpf.Direction = proto.String("tx")
			}
			if v.GetAf() == df2.AddressFamily_IPV6 {
				itf.Urpf.Ipv6 = proto.String(m)
			} else {
				itf.Urpf.Ipv4 = proto.String(m)
			}
		case *adl.Interface:
			get(v.GetInterface()).Adl = &vrxv1.AdlConfig{} // the binding is write-only: only presence is known
		}
	}
	// an off-equivalent object as the operator wrote it, where VPP has nothing either
	for name, s := range stored {
		itf, ok := ds.Interfaces[name]
		if !ok {
			continue
		}
		if u := s.GetUrpf(); u != nil && itf.Urpf == nil && u.Ipv4 == nil && u.Ipv6 == nil {
			itf.Urpf = proto.Clone(u).(*vrxv1.UrpfConfig)
		}
		if a := s.GetAdl(); a != nil && itf.Adl == nil && !a.GetIpv4() && !a.GetIpv6() {
			itf.Adl = proto.Clone(a).(*vrxv1.AdlConfig)
		}
	}
}

func assemblePbr(kvs []scheduler.KV, tableName func(uint32) string) *vrxv1.PbrConfig {
	records := map[uint32]PbrPolicyRecord{}
	policies := map[uint32]*abf.Policy{}
	var attaches []*abf.Attach
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *abf.Policy:
			policies[v.GetPolicyId()] = v
		case *abf.Attach:
			attaches = append(attaches, v)
		default:
			if kv.Key.Descriptor() == PbrPolicyName {
				if r, err := DecodePbrPolicyRecord(kv.Value); err == nil {
					records[r.PolicyID] = r
				}
			}
		}
	}
	if len(policies) == 0 && len(attaches) == 0 {
		return nil
	}
	// a policy or attachment without a name record (a leftover) is named "#<id>": '#' is not a
	// valid policy name, so it shows as drift and the next apply deletes it
	nameOf := func(id uint32) string {
		if r, ok := records[id]; ok {
			return r.Name
		}
		return "#" + strconv.FormatUint(uint64(id), 10)
	}
	out := &vrxv1.PbrConfig{Policies: map[string]*vrxv1.PbrPolicy{}}
	for id, pol := range policies {
		prio := uint32(100)
		if r, ok := records[id]; ok {
			prio = r.Priority
		}
		pp := &vrxv1.PbrPolicy{Acl: proto.String(pol.GetAcl()), Priority: proto.Uint32(prio)}
		for _, fp := range pol.GetPaths() {
			path := &vrxv1.PbrPath{Vrf: proto.String(tableName(fp.GetTableId())), Weight: proto.Uint32(fp.GetWeight())}
			if fp.GetNextHop() != "" {
				path.Address = proto.String(fp.GetNextHop())
			}
			if fp.GetInterface() != "" {
				path.Interface = proto.String(fp.GetInterface())
			}
			pp.Paths = append(pp.Paths, path)
		}
		out.Policies[nameOf(id)] = pp
	}
	sort.Slice(attaches, func(i, j int) bool {
		a, b := attaches[i], attaches[j]
		na, nb := nameOf(a.GetPolicyId()), nameOf(b.GetPolicyId())
		switch {
		case na != nb:
			return na < nb
		case a.GetInterface() != b.GetInterface():
			return a.GetInterface() < b.GetInterface()
		default:
			return !a.GetIpv6() && b.GetIpv6()
		}
	})
	for _, a := range attaches {
		fam := "ipv4"
		if a.GetIpv6() {
			fam = "ipv6"
		}
		out.Attachments = append(out.Attachments, &vrxv1.PbrAttachment{Policy: proto.String(nameOf(a.GetPolicyId())), Interface: proto.String(a.GetInterface()), Family: proto.String(fam)})
	}
	return out
}
