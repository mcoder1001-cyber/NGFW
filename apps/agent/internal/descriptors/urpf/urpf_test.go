package urpf

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	urpfapi "ngfw/agent/binapi/urpf"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type cfgKey struct {
	idx   uint32
	af    ip_types.AddressFamily
	input bool
}

type fakeVPP struct {
	*fake.Client
	cfg map[cfgKey]*urpfapi.UrpfUpdateV2
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), cfg: map[cfgKey]*urpfapi.UrpfUpdateV2{}}
	v.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
	)
	v.On("urpf_update_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*urpfapi.UrpfUpdateV2)
		if r.TableID >= 4000 {
			return []api.Message{&urpfapi.UrpfUpdateV2Reply{Retval: -12}}, nil // NO_SUCH_FIB
		}
		k := cfgKey{uint32(r.SwIfIndex), r.Af, r.IsInput}
		if r.Mode == urpfapi.URPF_API_MODE_OFF {
			delete(v.cfg, k)
		} else {
			cp := *r
			v.cfg[k] = &cp
		}
		return []api.Message{&urpfapi.UrpfUpdateV2Reply{}}, nil
	})
	v.On("urpf_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, c := range v.cfg {
			out = append(out, &urpfapi.UrpfInterfaceDetails{SwIfIndex: c.SwIfIndex, IsInput: c.IsInput, Mode: c.Mode, Af: c.Af, TableID: c.TableID})
		}
		return out, nil
	})
	return v
}

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	v.cfg[cfgKey{7, ip_types.ADDRESS_IP4, true}] = &urpfapi.UrpfUpdateV2{IsInput: true, Mode: urpfapi.URPF_API_MODE_LOOSE, SwIfIndex: 7}
	d := New(v, "w3")
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	desired := &Interface{Interface: "loop300", Af: df2.AddressFamily_IPV4, Direction: Interface_RX, Mode: Interface_STRICT}
	if k := d.KeyOf(desired); k != "urpf.interface/loop300/ipv4/rx" {
		t.Fatalf("KeyOf = %s", k)
	}
	if k := d.KeyOf(&Interface{Interface: "loop300", Af: df2.AddressFamily_IPV6, Direction: Interface_TX}); k != "urpf.interface/loop300/ipv6/tx" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "interface/loop300" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	withTable := &Interface{Interface: "loop300", Af: df2.AddressFamily_IPV6, Direction: Interface_TX, Mode: Interface_LOOSE, TableId: 3001}
	if deps := d.Dependencies(withTable); len(deps) != 2 || deps[1].Key != "vrf/3001" {
		t.Fatalf("Dependencies = %+v", deps)
	}

	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (Meta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("urpf_update_v2")[0].(*urpfapi.UrpfUpdateV2)
	if !req.IsInput || req.Mode != urpfapi.URPF_API_MODE_STRICT || req.Af != ip_types.ADDRESS_IP4 || req.SwIfIndex != 5 || req.TableID != 0 {
		t.Fatalf("request = %+v", req)
	}
	if _, err := d.Create(ctx, withTable); err != nil {
		t.Fatal(err)
	}
	req = v.CallsNamed("urpf_update_v2")[1].(*urpfapi.UrpfUpdateV2)
	if req.IsInput || req.Mode != urpfapi.URPF_API_MODE_LOOSE || req.Af != ip_types.ADDRESS_IP6 || req.TableID != 3001 {
		t.Fatalf("request = %+v", req)
	}

	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 2 {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	for _, want := range []*Interface{desired, withTable} {
		var got *scheduler.KV
		for i := range actual {
			if actual[i].Key == d.KeyOf(want) {
				got = &actual[i]
			}
		}
		if got == nil || !proto.Equal(got.Value, want) || got.Meta != (Meta{SwIfIndex: 5}) {
			t.Fatalf("Retrieve[%s] = %+v, want %+v", d.KeyOf(want), got, want)
		}
	}

	// Update in place: mode and table; key change → recreate.
	if _, err := d.Update(ctx, desired, &Interface{Interface: "loop300", Af: df2.AddressFamily_IPV4, Mode: Interface_LOOSE, TableId: 3002}, meta); err != nil {
		t.Fatal(err)
	}
	req = v.CallsNamed("urpf_update_v2")[2].(*urpfapi.UrpfUpdateV2)
	if req.Mode != urpfapi.URPF_API_MODE_LOOSE || req.TableID != 3002 {
		t.Fatalf("update request = %+v", req)
	}
	if _, err := d.Update(ctx, desired, &Interface{Interface: "loop300", Af: df2.AddressFamily_IPV4, Direction: Interface_TX, Mode: Interface_LOOSE}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("direction change: %v", err)
	}
	if _, err := d.Update(ctx, desired, &Interface{Interface: "loop300", Af: df2.AddressFamily_IPV4, Mode: Interface_OFF}, meta); err == nil {
		t.Fatal("mode OFF accepted as desired state")
	}

	// Delete sends OFF; Retrieve then shows nothing of ours; the foreign check stays.
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, withTable, Meta{SwIfIndex: 5}); err != nil {
		t.Fatal(err)
	}
	last := v.CallsNamed("urpf_update_v2")[4].(*urpfapi.UrpfUpdateV2)
	if last.Mode != urpfapi.URPF_API_MODE_OFF || last.TableID != 3001 {
		t.Fatalf("delete request = %+v", last)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || len(v.cfg) != 1 {
		t.Fatalf("after Delete: %+v cfg=%d", actual, len(v.cfg))
	}

	// Errors.
	if _, err := d.Create(ctx, &Interface{Interface: "loop300", Mode: Interface_OFF}); err == nil {
		t.Fatal("OFF accepted")
	}
	if _, err := d.Create(ctx, &Interface{Interface: "nope", Mode: Interface_LOOSE}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if _, err := d.Create(ctx, &Interface{Interface: "loop300", Mode: Interface_LOOSE, TableId: 4001}); err == nil {
		t.Fatal("VPP retval must surface")
	}
	if err := d.Delete(ctx, desired, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3")
	if got := reg.Names(); len(got) != 1 || got[0] != Name {
		t.Fatalf("Names = %v", got)
	}
}
