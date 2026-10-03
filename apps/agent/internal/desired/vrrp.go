package desired

// F-vrrp-config-sync: the desired-state builder and assembler of `ha.vrrp` over DF-7's VPP vrrp
// descriptors (engine "vpp", docs/agent/descriptors/vrrp.md) and RF-4's keepalived renderer (engine
// "keepalived", docs/agent/renderers/keepalived.md, through the keepalived.config stage in
// internal/subsystems/keepalived.go):
//
//	ha.vrrp.<name> (engine vpp)         → vrrp.vr/<if>/<vrid>/<af>                   (dep interface/<if>)
//	  unicast.peers                     → vrrp.vr-peers/<if>/<vrid>/<af>
//	  track[]                           → vrrp.vr-track-interface/<if>/<vrid>/<af>/<tracked>
//	  enabled (default true)            → vrrp.vr-state/<if>/<vrid>/<af>             (started; absent = stopped)
//	  name, description, vrf            → vrrp.meta/<if>/<vrid>/<af>                 (agent-local: VPP holds no name)
//	ha.vrrp.<name> (engine keepalived)  → keepalived.config/ngfw                      (one singleton: every keepalived instance
//	                                                                                  with a linux-cp pair, plus those pairs)
//
// A keepalived instance whose interface has no linux-cp pair (interfaces.<if>.lcp) is skipped with an
// `ha.vrrp-keepalived-no-lcp` warning (DryRun shows it) — never silently dropped, never bound to a Linux
// interface that merely shares the VPP name (RF-4 NoMapper rule). A disabled keepalived instance is kept in
// the stage's value (the renderer renders only enabled ones), so Retrieve round-trips it.
//
// VRRP needs no VPP id (the VR is keyed by interface + VRID + family), so the TD-8b id range does not apply;
// ownership is the owner of the VR's interface (DF-7).

import (
	"context"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/scheduler"
)

const (
	// VrrpMetaName is the agent-local descriptor holding what VPP cannot: the name, description and VRF
	// of every VPP-engine virtual router this agent created.
	VrrpMetaName = "vrrp.meta"
	// KeepalivedConfigName is the keepalived renderer stage (singleton, subsystems/keepalived.go).
	KeepalivedConfigName = "keepalived.config"

	vrrpRule         = "ha.vrrp"
	vrrpRuleNoLcp    = "ha.vrrp-keepalived-no-lcp"
	vrrpRuleDup      = "ha.vrrp-duplicate-vrid"
	vrrpEngineVPP    = "vpp"
	vrrpEngineKeepal = "keepalived"
)

// KeepalivedKey is the single key of the keepalived stage.
var KeepalivedKey = scheduler.Join(KeepalivedConfigName, "ngfw")

// ---- vrrp.meta ----------------------------------------------------------------------------------

// VrrpMetaSpec is one vrrp.meta entry; ID is "<interface>/<vrid>/<ipv4|ipv6>".
type VrrpMetaSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Vrf         string `json:"vrf,omitempty"`
}

func vrrpID(v vrrp.VR) string {
	af := "ipv4"
	if v.IPv6 {
		af = "ipv6"
	}
	return v.Interface + "/" + strconv.Itoa(int(v.VRID)) + "/" + af
}

// VrrpMetaKey is "vrrp.meta/<interface>/<vrid>/<af>".
func VrrpMetaKey(id string) scheduler.Key { return scheduler.Join(VrrpMetaName, id) }

// VrrpMetaStore persists the table (subsystems: a JSON file in the state dir). Load returns a copy.
type VrrpMetaStore interface {
	Load() (map[string]VrrpMetaSpec, error)
	Save(map[string]VrrpMetaSpec) error
}

// VrrpMeta is the vrrp.meta descriptor: no VPP call; journaled and rolled back like any other.
type VrrpMeta struct{ store VrrpMetaStore }

// NewVrrpMeta returns the descriptor over store.
func NewVrrpMeta(store VrrpMetaStore) *VrrpMeta { return &VrrpMeta{store: store} }

// Name implements scheduler.Descriptor.
func (*VrrpMeta) Name() string { return VrrpMetaName }

// CheckPersistent (TD-11b): the table must survive an agent restart, or Retrieve loses the names.
func (d *VrrpMeta) CheckPersistent() error {
	if p, ok := d.store.(interface{ Persistent() bool }); ok && p.Persistent() {
		return nil
	}
	return dfkit.Specf("%s: metadata store %T does not survive an agent restart", VrrpMetaName, d.store)
}

