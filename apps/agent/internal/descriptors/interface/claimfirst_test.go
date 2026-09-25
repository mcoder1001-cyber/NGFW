package iface_test

// TD-11b (review 3.3, R2-stores): the six DF-1 attribute descriptors claim an untagged interface
// BEFORE they write to VPP, and release a claim they made when the write fails. With the old order
// (write, then claim) a claim failure left the value in VPP, unjournaled and invisible to Retrieve.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
)

// recClaims is an in-memory iface.ClaimStore that can refuse to record and counts calls.
type recClaims struct {
	mu       sync.Mutex
	m        map[[2]string]bool
	failWith error // Claim returns it (nothing recorded)
	ctxCalls int   // ClaimContext calls (the ctx-bounded path of persisted stores)
}

func newRecClaims() *recClaims { return &recClaims{m: map[[2]string]bool{}} }

func (c *recClaims) Claim(ifName, holder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failWith != nil {
		return c.failWith
	}
	c.m[[2]string{ifName, holder}] = true
	return nil
}

func (c *recClaims) Release(ifName, holder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, [2]string{ifName, holder})
	return nil
}

func (c *recClaims) Claimed(ifName, holder string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[[2]string{ifName, holder}]
}

// ctxRecClaims also offers the ctx-bounded claim (subsystems.IfaceClaims' shape) and records
// the caller marker found in the context it was given.
type ctxRecClaims struct {
	*recClaims
	seen *[]any
}

type callerKey struct{}

func (c ctxRecClaims) ClaimContext(ctx context.Context, ifName, holder string) error {
	c.mu.Lock()
	c.ctxCalls++
	*c.seen = append(*c.seen, ctx.Value(callerKey{}))
	c.mu.Unlock()
	return c.Claim(ifName, holder)
}

func (c ctxRecClaims) ClaimedContext(ctx context.Context, ifName, holder string) bool {
	c.mu.Lock()
	*c.seen = append(*c.seen, ctx.Value(callerKey{}))
	c.mu.Unlock()
	return c.Claimed(ifName, holder)
}

// attrCase is one DF-1 attribute descriptor, a desired object on the untagged NIC "ens224" and the
// VPP message its Create writes with.
type attrCase struct {
	name string
	d    func(v *ifacetest.VPP) scheduler.Descriptor
	obj  proto.Message
	msg  string
}

func attrCases() []attrCase {
	const ref = "interface/ens224"
	return []attrCase{
		{iface.AdminStateName, func(v *ifacetest.VPP) scheduler.Descriptor { return iface.NewAdminState(v, owner) },
			&iface.AdminState{Interface: ref}, "sw_interface_set_flags"},
		{iface.MtuName, func(v *ifacetest.VPP) scheduler.Descriptor { return iface.NewMtu(v, owner) },
			&iface.Mtu{Interface: ref, Mtu: 1400}, "sw_interface_set_mtu"},
		{iface.MacAddressName, func(v *ifacetest.VPP) scheduler.Descriptor { return iface.NewMacAddress(v, owner) },
			&iface.MacAddress{Interface: ref, Mac: "02:00:00:00:11:01"}, "sw_interface_set_mac_address"},
		{iface.PromiscName, func(v *ifacetest.VPP) scheduler.Descriptor { return iface.NewPromisc(v, owner) },
			&iface.Promisc{Interface: ref}, "sw_interface_set_promisc"},
		{iface.RxModeName, func(v *ifacetest.VPP) scheduler.Descriptor { return iface.NewRxMode(v, owner) },
			&iface.RxMode{Interface: ref, Mode: iface.RxModeKind_RX_MODE_KIND_ADAPTIVE}, "sw_interface_set_rx_mode"},
		{iface.RxPlacementName, func(v *ifacetest.VPP) scheduler.Descriptor { return iface.NewRxPlacement(v, owner) },
			&iface.RxPlacement{Interface: ref, Queue: 0, Worker: 0}, "sw_interface_set_rx_placement"},
	}
}

func nicWorld(t *testing.T, s iface.ClaimStore) *ifacetest.VPP {
	t.Helper()
	v := ifacetest.New()
	v.Workers = 1
	v.Add("ens224", "dpdk", "") // untagged: ownership only through the claim
	iface.SetClaimStore(owner, s)
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	return v
}

