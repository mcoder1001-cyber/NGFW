package subsystems

import (
	"testing"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/gso"
	"ngfw/agent/internal/descriptors/lldp"
	"ngfw/agent/internal/descriptors/nsim"
	"ngfw/agent/internal/descriptors/span"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

// recorder keeps every descriptor registered through it.
type recorder struct {
	scheduler.Registry
	ds []scheduler.Descriptor
}

func (r *recorder) Register(d scheduler.Descriptor) { r.Registry.Register(d); r.ds = append(r.ds, d) }

// Every descriptor this feature registers declares how it records ownership (TD-11b's guard: exactly one
// of CheckPersistent and RecordsNoOwnership), the services names are appended to Domains["services"]
// once, and the VPP-global families are registered only on the globals owner (D-071).
func TestLoopbackBviGsoLldpSpanWiring(t *testing.T) {
	mine := map[string]bool{gso.Name: true, span.NameMirror: true, lldp.NameInterface: true, lldp.NameGlobal: true,
		nsim.ConfigName: true, nsim.CrossConnectName: true, nsim.OutputName: true}
	for _, owner := range []bool{false, true} {
		dir := t.TempDir()
		owned, err := ownertable.Open(dir, "w7")
		if err != nil {
			t.Fatal(err)
		}
		r := &recorder{Registry: scheduler.NewRegistry()}
		if _, err := Register(r, Env{Client: coretest.New(), Owner: "w7", StateDir: dir, Owned: owned, GlobalsOwner: owner,
			NetdevKind: func(string) (string, bool, error) { return "", false, nil }}); err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, d := range r.ds {
			if !mine[d.Name()] {
				continue
			}
			seen[d.Name()] = true
			_, checks := d.(interface{ CheckPersistent() error })
			_, none := d.(interface{ RecordsNoOwnership() })
			if checks == none {
				t.Errorf("%s (%T): CheckPersistent %v, RecordsNoOwnership %v — exactly one is required", d.Name(), d, checks, none)
			}
		}
		for _, n := range []string{gso.Name, span.NameMirror, lldp.NameInterface} {
			if !seen[n] {
				t.Errorf("globals owner %v: %s not registered", owner, n)
			}
		}
		for _, n := range []string{lldp.NameGlobal, nsim.ConfigName, nsim.CrossConnectName, nsim.OutputName} {
			if seen[n] != owner {
				t.Errorf("globals owner %v: %s registered = %v", owner, n, seen[n])
			}
		}
		if got := LoopbackBviGsoLldpSpanEnv().GlobalsOwner; got != owner {
			t.Errorf("projection env globals owner %v, want %v", got, owner)
		}
	}
	count := map[string]int{}
	for _, n := range Domains[servicesDomain] {
		count[n]++
	}
	for _, n := range loopbackServices {
		if count[n] != 1 || DomainOf(n) != servicesDomain {
			t.Errorf("%s in Domains[services] %d times (domain %q)", n, count[n], DomainOf(n))
		}
	}
	for _, n := range []string{gso.Name, span.NameMirror} {
		if DomainOf(n) != Interfaces {
			t.Errorf("%s belongs to %q, want interfaces", n, DomainOf(n))
		}
	}
	// an in-memory store never passes the check (the product wiring must pass the persisted ones)
	if err := gso.New(coretest.New(), "w7-mem", nil).CheckPersistent(); err == nil {
		t.Error("gso with an in-memory boot store passed CheckPersistent")
	}
	if err := nsim.NewConfig(coretest.New(), nil).CheckPersistent(); err == nil {
		t.Error("nsim.config with an in-memory boot store passed CheckPersistent")
	}
}
