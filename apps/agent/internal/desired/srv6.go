package desired

// F-srv6: the desired-state builder and assembler of `routing.srv6` over DF-6's sr descriptors
// (apps/agent/internal/descriptors/sr, docs/agent/descriptors/sr.md):
//
//	routing.srv6.localSids.<sid>       → sr.localsid/<sid>                       (deps vrf/<table>, vrf/<lookup>, interface/<if>)
//	routing.srv6.policies.<bsid>       → sr.policy/<bsid>                        (dep vrf/<table>)
//	routing.srv6.steering[] l3         → sr.steering/<ipv4|ipv6>/<table>/<prefix> (deps sr.policy/<bsid>, vrf/<table>)
//	routing.srv6.steering[] l2         → sr.steering/l2/<interface>              (deps sr.policy/<bsid>, interface/<if>)
//	routing.srv6.encapSource           → sr.encap-source/global                  (VPP-global, write-only)
//	routing.srv6.encapHopLimit         → sr.encap-hop-limit/global               (VPP-global, write-only)
//
// Every encapsulating policy carries its own outer source (D-074): the policy's encapSource, else
// routing.srv6.encapSource; the insert policies carry none. The two globals are applied by the globals
// owner only (D-071: the sr.Register require variant fails on every other agent, a write-only global
// cannot be checked); Retrieve never reports them (D-063), which DryRun says with an
// agent.unsupported-field note so /state/drift does not compare them.
//
// Delete order (V15/V22a): steering depends on its policy and table, policies and local SIDs on their
// tables, so the scheduler removes steering before policies, and everything SR before the VRF — no SR
// FIB entry is ever left in a table that is deleted. Nothing here flushes a table.

import (
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/scheduler"
)

// Srv6Env is what the SRv6 assembler needs from the wiring (subsystems.Srv6Env): the global encap
// source this agent applied (globals owner only; "" otherwise or before the first resync). A policy
// whose source equals it inherited routing.srv6.encapSource, so its own encapSource is left unset.
type Srv6Env struct {
	EncapSource func() string
}

const (
	srv6Unsupported = "agent.unsupported-field"
	srv6Rule        = "routing.srv6"
)

var srv6Behaviors = map[string]sr.Behavior{
	"end": sr.Behavior_END, "end.x": sr.Behavior_END_X, "end.t": sr.Behavior_END_T,
	"end.dx2": sr.Behavior_END_DX2, "end.dx4": sr.Behavior_END_DX4, "end.dx6": sr.Behavior_END_DX6,
	"end.dt4": sr.Behavior_END_DT4, "end.dt6": sr.Behavior_END_DT6,
}

var srv6PolicyTypes = map[string]sr.PolicyType{"default": sr.PolicyType_DEFAULT, "spray": sr.PolicyType_SPRAY, "tef": sr.PolicyType_TEF}

// Srv6BehaviorName is the document spelling of b ("" for an unknown behaviour).
func Srv6BehaviorName(b sr.Behavior) string {
	for n, v := range srv6Behaviors {
		if v == b {
			return n
		}
	}
	return ""
}

// Srv6PolicyTypeName is the document spelling of t.
func Srv6PolicyTypeName(t sr.PolicyType) string {
	for n, v := range srv6PolicyTypes {
		if v == t {
			return n
		}
	}
	return ""
}

func needsSrv6Interface(b sr.Behavior) bool {
	return b == sr.Behavior_END_X || b == sr.Behavior_END_DX2 || b == sr.Behavior_END_DX4 || b == sr.Behavior_END_DX6
}

func needsSrv6NextHop(b sr.Behavior) bool {
	return b == sr.Behavior_END_X || b == sr.Behavior_END_DX4 || b == sr.Behavior_END_DX6
}

func needsSrv6Lookup(b sr.Behavior) bool {
	return b == sr.Behavior_END_T || b == sr.Behavior_END_DT4 || b == sr.Behavior_END_DT6
}

