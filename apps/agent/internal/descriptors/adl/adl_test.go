package adl

import (
	"context"
	"errors"
	"testing"

	adlapi "ngfw/agent/binapi/adl"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

func newFake() *fake.Client {
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
	)
	f.Reply("adl_interface_enable_disable", &adlapi.AdlInterfaceEnableDisableReply{})
	f.Reply("adl_allowlist_enable_disable", &adlapi.AdlAllowlistEnableDisableReply{})
	return f
}

func TestInterface(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	d := NewInterface(f, "w3")
	desired := &Interface{Interface: "loop300"}
	if k := d.KeyOf(desired); k != "adl.interface/loop300" || !scheduler.ValidName(d.Name()) {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "interface/loop300" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (Meta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := f.CallsNamed("adl_interface_enable_disable")[0].(*adlapi.AdlInterfaceEnableDisable)
	if req.SwIfIndex != 5 || !req.EnableDisable {
		t.Fatalf("request = %+v", req)
	}
	if _, err := d.Update(ctx, desired, &Interface{Interface: "loop301"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	req = f.CallsNamed("adl_interface_enable_disable")[1].(*adlapi.AdlInterfaceEnableDisable)
	if req.SwIfIndex != 5 || req.EnableDisable {
		t.Fatalf("delete request = %+v", req)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve: %v", err)
	}
	if _, err := d.Create(ctx, &Interface{Interface: "nope"}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if err := d.Delete(ctx, desired, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
	f.Reply("adl_interface_enable_disable", &adlapi.AdlInterfaceEnableDisableReply{Retval: -2})
	if _, err := d.Create(ctx, desired); err == nil {
		t.Fatal("retval must surface")
	}
}

func TestAllowlist(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	d := NewAllowlist(f, "w3")
	desired := &Allowlist{Interface: "loop300", FibId: 3001, Ip4: true, DefaultAdl: true}
	if k := d.KeyOf(desired); k != "adl.allowlist/loop300" || !scheduler.ValidName(d.Name()) {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(desired)
	if len(deps) != 3 || deps[0].Key != "interface/loop300" || deps[1].Key != "adl.interface/loop300" || !deps[1].Optional || deps[2].Key != "vrf/3001" || deps[2].Optional {
		t.Fatalf("Dependencies = %+v", deps)
	}
	if deps := d.Dependencies(&Allowlist{Interface: "loop300", Ip6: true}); len(deps) != 2 {
		t.Fatalf("table 0: Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (Meta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := f.CallsNamed("adl_allowlist_enable_disable")[0].(*adlapi.AdlAllowlistEnableDisable)
	if req.SwIfIndex != 5 || req.FibID != 3001 || !req.IP4 || req.IP6 || !req.DefaultAdl {
		t.Fatalf("request = %+v", req)
	}
	if _, err := d.Update(ctx, desired, &Allowlist{Interface: "loop300", FibId: 3002, Ip4: true, Ip6: true}, meta); err != nil {
		t.Fatal(err)
	}
	req = f.CallsNamed("adl_allowlist_enable_disable")[1].(*adlapi.AdlAllowlistEnableDisable)
	if req.FibID != 3002 || !req.IP6 {
		t.Fatalf("update request = %+v", req)
	}
	if _, err := d.Update(ctx, desired, &Allowlist{Interface: "loop301", Ip4: true}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	req = f.CallsNamed("adl_allowlist_enable_disable")[2].(*adlapi.AdlAllowlistEnableDisable)
	if req.IP4 || req.IP6 || req.FibID != 3001 {
		t.Fatalf("delete request = %+v", req)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve: %v", err)
	}
	if _, err := d.Create(ctx, &Allowlist{Interface: "loop300", FibId: 3001}); err == nil {
		t.Fatal("neither ip4 nor ip6 accepted")
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3")
	if got := reg.Names(); len(got) != 2 || got[0] != InterfaceName || got[1] != AllowlistName {
		t.Fatalf("Names = %v", got)
	}
}
