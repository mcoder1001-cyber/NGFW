package iface

// Attribute descriptors decorate an existing interface of any owned type with one setting each.
// They are keyed on the interface id ("interface.mtu/loop201"), depend on the interface's full
// key, update in place and never recreate the interface. Retrieve rules (which owned interfaces
// yield an object) are documented per descriptor and in docs/agent/descriptors/interface.md:
//
//	admin-state  present ⇔ IF_STATUS_API_FLAG_ADMIN_UP          Delete → admin down
//	mtu          present ⇔ any of the four MTUs is non-zero     Delete → all four 0 (inherit link MTU)
//	mac-address  present for every owned hardware interface     Delete → no-op (VPP has no "unset MAC")
//	promisc      present ⇔ this process switched it on          Delete → off  (not readable back from VPP)
//	rx-mode      present ⇔ a queue is interrupt/adaptive         Delete → polling
//	rx-placement present ⇔ a queue sits on a worker thread       Delete → main thread

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"sync"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	AdminStateName  = "interface.admin-state"
	MtuName         = "interface.mtu"
	MacAddressName  = "interface.mac-address"
	PromiscName     = "interface.promisc"
	RxModeName      = "interface.rx-mode"
	RxPlacementName = "interface.rx-placement"
)

type base struct {
	client vpp.Client
	owner  string
}

func (b base) svc() ifapi.RPCService { return ifapi.NewServiceClient(b.client) }

func (b base) resolve(ctx context.Context, ref string) (uint32, error) {
	return Resolve(ctx, b.client, b.owner, ref)
}

func dep(ref string) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: scheduler.Key(ref)}}
}

// ---------------------------------------------------------------------------- admin-state

// AdminState implements interface.admin-state (sw_interface_set_flags).
type AdminStateDescriptor struct{ base }

// NewAdminState returns the descriptor for owner.
func NewAdminState(c vpp.Client, owner string) *AdminStateDescriptor {
	return &AdminStateDescriptor{base{c, owner}}
}

func (*AdminStateDescriptor) Name() string { return AdminStateName }

func (*AdminStateDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(AdminStateName, RefID(obj.(*AdminState).GetInterface()))
}

func (*AdminStateDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*AdminState).GetInterface())
}

func (d *AdminStateDescriptor) setFlags(ctx context.Context, idx uint32, up bool) error {
	var flags interface_types.IfStatusFlags
	if up {
		flags = interface_types.IF_STATUS_API_FLAG_ADMIN_UP
	}
	if _, err := d.svc().SwInterfaceSetFlags(ctx, &ifapi.SwInterfaceSetFlags{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: flags}); err != nil {
		return fmt.Errorf("sw_interface_set_flags: %w", err)
	}
	return nil
}

func (d *AdminStateDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*AdminState)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	return Meta{idx}, d.setFlags(ctx, idx, true)
}

// Update has nothing to change in place: the object has no mutable field.
func (d *AdminStateDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return MetaOf(meta)
}

func (d *AdminStateDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	return d.setFlags(ctx, m.SwIfIndex, false)
}

func (d *AdminStateDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.KeyFor(idx)
		if !ok || t.byIndex[idx].Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP == 0 {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(AdminStateName, key.ID()),
			Value: &AdminState{Interface: string(key)},
			Meta:  Meta{idx},
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------- mtu

// MtuDescriptor implements interface.mtu (sw_interface_set_mtu, the per-protocol form). The
// hardware MTU (hw_interface_set_mtu) is deliberately not managed: it is a driver property that
// most virtual devices reject, while the sw MTU works on every interface incl. sub-interfaces.
type MtuDescriptor struct{ base }

// NewMtu returns the descriptor for owner.
func NewMtu(c vpp.Client, owner string) *MtuDescriptor { return &MtuDescriptor{base{c, owner}} }

func (*MtuDescriptor) Name() string { return MtuName }

func (*MtuDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(MtuName, RefID(obj.(*Mtu).GetInterface()))
}

func (*MtuDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*Mtu).GetInterface())
}

func (d *MtuDescriptor) set(ctx context.Context, idx uint32, o *Mtu) error {
	req := &ifapi.SwInterfaceSetMtu{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Mtu:       []uint32{o.GetMtu(), o.GetIp4(), o.GetIp6(), o.GetMpls()},
	}
	if _, err := d.svc().SwInterfaceSetMtu(ctx, req); err != nil {
		return fmt.Errorf("sw_interface_set_mtu: %w", err)
	}
	return nil
}

