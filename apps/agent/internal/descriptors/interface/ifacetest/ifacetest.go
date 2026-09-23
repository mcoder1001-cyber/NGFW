// Package ifacetest is the shared test double of the interface layer for the DF-1 descriptor
// packages: a stateful fake VPP (internal/vpp/fake plus an interface table that
// sw_interface_dump, tag/flags/mtu/mac/promisc/rx-mode/rx-placement/subif handlers operate
// on) and, for integration tests, a govpp connection to the host VPP wrapped as vpp.Client.
// Plugin packages embed *VPP and add their own message handlers.
package ifacetest

import (
	"fmt"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/vpp/fake"
)

// VPP retval codes used by the fake (vnet/api_errno.h).
const (
	RetvalInvalidSwIfIndex = -2
	RetvalUnimplemented    = -3
	RetvalInvalidValue     = -9
	RetvalSubifDoesntExist = -55
)

// Queue is one rx queue of a fake hardware interface: its thread (0 = main) and rx mode.
type Queue struct {
	ID     uint32
	Thread uint32
	Mode   interface_types.RxMode
}

// VPP is the fake. Exported fields may be inspected and pre-populated by tests; use the
// methods when the fake is running concurrently.
type VPP struct {
	*fake.Client
	Mu      sync.Mutex
	Next    uint32
	Ifs     map[uint32]*ifapi.SwInterfaceDetails
	Promisc map[uint32]bool
	Queues  map[uint32][]Queue
	Workers uint32
	// FailTag makes the next FailTag sw_interface_tag_add_del calls fail (retval -9).
	FailTag int
	// PID is the main-thread PID show_threads reports (the VPP identity); change it to simulate a
	// VPP restart.
	PID uint32
}