// canonIP6 parses a unicast IPv6 address and returns its canonical text.
func canonIP6(s string) (string, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil || !a.Is6() || a.Is4In6() || a.Zone() != "" || a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() {
		return "", false
	}
	return a.String(), true
}

// Srv6 projects routing.srv6 (when the routing domain is authoritative).
func Srv6(s Sink, ds *vrxv1.DesiredState, in map[string]bool, vrfID func(string) (uint32, bool)) {
	if !in["routing"] {
		return
	}
	cfg := ds.GetRouting().GetSrv6()
	if cfg == nil {
		return
	}
	root := func(segs ...string) string { return Ptr(append([]string{"routing", "srv6"}, segs...)...) }
	table := func(vrf *string, pt string) (uint32, bool) {
		name := "default"
		if vrf != nil {
			name = *vrf
		}
		id, ok := vrfID(name)
		if !ok {
			s.Errorf(pt, srv6Rule+"-vrf-exists", "VRF %q does not exist", name)
		}
		return id, ok
	}

	globalSrc := ""
	if cfg.EncapSource != nil {
		pt := root("encapSource")
		if src, ok := canonIP6(cfg.GetEncapSource()); ok {
			globalSrc = src
			s.Add(scheduler.Join(sr.EncapSourceName, "global"), &sr.EncapSource{Address: src}, pt)
			s.Warnf(pt, srv6Unsupported, "routing.srv6.encapSource is a write-only VPP global (no getter, D-063): the globals owner applies it and every other agent only requires it (D-071); Retrieve never reports it")
		} else {
			s.Errorf(pt, srv6Rule+"-sid-address", "encapSource %q must be a unicast IPv6 address", cfg.GetEncapSource())
		}
	}
	if cfg.EncapHopLimit != nil {
		pt := root("encapHopLimit")
		if h := cfg.GetEncapHopLimit(); h >= 1 && h <= 255 {
			s.Add(scheduler.Join(sr.EncapHopLimitName, "global"), &sr.EncapHopLimit{HopLimit: h}, pt)
			s.Warnf(pt, srv6Unsupported, "routing.srv6.encapHopLimit is a write-only VPP global (no getter, D-063): the globals owner applies it and every other agent only requires it (D-071); Retrieve never reports it")
		} else {
			s.Errorf(pt, srv6Rule, "encapHopLimit %d must be 1–255", h)
		}
	}

	for _, key := range sortedKeys(cfg.GetLocalSids()) {
		l := cfg.GetLocalSids()[key]
		pt := root("localSids", key)
		sid, ok := canonIP6(key)
		if !ok || sid != key {
			s.Errorf(pt, srv6Rule+"-canonical", "local SID %q must be a unicast IPv6 address in canonical form", key)
			continue
		}
		b, ok := srv6Behaviors[l.GetBehavior()]
		if !ok {
			s.Errorf(root("localSids", key, "behavior"), srv6Rule, "behavior %q is not one of end, end.x, end.t, end.dx2, end.dx4, end.dx6, end.dt4, end.dt6 (End.AD/AM/AS have no VPP binary API)", l.GetBehavior())
			continue
		}
		v := &sr.LocalSid{Sid: sid, Behavior: b, EndPsp: l.GetPsp()}
		bad := false
		fib, ok := table(l.Vrf, root("localSids", key, "vrf"))
		bad = bad || !ok
		v.FibTable = fib
		if needsSrv6Interface(b) != (l.Interface != nil) {
			s.Errorf(root("localSids", key, "interface"), srv6Rule+"-behavior-fields", "%s: interface is required for end.x/end.dx2/end.dx4/end.dx6 and not allowed otherwise", l.GetBehavior())
			bad = true
		}
		v.Interface = l.GetInterface()
		if needsSrv6NextHop(b) != (l.NextHop != nil) {
			s.Errorf(root("localSids", key, "nextHop"), srv6Rule+"-behavior-fields", "%s: nextHop is required for end.x/end.dx4/end.dx6 and not allowed otherwise", l.GetBehavior())
			bad = true
		} else if l.NextHop != nil {
			nh, err := netip.ParseAddr(l.GetNextHop())
			if err != nil || nh.Is4() != (b == sr.Behavior_END_DX4) || nh.String() != l.GetNextHop() {
				s.Errorf(root("localSids", key, "nextHop"), srv6Rule+"-behavior-fields", "%s: nextHop %q must be a canonical %s address", l.GetBehavior(), l.GetNextHop(), map[bool]string{true: "IPv4", false: "IPv6"}[b == sr.Behavior_END_DX4])
				bad = true
			} else {
				v.NextHop = nh.String()
			}
		}
		if needsSrv6Lookup(b) != (l.LookupVrf != nil) {
			s.Errorf(root("localSids", key, "lookupVrf"), srv6Rule+"-behavior-fields", "%s: lookupVrf is required for end.t/end.dt4/end.dt6 and not allowed otherwise", l.GetBehavior())
			bad = true
		} else if l.LookupVrf != nil {
			id, ok := table(l.LookupVrf, root("localSids", key, "lookupVrf"))
			bad = bad || !ok
			v.LookupTable = id
		}
		if l.GetPsp() && b != sr.Behavior_END && b != sr.Behavior_END_X && b != sr.Behavior_END_T {
			s.Errorf(root("localSids", key, "psp"), srv6Rule+"-behavior-fields", "%s does not support PSP (only end, end.x, end.t)", l.GetBehavior())
			bad = true
		}
		if !bad {
			s.Add(scheduler.Join(sr.LocalSidName, sid), v, pt)
		}
	}

	policies := map[string]bool{} // canonical BSID → encap
	for _, key := range sortedKeys(cfg.GetPolicies()) {
		p := cfg.GetPolicies()[key]
		pt := root("policies", key)
		bsid, ok := canonIP6(key)
		if !ok || bsid != key {
			s.Errorf(pt, srv6Rule+"-canonical", "binding SID %q must be a unicast IPv6 address in canonical form", key)
			continue
		}
		if _, dup := cfg.GetLocalSids()[bsid]; dup {
			s.Errorf(pt, srv6Rule+"-sid-unique", "%s is also a local SID", bsid)
			continue
		}
		typ := sr.PolicyType_DEFAULT
		if p.Type != nil {
			t, ok := srv6PolicyTypes[p.GetType()]
			if !ok {
				s.Errorf(root("policies", key, "type"), srv6Rule, "type %q is not one of default, spray, tef", p.GetType())
				continue
			}
			typ = t
		}
		encap := p.Encap == nil || p.GetEncap() // Zod default true
		v := &sr.Policy{Bsid: bsid, Type: typ, Encap: encap}
		bad := false
		fib, ok := table(p.Vrf, root("policies", key, "vrf"))
		bad = bad || !ok
		v.FibTable = fib
		switch {
		case encap && p.EncapSource != nil:
			src, ok := canonIP6(p.GetEncapSource())
			if !ok || src != p.GetEncapSource() {
				s.Errorf(root("policies", key, "encapSource"), srv6Rule+"-sid-address", "encapSource %q must be a unicast IPv6 address in canonical form", p.GetEncapSource())
				bad = true
			}
			v.EncapSrc = src
		case encap && globalSrc != "":
			v.EncapSrc = globalSrc
		case encap:
			s.Errorf(root("policies", key, "encapSource"), srv6Rule+"-encap-source", "an encapsulating policy needs an outer source address: set encapSource here or routing.srv6.encapSource (D-074)")
			bad = true
		case p.EncapSource != nil:
			s.Errorf(root("policies", key, "encapSource"), srv6Rule+"-encap-source", "an insert policy (encap off) has no outer header: remove encapSource")
			bad = true
		}
		if len(p.GetSidLists()) == 0 {
			s.Errorf(root("policies", key, "sidLists"), srv6Rule, "a policy needs at least one segment list")
			bad = true
		}
		for i, l := range p.GetSidLists() {
			lp := root("policies", key, "sidLists", strconv.Itoa(i))
			if n := len(l.GetSids()); n == 0 || n > sr.MaxSids {
				s.Errorf(lp+"/sids", srv6Rule, "a segment list has 1–%d SIDs, not %d", sr.MaxSids, n)
				bad = true
				continue
			}
			out := &sr.SidList{Weight: 1}
			if l.Weight != nil {
				out.Weight = l.GetWeight()
			}
			for j, sid := range l.GetSids() {
				c, ok := canonIP6(sid)
				if !ok || c != sid {
					s.Errorf(lp+"/sids/"+strconv.Itoa(j), srv6Rule+"-canonical", "segment %q must be a unicast IPv6 address in canonical form", sid)
					bad = true
				}
				out.Sids = append(out.Sids, c)
			}
			v.SidLists = append(v.SidLists, out)
		}
		if !bad {
			policies[bsid] = encap
			s.Add(sr.PolicyKey(bsid), v, pt)
		}
	}

	seen := map[string]int{}
	for i, st := range cfg.GetSteering() {
		pt := root("steering", strconv.Itoa(i))
		bsid, ok := canonIP6(st.GetBsid())
		if !ok || bsid != st.GetBsid() {
			s.Errorf(pt+"/bsid", srv6Rule+"-canonical", "binding SID %q must be a unicast IPv6 address in canonical form", st.GetBsid())
			continue
		}
		encap, exists := policies[bsid]
		if _, configured := cfg.GetPolicies()[bsid]; !configured {
			s.Errorf(pt+"/bsid", srv6Rule+"-steering-bsid", "no policy with binding SID %s", bsid)
			continue
		}
		v := &sr.Steering{Bsid: bsid}
		switch st.GetType() {
		case "l3":
			if st.Interface != nil {
				s.Errorf(pt+"/interface", srv6Rule, "an l3 steering entry takes prefix and vrf, not interface")
				continue
			}
			pfx, err := netip.ParsePrefix(st.GetPrefix())
			if err != nil || pfx.Masked() != pfx || pfx.String() != st.GetPrefix() {
				s.Errorf(pt+"/prefix", srv6Rule+"-canonical", "prefix %q must be a network prefix in canonical form", st.GetPrefix())
				continue
			}
			id, ok := table(st.Vrf, pt+"/vrf")
			if !ok {
				continue
			}
			v.TrafficType, v.Prefix, v.TableId = sr.SteerType_IPV6, pfx.String(), id
			if pfx.Addr().Is4() {
				v.TrafficType = sr.SteerType_IPV4
			}
			if exists && !encap && v.TrafficType == sr.SteerType_IPV4 {
				s.Errorf(pt+"/bsid", srv6Rule+"-steering-encap", "IPv4 steering needs an encapsulating policy; %s inserts (encap off)", bsid)
				continue
			}
		case "l2":
			if st.Prefix != nil || st.Vrf != nil || st.GetInterface() == "" {
				s.Errorf(pt, srv6Rule, "an l2 steering entry takes interface only")
				continue
			}
			v.TrafficType, v.Interface = sr.SteerType_L2, st.GetInterface()
			if exists && !encap {
				s.Errorf(pt+"/bsid", srv6Rule+"-steering-encap", "L2 steering needs an encapsulating policy; %s inserts (encap off)", bsid)
				continue
			}
		default:
			s.Errorf(pt+"/type", srv6Rule, "steering type %q is not l3 or l2", st.GetType())
			continue
		}
		id := sr.SteeringID(v)
		if first, dup := seen[id]; dup {
			s.Errorf(pt, srv6Rule+"-steering-unique", "steered twice (first defined at %s)", root("steering", strconv.Itoa(first)))
			continue
		}
		seen[id] = i
		s.Add(scheduler.Join(sr.SteeringName, id), v, pt)
	}
}