func (d *MtuDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Mtu)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	return Meta{idx}, d.set(ctx, idx, o)
}

func (d *MtuDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := MetaOf(meta)
	if err != nil {
		return nil, err
	}
	if oldObj.(*Mtu).GetInterface() != newObj.(*Mtu).GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	return m, d.set(ctx, m.SwIfIndex, newObj.(*Mtu))
}

func (d *MtuDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	return d.set(ctx, m.SwIfIndex, &Mtu{})
}

func (d *MtuDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.KeyFor(idx)
		if !ok {
			continue
		}
		mtu := t.byIndex[idx].Mtu
		for len(mtu) < 4 {
			mtu = append(mtu, 0)
		}
		if mtu[0]|mtu[1]|mtu[2]|mtu[3] == 0 {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(MtuName, key.ID()),
			Value: &Mtu{Interface: string(key), Mtu: mtu[0], Ip4: mtu[1], Ip6: mtu[2], Mpls: mtu[3]},
			Meta:  Meta{idx},
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------- mac-address

// MacAddressDescriptor implements interface.mac-address (sw_interface_set_mac_address).
type MacAddressDescriptor struct{ base }

// NewMacAddress returns the descriptor for owner.
func NewMacAddress(c vpp.Client, owner string) *MacAddressDescriptor {
	return &MacAddressDescriptor{base{c, owner}}
}

func (*MacAddressDescriptor) Name() string { return MacAddressName }

func (*MacAddressDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(MacAddressName, RefID(obj.(*MacAddress).GetInterface()))
}

func (*MacAddressDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*MacAddress).GetInterface())
}

// ParseMAC parses a 48-bit MAC and returns it in VPP's array form.
func ParseMAC(s string) ([6]uint8, error) {
	var out [6]uint8
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return out, fmt.Errorf("iface: invalid mac address %q", s)
	}
	copy(out[:], hw)
	return out, nil
}

// FormatMAC is the canonical lower-case form Retrieve uses; "" for the zero address.
func FormatMAC(mac [6]uint8) string {
	if mac == [6]uint8{} {
		return ""
	}
	return net.HardwareAddr(mac[:]).String()
}

func (d *MacAddressDescriptor) set(ctx context.Context, idx uint32, mac string) error {
	m, err := ParseMAC(mac)
	if err != nil {
		return err
	}
	if _, err := d.svc().SwInterfaceSetMacAddress(ctx, &ifapi.SwInterfaceSetMacAddress{SwIfIndex: interface_types.InterfaceIndex(idx), MacAddress: m}); err != nil {
		return fmt.Errorf("sw_interface_set_mac_address: %w", err)
	}
	return nil
}

func (d *MacAddressDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*MacAddress)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	return Meta{idx}, d.set(ctx, idx, o.GetMac())
}

func (d *MacAddressDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := MetaOf(meta)
	if err != nil {
		return nil, err
	}
	if oldObj.(*MacAddress).GetInterface() != newObj.(*MacAddress).GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	return m, d.set(ctx, m.SwIfIndex, newObj.(*MacAddress).GetMac())
}

// Delete keeps the current address: VPP has no notion of an unset MAC. The interface's own
// descriptor deleting the interface is what removes it.
func (d *MacAddressDescriptor) Delete(_ context.Context, _ proto.Message, meta any) error {
	_, err := MetaOf(meta)
	return err
}

func (d *MacAddressDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.KeyFor(idx)
		if !ok || key.Descriptor() == SubinterfaceName {
			continue // sub-interfaces share the parent's address
		}
		mac := FormatMAC(t.byIndex[idx].L2Address)
		if mac == "" {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(MacAddressName, key.ID()),
			Value: &MacAddress{Interface: string(key), Mac: mac},
			Meta:  Meta{idx},
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------- promisc

// PromiscDescriptor implements interface.promisc (sw_interface_set_promisc). VPP 26.06 does not
// report promiscuous mode in any dump, so Retrieve reflects what this process switched on; after
// an agent restart the desired objects are simply re-applied (the set is idempotent).
type PromiscDescriptor struct {
	base
	mu sync.Mutex
	on map[uint32]bool
}

// NewPromisc returns the descriptor for owner.
func NewPromisc(c vpp.Client, owner string) *PromiscDescriptor {
	return &PromiscDescriptor{base: base{c, owner}, on: make(map[uint32]bool)}
}

func (*PromiscDescriptor) Name() string { return PromiscName }

func (*PromiscDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(PromiscName, RefID(obj.(*Promisc).GetInterface()))
}

func (*PromiscDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*Promisc).GetInterface())
}

