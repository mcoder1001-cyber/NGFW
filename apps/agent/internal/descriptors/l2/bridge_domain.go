package l2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	BridgeDomainName   = "l2.bridge-domain"
	MemberName         = "l2.bridge-domain-member"
	XconnectName       = "l2.xconnect"
	FibEntryName       = "l2.fib-entry"
	FlagsName          = "l2.flags"
	VlanTagRewriteName = "l2.vlan-tag-rewrite"
)

// ErrEmptyValue is returned for a nil or foreign desired value.
var ErrEmptyValue = errors.New("l2: nil or wrong desired value type")

type base struct {
	client vpp.Client
	owner  string
}

func (b base) svc() l2api.RPCService { return l2api.NewServiceClient(b.client) }

// bdID formats a bridge-domain id as the key segment / tag id.
func bdID(id uint32) string { return strconv.FormatUint(uint64(id), 10) }

// BridgeDomainKey is the key of the bridge domain id.
func BridgeDomainKey(id uint32) scheduler.Key { return scheduler.Join(BridgeDomainName, bdID(id)) }

// bridgeDomains dumps every bridge domain and returns the ones owned by this agent (bd_tag =
// "<owner>:<id>" or "<owner>:<id>/<name>", ParseBDTag), in ascending id order.
func (b base) bridgeDomains(ctx context.Context) ([]*l2api.BridgeDomainDetails, error) {
	return OwnedBridgeDomains(ctx, b.client, b.owner)
}

// OwnedBridgeDomains dumps every bridge domain and returns the ones owned by owner (bd_tag
// "<owner>:<id>[/<name>]" with <id> = the bridge-domain id), in ascending id order. The live-state
// RPCs (F-bridge-l2) use it with the descriptors' ownership rule.
func OwnedBridgeDomains(ctx context.Context, c vpp.Client, owner string) ([]*l2api.BridgeDomainDetails, error) {
	stream, err := l2api.NewServiceClient(c).BridgeDomainDump(ctx, &l2api.BridgeDomainDump{BdID: ^uint32(0), SwIfIndex: interface_types.InterfaceIndex(iface.AllInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("bridge_domain_dump: %w", err)
	}
	var out []*l2api.BridgeDomainDetails
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bridge_domain_dump: %w", err)
		}
		if _, ok := ParseBDTag(d.BdTag, owner, d.BdID); ok {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BdID < out[j].BdID })
	return out, nil
}

// ParseBDTag reports whether tag is owner's tag of bridge domain id and returns the record name it
// carries ("" for the plain "<owner>:<id>" form).
func ParseBDTag(tag, owner string, id uint32) (name string, ok bool) {
	rest, ok := vpp.ParseOwnerTag(tag, owner)
	if !ok {
		return "", false
	}
	idPart, name, _ := strings.Cut(rest, "/")
	if idPart != bdID(id) {
		return "", false
	}
	return name, true
}

// bdTag is the owner tag of o: "<owner>:<id>" or "<owner>:<id>/<name>".
func bdTag(owner string, o *BridgeDomain) (string, error) {
	id := bdID(o.GetId())
	if n := o.GetName(); n != "" {
		if strings.ContainsAny(n, "/\x00\r\n") {
			return "", fmt.Errorf("l2: bridge-domain name %q contains a reserved character", n)
		}
		id += "/" + n
	}
	return vpp.OwnerTag(owner, id)
}

// flagsOf converts the model booleans into the bd_flags bitmap.
func flagsOf(o *BridgeDomain) l2api.BdFlags {
	var f l2api.BdFlags
	if o.GetLearn() {
		f |= l2api.BRIDGE_API_FLAG_LEARN
	}
	if o.GetForward() {
		f |= l2api.BRIDGE_API_FLAG_FWD
	}
	if o.GetFlood() {
		f |= l2api.BRIDGE_API_FLAG_FLOOD
	}
	if o.GetUuFlood() {
		f |= l2api.BRIDGE_API_FLAG_UU_FLOOD
	}
	if o.GetArpTerm() {
		f |= l2api.BRIDGE_API_FLAG_ARP_TERM
	}
	if o.GetArpUfwd() {
		f |= l2api.BRIDGE_API_FLAG_ARP_UFWD
	}
	return f
}

func decodeBD(d *l2api.BridgeDomainDetails, owner string) *BridgeDomain {
	name, _ := ParseBDTag(d.BdTag, owner, d.BdID)
	return &BridgeDomain{Id: d.BdID, Flood: d.Flood, UuFlood: d.UuFlood, Forward: d.Forward, Learn: d.Learn, ArpTerm: d.ArpTerm, ArpUfwd: d.ArpUfwd, MacAge: uint32(d.MacAge), Name: name}
}

