// Package lcp holds the reconciler descriptors for VPP's linux-cp plugin (task DF-8, WBS D3.1):
// the default network namespace for host interfaces and the interface pairs (a VPP interface
// mirrored by a Linux tap/tun), plus the replace-transaction helpers P12 uses. Message names come
// only from apps/agent/binapi/lcp; docs/agent/descriptors/lcp.md is the object ↔ message table.
// linux_nl (the netlink listener) has no binary API: it is configured in startup.conf only.
//
// linux_cp and linux_nl are loaded on vrx-a since 2026-09-24 (D-060). Where they are not, every
// call fails with ErrPluginNotLoaded (govpp does not know the message ids).
//
// Ownership: a pair is owned through its VPP-side interface, which must carry this agent's owner
// tag; the Linux-side name should carry the owner prefix too (tests: "w<N>-…").
package lcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameDefaultNetns = "lcp.default-netns"
	NameItfPair      = "lcp.itf-pair"
)

// Plugin is the plugin name used in ErrPluginNotLoaded.
const Plugin = "linux_cp"

// DefaultNetns is the lcp.default-netns singleton: the namespace host interfaces are created in
// when a pair names none ("" = VPP's own namespace).
type DefaultNetns struct {
	Netns string `json:"netns"`
}

// ItfPair is one linux-cp pair: the VPP interface, the Linux interface name (≤ 15 bytes), its
// type (tap or tun) and namespace. Netns must be the effective namespace: VPP reports a pair
// created with "" as created in the default namespace, so desire "" only while no default
// namespace is set.
type ItfPair struct {
	Interface  string `json:"interface"`
	HostIfName string `json:"host_if_name"`
	HostIfType string `json:"host_if_type"`
	Netns      string `json:"netns"`
}

// Proto returns the canonical structpb document.
func (s DefaultNetns) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s ItfPair) Proto() *structpb.Struct { return dfkit.Encode(s) }

func validName(what, v string, maxLen int, allowEmpty bool) error {
	if v == "" {
		if allowEmpty {
			return nil
		}
		return dfkit.Specf("%s is empty", what)
	}
	if len(v) > maxLen {
		return dfkit.Specf("%s %q longer than %d bytes", what, v, maxLen)
	}
	for _, r := range v {
		ok := r == '-' || r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return dfkit.Specf("%s %q: invalid character %q", what, v, r)
		}
	}
	if v == "." || v == ".." {
		return dfkit.Specf("%s %q is not a name", what, v)
	}
	return nil
}

// Validate checks the namespace name (≤ 31 bytes; it becomes /var/run/netns/<name> in VPP).
func (s DefaultNetns) Validate() error { return validName("netns", s.Netns, 31, true) }

// Validate checks the names (Linux IFNAMSIZ 15, netns 31) and the host type.
func (s ItfPair) Validate() error {
	if s.Interface == "" {
		return dfkit.Specf("lcp pair: interface is empty")
	}
	if err := validName("host_if_name", s.HostIfName, 15, false); err != nil {
		return err
	}
	if _, ok := hostTypeToAPI[s.HostIfType]; !ok {
		return dfkit.Specf("lcp pair: host_if_type %q must be tap or tun", s.HostIfType)
	}
	return validName("netns", s.Netns, 31, true)
}

var (
	hostTypeToAPI   = map[string]lcp.LcpItfHostType{"tap": lcp.LCP_API_ITF_HOST_TAP, "tun": lcp.LCP_API_ITF_HOST_TUN}
	hostTypeFromAPI = map[lcp.LcpItfHostType]string{lcp.LCP_API_ITF_HOST_TAP: "tap", lcp.LCP_API_ITF_HOST_TUN: "tun"}
)

// Option configures the descriptors of this package.
type Option func(*options)

type options struct{ ifaceKey dfkit.KeyFunc }

// WithInterfaceKey sets the interface key scheme of Dependencies (default "interface/<name>", D-065).
func WithInterfaceKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.ifaceKey = f
		}
	}
}

// Register constructs and registers the lcp descriptors.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...Option) {
	r.Register(NewDefaultNetns(client))
	r.Register(NewItfPair(client, owner, opts...))
}

// ---- lcp.default-netns ----------------------------------------------------------------------

// DefaultNetnsID is the object id of the singleton (key lcp.default-netns/global).
const DefaultNetnsID = "global"

// KeyDefaultNetns is the key of the singleton.
var KeyDefaultNetns = scheduler.Join(NameDefaultNetns, DefaultNetnsID)

// DefaultNetnsDescriptor manages the lcp.default-netns singleton (lcp_default_ns_set /
// lcp_default_ns_get). Retrieve reports it while set; Delete unsets it.
type DefaultNetnsDescriptor struct{ client vpp.Client }

var _ scheduler.Descriptor = (*DefaultNetnsDescriptor)(nil)

// NewDefaultNetns returns the lcp.default-netns descriptor.
func NewDefaultNetns(client vpp.Client) *DefaultNetnsDescriptor {
	return &DefaultNetnsDescriptor{client: client}
}

