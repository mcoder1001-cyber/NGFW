package iface_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

func TestAlias(t *testing.T) {
	w := newWorld()
	w.v.Ifs[w.untagged].InterfaceName = "ens161" // a physical NIC no descriptor created
	d := iface.NewAlias(w.v, owner)

	ours := &iface.InterfaceAlias{Name: "w2-tap0", Creator: tapKey}
	nic := &iface.InterfaceAlias{Name: "ens161"}
	if k := d.KeyOf(ours); k != "interface/w2-tap0" || k != iface.AliasKey("w2-tap0") {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(ours); len(deps) != 1 || deps[0].Key != tapKey || deps[0].Optional {
		t.Fatalf("creator dependency = %+v", deps)
	}
	if deps := d.Dependencies(nic); len(deps) != 0 {
		t.Fatalf("physical interface has no dependency: %+v", deps)
	}

	// Retrieve: every interface but local0; ours by stable name + creator, others by VPP name
	assertOnly(t, d, map[scheduler.Key]proto.Message{
		"interface/loop201": &iface.InterfaceAlias{Name: "loop201", Creator: loopKey},
		"interface/w2-tap0": ours,
		"interface/gre0":    &iface.InterfaceAlias{Name: "gre0"}, // ours, device class nobody claimed
		"interface/loop300": &iface.InterfaceAlias{Name: "loop300"},
		"interface/ens161":  nic,
	}, map[scheduler.Key]any{"interface/w2-tap0": iface.Meta{SwIfIndex: w.tap}, "interface/ens161": iface.Meta{SwIfIndex: w.untagged}})

	// Create verifies and creates nothing; the Meta is the sw_if_index
	calls := len(w.v.Calls())
	if m := mustCreate(t, d, ours); m != (iface.Meta{SwIfIndex: w.tap}) {
		t.Fatalf("meta = %+v", m)
	}
	if m := mustCreate(t, d, nic); m != (iface.Meta{SwIfIndex: w.untagged}) {
		t.Fatalf("nic meta = %+v", m)
	}
	for _, c := range w.v.Calls()[calls:] {
		if c.GetMessageName() != "sw_interface_dump" && c.GetMessageName() != "control_ping" {
			t.Fatalf("alias Create sent %s", c.GetMessageName())
		}
	}
	// by name without creator: our tag id first
	if m := mustCreate(t, d, &iface.InterfaceAlias{Name: "loop201"}); m != (iface.Meta{SwIfIndex: w.loop}) {
		t.Fatalf("by name = %+v", m)
	}
	// errors: missing interface, local0, creator/name mismatch, wrong kind, bad ref
	for name, bad := range map[string]*iface.InterfaceAlias{
		"missing":  {Name: "ens999"},
		"local0":   {Name: "local0"},
		"mismatch": {Name: "w2-tap9", Creator: tapKey},
		"kind":     {Name: "w2-tap0", Creator: "memif.memif/w2-tap0"},
		"bad ref":  {Name: "w2-tap0", Creator: "w2-tap0"},
		"empty":    {},
	} {
		if _, err := d.Create(ctx, bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := d.Create(ctx, &iface.InterfaceAlias{Name: "ens999"}); !errors.Is(err, iface.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := d.Update(ctx, nic, ours, iface.Meta{SwIfIndex: w.untagged}); err != nil {
		t.Fatalf("Update re-verifies: %v", err)
	}

	// Delete is a no-op: nothing sent, the (foreign) interface stays
	calls = len(w.v.Calls())
	if err := d.Delete(ctx, &iface.InterfaceAlias{Name: "loop300"}, iface.Meta{SwIfIndex: w.other}); err != nil {
		t.Fatal(err)
	}
	if len(w.v.Calls()) != calls {
		t.Fatal("alias Delete talked to VPP")
	}
	if _, ok := w.v.Get(w.other); !ok {
		t.Fatal("foreign interface removed")
	}
	w.v.SetConnected(false)
	if _, err := d.Create(ctx, nic); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
}
