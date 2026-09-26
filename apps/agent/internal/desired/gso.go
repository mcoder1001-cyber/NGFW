package desired

// F-loopback-bvi-gso-lldp-span: interfaces.<if>.gso ⇄ gso.interface/<if> (descriptors/gso).
//
//	interfaces.<if>.gso = true   → gso.interface/<if>   (present ⇔ on; depends on interface/<if>)
//
// Retrieve reports gso.interface only for an interface whose applied-once record of the running VPP
// boot and VPP's feature_is_enabled read-back agree (descriptors/gso); the assembler turns it back
// into `gso: true`, and into `gso: false` where the stored document sets the key and VPP has no GSO.

import (
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/gso"
	"ngfw/agent/internal/scheduler"
)

// Gso emits gso.interface/<if> for every interface of ifs with gso: true.
func Gso(s Sink, ifs map[string]*vrxv1.Interface) {
	for _, name := range sortedKeys(ifs) {
		if ifs[name].GetGso() {
			s.Add(gso.Key(name), gso.Interface{Interface: name}.Proto(), Ptr("interfaces", name, "gso"))
		}
	}
}

// AssembleGso sets interfaces.<if>.gso from the retrieved gso.interface objects (see the file
// comment); stored is the agent's stored `interfaces` document.
func AssembleGso(ds *vrxv1.DesiredState, kvs []scheduler.KV, stored map[string]*vrxv1.Interface) {
	on := map[string]bool{}
	for _, kv := range kvs {
		if kv.Key.Descriptor() == gso.Name {
			on[kv.Key.ID()] = true
		}
	}
	for _, name := range sortedKeys(on) {
		if lookupNode(ds, name, true) {
			ds.Interfaces[name].Gso = proto.Bool(true)
		}
	}
	for name, itf := range stored {
		if itf.Gso != nil && !on[name] && lookupNode(ds, name, false) {
			ds.Interfaces[name].Gso = proto.Bool(false)
		}
	}
}
