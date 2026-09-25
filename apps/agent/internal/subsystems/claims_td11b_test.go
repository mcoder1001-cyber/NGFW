package subsystems

// TD-11b: the product wiring's claim stores and persistence guard.
//
//	TestClaimRefreshBoundedByCallerContext  R2-stores: the claim's sw_if_index refresh runs within the
//	                                        caller's deadline, not a 5 s bound of the store's own
//	TestClaimRefreshCancelledWithCaller     … and ends with the caller (nothing written: claim first)
//	TestRegisterGuardsEveryDescriptor       review 3.2: Register checks every descriptor it registered
//	TestRequirePersistentPerFamily          review 3.2: in-memory stores fail, every Wiring store passes

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// nicWiring registers the product wiring for owner over a fake VPP with the untagged NIC ens224 and
// makes the VPP boot identity known (Connected).
func nicWiring(t *testing.T, owner string) (*ifacetest.VPP, *scheduler.MapRegistry, *Wiring) {
	t.Helper()
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, owner)
	if err != nil {
		t.Fatal(err)
	}
	v := ifacetest.New()
	v.Add("ens224", "dpdk", "")
	reg := scheduler.NewRegistry()
	w, err := Register(reg, Env{Client: v, Owner: owner, StateDir: dir, Owned: owned, NetdevKind: func(string) (string, bool, error) { return "veth", true, nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	w.Connected(context.Background())
	if w.Identity().IsZero() {
		t.Fatal("no VPP boot identity from the fake")
	}
	return v, reg, w
}

// slowDumps makes every sw_interface_dump after the first one take d (a VPP API stall).
func slowDumps(v *ifacetest.VPP, d time.Duration) {
	var n atomic.Int32
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		if n.Add(1) > 1 {
			time.Sleep(d)
		}
		v.Mu.Lock()
		defer v.Mu.Unlock()
		out := make([]api.Message, 0, len(v.Ifs))
		for idx := uint32(0); idx < v.Next; idx++ {
			if det, ok := v.Ifs[idx]; ok {
				cp := *det
				out = append(out, &cp)
			}
		}
		return out, nil
	})
}

func attr(reg *scheduler.MapRegistry, name string) scheduler.Descriptor {
	d, _ := reg.Get(name)
	return d
}

