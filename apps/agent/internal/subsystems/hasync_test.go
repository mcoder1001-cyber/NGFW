package subsystems

import (
	"context"
	"ngfw/agent/binapi/ip_types"
	nat "ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/hasync"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
	"testing"
)

func TestHaSyncRegisteredRequireOnly(t *testing.T) {
	f := fake.New()
	f.Reply("nat44_ei_ha_get_listener", &nat.Nat44EiHaGetListenerReply{IPAddress: ip_types.IP4Address{192, 0, 2, 1}, Port: 8750, PathMtu: 1500})
	r := scheduler.NewRegistry()
	registerHaSync(r, &Wiring{env: Env{Client: f, GlobalsOwner: false}})
	d, ok := r.Get(hasync.NameListener)
	if !ok {
		t.Fatal("listener missing")
	}
	if _, err := d.Create(context.Background(), dfkit.Encode(hasync.Listener{Address: "192.0.2.1", Port: 8750, PathMtu: 1500})); err != nil {
		t.Fatal(err)
	}
	if len(f.CallsNamed("nat44_ei_ha_set_listener")) != 0 {
		t.Fatal("slot set HA globals")
	}
	if _, ok := r.Get(hasync.NameFailover); !ok {
		t.Fatal("peer missing")
	}
	for _, name := range []string{hasync.NameListener, hasync.NameFailover} {
		if DomainOf(name) != HA {
			t.Fatalf("domain %s", name)
		}
	}
}