func decodeVrrpMeta(v proto.Message) (VrrpMetaSpec, error) {
	var s VrrpMetaSpec
	err := dfkit.Decode(v, &s)
	return s, err
}

// KeyOf implements scheduler.Descriptor.
func (*VrrpMeta) KeyOf(obj proto.Message) scheduler.Key {
	s, _ := decodeVrrpMeta(obj)
	return VrrpMetaKey(s.ID)
}

// Dependencies implements scheduler.Descriptor (none: agent-local).
func (*VrrpMeta) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *VrrpMeta) put(obj proto.Message) error {
	s, err := decodeVrrpMeta(obj)
	if err != nil {
		return err
	}
	if s.ID == "" || s.Name == "" {
		return dfkit.Specf("%s: id and name are required", VrrpMetaName)
	}
	m, err := d.store.Load()
	if err != nil {
		return err
	}
	m[s.ID] = s
	return d.store.Save(m)
}

// Create implements scheduler.Descriptor.
func (d *VrrpMeta) Create(_ context.Context, obj proto.Message) (any, error) { return nil, d.put(obj) }

// Update implements scheduler.Descriptor.
func (d *VrrpMeta) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.put(newObj)
}

// Delete implements scheduler.Descriptor.
func (d *VrrpMeta) Delete(_ context.Context, obj proto.Message, _ any) error {
	s, err := decodeVrrpMeta(obj)
	if err != nil {
		return err
	}
	m, err := d.store.Load()
	if err != nil {
		return err
	}
	if _, ok := m[s.ID]; !ok {
		return nil
	}
	delete(m, s.ID)
	return d.store.Save(m)
}

// Retrieve implements scheduler.Descriptor: the whole table, sorted by id.
func (d *VrrpMeta) Retrieve(context.Context) ([]scheduler.KV, error) {
	m, err := d.store.Load()
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(m))
	for _, id := range sortedKeys(m) {
		out = append(out, scheduler.KV{Key: VrrpMetaKey(id), Value: dfkit.Encode(m[id])})
	}
	return out, nil
}

// ---- builder ------------------------------------------------------------------------------------

func canonAddrs(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, a := range in {
		p, err := netip.ParseAddr(a)
		if err != nil {
			return nil, err
		}
		out = append(out, p.Unmap().String())
	}
	return df7.SortedAddrs(out)
}

func vrrpEngine(v *ngfwv1.VrrpInstance) string {
	if v.Engine == nil {
		return vrrpEngineVPP
	}
	return v.GetEngine()
}

// Vrrp projects ha.vrrp (both engines). in is the set of authoritative domains of the transaction.
func Vrrp(s Sink, ds *ngfwv1.DesiredState, in map[string]bool) {
	if !in["ha"] {
		return
	}
	ha := ds.GetHa()
	names := make([]string, 0, len(ha.GetVrrp()))
	for n := range ha.GetVrrp() {
		names = append(names, n)
	}
	sort.Strings(names)
	seen := map[string]string{}
	lcp := lcpmap.FromDesired(ds)
	keep := map[string]*ngfwv1.VrrpInstance{}
	pairs := map[string]*ngfwv1.Interface{}
	for _, name := range names {
		v := ha.GetVrrp()[name]
		pt := Ptr("ha", "vrrp", name)
		vr := vrrp.VR{Interface: v.GetInterface(), VRID: uint8(v.GetVrId()), IPv6: v.GetAddressFamily() == "ipv6"} //nolint:gosec // 1–255 checked below
		if v.GetInterface() == "" || v.GetVrId() < 1 || v.GetVrId() > 255 {
			s.Errorf(pt, vrrpRule, "a virtual router needs an interface and a VRID 1–255")
			continue
		}
		if other, dup := seen[vrrpID(vr)]; dup {
			s.Errorf(Ptr("ha", "vrrp", name, "vrId"), vrrpRuleDup, "ha.vrrp.%s and ha.vrrp.%s share interface %s, address family and VRID %d", other, name, vr.Interface, vr.VRID)
			continue
		}
		seen[vrrpID(vr)] = name
		switch vrrpEngine(v) {
		case vrrpEngineKeepal:
			if f := v.GetVrf(); f != "" && f != "default" {
				s.Warnf(Ptr("ha", "vrrp", name, "vrf"), "ha.vrrp-keepalived-vrf", "keepalived instance %s is skipped: the keepalived engine supports only the default VRF (got %q)", name, f)
				continue
			}
			if _, ok := lcp[v.GetInterface()]; !ok {
				s.Warnf(Ptr("ha", "vrrp", name, "interface"), vrrpRuleNoLcp,
					"keepalived instance %s is skipped: interface %s has no linux-cp pair (interfaces.%s.lcp), keepalived has no Linux interface to bind to", name, v.GetInterface(), v.GetInterface())
				continue
			}
			keep[name] = v
			pairs[v.GetInterface()] = &ngfwv1.Interface{Lcp: ds.GetInterfaces()[v.GetInterface()].GetLcp()}
		case vrrpEngineVPP:
			vrrpVPP(s, pt, name, vr, v)
		default:
			s.Errorf(Ptr("ha", "vrrp", name, "engine"), vrrpRule, "unknown engine %q (vpp | keepalived)", v.GetEngine())
		}
	}
	if len(keep) > 0 {
		val := &ngfwv1.DesiredState{Ha: &ngfwv1.HaConfig{Vrrp: keep}, Interfaces: pairs}
		if h := ds.GetSystem().GetHostname(); h != "" {
			val.System = &ngfwv1.SystemConfig{Hostname: proto.String(h)}
		}
		s.Add(KeepalivedKey, val, Ptr("ha", "vrrp"))
	}
}