// New returns a fake with local0 (index 0) and handlers for the interface.api messages the
// DF-1 descriptors use.
func New() *VPP {
	v := &VPP{
		Client:  fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		Next:    1,
		Ifs:     map[uint32]*ifapi.SwInterfaceDetails{},
		Promisc: map[uint32]bool{},
		Queues:  map[uint32][]Queue{},
	}
	v.PID = 4242
	v.On("show_threads", func(api.Message) ([]api.Message, error) {
		v.Mu.Lock()
		defer v.Mu.Unlock()
		return []api.Message{&vlib.ShowThreadsReply{Count: 1, ThreadData: []vlib.ThreadData{{ID: 0, Name: "vpp_main", PID: v.PID}}}}, nil
	})
	v.Ifs[0] = &ifapi.SwInterfaceDetails{InterfaceName: "local0", InterfaceDevType: "local", Mtu: []uint32{0, 0, 0, 0}}
	v.On("sw_interface_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceDump)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		var out []api.Message
		for _, idx := range v.sortedLocked() {
			if uint32(r.SwIfIndex) != ^uint32(0) && uint32(r.SwIfIndex) != idx {
				continue
			}
			cp := *v.Ifs[idx]
			cp.Mtu = append([]uint32(nil), cp.Mtu...)
			out = append(out, &cp)
		}
		return out, nil
	})
	v.On("sw_interface_tag_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceTagAddDel)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		if v.FailTag > 0 { // injected failure (review M3 tests)
			v.FailTag--
			return []api.Message{&ifapi.SwInterfaceTagAddDelReply{Retval: RetvalInvalidValue}}, nil
		}
		i, ok := v.Ifs[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&ifapi.SwInterfaceTagAddDelReply{Retval: RetvalInvalidSwIfIndex}}, nil
		}
		if r.IsAdd {
			i.Tag = r.Tag
		} else {
			i.Tag = ""
		}
		return []api.Message{&ifapi.SwInterfaceTagAddDelReply{}}, nil
	})
	v.On("sw_interface_set_flags", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceSetFlags)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		i, ok := v.Ifs[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&ifapi.SwInterfaceSetFlagsReply{Retval: RetvalInvalidSwIfIndex}}, nil
		}
		i.Flags = r.Flags & interface_types.IF_STATUS_API_FLAG_ADMIN_UP
		return []api.Message{&ifapi.SwInterfaceSetFlagsReply{}}, nil
	})
	v.On("sw_interface_set_mtu", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceSetMtu)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		i, ok := v.Ifs[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&ifapi.SwInterfaceSetMtuReply{Retval: RetvalInvalidSwIfIndex}}, nil
		}
		i.Mtu = append([]uint32(nil), r.Mtu...)
		return []api.Message{&ifapi.SwInterfaceSetMtuReply{}}, nil
	})
	v.On("sw_interface_set_mac_address", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceSetMacAddress)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		i, ok := v.Ifs[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&ifapi.SwInterfaceSetMacAddressReply{Retval: RetvalInvalidSwIfIndex}}, nil
		}
		i.L2Address = r.MacAddress
		return []api.Message{&ifapi.SwInterfaceSetMacAddressReply{}}, nil
	})
	v.On("sw_interface_set_promisc", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceSetPromisc)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		if _, ok := v.Ifs[uint32(r.SwIfIndex)]; !ok {
			return []api.Message{&ifapi.SwInterfaceSetPromiscReply{Retval: RetvalInvalidSwIfIndex}}, nil
		}
		v.Promisc[uint32(r.SwIfIndex)] = r.PromiscOn
		return []api.Message{&ifapi.SwInterfaceSetPromiscReply{}}, nil
	})
	v.On("sw_interface_set_rx_mode", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceSetRxMode)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		qs := v.Queues[uint32(r.SwIfIndex)]
		if len(qs) == 0 {
			return []api.Message{&ifapi.SwInterfaceSetRxModeReply{Retval: RetvalUnimplemented}}, nil
		}
		for i := range qs {
			if !r.QueueIDValid || qs[i].ID == r.QueueID {
				qs[i].Mode = r.Mode
			}
		}
		return []api.Message{&ifapi.SwInterfaceSetRxModeReply{}}, nil
	})
	v.On("sw_interface_set_rx_placement", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceSetRxPlacement)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		qs := v.Queues[uint32(r.SwIfIndex)]
		for i := range qs {
			if qs[i].ID != r.QueueID {
				continue
			}
			if r.IsMain {
				qs[i].Thread = 0
			} else if r.WorkerID >= v.Workers {
				return []api.Message{&ifapi.SwInterfaceSetRxPlacementReply{Retval: RetvalInvalidValue}}, nil
			} else {
				qs[i].Thread = r.WorkerID + 1
			}
			return []api.Message{&ifapi.SwInterfaceSetRxPlacementReply{}}, nil
		}
		return []api.Message{&ifapi.SwInterfaceSetRxPlacementReply{Retval: RetvalInvalidValue}}, nil
	})
	v.On("sw_interface_rx_placement_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceRxPlacementDump)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		var out []api.Message
		for _, idx := range v.sortedLocked() {
			if uint32(r.SwIfIndex) != ^uint32(0) && uint32(r.SwIfIndex) != idx {
				continue
			}
			for _, q := range v.Queues[idx] {
				out = append(out, &ifapi.SwInterfaceRxPlacementDetails{SwIfIndex: interface_types.InterfaceIndex(idx), QueueID: q.ID, WorkerID: q.Thread, Mode: q.Mode})
			}
		}
		return out, nil
	})
	v.On("create_subif", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.CreateSubif)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		p, ok := v.Ifs[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&ifapi.CreateSubifReply{Retval: RetvalInvalidSwIfIndex}}, nil
		}
		idx := v.addLocked(fmt.Sprintf("%s.%d", p.InterfaceName, r.SubID), p.InterfaceDevType)
		i := v.Ifs[idx]
		i.Type = interface_types.IF_API_TYPE_SUB
		i.SupSwIfIndex = uint32(r.SwIfIndex)
		i.SubID = r.SubID
		i.SubIfFlags = r.SubIfFlags
		i.SubOuterVlanID, i.SubInnerVlanID = r.OuterVlanID, r.InnerVlanID
		i.L2Address = p.L2Address
		i.Mtu = []uint32{0, 0, 0, 0} // VPP 26.06: sub-interfaces start without MTUs (host-verified)
		switch {
		case r.SubIfFlags&interface_types.SUB_IF_API_FLAG_TWO_TAGS != 0:
			i.SubNumberOfTags = 2
		case r.SubIfFlags&interface_types.SUB_IF_API_FLAG_ONE_TAG != 0:
			i.SubNumberOfTags = 1
		}
		return []api.Message{&ifapi.CreateSubifReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	v.On("delete_subif", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.DeleteSubif)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		i, ok := v.Ifs[uint32(r.SwIfIndex)]
		if !ok || i.Type != interface_types.IF_API_TYPE_SUB {
			return []api.Message{&ifapi.DeleteSubifReply{Retval: RetvalSubifDoesntExist}}, nil
		}
		delete(v.Ifs, uint32(r.SwIfIndex))
		return []api.Message{&ifapi.DeleteSubifReply{}}, nil
	})
	v.On("create_loopback_instance", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.CreateLoopbackInstance)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		idx := v.addLocked(fmt.Sprintf("loop%d", r.UserInstance), "Loopback")
		v.Ifs[idx].L2Address = [6]uint8{0xde, 0xad, 0, 0, 0, uint8(r.UserInstance)} //nolint:gosec // fake
		return []api.Message{&ifapi.CreateLoopbackInstanceReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	v.On("delete_loopback", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.DeleteLoopback)
		v.Mu.Lock()
		defer v.Mu.Unlock()
		delete(v.Ifs, uint32(r.SwIfIndex))
		return []api.Message{&ifapi.DeleteLoopbackReply{}}, nil
	})
	return v
}

