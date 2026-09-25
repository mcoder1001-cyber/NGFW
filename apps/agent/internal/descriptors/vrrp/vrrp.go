// Package vrrp holds the reconciler descriptors for the VPP VRRPv3 plugin (task DF-7, WBS
// D9.1): virtual routers (vrrp_vr_update creates and updates, vrrp_vr_add_del deletes;
// retrieved with vrrp_vr_dump), unicast peers (vrrp_vr_set_peers / vrrp_vr_peer_dump), tracked
// interfaces (vrrp_vr_track_if_add_del / vrrp_vr_track_if_dump) and the running state
// (vrrp_vr_start_stop, read back from the runtime state in vrrp_vr_dump). WatchEvents turns
// want_vrrp_vr_events + vrrp_vr_event into master/backup transitions.
//
// Messages come only from apps/agent/binapi/vrrp. Ownership: a VR (and its peers, tracking and
// state) belongs to the owner of its interface. docs/agent/descriptors/vrrp.md is the object ↔
// message table.
package vrrp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/vrrp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameVR    = "vrrp.vr"
	NamePeers = "vrrp.vr-peers"
	NameTrack = "vrrp.vr-track-interface"
	NameState = "vrrp.vr-state"
)

// VR identifies a virtual router: interface, VR id (1–255) and address family.
type VR struct {
	Interface string `json:"interface,omitempty"`
	VRID      uint8  `json:"vr_id,omitempty"`
	IPv6      bool   `json:"ipv6,omitempty"`
}

func (v VR) validate() error {
	if v.Interface == "" || v.VRID == 0 {
		return df7.Specf("vrrp VR needs an interface and a vr_id 1–255")
	}
	return nil
}

func (v VR) id() []string {
	af := "ipv4"
	if v.IPv6 {
		af = "ipv6"
	}
	return []string{v.Interface, strconv.Itoa(int(v.VRID)), af}
}