func (d *PromiscDescriptor) set(ctx context.Context, idx uint32, on bool) error {
	if _, err := d.svc().SwInterfaceSetPromisc(ctx, &ifapi.SwInterfaceSetPromisc{SwIfIndex: interface_types.InterfaceIndex(idx), PromiscOn: on}); err != nil {
		return fmt.Errorf("sw_interface_set_promisc: %w", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if on {
		d.on[idx] = true
	} else {
		delete(d.on, idx)
	}
	return nil
}

func (d *PromiscDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Promisc)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	return Meta{idx}, d.set(ctx, idx, true)
}

func (d *PromiscDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return MetaOf(meta)
}

func (d *PromiscDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	return d.set(ctx, m.SwIfIndex, false)
}

func (d *PromiscDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.KeyFor(idx)
		if !ok || !d.on[idx] {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(PromiscName, key.ID()),
			Value: &Promisc{Interface: string(key)},
			Meta:  Meta{idx},
		})
	}
	// forget interfaces that disappeared (index reuse would otherwise mark a new one promisc)
	for idx := range d.on {
		if _, ok := t.byIndex[idx]; !ok {
			delete(d.on, idx)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------- rx-mode

// RxModeDescriptor implements interface.rx-mode (sw_interface_set_rx_mode for all queues).
type RxModeDescriptor struct{ base }

// NewRxMode returns the descriptor for owner.
func NewRxMode(c vpp.Client, owner string) *RxModeDescriptor { return &RxModeDescriptor{base{c, owner}} }

func (*RxModeDescriptor) Name() string { return RxModeName }

func (*RxModeDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(RxModeName, RefID(obj.(*RxMode).GetInterface()))
}

func (*RxModeDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*RxMode).GetInterface())
}

func rxModeToVPP(k RxModeKind) (interface_types.RxMode, error) {
	switch k {
	case RxModeKind_RX_MODE_KIND_POLLING:
		return interface_types.RX_MODE_API_POLLING, nil
	case RxModeKind_RX_MODE_KIND_INTERRUPT:
		return interface_types.RX_MODE_API_INTERRUPT, nil
	case RxModeKind_RX_MODE_KIND_ADAPTIVE:
		return interface_types.RX_MODE_API_ADAPTIVE, nil
	}
	return 0, fmt.Errorf("iface: rx-mode %v is not polling/interrupt/adaptive", k)
}

func rxModeFromVPP(m interface_types.RxMode) RxModeKind {
	switch m {
	case interface_types.RX_MODE_API_INTERRUPT:
		return RxModeKind_RX_MODE_KIND_INTERRUPT
	case interface_types.RX_MODE_API_ADAPTIVE:
		return RxModeKind_RX_MODE_KIND_ADAPTIVE
	}
	return RxModeKind_RX_MODE_KIND_POLLING
}

func (d *RxModeDescriptor) set(ctx context.Context, idx uint32, mode interface_types.RxMode) error {
	if _, err := d.svc().SwInterfaceSetRxMode(ctx, &ifapi.SwInterfaceSetRxMode{SwIfIndex: interface_types.InterfaceIndex(idx), QueueIDValid: false, Mode: mode}); err != nil {
		return fmt.Errorf("sw_interface_set_rx_mode: %w", err)
	}
	return nil
}

func (d *RxModeDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*RxMode)
	if !ok {
		return nil, ErrEmptyValue
	}
	mode, err := rxModeToVPP(o.GetMode())
	if err != nil {
		return nil, err
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	return Meta{idx}, d.set(ctx, idx, mode)
}

func (d *RxModeDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := MetaOf(meta)
	if err != nil {
		return nil, err
	}
	if oldObj.(*RxMode).GetInterface() != newObj.(*RxMode).GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	mode, err := rxModeToVPP(newObj.(*RxMode).GetMode())
	if err != nil {
		return nil, err
	}
	return m, d.set(ctx, m.SwIfIndex, mode)
}