// Name implements scheduler.Descriptor.
func (*DefaultNetnsDescriptor) Name() string { return NameDefaultNetns }

// KeyOf implements scheduler.Descriptor.
func (*DefaultNetnsDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyDefaultNetns }

// Dependencies implements scheduler.Descriptor: none.
func (*DefaultNetnsDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *DefaultNetnsDescriptor) set(ctx context.Context, ns string) error {
	if _, err := lcp.NewServiceClient(d.client).LcpDefaultNsSet(ctx, &lcp.LcpDefaultNsSet{Netns: ns}); err != nil {
		return fmt.Errorf("lcp_default_ns_set(%q): %w", ns, dfkit.PluginError(Plugin, err))
	}
	return nil
}

func (d *DefaultNetnsDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s DefaultNetns
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	return d.set(ctx, s.Netns)
}

// Create implements scheduler.Descriptor.
func (d *DefaultNetnsDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor (existing pairs keep their namespace).
func (d *DefaultNetnsDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: unset.
func (d *DefaultNetnsDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.set(ctx, "")
}

// Current returns the default namespace ("" = unset).
func (d *DefaultNetnsDescriptor) Current(ctx context.Context) (string, error) {
	rep, err := lcp.NewServiceClient(d.client).LcpDefaultNsGet(ctx, &lcp.LcpDefaultNsGet{})
	if err != nil {
		return "", fmt.Errorf("lcp_default_ns_get: %w", dfkit.PluginError(Plugin, err))
	}
	return strings.TrimRight(rep.Netns, "\x00"), nil
}

// Retrieve implements scheduler.Descriptor: lcp_default_ns_get while set.
func (d *DefaultNetnsDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ns, err := d.Current(ctx)
	if err != nil || ns == "" {
		return nil, err
	}
	return []scheduler.KV{{Key: KeyDefaultNetns, Value: DefaultNetns{Netns: ns}.Proto()}}, nil
}

// ---- lcp.itf-pair ---------------------------------------------------------------------------

// PairMeta is the Meta of an lcp.itf-pair: the VPP-side and host-side (tap) sw_if_index and the
// Linux ifindex of the host interface.
type PairMeta struct {
	PhySwIfIndex  uint32
	HostSwIfIndex uint32
	VifIndex      uint32
}

// ItfPairDescriptor manages lcp.itf-pair objects (lcp_itf_pair_add_del_v3 / lcp_itf_pair_get):
// key lcp.itf-pair/<interface>. Any change recreates the pair (VPP has no in-place update).
type ItfPairDescriptor struct {
	client vpp.Client
	owner  string
	o      options
}

var _ scheduler.Descriptor = (*ItfPairDescriptor)(nil)

// NewItfPair returns the lcp.itf-pair descriptor.
func NewItfPair(client vpp.Client, owner string, opts ...Option) *ItfPairDescriptor {
	o := options{ifaceKey: dfkit.DefaultInterfaceKey}
	for _, opt := range opts {
		opt(&o)
	}
	return &ItfPairDescriptor{client: client, owner: owner, o: o}
}

// Name implements scheduler.Descriptor.
func (*ItfPairDescriptor) Name() string { return NameItfPair }

// KeyOf implements scheduler.Descriptor.
func (*ItfPairDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s ItfPair
	if err := dfkit.Decode(obj, &s); err != nil {
		return scheduler.Join(NameItfPair, "invalid")
	}
	return scheduler.Join(NameItfPair, s.Interface)
}

// Dependencies implements scheduler.Descriptor: the VPP-side interface (D-065 alias key) and the
// default namespace (optional — only orders the plan when it is configured).
func (d *ItfPairDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s ItfPair
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: d.o.ifaceKey(s.Interface)}, {Key: KeyDefaultNetns, Optional: true}}
}

func (d *ItfPairDescriptor) spec(obj proto.Message) (ItfPair, error) {
	var s ItfPair
	if err := dfkit.Decode(obj, &s); err != nil {
		return s, err
	}
	return s, s.Validate()
}

// Create implements scheduler.Descriptor. If the interface already has an identical pair (a
// re-apply) Create succeeds with its Meta; a different pair is an error.
func (d *ItfPairDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := d.spec(obj)
	if err != nil {
		return nil, err
	}
	idx, err := dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner)
	if err != nil {
		return nil, err
	}
	rep, err := lcp.NewServiceClient(d.client).LcpItfPairAddDelV3(ctx, &lcp.LcpItfPairAddDelV3{
		IsAdd: true, SwIfIndex: interface_types.InterfaceIndex(idx), HostIfName: s.HostIfName,
		HostIfType: hostTypeToAPI[s.HostIfType], Netns: s.Netns,
	})
	if err != nil {
		if dfkit.IsVPPError(err, api.VALUE_EXIST) {
			kvs, rerr := d.Retrieve(ctx)
			if rerr == nil {
				for _, kv := range kvs {
					if kv.Key == d.KeyOf(obj) && proto.Equal(kv.Value, s.Proto()) {
						return kv.Meta, nil
					}
				}
			}
		}
		return nil, fmt.Errorf("lcp_itf_pair_add_del_v3(add %s ↔ %s): %w", s.Interface, s.HostIfName, dfkit.PluginError(Plugin, err))
	}
	return PairMeta{PhySwIfIndex: idx, HostSwIfIndex: uint32(rep.HostSwIfIndex), VifIndex: rep.VifIndex}, nil
}

