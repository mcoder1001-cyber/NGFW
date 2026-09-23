package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// VRFDescriptor manages one VRF = an IPv4 and an IPv6 FIB table with the same id, named
// "<owner>:<vrf>" (ip_table_add_del). VPP holds a single API lock per table (adding twice needs
// one delete) and ignores add/delete of table 0, so table 0 ("default") is never an object here.
type VRFDescriptor struct{ Env }

var (
	_ scheduler.Descriptor = (*VRFDescriptor)(nil)
	_ scheduler.Reapplier  = (*VRFDescriptor)(nil)
)

// Name implements scheduler.Descriptor.
func (*VRFDescriptor) Name() string { return VRFName }

func asTable(obj proto.Message) *Table {
	v, _ := obj.(*Table)
	if v == nil {
		return &Table{}
	}
	return v
}

// KeyOf implements scheduler.Descriptor.
func (*VRFDescriptor) KeyOf(obj proto.Message) scheduler.Key { return VRFKey(asTable(obj).GetId()) }

// Dependencies implements scheduler.Descriptor.
func (*VRFDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// TableName is the VPP table name of an owned VRF: "<owner>:<vrf>".
func TableName(owner, vrf string) (string, error) { return vpp.OwnerTag(owner, vrf) }

func (d *VRFDescriptor) addDel(ctx context.Context, v *Table, add bool, families ...bool) error {
	if v.GetId() == 0 {
		return fmt.Errorf("%w: table 0 is VPP's default table and never managed", ErrBadValue)
	}
	name, err := TableName(d.Owner, v.GetVrf())
	if err != nil {
		return err
	}
	if len(families) == 0 {
		families = []bool{false, true}
	}
	svc := ip.NewServiceClient(d.Client)
	for _, v6 := range families {
		if _, err := svc.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: add, Table: ip.IPTable{TableID: v.GetId(), IsIP6: v6, Name: name}}); err != nil {
			return fmt.Errorf("ip_table_add_del %d ipv6=%v add=%v: %w", v.GetId(), v6, add, err)
		}
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *VRFDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*Table)
	if !ok {
		return nil, fmt.Errorf("%w %T", ErrBadValue, obj)
	}
	if err := d.addDel(ctx, v, true); err != nil {
		// do not leave half a VRF behind
		_ = d.addDel(context.WithoutCancel(ctx), v, false)
		return nil, err
	}
	return nil, nil
}

// Update implements scheduler.Descriptor. A renamed VRF needs new VPP tables (VPP does not rename
// an existing table); a VRF missing one address family gets it re-added in place.
func (d *VRFDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, n := asTable(oldObj), asTable(newObj)
	if o.GetVrf() != n.GetVrf() || o.GetId() != n.GetId() {
		return nil, scheduler.ErrRecreate
	}
	var fams []bool
	if o.GetMissingIp4() && !n.GetMissingIp4() {
		fams = append(fams, false)
	}
	if o.GetMissingIp6() && !n.GetMissingIp6() {
		fams = append(fams, true)
	}
	if len(fams) == 0 {
		return meta, nil
	}
	return meta, d.addDel(ctx, n, true, fams...)
}

// Reapply implements scheduler.Reapplier: VPP keeps a table alive while routes or interfaces
// reference it even after its API lock is gone (seen in the P05 loss simulation), so Retrieve
// cannot tell a lost lock from a healthy table. ip_table_add_del(add) is idempotent (VPP holds a
// single API lock per table), so a resync re-asserts it.
func (d *VRFDescriptor) Reapply(ctx context.Context, obj proto.Message, _ any) error {
	return d.addDel(ctx, asTable(obj), true)
}

// Delete implements scheduler.Descriptor.
func (d *VRFDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	return d.addDel(ctx, asTable(obj), false)
}

// Retrieve implements scheduler.Descriptor: tables named "<owner>:<vrf>", both families merged.
// A VRF with only one family present is reported with missing_ip4/missing_ip6 set so the diff
// repairs it.
func (d *VRFDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tables, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, v := range tables {
		out = append(out, scheduler.KV{Key: VRFKey(v.GetId()), Value: v})
	}
	return out, nil
}

func (d *VRFDescriptor) dump(ctx context.Context) ([]*Table, error) {
	stream, err := ip.NewServiceClient(d.Client).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		return nil, fmt.Errorf("ip_table_dump: %w", err)
	}
	type fam struct {
		vrf    string
		v4, v6 bool
	}
	byID := map[uint32]*fam{}
	for {
		t, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ip_table_dump: %w", err)
		}
		vrf, ok := vpp.ParseOwnerTag(trimNul(t.Table.Name), d.Owner)
		if !ok || t.Table.TableID == 0 || strings.ContainsAny(vrf, "\x00") {
			continue
		}
		f := byID[t.Table.TableID]
		if f == nil {
			f = &fam{vrf: vrf}
			byID[t.Table.TableID] = f
		}
		if t.Table.IsIP6 {
			f.v6 = true
		} else {
			f.v4 = true
		}
	}
	ids := make([]uint32, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*Table, 0, len(ids))
	for _, id := range ids {
		f := byID[id]
		out = append(out, &Table{Id: id, Vrf: f.vrf, MissingIp4: !f.v4, MissingIp6: !f.v6})
	}
	return out, nil
}