// BridgeDomainDescriptor implements l2.bridge-domain (bridge_domain_add_del_v2, bridge_flags,
// bridge_domain_set_mac_age). Ownership is the bd_tag; ids come from the slot's table range in
// tests and from the API's allocator in production.
type BridgeDomainDescriptor struct{ base }

// NewBridgeDomain returns the descriptor for owner.
func NewBridgeDomain(c vpp.Client, owner string) *BridgeDomainDescriptor {
	return &BridgeDomainDescriptor{base{c, owner}}
}

// BDMeta is the runtime handle: the bridge-domain id (there is no separate index in the API).
type BDMeta struct{ ID uint32 }

// Name implements scheduler.Descriptor.
func (*BridgeDomainDescriptor) Name() string { return BridgeDomainName }

// KeyOf implements scheduler.Descriptor.
func (*BridgeDomainDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return BridgeDomainKey(obj.(*BridgeDomain).GetId())
}

// Dependencies implements scheduler.Descriptor.
func (*BridgeDomainDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor.
func (d *BridgeDomainDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*BridgeDomain)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetId() == 0 || o.GetId() == ^uint32(0) {
		return nil, fmt.Errorf("l2: bridge-domain id %d is reserved", o.GetId())
	}
	if o.GetMacAge() > 255 {
		return nil, fmt.Errorf("l2: mac_age %d out of range (0-255 minutes)", o.GetMacAge())
	}
	tag, err := bdTag(d.owner, o)
	if err != nil {
		return nil, err
	}
	_, err = d.svc().BridgeDomainAddDelV2(ctx, &l2api.BridgeDomainAddDelV2{
		BdID: o.GetId(), Flood: o.GetFlood(), UuFlood: o.GetUuFlood(), Forward: o.GetForward(), Learn: o.GetLearn(),
		ArpTerm: o.GetArpTerm(), ArpUfwd: o.GetArpUfwd(), MacAge: uint8(o.GetMacAge()), BdTag: tag, IsAdd: true, //nolint:gosec // range-checked
	})
	if err != nil {
		return nil, fmt.Errorf("bridge_domain_add_del_v2: %w", err)
	}
	return BDMeta{o.GetId()}, nil
}

// Update implements scheduler.Descriptor.
func (d *BridgeDomainDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, ok := meta.(BDMeta)
	if !ok {
		return nil, fmt.Errorf("l2: unexpected meta %T", meta)
	}
	o, n := oldObj.(*BridgeDomain), newObj.(*BridgeDomain)
	if o.GetId() != n.GetId() || o.GetName() != n.GetName() {
		return nil, scheduler.ErrRecreate // the name lives in the tag, which VPP cannot change in place
	}
	oldF, newF := flagsOf(o), flagsOf(n)
	if set := newF &^ oldF; set != 0 {
		if _, err := d.svc().BridgeFlags(ctx, &l2api.BridgeFlags{BdID: m.ID, IsSet: true, Flags: set}); err != nil {
			return nil, fmt.Errorf("bridge_flags set: %w", err)
		}
	}
	if clr := oldF &^ newF; clr != 0 {
		if _, err := d.svc().BridgeFlags(ctx, &l2api.BridgeFlags{BdID: m.ID, IsSet: false, Flags: clr}); err != nil {
			return nil, fmt.Errorf("bridge_flags clear: %w", err)
		}
	}
	if o.GetMacAge() != n.GetMacAge() {
		if n.GetMacAge() > 255 {
			return nil, fmt.Errorf("l2: mac_age %d out of range", n.GetMacAge())
		}
		if _, err := d.svc().BridgeDomainSetMacAge(ctx, &l2api.BridgeDomainSetMacAge{BdID: m.ID, MacAge: uint8(n.GetMacAge())}); err != nil { //nolint:gosec // range-checked
			return nil, fmt.Errorf("bridge_domain_set_mac_age: %w", err)
		}
	}
	return m, nil
}

// Delete implements scheduler.Descriptor.
func (d *BridgeDomainDescriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(BDMeta)
	if !ok {
		return fmt.Errorf("l2: unexpected meta %T", meta)
	}
	if _, err := d.svc().BridgeDomainAddDelV2(ctx, &l2api.BridgeDomainAddDelV2{BdID: m.ID, IsAdd: false}); err != nil {
		return fmt.Errorf("bridge_domain_add_del_v2 (del): %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *BridgeDomainDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	bds, err := d.bridgeDomains(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.KV, 0, len(bds))
	for _, bd := range bds {
		out = append(out, scheduler.KV{Key: BridgeDomainKey(bd.BdID), Value: decodeBD(bd, d.owner), Meta: BDMeta{bd.BdID}})
	}
	return out, nil
}
