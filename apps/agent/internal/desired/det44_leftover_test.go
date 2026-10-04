package desired

import (
	"testing"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// F-det44-cnat-fix: a det44.interface "<if>#leftover/<side>" object (leftover arc node) never reaches nat.det44.
func TestAssembleDet44SkipsLeftovers(t *testing.T) {
	kv := func(iface, side string) scheduler.KV {
		s := det44.InterfaceSpec{Interface: iface, Side: side}
		return scheduler.KV{Key: scheduler.Join(det44.NameInterface, iface+"/"+side), Value: natcommon.MustEncode(&s)}
	}
	out := &ngfwv1.NatConfig{}
	assembleDet44(out, []scheduler.KV{kv("host-w16l0"+det44.LeftoverSuffix, det44.SideInside)})
	if out.Det44 != nil {
		t.Fatalf("leftover alone projected: %v", out.Det44)
	}
	assembleDet44(out, []scheduler.KV{kv("host-w16l0", det44.SideInside), kv("host-w16w0"+det44.LeftoverSuffix, det44.SideOutside)})
	if d := out.GetDet44(); len(d.GetInside()) != 1 || d.GetInside()[0] != "host-w16l0" || len(d.GetOutside()) != 0 {
		t.Fatalf("det44 = %v", d)
	}
}
