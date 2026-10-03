package vrrp_test

import (
	"context"
	"fmt"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	vrrpapi "ngfw/agent/binapi/vrrp"
	"ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

// A private VPP is mandatory: F4 intentionally leaves a VR whose interface vanished.
func TestVRRPProductFixesOnDisposableVPP(t *testing.T) {
	if os.Getenv("NGFW_DISPOSABLE_VPP") != "1" || os.Getenv("NGFW_VRRP_VPP") != "on" {
		t.Skip("disposable VPP and explicit VRRP window required")
	}
	h := df7test.StartHost(t)
	env := core.Env{Client: h.C, Owner: h.Owner, Owned: ownertable.NewMemory()}
	loop := &core.LoopbackDescriptor{Env: env}
	name := fmt.Sprintf("loop%d70", h.Slot)
	object := &core.Loopback{Name: name, Instance: uint32(h.Slot*100 + 70)}
	meta, err := loop.Create(h.Ctx, object)
	if err != nil {
		t.Fatal(err)
	}
	idx := meta.(core.IfMeta).SwIfIndex
	t.Cleanup(func() {
		if err := loop.Delete(context.Background(), object, meta); err != nil {
			t.Error(err)
		}
	})
	if _, err := interfaces.NewServiceClient(h.C).SwInterfaceSetFlags(h.Ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP}); err != nil {
		t.Fatal(err)
	}
	addr := &core.InterfaceAddrDescriptor{Env: env}
	addr.SetVirtualAddressSource(vrrp.OwnedVirtualAddresses)
	primary := &core.InterfaceAddress{Interface: name, Prefix: h.Addr(70, 1) + "/24"}
	if _, err := addr.Create(h.Ctx, primary); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = addr.Delete(context.Background(), primary, nil) })
	vd := vrrp.NewVR(h.C, h.Owner)
	sd := vrrp.NewState(h.C, h.Owner)
	spec := vrrp.VRSpec{VR: vrrp.VR{Interface: name, VRID: uint8(h.Slot)}, Priority: 100, Interval: 10, Accept: true, Preempt: true, Addresses: []string{h.Addr(70, 253), h.Addr(70, 254)}}
	vrValue := df7.Encode(spec)
	vrMeta, err := vd.Create(h.Ctx, vrValue)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vd.Delete(context.Background(), vrValue, vrMeta) })
	state := df7.Encode(vrrp.State{VR: spec.VR, Running: true})
	if _, err := sd.Create(h.Ctx, state); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sd.Delete(context.Background(), state, nil) })
	deadline := time.Now().Add(5 * time.Second)
	for {
		virtual, err := vrrp.OwnedVirtualAddresses(h.Ctx, h.C, h.Owner)
		if err != nil {
			t.Fatal(err)
		}
		if virtual[idx][spec.Addresses[0]] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("accept-mode VR did not become Master")
		}
		time.Sleep(50 * time.Millisecond)
	}
	show := func() {
		out, err := exec.Command("timeout", "10", "vppctl", "show", "vrrp", "vr").CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("show vrrp vr:\n%s", out)
	}
	show()
	rawAddresses, err := ip.NewServiceClient(h.C).IPAddressDump(h.Ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatal(err)
	}
	dumpedAddresses, err := df7.Collect(rawAddresses.Recv)
	if err != nil || len(dumpedAddresses) != 3 {
		t.Fatalf("Master did not install runtime VIP addresses: %v %v", dumpedAddresses, err)
	}
	t.Logf("F1 raw interface addresses before filtering: %v", dumpedAddresses)
	got, err := addr.Retrieve(h.Ctx)
	if err != nil || len(got) != 1 || got[0].Key != addr.KeyOf(primary) {
		t.Fatalf("Master VIP leaked into desired addresses: %v %v", got, err)
	}
	reg := scheduler.NewRegistry()
	reg.Register(addr)
	plan, err := scheduler.New(reg, nil).Plan(h.Ctx, []scheduler.KV{{Key: addr.KeyOf(primary), Value: primary}}, nil)
	if err != nil || len(plan.Ops) != 0 {
		t.Fatalf("unchanged commit touches Master VIP: %v %v", plan, err)
	}
	t.Log("F1: accept-mode Master VIPs excluded; unchanged commit has zero operations")
	live, err := vd.Retrieve(h.Ctx)
	if err != nil || len(live) != 1 {
		t.Fatalf("VR dump: %v %v", live, err)
	}
	// Use deliberately opposite presentation order to the live dump.
	actual, err := df7.Decode[vrrp.VRSpec](live[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	ordered := append([]string(nil), actual.Addresses...)
	ordered[0], ordered[1] = ordered[1], ordered[0]
	id := fmt.Sprintf("%s/%d/ipv4", name, h.Slot)
	live = append(live, scheduler.KV{Key: desired.VrrpMetaKey(id), Value: dfkit.Encode(desired.VrrpMetaSpec{ID: id, Name: "site", Addresses: ordered})})
	ds := &ngfwv1.DesiredState{}
	desired.AssembleVrrp(ds, live, map[string]bool{"ha": true})
	if !reflect.DeepEqual(ds.GetHa().GetVrrp()["site"].GetAddresses(), ordered) {
		t.Fatalf("VIP order changed: %v", ds)
	}
	t.Logf("F2: live VIP order %v restored presentation order %v", actual.Addresses, ordered)
	commitRegistry := scheduler.NewRegistry()
	commitRegistry.Register(loop)
	commitRegistry.Register(iface.NewAlias(h.C, h.Owner))
	commitRegistry.Register(addr)
	commitRegistry.Register(vd)
	commitRegistry.Register(sd)
	result := scheduler.New(commitRegistry, nil).Apply(h.Ctx, []scheduler.KV{
		{Key: loop.KeyOf(object), Value: object},
		{Key: addr.KeyOf(primary), Value: primary}, {Key: vd.KeyOf(vrValue), Value: vrValue},
	}, nil)
	if result.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("disabled Master commit failed: %v", result)
	}
	if running, err := sd.Retrieve(h.Ctx); err != nil || len(running) != 0 {
		t.Fatalf("disabled VR still running: %v %v", running, err)
	}
	t.Log("F1: enabled:false transaction APPLIED and VR stopped")
	if err := addr.Delete(h.Ctx, primary, nil); err != nil {
		t.Fatal(err)
	}
	if err := loop.Delete(h.Ctx, object, meta); err != nil {
		t.Fatal(err)
	}
	if err := vd.Delete(h.Ctx, vrValue, vrMeta); err != nil {
		t.Fatalf("vanished-interface delete: %v", err)
	}
	if got, err := vd.Retrieve(h.Ctx); err != nil || len(got) != 0 {
		t.Fatalf("orphan VR scheduled repeatedly: %v %v", got, err)
	}
	show()
	stream, err := vrrpapi.NewServiceClient(h.C).VrrpVrDump(h.Ctx, &vrrpapi.VrrpVrDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := df7.Collect(stream.Recv)
	if err != nil || len(raw) != 1 {
		t.Fatalf("expected documented VPP residue: %v %v", raw, err)
	}
	t.Log("F4: deleting vanished-interface VR succeeds; Retrieve omits residue; private VPP disposal removes residue")
}
