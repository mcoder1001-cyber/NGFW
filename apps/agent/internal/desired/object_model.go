package desired

// F-object-model: builder and assembler of the `objects` domain. Every entry of the objects
// document becomes one object of the agent-local objects.* family (internal/objects):
//
//	objects.<kind>.<name>  →  objects.<descriptor>/<name>  value: ObjectsConfig{<kind>: {<name>: entry}}
//	                          pointer /objects/<kind>/<name>
//
// with <kind> ∈ tags, addresses, addressGroups, services, serviceGroups, schedules, zones
// (descriptors objects.tag, objects.address, objects.address-group, objects.service,
// objects.service-group, objects.schedule, objects.zone). There are no VPP objects: the
// descriptors write the agent's objects store, and Retrieve returns it.
//
// Defence in depth (the schema's semantic rules already reject these): every address and service
// group is expanded once — a membership cycle, an unknown member or an invalid leaf is an ERROR
// (rule objects.object-model-expansion) at the group; a group larger than objects.MaxEntries is a
// WARNING (objects.object-model-expansion-limit), because an ACL rule using it would be refused.

import (
	"errors"
	"sort"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/scheduler"
)

// ObjectModel emits the objects.* objects of objs.
func ObjectModel(s Sink, objs *vrxv1.ObjectsConfig) {
	if objs == nil {
		return
	}
	for _, k := range objects.Kinds {
		for _, e := range kindEntries(objs, k) {
			v, err := objects.Value(k, e.name, e.value)
			pt := Ptr("objects", string(k), e.name)
			if err != nil {
				s.Errorf(pt, "objects.object-model-invalid", "%v", err)
				continue
			}
			s.Add(objects.Key(k, e.name), v, pt)
		}
	}
	for _, name := range sortedKeys(objs.GetAddressGroups()) {
		_, err := objects.Expand(objs, name)
		groupIssue(s, Ptr("objects", "addressGroups", name), err)
	}
	for _, name := range sortedKeys(objs.GetServiceGroups()) {
		_, err := objects.ExpandService(objs, name)
		groupIssue(s, Ptr("objects", "serviceGroups", name), err)
	}
}

func groupIssue(s Sink, pt string, err error) {
	var le *objects.LimitError
	switch {
	case err == nil:
	case errors.As(err, &le):
		s.Warnf(pt, "objects.object-model-expansion-limit", "%v; an ACL rule that uses it is refused", err)
	default:
		s.Errorf(pt, "objects.object-model-expansion", "%v", err)
	}
}

type objectEntry struct {
	name  string
	value proto.Message
}

// kindEntries lists kind k of objs sorted by name.
func kindEntries(objs *vrxv1.ObjectsConfig, k objects.Kind) []objectEntry {
	var out []objectEntry
	add := func(n string, v proto.Message) { out = append(out, objectEntry{n, v}) }
	switch k {
	case objects.KindTags:
		for n, v := range objs.GetTags() {
			add(n, v)
		}
	case objects.KindAddresses:
		for n, v := range objs.GetAddresses() {
			add(n, v)
		}
	case objects.KindAddressGroups:
		for n, v := range objs.GetAddressGroups() {
			add(n, v)
		}
	case objects.KindServices:
		for n, v := range objs.GetServices() {
			add(n, v)
		}
	case objects.KindServiceGroups:
		for n, v := range objs.GetServiceGroups() {
			add(n, v)
		}
	case objects.KindSchedules:
		for n, v := range objs.GetSchedules() {
			add(n, v)
		}
	case objects.KindZones:
		for n, v := range objs.GetZones() {
			add(n, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// AssembleObjectModel merges the retrieved objects.* values into one objects document; nil when
// the agent holds no object (an empty map domain and an absent one are the same thing, proto.md §1).
func AssembleObjectModel(kvs []scheduler.KV) *vrxv1.ObjectsConfig {
	family := map[string]bool{}
	for _, n := range objects.DescriptorNames() {
		family[n] = true
	}
	var out *vrxv1.ObjectsConfig
	for _, kv := range kvs {
		v, ok := kv.Value.(*vrxv1.ObjectsConfig)
		if !ok || !family[kv.Key.Descriptor()] {
			continue
		}
		if out == nil {
			out = &vrxv1.ObjectsConfig{}
		}
		proto.Merge(out, v)
	}
	return out
}
