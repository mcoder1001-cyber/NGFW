package vpn_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// setter is a minimal global setter for the wrapper tests.
type setter struct{ sets int }

func (*setter) Name() string                                      { return "x.global" }
func (*setter) KeyOf(proto.Message) scheduler.Key                 { return "x.global/global" }
func (*setter) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (s *setter) Create(context.Context, proto.Message) (any, error) {
	s.sets++
	return nil, nil
}
func (s *setter) Update(ctx context.Context, _, n proto.Message, _ any) (any, error) {
	return s.Create(ctx, n)
}
func (*setter) Delete(context.Context, proto.Message, any) error { return nil }
func (*setter) Retrieve(context.Context) ([]scheduler.KV, error) {
	return []scheduler.KV{{Key: "x.global/global", Value: &vpnpb.Ikev2SleepInterval{Seconds: 1}}}, nil
}

func TestGlobalRoles(t *testing.T) {
	ctx := context.Background()
	want := &vpnpb.Ikev2SleepInterval{Seconds: 2}

	s := &setter{}
	owner := vpn.Global(true, s, nil)
	if _, err := owner.Create(ctx, want); err != nil || s.sets != 1 {
		t.Fatalf("owner Create: %v sets=%d", err, s.sets)
	}
	if a, ok := owner.(scheduler.AbsenceDeleter); !ok || a.DeleteOnAbsence() {
		t.Fatal("owner global must never delete on absence")
	}

	// non-owner with a getter: succeeds only when VPP already has the value, never sets
	have := proto.Message(&vpnpb.Ikev2SleepInterval{Seconds: 2})
	get := func(context.Context, proto.Message) (proto.Message, bool, error) { return have, true, nil }
	req := vpn.Global(false, s, get)
	if _, err := req.Create(ctx, want); err != nil {
		t.Fatalf("require (equal): %v", err)
	}
	have = &vpnpb.Ikev2SleepInterval{Seconds: 3}
	if _, err := req.Create(ctx, want); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
		t.Fatalf("require (differs): %v", err)
	}
	if _, err := req.Update(ctx, want, want, nil); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
		t.Fatalf("require Update: %v", err)
	}
	if s.sets != 1 {
		t.Fatalf("non-owner set the global (%d sets)", s.sets)
	}
	if kvs, err := req.Retrieve(ctx); kvs != nil || !scheduler.IsRetrieveUnsupported(err) {
		t.Fatalf("require Retrieve = %v, %v", kvs, err)
	}
	if err := req.Delete(ctx, want, nil); err != nil {
		t.Fatal(err)
	}
	if req.Name() != "x.global" || req.KeyOf(want) != "x.global/global" || req.Dependencies(want) != nil {
		t.Fatal("require must delegate identity to the setter")
	}
	// getter-less global: a non-owner can never satisfy it
	if _, err := vpn.Global(false, s, nil).Create(ctx, want); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
		t.Fatalf("getter-less require: %v", err)
	}
}

func TestRecords(t *testing.T) {
	restore := vpn.IdentitySource
	t.Cleanup(func() { vpn.IdentitySource = restore })
	pid := 100
	vpn.IdentitySource = func(context.Context, vpp.Client) (bootid.Identity, error) {
		return bootid.Identity{BootID: "b", PID: pid, StartTime: 7}, nil
	}
	ctx := context.Background()
	r := vpn.Records{Store: dfkit.NewMemoryBootStore()}
	id, err := r.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Valid(id, "k"); ok {
		t.Fatal("no record yet")
	}
	if err := r.Put(id, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Valid(id, "k"); !ok || v != "v" {
		t.Fatalf("Valid = %q %v", v, ok)
	}
	pid++ // VPP restart: every record has expired
	id2, _ := r.Identity(ctx)
	if _, ok := r.Valid(id2, "k"); ok {
		t.Fatal("record of an earlier VPP instance must not match")
	}
	if err := r.Drop("k"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Valid(id, "k"); ok {
		t.Fatal("dropped")
	}
}