func (v *VPP) sortedLocked() []uint32 {
	out := make([]uint32, 0, len(v.Ifs))
	for idx := range v.Ifs {
		out = append(out, idx)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (v *VPP) addLocked(name, devType string) uint32 {
	idx := v.Next
	v.Next++
	v.Ifs[idx] = &ifapi.SwInterfaceDetails{
		SwIfIndex:        interface_types.InterfaceIndex(idx),
		SupSwIfIndex:     idx,
		InterfaceName:    name,
		InterfaceDevType: devType,
		L2Address:        [6]uint8{0x02, 0xfe, 0, 0, 0, uint8(idx)}, //nolint:gosec // fake
		Mtu:              []uint32{9000, 0, 0, 0},
		LinkMtu:          9000,
	}
	return idx
}

// Add creates a hardware interface of the given VPP device class ("Loopback", "tap",
// "af-packet", "bond", "memif", …) with an optional tag and one rx queue on the main thread in
// the class's default mode (polling; interrupt for af-packet). It returns the sw_if_index.
func (v *VPP) Add(name, devType, tag string) uint32 {
	v.Mu.Lock()
	defer v.Mu.Unlock()
	idx := v.addLocked(name, devType)
	v.Ifs[idx].Tag = tag
	if devType != "Loopback" && devType != "bond" {
		mode := interface_types.RX_MODE_API_POLLING
		if devType == "af-packet" { // af_packet.c forces interrupt mode on creation
			mode = interface_types.RX_MODE_API_INTERRUPT
		}
		v.Queues[idx] = []Queue{{ID: 0, Thread: 0, Mode: mode}}
	}
	return idx
}

// Remove deletes an interface (and its queues / promisc state).
func (v *VPP) Remove(idx uint32) {
	v.Mu.Lock()
	defer v.Mu.Unlock()
	delete(v.Ifs, idx)
	delete(v.Queues, idx)
	delete(v.Promisc, idx)
}

// Get returns a copy of the interface row, false when it does not exist.
func (v *VPP) Get(idx uint32) (ifapi.SwInterfaceDetails, bool) {
	v.Mu.Lock()
	defer v.Mu.Unlock()
	i, ok := v.Ifs[idx]
	if !ok {
		return ifapi.SwInterfaceDetails{}, false
	}
	return *i, true
}

// IndexByName returns the sw_if_index of the interface named name.
func (v *VPP) IndexByName(name string) (uint32, bool) {
	v.Mu.Lock()
	defer v.Mu.Unlock()
	for idx, i := range v.Ifs {
		if i.InterfaceName == name {
			return idx, true
		}
	}
	return 0, false
}
