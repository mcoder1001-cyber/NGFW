package iface

// Attribute descriptors decorate an existing interface of any owned type with one setting each.
// They are keyed on the interface id ("interface.mtu/loop201"), depend on the interface's full
// key, update in place and never recreate the interface. Retrieve rules (which owned interfaces
// yield an object) are documented per descriptor and in docs/agent/descriptors/interface.md:
//
//	admin-state  present ⇔ IF_STATUS_API_FLAG_ADMIN_UP          Delete → admin down
//	mtu          present ⇔ MTUs differ from the creation default Delete → creation default ({link_mtu,0,0,0}; sub-if {0,0,0,0})
//	mac-address  present ⇔ this process set it                   Delete → no-op (VPP has no "unset MAC")
//	promisc      present ⇔ this process switched it on          Delete → off  (not readable back from VPP)
//	rx-mode      present ⇔ a queue differs from the class default Delete → class default (polling; af-packet interrupt)
//	rx-placement present ⇔ a queue sits on a worker thread       Delete → main thread

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
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

// claim records holder's claim on idx after a successful apply when idx is an untagged
// (physical / pre-existing) interface, so Retrieve reports the object as ours (names.go).
func (b base) claim(ctx context.Context, idx uint32, holder string) error {
	t, err := Dump(ctx, b.client, b.owner)
	if err != nil {
		return err
	}
	return t.ClaimIfUntagged(idx, holder)
}

// release drops holder's claim on the interface obj names (no-op for tagged interfaces).
func (b base) release(obj proto.Message, holder string) {
	if o, ok := obj.(interface{ GetInterface() string }); ok {
		_ = ReleaseRef(b.owner, o.GetInterface(), holder)
	}
}

// ownedDetails returns idx's dump row when a per-interface object of holder on it is ours; false
// for a vanished interface or stale Meta pointing at someone else's interface (review L2).
func (b base) ownedDetails(ctx context.Context, idx uint32, holder string) (*ifapi.SwInterfaceDetails, bool, error) {
	t, err := Dump(ctx, b.client, b.owner)
	if err != nil {
		return nil, false, err
	}
	det, ok := t.Details(idx)
	if !ok || !t.Owns(idx, holder) {
		return nil, false, nil
	}
	return det, true, nil
}

func dep(ref string) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: scheduler.Key(ref)}}
}

// ---------------------------------------------------------------------------- admin-state

// AdminStateDescriptor implements interface.admin-state (sw_interface_set_flags).
type AdminStateDescriptor struct{ base }

// NewAdminState returns the descriptor for owner.
func NewAdminState(c vpp.Client, owner string) *AdminStateDescriptor {
	return &AdminStateDescriptor{base{c, owner}}
}

// Name implements scheduler.Descriptor.
func (*AdminStateDescriptor) Name() string { return AdminStateName }

// KeyOf implements scheduler.Descriptor.
func (*AdminStateDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(AdminStateName, RefID(obj.(*AdminState).GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
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

// Create implements scheduler.Descriptor.
func (d *AdminStateDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*AdminState)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.setFlags(ctx, idx, true); err != nil {
		return nil, err
	}
	return Meta{idx}, d.claim(ctx, idx, AdminStateName)
}

// Update has nothing to change in place: the object has no mutable field.
func (d *AdminStateDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return MetaOf(meta)
}

// Delete implements scheduler.Descriptor.
func (d *AdminStateDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) (err error) {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			d.release(obj, AdminStateName)
		}
	}()
	return d.setFlags(ctx, m.SwIfIndex, false)
}

