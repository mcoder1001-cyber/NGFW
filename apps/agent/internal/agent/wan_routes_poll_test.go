package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWANRoutesEmptyPollingAndConfiguredRepair(t *testing.T) {
	for _, tc := range []struct {
		name   string
		states []bool
		counts []int32
		fails  map[int32]bool
	}{
		{"initial-retry", []bool{false, false, false}, []int32{1, 2, 2}, map[int32]bool{1: true}},
		{"withdraw-retry-restore", []bool{false, false, true, true, false, false, false, true, true}, []int32{1, 1, 2, 3, 4, 5, 5, 6, 7}, map[int32]bool{4: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ticks := make(chan time.Time)
			states := make(chan bool)
			done := make(chan struct{})
			var calls atomic.Int32
			go func() {
				defer close(done)
				runWANRoutes(ctx, ticks, func() bool { return <-states }, func(context.Context) error {
					n := calls.Add(1)
					if tc.fails[n] {
						return errors.New("finite failure")
					}
					return nil
				})
			}()
			for i, state := range tc.states {
				states <- state
				if i == len(tc.states)-1 {
					cancel()
					<-done
				} else {
					ticks <- time.Now()
				}
				if n := calls.Load(); n != tc.counts[i] {
					t.Fatalf("iteration%d configured%t calls%d want%d", i, state, n, tc.counts[i])
				}
			}
		})
	}
}
