// Package df6test holds the test doubles and shared-host fixtures the DF-6 descriptor tests
// use: a fake VPP with an interface table (sw_interface_dump / tag / loopback handlers on top
// of internal/vpp/fake), a govpp connection to the host VPP for VRX_INTEGRATION=1 tests, and
// prefixed fixtures (loopbacks, IP tables, the MPLS table) that clean themselves up.
package df6test

import (
	"fmt"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/fake"
)

// FakeVPP is a fake.Client with an interface table behind sw_interface_dump,
// sw_interface_tag_add_del and create_loopback_instance / delete_loopback. Plugin tests add
// their tunnel handlers on top and call AddInterface when a "tunnel" is created so the
// descriptor's Retrieve can resolve names and tags.
type FakeVPP struct {
	*fake.Client
	mu     sync.Mutex
	next   uint32
	ifaces map[uint32]*interfaces.SwInterfaceDetails
	// features counts enables per "<arc>/<node>/<sw_if_index>" (VPP does not deduplicate)
	features map[string]int
}

// NewFakeVPP returns a fake with local0 at index 0 and the next index at 1.
func NewFakeVPP() *FakeVPP {
	v := &FakeVPP{
		Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		next:   1,
		ifaces: map[uint32]*interfaces.SwInterfaceDetails{0: {SwIfIndex: 0, InterfaceName: "local0"}},
	}
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		idx := make([]int, 0, len(v.ifaces))
		for i := range v.ifaces {
			idx = append(idx, int(i))
		}
		sort.Ints(idx)
		out := make([]api.Message, 0, len(idx))
		for _, i := range idx {
			d := *v.ifaces[uint32(i)] //nolint:gosec // indices are small
			out = append(out, &d)
		}
		return out, nil
	})
	v.On("sw_interface_tag_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceTagAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		d, ok := v.ifaces[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&interfaces.SwInterfaceTagAddDelReply{Retval: -2}}, nil
		}
		if r.IsAdd {
			d.Tag = r.Tag
		} else {
			d.Tag = ""
		}
		return []api.Message{&interfaces.SwInterfaceTagAddDelReply{}}, nil
	})
	v.On("create_loopback_instance", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.CreateLoopbackInstance)
		idx := v.AddInterface(fmt.Sprintf("loop%d", r.UserInstance), "")
		return []api.Message{&interfaces.CreateLoopbackInstanceReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	v.On("delete_loopback", func(req api.Message) ([]api.Message, error) {
		v.RemoveInterface(uint32(req.(*interfaces.DeleteLoopback).SwIfIndex))
		return []api.Message{&interfaces.DeleteLoopbackReply{}}, nil
	})
	v.Reply("sw_interface_add_del_address", &interfaces.SwInterfaceAddDelAddressReply{})
	v.Reply("sw_interface_set_flags", &interfaces.SwInterfaceSetFlagsReply{})
	v.features = map[string]int{}
	v.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*feature.FeatureIsEnabled)
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: v.Feature(r.ArcName, r.FeatureName, uint32(r.SwIfIndex)) > 0}}, nil
	})
	return v
}

// AddInterface adds an interface with the next free sw_if_index and returns it.
func (v *FakeVPP) AddInterface(name, tag string) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()
	idx := v.next
	v.next++
	v.ifaces[idx] = &interfaces.SwInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), InterfaceName: name, Tag: tag, SupSwIfIndex: idx}
	return idx
}

// AddInterfaceAt adds an interface at a fixed sw_if_index (for "other owner" fixtures).
func (v *FakeVPP) AddInterfaceAt(idx uint32, name, tag string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ifaces[idx] = &interfaces.SwInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), InterfaceName: name, Tag: tag, SupSwIfIndex: idx}
	if idx >= v.next {
		v.next = idx + 1
	}
}

// SetL2Address sets the MAC of an interface (L2 tunnels have one, L3 tunnels do not).
func (v *FakeVPP) SetL2Address(idx uint32, mac [6]byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if d, ok := v.ifaces[idx]; ok {
		d.L2Address = mac
	}
}

// RemoveInterface deletes an interface.
func (v *FakeVPP) RemoveInterface(idx uint32) {
	v.ClearFeatures(idx)
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.ifaces, idx)
}

// Has reports whether idx exists.
func (v *FakeVPP) Has(idx uint32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.ifaces[idx]
	return ok
}

// Tag returns the tag of idx.
func (v *FakeVPP) Tag(idx uint32) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if d, ok := v.ifaces[idx]; ok {
		return d.Tag
	}
	return ""
}

// Index returns the sw_if_index of the interface named name (ok false when absent).
func (v *FakeVPP) Index(name string) (uint32, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i, d := range v.ifaces {
		if d.InterfaceName == name {
			return i, true
		}
	}
	return 0, false
}

// Addr is a shorthand for ip_types.Address from a string; it panics on a bad literal (test
// fixtures only).
func Addr(s string) ip_types.Address {
	a, err := ip_types.ParseAddress(s)
	if err != nil {
		panic(err)
	}
	return a
}

// Prefix is a shorthand for ip_types.Prefix from a string; it panics on a bad literal.
func Prefix(s string) ip_types.Prefix {
	p, err := ip_types.ParsePrefix(s)
	if err != nil {
		panic(err)
	}
	return p
}

// IP6 is a shorthand for ip_types.IP6Address from a string; it panics on a bad literal.
func IP6(s string) ip_types.IP6Address {
	a, err := ip_types.ParseIP6Address(s)
	if err != nil {
		panic(err)
	}
	return a
}

// IP4 is a shorthand for ip_types.IP4Address from a string; it panics on a bad literal.
func IP4(s string) ip_types.IP4Address {
	a, err := ip_types.ParseIP4Address(s)
	if err != nil {
		panic(err)
	}
	return a
}

// SetBoot makes control_ping report VPP main-thread PID pid (df6.BootID): a new value
// simulates a VPP restart for the per-boot claims of write-only descriptors (D-076).
func (v *FakeVPP) SetBoot(pid uint32) {
	v.Reply("control_ping", &memclnt.ControlPingReply{VpePID: pid})
}

// SetFeature applies one vnet_feature_enable_disable to the fake's feature table: an enable
// always adds one more instance (VPP stacks), a disable removes one.
func (v *FakeVPP) SetFeature(arc, node string, idx uint32, enable bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	k := fmt.Sprintf("%s/%s/%d", arc, node, idx)
	if enable {
		v.features[k]++
	} else if v.features[k] > 0 {
		v.features[k]--
	}
}

// Feature returns how many instances of node are on the arc of idx.
func (v *FakeVPP) Feature(arc, node string, idx uint32) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.features[fmt.Sprintf("%s/%s/%d", arc, node, idx)]
}

// ClearFeatures drops every feature of idx (VPP forgets them when the interface is deleted).
func (v *FakeVPP) ClearFeatures(idx uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	suffix := fmt.Sprintf("/%d", idx)
	for k := range v.features {
		if len(k) > len(suffix) && k[len(k)-len(suffix):] == suffix {
			delete(v.features, k)
		}
	}
}
