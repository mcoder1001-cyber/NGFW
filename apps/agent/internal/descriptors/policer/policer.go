package policer

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/policer"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Meta is the runtime handle of a policer: the pool index VPP assigned (policer_add reply).
type Meta struct{ Index uint32 }

// KeyPolicer is "policer.policer/<name>"; attachments depend on it.
func KeyPolicer(name string) scheduler.Key { return scheduler.Join(NamePolicer, name) }

// Descriptor manages policer.policer objects.
type Descriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*Descriptor)(nil)

// NewPolicer returns the policer.policer descriptor.
func NewPolicer(c vpp.Client, owner string, opts ...df7.Option) *Descriptor {
	return &Descriptor{df7.NewBase(NamePolicer, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *Descriptor) KeyOf(obj proto.Message) scheduler.Key {
	p, _ := df7.Decode[Policer](obj)
	return KeyPolicer(p.Name)
}

// Dependencies implements scheduler.Descriptor: none.
func (*Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// vppName is the name the policer carries in VPP.
func vppName(owner, name string) (string, error) { return vpp.OwnerTag(owner, name) }

// Create implements scheduler.Descriptor: policer_add.
func (d *Descriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p, err := df7.DecodeValid[Policer](obj)
	if err != nil {
		return nil, err
	}
	name, err := vppName(d.Owner, p.Name)
	if err != nil {
		return nil, err
	}
	rep, err := policer.NewServiceClient(d.Client).PolicerAdd(ctx, &policer.PolicerAdd{Name: name, Infos: p.config()})
	if err != nil {
		return nil, d.Wrap("policer_add "+name, df7.PluginError("policer", err))
	}
	return Meta{Index: rep.PolicerIndex}, nil
}

// Update implements scheduler.Descriptor: policer_update in place (the index, and thus every
// attachment, is kept; VPP resets the policer's counters and buckets).
func (d *Descriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	oldP, err := df7.Decode[Policer](oldObj)
	if err != nil {
		return nil, err
	}
	newP, err := df7.DecodeValid[Policer](newObj)
	if err != nil {
		return nil, err
	}
	if oldP.Name != newP.Name {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(Meta)
	if !ok {
		return nil, df7.BadMeta(NamePolicer, meta)
	}
	if _, err := policer.NewServiceClient(d.Client).PolicerUpdate(ctx, &policer.PolicerUpdate{PolicerIndex: m.Index, Infos: newP.config()}); err != nil {
		return nil, d.Wrap(fmt.Sprintf("policer_update %d", m.Index), err)
	}
	return m, nil
}

// Delete implements scheduler.Descriptor: policer_del by index. Attachments depend on the
// policer, so the scheduler detaches first.
func (d *Descriptor) Delete(ctx context.Context, _ proto.Message, meta any) error {
	m, ok := meta.(Meta)
	if !ok {
		return df7.BadMeta(NamePolicer, meta)
	}
	if _, err := policer.NewServiceClient(d.Client).PolicerDel(ctx, &policer.PolicerDel{PolicerIndex: m.Index}); err != nil {
		return d.Wrap(fmt.Sprintf("policer_del %d", m.Index), err)
	}
	return nil
}

// owned is one policer of this owner as dumped.
type owned struct {
	Index uint32
	Spec  Policer
}

// Retrieve implements scheduler.Descriptor: policer_dump_v2 for every pool index. VPP's
// policer_details carries no index, so the dump is done per index (policer_dump_v2 with a
// single policer_index answers nothing for a free slot) until every policer the full dump
// listed has been seen; only names parsing as this owner's tag are kept.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	list, err := dumpOwned(ctx, d.Client, d.Owner)
	if err != nil {
		return nil, d.Wrap("policer_dump_v2", err)
	}
	out := make([]scheduler.KV, 0, len(list))
	for _, o := range list {
		out = append(out, df7.KV(KeyPolicer(o.Spec.Name), o.Spec, Meta{Index: o.Index}))
	}
	return out, nil
}

func dumpV2(ctx context.Context, c vpp.Client, index uint32) ([]*policer.PolicerDetails, error) {
	stream, err := policer.NewServiceClient(c).PolicerDumpV2(ctx, &policer.PolicerDumpV2{PolicerIndex: index})
	if err != nil {
		return nil, df7.PluginError("policer", err)
	}
	return df7.Collect(stream.Recv)
}

// maxProbe bounds the per-index walk (a pool never has more free slots than this in practice).
const maxProbe = 1 << 16

// dumpOwned returns this owner's policers with their pool index, sorted by name.
func dumpOwned(ctx context.Context, c vpp.Client, owner string) ([]owned, error) {
	all, err := dumpV2(ctx, c, df7.NoIndex)
	if err != nil {
		return nil, err
	}
	want := 0
	for _, det := range all {
		if _, ok := vpp.ParseOwnerTag(det.Name, owner); ok {
			want++
		}
	}
	var out []owned
	seen := 0
	for idx := uint32(0); seen < len(all) && len(out) < want && idx < maxProbe; idx++ {
		dets, err := dumpV2(ctx, c, idx)
		if err != nil {
			return nil, err
		}
		if len(dets) == 0 {
			continue
		}
		seen++
		name, ok := vpp.ParseOwnerTag(dets[0].Name, owner)
		if !ok {
			continue
		}
		out = append(out, owned{Index: idx, Spec: decode(name, dets[0])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec.Name < out[j].Spec.Name })
	return out, nil
}

func decode(name string, det *policer.PolicerDetails) Policer {
	return Policer{
		Name: name,
		CIR:  det.Cir, EIR: det.Eir, CB: det.Cb, EB: det.Eb,
		RateType:   nameOf(rateTypes, det.RateType),
		RoundType:  nameOf(roundTypes, det.RoundType),
		Type:       nameOf(policerTypes, det.Type),
		ColorAware: det.ColorAware,
		Conform:    Action{Type: nameOf(actionTypes, det.ConformAction.Type), DSCP: det.ConformAction.Dscp},
		Exceed:     Action{Type: nameOf(actionTypes, det.ExceedAction.Type), DSCP: det.ExceedAction.Dscp},
		Violate:    Action{Type: nameOf(actionTypes, det.ViolateAction.Type), DSCP: det.ViolateAction.Dscp},
	}
}

// LookupIndex returns the pool index of this owner's policer called name (for other plugins
// and the attachments' Meta).
func LookupIndex(ctx context.Context, c vpp.Client, owner, name string) (uint32, bool, error) {
	list, err := dumpOwned(ctx, c, owner)
	if err != nil {
		return 0, false, err
	}
	for _, o := range list {
		if o.Spec.Name == name {
			return o.Index, true, nil
		}
	}
	return 0, false, nil
}

// Reset is the policer_reset action helper: refill the policer's token buckets.
func Reset(ctx context.Context, c vpp.Client, index uint32) error {
	_, err := policer.NewServiceClient(c).PolicerReset(ctx, &policer.PolicerReset{PolicerIndex: index})
	return err
}
