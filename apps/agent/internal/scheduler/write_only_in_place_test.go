package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type inPlaceWO struct {
	wo
	enabled bool
}

func (w *inPlaceWO) CreatePreservesDependents() bool { return w.enabled }

type inPlaceReadable struct{ mem }

func (*inPlaceReadable) CreatePreservesDependents() bool { return true }

func TestWriteOnlyMissingCreatePreservesDependentsOnlyByOptIn(t *testing.T) {
	for _, variant := range []string{"write-only-opt-in", "write-only-default", "readable-opt-in", "cached-replay"} {
		t.Run(variant, func(t *testing.T) {
			st := newStore()
			reg := NewRegistry()
			if variant == "readable-opt-in" {
				reg.Register(&inPlaceReadable{mem{name: "w", st: st}})
			} else {
				reg.Register(&inPlaceWO{wo: wo{mem{name: "w", st: st}}, enabled: variant != "write-only-default"})
			}
			reg.Register(&mem{name: "d", st: st})
			desired := []KV{kv("w", obj("q", "1")), kv("d", obj("x", "1", "w/q"))}
			s := New(reg, nil)
			mustApplied(t, s.Apply(context.Background(), desired, nil))
			st.reset()
			if variant == "cached-replay" {
				mustApplied(t, s.ApplyWith(context.Background(), desired, nil, ApplyOptions{Resync: true}))
			} else {
				if variant == "readable-opt-in" {
					st.mu.Lock()
					delete(st.objs, "w/q")
					st.mu.Unlock()
				}
				mustApplied(t, New(reg, nil).Apply(context.Background(), desired, nil))
			}
			got := strings.Join(st.ops(), ",")
			want := "delete d/x,create w/q,create d/x"
			if variant == "write-only-opt-in" {
				want = "create w/q"
			}
			if got != want {
				t.Fatalf("backend operations %q want %q", got, want)
			}
		})
	}
}

func TestWriteOnlyInPlaceFailureRetainsFailureSemantics(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		st := newStore()
		reg := NewRegistry()
		reg.Register(&inPlaceWO{wo: wo{mem{name: "w", st: st}}, enabled: true})
		reg.Register(&mem{name: "d", st: st})
		desired := []KV{kv("w", obj("q", "1")), kv("d", obj("x", "1", "w/q"))}
		mustApplied(t, New(reg, nil).Apply(context.Background(), desired, nil))
		st.reset()
		fail := errors.New("setter refused")
		if uncertain {
			fail = ErrUncertainOutcome
		}
		st.failOn["create:w/q"] = fail
		result := New(reg, nil).Apply(context.Background(), desired, nil)
		if result.Outcome == OutcomeApplied || !errors.Is(result.Err, fail) {
			t.Fatalf("setter failure lost: %+v", result)
		}
		if uncertain && result.Outcome != OutcomeDegraded {
			t.Fatalf("uncertainty became known state: %s", result.Outcome)
		}
		if len(st.ops()) != 0 {
			t.Fatalf("failed setter changed dependent: %v", st.ops())
		}
	}
}
