package sr_mpls //nolint:revive // see policy.go

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib"
	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	srmplsapi "ngfw/agent/binapi/sr_mpls"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// SteeringName is the descriptor name; keys are "sr-mpls.steering/<table>/<prefix>".
const SteeringName = "sr-mpls.steering"

// EndpointColorName is the descriptor name; keys are "sr-mpls.endpoint-color/<bsid>".
const EndpointColorName = "sr-mpls.endpoint-color"

// noLabel is VPP's "no label" (~0) for vpn_label and color.
const noLabel = ^uint32(0)

// SteeringDescriptor manages SR-MPLS steering by BSID (write-only, ours by claim).
type SteeringDescriptor = df6.KeyedDescriptor[*Steering]

// NewSteering returns the descriptor.
func NewSteering(c vpp.Client, owner string, opts ...df6.Option) *SteeringDescriptor {
	return df6.NewKeyedDescriptor(steeringSpec, c, owner, opts...)
}

func canonSteering(s *Steering) (*Steering, error) {
	c, err := df6.CanonicalPrefix(s.GetPrefix())
	if err != nil {
		return nil, err
	}
	if !validLabel(s.GetBsid()) {
		return nil, fmt.Errorf("%w: bsid %d", df6.ErrBadValue, s.GetBsid())
	}
	if s.GetVpnLabel() != 0 && !validLabel(s.GetVpnLabel()) {
		return nil, fmt.Errorf("%w: vpn_label %d", df6.ErrBadValue, s.GetVpnLabel())
	}
	out := proto.Clone(s).(*Steering)
	out.Prefix = c
	return out, nil
}

func steeringRequest(ctx context.Context, c vpp.Client, s *Steering, isDel bool) (*srmplsapi.SrMplsSteeringAddDel, error) {
	p, err := df6.ParsePrefix(s.GetPrefix())
	if err != nil {
		return nil, err
	}
	// VPP adds/deletes the steering route in fib_table_find(table) unchecked.
	if err := df6.RequireTable(ctx, c, s.GetTableId(), p.Addr().Is6()); err != nil {
		return nil, err
	}
	vpn := s.GetVpnLabel()
	if vpn == 0 {
		vpn = noLabel
	}
	return &srmplsapi.SrMplsSteeringAddDel{
		IsDel: isDel, Bsid: s.GetBsid(), TableID: s.GetTableId(), Prefix: df6.ToPrefix(p),
		MaskWidth: uint32(p.Bits()), Color: noLabel, VPNLabel: vpn, //nolint:gosec // 0..128
	}, nil
}

var steeringSpec = df6.KeyedSpec[*Steering]{
	Name:      SteeringName,
	Plugin:    Plugin,
	WriteOnly: true,
	Canon:     canonSteering,
	ID:        func(s *Steering) string { return df6.U32(s.GetTableId()) + "/" + s.GetPrefix() },
	Deps: func(s *Steering) []scheduler.Dependency {
		return append([]scheduler.Dependency{{Key: PolicyKey(s.GetBsid())}}, df6.VRFDeps(s.GetTableId())...)
	},
	// Add: the policy must exist — VPP 26.06 leaves a half-created steering entry behind when
	// the BSID is unknown.
	Add: func(ctx context.Context, c vpp.Client, s *Steering) error {
		ok, err := BSIDPresent(ctx, c, s.GetBsid())
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: bsid %d", ErrNoSuchPolicy, s.GetBsid())
		}
		req, err := steeringRequest(ctx, c, s, false)
		if err != nil {
			return err
		}
		if _, err := srmplsapi.NewServiceClient(c).SrMplsSteeringAddDel(ctx, req); err != nil {
			return fmt.Errorf("sr_mpls_steering_add_del: %w", err)
		}
		return nil
	},
	Del: func(ctx context.Context, c vpp.Client, s *Steering) error {
		req, err := steeringRequest(ctx, c, s, true)
		if err != nil {
			return err
		}
		if _, err := srmplsapi.NewServiceClient(c).SrMplsSteeringAddDel(ctx, req); err != nil {
			return fmt.Errorf("sr_mpls_steering_add_del (del): %w", err)
		}
		return nil
	},
	// Identity: the VPN label pushed under the BSID (the BSID itself is not readable back).
	Identity: func(want, have *Steering) bool { return want.GetVpnLabel() == have.GetVpnLabel() },
	List:     listSteering,
}

