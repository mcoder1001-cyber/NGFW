package subsystems

// F-srv6 wiring: the sr family is registered with the persisted df6 claim store and passes the TD-11b
// guard, every sr descriptor belongs to the routing domain, the globals are setters only on the
// globals owner (D-071), and the applied global encap source feeds the assembler (Srv6Env).

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

func srv6Wiring(t *testing.T, owner string, globalsOwner bool) (*coretest.VPP, *scheduler.MapRegistry) {
	t.Helper()
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, owner)
	if err != nil {
		t.Fatal(err)
	}
	v := coretest.New()
	reg := scheduler.NewRegistry()
	w, err := Register(reg, Env{Client: v, Owner: owner, StateDir: dir, Owned: owned, GlobalsOwner: globalsOwner,
		NetdevKind: func(string) (string, bool, error) { return "veth", true, nil }})
	if err != nil {
		t.Fatalf("Register (TD-11b guard): %v", err)
	}
	w.Connected(context.Background())
	return v, reg
}

func TestSrv6Wiring(t *testing.T) {
	ctx := context.Background()
	for _, n := range []string{sr.LocalSidName, sr.PolicyName, sr.SteeringName, sr.EncapSourceName, sr.EncapHopLimitName} {
		if DomainOf(n) != Routing {
			t.Errorf("%s is in domain %q", n, DomainOf(n))
		}
	}

	// Slot agent (D-071): the globals are require variants — never set, never deleted on absence.
	v, reg := srv6Wiring(t, "w4a", false)
	for _, n := range []string{sr.LocalSidName, sr.PolicyName, sr.SteeringName} {
		if _, ok := reg.Get(n); !ok {
			t.Fatalf("%s not registered", n)
		}
	}
	src, _ := reg.Get(sr.EncapSourceName)
	if _, err := src.Create(ctx, &sr.EncapSource{Address: "fd00:4::1"}); !errors.Is(err, df6.ErrNotGlobalsOwner) {
		t.Fatalf("slot agent set the encap source: %v", err)
	}
	if a, ok := src.(scheduler.AbsenceDeleter); !ok || a.DeleteOnAbsence() {
		t.Fatal("slot agent's global may be deleted on absence")
	}
	if got, _ := v.SR().Globals(); got != "::" {
		t.Fatalf("encap source %s", got)
	}
	if e := Srv6Env(); e.EncapSource == nil || e.EncapSource() != "" {
		t.Fatal("slot agent reports an applied encap source")
	}

	// Globals owner: the setter applies and the assembler learns the applied value; Delete resets.
	v, reg = srv6Wiring(t, "w4b", true)
	src, _ = reg.Get(sr.EncapSourceName)
	if _, err := src.Create(ctx, &sr.EncapSource{Address: "FD00:4::1"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.SR().Globals(); got != "fd00:4::1" || Srv6Env().EncapSource() != "fd00:4::1" {
		t.Fatalf("encap source %s, env %s", got, Srv6Env().EncapSource())
	}
	if _, err := src.Update(ctx, &sr.EncapSource{Address: "fd00:4::1"}, &sr.EncapSource{Address: "fd00:4::2"}, nil); err != nil || Srv6Env().EncapSource() != "fd00:4::2" {
		t.Fatalf("update: %v %s", err, Srv6Env().EncapSource())
	}
	if err := src.Delete(ctx, &sr.EncapSource{Address: "fd00:4::2"}, nil); err != nil || Srv6Env().EncapSource() != "" {
		t.Fatalf("delete: %v %s", err, Srv6Env().EncapSource())
	}
	if got, _ := v.SR().Globals(); got != "::" {
		t.Fatalf("encap source not reset: %s", got)
	}
	hl, _ := reg.Get(sr.EncapHopLimitName)
	if _, err := hl.Create(ctx, &sr.EncapHopLimit{HopLimit: 9}); err != nil {
		t.Fatal(err)
	}
	if _, h := v.SR().Globals(); h != 9 {
		t.Fatalf("hop limit %d", h)
	}

	// Srv6State: only for a registered owner.
	if _, err := Srv6State(ctx, "w4-none"); !errors.Is(err, ErrSrv6NotWired) {
		t.Fatalf("unregistered owner: %v", err)
	}
	st, err := Srv6State(ctx, "w4b")
	if err != nil || len(st.LocalSids)+len(st.Policies)+len(st.Steering) != 0 {
		t.Fatalf("state %v %v", st, err)
	}
}
