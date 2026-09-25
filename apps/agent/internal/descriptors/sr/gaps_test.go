package sr_test

// F-srv6 gaps: the TD-11b declaration of the globals (sr.Global) and the local SID counters.

import (
	"context"
	"testing"

	"go.fd.io/govpp/api"

	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/scheduler"
)

type listRegistry struct{ ds []scheduler.Descriptor }

func (r *listRegistry) Register(d scheduler.Descriptor) { r.ds = append(r.ds, d) }

// TestGlobalsDeclareNoOwnership: every descriptor sr.Register registers passes the product agent's
// TD-11b guard (declared, persisted claims), for the globals owner and for a slot agent; the require
// variant keeps "never delete on absence", the setter keeps "delete on absence".
func TestGlobalsDeclareNoOwnership(t *testing.T) {
	for _, owner := range []bool{true, false} {
		f := newFakeSR()
		r := &listRegistry{}
		claims, err := df6.OpenFileClaimStore(t.TempDir() + "/claims.json")
		if err != nil {
			t.Fatal(err)
		}
		sr.Register(r, f, "w4", df6.WithGlobalsOwner(owner), df6.WithClaims(claims))
		if len(r.ds) != 5 {
			t.Fatalf("registered %d", len(r.ds))
		}
		for _, d := range r.ds {
			if err := persist.Declared(d); err != nil {
				t.Errorf("owner=%v %s: %v", owner, d.Name(), err)
			}
			if err := persist.Check(d); err != nil {
				t.Errorf("owner=%v %s: %v", owner, d.Name(), err)
			}
			g, ok := d.(*sr.Global)
			if !ok {
				continue
			}
			if g.DeleteOnAbsence() != owner {
				t.Errorf("owner=%v %s: DeleteOnAbsence=%v", owner, d.Name(), g.DeleteOnAbsence())
			}
			if g.Unwrap() == nil {
				t.Errorf("%s: Unwrap nil", d.Name())
			}
		}
	}
}

// TestLocalSidCounters reads sr_localsids_with_packet_stats_dump keyed by canonical SID.
func TestLocalSidCounters(t *testing.T) {
	f := newFakeSR()
	f.On("sr_localsids_with_packet_stats_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{
			&srapi.SrLocalsidsWithPacketStatsDetails{Addr: df6test.IP6("fd00:4:ff::1"), GoodTrafficPktCount: 3, GoodTrafficBytes: 300, BadTrafficPktCount: 1, BadTrafficBytes: 60},
			&srapi.SrLocalsidsWithPacketStatsDetails{}, // :: is skipped
		}, nil
	})
	got, err := sr.LocalSidCounters(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["fd00:4:ff::1"] != (sr.Counters{GoodPackets: 3, GoodBytes: 300, BadPackets: 1, BadBytes: 60}) {
		t.Fatalf("counters %v", got)
	}
}
