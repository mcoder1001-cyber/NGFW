package policer

import (
	"context"
	"errors"
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
	idx, found, err := d.currentIndex(ctx, newP.Name, m.Index)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: policer %q no longer exists", NamePolicer, newP.Name)
	}
	if _, err := policer.NewServiceClient(d.Client).PolicerUpdate(ctx, &policer.PolicerUpdate{PolicerIndex: idx, Infos: newP.config()}); err != nil {
		return nil, d.Wrap(fmt.Sprintf("policer_update %d", idx), err)
	}
	return Meta{Index: idx}, nil
}

// currentIndex re-verifies, right before an index-addressed call (D-071, review M5), that pool
// index hint still holds this owner's policer name; otherwise it looks the policer up by name.
func (d *Descriptor) currentIndex(ctx context.Context, name string, hint uint32) (uint32, bool, error) {
	want, err := vppName(d.Owner, name)
	if err != nil {
		return 0, false, err
	}
	dets, err := dumpV2(ctx, d.Client, hint)
	if err != nil {
		return 0, false, d.Wrap("policer_dump_v2", err)
	}
	if len(dets) > 0 && dets[0].Name == want {
		return hint, true, nil
	}
	idx, found, err := LookupIndex(ctx, d.Client, d.Owner, name)
	if err != nil {
		return 0, false, d.Wrap("policer lookup", err)
	}
	return idx, found, nil
}

// Delete implements scheduler.Descriptor: policer_del by index, after re-verifying right
// before the delete that the index still holds this owner's policer of that name (indexes are
// reused after a VPP restart, D-071); when it does not, the policer is looked up by name, and a
// policer that no longer exists is success. Attachments depend on the policer, so the scheduler
// detaches first.
func (d *Descriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	p, err := df7.Decode[Policer](obj)
	if err != nil {
		return err
	}
	m, ok := meta.(Meta)
	if !ok {
		return df7.BadMeta(NamePolicer, meta)
	}
	index, found, err := d.currentIndex(ctx, p.Name, m.Index)
	if err != nil || !found {
		return err
	}
	if _, err := policer.NewServiceClient(d.Client).PolicerDel(ctx, &policer.PolicerDel{PolicerIndex: index}); err != nil {
		return d.Wrap(fmt.Sprintf("policer_del %d (%s)", index, p.Name), err)
	}
	return nil
}

// owned is one policer of this owner as dumped.
type owned struct {
	Index uint32
	Spec  Policer
	Det   *policer.PolicerDetails
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
		out = append(out, owned{Index: idx, Spec: decode(name, dets[0]), Det: dets[0]})
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

// Reset is the policer_reset action helper: refill the token buckets of this owner's policer
// called name (looked up by name right before the call — never a stored index, review M5).
func Reset(ctx context.Context, c vpp.Client, owner, name string) error {
	_, err := ResetIndex(ctx, c, owner, name)
	return err
}

// ErrNoPolicer is returned (wrapped) by ResetIndex when this owner has no policer of that name.
var ErrNoPolicer = errors.New(NamePolicer + ": no such policer")

// ResetIndex is Reset that also returns the pool index it reset (F-qos-flat's QosPolicerReset);
// ErrNoPolicer when the owner has no policer of that name.
func ResetIndex(ctx context.Context, c vpp.Client, owner, name string) (uint32, error) {
	idx, found, err := LookupIndex(ctx, c, owner, name)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, fmt.Errorf("%w: %q of owner %q", ErrNoPolicer, name, owner)
	}
	if _, err := policer.NewServiceClient(c).PolicerReset(ctx, &policer.PolicerReset{PolicerIndex: idx}); err != nil {
		return 0, fmt.Errorf("%s: policer_reset %d (%s): %w", NamePolicer, idx, name, err)
	}
	return idx, nil
}

// State is one of this owner's policers as VPP reports it: the pool index, the configuration and
// the token buckets (policer_details, VPP's internal token units). F-qos-flat's QosPolicerState.
type State struct {
	Index          uint32
	Spec           Policer
	CurrentBucket  uint32
	CurrentLimit   uint32
	ExtendedBucket uint32
	ExtendedLimit  uint32
}

// States returns this owner's policers, sorted by name, with their pool index and token buckets:
// the same walk as Retrieve (one full policer_dump_v2, then one per pool index until every
// policer the full dump listed was seen). Read-only.
func States(ctx context.Context, c vpp.Client, owner string) ([]State, error) {
	list, err := dumpOwned(ctx, c, owner)
	if err != nil {
		return nil, fmt.Errorf("%s: policer_dump_v2: %w", NamePolicer, err)
	}
	out := make([]State, 0, len(list))
	for _, o := range list {
		out = append(out, State{Index: o.Index, Spec: o.Spec, CurrentBucket: o.Det.CurrentBucket, CurrentLimit: o.Det.CurrentLimit,
			ExtendedBucket: o.Det.ExtendedBucket, ExtendedLimit: o.Det.ExtendedLimit})
	}
	return out, nil
}