// R2-stores (P08 re-review): a VPP API stall longer than 5 s inside the claim's index refresh
// failed the claim although the transaction's own deadline was far away — and, with the old order,
// after the value was already written. The refresh now runs within the caller's context.
func TestClaimRefreshBoundedByCallerContext(t *testing.T) {
	if testing.Short() {
		t.Skip("stalls the fake VPP for 5.2 s")
	}
	v, reg, _ := nicWiring(t, "w1r2")
	slowDumps(v, 5200*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // the transaction's deadline
	defer cancel()
	start := time.Now()
	if _, err := attr(reg, iface.AdminStateName).Create(ctx, &iface.AdminState{Interface: "interface/ens224"}); err != nil {
		t.Fatalf("Create failed after %v although the caller's deadline is 30 s: %v", time.Since(start).Round(time.Millisecond), err)
	}
	if !iface.Claims("w1r2").Claimed("ens224", iface.AdminStateName) {
		t.Fatal("claim not recorded")
	}
}

// A caller whose context ends during the refresh fails the claim with its context error — before
// anything was written (claim first).
func TestClaimRefreshCancelledWithCaller(t *testing.T) {
	v, reg, _ := nicWiring(t, "w1r3")
	slowDumps(v, 300*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := attr(reg, iface.MtuName).Create(ctx, &iface.Mtu{Interface: "interface/ens224", Mtu: 1400})
	if !errors.Is(err, ErrClaimUnbound) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Create = %v, want the claim to end with the caller's deadline", err)
	}
	if n := len(v.CallsNamed("sw_interface_set_mtu")); n != 0 {
		t.Fatalf("MTU written %d time(s) although the claim failed", n)
	}
}

// Register runs the guard over every descriptor it registered (through the tolerant and veth
// wrappers): every one of them declares how it records ownership (fix round 1, review M1), and the
// product wiring passes its own guard.
func TestRegisterGuardsEveryDescriptor(t *testing.T) {
	_, reg, _ := nicWiring(t, "w1g")
	g := &guardRegistry{Registry: scheduler.NewRegistry()}
	dir := t.TempDir()
	owned, _ := ownertable.Open(dir, "w1g")
	if _, err := register(g, Env{Client: ifacetest.New(), Owner: "w1g", StateDir: dir, Owned: owned}); err != nil {
		t.Fatal(err)
	}
	if len(g.ds) != reg.Len() {
		t.Fatalf("guard saw %d descriptors, the registry has %d", len(g.ds), reg.Len())
	}
	for _, d := range g.ds {
		if err := persist.Declared(d); err != nil {
			t.Errorf("%s (%T): %v", d.Name(), d, err)
		}
	}
	if err := RequirePersistent(g.ds); err != nil {
		t.Fatalf("product wiring fails its own guard: %v", err)
	}
	// the same descriptors with the owner's store swapped for the in-memory default: refused
	iface.SetClaimStore("w1g", nil)
	if err := RequirePersistent(g.ds); !errors.Is(err, ErrVolatileStores) || !errors.Is(err, persist.ErrVolatile) {
		t.Fatalf("in-memory DF-1 store: %v", err)
	}
}

// undeclared is a descriptor a feature row registered without saying how it records ownership.
type undeclared struct{ scheduler.Descriptor }

func (undeclared) Name() string { return "test.undeclared" }

// An undeclared descriptor — plain or behind a wrapper, registered through Register's guarded
// registry — is refused: the agent does not start (fix round 1, review M1).
func TestRequirePersistentRefusesUndeclared(t *testing.T) {
	g := &guardRegistry{Registry: scheduler.NewRegistry()}
	g.Register(undeclared{})
	if err := RequirePersistent(g.ds); !errors.Is(err, ErrUndeclaredDescriptors) || !errors.Is(err, persist.ErrUndeclared) {
		t.Fatalf("undeclared descriptor: %v", err)
	}
	wrapped := newDefaultTolerant(undeclared{}, iface.ErrMtuDefault, nil)
	if err := RequirePersistent([]scheduler.Descriptor{wrapped}); !errors.Is(err, ErrUndeclaredDescriptors) {
		t.Fatalf("undeclared behind a wrapper: %v", err)
	}
	if err := RequirePersistent([]scheduler.Descriptor{natDesc(nil), undeclared{}}); !errors.Is(err, ErrUndeclaredDescriptors) || !errors.Is(err, ErrVolatileStores) {
		t.Fatalf("both findings are reported: %v", err)
	}
}

// df2Desc stands for a DF-2 descriptor (urpf, adl, abf, …): its CheckPersistent is its Options'.
type df2Desc struct {
	scheduler.Descriptor
	opts df2.Options
}

func (d df2Desc) CheckPersistent() error { return d.opts.CheckPersistent("urpf.interface") }

// dfkitDesc stands for a DF-8 descriptor with an applied-once BootStore (pcap).
type dfkitDesc struct {
	scheduler.Descriptor
	owner string
	boot  dfkit.BootStore
}

func (d dfkitDesc) CheckPersistent() error {
	return errors.Join(dfkit.CheckClaims("pcap.capture", d.owner), dfkit.CheckBoot("pcap.capture", d.boot))
}

func keyedSpec() df6.KeyedSpec[*wrapperspb.StringValue] {
	return df6.KeyedSpec[*wrapperspb.StringValue]{
		Name: "test.keyed", Plugin: "test",
		Canon: func(v *wrapperspb.StringValue) (*wrapperspb.StringValue, error) { return v, nil },
		ID:    func(v *wrapperspb.StringValue) string { return v.GetValue() },
		Add:   func(context.Context, vpp.Client, *wrapperspb.StringValue) error { return nil },
		Del:   func(context.Context, vpp.Client, *wrapperspb.StringValue) error { return nil },
		List:  func(context.Context, vpp.Client) ([]*wrapperspb.StringValue, error) { return nil, nil },
	}
}

type natSpec struct {
	ID string `json:"id"`
}

func natDesc(c natcommon.ClaimStore) scheduler.Descriptor {
	return natcommon.New(natcommon.Ops[natSpec]{
		Name: "test.nat", Claims: c,
		ID:       func(s natSpec) string { return s.ID },
		Create:   func(context.Context, natSpec) (any, error) { return nil, nil },
		Delete:   func(context.Context, natSpec, any) error { return nil },
		Retrieve: func(context.Context) ([]natcommon.Item[natSpec], error) { return nil, nil },
	})
}

// Every family: the in-memory default (what a feature row gets when it forgets the Wiring store)
// fails the guard, the Wiring store passes — the guard is what makes the agent refuse to start.
func TestRequirePersistentPerFamily(t *testing.T) {
	v, _, w := nicWiring(t, "w1f")
	acl, err := w.KeyedClaims("acl")
	if err != nil {
		t.Fatal(err)
	}
	nat, err := w.KeyedClaims("nat")
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := w.PairClaims("df6")
	if err != nil {
		t.Fatal(err)
	}
	var none scheduler.Descriptor
	for _, c := range []struct {
		family       string
		volatile, ok scheduler.Descriptor
		wantKind     error
	}{
		{"natcommon", natDesc(nil), natDesc(nat), persist.ErrVolatile},
		{"interface attributes", newDefaultTolerant(iface.NewMtu(v, "w1f-mem"), iface.ErrMtuDefault, nil), newDefaultTolerant(iface.NewMtu(v, "w1f"), iface.ErrMtuDefault, nil), persist.ErrVolatile},
		{"dfkit", dfkitDesc{none, "w1f", dfkit.NewMemoryBootStore()}, dfkitDesc{none, "w1f", w.BootStore()}, persist.ErrVolatile},
		{"df6 (in memory)", df6.NewKeyedDescriptor(keyedSpec(), v, "w1f", df6.WithClaims(iface.NewMemoryClaimStore())), df6.NewKeyedDescriptor(keyedSpec(), v, "w1f", df6.WithClaims(pairs)), persist.ErrVolatile},
		{"df6 (interface store)", df6.NewKeyedDescriptor(keyedSpec(), v, "w1f"), df6.NewKeyedDescriptor(keyedSpec(), v, "w1f", df6.WithClaims(pairs)), df6.ErrClaimStoreKind},
		{"df2 claims", df2Desc{none, df2.BuildOptions()}, df2Desc{none, df2.BuildOptions(df2.WithClaims(acl))}, persist.ErrVolatile},
	} {
		t.Run(c.family, func(t *testing.T) {
			if err := RequirePersistent([]scheduler.Descriptor{c.volatile}); !errors.Is(err, ErrVolatileStores) || !errors.Is(err, c.wantKind) {
				t.Fatalf("in-memory store: %v", err)
			}
			if err := RequirePersistent([]scheduler.Descriptor{c.ok}); err != nil {
				t.Fatalf("Wiring store: %v", err)
			}
		})
	}
	iface.SetClaimStore("w1f-mem", nil)
}

// PairClaims (df6) persist across an agent restart, expire with the VPP instance, and never bind
// to an interface (keyed ids are not interface names).
func TestPairClaims(t *testing.T) {
	_, _, w := nicWiring(t, "w1p")
	p, err := w.PairClaims("df6")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Claim("fd00::1:0/112", "sr.localsid@vpp-x"); err != nil {
		t.Fatalf("keyed id claim: %v", err)
	}
	if err := w.IfaceClaims().ClaimContext(context.Background(), "fd00::1:0/112", "sr.localsid@vpp-x"); !errors.Is(err, ErrClaimUnbound) {
		t.Fatalf("the interface store must refuse ids that are not interfaces (why df6 needs PairClaims): %v", err)
	}
	again, err := OpenKeyedClaims(filepath.Dir(p.path), "df6", "w1p", w.identity)
	if err != nil || !again.Pairs().Claimed("fd00::1:0/112", "sr.localsid@vpp-x") {
		t.Fatalf("pair claim lost across reopen: %v", err)
	}
	if err := p.Release("fd00::1:0/112", "sr.localsid@vpp-x"); err != nil || p.Claimed("fd00::1:0/112", "sr.localsid@vpp-x") {
		t.Fatalf("release: %v", err)
	}
	if !persist.Is(p) || !persist.Is(w.IfaceClaims()) {
		t.Fatal("Wiring stores survive an agent restart")
	}
}

// Review L1: a caller without a deadline (the resync on a VPP connect) gets the refresh capped at
// legacyBound; a caller's own deadline still wins (R2).
func TestIndexRefreshCappedWithoutDeadline(t *testing.T) {
	var got []time.Duration
	c := NewIndexCache(time.Second, func(ctx context.Context) (map[string]uint32, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("refresh ran without a deadline")
		}
		got = append(got, time.Until(dl))
		return map[string]uint32{"ens224": 5}, nil
	})
	if idx, ok := c.Resolve(context.Background(), "ens224"); !ok || idx != 5 {
		t.Fatalf("resolve %d %v", idx, ok)
	}
	c.Invalidate()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c.Resolve(ctx, "ens224")
	if len(got) != 2 || got[0] > legacyBound || got[0] < legacyBound-time.Second || got[1] < 25*time.Second {
		t.Fatalf("refresh deadlines %v: want ≈%v without a caller deadline, ≈30 s with one", got, legacyBound)
	}
}
