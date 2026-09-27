package dslite_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"go.fd.io/govpp/api"

	dsliteapi "ngfw/agent/binapi/dslite"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/dslite"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/fake"
)

// fakeDS models dslite_api.c: AFTR/B4 are plain overwrites, pool addresses are single entries,
// an add of an existing address answers VNET_API_ERROR_VALUE_EXIST, a delete of a missing one
// NO_SUCH_ENTRY.
type fakeDS struct {
	*fake.Client
	aftr, b4 dsliteapi.DsliteGetAftrAddrReply
	pool     map[netip.Addr]bool
	setCalls int
}

func newFakeDS() *fakeDS {
	f := &fakeDS{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), pool: map[netip.Addr]bool{}}
	f.On("dslite_get_aftr_addr", func(api.Message) ([]api.Message, error) {
		r := f.aftr
		return []api.Message{&r}, nil
	})
	f.On("dslite_set_aftr_addr", func(req api.Message) ([]api.Message, error) {
		r := req.(*dsliteapi.DsliteSetAftrAddr)
		f.setCalls++
		f.aftr = dsliteapi.DsliteGetAftrAddrReply{IP4Addr: r.IP4Addr, IP6Addr: r.IP6Addr}
		return []api.Message{&dsliteapi.DsliteSetAftrAddrReply{}}, nil
	})
	f.On("dslite_get_b4_addr", func(api.Message) ([]api.Message, error) {
		return []api.Message{&dsliteapi.DsliteGetB4AddrReply{IP4Addr: f.b4.IP4Addr, IP6Addr: f.b4.IP6Addr}}, nil
	})
	f.On("dslite_set_b4_addr", func(req api.Message) ([]api.Message, error) {
		r := req.(*dsliteapi.DsliteSetB4Addr)
		f.setCalls++
		f.b4 = dsliteapi.DsliteGetAftrAddrReply{IP4Addr: r.IP4Addr, IP6Addr: r.IP6Addr}
		return []api.Message{&dsliteapi.DsliteSetB4AddrReply{}}, nil
	})
	f.On("dslite_add_del_pool_addr_range", func(req api.Message) ([]api.Message, error) {
		r := req.(*dsliteapi.DsliteAddDelPoolAddrRange)
		a, b := netip.AddrFrom4(r.StartAddr), netip.AddrFrom4(r.EndAddr)
		for x := a; !b.Less(x); x = x.Next() {
			if r.IsAdd == f.pool[x] {
				rv := int32(-16) // VALUE_EXIST
				if !r.IsAdd {
					rv = -6 // NO_SUCH_ENTRY
				}
				return []api.Message{&dsliteapi.DsliteAddDelPoolAddrRangeReply{Retval: rv}}, nil
			}
		}
		for x := a; !b.Less(x); x = x.Next() {
			if r.IsAdd {
				f.pool[x] = true
			} else {
				delete(f.pool, x)
			}
		}
		return []api.Message{&dsliteapi.DsliteAddDelPoolAddrRangeReply{}}, nil
	})
	f.On("dslite_address_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for a := range f.pool {
			out = append(out, &dsliteapi.DsliteAddressDetails{IPAddress: a.As4()})
		}
		return out, nil
	})
	return f
}

func ep(v6, v4 string) *dslite.EndpointSpec { return &dslite.EndpointSpec{IPv6: v6, IPv4: v4} }

func TestGlobalsOwnerSetsRetrievesResets(t *testing.T) {
	f := newFakeDS()
	p := dslite.New(f, "w9", natcommon.WithGlobalsOwner(true))
	ctx := context.Background()
	aftr, _ := natcommon.Encode(ep("fd00:9::1", "192.0.0.1"))
	if n := nattest.Apply(t, p.Aftr, aftr); n != 1 {
		t.Fatalf("first apply: %d ops", n)
	}
	if n := nattest.Apply(t, p.Aftr, aftr); n != 0 {
		t.Fatalf("second apply must be empty, got %d ops", n)
	}
	kvs, err := p.Aftr.Retrieve(ctx)
	if err != nil || len(kvs) != 1 {
		t.Fatalf("retrieve: %v %v", kvs, err)
	}
	// reset → unspecified → absent
	nattest.DeleteAll(ctx, t, p.Aftr)
	if f.aftr.IP6Addr != [16]uint8{} {
		t.Fatalf("aftr not reset: %v", f.aftr)
	}
}

func TestNonOwnerRequiresEndpoint(t *testing.T) {
	f := newFakeDS()
	p := dslite.New(f, "w9")
	ctx := context.Background()
	b4, _ := natcommon.Encode(ep("fd00:9::2", ""))
	if _, err := p.B4.Create(ctx, b4); !errors.Is(err, natcommon.ErrGlobalNotSet) {
		t.Fatalf("absent B4: want ErrGlobalNotSet, got %v", err)
	}
	f.b4.IP6Addr = netip.MustParseAddr("fd00:9::3").As16()
	if _, err := p.B4.Create(ctx, b4); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("different B4: want ErrGlobalMismatch, got %v", err)
	}
	f.b4.IP6Addr = netip.MustParseAddr("fd00:9::2").As16()
	calls := f.setCalls
	if _, err := p.B4.Create(ctx, b4); err != nil {
		t.Fatalf("matching B4: %v", err)
	}
	if err := p.B4.Delete(ctx, b4, nil); err != nil || f.setCalls != calls {
		t.Fatalf("non-owner must never set/reset (calls %d→%d, err %v)", calls, f.setCalls, err)
	}
	if _, err := p.B4.Retrieve(ctx); !errors.Is(err, natcommon.ErrRetrieveUnsupported) {
		t.Fatalf("non-owner retrieve: %v", err)
	}
}

func TestPoolsScopedAndClaimed(t *testing.T) {
	f := newFakeDS()
	f.pool[netip.MustParseAddr("10.3.0.1")] = true // another slot's
	claims := natcommon.NewMemoryClaimStore()
	p := dslite.New(f, "w9", natcommon.WithClaims(claims))
	ctx := context.Background()
	pool, _ := natcommon.Encode(&dslite.PoolSpec{First: "10.9.5.1", Last: "10.9.5.4"})
	if n := nattest.Apply(t, p.Pool, pool); n != 1 {
		t.Fatalf("apply: %d", n)
	}
	if n := nattest.Apply(t, p.Pool, pool); n != 0 {
		t.Fatalf("idempotent apply: %d", n)
	}
	if !claims.Claimed("dslite.pool/10.9.5.1-10.9.5.4") {
		t.Fatal("pool not claimed")
	}
	nattest.DeleteAll(ctx, t, p.Pool)
	if !f.pool[netip.MustParseAddr("10.3.0.1")] || len(f.pool) != 1 {
		t.Fatalf("foreign pool touched: %v", f.pool)
	}
}

func TestMergeRanges(t *testing.T) {
	got := dslite.MergeRanges([]netip.Addr{netip.MustParseAddr("10.9.0.3"), netip.MustParseAddr("10.9.0.1"), netip.MustParseAddr("10.9.0.2"), netip.MustParseAddr("10.9.0.9")})
	if len(got) != 2 || got[0] != (dslite.PoolSpec{First: "10.9.0.1", Last: "10.9.0.3"}) || got[1] != (dslite.PoolSpec{First: "10.9.0.9", Last: "10.9.0.9"}) {
		t.Fatalf("merge: %+v", got)
	}
}
