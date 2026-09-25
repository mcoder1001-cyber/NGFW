package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TD-9 fix round 1, M1: govpp's own reply timeout ("no reply received within the timeout period …",
// govpp core.ErrReplyTimeout), also formatted with %v by a descriptor, is an unknown outcome: DEGRADED,
// never ROLLED_BACK.
func TestGovppReplyTimeoutIsUncertain(t *testing.T) {
	for name, cause := range map[string]error{
		"wrapped": fmt.Errorf("create_loopback: %w", errors.New("no reply received within the timeout period 30s")),
		"%v":      fmt.Errorf("sw_interface_add_del_address: %v", errors.New("no reply received within the timeout period 30s")),
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := hookFixture(t, func(_ context.Context, c string, k Key) (error, bool) {
				if c == "create" && k == "b/y" {
					return cause, true
				}
				return nil, false
			})
			r := s.Apply(context.Background(), []KV{kv("a", obj("x", "1")), kv("b", obj("y", "1", "a/x"))}, nil)
			if r.Outcome != OutcomeDegraded || !uncertainOf(r) {
				t.Fatalf("outcome %s uncertain %v, want DEGRADED", r.Outcome, uncertainOf(r))
			}
		})
	}
}