// listSteering returns the SR-MPLS steering routes: FIB_SOURCE_SR routes (fib_source_dump
// name "SR") with a recursive MPLS path, per IP table. Partial: prefix, table, vpn label.
func listSteering(ctx context.Context, c vpp.Client) ([]*Steering, error) {
	src, err := fibSource(ctx, c, "SR")
	if err != nil {
		return nil, err
	}
	stream, err := ip.NewServiceClient(c).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		return nil, fmt.Errorf("ip_table_dump: %w", err)
	}
	tables, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("ip_table_dump: %w", err)
	}
	var out []*Steering
	for _, t := range tables {
		rs, err := ip.NewServiceClient(c).IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Src: src, Table: t.Table})
		if err != nil {
			return nil, fmt.Errorf("ip_route_v2_dump: %w", err)
		}
		routes, err := df6.Collect(rs.Recv)
		if err != nil {
			return nil, fmt.Errorf("ip_route_v2_dump: %w", err)
		}
		for _, r := range routes {
			for _, p := range r.Route.Paths {
				if p.Proto != fib_types.FIB_API_PATH_NH_PROTO_MPLS || p.SwIfIndex != df6.NoInterface {
					continue
				}
				s := &Steering{Prefix: df6.PrefixString(r.Route.Prefix), TableId: r.Route.TableID}
				if p.NLabels > 0 {
					s.VpnLabel = p.LabelStack[0].Label
				}
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

// fibSource returns the id of the FIB source called name.
func fibSource(ctx context.Context, c vpp.Client, name string) (uint8, error) {
	stream, err := fib.NewServiceClient(c).FibSourceDump(ctx, &fib.FibSourceDump{})
	if err != nil {
		return 0, fmt.Errorf("fib_source_dump: %w", err)
	}
	srcs, err := df6.Collect(stream.Recv)
	if err != nil {
		return 0, fmt.Errorf("fib_source_dump: %w", err)
	}
	for _, s := range srcs {
		if strings.EqualFold(strings.TrimRight(s.Src.Name, "\x00"), name) {
			return s.Src.ID, nil
		}
	}
	return 0, fmt.Errorf("fib source %q not found", name)
}

// EndpointColorDescriptor assigns (endpoint, color) to a policy. Write-only (no dump). The
// re-apply guard is a claim for the running VPP boot (D-076): VPP's assign relocks internal
// labels on every call, so it is sent once per VPP instance. VPP has no un-assign message;
// the assignment is cleared together with the policy (sr_mpls_policy_del), so Delete is a
// documented no-op (review M4) that only drops the claim.
type EndpointColorDescriptor struct {
	client vpp.Client
	claims df6.ClaimStore
}

// NewEndpointColor returns the descriptor.
func NewEndpointColor(c vpp.Client, owner string, opts ...df6.Option) *EndpointColorDescriptor {
	return &EndpointColorDescriptor{client: c, claims: df6.BuildOptions(owner, opts).Claims}
}

// Name implements scheduler.Descriptor.
func (d *EndpointColorDescriptor) Name() string { return EndpointColorName }

func (d *EndpointColorDescriptor) cast(obj proto.Message) (*EndpointColor, error) {
	e, ok := obj.(*EndpointColor)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %T", EndpointColorName, df6.ErrBadValue, obj)
	}
	if !validLabel(e.GetBsid()) {
		return nil, fmt.Errorf("%s: %w: bsid %d", EndpointColorName, df6.ErrBadValue, e.GetBsid())
	}
	a, err := df6.ParseAddr(e.GetEndpoint())
	if err != nil || a.IsUnspecified() {
		return nil, fmt.Errorf("%s: %w: endpoint %q", EndpointColorName, df6.ErrBadValue, e.GetEndpoint())
	}
	if e.GetColor() == noLabel {
		return nil, fmt.Errorf("%s: %w: color ~0 is reserved", EndpointColorName, df6.ErrBadValue)
	}
	out := proto.Clone(e).(*EndpointColor)
	out.Endpoint = a.String()
	return out, nil
}

// KeyOf implements scheduler.Descriptor.
func (d *EndpointColorDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	e, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(EndpointColorName, "invalid")
	}
	return scheduler.Join(EndpointColorName, df6.U32(e.GetBsid()))
}

