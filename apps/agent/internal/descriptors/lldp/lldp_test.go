package lldp

import (
	"errors"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/lldp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
)

// fakeLLDP models lldp_dump; hwOf maps the sw_if_index VPP receives to the interface it
// actually enables (identity unless a test simulates the sw/hw mismatch).
func fakeLLDP(hwOf map[uint32]uint32) (*df7test.Fake, map[uint32]bool) {
	f := df7test.NewFake()
	on := map[uint32]bool{}
	f.On("sw_interface_set_lldp", func(m api.Message) ([]api.Message, error) {
		r := m.(*lldp.SwInterfaceSetLldp)
		target := uint32(r.SwIfIndex)
		if t, ok := hwOf[target]; ok {
			target = t
		}
		if r.Enable {
			on[target] = true
		} else {
			delete(on, target)
		}
		return []api.Message{&lldp.SwInterfaceSetLldpReply{}}, nil
	})
	f.On("lldp_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := range on {
			out = append(out, &lldp.LldpDetails{SwIfIndex: interface_types.InterfaceIndex(idx), ChassisID: make([]byte, 64), PortID: make([]byte, 64), TTL: 120})
		}
		return append(out, &lldp.LldpDumpReply{}), nil
	})
	f.Reply("lldp_config", &lldp.LldpConfigReply{})
	return f, on
}

func TestGlobal(t *testing.T) {
	f, _ := fakeLLDP(nil)
	d := NewGlobal(f, df7test.Owner)
	v := df7.Encode(Global{SystemName: "w0-vrx", TxHold: 5, TxInterval: 20})
	if d.KeyOf(v) != "lldp.global/global" || d.Dependencies(v) != nil {
		t.Fatal("key/deps")
	}
	if _, err := d.Create(t.Context(), v); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*lldp.LldpConfig](t, f, "lldp_config"); r.SystemName != "w0-vrx" || r.TxHold != 5 || r.TxInterval != 20 {
		t.Fatalf("%+v", r)
	}
	if _, err := d.Update(t.Context(), v, df7.Encode(Global{TxHold: 6}), nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(t.Context(), v, nil); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*lldp.LldpConfig](t, f, "lldp_config"); r.TxHold != DefaultTxHold || r.TxInterval != DefaultTxInterval || r.SystemName != "" {
		t.Fatalf("delete restores defaults: %+v", r)
	}
	if _, err := d.Retrieve(t.Context()); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	if _, err := d.Create(t.Context(), df7.Encode(Global{TxHold: 101})); !errors.Is(err, df7.ErrSpec) {
		t.Fatal(err)
	}
}

