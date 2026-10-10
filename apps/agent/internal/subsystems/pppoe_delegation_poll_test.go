package subsystems

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"ngfw/agent/internal/desired"
)

func TestPppoeDelegationEmptyPollingRetriesAndTransitions(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ticks := make(chan time.Time)
	observed := make(chan int)
	finished := make(chan struct{})
	lease := desired.PppoeDelegationLease{Logical: "wan", Generation: "live", Ready: true}
	snapshots := [][]desired.PppoeDelegationLease{nil, nil, nil, {lease}, {lease}, nil, nil}
	var applied []int
	failures := 0
	go func() {
		defer close(finished)
		index := -1
		runPppoeDelegation(ctx, ticks, func() []desired.PppoeDelegationLease {
			index++
			return snapshots[index]
		}, func(leases []desired.PppoeDelegationLease) {
			observed <- index
		}, func(context.Context) error {
			applied = append(applied, index)
			if index == 0 {
				return errors.New("initial withdrawal failed")
			}
			return nil
		}, func(error) { failures++ })
	}()
	for step := range snapshots {
		select {
		case got := <-observed:
			if got != step {
				t.Fatalf("snapshot %d, want %d", got, step)
			}
		case <-time.After(time.Second):
			t.Fatalf("polling stopped at snapshot %d", step)
		}
		if step == len(snapshots)-1 {
			cancel()
			break
		}
		select {
		case ticks <- time.Now():
		case <-time.After(time.Second):
			t.Fatalf("polling did not consume tick %d", step)
		}
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("source did not stop on cancellation")
	}
	// Retry the failed initial empty cleanup; suppress successful empty repeats;
	// preserve every active tick; withdraw once when the active lease disappears.
	if want := []int{0, 1, 3, 4, 5}; !reflect.DeepEqual(applied, want) {
		t.Fatalf("reconciled snapshots %v, want %v", applied, want)
	}
	if failures != 1 {
		t.Fatalf("failure callbacks %d, want 1", failures)
	}
}
