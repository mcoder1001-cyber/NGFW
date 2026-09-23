package sr

import (
	"context"
	"fmt"
	"net/netip"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/binapi/sr_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// SteeringName is the descriptor name; keys are "sr.steering/l2/<interface>" and
// "sr.steering/<ipv4|ipv6>/<table>/<prefix>".
const SteeringName = "sr.steering"

// SteeringDescriptor manages SRv6 steering entries (attributed by the policy BSID).
type SteeringDescriptor struct {
	client vpp.Client
	scope  *df6.Scope
}

// NewSteering returns the descriptor.
func NewSteering(c vpp.Client, scope *df6.Scope) *SteeringDescriptor {
	return &SteeringDescriptor{client: c, scope: scope}
}

// Name implements scheduler.Descriptor.
func (d *SteeringDescriptor) Name() string { return SteeringName }

func canonSteering(s *Steering) (*Steering, error) {
	bsid, err := df6.ParseAddr6(s.GetBsid())
	if err != nil {
		return nil, err
	}
	c := proto.Clone(s).(*Steering)
	c.Bsid = bsid.String()
	switch s.GetTrafficType() {
	case SteerType_L2:
		if s.GetInterface() == "" || s.GetPrefix() != "" || s.GetTableId() != 0 {
			return nil, fmt.Errorf("%w: L2 steering takes interface only", df6.ErrBadValue)
		}
	case SteerType_IPV4, SteerType_IPV6:
		if s.GetInterface() != "" {
			return nil, fmt.Errorf("%w: L3 steering takes prefix and table only", df6.ErrBadValue)
		}
		p, err := df6.ParsePrefix(s.GetPrefix())
		if err != nil {
			return nil, err
		}
		if p.Addr().Is4() != (s.GetTrafficType() == SteerType_IPV4) {
			return nil, fmt.Errorf("%w: prefix %s does not match traffic type %s", df6.ErrBadValue, p, s.GetTrafficType())
		}
		c.Prefix = p.String()
	default:
		return nil, fmt.Errorf("%w: traffic_type %d", df6.ErrBadValue, s.GetTrafficType())
	}
	return c, nil
}

// SteeringID is the object id of s (canonical).
func SteeringID(s *Steering) string {
	if s.GetTrafficType() == SteerType_L2 {
		return "l2/" + s.GetInterface()
	}
	fam := "ipv6"
	if s.GetTrafficType() == SteerType_IPV4 {
		fam = "ipv4"
	}
	return fam + "/" + df6.U32(s.GetTableId()) + "/" + s.GetPrefix()
}

func (d *SteeringDescriptor) cast(obj proto.Message) (*Steering, error) {
	s, ok := obj.(*Steering)
	if !ok {
		return nil, fmt.Errorf("%s: %w: %T", SteeringName, df6.ErrBadValue, obj)
	}
	c, err := canonSteering(s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SteeringName, err)
	}
	return c, nil
}

// KeyOf implements scheduler.Descriptor.
func (d *SteeringDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := d.cast(obj)
	if err != nil {
		return scheduler.Join(SteeringName, "invalid")
	}
	return scheduler.Join(SteeringName, SteeringID(s))
}

// Dependencies implements scheduler.Descriptor: the policy, plus the table (L3) or the
// interface (L2).
func (d *SteeringDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := d.cast(obj)
	if err != nil {
		return nil
	}
	deps := []scheduler.Dependency{{Key: PolicyKey(s.GetBsid())}}
	deps = append(deps, df6.VRFDeps(s.GetTableId())...)
	return append(deps, df6.InterfaceDeps(s.GetInterface())...)
}

func (d *SteeringDescriptor) request(ctx context.Context, s *Steering, isDel bool) (*srapi.SrSteeringAddDel, error) {
	bsid, err := df6.IP6Of(s.GetBsid())
	if err != nil {
		return nil, err
	}
	req := &srapi.SrSteeringAddDel{
		IsDel:         isDel,
		BsidAddr:      bsid,
		SrPolicyIndex: ^uint32(0),
		TableID:       s.GetTableId(),
		SwIfIndex:     interface_types.InterfaceIndex(df6.NoInterface),
		TrafficType:   sr_types.SrSteer(s.GetTrafficType()), //nolint:gosec // validated enum
	}
	if s.GetTrafficType() == SteerType_L2 {
		ifs, err := df6.DumpInterfaces(ctx, d.client, "")
		if err != nil {
			return nil, err
		}
		if req.SwIfIndex, err = ifs.Index(s.GetInterface()); err != nil {
			return nil, err
		}
		return req, nil
	}
	if req.Prefix, err = df6.PrefixOf(s.GetPrefix()); err != nil {
		return nil, err
	}
	// VPP adds/deletes the steering FIB entry in fib_table_find(table) unchecked.
	if err := df6.RequireTable(ctx, d.client, s.GetTableId(), s.GetTrafficType() == SteerType_IPV6); err != nil {
		return nil, err
	}
	return req, nil
}

