package objects

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

// Descriptor names of the objects.* family (subsystems.Domains["objects"]).
const (
	TagName          = "objects.tag"
	AddressName      = "objects.address"
	AddressGroupName = "objects.address-group"
	ServiceName      = "objects.service"
	ServiceGroupName = "objects.service-group"
	ScheduleName     = "objects.schedule"
	ZoneName         = "objects.zone"
)

var descriptorOf = map[Kind]string{
	KindTags: TagName, KindAddresses: AddressName, KindAddressGroups: AddressGroupName, KindServices: ServiceName,
	KindServiceGroups: ServiceGroupName, KindSchedules: ScheduleName, KindZones: ZoneName,
}

// DescriptorNames lists the family in registration order (Kinds order).
func DescriptorNames() []string {
	out := make([]string, 0, len(Kinds))
	for _, k := range Kinds {
		out = append(out, descriptorOf[k])
	}
	return out
}

// DescriptorName is the descriptor of kind k ("" for an unknown kind).
func DescriptorName(k Kind) string { return descriptorOf[k] }

// Key is the scheduler key of kind k's object name: "<descriptor>/<name>".
func Key(k Kind, name string) scheduler.Key { return scheduler.Join(descriptorOf[k], name) }

// Value wraps one object as the family's value: an ObjectsConfig holding exactly that entry in
// kind k's map (v is not cloned). v must be kind k's message type (*vrxv1.AddressObject for
// KindAddresses, …).
func Value(k Kind, name string, v proto.Message) (*vrxv1.ObjectsConfig, error) {
	doc := &vrxv1.ObjectsConfig{}
	if err := setEntry(doc, k, name, v); err != nil {
		return nil, err
	}
	return doc, nil
}

// single returns the one entry of kind k in a family value — O(1): the scheduler calls KeyOf and
// Dependencies for every object of a transaction many times (review F1 profile).
func single(k Kind, obj proto.Message) (string, proto.Message, error) {
	doc, ok := obj.(*vrxv1.ObjectsConfig)
	if !ok {
		return "", nil, fmt.Errorf("%w: value %T is not an ObjectsConfig", ErrInvalid, obj)
	}
	total := len(doc.GetAddresses()) + len(doc.GetAddressGroups()) + len(doc.GetServices()) + len(doc.GetServiceGroups()) +
		len(doc.GetSchedules()) + len(doc.GetZones()) + len(doc.GetTags())
	var name string
	var value proto.Message
	n := 0
	one := func(key string, v proto.Message) { name, value, n = key, v, n+1 }
	switch k {
	case KindAddresses:
		for key, v := range doc.GetAddresses() {
			one(key, v)
		}
	case KindAddressGroups:
		for key, v := range doc.GetAddressGroups() {
			one(key, v)
		}
	case KindServices:
		for key, v := range doc.GetServices() {
			one(key, v)
		}
	case KindServiceGroups:
		for key, v := range doc.GetServiceGroups() {
			one(key, v)
		}
	case KindSchedules:
		for key, v := range doc.GetSchedules() {
			one(key, v)
		}
	case KindZones:
		for key, v := range doc.GetZones() {
			one(key, v)
		}
	case KindTags:
		for key, v := range doc.GetTags() {
			one(key, v)
		}
	}
	if n != 1 || total != 1 {
		return "", nil, fmt.Errorf("%w: a %s value holds exactly one %s entry (has %d of %d entries)", ErrInvalid, descriptorOf[k], k, n, total)
	}
	return name, value, nil
}

// Register adds the objects.* family over rt's store to r (Kinds order: tags, addresses, groups,
// services, service groups, schedules, zones).
func Register(r scheduler.Registry, rt *Runtime) {
	for _, k := range Kinds {
		r.Register(&descriptor{kind: k, store: rt.store})
	}
}

// descriptor is one kind of the family. It has no VPP object: its actual state is the Store.
type descriptor struct {
	kind  Kind
	store *Store
}

var _ scheduler.Descriptor = (*descriptor)(nil)

func (d *descriptor) Name() string { return descriptorOf[d.kind] }

func (d *descriptor) KeyOf(obj proto.Message) scheduler.Key {
	name, _, err := single(d.kind, obj)
	if err != nil {
		return scheduler.Join(d.Name(), "") // never a real object; Create refuses it
	}
	return Key(d.kind, name)
}

// Dependencies only order a transaction (all optional): groups after their members, tagged
// objects after their tags. Existence is the schema's job (objects.*-group-members, tags-exist).
func (d *descriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	_, v, err := single(d.kind, obj)
	if err != nil {
		return nil
	}
	var deps []scheduler.Dependency
	opt := func(k Kind, name string) {
		deps = append(deps, scheduler.Dependency{Key: Key(k, name), Optional: true})
	}
	switch o := v.(type) {
	case *vrxv1.AddressGroup:
		for _, m := range o.GetMembers() {
			opt(KindAddresses, m)
			opt(KindAddressGroups, m)
		}
	case *vrxv1.ServiceGroup:
		for _, m := range o.GetMembers() {
			opt(KindServices, m)
			opt(KindServiceGroups, m)
		}
	}
	if t, ok := v.(interface{ GetTags() []string }); ok {
		for _, tag := range t.GetTags() {
			opt(KindTags, tag)
		}
	}
	return deps
}

func (d *descriptor) Create(_ context.Context, obj proto.Message) (any, error) {
	name, v, err := single(d.kind, obj)
	if err != nil {
		return nil, err
	}
	return nil, d.store.put(d.kind, name, v)
}

func (d *descriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

func (d *descriptor) Delete(_ context.Context, obj proto.Message, _ any) error {
	name, _, err := single(d.kind, obj)
	if err != nil {
		return err
	}
	return d.store.remove(d.kind, name)
}

// Retrieve returns the kind's applied objects from the store. It first writes pending changes (the
// scheduler retrieves to verify every transaction, so a transaction is written once, review F1); a write
// error is logged and counted by the store and does not fail Retrieve — memory is the applied state.
func (d *descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_ = d.store.Flush()
	es := d.store.entries(d.kind)
	out := make([]scheduler.KV, 0, len(es))
	for _, e := range es {
		v, err := Value(d.kind, e.name, e.value)
		if err != nil {
			return nil, err
		}
		out = append(out, scheduler.KV{Key: Key(d.kind, e.name), Value: v})
	}
	return out, nil
}