func (d *RxModeDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	return d.set(ctx, m.SwIfIndex, interface_types.RX_MODE_API_POLLING)
}

// placements dumps sw_interface_rx_placement for every interface, grouped by sw_if_index in
// ascending queue order.
func placements(ctx context.Context, client vpp.Client) (map[uint32][]*ifapi.SwInterfaceRxPlacementDetails, error) {
	stream, err := ifapi.NewServiceClient(client).SwInterfaceRxPlacementDump(ctx, &ifapi.SwInterfaceRxPlacementDump{SwIfIndex: interface_types.InterfaceIndex(AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_rx_placement_dump: %w", err)
	}
	out := make(map[uint32][]*ifapi.SwInterfaceRxPlacementDetails)
	for {
		p, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_rx_placement_dump: %w", err)
		}
		out[uint32(p.SwIfIndex)] = append(out[uint32(p.SwIfIndex)], p)
	}
	for _, qs := range out {
		sort.Slice(qs, func(i, j int) bool { return qs[i].QueueID < qs[j].QueueID })
	}
	return out, nil
}

func (d *RxModeDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	qs, err := placements(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.KeyFor(idx)
		if !ok {
			continue
		}
		for _, q := range qs[idx] {
			if k := rxModeFromVPP(q.Mode); k != RxModeKind_RX_MODE_KIND_POLLING {
				out = append(out, scheduler.KV{
					Key:   scheduler.Join(RxModeName, key.ID()),
					Value: &RxMode{Interface: string(key), Mode: k},
					Meta:  Meta{idx},
				})
				break
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------- rx-placement

// RxPlacementDescriptor implements interface.rx-placement (sw_interface_set_rx_placement).
// worker is the 0-based worker number of the set message; the dump reports thread indexes
// (0 = main), so worker = thread - 1. Hosts without workers (cpu { } in startup.conf) reject
// every Create; the integration test skips there.
type RxPlacementDescriptor struct{ base }

// NewRxPlacement returns the descriptor for owner.
func NewRxPlacement(c vpp.Client, owner string) *RxPlacementDescriptor {
	return &RxPlacementDescriptor{base{c, owner}}
}

func (*RxPlacementDescriptor) Name() string { return RxPlacementName }

func (*RxPlacementDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*RxPlacement)
	return scheduler.Join(RxPlacementName, RefID(o.GetInterface()), strconv.FormatUint(uint64(o.GetQueue()), 10))
}

func (*RxPlacementDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*RxPlacement).GetInterface())
}

func (d *RxPlacementDescriptor) set(ctx context.Context, idx uint32, o *RxPlacement, main bool) error {
	req := &ifapi.SwInterfaceSetRxPlacement{SwIfIndex: interface_types.InterfaceIndex(idx), QueueID: o.GetQueue(), WorkerID: o.GetWorker(), IsMain: main}
	if _, err := d.svc().SwInterfaceSetRxPlacement(ctx, req); err != nil {
		return fmt.Errorf("sw_interface_set_rx_placement: %w", err)
	}
	return nil
}

func (d *RxPlacementDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*RxPlacement)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	return Meta{idx}, d.set(ctx, idx, o, false)
}

func (d *RxPlacementDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := MetaOf(meta)
	if err != nil {
		return nil, err
	}
	o, n := oldObj.(*RxPlacement), newObj.(*RxPlacement)
	if o.GetInterface() != n.GetInterface() || o.GetQueue() != n.GetQueue() {
		return nil, scheduler.ErrRecreate
	}
	return m, d.set(ctx, m.SwIfIndex, n, false)
}

func (d *RxPlacementDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	o, ok := obj.(*RxPlacement)
	if !ok {
		return ErrEmptyValue
	}
	return d.set(ctx, m.SwIfIndex, o, true)
}

func (d *RxPlacementDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	qs, err := placements(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.KeyFor(idx)
		if !ok {
			continue
		}
		for _, q := range qs[idx] {
			if q.WorkerID == 0 {
				continue // main thread = default
			}
			out = append(out, scheduler.KV{
				Key:   scheduler.Join(RxPlacementName, key.ID(), strconv.FormatUint(uint64(q.QueueID), 10)),
				Value: &RxPlacement{Interface: string(key), Queue: q.QueueID, Worker: q.WorkerID - 1},
				Meta:  Meta{idx},
			})
		}
	}
	return out, nil
}
