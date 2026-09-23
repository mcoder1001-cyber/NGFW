package l2

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l2api "ngfw/agent/binapi/l2"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// FibEntryDescriptor implements l2.fib-entry (l2fib_add_del). Only static, filter and BVI
// entries are configuration; learned entries are runtime state and are never retrieved. VPP
// stores filter entries with the static flag set (l2fib_add_filter_entry), so the model keeps
// `static` false for filter entries and Retrieve masks the implied flag.
type FibEntryDescriptor struct{ base }

// NewFibEntry returns the descriptor for owner.
func NewFibEntry(c vpp.Client, owner string) *FibEntryDescriptor { return &FibEntryDescriptor{base{c, owner}} }

func (*FibEntryDescriptor) Name() string { return FibEntryName }

// FibEntryKey is "l2.fib-entry/<bd>/<mac>" with the MAC in canonical lower-case form.
func FibEntryKey(bd uint32, mac string) scheduler.Key {
	if m, err := iface.ParseMAC(mac); err == nil {
		mac = iface.FormatMAC(m)
	}
	return scheduler.Join(FibEntryName, bdID(bd), mac)
}

func (*FibEntryDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	o := obj.(*FibEntry)
	return FibEntryKey(o.GetBridgeDomain(), o.GetMac())
}

func (*FibEntryDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	o := obj.(*FibEntry)
	deps := []scheduler.Dependency{{Key: BridgeDomainKey(o.GetBridgeDomain())}}
	if o.GetInterface() != "" {
		deps = append(deps, scheduler.Dependency{Key: scheduler.Key(o.GetInterface())})
	}
	return deps
}

func (d *FibEntryDescriptor) addDel(ctx context.Context, o *FibEntry, idx uint32, add bool) error {
	mac, err := iface.ParseMAC(o.GetMac())
	if err != nil {
		return err
	}
	_, err = d.svc().L2fibAddDel(ctx, &l2api.L2fibAddDel{
		Mac: mac, BdID: o.GetBridgeDomain(), SwIfIndex: interface_types.InterfaceIndex(idx), IsAdd: add,
		StaticMac: o.GetStatic(), FilterMac: o.GetFilter(), BviMac: o.GetBvi(),
	})
	if err != nil {
		return fmt.Errorf("l2fib_add_del: %w", err)
	}
	return nil
}

func (d *FibEntryDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*FibEntry)
	if !ok {
		return nil, ErrEmptyValue
	}
	if !o.GetStatic() && !o.GetFilter() && !o.GetBvi() {
		return nil, errors.New("l2: fib-entry must be static, filter or bvi (learned entries are not configuration)")
	}
	if o.GetFilter() && (o.GetStatic() || o.GetBvi() || o.GetInterface() != "") {
		return nil, errors.New("l2: a filter entry is implicitly static and has no interface or bvi flag")
	}
	idx := iface.AllInterfaces
	if o.GetInterface() != "" {
		var err error
		if idx, err = iface.Resolve(ctx, d.client, d.owner, o.GetInterface()); err != nil {
			return nil, err
		}
	} else if !o.GetFilter() {
		return nil, errors.New("l2: fib-entry needs an interface unless it is a filter entry")
	}
	return iface.Meta{SwIfIndex: idx}, d.addDel(ctx, o, idx, true)
}

func (*FibEntryDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

func (d *FibEntryDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, err := iface.MetaOf(meta)
	if err != nil {
		return err
	}
	o, ok := obj.(*FibEntry)
	if !ok {
		return ErrEmptyValue
	}
	return d.addDel(ctx, o, m.SwIfIndex, false)
}

func (d *FibEntryDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	bds, err := d.bridgeDomains(ctx)
	if err != nil {
		return nil, err
	}
	owned := make(map[uint32]bool, len(bds))
	bvi := make(map[uint32]uint32, len(bds)) // bd id → BVI sw_if_index (~0 = none)
	for _, bd := range bds {
		owned[bd.BdID] = true
		bvi[bd.BdID] = uint32(bd.BviSwIfIndex)
	}
	t, err := iface.Dump(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	stream, err := d.svc().L2FibTableDump(ctx, &l2api.L2FibTableDump{BdID: ^uint32(0)})
	if err != nil {
		return nil, fmt.Errorf("l2_fib_table_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		e, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("l2_fib_table_dump: %w", err)
		}
		if !owned[e.BdID] || !(e.StaticMac || e.FilterMac || e.BviMac) {
			continue
		}
		if autoBviEntry(t, e, bvi[e.BdID]) {
			continue
		}
		v := &FibEntry{BridgeDomain: e.BdID, Mac: iface.FormatMAC(e.Mac), Static: e.StaticMac && !e.FilterMac, Filter: e.FilterMac, Bvi: e.BviMac}
		idx := uint32(e.SwIfIndex)
		if idx != iface.AllInterfaces && !e.FilterMac {
			key, ok := t.KeyFor(idx)
			if !ok {
				continue
			}
			v.Interface = string(key)
		}
		out = append(out, scheduler.KV{Key: FibEntryKey(e.BdID, v.Mac), Value: v, Meta: iface.Meta{SwIfIndex: idx}})
	}
	return out, nil
}

// autoBviEntry reports the static BVI entry VPP installs by itself when an interface joins a
// bridge domain as BVI (l2_input.c: l2fib_add_entry(hi->hw_address, …, BVI|STATIC)) and removes
// when it leaves. It belongs to the l2.bridge-domain-member object, not to l2.fib-entry, so
// Retrieve skips it (else every BVI would produce a Delete in every plan). A desired bvi entry
// for the BVI's own address is therefore redundant and must not be configured.
func autoBviEntry(t *iface.Table, e *l2api.L2FibTableDetails, bviIdx uint32) bool {
	if !e.BviMac || uint32(e.SwIfIndex) != bviIdx {
		return false
	}
	d, ok := t.Details(bviIdx)
	return ok && [6]uint8(d.L2Address) == [6]uint8(e.Mac)
}