// Retrieve implements scheduler.Descriptor.
func (d *AdminStateDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.OwnedRef(idx, AdminStateName)
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
//
// VPP creates a hardware interface with the MTUs {link_mtu, 0, 0, 0} (L3 = the link MTU, the
// per-protocol MTUs inherit L3) and a sub-interface with {0, 0, 0, 0} (VPP then falls back to
// 9000, interface_funcs.h vnet_sw_interface_get_mtu); both verified on the host. That creation
// default is "no object": Retrieve omits it and Delete restores it — never {0,0,0,0} on a
// hardware interface, which would silently turn its L3 MTU into the 9000 fallback. The desired
// L3 MTU (field mtu) must be non-zero (Create/Update return ErrZeroMtu): 0 is not an MTU and on
// a sub-interface such an object would be indistinguishable from the default.
type MtuDescriptor struct{ base }

// ErrZeroMtu is returned for a desired interface.mtu whose L3 MTU is 0.
var ErrZeroMtu = errors.New("iface: interface.mtu needs a non-zero L3 mtu")

// ErrMtuDefault is returned for a desired interface.mtu equal to the interface's creation default
// ({link_mtu,0,0,0}): such an object is invisible to Retrieve and would be re-created on every
// resync (review M4) — remove it from the desired state instead.
var ErrMtuDefault = errors.New("iface: interface.mtu equals the interface's default; remove the object instead")

// defaultMtu is VPP's creation default for d: {link_mtu, 0, 0, 0}, {0, 0, 0, 0} for a sub-interface.
func defaultMtu(d *ifapi.SwInterfaceDetails) [4]uint32 {
	if Kind(d) == SubinterfaceName {
		return [4]uint32{}
	}
	return [4]uint32{uint32(d.LinkMtu), 0, 0, 0}
}

func mtuOf(d *ifapi.SwInterfaceDetails) [4]uint32 {
	var m [4]uint32
	copy(m[:], d.Mtu)
	return m
}

// NewMtu returns the descriptor for owner.
func NewMtu(c vpp.Client, owner string) *MtuDescriptor { return &MtuDescriptor{base{c, owner}} }

// Name implements scheduler.Descriptor.
func (*MtuDescriptor) Name() string { return MtuName }

// KeyOf implements scheduler.Descriptor.
func (*MtuDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(MtuName, RefID(obj.(*Mtu).GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
func (*MtuDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return dep(obj.(*Mtu).GetInterface())
}

func (d *MtuDescriptor) set(ctx context.Context, idx uint32, m [4]uint32) error {
	req := &ifapi.SwInterfaceSetMtu{SwIfIndex: interface_types.InterfaceIndex(idx), Mtu: m[:]}
	if _, err := d.svc().SwInterfaceSetMtu(ctx, req); err != nil {
		return fmt.Errorf("sw_interface_set_mtu: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *MtuDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Mtu)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetMtu() == 0 {
		return nil, ErrZeroMtu
	}
	idx, det, err := d.resolveDetails(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if mtuArr(o) == defaultMtu(det) {
		return nil, ErrMtuDefault
	}
	if err := d.set(ctx, idx, mtuArr(o)); err != nil {
		return nil, err
	}
	return Meta{idx}, d.claim(ctx, idx, MtuName)
}

func mtuArr(o *Mtu) [4]uint32 { return [4]uint32{o.GetMtu(), o.GetIp4(), o.GetIp6(), o.GetMpls()} }

// Update implements scheduler.Descriptor.
func (d *MtuDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := MetaOf(meta)
	if err != nil {
		return nil, err
	}
	if oldObj.(*Mtu).GetInterface() != newObj.(*Mtu).GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	if newObj.(*Mtu).GetMtu() == 0 {
		return nil, ErrZeroMtu
	}
	_, det, err := d.resolveDetails(ctx, newObj.(*Mtu).GetInterface())
	if err != nil {
		return nil, err
	}
	if mtuArr(newObj.(*Mtu)) == defaultMtu(det) {
		return nil, ErrMtuDefault
	}
	return m, d.set(ctx, m.SwIfIndex, mtuArr(newObj.(*Mtu)))
}

// Delete restores VPP's creation default {link_mtu, 0, 0, 0}; a vanished interface is a no-op.
func (d *MtuDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) (err error) {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			d.release(obj, MtuName)
		}
	}()
	det, ok, err := d.ownedDetails(ctx, m.SwIfIndex, MtuName)
	if err != nil || !ok {
		return err
	}
	return d.set(ctx, m.SwIfIndex, defaultMtu(det))
}

// Retrieve implements scheduler.Descriptor.
func (d *MtuDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.OwnedRef(idx, MtuName)
		if !ok {
			continue
		}
		det := t.byIndex[idx]
		mtu := mtuOf(det)
		if mtu == defaultMtu(det) {
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

// MacAddressDescriptor implements interface.mac-address (sw_interface_set_mac_address). Every
// hardware interface has a MAC and VPP keeps no "configured vs. default" distinction, so a
// Retrieve that reported every interface's address would make the scheduler plan a Delete for
// each interface without a desired MAC on every reconcile. Retrieve therefore reports only the
// interfaces this process set a MAC on (with the address VPP reports now, so drift is seen);
// after an agent restart the desired MACs are re-applied once (the set is idempotent), and a
// MAC removed from the desired state while the agent was down simply stays (Delete is a no-op).
type MacAddressDescriptor struct {
	base
	mu  sync.Mutex
	set map[uint32]bool
}

// NewMacAddress returns the descriptor for owner.
func NewMacAddress(c vpp.Client, owner string) *MacAddressDescriptor {
	return &MacAddressDescriptor{base: base{c, owner}, set: make(map[uint32]bool)}
}

// Name implements scheduler.Descriptor.
func (*MacAddressDescriptor) Name() string { return MacAddressName }

// KeyOf implements scheduler.Descriptor.
func (*MacAddressDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(MacAddressName, RefID(obj.(*MacAddress).GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
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

func (d *MacAddressDescriptor) apply(ctx context.Context, idx uint32, mac string) error {
	m, err := ParseMAC(mac)
	if err != nil {
		return err
	}
	if _, err := d.svc().SwInterfaceSetMacAddress(ctx, &ifapi.SwInterfaceSetMacAddress{SwIfIndex: interface_types.InterfaceIndex(idx), MacAddress: m}); err != nil {
		return fmt.Errorf("sw_interface_set_mac_address: %w", err)
	}
	d.mu.Lock()
	d.set[idx] = true
	d.mu.Unlock()
	return nil
}

// Create implements scheduler.Descriptor.
func (d *MacAddressDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*MacAddress)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.apply(ctx, idx, o.GetMac()); err != nil {
		return nil, err
	}
	return Meta{idx}, d.claim(ctx, idx, MacAddressName)
}

// Update implements scheduler.Descriptor.
func (d *MacAddressDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := MetaOf(meta)
	if err != nil {
		return nil, err
	}
	if oldObj.(*MacAddress).GetInterface() != newObj.(*MacAddress).GetInterface() {
		return nil, scheduler.ErrRecreate
	}
	return m, d.apply(ctx, m.SwIfIndex, newObj.(*MacAddress).GetMac())
}

// Delete keeps the current address (VPP has no notion of an unset MAC) and stops reporting it.
// The interface's own descriptor deleting the interface is what removes it.
func (d *MacAddressDescriptor) Delete(_ context.Context, obj proto.Message, meta any) (err error) {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			d.release(obj, MacAddressName)
		}
	}()
	d.mu.Lock()
	delete(d.set, m.SwIfIndex)
	d.mu.Unlock()
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *MacAddressDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.OwnedRef(idx, MacAddressName)
		if !ok || Kind(t.byIndex[idx]) == SubinterfaceName {
			continue // sub-interfaces share the parent's address
		}
		mac := FormatMAC(t.byIndex[idx].L2Address)
		if mac == "" || !d.set[idx] {
			continue
		}
		out = append(out, scheduler.KV{
			Key:   scheduler.Join(MacAddressName, key.ID()),
			Value: &MacAddress{Interface: string(key), Mac: mac},
			Meta:  Meta{idx},
		})
	}
	// forget interfaces that disappeared (index reuse would otherwise report a new one)
	for idx := range d.set {
		if _, ok := t.byIndex[idx]; !ok {
			delete(d.set, idx)
		}
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

// Name implements scheduler.Descriptor.
func (*PromiscDescriptor) Name() string { return PromiscName }

// KeyOf implements scheduler.Descriptor.
func (*PromiscDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(PromiscName, RefID(obj.(*Promisc).GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
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

// Create implements scheduler.Descriptor.
func (d *PromiscDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Promisc)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, true); err != nil {
		return nil, err
	}
	return Meta{idx}, d.claim(ctx, idx, PromiscName)
}

// Update implements scheduler.Descriptor.
func (d *PromiscDescriptor) Update(_ context.Context, _, _ proto.Message, meta any) (any, error) {
	return MetaOf(meta)
}

// Delete implements scheduler.Descriptor.
func (d *PromiscDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) (err error) {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			d.release(obj, PromiscName)
		}
	}()
	return d.set(ctx, m.SwIfIndex, false)
}

// Retrieve implements scheduler.Descriptor.
func (d *PromiscDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	t, err := Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []scheduler.KV
	for _, idx := range t.order {
		key, ok := t.OwnedRef(idx, PromiscName)
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
//
// The object exists only while the queues' mode differs from the mode VPP gives a new interface
// of that device class: interface_main.default_rx_mode (polling unless startup.conf says
// otherwise) for every class except af-packet, which forces interrupt on creation
// (af_packet.c: vnet_hw_if_set_rx_queue_mode(…, VNET_HW_IF_RX_MODE_INTERRUPT); verified on the
// host). Desiring the default is rejected with ErrRxModeDefault (it would vanish from Retrieve
// and be re-created forever); Delete restores the default.
type RxModeDescriptor struct{ base }

// ErrRxModeDefault is returned for a desired rx-mode equal to the device class default.
var ErrRxModeDefault = errors.New("iface: rx-mode equals the interface's default; remove the object instead")

// defaultRxMode is the rx mode VPP gives a new interface of d's device class.
func defaultRxMode(d *ifapi.SwInterfaceDetails) RxModeKind {
	if strings.TrimRight(d.InterfaceDevType, "\x00") == "af-packet" {
		return RxModeKind_RX_MODE_KIND_INTERRUPT
	}
	return RxModeKind_RX_MODE_KIND_POLLING
}

// resolveDetails is resolve plus the interface's dump row.
func (b base) resolveDetails(ctx context.Context, ref string) (uint32, *ifapi.SwInterfaceDetails, error) {
	t, err := Dump(ctx, b.client, b.owner)
	if err != nil {
		return 0, nil, err
	}
	idx, err := t.Index(ref)
	if err != nil {
		return 0, nil, err
	}
	det, _ := t.Details(idx)
	return idx, det, nil
}

// NewRxMode returns the descriptor for owner.
func NewRxMode(c vpp.Client, owner string) *RxModeDescriptor {
	return &RxModeDescriptor{base{c, owner}}
}

// Name implements scheduler.Descriptor.
func (*RxModeDescriptor) Name() string { return RxModeName }

// KeyOf implements scheduler.Descriptor.
func (*RxModeDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(RxModeName, RefID(obj.(*RxMode).GetInterface()))
}

// Dependencies implements scheduler.Descriptor.
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

// Create implements scheduler.Descriptor.
func (d *RxModeDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*RxMode)
	if !ok {
		return nil, ErrEmptyValue
	}
	mode, err := rxModeToVPP(o.GetMode())
	if err != nil {
		return nil, err
	}
	idx, det, err := d.resolveDetails(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if o.GetMode() == defaultRxMode(det) {
		return nil, ErrRxModeDefault
	}
	if err := d.set(ctx, idx, mode); err != nil {
		return nil, err
	}
	return Meta{idx}, d.claim(ctx, idx, RxModeName)
}

// Update implements scheduler.Descriptor.
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
	_, det, err := d.resolveDetails(ctx, newObj.(*RxMode).GetInterface())
	if err != nil {
		return nil, err
	}
	if newObj.(*RxMode).GetMode() == defaultRxMode(det) {
		return nil, ErrRxModeDefault
	}
	return m, d.set(ctx, m.SwIfIndex, mode)
}

// Delete restores the device class default; a vanished interface is a no-op.
func (d *RxModeDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) (err error) {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			d.release(obj, RxModeName)
		}
	}()
	det, ok, err := d.ownedDetails(ctx, m.SwIfIndex, RxModeName)
	if err != nil || !ok {
		return err
	}
	mode, _ := rxModeToVPP(defaultRxMode(det))
	return d.set(ctx, m.SwIfIndex, mode)
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

// Retrieve implements scheduler.Descriptor.
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
		key, ok := t.OwnedRef(idx, RxModeName)
		if !ok {
			continue
		}
		def := defaultRxMode(t.byIndex[idx])
		for _, q := range qs[idx] {
			if k := rxModeFromVPP(q.Mode); k != def {
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

// Name implements scheduler.Descriptor.
func (*RxPlacementDescriptor) Name() string { return RxPlacementName }

// KeyOf implements scheduler.Descriptor.
func (*RxPlacementDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*RxPlacement)
	return scheduler.Join(RxPlacementName, RefID(o.GetInterface()), strconv.FormatUint(uint64(o.GetQueue()), 10))
}

// Dependencies implements scheduler.Descriptor.
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

// Create implements scheduler.Descriptor.
func (d *RxPlacementDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*RxPlacement)
	if !ok {
		return nil, ErrEmptyValue
	}
	idx, err := d.resolve(ctx, o.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, o, false); err != nil {
		return nil, err
	}
	return Meta{idx}, d.claim(ctx, idx, RxPlacementName)
}

// Update implements scheduler.Descriptor.
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

// Delete implements scheduler.Descriptor.
func (d *RxPlacementDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) (err error) {
	m, err := MetaOf(meta)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			d.release(obj, RxPlacementName)
		}
	}()
	o, ok := obj.(*RxPlacement)
	if !ok {
		return ErrEmptyValue
	}
	return d.set(ctx, m.SwIfIndex, o, true)
}

// Retrieve implements scheduler.Descriptor.
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
		key, ok := t.OwnedRef(idx, RxPlacementName)
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

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*AdminStateDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "interface")
}

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*MtuDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "interface")
}

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*MacAddressDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "interface")
}

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*PromiscDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "interface")
}

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*RxModeDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "interface")
}

// Normalize implements scheduler.Normalizer: the interface reference in canonical alias form.
func (*RxPlacementDescriptor) Normalize(obj proto.Message) proto.Message {
	return NormalizeRefs(obj, "interface")
}
