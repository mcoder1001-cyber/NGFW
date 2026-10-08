package iface_test

import (
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	iface "ngfw/agent/internal/descriptors/interface"
)

func unnumberedWorld(t *testing.T) (*world, map[uint32]uint32) {
	t.Helper()
	w := newWorld()
	rel := map[uint32]uint32{}
	iface.SetClaimStore(owner, iface.NewMemoryClaimStore())
	w.v.On("ip_unnumbered_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for b, d := range rel {
			out = append(out, &ip.IPUnnumberedDetails{SwIfIndex: interface_types.InterfaceIndex(b), IPSwIfIndex: interface_types.InterfaceIndex(d)})
		}
		return out, nil
	})
	w.v.On("sw_interface_get_table", func(api.Message) ([]api.Message, error) { return []api.Message{&ifapi.SwInterfaceGetTableReply{}}, nil })
	w.v.On("sw_interface_set_unnumbered", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceSetUnnumbered)
		if r.IsAdd {
			rel[uint32(r.UnnumberedSwIfIndex)] = uint32(r.SwIfIndex)
		} else {
			delete(rel, uint32(r.UnnumberedSwIfIndex))
		}
		return []api.Message{&ifapi.SwInterfaceSetUnnumberedReply{}}, nil
	})
	return w, rel
}
func TestUnnumberedLifecycle(t *testing.T) {
	w, rel := unnumberedWorld(t)
	d := iface.NewUnnumbered(w.v, owner)
	o := &iface.Unnumbered{Interface: "interface/tap1", Donor: "interface/loop201"}
	m := mustCreate(t, d, o)
	if rel[w.untagged] != w.loop {
		t.Fatal("wrong borrower/donor API mapping")
	}
	// Fresh descriptor retrieves live state across an agent restart.
	d = iface.NewUnnumbered(w.v, owner)
	got := retrieve(t, d)
	if len(got) != 1 || !proto.Equal(got[0].Value, o) {
		t.Fatalf("retrieve=%v", got)
	}
	if err := d.Delete(ctx, o, m); err != nil {
		t.Fatal(err)
	}
	if len(rel) != 0 || len(retrieve(t, d)) != 0 {
		t.Fatal("delete left relationship")
	}
	mustCreate(t, d, o)
	delete(rel, w.untagged)
	if len(retrieve(t, d)) != 0 {
		t.Fatal("retrieve cached absent relationship")
	}
}
func TestUnnumberedOwnershipGuards(t *testing.T) {
	w, rel := unnumberedWorld(t)
	d := iface.NewUnnumbered(w.v, owner)
	o := &iface.Unnumbered{Interface: "interface/tap1", Donor: "interface/loop201"}
	rel[w.untagged] = w.tap
	if _, err := d.Create(ctx, o); err == nil {
		t.Fatal("claimed foreign existing relationship")
	}
	delete(rel, w.untagged)
	m := mustCreate(t, d, o)
	rel[w.untagged] = w.tap
	if err := d.Delete(ctx, o, m); err == nil {
		t.Fatal("removed changed donor")
	}
	if rel[w.untagged] != w.tap {
		t.Fatal("mutated changed donor")
	}
	rel[w.untagged] = w.loop
	w.v.Ifs[w.untagged].Tag = "other:nic"
	if err := d.Delete(ctx, o, m); err == nil {
		t.Fatal("removed foreign interface")
	}
}
func TestUnnumberedRejectsVRFAndNestedDonor(t *testing.T) {
	w, rel := unnumberedWorld(t)
	d := iface.NewUnnumbered(w.v, owner)
	o := &iface.Unnumbered{Interface: "interface/tap1", Donor: "interface/loop201"}
	rel[w.loop] = w.tap
	if _, err := d.Create(ctx, o); err == nil {
		t.Fatal("accepted nested donor")
	}
	delete(rel, w.loop)
	w.v.On("sw_interface_get_table", func(req api.Message) ([]api.Message, error) {
		r := req.(*ifapi.SwInterfaceGetTable)
		var id uint32
		if r.IsIPv6 && uint32(r.SwIfIndex) == w.loop {
			id = 8
		}
		return []api.Message{&ifapi.SwInterfaceGetTableReply{VrfID: id}}, nil
	})
	if _, err := d.Create(ctx, o); err == nil {
		t.Fatal("accepted cross IPv6 VRF")
	}
	if len(rel) != 0 {
		t.Fatal("wrote before validation")
	}
}
func TestUnnumberedFailedWriteReleasesClaim(t *testing.T) {
	w, _ := unnumberedWorld(t)
	d := iface.NewUnnumbered(w.v, owner)
	o := &iface.Unnumbered{Interface: "interface/tap1", Donor: "interface/loop201"}
	w.v.On("sw_interface_set_unnumbered", func(api.Message) ([]api.Message, error) { return nil, fmt.Errorf("rejected") })
	if _, err := d.Create(ctx, o); err == nil {
		t.Fatal("ignored failure")
	}
	if iface.Claims(owner).Claimed("tap1", iface.UnnumberedName) {
		t.Fatal("leaked claim")
	}
}

func TestUnnumberedRejectsConvertingLiveDonor(t *testing.T) {
	w, rel := unnumberedWorld(t)
	d := iface.NewUnnumbered(w.v, owner)
	rel[w.other] = w.untagged
	if _, err := d.Create(ctx, &iface.Unnumbered{Interface: "interface/tap1", Donor: "interface/loop201"}); err == nil {
		t.Fatal("converted donor of foreign live borrower")
	}
	if len(rel) != 1 || rel[w.other] != w.untagged {
		t.Fatal("modified live foreign relationship")
	}
}
