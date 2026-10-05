package subsystems

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/tapv2"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
	"testing"
)

type raMemoryDescriptor struct {
	name   string
	rows   map[scheduler.Key]scheduler.KV
	fail   bool
	writes int
}

func (d *raMemoryDescriptor) Name() string { return d.name }
func (d *raMemoryDescriptor) KeyOf(v proto.Message) scheduler.Key {
	if route, ok := v.(*core.Route); ok {
		return scheduler.Join(d.name, core.RouteKey(route.TableId, route.Prefix).ID())
	}
	tap, _ := v.(*tapv2.Tap)
	return scheduler.Join(d.name, tap.Name)
}
func (d *raMemoryDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (d *raMemoryDescriptor) Create(_ context.Context, v proto.Message) (any, error) {
	d.writes++
	k := d.KeyOf(v)
	d.rows[k] = scheduler.KV{Key: k, Value: proto.Clone(v)}
	return nil, nil
}
func (d *raMemoryDescriptor) Update(ctx context.Context, _, v proto.Message, _ any) (any, error) {
	return d.Create(ctx, v)
}
func (d *raMemoryDescriptor) Delete(_ context.Context, v proto.Message, _ any) error {
	d.writes++
	delete(d.rows, d.KeyOf(v))
	return nil
}
func (d *raMemoryDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	if d.fail {
		return nil, errors.New("stale boot")
	}
	out := []scheduler.KV{}
	for _, v := range d.rows {
		out = append(out, v)
	}
	return out, nil
}
func (d *raMemoryDescriptor) RecordsNoOwnership() {}
func TestRAScopedOwnershipNormalApplyAndRollbackRemainDisjoint(t *testing.T) {
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	rr := &raFilteringRegistry{Registry: reg, byName: map[string]scheduler.Descriptor{}}
	raw := &raMemoryDescriptor{name: core.RouteName, rows: map[scheduler.Key]scheduler.KV{}}
	rr.Register(raw)
	name := ravpn.LinkName(ravpn.InstanceID("w19", "road"), false)
	tap := &tapv2.Tap{Name: name}
	key := scheduler.Join(tapv2.TapName, name)
	taps := &raMemoryDescriptor{name: tapv2.TapName, rows: map[scheduler.Key]scheduler.KV{key: {Key: key, Value: tap, Meta: ravpn.TAPReceipt{Instance: ravpn.InstanceID("w19", "road"), NamespaceInode: 101, HostNamespaceInode: 102, Boot: bootid.Identity{BootID: "test", PID: 3, StartTime: 4}, Endpoint: proto.Clone(tap).(*tapv2.Tap)}}}}
	rr.Register(taps)
	normal, _ := reg.Get(core.RouteName)
	private, _ := reg.Get("remote-access." + core.RouteName)
	lan := &core.Route{TableId: 7, Prefix: "192.0.2.0/24", Paths: []*core.RoutePath{{Interface: "loop7", Weight: 1}}}
	road := &core.Route{TableId: 8, Prefix: "10.19.0.0/24", Paths: []*core.RoutePath{{Interface: name, Address: "198.18.19.3", Weight: 1}}}
	if _, e := normal.Create(ctx, lan); e != nil {
		t.Fatal(e)
	}
	if _, e := private.Create(ctx, road); e != nil {
		t.Fatal(e)
	}
	n, e := normal.Retrieve(ctx)
	if e != nil || len(n) != 1 || !proto.Equal(n[0].Value, lan) {
		t.Fatal("normal authority includes private route")
	}
	p, e := private.Retrieve(ctx)
	if e != nil || len(p) != 1 || !proto.Equal(p[0].Value, road) {
		t.Fatal("private authority includes ordinary route")
	}
	if _, e := normal.Update(ctx, lan, lan, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := private.Retrieve(ctx); e != nil {
		t.Fatal("normal apply damaged RA")
	}
	colliding := proto.Clone(road).(*core.Route)
	colliding.Paths[0].Interface = "loop8"
	if _, e := normal.Create(ctx, colliding); e == nil {
		t.Fatal("normal route overwrote private prefix")
	}
	if _, e := normal.Create(ctx, road); e == nil {
		t.Fatal("ordinary desired adopted reserved RA path")
	}
	if e := private.Delete(ctx, road, nil); e != nil {
		t.Fatal(e)
	}
	n, e = normal.Retrieve(ctx)
	if e != nil || len(n) != 1 || !proto.Equal(n[0].Value, lan) {
		t.Fatal("RA rollback deleted ordinary route")
	}
	if len(raw.rows) != 1 {
		t.Fatal("private leftover")
	}
}
func TestRAScopedForeignReservedNameAndStaleBootFailBeforeMutation(t *testing.T) {
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	rr := &raFilteringRegistry{Registry: reg, byName: map[string]scheduler.Descriptor{}}
	raw := &raMemoryDescriptor{name: core.RouteName, rows: map[scheduler.Key]scheduler.KV{}}
	rr.Register(raw)
	name := ravpn.LinkName(ravpn.InstanceID("w19", "foreign"), false)
	tap := &tapv2.Tap{Name: name}
	key := scheduler.Join(tapv2.TapName, name)
	taps := &raMemoryDescriptor{name: tapv2.TapName, rows: map[scheduler.Key]scheduler.KV{key: {Key: key, Value: tap}}}
	rr.Register(taps)
	private, _ := reg.Get("remote-access." + core.RouteName)
	route := &core.Route{Prefix: "10.19.0.0/24", Paths: []*core.RoutePath{{Interface: name, Weight: 1}}}
	if _, e := private.Create(ctx, route); e == nil || raw.writes != 0 {
		t.Fatal("foreign name adopted")
	}
	taps.rows[key] = scheduler.KV{Key: key, Value: tap, Meta: ravpn.TAPReceipt{Endpoint: proto.Clone(tap).(*tapv2.Tap)}}
	taps.fail = true
	if _, e := private.Create(ctx, route); e == nil || raw.writes != 0 {
		t.Fatal("stale boot adopted")
	}
}

func TestRAEnvironmentTwoOwnersAndCloseAreIsolated(t *testing.T) {
	regA, regB := scheduler.NewRegistry(), scheduler.NewRegistry()
	wA := &Wiring{env: Env{Owner: "w19-ra-a", StateDir: t.TempDir(), Log: slog.Default(), IDs: IDScope{}}}
	wB := &Wiring{env: Env{Owner: "w19-ra-b", StateDir: t.TempDir(), Log: slog.Default(), IDs: IDScope{}}}
	if wA.registerRAController(regA) != nil || wB.registerRAController(regB) != nil {
		t.Fatal("register")
	}
	defer wA.Close()
	defer wB.Close()
	a, b := RAEnvFor(wA.env.Owner), RAEnvFor(wB.env.Owner)
	if a.Owner != wA.env.Owner || b.Owner != wB.env.Owner || RARuntimeFor(a.Owner) == RARuntimeFor(b.Owner) {
		t.Fatal("last wiring adopted foreign owner")
	}
	if RAEnvFor("missing").Owner != "" {
		t.Fatal("singleton fallback")
	}
	wB.Close()
	if RAEnvFor(a.Owner).Owner != a.Owner || RARuntimeFor(a.Owner) == nil || RARuntimeFor(b.Owner) != nil || RAEnvFor(b.Owner).Owner != "" {
		t.Fatal("close damaged another owner")
	}
}

type raView map[scheduler.Key]proto.Message

func (v raView) Get(k scheduler.Key) (proto.Message, bool) { x, ok := v[k]; return x, ok }
func (v raView) List(name string) []scheduler.KV {
	var out []scheduler.KV
	for k, x := range v {
		if k.Descriptor() == name {
			out = append(out, scheduler.KV{Key: k, Value: x})
		}
	}
	return out
}

type raObserveOnly struct{ *raMemoryDescriptor }

func (*raObserveOnly) DeleteOnAbsence() bool { return false }
func TestRAScopedValidationRejectsCollisionBeforeAnyWrite(t *testing.T) {
	raw := &raMemoryDescriptor{name: core.RouteName, rows: map[scheduler.Key]scheduler.KV{}}
	normal := &raScopedDescriptor{inner: raw}
	route := &core.Route{TableId: 7, Prefix: "192.0.2.0/24", Paths: []*core.RoutePath{{Interface: "loop7", Weight: 1}}}
	key := normal.KeyOf(route)
	view := raView{scheduler.Join("remote-access."+core.RouteName, key.ID()): route}
	if normal.Validate(context.Background(), key, route, view) == nil || raw.writes != 0 {
		t.Fatal("collision was not rejected without mutation")
	}
	reserved := proto.Clone(route).(*core.Route)
	reserved.Paths[0].Interface = ravpn.LinkName(ravpn.InstanceID("w19", "road"), false)
	if normal.Validate(context.Background(), normal.KeyOf(reserved), reserved, raView{}) == nil {
		t.Fatal("normal desired reserved path accepted")
	}
	wrapped := &raScopedDescriptor{inner: &raObserveOnly{raw}, private: true}
	if wrapped.DeleteOnAbsence() {
		t.Fatal("observe-only semantics lost")
	}
}
