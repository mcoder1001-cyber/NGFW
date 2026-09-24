package coretest

// F-bonding extension of the model: the bond plugin (bond_create2, bond_delete, bond_add_member,
// bond_detach_member, sw_interface_set_bond_weight and the two dumps) and the lacp plugin's
// sw_interface_lacp_dump, with the behaviour the host VPP 26.06 shows (vnet/bonding/cli.c): the lb
// algorithm is forced for round-robin / active-backup / broadcast and only l2/l34/l23 are accepted in
// a request, a bond interface cannot be a member, an interface is a member of at most one bond, a
// new membership has weight 0, weights are accepted on active-backup bonds only, deleting a bond
// detaches its members and deletes its sub-interfaces. Installed by New (one line in fakevpp.go).

import (
	"fmt"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/ethernet_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/lacp"
)

// VPP retvals of the bond plugin (vnet/api_errno.h, as govpp's api error table names them).
const (
	RetvalValueExist       = int32(api.VALUE_EXIST)       // already a member
	RetvalInvalidInterface = int32(api.INVALID_INTERFACE) // not a bond / not a member / a bond as member
	RetvalInvalidArgument  = int32(api.INVALID_ARGUMENT)  // lb value / weight on a non-active-backup bond
	retvalBondIDInUse      = int32(api.INSTANCE_IN_USE)   // bond_create2 with an id in use
)

// ModelBond is one modelled bond.
type ModelBond struct {
	Index    uint32
	ID       uint32
	Mode     bondapi.BondMode
	Lb       bondapi.BondLbAlgo
	NumaOnly bool
}

// ModelMember is one modelled membership.
type ModelMember struct {
	Index       uint32
	Bond        uint32
	Passive     bool
	LongTimeout bool
	Weight      uint32
}

type bondModel struct {
	bonds   map[uint32]*ModelBond   // bond sw_if_index →
	members map[uint32]*ModelMember // member sw_if_index →
}

// bondModels holds each VPP model's bond state (the VPP struct lives in fakevpp.go; guarded by VPP.mu).
var bondModels sync.Map // *VPP → *bondModel

func (v *VPP) bondModel() *bondModel {
	m, _ := bondModels.LoadOrStore(v, &bondModel{bonds: map[uint32]*ModelBond{}, members: map[uint32]*ModelMember{}})
	return m.(*bondModel)
}

// syncLocked forgets memberships and bonds whose interface was deleted behind the model's back (DeleteInterface,
// a tap removed by a test): VPP detaches a deleted member.
func (b *bondModel) syncLocked(v *VPP) {
	for idx := range b.bonds {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(b.bonds, idx)
		}
	}
	for idx, m := range b.members {
		if _, ok := v.Ifaces[idx]; !ok {
			delete(b.members, idx)
		} else if _, ok := b.bonds[m.Bond]; !ok {
			delete(b.members, idx)
		}
	}
}

