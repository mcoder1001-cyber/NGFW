package sr_mpls //nolint:revive,stylecheck // see policy.go

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

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

// SteeringDescriptor manages SR-MPLS steering by BSID (write-only, see policy.go).
type SteeringDescriptor struct {
	client vpp.Client
}

// NewSteering returns the descriptor.
func NewSteering(c vpp.Client) *SteeringDescriptor { return &SteeringDescriptor{client: c} }

// Name implements scheduler.Descriptor.
func (d *SteeringDescriptor) Name() string { return SteeringName }

func (d *SteeringDescriptor) cast(obj proto.Message) (*Steering, error) {
	s, ok := obj.(*Steering)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %T", SteeringName, df6.ErrBadValue, obj)
	}
	c, err := df6.CanonicalPrefix(s.GetPrefix())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SteeringName, err)
	}
	if !validLabel(s.GetBsid()) {
		return nil, fmt.Errorf("%s: %w: bsid %d", SteeringName, df6.ErrBadValue, s.GetBsid())
	}
	if s.GetVpnLabel() != 0 && !validLabel(s.GetVpnLabel()) {
		return nil, fmt.Errorf("%s: %w: vpn_label %d", SteeringName, df6.ErrBadValue, s.GetVpnLabel())
	}
	out := proto.Clone(s).(*Steering)
	out.Prefix = c
	return out, nil
}

// KeyOf implements scheduler.Descriptor.
func (d *SteeringDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(SteeringName, "invalid")
	}
	return scheduler.Join(SteeringName, df6.U32(s.GetTableId()), s.GetPrefix())
}

// Dependencies implements scheduler.Descriptor: the policy and the table.
func (d *SteeringDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := d.cast(obj)
	if err != nil {
		return nil
	}
	return append([]scheduler.Dependency{{Key: PolicyKey(s.GetBsid())}}, df6.VRFDeps(s.GetTableId())...)
}

func (d *SteeringDescriptor) request(ctx context.Context, s *Steering, isDel bool) (*srmplsapi.SrMplsSteeringAddDel, error) {
	p, err := df6.ParsePrefix(s.GetPrefix())
	if err != nil {
		return nil, err
	}
	// VPP adds/deletes the steering route in fib_table_find(table) unchecked.
	if err := df6.RequireTable(ctx, d.client, s.GetTableId(), p.Addr().Is6()); err != nil {
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

// routePresent reports whether the steering route (prefix in table) exists.
func (d *SteeringDescriptor) routePresent(ctx context.Context, s *Steering) (bool, error) {
	p, err := df6.ParsePrefix(s.GetPrefix())
	if err != nil {
		return false, err
	}
	stream, err := ip.NewServiceClient(d.client).IPRouteV2Dump(ctx, &ip.IPRouteV2Dump{Src: 0, Table: ip.IPTable{TableID: s.GetTableId(), IsIP6: p.Addr().Is6()}})
	if err != nil {
		return false, fmt.Errorf("ip_route_v2_dump: %w", err)
	}
	routes, err := df6.Collect(stream.Recv)
	if err != nil {
		return false, fmt.Errorf("ip_route_v2_dump: %w", err)
	}
	for _, r := range routes {
		if df6.FromPrefix(r.Route.Prefix) == p {
			return true, nil
		}
	}
	return false, nil
}

// Create implements scheduler.Descriptor. The policy must exist: VPP 26.06 leaves a
// half-created steering entry behind when the BSID is unknown.
func (d *SteeringDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	ok, err := BSIDPresent(ctx, d.client, s.GetBsid())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SteeringName, err)
	}
	if !ok {
		return nil, fmt.Errorf("%s: %w: bsid %d", SteeringName, ErrNoSuchPolicy, s.GetBsid())
	}
	req, err := d.request(ctx, s, false)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SteeringName, err)
	}
	if _, err := srmplsapi.NewServiceClient(d.client).SrMplsSteeringAddDel(ctx, req); err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_mpls_steering_add_del: %w", SteeringName, err))
	}
	return nil, nil
}

// Present reports whether the steering route of obj exists in VPP (the presence probe Delete
// uses; also the host-test evidence, since Retrieve is unsupported).
func (d *SteeringDescriptor) Present(ctx context.Context, obj proto.Message) (bool, error) {
	s, err := d.cast(obj)
	if err != nil {
		return false, err
	}
	return d.routePresent(ctx, s)
}

// Update implements scheduler.Descriptor: VPP refuses to re-point BSID steering; recreate.
func (d *SteeringDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor; a steering route VPP no longer has is deleted.
func (d *SteeringDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	s, err := d.cast(obj)
	if err != nil {
		return err
	}
	req, err := d.request(ctx, s, true)
	if err != nil {
		return fmt.Errorf("%s: %w", SteeringName, err)
	}
	ok, err := d.routePresent(ctx, s)
	if err != nil || !ok {
		return err
	}
	if _, err := srmplsapi.NewServiceClient(d.client).SrMplsSteeringAddDel(ctx, req); err != nil {
		return df6.PluginError(Plugin, fmt.Errorf("%s: sr_mpls_steering_add_del (del): %w", SteeringName, err))
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: no dump (see policy.go).
func (d *SteeringDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", SteeringName, df6.ErrRetrieveUnsupported)
}

// EndpointColorDescriptor assigns (endpoint, color) to a policy. Write-only; VPP has no
// message to clear the assignment other than deleting the policy, so Delete succeeds only
// when the policy is gone and otherwise returns df6.ErrNoDelete.
type EndpointColorDescriptor struct {
	client vpp.Client
}

// NewEndpointColor returns the descriptor.
func NewEndpointColor(c vpp.Client) *EndpointColorDescriptor {
	return &EndpointColorDescriptor{client: c}
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

// Create implements scheduler.Descriptor.
func (d *EndpointColorDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	e, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	ep, _ := df6.AddressOf(e.GetEndpoint())
	if _, err := srmplsapi.NewServiceClient(d.client).SrMplsPolicyAssignEndpointColor(ctx, &srmplsapi.SrMplsPolicyAssignEndpointColor{
		Bsid: e.GetBsid(), Endpoint: ep, Color: e.GetColor(),
	}); err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_mpls_policy_assign_endpoint_color: %w", EndpointColorName, err))
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: re-assigning replaces the previous pair in place.
func (d *EndpointColorDescriptor) Update(ctx context.Context, _, newObj proto.Message, meta any) (any, error) {
	if _, err := d.Create(ctx, newObj); err != nil {
		return nil, err
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor.
func (d *EndpointColorDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	e, err := d.cast(obj)
	if err != nil {
		return err
	}
	ok, err := BSIDPresent(ctx, d.client, e.GetBsid())
	if err != nil || !ok {
		return err
	}
	return fmt.Errorf("%s: %w: the assignment is cleared only by deleting policy %d", EndpointColorName, df6.ErrNoDelete, e.GetBsid())
}

// Retrieve implements scheduler.Descriptor: no dump.
func (d *EndpointColorDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", EndpointColorName, df6.ErrRetrieveUnsupported)
}
