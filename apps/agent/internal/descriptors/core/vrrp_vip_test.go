package core_test

import (
	"context"
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

func TestVRRPMasterVIPIsRuntimeOwned(t *testing.T) {
	v := coretest.New()
	idx := v.AddInterface("loop701", "Loopback", "w7:loop701")
	foreign := v.AddInterface("loop702", "Loopback", "w8:loop702")
	address, _ := ip_types.ParseAddress("10.7.1.254")
	state := vrrpapi.VRRP_API_VR_STATE_MASTER
	v.On("vrrp_vr_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{
			&vrrpapi.VrrpVrDetails{Config: vrrpapi.VrrpVrConf{SwIfIndex: interface_types.InterfaceIndex(idx), VrID: 7, Flags: vrrpapi.VRRP_API_VR_ACCEPT}, Runtime: vrrpapi.VrrpVrRuntime{State: state}, Addrs: []ip_types.Address{address}},
			&vrrpapi.VrrpVrDetails{Config: vrrpapi.VrrpVrConf{SwIfIndex: interface_types.InterfaceIndex(foreign), VrID: 8, Flags: vrrpapi.VRRP_API_VR_ACCEPT}, Runtime: vrrpapi.VrrpVrRuntime{State: vrrpapi.VRRP_API_VR_STATE_MASTER}, Addrs: []ip_types.Address{address}},
		}, nil
	})
	d := &core.InterfaceAddrDescriptor{Env: core.Env{Client: v, Owner: "w7", Owned: ownertable.NewMemory()}}
	d.SetVirtualAddressSource(vrrp.OwnedVirtualAddresses)
	v.Ifaces[idx].Addrs["10.7.1.1/24"] = true
	v.Ifaces[idx].Addrs["10.7.1.254/24"] = true
	want := []scheduler.KV{{Key: core.InterfaceAddrKey("loop701", "10.7.1.1/24"), Value: &core.InterfaceAddress{Interface: "loop701", Prefix: "10.7.1.1/24"}}}
	virtual, err := vrrp.OwnedVirtualAddresses(context.Background(), v, "w7")
	if err != nil || virtual[foreign]["10.7.1.254"] {
		t.Fatalf("foreign VR classified as ours: %v %v", virtual, err)
	}
	got, err := d.Retrieve(context.Background())
	if err != nil || len(got) != 1 || got[0].Key != want[0].Key {
		t.Fatalf("Master VIP leaked: %v %v", got, err)
	}
	reg := scheduler.NewRegistry()
	reg.Register(d)
	plan, err := scheduler.New(reg, nil).Plan(context.Background(), want, nil)
	if err != nil || len(plan.Ops) != 0 {
		t.Fatalf("unchanged commit deletes VIP: %v %v", plan, err)
	}
	state = vrrpapi.VRRP_API_VR_STATE_BACKUP
	got, err = d.Retrieve(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("Backup hides configured address: %v %v", got, err)
	}
	v.On("vrrp_vr_dump", func(api.Message) ([]api.Message, error) { return nil, errors.New("dump failed") })
	if _, err := d.Retrieve(context.Background()); err == nil {
		t.Fatal("unclassified address reported after failed VIP dump")
	}
}
