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

// SteeringDescriptor manages SRv6 steering entries (ours by claim, D-071). The id is the
// steering key (family/table/prefix or l2/interface); the BSID is part of the identity: an
// entry that points at another BSID is never deleted or taken over (review M3).
type SteeringDescriptor = df6.KeyedDescriptor[*Steering]

// NewSteering returns the descriptor.
func NewSteering(c vpp.Client, owner string, opts ...df6.Option) *SteeringDescriptor {
	return df6.NewKeyedDescriptor(steeringSpec(owner), c, owner, opts...)
}

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

func steeringRequest(ctx context.Context, c vpp.Client, owner string, s *Steering, isDel bool) (*srapi.SrSteeringAddDel, error) {
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
		ifs, err := df6.DumpInterfaces(ctx, c, owner)
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
	if err := df6.RequireTable(ctx, c, s.GetTableId(), s.GetTrafficType() == SteerType_IPV6); err != nil {
		return nil, err
	}
	return req, nil
}

func steeringSpec(owner string) df6.KeyedSpec[*Steering] {
	send := func(ctx context.Context, c vpp.Client, s *Steering, isDel bool) error {
		req, err := steeringRequest(ctx, c, owner, s, isDel)
		if err != nil {
			return err
		}
		if _, err := srapi.NewServiceClient(c).SrSteeringAddDel(ctx, req); err != nil {
			return fmt.Errorf("sr_steering_add_del: %w", err)
		}
		return nil
	}
	return df6.KeyedSpec[*Steering]{
		Name:   SteeringName,
		Plugin: Plugin,
		Canon:  canonSteering,
		ID:     SteeringID,
		Deps: func(s *Steering) []scheduler.Dependency {
			deps := []scheduler.Dependency{{Key: PolicyKey(s.GetBsid())}}
			deps = append(deps, df6.VRFDeps(s.GetTableId())...)
			return append(deps, df6.InterfaceDeps(s.GetInterface())...)
		},
		Add:      func(ctx context.Context, c vpp.Client, s *Steering) error { return send(ctx, c, s, false) },
		Del:      func(ctx context.Context, c vpp.Client, s *Steering) error { return send(ctx, c, s, true) },
		Identity: func(want, have *Steering) bool { return want.GetBsid() == have.GetBsid() },
		// Update: a new BSID re-points our entry in place (VPP's add on an existing key);
		// only reached for claimed entries.
		Update: func(ctx context.Context, c vpp.Client, o, n *Steering) (bool, error) {
			probe := proto.Clone(n).(*Steering)
			probe.Bsid = o.GetBsid()
			if !proto.Equal(probe, o) {
				return false, nil
			}
			return true, send(ctx, c, n, false)
		},
		List: func(ctx context.Context, c vpp.Client) ([]*Steering, error) {
			recs, err := dumpSteering(ctx, c)
			if err != nil {
				return nil, err
			}
			ifs, err := df6.DumpInterfaces(ctx, c, owner)
			if err != nil {
				return nil, err
			}
			var out []*Steering
			for _, r := range recs {
				s := &Steering{TrafficType: SteerType(r.TrafficType), Bsid: df6.IP6String(r.Bsid)}
				switch s.GetTrafficType() {
				case SteerType_L2:
					s.Interface = ifs.NameOrEmpty(uint32(r.SwIfIndex))
					if s.Interface == "" {
						continue // foreign / unknown interface: never ours, and "l2/" would collide (L3)
					}
				case SteerType_IPV4, SteerType_IPV6:
					s.TableId = r.FibTable
					s.Prefix = steerPrefix(r.Prefix, s.GetTrafficType() == SteerType_IPV4)
				default:
					continue
				}
				out = append(out, s)
			}
			return out, nil
		},
	}
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