// Create implements scheduler.Descriptor.
func (d *SteeringDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := d.cast(obj)
	if err != nil {
		return nil, err
	}
	req, err := d.request(ctx, s, false)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SteeringName, err)
	}
	if _, err := srapi.NewServiceClient(d.client).SrSteeringAddDel(ctx, req); err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("%s: sr_steering_add_del: %w", SteeringName, err))
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: a new BSID re-points the existing entry in place
// (VPP's add on an existing steering key); anything else is a different key anyway.
func (d *SteeringDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, err := d.cast(oldObj)
	if err != nil {
		return nil, err
	}
	n, err := d.cast(newObj)
	if err != nil {
		return nil, err
	}
	probe := proto.Clone(n).(*Steering)
	probe.Bsid = o.GetBsid()
	if !proto.Equal(probe, o) {
		return nil, scheduler.ErrRecreate
	}
	if _, err := d.Create(ctx, n); err != nil {
		return nil, err
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor; an entry VPP no longer has is already deleted.
func (d *SteeringDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	s, err := d.cast(obj)
	if err != nil {
		return err
	}
	kvs, err := d.retrieve(ctx, nil)
	if err != nil {
		return err
	}
	id := SteeringID(s)
	found := false
	for _, kv := range kvs {
		if kv.Key.ID() == id {
			found = true
		}
	}
	if !found {
		return nil
	}
	req, err := d.request(ctx, s, true)
	if err != nil {
		return fmt.Errorf("%s: %w", SteeringName, err)
	}
	if _, err := srapi.NewServiceClient(d.client).SrSteeringAddDel(ctx, req); err != nil {
		return df6.PluginError(Plugin, fmt.Errorf("%s: sr_steering_add_del (del): %w", SteeringName, err))
	}
	return nil
}

func dumpSteering(ctx context.Context, c vpp.Client) ([]*srapi.SrSteeringPolDetails, error) {
	stream, err := srapi.NewServiceClient(c).SrSteeringPolDump(ctx, &srapi.SrSteeringPolDump{})
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("sr_steering_pol_dump: %w", err))
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("sr_steering_pol_dump: %w", err))
	}
	return recs, nil
}

// Retrieve implements scheduler.Descriptor: every steering entry whose policy BSID is in scope.
func (d *SteeringDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	return d.retrieve(ctx, d.scope)
}

func (d *SteeringDescriptor) retrieve(ctx context.Context, scope *df6.Scope) ([]scheduler.KV, error) {
	recs, err := dumpSteering(ctx, d.client)
	if err != nil {
		return nil, err
	}
	ifs, err := df6.DumpInterfaces(ctx, d.client, d.scope.OwnerOf())
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, r := range recs {
		bsid := df6.IP6String(r.Bsid)
		if !scope.OwnsAddrString(bsid) {
			continue
		}
		s := &Steering{TrafficType: SteerType(r.TrafficType), Bsid: bsid}
		switch s.GetTrafficType() {
		case SteerType_L2:
			s.Interface = ifs.NameOrEmpty(uint32(r.SwIfIndex))
		case SteerType_IPV4, SteerType_IPV6:
			s.TableId = r.FibTable
			s.Prefix = steerPrefix(r.Prefix, s.GetTrafficType() == SteerType_IPV4)
		default:
			continue
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(SteeringName, SteeringID(s)), Value: s})
	}
	return out, nil
}

// steerPrefix decodes the dump's prefix; VPP encodes it from an ip46 address with
// IP46_TYPE_ANY, so an IPv4 prefix may arrive tagged as either family.
func steerPrefix(p ip_types.Prefix, v4 bool) string {
	a := df6.FromAddress(p.Address)
	if v4 && a.Is6() {
		b := a.As16()
		var four [4]byte
		copy(four[:], b[12:])
		p = ip_types.Prefix{Address: df6.ToAddress(netip.AddrFrom4(four)), Len: p.Len}
	}
	return df6.PrefixString(p)
}
