package scheduler

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestRecreationIncludesPendingPrerequisiteClosureAndRollsItBack(t *testing.T) {
	for _, failLater := range []bool{false, true} {
		t.Run(map[bool]string{false: "converges-once", true: "rollback-removes-early-prerequisites"}[failLater], func(t *testing.T) {
			s, st := fixture(t)
			before := []KV{kv("a", obj("base", "imm:old")), kv("b", obj("client", "old", "if/base"))}
			mustApplied(t, s.Apply(t.Context(), before, nil))
			st.reset()
			desired := []KV{
				kv("a", obj("base", "imm:new")),
				kv("a", obj("helper", "new", "if/base")),
				kv("a", obj("extra", "new", "if/helper")),
				kv("b", obj("client", "new", "if/base", "if/extra", "?a/not-configured")),
			}
			if failLater {
				st.failOn["create:c/late"] = errors.New("injected later failure")
				desired = append(desired, kv("c", obj("late", "new", "b/client")))
			}
			result := s.Apply(t.Context(), desired, nil)
			prefix := []string{"delete b/client", "delete a/base", "create a/base", "create a/helper", "create a/extra", "create b/client"}
			ops := st.ops()
			if len(ops) < len(prefix) || !reflect.DeepEqual(ops[:len(prefix)], prefix) {
				t.Fatalf("prerequisite order %v", ops)
			}
			if !failLater {
				mustApplied(t, result)
				if !reflect.DeepEqual(ops, prefix) {
					t.Fatalf("pending CREATE executed twice: %v", ops)
				}
				if len(st.objs) != 4 {
					t.Fatalf("optional absent dependency fabricated: %v", st.objs)
				}
			} else {
				if result.Outcome != OutcomeRolledBack {
					t.Fatalf("rollback outcome: %+v", result)
				}
				if len(st.objs) != 2 {
					t.Fatalf("early prerequisite leaked after rollback: %v", st.objs)
				}
				for _, v := range before {
					if !proto.Equal(st.objs[v.Key], v.Value) {
						t.Fatalf("old object not restored: %s", v.Key)
					}
				}
				if !strings.Contains(strings.Join(ops, ","), "delete b/client,delete a/extra,delete a/helper,delete a/base,create a/base,create b/client") {
					t.Fatalf("rollback journal order %v", ops)
				}
			}
		})
	}
}
