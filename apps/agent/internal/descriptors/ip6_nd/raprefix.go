package ip6nd

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// RaPrefixName is the descriptor name; keys are "ip6-nd.ra-prefix/<interface>/<prefix>".
const RaPrefixName = "ip6-nd.ra-prefix"

// VPP defaults for prefix lifetimes (ip6_ra.c DEF_ADV_VALID_LIFETIME / DEF_ADV_PREF_LIFETIME).
const (
	DefaultValidLifetime     = 2592000
	DefaultPreferredLifetime = 604800
)

// NormalizeRaPrefix returns p in canonical form: masked prefix, zero lifetimes replaced by
// the VPP defaults — the form Retrieve produces.
func NormalizeRaPrefix(p *RaPrefix) *RaPrefix {
	n := proto.Clone(p).(*RaPrefix)
	if pf, err := df2.ParsePrefix(n.Prefix); err == nil {
		n.Prefix = pf.String()
	}
	if n.ValidLifetime == 0 {
		n.ValidLifetime = DefaultValidLifetime
	}
	if n.PreferredLifetime == 0 {
		n.PreferredLifetime = DefaultPreferredLifetime
	}
	return n
}

// RaPrefixDescriptor manages advertised prefixes (sw_interface_ip6nd_ra_prefix).
type RaPrefixDescriptor struct {
	client vpp.Client
	owner  string
	opts   df2.Options
}

// NewRaPrefix returns the descriptor for the given owner.
func NewRaPrefix(c vpp.Client, owner string, opts ...df2.Option) *RaPrefixDescriptor {
	return &RaPrefixDescriptor{client: c, owner: owner, opts: df2.BuildOptions(opts...)}
}

// Name implements scheduler.Descriptor.
func (*RaPrefixDescriptor) Name() string { return RaPrefixName }

// KeyOf implements scheduler.Descriptor.
func (*RaPrefixDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	p := obj.(*RaPrefix)
	pf := p.GetPrefix()
	if parsed, err := df2.ParsePrefix(pf); err == nil {
		pf = parsed.String()
	}
	return scheduler.Join(RaPrefixName, p.GetInterface(), pf)
}

// Dependencies implements scheduler.Descriptor: the interface, and (optionally, for
// ordering only) the interface address inside the advertised prefix.
func (*RaPrefixDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	p := obj.(*RaPrefix)
	deps := []scheduler.Dependency{df2.InterfaceDep(p.GetInterface())}
	if pf, err := df2.ParsePrefix(p.GetPrefix()); err == nil {
		deps = append(deps, scheduler.Dependency{Key: df2.InterfaceIPKey(p.GetInterface(), pf.String()), Optional: true})
	}
	return deps
}

func (d *RaPrefixDescriptor) set(ctx context.Context, p *RaPrefix, idx interface_types.InterfaceIndex, isNo bool) error {
	pf, err := df2.ParsePrefix(p.GetPrefix())
	if err != nil {
		return err
	}
	if !pf.Addr().Is6() {
		return fmt.Errorf("%s: %s is not an IPv6 prefix", RaPrefixName, pf)
	}
	if p.GetPreferredLifetime() > p.GetValidLifetime() {
		return fmt.Errorf("%s: preferred_lifetime %d exceeds valid_lifetime %d", RaPrefixName, p.GetPreferredLifetime(), p.GetValidLifetime())
	}
	req := &ip6_nd.SwInterfaceIP6ndRaPrefix{
		SwIfIndex:    idx,
		Prefix:       df2.ToPrefix(pf),
		NoAdvertise:  p.GetNoAdvertise(),
		OffLink:      p.GetOffLink(),
		NoAutoconfig: p.GetNoAutoconfig(),
		IsNo:         isNo,
		ValLifetime:  p.GetValidLifetime(),
		PrefLifetime: p.GetPreferredLifetime(),
	}
	if _, err := ip6_nd.NewServiceClient(d.client).SwInterfaceIP6ndRaPrefix(ctx, req); err != nil {
		return fmt.Errorf("sw_interface_ip6nd_ra_prefix: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *RaPrefixDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p := NormalizeRaPrefix(obj.(*RaPrefix))
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx32, untagged, err := ifs.Resolve(p.GetInterface())
	if err != nil {
		return nil, err
	}
	idx := interface_types.InterfaceIndex(idx32)
	if err := d.set(ctx, p, idx, false); err != nil {
		return nil, err
	}
	if err := df2.Claim(d.opts.Claims, untagged, d.KeyOf(obj)); err != nil {
		return nil, err
	}
	return RaMeta{SwIfIndex: idx32}, nil
}

// Update changes lifetimes and flags in place (VPP updates an existing prefix entry).
func (d *RaPrefixDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if d.KeyOf(oldObj) != d.KeyOf(newObj) {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(RaMeta)
	if !ok {
		return nil, fmt.Errorf("%s: %w %T", RaPrefixName, df2.ErrBadMeta, meta)
	}
	if err := d.set(ctx, NormalizeRaPrefix(newObj.(*RaPrefix)), interface_types.InterfaceIndex(m.SwIfIndex), false); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor.
func (d *RaPrefixDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(RaMeta)
	if !ok {
		return fmt.Errorf("%s: %w %T", RaPrefixName, df2.ErrBadMeta, meta)
	}
	if skip, err := df2.SkipDelete(ctx, d.client, d.owner, m.SwIfIndex, obj.(df2.Named), d.KeyOf(obj), d.opts.Claims); err != nil {
		return err
	} else if skip {
		return df2.Release(d.opts.Claims, d.KeyOf(obj)) // interface gone or index reused: nothing of ours to remove
	}
	if err := d.set(ctx, NormalizeRaPrefix(obj.(*RaPrefix)), interface_types.InterfaceIndex(m.SwIfIndex), true); err != nil {
		return err
	}
	return df2.Release(d.opts.Claims, d.KeyOf(obj))
}

// Retrieve decodes the prefix list of every owned interface from sw_interface_ip6nd_ra_dump.
func (d *RaPrefixDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	details, err := dumpRa(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, det := range details {
		name, ok := ifs.Name(uint32(det.SwIfIndex))
		if !ok {
			continue
		}
		for _, p := range det.Prefixes {
			v := &RaPrefix{
				Interface:         name,
				Prefix:            df2.FromPrefix(p.Prefix).String(),
				ValidLifetime:     p.ValLifetime,
				PreferredLifetime: p.PrefLifetime,
				NoAdvertise:       p.NoAdvertise,
				OffLink:           !p.OnlinkFlag,
				NoAutoconfig:      !p.AutonomousFlag,
			}
			if !ifs.OwnsObject(uint32(det.SwIfIndex), d.KeyOf(v), d.opts.Claims) {
				continue
			}
			out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: RaMeta{SwIfIndex: uint32(det.SwIfIndex)}})
		}
	}
	return out, nil
}
