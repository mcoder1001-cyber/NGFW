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

// ErrTableConflict means the table id exists in VPP under a name that is not this owner's (D-071:
// foreign → never touched).
var ErrTableConflict = errors.New("core: VRF table id is in use by another owner or by VPP")

// families returns the VPP name of table id per family (false = IPv4, true = IPv6) for the
// families that exist.
func (d *VRFDescriptor) families(ctx context.Context, id uint32) (map[bool]string, error) {
	stream, err := ip.NewServiceClient(d.Client).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		return nil, fmt.Errorf("ip_table_dump: %w", err)
	}
	out := map[bool]string{}
	for {
		t, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ip_table_dump: %w", err)
		}
		if t.Table.TableID == id {
			out[t.Table.IsIP6] = trimNul(t.Table.Name)
		}
	}
}

func (d *VRFDescriptor) send(ctx context.Context, id uint32, name string, v6, add bool) error {
	if _, err := ip.NewServiceClient(d.Client).IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: add, Table: ip.IPTable{TableID: id, IsIP6: v6, Name: name}}); err != nil {
		return fmt.Errorf("ip_table_add_del %d ipv6=%v add=%v: %w", id, v6, add, err)
	}
	return nil
}

// ensure adds both families of v under the claim rule (D-071): a family that exists under our
// name is (re-)asserted — ip_table_add_del(add) is idempotent and restores a lost API lock — a
// missing one is created, a family under any other name fails the call before any message is
// sent. It returns the families this call created.
func (d *VRFDescriptor) ensure(ctx context.Context, v *Table) (created []bool, err error) {
	if v.GetId() == 0 {
		return nil, fmt.Errorf("%w: table 0 is VPP's default table and never managed", ErrBadValue)
	}
	name, err := TableName(d.Owner, v.GetVrf())
	if err != nil {
		return nil, err
	}
	have, err := d.families(ctx, v.GetId())
	if err != nil {
		return nil, err
	}
	for _, v6 := range []bool{false, true} {
		if n, ok := have[v6]; ok && n != name {
			return nil, fmt.Errorf("%w: table %d ipv6=%v is named %q, we are %q", ErrTableConflict, v.GetId(), v6, n, name)
		}
	}
	for _, v6 := range []bool{false, true} {
		if err := d.send(ctx, v.GetId(), name, v6, true); err != nil {
			return created, err
		}
		if _, existed := have[v6]; !existed {
			created = append(created, v6)
		}
	}
	return created, nil
}

// remove deletes the families of v that still carry our name, re-verified right before the
// delete (D-071); foreign or missing families are left alone.
func (d *VRFDescriptor) remove(ctx context.Context, v *Table, only ...bool) error {
	name, err := TableName(d.Owner, v.GetVrf())
	if err != nil {
		return err
	}
	have, err := d.families(ctx, v.GetId())
	if err != nil {
		return err
	}
	fams := only
	if len(fams) == 0 {
		fams = []bool{false, true}
	}
	for _, v6 := range fams {
		if n, ok := have[v6]; !ok || n != name {
			continue
		}
		if err := d.send(ctx, v.GetId(), name, v6, false); err != nil {
			return err
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
	created, err := d.ensure(ctx, v)
	if err != nil {
		if len(created) > 0 {
			// do not leave half a VRF behind — only what this call created
			_ = d.remove(context.WithoutCancel(ctx), v, created...)
		}
		return nil, err
	}
	return nil, nil
}

// Update implements scheduler.Descriptor. A renamed VRF needs new VPP tables (VPP does not rename
// an existing table); a VRF missing one address family is repaired in place (ensure re-asserts
// both families, which also restores a lost API lock on the surviving one).
func (d *VRFDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, n := asTable(oldObj), asTable(newObj)
	if o.GetVrf() != n.GetVrf() || o.GetId() != n.GetId() {
		return nil, scheduler.ErrRecreate
	}
	_, err := d.ensure(ctx, n)
	return meta, err
}

// Reapply implements scheduler.Reapplier: VPP keeps a table alive while routes or interfaces
// reference it even after its API lock is gone (seen in the P05 loss simulation), so Retrieve
// cannot tell a lost lock from a healthy table; a resync re-asserts it (idempotent).
func (d *VRFDescriptor) Reapply(ctx context.Context, obj proto.Message, _ any) error {
	_, err := d.ensure(ctx, asTable(obj))
	return err
}

// Delete implements scheduler.Descriptor: only families still named "<owner>:<vrf>".
func (d *VRFDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	return d.remove(ctx, asTable(obj))
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
