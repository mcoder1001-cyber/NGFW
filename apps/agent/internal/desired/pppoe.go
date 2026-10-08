package desired

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	pppoedesc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/scheduler"
)

// PppoeClientName is the client singleton descriptor.
const PppoeClientName = "pppoe.client.config"

// PppoeClientKey is the client singleton key.
var PppoeClientKey = scheduler.Join(PppoeClientName, "ngfw")

// Pppoe projects references only. Resolution happens inside the daemon descriptor.
func Pppoe(s Sink, ifs map[string]*ngfwv1.Interface, supervised bool, carrierOwner ...string) {
	doc := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{}}
	hosts := map[string]string{}
	for _, name := range sortedKeys(ifs) {
		c := ifs[name].GetPppoe()
		if c == nil || (c.Enabled != nil && !c.GetEnabled()) {
			continue
		}
		pt := Ptr("interfaces", name, "pppoe")
		parent := c.GetParent()
		if parent == "" {
			parent = name
		}
		p := ifs[parent]
		if supervised && len(carrierOwner) > 0 && carrierOwner[0] != "" {
			if parent == name {
				s.Errorf(pt+"/parent", "pppoe.carrier-parent", "kernel PPP requires a distinct existing raw parent")
				continue
			}
			if err := pppoedesc.CopyCarrierParentReference(doc, ifs, parent); err != nil {
				s.Errorf(pt+"/parent", "pppoe.carrier-parent", "%v", err)
				continue
			}
			doc.Interfaces[name] = &ngfwv1.Interface{Pppoe: proto.Clone(c).(*ngfwv1.Pppoe)}
			hosts[parent] = name
			s.Warnf(pt+"/passwordRef", "pppoe.secret-unavailable", "the password reference must resolve in the agent secret cache before apply")
			continue
		}
		if p.GetLcp() == nil {
			s.Errorf(pt+"/parent", "pppoe.lcp-required", "parent requires a linux-cp pair")
			continue
		}
		host, err := lcpmap.HostName(parent, p.GetLcp())
		if err != nil {
			s.Errorf(pt+"/parent", "pppoe.lcp-invalid", "%v", err)
			continue
		}
		if supervised && p.GetLcp().GetNetns() != "" {
			s.Errorf(pt+"/parent", "pppoe.netns-unavailable", "pppd supervision in a network namespace is unavailable")
			continue
		}
		if previous, exists := hosts[host]; exists {
			s.Errorf(pt+"/parent", "pppoe.parent-conflict", "parent tap already used by %s", previous)
			continue
		}
		hosts[host] = name
		record := proto.Clone(c).(*ngfwv1.Pppoe)
		record.Parent = proto.String(parent)
		if doc.Interfaces[name] == nil {
			doc.Interfaces[name] = &ngfwv1.Interface{}
		}
		doc.Interfaces[name].Pppoe = record
		if doc.Interfaces[parent] == nil {
			doc.Interfaces[parent] = &ngfwv1.Interface{}
		}
		doc.Interfaces[parent].Lcp = proto.Clone(p.GetLcp()).(*ngfwv1.InterfaceLcp)
		s.Warnf(pt+"/passwordRef", "pppoe.secret-unavailable", "the password reference must resolve in the agent secret cache before apply")
		if !supervised {
			s.Warnf(pt, "pppoe.not-supervised", "session files are rendered in the slot tree; the host units are not supervised")
		}
	}
	if len(hosts) > 0 {
		s.Add(PppoeClientKey, doc, "/interfaces")
	}
}

// AssemblePppoe reconstructs only the PPPoE config from the applied manifest.
func AssemblePppoe(ds *ngfwv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key != PppoeClientKey {
			continue
		}
		doc, ok := kv.Value.(*ngfwv1.DesiredState)
		if !ok {
			continue
		}
		for name, itf := range doc.GetInterfaces() {
			if itf.Pppoe == nil {
				continue
			}
			if ds.Interfaces == nil {
				ds.Interfaces = map[string]*ngfwv1.Interface{}
			}
			if ds.Interfaces[name] == nil {
				ds.Interfaces[name] = &ngfwv1.Interface{}
			}
			ds.Interfaces[name].Pppoe = proto.Clone(itf.Pppoe).(*ngfwv1.Pppoe)
			if ds.Interfaces[name].Pppoe.GetParent() == name {
				ds.Interfaces[name].Pppoe.Parent = nil
			}
		}
	}
}

// PppoePointer reports the password location without ever quoting material.
func PppoePointer(name string) string {
	return fmt.Sprintf("%s/passwordRef", Ptr("interfaces", name, "pppoe"))
}