func isIPv6(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

// VRSpec is the desired state of one vrrp.vr object. Interval is the advertisement interval in
// centiseconds (VRRPv3, default 100). Addresses are the virtual addresses, canonical and
// sorted.
type VRSpec struct {
	VR
	Priority  uint8    `json:"priority,omitempty"`
	Interval  uint16   `json:"interval,omitempty"`
	Preempt   bool     `json:"preempt,omitempty"`
	Accept    bool     `json:"accept,omitempty"`
	Unicast   bool     `json:"unicast,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
}

// Validate checks v.
func (v VRSpec) Validate() error {
	if err := v.validate(); err != nil {
		return err
	}
	if v.Priority == 0 || v.Interval == 0 {
		return df7.Specf("vrrp priority and interval must be > 0")
	}
	if len(v.Addresses) == 0 || len(v.Addresses) > 255 {
		return df7.Specf("vrrp VR needs 1–255 virtual addresses")
	}
	sorted, err := df7.SortedAddrs(v.Addresses)
	if err != nil {
		return err
	}
	for i, a := range sorted {
		if a != v.Addresses[i] {
			return df7.Specf("vrrp addresses must be canonical and sorted (%v)", sorted)
		}
		if p, _ := df7.ParseAddr(a); p.Is6() != v.IPv6 {
			return df7.Specf("vrrp address %s does not match the VR family", a)
		}
	}
	return nil
}

// Peers is the desired state of one vrrp.vr-peers object (unicast VRs): the peer addresses,
// canonical and sorted.
type Peers struct {
	VR
	Peers []string `json:"peers,omitempty"`
}

// Validate checks p.
func (p Peers) Validate() error {
	if err := p.validate(); err != nil {
		return err
	}
	if len(p.Peers) == 0 {
		return df7.Specf("vrrp peers list is empty; omit the object instead")
	}
	sorted, err := df7.SortedAddrs(p.Peers)
	if err != nil {
		return err
	}
	for i, a := range sorted {
		if a != p.Peers[i] {
			return df7.Specf("vrrp peers must be canonical and sorted (%v)", sorted)
		}
	}
	return nil
}

// Track is the desired state of one vrrp.vr-track-interface object: while Tracked is down the
// VR's priority is lowered by Priority.
type Track struct {
	VR
	Tracked  string `json:"tracked,omitempty"`
	Priority uint8  `json:"priority,omitempty"`
}

// Validate checks t.
func (t Track) Validate() error {
	if err := t.validate(); err != nil {
		return err
	}
	if t.Tracked == "" || t.Tracked == t.Interface {
		return df7.Specf("vrrp track needs a tracked interface other than the VR's own")
	}
	if t.Priority == 0 {
		return df7.Specf("vrrp track priority decrement must be > 0")
	}
	return nil
}

// State is the desired state of one vrrp.vr-state object: the VR is started (Running must be
// true; omit the object to stop the VR — a stopped VR is reported as absent).
type State struct {
	VR
	Running bool `json:"running,omitempty"`
}

// Validate checks s.
func (s State) Validate() error {
	if err := s.validate(); err != nil {
		return err
	}
	if !s.Running {
		return df7.Specf("vrrp state running=false: omit the object to stop the VR (canonical form)")
	}
	return nil
}

// ---- keys -------------------------------------------------------------------------------------

// KeyVR is "vrrp.vr/<interface>/<vr id>/<ipv4|ipv6>".
func KeyVR(v VR) scheduler.Key { return scheduler.Join(NameVR, v.id()...) }

// KeyPeers is "vrrp.vr-peers/<interface>/<vr id>/<af>".
func KeyPeers(v VR) scheduler.Key { return scheduler.Join(NamePeers, v.id()...) }

// KeyTrack is "vrrp.vr-track-interface/<interface>/<vr id>/<af>/<tracked>".
func KeyTrack(v VR, tracked string) scheduler.Key {
	return scheduler.Join(NameTrack, append(v.id(), tracked)...)
}

// KeyState is "vrrp.vr-state/<interface>/<vr id>/<af>".
func KeyState(v VR) scheduler.Key { return scheduler.Join(NameState, v.id()...) }

// ---- shared -----------------------------------------------------------------------------------

// Meta of every vrrp object: the interface index and, when known, the VR's pool index
// (vrrp_vr_update reply; the dump does not report it).
type Meta struct {
	SwIfIndex uint32
	Index     uint32
	HasIndex  bool
}

func toAddrs(in []string) []ip_types.Address {
	out := make([]ip_types.Address, 0, len(in))
	for _, s := range in {
		a, _ := df7.ParseAddr(s) // validated
		out = append(out, df7.ToAddress(a))
	}
	return out
}

func fromAddrs(in []ip_types.Address) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		out = append(out, df7.FromAddress(a).String())
	}
	df7.SortAddrStrings(out)
	return out
}

func flags(v VRSpec) vrrp.VrrpVrFlags {
	var f vrrp.VrrpVrFlags
	if v.Preempt {
		f |= vrrp.VRRP_API_VR_PREEMPT
	}
	if v.Accept {
		f |= vrrp.VRRP_API_VR_ACCEPT
	}
	if v.Unicast {
		f |= vrrp.VRRP_API_VR_UNICAST
	}
	if v.IPv6 {
		f |= vrrp.VRRP_API_VR_IPV6
	}
	return f
}

// dumpVRs runs vrrp_vr_dump for all interfaces.
func dumpVRs(ctx context.Context, c vpp.Client) ([]*vrrp.VrrpVrDetails, error) {
	stream, err := vrrp.NewServiceClient(c).VrrpVrDump(ctx, &vrrp.VrrpVrDump{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex)})
	if err != nil {
		return nil, fmt.Errorf("vrrp_vr_dump: %w", df7.PluginError("vrrp", err))
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("vrrp_vr_dump: %w", err)
	}
	return dets, nil
}

// ownedVR is one VR of this owner as dumped.
type ownedVR struct {
	VR     VR
	Idx    uint32
	Detail *vrrp.VrrpVrDetails
}

func ownedVRs(ctx context.Context, b df7.Base) ([]ownedVR, *df7.Interfaces, error) {
	ifs, err := b.Ifaces(ctx)
	if err != nil {
		return nil, nil, err
	}
	dets, err := dumpVRs(ctx, b.Client)
	if err != nil {
		return nil, nil, err
	}
	var out []ownedVR
	for _, d := range dets {
		v6 := d.Config.Flags&vrrp.VRRP_API_VR_IPV6 != 0
		name, ok := ifs.Owned(uint32(d.Config.SwIfIndex), func(n string) string {
			return string(KeyVR(VR{Interface: n, VRID: d.Config.VrID, IPv6: v6}))
		})
		if !ok {
			continue
		}
		out = append(out, ownedVR{VR: VR{Interface: name, VRID: d.Config.VrID, IPv6: v6}, Idx: uint32(d.Config.SwIfIndex), Detail: d})
	}
	return out, ifs, nil
}

func sortKVs(kvs []scheduler.KV) {
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
}

func startStop(ctx context.Context, c vpp.Client, idx uint32, v VR, start bool) error {
	var s uint8
	if start {
		s = 1
	}
	_, err := vrrp.NewServiceClient(c).VrrpVrStartStop(ctx, &vrrp.VrrpVrStartStop{SwIfIndex: interface_types.InterfaceIndex(idx), VrID: v.VRID, IsIPv6: isIPv6(v.IPv6), IsStart: s})
	if err != nil {
		return fmt.Errorf("vrrp_vr_start_stop %s start=%v: %w", KeyVR(v), start, err)
	}
	return nil
}

func running(d *vrrp.VrrpVrDetails) bool { return d.Runtime.State != vrrp.VRRP_API_VR_STATE_INIT }

// vrIndex re-resolves the interface of a VR by logical name and checks the VR is ours (its
// interface is ours, or untagged and claimed by the VR's key) — D-071: never a stored index.
func vrIndex(ctx context.Context, b df7.Base, v VR) (uint32, bool, error) {
	tg, found, err := b.Detach(ctx, v.Interface, string(KeyVR(v)))
	return tg.Index, found, err
}

// mustVRIndex is vrIndex for operations on a VR that must exist.
func mustVRIndex(ctx context.Context, b df7.Base, v VR) (uint32, error) {
	idx, found, err := vrIndex(ctx, b, v)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, fmt.Errorf("%s: %w: %q", b.Name(), df7.ErrNoSuchInterface, v.Interface)
	}
	return idx, nil
}

// ---- vrrp.vr ----------------------------------------------------------------------------------

// VRDescriptor manages vrrp.vr objects.
type VRDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*VRDescriptor)(nil)

// NewVR returns the vrrp.vr descriptor.
func NewVR(c vpp.Client, owner string, opts ...df7.Option) *VRDescriptor {
	return &VRDescriptor{df7.NewBase(NameVR, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *VRDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	v, _ := df7.Decode[VRSpec](obj)
	return KeyVR(v.VR)
}

// Dependencies implements scheduler.Descriptor: the interface. (The VR's own source address
// comes from the interface; no interface-ip key is declared because the prefix is not part of
// the VR — docs/agent/descriptors/vrrp.md.)
func (d *VRDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	v, _ := df7.Decode[VRSpec](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(v.Interface)}
}

func (d *VRDescriptor) update(ctx context.Context, index uint32, idx uint32, v VRSpec) (uint32, error) {
	addrs := toAddrs(v.Addresses)
	rep, err := vrrp.NewServiceClient(d.Client).VrrpVrUpdate(ctx, &vrrp.VrrpVrUpdate{VrrpIndex: index, SwIfIndex: interface_types.InterfaceIndex(idx),
		VrID: v.VRID, Priority: v.Priority, Interval: v.Interval, Flags: flags(v), NAddrs: uint8(len(addrs)), Addrs: addrs}) //nolint:gosec // ≤ 255
	if err != nil {
		return 0, err
	}
	return rep.VrrpIndex, nil
}

// Create implements scheduler.Descriptor: vrrp_vr_update with an invalid index creates the VR
// and returns its pool index.
func (d *VRDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, err := df7.DecodeValid[VRSpec](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, v.Interface, string(KeyVR(v.VR)))
	if err != nil {
		return nil, err
	}
	idx := tg.Index
	index, err := d.update(ctx, df7.NoIndex, idx, v)
	if err != nil {
		// ENTRY_ALREADY_EXISTS: a VR with this key exists and is not ours (Retrieve would have
		// reported ours) — never adopted, nothing claimed (review M1)
		return nil, d.Wrap("vrrp_vr_update (create) "+string(KeyVR(v.VR)), df7.PluginError("vrrp", err))
	}
	return Meta{SwIfIndex: idx, Index: index, HasIndex: true}, tg.Claim()
}

// maxProbe bounds the pool-index search of Update after a restart.
const maxProbe = 4096

// Update implements scheduler.Descriptor: vrrp_vr_update on the VR's pool index (priority,
// interval, preempt/accept, addresses; VPP restarts a running VR itself). Switching unicast
// on or off is ErrRecreate (peers only exist on unicast VRs). After an agent restart the Meta
// from Retrieve has no index (the dump does not carry it): Update then walks the pool — an
// index of another VR is refused by VPP (INVALID_ARGUMENT, key mismatch) without effect, a
// free one answers NO_SUCH_ENTRY — until the VR's own index accepts the update. A stored index
// is tried first; VPP's own key check makes a stale one harmless (same two errors → walk).
func (d *VRDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, err := df7.Decode[VRSpec](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[VRSpec](newObj)
	if err != nil {
		return nil, err
	}
	if o.VR != n.VR || o.Unicast != n.Unicast {
		return nil, scheduler.ErrRecreate
	}
	sw, err := mustVRIndex(ctx, d.Base, n.VR)
	if err != nil {
		return nil, err
	}
	m := Meta{SwIfIndex: sw}
	if old, ok := meta.(Meta); ok && old.HasIndex {
		_, err := d.update(ctx, old.Index, sw, n)
		switch {
		case err == nil:
			m.Index, m.HasIndex = old.Index, true
			return m, nil
		case !df7.IsVPPError(err, api.INVALID_ARGUMENT, api.NO_SUCH_ENTRY):
			return nil, d.Wrap(fmt.Sprintf("vrrp_vr_update %d", old.Index), err)
		}
	}
	for i := uint32(0); i < maxProbe; i++ {
		_, err := d.update(ctx, i, sw, n)
		switch {
		case err == nil:
			m.Index, m.HasIndex = i, true
			return m, nil
		case df7.IsVPPError(err, api.INVALID_ARGUMENT, api.NO_SUCH_ENTRY):
			continue
		default:
			return nil, d.Wrap(fmt.Sprintf("vrrp_vr_update %d", i), err)
		}
	}
	return nil, fmt.Errorf("%s: pool index of %s not found in %d slots", NameVR, KeyVR(n.VR), maxProbe)
}

// Delete implements scheduler.Descriptor: vrrp_vr_add_del is_add=0 by key — interface
// re-resolved by logical name (D-071), NO_SUCH_ENTRY = already gone (tracking, peers and
// addresses go with the VR; the state object depends on the VR and stops it first).
func (d *VRDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	v, err := df7.Decode[VRSpec](obj)
	if err != nil {
		return err
	}
	tg, found, err := d.Detach(ctx, v.Interface, string(KeyVR(v.VR)))
	if err != nil || !found {
		return err
	}
	// the handler validates priority/interval/vr_id before it looks at is_add
	_, err = vrrp.NewServiceClient(d.Client).VrrpVrAddDel(ctx, &vrrp.VrrpVrAddDel{IsAdd: 0, SwIfIndex: interface_types.InterfaceIndex(tg.Index),
		VrID: v.VRID, Priority: max(v.Priority, 1), Interval: max(v.Interval, 1), Flags: flags(v)})
	if err != nil && !df7.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return d.Wrap("vrrp_vr_add_del (del) "+string(KeyVR(v.VR)), err)
	}
	return tg.Release()
}

// Retrieve implements scheduler.Descriptor: vrrp_vr_dump, VRs on owned interfaces.
func (d *VRDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	vrs, _, err := ownedVRs(ctx, d.Base)
	if err != nil {
		return nil, d.Wrap("retrieve", err)
	}
	var out []scheduler.KV
	for _, o := range vrs {
		c := o.Detail.Config
		v := VRSpec{VR: o.VR, Priority: c.Priority, Interval: c.Interval,
			Preempt: c.Flags&vrrp.VRRP_API_VR_PREEMPT != 0, Accept: c.Flags&vrrp.VRRP_API_VR_ACCEPT != 0, Unicast: c.Flags&vrrp.VRRP_API_VR_UNICAST != 0,
			Addresses: fromAddrs(o.Detail.Addrs)}
		out = append(out, df7.KV(KeyVR(o.VR), v, Meta{SwIfIndex: o.Idx}))
	}
	sortKVs(out)
	return out, nil
}

// ---- vrrp.vr-peers ----------------------------------------------------------------------------

// PeersDescriptor manages vrrp.vr-peers objects.
type PeersDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*PeersDescriptor)(nil)

// NewPeers returns the vrrp.vr-peers descriptor.
func NewPeers(c vpp.Client, owner string, opts ...df7.Option) *PeersDescriptor {
	return &PeersDescriptor{df7.NewBase(NamePeers, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *PeersDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	p, _ := df7.Decode[Peers](obj)
	return KeyPeers(p.VR)
}

// Dependencies implements scheduler.Descriptor: the VR.
func (d *PeersDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	p, _ := df7.Decode[Peers](obj)
	return []scheduler.Dependency{{Key: KeyVR(p.VR)}}
}

// set sends vrrp_vr_set_peers; VPP refuses it on a running VR (RSRC_IN_USE), so a running VR
// is stopped around the change and started again.
func (d *PeersDescriptor) set(ctx context.Context, p Peers) (Meta, error) {
	vrs, _, err := ownedVRs(ctx, d.Base)
	if err != nil {
		return Meta{}, err
	}
	var cur *ownedVR
	for i := range vrs {
		if vrs[i].VR == p.VR {
			cur = &vrs[i]
		}
	}
	if cur == nil {
		return Meta{}, fmt.Errorf("%s: %s does not exist", NamePeers, KeyVR(p.VR))
	}
	wasRunning := running(cur.Detail)
	if wasRunning {
		if err := startStop(ctx, d.Client, cur.Idx, p.VR, false); err != nil {
			return Meta{}, err
		}
	}
	addrs := toAddrs(p.Peers)
	_, err = vrrp.NewServiceClient(d.Client).VrrpVrSetPeers(ctx, &vrrp.VrrpVrSetPeers{SwIfIndex: interface_types.InterfaceIndex(cur.Idx), VrID: p.VRID,
		IsIPv6: isIPv6(p.IPv6), NAddrs: uint8(len(addrs)), Addrs: addrs}) //nolint:gosec // ≤ 255
	if err != nil {
		return Meta{}, d.Wrap("vrrp_vr_set_peers "+string(KeyVR(p.VR)), err)
	}
	if wasRunning {
		if err := startStop(ctx, d.Client, cur.Idx, p.VR, true); err != nil {
			return Meta{}, err
		}
	}
	return Meta{SwIfIndex: cur.Idx}, nil
}

// Create implements scheduler.Descriptor.
func (d *PeersDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p, err := df7.DecodeValid[Peers](obj)
	if err != nil {
		return nil, err
	}
	return d.set(ctx, p)
}

// Update implements scheduler.Descriptor: set the new list in place.
func (d *PeersDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	o, err := df7.Decode[Peers](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[Peers](newObj)
	if err != nil {
		return nil, err
	}
	if o.VR != n.VR {
		return nil, scheduler.ErrRecreate
	}
	return d.set(ctx, n)
}

// Delete implements scheduler.Descriptor: a no-op. VPP cannot clear the peers of a VR
// (vrrp_vr_set_peers refuses an empty list); they are removed with the VR, which the
// scheduler deletes right after its dependents. Removing only the peers of a unicast VR that
// stays is not expressible in VPP 26.06 — switch the VR to multicast (ErrRecreate) instead.
func (d *PeersDescriptor) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: vrrp_vr_peer_dump per owned VR (the dump-all form
// of VPP 26.06 answers with vrrp_vr_details instead of vrrp_vr_peer_details).
func (d *PeersDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	vrs, _, err := ownedVRs(ctx, d.Base)
	if err != nil {
		return nil, d.Wrap("retrieve", err)
	}
	var out []scheduler.KV
	for _, o := range vrs {
		if o.Detail.Config.Flags&vrrp.VRRP_API_VR_UNICAST == 0 {
			continue
		}
		stream, err := vrrp.NewServiceClient(d.Client).VrrpVrPeerDump(ctx, &vrrp.VrrpVrPeerDump{SwIfIndex: interface_types.InterfaceIndex(o.Idx),
			IsIPv6: isIPv6(o.VR.IPv6), VrID: o.VR.VRID})
		if err != nil {
			return nil, d.Wrap("vrrp_vr_peer_dump", err)
		}
		dets, err := df7.Collect(stream.Recv)
		if err != nil {
			return nil, d.Wrap("vrrp_vr_peer_dump", err)
		}
		for _, det := range dets {
			if len(det.PeerAddrs) == 0 {
				continue
			}
			p := Peers{VR: o.VR, Peers: fromAddrs(det.PeerAddrs)}
			out = append(out, df7.KV(KeyPeers(o.VR), p, Meta{SwIfIndex: o.Idx}))
		}
	}
	sortKVs(out)
	return out, nil
}

// ---- vrrp.vr-track-interface ------------------------------------------------------------------

// TrackDescriptor manages vrrp.vr-track-interface objects.
type TrackDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*TrackDescriptor)(nil)

// NewTrack returns the vrrp.vr-track-interface descriptor.
func NewTrack(c vpp.Client, owner string, opts ...df7.Option) *TrackDescriptor {
	return &TrackDescriptor{df7.NewBase(NameTrack, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *TrackDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	t, _ := df7.Decode[Track](obj)
	return KeyTrack(t.VR, t.Tracked)
}

// Dependencies implements scheduler.Descriptor: the VR and the tracked interface.
func (d *TrackDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	t, _ := df7.Decode[Track](obj)
	return []scheduler.Dependency{{Key: KeyVR(t.VR)}, d.Opts.IfaceDep(t.Tracked)}
}

// TrackMeta holds the VR's and the tracked interface's indexes.
type TrackMeta struct{ SwIfIndex, Tracked uint32 }

func (d *TrackDescriptor) addDel(ctx context.Context, m TrackMeta, t Track, add bool) error {
	var a uint8
	if add {
		a = 1
	}
	_, err := vrrp.NewServiceClient(d.Client).VrrpVrTrackIfAddDel(ctx, &vrrp.VrrpVrTrackIfAddDel{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex),
		IsIPv6: isIPv6(t.IPv6), VrID: t.VRID, IsAdd: a, NIfs: 1,
		Ifs: []vrrp.VrrpVrTrackIf{{SwIfIndex: interface_types.InterfaceIndex(m.Tracked), Priority: t.Priority}}})
	return d.Wrap(fmt.Sprintf("vrrp_vr_track_if_add_del %s add=%v", KeyTrack(t.VR, t.Tracked), add), err)
}

// Create implements scheduler.Descriptor.
func (d *TrackDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	t, err := df7.DecodeValid[Track](obj)
	if err != nil {
		return nil, err
	}
	m, found, err := d.resolve(ctx, t)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: %w: %q or %q", NameTrack, df7.ErrNoSuchInterface, t.Interface, t.Tracked)
	}
	return m, d.addDel(ctx, m, t, true)
}

// resolve finds the VR's interface (ours via the VR, D-071) and the tracked interface (not
// another owner's, D-069) by logical name.
func (d *TrackDescriptor) resolve(ctx context.Context, t Track) (TrackMeta, bool, error) {
	idx, found, err := vrIndex(ctx, d.Base, t.VR)
	if err != nil || !found {
		return TrackMeta{}, false, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return TrackMeta{}, false, err
	}
	tr, err := ifs.Resolve(t.Tracked)
	if errors.Is(err, df7.ErrNoSuchInterface) {
		return TrackMeta{}, false, nil
	}
	if err != nil {
		return TrackMeta{}, false, err
	}
	return TrackMeta{SwIfIndex: idx, Tracked: tr}, true, nil
}

// Update implements scheduler.Descriptor: VPP keeps the first priority of a tracked interface,
// so a new decrement is applied as delete + add.
func (d *TrackDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	o, err := df7.Decode[Track](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[Track](newObj)
	if err != nil {
		return nil, err
	}
	if o.VR != n.VR || o.Tracked != n.Tracked {
		return nil, scheduler.ErrRecreate
	}
	m, found, err := d.resolve(ctx, n)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: %w: %q or %q", NameTrack, df7.ErrNoSuchInterface, n.Interface, n.Tracked)
	}
	if err := d.addDel(ctx, m, o, false); err != nil {
		return nil, err
	}
	return m, d.addDel(ctx, m, n, true)
}

// Delete implements scheduler.Descriptor: interfaces re-resolved (D-071); a vanished VR
// interface or tracked interface is success.
func (d *TrackDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	t, err := df7.Decode[Track](obj)
	if err != nil {
		return err
	}
	m, found, err := d.resolve(ctx, t)
	if err != nil || !found {
		return err
	}
	if err := d.addDel(ctx, m, t, false); err != nil && !df7.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return err
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: vrrp_vr_track_if_dump (dump_all), owned VRs.
func (d *TrackDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := vrrp.NewServiceClient(d.Client).VrrpVrTrackIfDump(ctx, &vrrp.VrrpVrTrackIfDump{DumpAll: 1})
	if err != nil {
		return nil, d.Wrap("vrrp_vr_track_if_dump", df7.PluginError("vrrp", err))
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("vrrp_vr_track_if_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		name, ok := ifs.Owned(uint32(det.SwIfIndex), func(n string) string {
			return string(KeyVR(VR{Interface: n, VRID: det.VrID, IPv6: det.IsIPv6 != 0}))
		})
		if !ok {
			continue
		}
		v := VR{Interface: name, VRID: det.VrID, IPv6: det.IsIPv6 != 0}
		for _, tr := range det.Ifs {
			t := Track{VR: v, Tracked: ifs.Name(uint32(tr.SwIfIndex)), Priority: tr.Priority}
			out = append(out, df7.KV(KeyTrack(v, t.Tracked), t, TrackMeta{SwIfIndex: uint32(det.SwIfIndex), Tracked: uint32(tr.SwIfIndex)}))
		}
	}
	sortKVs(out)
	return out, nil
}

// ---- vrrp.vr-state ----------------------------------------------------------------------------

// StateDescriptor manages vrrp.vr-state objects (vrrp_vr_start_stop).
type StateDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*StateDescriptor)(nil)

// NewState returns the vrrp.vr-state descriptor.
func NewState(c vpp.Client, owner string, opts ...df7.Option) *StateDescriptor {
	return &StateDescriptor{df7.NewBase(NameState, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *StateDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, _ := df7.Decode[State](obj)
	return KeyState(s.VR)
}

// Dependencies implements scheduler.Descriptor: the VR, and its peers (optional: a unicast VR
// cannot start without them).
func (d *StateDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, _ := df7.Decode[State](obj)
	return []scheduler.Dependency{{Key: KeyVR(s.VR)}, {Key: KeyPeers(s.VR), Optional: true}}
}

// Create implements scheduler.Descriptor: start (VPP returns success when already started).
func (d *StateDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := df7.DecodeValid[State](obj)
	if err != nil {
		return nil, err
	}
	idx, err := mustVRIndex(ctx, d.Base, s.VR)
	if err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: idx}, startStop(ctx, d.Client, idx, s.VR, true)
}

// Update implements scheduler.Descriptor: nothing but the key can differ.
func (*StateDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return meta, nil
}

// Delete implements scheduler.Descriptor: stop (interface re-resolved, D-071; a VR that is
// gone is stopped already).
func (d *StateDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	s, err := df7.Decode[State](obj)
	if err != nil {
		return err
	}
	idx, found, err := vrIndex(ctx, d.Base, s.VR)
	if err != nil || !found {
		return err
	}
	if err := startStop(ctx, d.Client, idx, s.VR, false); err != nil && !df7.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return err
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: owned VRs whose runtime state is not Init.
func (d *StateDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	vrs, _, err := ownedVRs(ctx, d.Base)
	if err != nil {
		return nil, d.Wrap("retrieve", err)
	}
	var out []scheduler.KV
	for _, o := range vrs {
		if running(o.Detail) {
			out = append(out, df7.KV(KeyState(o.VR), State{VR: o.VR, Running: true}, Meta{SwIfIndex: o.Idx}))
		}
	}
	sortKVs(out)
	return out, nil
}

// Register constructs every vrrp descriptor (VRs, peers, tracking, state).
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewVR(c, owner, opts...))
	r.Register(NewPeers(c, owner, opts...))
	r.Register(NewTrack(c, owner, opts...))
	r.Register(NewState(c, owner, opts...))
}
