package coretest

import (
	"sort"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/fake"
)

// F-nat44-ei-64-66-nptv6 / review M1 (D-134, TD-23): every VPP message is modelled by at most one feature extension.
// TD-23's registry panics in every New() when a second extension claims a message — F-nat44-ed-sessions' stub of
// nat44_ei_show_running_config and this branch's nat44-ei model did.
func TestExtensionsModelDisjointMessages(t *testing.T) {
	var names []string
	for _, msgs := range api.GetRegisteredMessages() {
		for _, m := range msgs {
			names = append(names, m.GetMessageName())
		}
	}
	sort.Strings(names)
	bare := func() *VPP { // New without the extensions
		v := &VPP{
			Client:   fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
			next:     1,
			Ifaces:   map[uint32]*Iface{0: {Index: 0, Name: "local0", DevType: "local", Addrs: map[string]bool{}}},
			Tables:   map[tableKey]string{{0, false}: "ipv4-VRF:0", {0, true}: "ipv6-VRF:0"},
			Routes:   map[routeKey]ip.IPRoute{},
			Internal: map[routeKey]uint8{},
		}
		v.install()
		v.installIfExt()
		return v
	}
	base := bare()
	extMu.Lock()
	all := append([]extension(nil), extAll...)
	extMu.Unlock()
	claimed := map[string]string{}
	for _, ext := range all {
		v := bare()
		ext.install(v)
		for _, n := range names {
			if !v.Handles(n) || base.Handles(n) {
				continue
			}
			if j, ok := claimed[n]; ok {
				t.Errorf("VPP message %q is modelled by extensions %q and %q", n, j, ext.name)
				continue
			}
			claimed[n] = ext.name
		}
	}
	for _, n := range []string{"nat44_ei_show_running_config", "nat44_show_running_config", "nat64_st_dump", "npt66_binding_add_del"} {
		if _, ok := claimed[n]; !ok {
			t.Errorf("no extension models %q", n)
		}
	}
}