// Dependencies implements scheduler.Descriptor: the policy.
func (d *EndpointColorDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	e, err := d.cast(obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{{Key: PolicyKey(e.GetBsid())}}
}

// claimValue is keyed by the BSID only: a policy (re-)add on the same VPP boot releases it
// (review N1: VPP cleared the assignment with the lost policy), see policySpec.Add.
func claimValue(e *EndpointColor) string { return df6.U32(e.GetBsid()) }

// releaseEndpointColor drops the applied-once record of bsid's endpoint/color on the running VPP.
func releaseEndpointColor(ctx context.Context, c vpp.Client, claims df6.ClaimStore, bsid uint32) error {
	boot, err := df6.BootID(ctx, c)
	if err != nil {
		return err
	}
	return claims.Release(df6.U32(bsid), df6.BootHolder(EndpointColorName, boot))
}

// Create implements scheduler.Descriptor.
func (d *EndpointColorDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	e, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	ours, err := df6.ClaimedNow(ctx, d.client, d.claims, PolicyName, df6.U32(e.GetBsid()))
	if err != nil {
		return nil, err
	}
	if !ours {
		return nil, fmt.Errorf("%s: %w: policy %d", EndpointColorName, df6.ErrNotOurs, e.GetBsid())
	}
	boot, err := df6.BootID(ctx, d.client)
	if err != nil {
		return nil, err
	}
	holder := df6.BootHolder(EndpointColorName, boot)
	if d.claims.Claimed(claimValue(e), holder) {
		return nil, nil // already assigned on this VPP instance
	}
	ep, _ := df6.AddressOf(e.GetEndpoint())
	if _, err := srmplsapi.NewServiceClient(d.client).SrMplsPolicyAssignEndpointColor(ctx, &srmplsapi.SrMplsPolicyAssignEndpointColor{
		Bsid: e.GetBsid(), Endpoint: ep, Color: e.GetColor(),
	}); err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_mpls_policy_assign_endpoint_color: %w", EndpointColorName, err))
	}
	return nil, d.claims.Claim(claimValue(e), holder)
}

// Update implements scheduler.Descriptor: re-assigning replaces the previous pair in place.
func (d *EndpointColorDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	if o, err := d.cast(oldObj); err == nil {
		if boot, err := df6.BootID(ctx, d.client); err == nil {
			_ = d.claims.Release(claimValue(o), df6.BootHolder(EndpointColorName, boot))
		}
	}
	if _, err := d.Create(ctx, newObj); err != nil {
		return nil, err
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor: documented no-op in VPP (cleared with the policy).
func (d *EndpointColorDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	e, err := d.cast(obj)
	if err != nil {
		return err
	}
	if boot, err := df6.BootID(ctx, d.client); err == nil {
		return d.claims.Release(claimValue(e), df6.BootHolder(EndpointColorName, boot))
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: no dump.
func (d *EndpointColorDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", EndpointColorName, df6.ErrRetrieveUnsupported)
}