func TestInterface(t *testing.T) {
	f, on := fakeLLDP(nil)
	ctx := t.Context()
	d := NewInterface(f, df7test.Owner)
	v := df7.Encode(Interface{Interface: "loop0", PortDesc: "uplink", MgmtIP4: "10.0.0.1", MgmtIP6: "2001:db8::1", MgmtOID: "1.3.6"})
	if d.KeyOf(v) != "lldp.interface/loop0" {
		t.Fatal(d.KeyOf(v))
	}
	deps := d.Dependencies(v)
	if len(deps) != 2 || deps[0].Key != "lldp.global/global" || !deps[0].Optional || deps[1].Key != "interface/loop0" {
		t.Fatalf("deps %v", deps)
	}
	meta, err := d.Create(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	r := df7test.Last[*lldp.SwInterfaceSetLldp](t, f, "sw_interface_set_lldp")
	if r.SwIfIndex != 1 || !r.Enable || r.PortDesc != "uplink" || r.MgmtIP4 != [4]uint8{10, 0, 0, 1} || r.MgmtIP6[15] != 1 || string(r.MgmtOid[:5]) != "1.3.6" || len(r.MgmtOid) != 128 {
		t.Fatalf("%+v", r)
	}
	if !on[1] {
		t.Fatal("not enabled")
	}
	f.Reset()
	v2 := df7.Encode(Interface{Interface: "loop0", PortDesc: "uplink2"})
	if _, err := d.Update(ctx, v, v2, meta); err != nil {
		t.Fatal(err)
	}
	calls := f.CallsNamed("sw_interface_set_lldp")
	if len(calls) != 2 || calls[0].(*lldp.SwInterfaceSetLldp).Enable || !calls[1].(*lldp.SwInterfaceSetLldp).Enable {
		t.Fatalf("update = disable + enable: %v", calls)
	}
	if _, err := d.Update(ctx, v, df7.Encode(Interface{Interface: "loop1"}), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, v2, meta); err != nil || on[1] {
		t.Fatalf("delete: %v %v", err, on)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	for i, bad := range []Interface{{}, {Interface: "x", MgmtIP4: "2001:db8::1"}, {Interface: "x", MgmtIP6: "10.0.0.1"}, {Interface: "x", MgmtIP4: "10.0.0.01"}} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}

	// VPP enables another interface (sw_if_index used as hw_if_index): Create reports it
	f2, _ := fakeLLDP(map[uint32]uint32{2: 7})
	_, err = NewInterface(f2, df7test.Owner).Create(ctx, df7.Encode(Interface{Interface: "loop1"}))
	if !errors.Is(err, ErrIndexMismatch) {
		t.Fatalf("mismatch: %v", err)
	}
	t.Log(err)
}

func TestNeighbours(t *testing.T) {
	f := df7test.NewFake()
	pages := 0
	f.On("lldp_dump", func(m api.Message) ([]api.Message, error) {
		pages++
		if m.(*lldp.LldpDump).Cursor == 0 {
			return []api.Message{&lldp.LldpDetails{SwIfIndex: 1, ChassisID: []byte("abc"), ChassisIDLen: 3, PortID: []byte("p1"), PortIDLen: 2},
				&lldp.LldpDumpReply{Retval: int32(api.EAGAIN), Cursor: 1}}, nil
		}
		return []api.Message{&lldp.LldpDetails{SwIfIndex: 2}, &lldp.LldpDumpReply{}}, nil
	})
	n, err := Neighbours(t.Context(), f)
	if err != nil || len(n) != 2 || string(n[1].ChassisID) != "abc" || string(n[1].PortID) != "p1" || pages != 2 {
		t.Fatalf("%v %v pages %d", n, err, pages)
	}
	r := scheduler.NewRegistry()
	Register(r, f, df7test.Owner)
	if r.Len() != 1 || r.Names()[0] != NameInterface {
		t.Fatal("non-owners register no globals (D-071):", r.Names())
	}
	RegisterGlobals(r, f, df7test.Owner)
	if r.Len() != 2 {
		t.Fatal(r.Names())
	}
}

// Review M6/M1: an enable that lands on another interface (VPP's sw/hw index mix-up) fails
// loudly, claims nothing and sends no disable: VPP's disable would resolve to yet another
// interface's entry (lldp_cli.c lldp_cfg_intf_set). An interface already enabled but not ours is
// not adopted.
func TestInterfaceMismatchUndo(t *testing.T) {
	ctx := t.Context()
	f, on := fakeLLDP(map[uint32]uint32{4: 7})
	d := NewInterface(f, df7test.Owner)
	v := df7.Encode(Interface{Interface: "eth0"})
	on[9] = true // another consumer's LLDP, reachable by a wrongly keyed disable
	_, err := d.Create(ctx, v)
	if !errors.Is(err, ErrIndexMismatch) || !strings.Contains(err.Error(), "NOT undone") {
		t.Fatalf("mismatch: %v", err)
	}
	for _, c := range f.CallsNamed("sw_interface_set_lldp") {
		if !c.(*lldp.SwInterfaceSetLldp).Enable {
			t.Fatal("no disable may be sent after a mismatch")
		}
	}
	if !on[9] || !on[7] {
		t.Fatalf("state %v", on)
	}
	if df7test.Claimed(ctx, f, df7test.Owner, "eth0", string(d.KeyOf(v))) {
		t.Fatal("a failed enable must not claim")
	}
	f.Reset()
	if err := d.Delete(ctx, v, nil); err != nil || len(f.CallsNamed("sw_interface_set_lldp")) != 0 {
		t.Fatal("Delete of an unclaimed interface sends nothing", err)
	}
	g, gon := fakeLLDP(nil)
	gd := NewInterface(g, df7test.Owner)
	gon[4] = true // enabled by someone else
	if _, err := gd.Create(ctx, v); !errors.Is(err, dfkit.ErrNotOurs) {
		t.Fatalf("adopted: %v", err)
	}
	if err := gd.Delete(ctx, v, nil); err != nil || !gon[4] {
		t.Fatal("a foreign LLDP enable must not be disabled", err)
	}
	delete(gon, 4)
	if _, err := gd.Create(ctx, v); err != nil || !gon[4] || !df7test.Claimed(ctx, g, df7test.Owner, "eth0", string(gd.KeyOf(v))) {
		t.Fatal(err, gon)
	}
	if err := gd.Delete(ctx, v, nil); err != nil || gon[4] {
		t.Fatal(err, gon)
	}
}
