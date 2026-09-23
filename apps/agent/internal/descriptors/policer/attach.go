package policer

import (
	"context"
	"fmt"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/policer"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// AttachMeta is the runtime handle of the write-only attachment objects: the interface index.
type AttachMeta struct{ SwIfIndex uint32 }

// ---- policer.interface ------------------------------------------------------------------------

// KeyInterface is "policer.interface/<interface>/<input|output>".
func KeyInterface(ifName, dir string) scheduler.Key { return scheduler.Join(NameInterface, ifName, dir) }

// InterfaceDescriptor manages policer.interface objects with policer_input / policer_output,
// the name-based messages: they need no pool index, which VPP's policer dump does not report
// (the _v2 variants take the index, and their 26.06 handlers reply with the v1 message id —
// docs/agent/descriptors/policer.md). Write-only (D-063): VPP has no dump of interface policers.
type InterfaceDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// NewInterface returns the policer.interface descriptor.
func NewInterface(c vpp.Client, owner string, opts ...df7.Option) *InterfaceDescriptor {
	return &InterfaceDescriptor{df7.NewBase(NameInterface, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	a, _ := df7.Decode[Attachment](obj)
	return KeyInterface(a.Interface, a.Direction)
}

// Dependencies implements scheduler.Descriptor: the policer and the interface.
func (d *InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	a, _ := df7.Decode[Attachment](obj)
	return []scheduler.Dependency{{Key: KeyPolicer(a.Policer)}, d.Opts.IfaceDep(a.Interface)}
}

func (d *InterfaceDescriptor) apply(ctx context.Context, swIfIndex uint32, a Attachment, apply bool) error {
	name, err := vppName(d.Owner, a.Policer)
	if err != nil {
		return err
	}
	svc := policer.NewServiceClient(d.Client)
	idx := interface_types.InterfaceIndex(swIfIndex)
	if a.Direction == DirOutput {
		_, err = svc.PolicerOutput(ctx, &policer.PolicerOutput{Name: name, SwIfIndex: idx, Apply: apply})
		return d.Wrap(fmt.Sprintf("policer_output %s %s apply=%v", name, a.Interface, apply), err)
	}
	_, err = svc.PolicerInput(ctx, &policer.PolicerInput{Name: name, SwIfIndex: idx, Apply: apply})
	return d.Wrap(fmt.Sprintf("policer_input %s %s apply=%v", name, a.Interface, apply), err)
}

// Create implements scheduler.Descriptor: apply once per VPP lifetime (D-076). VPP's apply is
// not idempotent — every policer_input(apply=1) enables the policer-input feature again, which
// stacks a second instance of the node — so a re-application while the boot identity is
// unchanged is skipped (df7.ApplyOnce).
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	a, err := df7.DecodeValid[Attachment](obj)
	if err != nil {
		return nil, err
	}
	key := string(KeyInterface(a.Interface, a.Direction))
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.Attach(a.Interface, key)
	if err != nil {
		return nil, err
	}
	if _, err := d.ApplyOnce(ctx, key, func() error { return d.apply(ctx, idx, a, true) }); err != nil {
		return nil, err
	}
	return AttachMeta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: another policer on the same interface/direction —
// un-apply the old one, apply the new one (applying on top would stack the feature).
func (d *InterfaceDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	oldA, err := df7.Decode[Attachment](oldObj)
	if err != nil {
		return nil, err
	}
	newA, err := df7.DecodeValid[Attachment](newObj)
	if err != nil {
		return nil, err
	}
	if oldA.Interface != newA.Interface || oldA.Direction != newA.Direction {
		return nil, scheduler.ErrRecreate
	}
	if err := d.Delete(ctx, oldObj, nil); err != nil {
		return nil, err
	}
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: re-resolve the interface (indexes are reused after a
// VPP restart, D-071) and un-apply — only when the attachment was applied in this VPP lifetime:
// after a VPP restart there is nothing to remove, and VPP 26.06's policer_input(apply=0) writes
// the per-interface slot without growing the vector first (policer_op.c), i.e. out of bounds on
// an interface that never had a policer since VPP started.
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	a, err := df7.Decode[Attachment](obj)
	if err != nil {
		return err
	}
	key := string(KeyInterface(a.Interface, a.Direction))
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return err
	}
	idx, found, err := ifs.Reresolve(a.Interface, key)
	if err != nil {
		return err
	}
	applied, err := d.AppliedNow(ctx, key)
	if err != nil {
		return err
	}
	if found && applied {
		if err := d.apply(ctx, idx, a, false); err != nil {
			return err
		}
	}
	if err := d.ForgetApplied(key); err != nil {
		return err
	}
	return df7.Release(d.Owner, a.Interface, key)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *InterfaceDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameInterface, "VPP 26.06 has no dump of interface policers")
}

// ---- policer.bind -----------------------------------------------------------------------------

// KeyBind is "policer.bind/<policer>".
func KeyBind(policerName string) scheduler.Key { return scheduler.Join(NameBind, policerName) }

// BindDescriptor manages policer.bind objects with policer_bind (by name). Write-only (D-063):
// policer_details does not report the bound thread. Idempotent (it sets the policer's thread
// index). The host has no worker threads, so a bind fails there with INVALID_WORKER (the
// integration test records the skip reason).
type BindDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*BindDescriptor)(nil)

// NewBind returns the policer.bind descriptor.
func NewBind(c vpp.Client, owner string, opts ...df7.Option) *BindDescriptor {
	return &BindDescriptor{df7.NewBase(NameBind, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *BindDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	b, _ := df7.Decode[Bind](obj)
	return KeyBind(b.Policer)
}

// Dependencies implements scheduler.Descriptor: the policer.
func (d *BindDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	b, _ := df7.Decode[Bind](obj)
	return []scheduler.Dependency{{Key: KeyPolicer(b.Policer)}}
}

func (d *BindDescriptor) bind(ctx context.Context, b Bind, enable bool) error {
	name, err := vppName(d.Owner, b.Policer)
	if err != nil {
		return err
	}
	_, err = policer.NewServiceClient(d.Client).PolicerBind(ctx, &policer.PolicerBind{Name: name, WorkerIndex: b.Worker, BindEnable: enable})
	return d.Wrap(fmt.Sprintf("policer_bind %s worker %d enable=%v", name, b.Worker, enable), err)
}

// Create implements scheduler.Descriptor (idempotent).
func (d *BindDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	b, err := df7.DecodeValid[Bind](obj)
	if err != nil {
		return nil, err
	}
	return nil, d.bind(ctx, b, true)
}

// Update implements scheduler.Descriptor: rebinding to another worker is in place.
func (d *BindDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	b, err := df7.DecodeValid[Bind](newObj)
	if err != nil {
		return nil, err
	}
	return nil, d.bind(ctx, b, true)
}

// Delete implements scheduler.Descriptor: unbind (any thread may handle the policer again).
// The policer is addressed by its owner-tagged name, never by index.
func (d *BindDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	b, err := df7.Decode[Bind](obj)
	if err != nil {
		return err
	}
	return d.bind(ctx, b, false)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *BindDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameBind, "policer_details does not report the bound worker")
}

// ---- policer.classify -------------------------------------------------------------------------

// KeyClassify is "policer.classify/<interface>".
func KeyClassify(ifName string) scheduler.Key { return scheduler.Join(NameClassify, ifName) }

// ClassifyDescriptor manages policer.classify objects with policer_classify_set_interface
// (binapi/classify). Write-only (D-063): VPP 26.06's policer_classify_dump returns nothing for
// sw_if_index ~0 (the handler compares ~0 against the vector length and returns) and reads out
// of bounds for a single interface (vec_len on a pointer into the vector), so it is never
// called. Create is idempotent: VPP returns success without re-enabling when a table of that
// kind is already set on the interface.
type ClassifyDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*ClassifyDescriptor)(nil)

// NewClassify returns the policer.classify descriptor. Table names are resolved with
// df7.WithClassifyTables (DF-2's classify Store).
func NewClassify(c vpp.Client, owner string, opts ...df7.Option) *ClassifyDescriptor {
	return &ClassifyDescriptor{df7.NewBase(NameClassify, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *ClassifyDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	c, _ := df7.Decode[Classify](obj)
	return KeyClassify(c.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface and every named classify table.
func (d *ClassifyDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	c, _ := df7.Decode[Classify](obj)
	deps := []scheduler.Dependency{d.Opts.IfaceDep(c.Interface)}
	for _, t := range []string{c.IP4Table, c.IP6Table, c.L2Table} {
		if t != "" {
			deps = append(deps, scheduler.Dependency{Key: df7.ClassifyTableKey(t)})
		}
	}
	return deps
}

// ErrNoTable is returned when a classify table name cannot be resolved.
var ErrNoTable = fmt.Errorf("%w: unknown classify table", df7.ErrSpec)

func (d *ClassifyDescriptor) tables(c Classify) ([3]uint32, error) {
	out := [3]uint32{df7.NoIndex, df7.NoIndex, df7.NoIndex}
	for i, name := range []string{c.IP4Table, c.IP6Table, c.L2Table} {
		if name == "" {
			continue
		}
		if d.Opts.ClassifyIndex == nil {
			return out, fmt.Errorf("%w %q (no classify table resolver configured)", ErrNoTable, name)
		}
		idx, ok := d.Opts.ClassifyIndex(name)
		if !ok {
			return out, fmt.Errorf("%w %q", ErrNoTable, name)
		}
		out[i] = idx
	}
	return out, nil
}

func (d *ClassifyDescriptor) set(ctx context.Context, swIfIndex uint32, c Classify, add bool) error {
	t, err := d.tables(c)
	if err != nil {
		return err
	}
	_, err = classify.NewServiceClient(d.Client).PolicerClassifySetInterface(ctx, &classify.PolicerClassifySetInterface{
		SwIfIndex:     interface_types.InterfaceIndex(swIfIndex),
		IP4TableIndex: t[0], IP6TableIndex: t[1], L2TableIndex: t[2],
		IsAdd: add,
	})
	return d.Wrap(fmt.Sprintf("policer_classify_set_interface %s add=%v", c.Interface, add), err)
}

// Create implements scheduler.Descriptor (idempotent, see the type doc).
func (d *ClassifyDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	c, err := df7.DecodeValid[Classify](obj)
	if err != nil {
		return nil, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.Attach(c.Interface, string(KeyClassify(c.Interface)))
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, c, true); err != nil {
		return nil, err
	}
	return AttachMeta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: VPP keeps the first table of a kind while the feature
// is enabled ("already enabled" returns success), so a changed table is ErrRecreate.
func (d *ClassifyDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: re-resolve the interface, then is_add=0 with the same
// tables (VPP checks the table matches before removing). A vanished interface is success.
func (d *ClassifyDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	c, err := df7.Decode[Classify](obj)
	if err != nil {
		return err
	}
	key := string(KeyClassify(c.Interface))
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return err
	}
	idx, found, err := ifs.Reresolve(c.Interface, key)
	if err != nil {
		return err
	}
	if found {
		// NO_SUCH_TABLE: not bound with these tables (e.g. after a VPP restart) — nothing to remove
		if err := d.set(ctx, idx, c, false); err != nil && !df7.IsVPPError(err, api.NO_SUCH_TABLE) {
			return err
		}
	}
	return df7.Release(d.Owner, c.Interface, key)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *ClassifyDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameClassify, "policer_classify_dump is broken in VPP 26.06 (empty for ~0, out-of-bounds read per interface)")
}