// A claim that cannot be recorded fails the Create BEFORE anything is written to VPP.
func TestAttributeClaimsBeforeWrite(t *testing.T) {
	refused := errors.New("claim store: VPP boot identity not known yet")
	for _, c := range attrCases() {
		t.Run(c.name, func(t *testing.T) {
			s := newRecClaims()
			s.failWith = refused
			v := nicWorld(t, s)
			meta, err := c.d(v).Create(context.Background(), c.obj)
			if !errors.Is(err, refused) {
				t.Fatalf("Create = %v, %v; want the claim error", meta, err)
			}
			if n := len(v.CallsNamed(c.msg)); n != 0 {
				t.Fatalf("%s sent %d time(s) although the claim failed (VPP written, claim missing)", c.msg, n)
			}
			if meta != nil {
				t.Fatalf("nothing was written, so no Meta: %+v", meta)
			}
		})
	}
}

// A failed write releases the claim the Create made (no claim without an object), but keeps a
// claim that existed before.
func TestAttributeWriteFailureReleasesNewClaim(t *testing.T) {
	for _, c := range attrCases() {
		t.Run(c.name, func(t *testing.T) {
			s := newRecClaims()
			v := nicWorld(t, s)
			v.On(c.msg, func(api.Message) ([]api.Message, error) { return nil, errors.New("vpp: write refused") })
			if _, err := c.d(v).Create(context.Background(), c.obj); err == nil {
				t.Fatal("Create succeeded although the write failed")
			}
			if s.Claimed("ens224", c.name) {
				t.Fatal("claim left behind by a failed write")
			}
			_ = s.Claim("ens224", c.name) // a claim of an earlier, successful Create
			if _, err := c.d(v).Create(context.Background(), c.obj); err == nil {
				t.Fatal("Create succeeded although the write failed")
			}
			if !s.Claimed("ens224", c.name) {
				t.Fatal("a claim that existed before the Create was released")
			}
		})
	}
}

// A persisted store's ctx-bounded claim gets the caller's context (R2-stores): its sw_if_index
// lookup is bounded by the transaction's deadline, not by the store's own timeout.
func TestAttributeClaimUsesCallerContext(t *testing.T) {
	for _, c := range attrCases() {
		t.Run(c.name, func(t *testing.T) {
			var seen []any
			s := ctxRecClaims{recClaims: newRecClaims(), seen: &seen}
			v := nicWorld(t, s)
			ctx := context.WithValue(context.Background(), callerKey{}, c.name)
			if _, err := c.d(v).Create(ctx, c.obj); err != nil {
				t.Fatal(err)
			}
			if s.ctxCalls != 1 || !s.Claimed("ens224", c.name) {
				t.Fatalf("ctx claims %d, claimed %v", s.ctxCalls, s.Claimed("ens224", c.name))
			}
			for _, got := range seen {
				if got != c.name {
					t.Fatalf("the store saw context %v, not the caller's", got)
				}
			}
		})
	}
}

// persistedClaims is recClaims that survives an agent restart (the subsystems.IfaceClaims shape).
type persistedClaims struct{ *recClaims }

func (persistedClaims) Persistent() bool { return true }

// TestCheckPersistent (TD-11b, review 3.2): every DF-1 descriptor of an owner fails the product
// agent's guard while the owner's claim store is the in-memory default, and passes once a persisted
// store is installed (subsystems.Register does that before registering them).
func TestCheckPersistent(t *testing.T) {
	v := ifacetest.New()
	ds := []scheduler.Descriptor{
		iface.NewAdminState(v, owner), iface.NewMtu(v, owner), iface.NewMacAddress(v, owner),
		iface.NewPromisc(v, owner), iface.NewRxMode(v, owner), iface.NewRxPlacement(v, owner),
		iface.NewSubinterface(v, owner), iface.NewAlias(v, owner),
	}
	iface.SetClaimStore(owner, nil) // the in-memory default
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	for _, d := range ds {
		if err := persist.Check(d); !errors.Is(err, persist.ErrVolatile) {
			t.Fatalf("%s with the in-memory store: %v", d.Name(), err)
		}
	}
	iface.SetClaimStore(owner, persistedClaims{newRecClaims()})
	for _, d := range ds {
		if err := persist.Check(d); err != nil {
			t.Fatalf("%s with a persisted store: %v", d.Name(), err)
		}
	}
}
