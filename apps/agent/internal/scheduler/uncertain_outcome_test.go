package scheduler

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestTypedUncertainOutcomeStopsDependentDeletion(t *testing.T) {
	s, st := fixture(t)
	desired := []KV{kv("a", obj("pair", "1")), kv("b", obj("admission", "1", "a/pair"))}
	mustApplied(t, s.Apply(context.Background(), desired, nil))
	st.reset()
	cause := errors.New("readback unavailable and compensation failed")
	st.failOn["delete:b/admission"] = errors.Join(ErrUncertainOutcome, cause)
	result := s.Apply(context.Background(), nil, nil)
	if result.Outcome != OutcomeDegraded || !result.Uncertain {
		t.Fatalf("unexpected result %+v", result)
	}
	if !errors.Is(result.Err, cause) {
		t.Fatal("lost cause", result.Err)
	}
	for _, operation := range st.ops() {
		if operation == "delete a/pair" {
			t.Fatal("deleted pair after unknown admission outcome")
		}
	}
}
func TestUncertainMarkerSurvivesWrapping(t *testing.T) {
	if !uncertain(fmt.Errorf("adapter: %w", ErrUncertainOutcome)) {
		t.Fatal("wrapped marker ignored")
	}
	if uncertain(errors.New("ordinary rejection")) {
		t.Fatal("ordinary rejection marked uncertain")
	}
}

// Model a command that committed but lost its reply, whose readback and local
// compensation failed. This is an actual mutation before the typed failure.
type committedUnknownDelete struct{ *mem }

func (d *committedUnknownDelete) Delete(ctx context.Context, value proto.Message, meta any) error {
	if err := d.mem.Delete(ctx, value, meta); err != nil {
		return err
	}
	return errors.Join(ErrUncertainOutcome, errors.New("IO reply lost; readback and compensation unavailable"))
}
func TestCommittedUnknownDeleteDegradesBeforePairRemoval(t *testing.T) {
	st := newStore()
	registry := NewRegistry()
	registry.Register(&mem{name: "a", st: st})
	registry.Register(&committedUnknownDelete{&mem{name: "b", st: st}})
	engine := New(registry, nil)
	engine.VerifyRetries = 0
	desired := []KV{kv("a", obj("pair", "1")), kv("b", obj("admission", "1", "a/pair"))}
	mustApplied(t, engine.Apply(context.Background(), desired, nil))
	st.reset()
	result := engine.Apply(context.Background(), nil, nil)
	if result.Outcome != OutcomeDegraded || !result.Uncertain {
		t.Fatalf("%+v", result)
	}
	if _, exists := st.objs[Join("b", "admission")]; exists {
		t.Fatal("fixture did not commit deletion")
	}
	if _, exists := st.objs[Join("a", "pair")]; !exists {
		t.Fatal("deleted pair after uncertain admission deletion")
	}
	if len(st.ops()) != 1 || st.ops()[0] != "delete b/admission" {
		t.Fatal(st.ops())
	}
}