func vrrpVPP(s Sink, pt, name string, vr vrrp.VR, v *ngfwv1.VrrpInstance) {
	addrs, err := canonAddrs(v.GetAddresses())
	if err != nil || len(addrs) == 0 {
		s.Errorf(Ptr("ha", "vrrp", name, "addresses"), vrrpRule, "virtual addresses: %v", err)
		return
	}
	interval := uint32(1000)
	if v.AdvertisementIntervalMs != nil {
		interval = v.GetAdvertisementIntervalMs()
	}
	prio := uint32(100)
	if v.Priority != nil {
		prio = v.GetPriority()
	}
	if interval%10 != 0 || interval/10 < 1 || interval/10 > 4095 || prio < 1 || prio > 255 {
		s.Errorf(pt, vrrpRule, "priority must be 1–255 and the advertisement interval a multiple of 10 ms in 10–40950")
		return
	}
	spec := vrrp.VRSpec{
		VR: vr, Priority: uint8(prio), Interval: uint16(interval / 10), //nolint:gosec // ranges checked above
		Preempt: boolOr(v.Preempt, true), Accept: v.GetAcceptMode(), Unicast: v.GetUnicast() != nil, Addresses: addrs,
	}
	if err := spec.Validate(); err != nil {
		s.Errorf(pt, vrrpRule, "%v", err)
		return
	}
	s.Add(vrrp.KeyVR(vr), df7.Encode(spec), pt)
	if v.GetUnicast() != nil {
		peers, err := canonAddrs(v.GetUnicast().GetPeers())
		if err != nil || len(peers) == 0 {
			s.Errorf(Ptr("ha", "vrrp", name, "unicast", "peers"), vrrpRule, "unicast peers: %v", err)
			return
		}
		s.Add(vrrp.KeyPeers(vr), df7.Encode(vrrp.Peers{VR: vr, Peers: peers}), Ptr("ha", "vrrp", name, "unicast"))
	}
	for i, t := range v.GetTrack() {
		dec := uint32(10)
		if t.PriorityDecrement != nil {
			dec = t.GetPriorityDecrement()
		}
		tr := vrrp.Track{VR: vr, Tracked: t.GetInterface(), Priority: uint8(min(dec, 255))} //nolint:gosec // clamped
		tp := Ptr("ha", "vrrp", name, "track", strconv.Itoa(i))
		if err := tr.Validate(); err != nil {
			s.Errorf(tp, vrrpRule, "%v", err)
			continue
		}
		s.Add(vrrp.KeyTrack(vr, tr.Tracked), df7.Encode(tr), tp)
	}
	if boolOr(v.Enabled, true) {
		s.Add(vrrp.KeyState(vr), df7.Encode(vrrp.State{VR: vr, Running: true}), Ptr("ha", "vrrp", name, "enabled"))
	}
	s.Add(VrrpMetaKey(vrrpID(vr)), dfkit.Encode(VrrpMetaSpec{ID: vrrpID(vr), Name: name, Description: v.GetDescription(), Vrf: v.GetVrf()}), pt)
}