// Update implements scheduler.Descriptor: recreate.
func (*ItfPairDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor (also removes the host tap); a pair or interface that
// is already gone counts as deleted.
func (d *ItfPairDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	s, err := d.spec(obj)
	if err != nil {
		return err
	}
	var idx uint32
	if m, ok := meta.(PairMeta); ok {
		idx = m.PhySwIfIndex
	} else if idx, err = dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner); err != nil {
		if errors.Is(err, dfkit.ErrNoInterface) {
			return nil
		}
		return err
	}
	_, err = lcp.NewServiceClient(d.client).LcpItfPairAddDelV3(ctx, &lcp.LcpItfPairAddDelV3{
		IsAdd: false, SwIfIndex: interface_types.InterfaceIndex(idx),
	})
	if err != nil && !dfkit.IsVPPError(err, api.INVALID_SW_IF_INDEX, api.NO_SUCH_ENTRY, api.INVALID_VALUE) {
		return fmt.Errorf("lcp_itf_pair_add_del_v3(del %s): %w", s.Interface, dfkit.PluginError(Plugin, err))
	}
	return nil
}

// Pairs reads every pair (lcp_itf_pair_get, cursor-paged). lcp_itf_pair_get_v2 is not used: for
// sw_if_index ~0 VPP 26.06 answers it with the v1 reply id (lcp_api.c), which the generated v2
// client rejects.
func Pairs(ctx context.Context, c vpp.Client) ([]*lcp.LcpItfPairDetails, error) {
	svc := lcp.NewServiceClient(c)
	var out []*lcp.LcpItfPairDetails
	cursor := uint32(0)
	for {
		stream, err := svc.LcpItfPairGet(ctx, &lcp.LcpItfPairGet{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("lcp_itf_pair_get: %w", dfkit.PluginError(Plugin, err))
		}
		var next *lcp.LcpItfPairGetReply
		for {
			det, rep, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				next = nil
				break
			}
			if err != nil && rep != nil && dfkit.IsVPPError(err, api.EAGAIN) {
				next = rep
				break
			}
			if err != nil {
				return nil, fmt.Errorf("lcp_itf_pair_get: %w", dfkit.PluginError(Plugin, err))
			}
			out = append(out, det)
		}
		if next == nil || next.Cursor == cursor {
			return out, nil
		}
		cursor = next.Cursor
	}
}

// Retrieve implements scheduler.Descriptor: lcp_itf_pair_get, pairs whose VPP-side interface is
// owned.
func (d *ItfPairDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	pairs, err := Pairs(ctx, d.client)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	ifaces, err := dfkit.DumpInterfaces(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, p := range pairs {
		name, ok := ifaces.OwnedName(uint32(p.PhySwIfIndex), d.owner)
		if !ok {
			continue
		}
		s := ItfPair{
			Interface: name, HostIfName: strings.TrimRight(p.HostIfName, "\x00"),
			HostIfType: hostTypeFromAPI[p.HostIfType], Netns: strings.TrimRight(p.Netns, "\x00"),
		}
		out = append(out, scheduler.KV{
			Key: scheduler.Join(NameItfPair, name), Value: s.Proto(),
			Meta: PairMeta{PhySwIfIndex: uint32(p.PhySwIfIndex), HostSwIfIndex: uint32(p.HostSwIfIndex), VifIndex: p.VifIndex},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ---- replace transaction (P12) --------------------------------------------------------------

// ReplaceBegin starts a linux-cp replace transaction (lcp_itf_pair_replace_begin): every existing
// pair is marked stale until re-added; ReplaceEnd deletes the pairs still stale. It is a helper
// for P12's full resync, not a descriptor, and affects every owner's pairs — call it only when
// this agent owns the whole VPP.
func ReplaceBegin(ctx context.Context, c vpp.Client) error {
	if _, err := lcp.NewServiceClient(c).LcpItfPairReplaceBegin(ctx, &lcp.LcpItfPairReplaceBegin{}); err != nil {
		return fmt.Errorf("lcp_itf_pair_replace_begin: %w", dfkit.PluginError(Plugin, err))
	}
	return nil
}

// ReplaceEnd ends a replace transaction (lcp_itf_pair_replace_end), deleting unrefreshed pairs.
func ReplaceEnd(ctx context.Context, c vpp.Client) error {
	if _, err := lcp.NewServiceClient(c).LcpItfPairReplaceEnd(ctx, &lcp.LcpItfPairReplaceEnd{}); err != nil {
		return fmt.Errorf("lcp_itf_pair_replace_end: %w", dfkit.PluginError(Plugin, err))
	}
	return nil
}
