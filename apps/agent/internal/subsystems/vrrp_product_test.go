package subsystems

import (
	"errors"
	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	vrrpapi "ngfw/agent/binapi/vrrp"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"testing"
)

func TestVrrpProductWiringFiltersOwnedMasterVIP(t *testing.T) {
	t.Setenv(EnvVrrpVPP, "on")
	t.Setenv(EnvKeepalived, "off")
	v := coretest.New()
	idx := v.AddInterface("loop701", "Loopback", "w7:loop701")
	v.Ifaces[idx].Addrs["10.7.1.1/24"] = true
	v.Ifaces[idx].Addrs["10.7.1.254/24"] = true
	address, _ := ip_types.ParseAddress("10.7.1.254")
	foreign := v.AddInterface("loop801", "Loopback", "w8:loop801")
	dumps := 0
	v.On("vrrp_vr_dump", func(api.Message) ([]api.Message, error) {
		dumps++
		return []api.Message{&vrrpapi.VrrpVrDetails{Config: vrrpapi.VrrpVrConf{SwIfIndex: interface_types.InterfaceIndex(idx), VrID: 7, Flags: vrrpapi.VRRP_API_VR_ACCEPT}, Runtime: vrrpapi.VrrpVrRuntime{State: vrrpapi.VRRP_API_VR_STATE_MASTER}, Addrs: []ip_types.Address{address}}, &vrrpapi.VrrpVrDetails{Config: vrrpapi.VrrpVrConf{SwIfIndex: interface_types.InterfaceIndex(foreign), VrID: 8, Flags: vrrpapi.VRRP_API_VR_ACCEPT}, Runtime: vrrpapi.VrrpVrRuntime{State: vrrpapi.VRRP_API_VR_STATE_MASTER}, Addrs: []ip_types.Address{address}}}, nil
	})
	reg := scheduler.NewRegistry()
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "w7")
	if err != nil {
		t.Fatal(err)
	}
	w, err := registerMock(reg, Env{Client: v, Owner: "w7", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	descriptor, ok := reg.Get(core.InterfaceAddrName)
	if !ok {
		t.Fatal("address descriptor missing")
	}
	got, err := descriptor.Retrieve(t.Context())
	if err != nil || len(got) != 1 || got[0].Key != core.InterfaceAddrKey("loop701", "10.7.1.1/24") || dumps != 1 {
		t.Fatalf("product hook missing or repeats dump: %v %v dumps=%d", got, err, dumps)
	}
	classified, err := vrrp.OwnedVirtualAddresses(t.Context(), v, "w7")
	if err != nil || !classified[idx]["10.7.1.254"] || classified[foreign]["10.7.1.254"] {
		t.Fatal("wrapper classifier adopted foreign VR or lost owned VIP", err)
	}
	v.On("vrrp_vr_dump", func(api.Message) ([]api.Message, error) { return nil, errors.New("readback unavailable") })
	if _, err := descriptor.Retrieve(t.Context()); err == nil {
		t.Fatal("wrapper reported addresses after failed VIP classification")
	}

}