func (v *VPP) installBonding() {
	b := v.bondModel()
	v.On("bond_create2", func(m api.Message) ([]api.Message, error) {
		req := m.(*bondapi.BondCreate2)
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		if req.ID == ^uint32(0) {
			return reply(&bondapi.BondCreate2Reply{Retval: RetvalInvalidArgument}) // the agent always names the id
		}
		for _, x := range b.bonds {
			if x.ID == req.ID {
				return reply(&bondapi.BondCreate2Reply{Retval: retvalBondIDInUse})
			}
		}
		if req.Lb > bondapi.BOND_API_LB_ALGO_L23 {
			return reply(&bondapi.BondCreate2Reply{Retval: RetvalInvalidArgument})
		}
		lb := req.Lb
		switch req.Mode {
		case bondapi.BOND_API_MODE_ROUND_ROBIN:
			lb = bondapi.BOND_API_LB_ALGO_RR
		case bondapi.BOND_API_MODE_ACTIVE_BACKUP:
			lb = bondapi.BOND_API_LB_ALGO_AB
		case bondapi.BOND_API_MODE_BROADCAST:
			lb = bondapi.BOND_API_LB_ALGO_BC
		}
		idx := v.next
		v.next++
		v.Ifaces[idx] = &Iface{
			Index: idx, Name: fmt.Sprintf("BondEthernet%d", req.ID), DevType: "bond", Addrs: map[string]bool{},
			LinkMtu: 9000, Mtu: [4]uint32{9000}, RxMode: interface_types.RX_MODE_API_POLLING,
			L2: [6]uint8{0x02, 0xfe, 0xb0, 0, 0, uint8(idx)}, //nolint:gosec // G115: a fake MAC byte; test indexes are small
		}
		b.bonds[idx] = &ModelBond{Index: idx, ID: req.ID, Mode: req.Mode, Lb: lb, NumaOnly: req.NumaOnly}
		return reply(&bondapi.BondCreate2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)})
	})
	v.On("bond_delete", func(m api.Message) ([]api.Message, error) {
		req := m.(*bondapi.BondDelete)
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		idx := uint32(req.SwIfIndex)
		if _, ok := b.bonds[idx]; !ok {
			return reply(&bondapi.BondDeleteReply{Retval: RetvalInvalidSwIfIndex})
		}
		for mi, mm := range b.members {
			if mm.Bond == idx {
				delete(b.members, mi)
			}
		}
		delete(b.bonds, idx)
		for _, sub := range v.Ifaces { // vnet_delete_hw_interface deletes the sub-interfaces too
			if sub.IsSub && sub.Sup == idx {
				v.dropInterfaceLocked(sub)
			}
		}
		v.dropInterfaceLocked(v.Ifaces[idx])
		return reply(&bondapi.BondDeleteReply{})
	})
	v.On("bond_add_member", func(m api.Message) ([]api.Message, error) {
		req := m.(*bondapi.BondAddMember)
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		bi, mi := uint32(req.BondSwIfIndex), uint32(req.SwIfIndex)
		if _, ok := b.bonds[bi]; !ok {
			return reply(&bondapi.BondAddMemberReply{Retval: RetvalInvalidInterface})
		}
		if _, ok := b.members[mi]; ok {
			return reply(&bondapi.BondAddMemberReply{Retval: RetvalValueExist})
		}
		if _, ok := v.Ifaces[mi]; !ok {
			return reply(&bondapi.BondAddMemberReply{Retval: RetvalInvalidSwIfIndex})
		}
		if _, isBond := b.bonds[mi]; isBond {
			return reply(&bondapi.BondAddMemberReply{Retval: RetvalInvalidInterface})
		}
		b.members[mi] = &ModelMember{Index: mi, Bond: bi, Passive: req.IsPassive, LongTimeout: req.IsLongTimeout}
		return reply(&bondapi.BondAddMemberReply{})
	})
	v.On("bond_detach_member", func(m api.Message) ([]api.Message, error) {
		req := m.(*bondapi.BondDetachMember)
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		if _, ok := b.members[uint32(req.SwIfIndex)]; !ok {
			return reply(&bondapi.BondDetachMemberReply{Retval: RetvalInvalidInterface})
		}
		delete(b.members, uint32(req.SwIfIndex))
		return reply(&bondapi.BondDetachMemberReply{})
	})
	v.On("sw_interface_set_bond_weight", func(m api.Message) ([]api.Message, error) {
		req := m.(*bondapi.SwInterfaceSetBondWeight)
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		mm, ok := b.members[uint32(req.SwIfIndex)]
		if !ok {
			return reply(&bondapi.SwInterfaceSetBondWeightReply{Retval: RetvalInvalidInterface})
		}
		if b.bonds[mm.Bond].Mode != bondapi.BOND_API_MODE_ACTIVE_BACKUP {
			return reply(&bondapi.SwInterfaceSetBondWeightReply{Retval: RetvalInvalidArgument})
		}
		mm.Weight = req.Weight
		return reply(&bondapi.SwInterfaceSetBondWeightReply{})
	})
	v.On("sw_bond_interface_dump", func(m api.Message) ([]api.Message, error) {
		req := m.(*bondapi.SwBondInterfaceDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		var out []api.Message
		for _, idx := range sortedIdx(b.bonds) {
			if uint32(req.SwIfIndex) != ^uint32(0) && uint32(req.SwIfIndex) != idx {
				continue
			}
			x := b.bonds[idx]
			d := &bondapi.SwBondInterfaceDetails{
				SwIfIndex: interface_types.InterfaceIndex(idx), ID: x.ID, Mode: x.Mode, Lb: x.Lb, NumaOnly: x.NumaOnly,
				InterfaceName: v.Ifaces[idx].Name,
			}
			for mi, mm := range b.members {
				if mm.Bond == idx {
					d.Members++
					if v.Ifaces[mi].AdminUp && v.Ifaces[idx].AdminUp && x.Mode != bondapi.BOND_API_MODE_LACP {
						d.ActiveMembers++ // LACP members stay inactive: there is no partner in the model
					}
				}
			}
			out = append(out, d)
		}
		return out, nil
	})
	v.On("sw_member_interface_dump", func(m api.Message) ([]api.Message, error) {
		req := m.(*bondapi.SwMemberInterfaceDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		var out []api.Message
		for _, mi := range sortedIdx(b.members) {
			mm := b.members[mi]
			if mm.Bond != uint32(req.SwIfIndex) {
				continue
			}
			out = append(out, &bondapi.SwMemberInterfaceDetails{
				SwIfIndex: interface_types.InterfaceIndex(mi), InterfaceName: v.Ifaces[mi].Name,
				IsPassive: mm.Passive, IsLongTimeout: mm.LongTimeout, IsLocalNuma: true, Weight: mm.Weight,
			})
		}
		return out, nil
	})
	v.On("sw_interface_lacp_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		b.syncLocked(v)
		var out []api.Message
		for _, mi := range sortedIdx(b.members) {
			mm := b.members[mi]
			bd := b.bonds[mm.Bond]
			if bd.Mode != bondapi.BOND_API_MODE_LACP {
				continue
			}
			actorState := uint8(0x01 | 0x04 | 0x40) // activity, aggregation, defaulted: no partner
			if !mm.LongTimeout {
				actorState |= 0x02
			}
			out = append(out, &lacp.SwInterfaceLacpDetails{
				SwIfIndex: interface_types.InterfaceIndex(mi), InterfaceName: v.Ifaces[mi].Name, BondInterfaceName: v.Ifaces[mm.Bond].Name,
				RxState: 4, TxState: 0, MuxState: 0, PtxState: 1, // defaulted, transmit, detached, fast periodic
				ActorSystemPriority: 0xffff, ActorSystem: ethernet_types.MacAddress(v.Ifaces[mm.Bond].L2), ActorKey: uint16(bd.ID), //nolint:gosec // G115: test ids are small
				ActorPortPriority: 0xff, ActorPortNumber: uint16(mi), ActorState: actorState, //nolint:gosec // G115: test indexes are small
				PartnerSystemPriority: 0xffff, PartnerKey: 0, PartnerPortPriority: 0xff, PartnerPortNumber: 0, PartnerState: 0,
			})
		}
		return out, nil
	})
}

func sortedIdx[V any](m map[uint32]V) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Bond returns a copy of the modelled bond named name (BondEthernet<id>).
func (v *VPP) Bond(name string) (ModelBond, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	b := v.bondModel()
	b.syncLocked(v)
	for idx, x := range b.bonds {
		if v.Ifaces[idx].Name == name {
			return *x, true
		}
	}
	return ModelBond{}, false
}

// BondMembers returns the VPP names of the members of the bond named name with their memberships, sorted by name.
func (v *VPP) BondMembers(name string) map[string]ModelMember {
	v.mu.Lock()
	defer v.mu.Unlock()
	b := v.bondModel()
	b.syncLocked(v)
	out := map[string]ModelMember{}
	for mi, mm := range b.members {
		if v.Ifaces[mm.Bond].Name == name {
			out[v.Ifaces[mi].Name] = *mm
		}
	}
	return out
}

// BondCount is the number of modelled bonds.
func (v *VPP) BondCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	b := v.bondModel()
	b.syncLocked(v)
	return len(b.bonds)
}