// AssembleSrv6 fills routing.srv6 from the retrieved SR objects (claimed ones only: the sr descriptors'
// Retrieve), in canonical form: every leaf VPP reports is set (psp, vrf, type, encap, weight), the
// optional ones only when the object carries them; steering sorted — L3 by VRF name then prefix, then
// L2 by interface (the UI saves it in this order). The globals are write-only and never reported.
func AssembleSrv6(ds *vrxv1.DesiredState, kvs []scheduler.KV, in map[string]bool, tableName func(uint32) string, env Srv6Env) {
	if !in["routing"] {
		return
	}
	applied := ""
	if env.EncapSource != nil {
		applied = env.EncapSource()
	}
	out := &vrxv1.Srv6Config{}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *sr.LocalSid:
			name := Srv6BehaviorName(v.GetBehavior())
			if name == "" {
				continue
			}
			l := &vrxv1.Srv6LocalSid{Behavior: proto.String(name), Psp: proto.Bool(v.GetEndPsp()), Vrf: proto.String(tableName(v.GetFibTable()))}
			if v.GetInterface() != "" {
				l.Interface = proto.String(v.GetInterface())
			}
			if v.GetNextHop() != "" {
				l.NextHop = proto.String(v.GetNextHop())
			}
			if needsSrv6Lookup(v.GetBehavior()) {
				l.LookupVrf = proto.String(tableName(v.GetLookupTable()))
			}
			if out.LocalSids == nil {
				out.LocalSids = map[string]*vrxv1.Srv6LocalSid{}
			}
			out.LocalSids[v.GetSid()] = l
		case *sr.Policy:
			p := &vrxv1.Srv6Policy{Type: proto.String(Srv6PolicyTypeName(v.GetType())), Encap: proto.Bool(v.GetEncap()), Vrf: proto.String(tableName(v.GetFibTable()))}
			if v.GetEncap() && v.GetEncapSrc() != "" && v.GetEncapSrc() != applied {
				p.EncapSource = proto.String(v.GetEncapSrc())
			}
			for _, l := range v.GetSidLists() {
				p.SidLists = append(p.SidLists, &vrxv1.Srv6SidList{Sids: append([]string(nil), l.GetSids()...), Weight: proto.Uint32(l.GetWeight())})
			}
			if out.Policies == nil {
				out.Policies = map[string]*vrxv1.Srv6Policy{}
			}
			out.Policies[v.GetBsid()] = p
		case *sr.Steering:
			st := &vrxv1.Srv6Steering{Bsid: proto.String(v.GetBsid())}
			if v.GetTrafficType() == sr.SteerType_L2 {
				st.Type, st.Interface = proto.String("l2"), proto.String(v.GetInterface())
			} else {
				st.Type, st.Prefix, st.Vrf = proto.String("l3"), proto.String(v.GetPrefix()), proto.String(tableName(v.GetTableId()))
			}
			out.Steering = append(out.Steering, st)
		}
	}
	sort.SliceStable(out.Steering, func(i, j int) bool { return steeringLess(out.Steering[i], out.Steering[j]) })
	if len(out.LocalSids) == 0 && len(out.Policies) == 0 && len(out.Steering) == 0 {
		return
	}
	if ds.Routing == nil {
		ds.Routing = &vrxv1.RoutingConfig{}
	}
	ds.Routing.Srv6 = out
}

// steeringLess orders L3 entries (by VRF, then prefix) before L2 entries (by interface).
func steeringLess(a, b *vrxv1.Srv6Steering) bool {
	if a.GetType() != b.GetType() {
		return a.GetType() == "l3"
	}
	if a.GetType() == "l2" {
		return a.GetInterface() < b.GetInterface()
	}
	if a.GetVrf() != b.GetVrf() {
		return a.GetVrf() < b.GetVrf()
	}
	return a.GetPrefix() < b.GetPrefix()
}
