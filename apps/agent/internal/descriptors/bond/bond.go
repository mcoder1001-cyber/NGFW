package bond

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	BondName   = iface.BondName // "bond.bond"
	MemberName = "bond.member"
)

// ErrEmptyValue is returned for a nil or foreign desired value.
var ErrEmptyValue = errors.New("bond: nil or wrong desired value type")

type base struct {
	client vpp.Client
	owner  string
}

func (b base) svc() bondapi.RPCService { return bondapi.NewServiceClient(b.client) }

// BondDescriptor implements bond.bond (bond_create2 / bond_delete). Everything in the model is
// immutable in VPP, so Update is always a recreate. enable_gso and a custom MAC are not modelled:
// sw_bond_interface_details does not report GSO, and the MAC belongs to interface.mac-address.
type BondDescriptor struct{ base }

// NewBond returns the descriptor for owner.
func NewBond(c vpp.Client, owner string) *BondDescriptor { return &BondDescriptor{base{c, owner}} }

func (*BondDescriptor) Name() string { return BondName }

func (*BondDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(BondName, obj.(*Bond).GetName())
}

func (*BondDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// lbFor validates the mode/lb combination the way VPP resolves it (vnet/bonding/cli.c
// bond_create_if): only XOR and LACP choose an algorithm; round-robin, active-backup and
// broadcast get their own value forced and reported by sw_bond_interface_dump, so the model must
// carry that value — while the request may only send L2/L23/L34 (anything else is
// VNET_API_ERROR_INVALID_ARGUMENT, verified on the host), hence the L2 placeholder for those modes.
func lbFor(o *Bond) (bondapi.BondMode, bondapi.BondLbAlgo, error) {
	mode := bondapi.BondMode(o.GetMode())
	lb := bondapi.BondLbAlgo(o.GetLb())
	var forced bondapi.BondLbAlgo
	switch mode {
	case bondapi.BOND_API_MODE_ROUND_ROBIN:
		forced = bondapi.BOND_API_LB_ALGO_RR
	case bondapi.BOND_API_MODE_ACTIVE_BACKUP:
		forced = bondapi.BOND_API_LB_ALGO_AB
	case bondapi.BOND_API_MODE_BROADCAST:
		forced = bondapi.BOND_API_LB_ALGO_BC
	case bondapi.BOND_API_MODE_XOR, bondapi.BOND_API_MODE_LACP:
		if lb != bondapi.BOND_API_LB_ALGO_L2 && lb != bondapi.BOND_API_LB_ALGO_L23 && lb != bondapi.BOND_API_LB_ALGO_L34 {
			return 0, 0, fmt.Errorf("bond: mode %v needs lb l2/l23/l34, got %v", o.GetMode(), o.GetLb())
		}
		return mode, lb, nil
	default:
		return 0, 0, fmt.Errorf("bond: unknown mode %v", o.GetMode())
	}
	if lb != forced {
		return 0, 0, fmt.Errorf("bond: mode %v requires lb %v (VPP forces it), got %v", o.GetMode(), LoadBalance(forced), o.GetLb())
	}
	return mode, bondapi.BOND_API_LB_ALGO_L2, nil
}

func (d *BondDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Bond)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetName() == "" || o.GetId() == ^uint32(0) {
		return nil, errors.New("bond: name and id are mandatory")
	}
	mode, lb, err := lbFor(o)
	if err != nil {
		return nil, err
	}
	rep, err := d.svc().BondCreate2(ctx, &bondapi.BondCreate2{Mode: mode, Lb: lb, NumaOnly: o.GetNumaOnly(), ID: o.GetId()})
	if err != nil {
		return nil, fmt.Errorf("bond_create2: %w", err)
	}
	idx := uint32(rep.SwIfIndex)
	if err := iface.Tag(ctx, d.client, d.owner, o.GetName(), idx); err != nil {
		return iface.Meta{SwIfIndex: idx}, err
	}
	return iface.Meta{SwIfIndex: idx}, nil
}

func (*BondDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

func (d *BondDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	if _, err := d.svc().BondDelete(ctx, &bondapi.BondDelete{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex)}); err != nil {
		return fmt.Errorf("bond_delete: %w", err)
	}
	return nil
}

// bonds returns every bond whose interface is owned by this agent, with its key.
func (b base) bonds(ctx context.Context, t *iface.Table) ([]*bondapi.SwBondInterfaceDetails, []scheduler.Key, error) {
	stream, err := b.svc().SwBondInterfaceDump(ctx, &bondapi.SwBondInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(iface.AllInterfaces)})
	if err != nil {
		return nil, nil, fmt.Errorf("sw_bond_interface_dump: %w", err)
	}
	var (
		out  []*bondapi.SwBondInterfaceDetails
		keys []scheduler.Key
	)
	for {
		bd, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("sw_bond_interface_dump: %w", err)
		}
		key, ok := t.KeyFor(uint32(bd.SwIfIndex))
		if !ok || key.Descriptor() != BondName {
			continue
		}
		out = append(out, bd)
		keys = append(keys, key)
	}
	return out, keys, nil
}

func (d *BondDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	bonds, keys, err := d.bonds(ctx, t)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(bonds))
	for i, bd := range bonds {
		out = append(out, scheduler.KV{
			Key:   keys[i],
			Value: &Bond{Name: keys[i].ID(), Id: bd.ID, Mode: Mode(bd.Mode), Lb: LoadBalance(bd.Lb), NumaOnly: bd.NumaOnly}, //nolint:gosec // enum values 0-5
			Meta:  iface.Meta{SwIfIndex: uint32(bd.SwIfIndex)},
		})
	}
	return out, nil
}