// ---- assembler ----------------------------------------------------------------------------------

var vrrpNameClean = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// vrrpFallbackName names a retrieved VR without a vrrp.meta entry (created before this agent build, or
// the metadata file was lost): vr-<interface>-<vrid>-<af>, restricted to objectName characters.
func vrrpFallbackName(vr vrrp.VR) string {
	return vrrpNameClean.ReplaceAllString("vr-"+strings.ReplaceAll(vrrpID(vr), "/", "-"), "-")
}

// AssembleVrrp adds ha.vrrp to the assembled document from the retrieved vrrp.*, vrrp.meta and
// keepalived.config objects.
func AssembleVrrp(ds *ngfwv1.DesiredState, kvs []scheduler.KV, in map[string]bool) {
	if !in["ha"] {
		return
	}
	type acc struct {
		spec    vrrp.VRSpec
		peers   []string
		track   []vrrp.Track
		running bool
	}
	vrs := map[string]*acc{}
	meta := map[string]VrrpMetaSpec{}
	out := map[string]*ngfwv1.VrrpInstance{}
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case vrrp.NameVR:
			if sp, err := df7.Decode[vrrp.VRSpec](kv.Value); err == nil {
				vrs[vrrpID(sp.VR)] = &acc{spec: sp}
			}
		case VrrpMetaName:
			if m, err := decodeVrrpMeta(kv.Value); err == nil {
				meta[m.ID] = m
			}
		case KeepalivedConfigName:
			if k, ok := kv.Value.(*ngfwv1.DesiredState); ok {
				for n, v := range k.GetHa().GetVrrp() {
					out[n] = proto.Clone(v).(*ngfwv1.VrrpInstance)
				}
			}
		}
	}
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case vrrp.NamePeers:
			if p, err := df7.Decode[vrrp.Peers](kv.Value); err == nil && vrs[vrrpID(p.VR)] != nil {
				vrs[vrrpID(p.VR)].peers = p.Peers
			}
		case vrrp.NameTrack:
			if t, err := df7.Decode[vrrp.Track](kv.Value); err == nil && vrs[vrrpID(t.VR)] != nil {
				vrs[vrrpID(t.VR)].track = append(vrs[vrrpID(t.VR)].track, t)
			}
		case vrrp.NameState:
			if st, err := df7.Decode[vrrp.State](kv.Value); err == nil && vrs[vrrpID(st.VR)] != nil {
				vrs[vrrpID(st.VR)].running = st.Running
			}
		}
	}
	for id, a := range vrs {
		sp := a.spec
		af := "ipv4"
		if sp.IPv6 {
			af = "ipv6"
		}
		inst := &ngfwv1.VrrpInstance{
			Enabled: proto.Bool(a.running), Interface: proto.String(sp.Interface), VrId: proto.Uint32(uint32(sp.VRID)),
			AddressFamily: proto.String(af), Priority: proto.Uint32(uint32(sp.Priority)),
			AdvertisementIntervalMs: proto.Uint32(uint32(sp.Interval) * 10), Preempt: proto.Bool(sp.Preempt),
			AcceptMode: proto.Bool(sp.Accept), Addresses: append([]string(nil), sp.Addresses...), Engine: proto.String(vrrpEngineVPP),
		}
		if sp.Unicast {
			inst.Unicast = &ngfwv1.VrrpInstance_Unicast{Peers: a.peers}
		}
		sort.Slice(a.track, func(i, j int) bool { return a.track[i].Tracked < a.track[j].Tracked })
		for _, t := range a.track {
			inst.Track = append(inst.Track, &ngfwv1.VrrpInstance_Track{Interface: proto.String(t.Tracked), PriorityDecrement: proto.Uint32(uint32(t.Priority))})
		}
		name := vrrpFallbackName(sp.VR)
		if m, ok := meta[id]; ok {
			name = m.Name
			if m.Description != "" {
				inst.Description = proto.String(m.Description)
			}
			if m.Vrf != "" {
				inst.Vrf = proto.String(m.Vrf)
			}
		}
		out[name] = inst
	}
	if len(out) == 0 {
		return
	}
	if ds.Ha == nil {
		ds.Ha = &ngfwv1.HaConfig{}
	}
	ds.Ha.Vrrp = out
}
