// Package hastatesync runs bounded native NAT44-EI HA actions.
package hastatesync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	natapi "ngfw/agent/binapi/nat44_ei"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/hasync"
	"ngfw/agent/internal/vpp"
)

var correlation atomic.Uint32

// ErrMissed indicates that native completion reported unacknowledged messages.
var ErrMissed = errors.New("NAT HA resync completed with unacknowledged messages")

// Observation records the latest completion in this agent process.
type Observation struct {
	CompletedAt time.Time
	MissedCount uint32
	Completed   uint64
}

// Runtime owns action role enforcement and bounded completion observations.
type Runtime struct {
	Client       vpp.Client
	GlobalsOwner bool
	LockDir      string
	Timeout      time.Duration
	mu           sync.Mutex
	latest       Observation
}

// Observation returns a consistent snapshot of the last native completion.
func (r *Runtime) Observation() Observation { r.mu.Lock(); defer r.mu.Unlock(); return r.latest }

// Run executes a globals-owner action and waits for correlated native completion.
func (r *Runtime) Run(ctx context.Context, op ngfwv1.HaSyncOp) (Observation, error) {
	if !r.GlobalsOwner {
		return Observation{}, dfkit.ErrNotGlobalsOwner
	}
	if op != ngfwv1.HaSyncOp_HA_SYNC_OP_RESYNC && op != ngfwv1.HaSyncOp_HA_SYNC_OP_FLUSH {
		return Observation{}, dfkit.Specf("unknown HA sync operation")
	}
	if r.Client == nil || !r.Client.Connected() {
		return Observation{}, vpp.ErrDisconnected
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dir := r.LockDir
	if dir == "" {
		dir = "/run/lock"
	}
	release, err := hasync.Lock(ctx, dir)
	if err != nil {
		return Observation{}, err
	}
	defer release()
	native := natapi.NewServiceClient(r.Client)
	listener, err := hasync.ReadListener(ctx, r.Client)
	if err != nil {
		return Observation{}, err
	}
	peer, err := hasync.ReadFailover(ctx, r.Client)
	if err != nil {
		return Observation{}, err
	}
	if listener.Port == 0 || peer.Port == 0 {
		return Observation{}, dfkit.Specf("NAT HA listener and peer must be enabled before action")
	}
	if op == ngfwv1.HaSyncOp_HA_SYNC_OP_FLUSH {
		_, err = native.Nat44EiHaFlush(ctx, &natapi.Nat44EiHaFlush{})
		return Observation{}, err
	}
	watcher, err := r.Client.WatchEvent(ctx, &natapi.Nat44EiHaResyncCompletedEvent{})
	if err != nil {
		return Observation{}, err
	}
	defer watcher.Close()
	pid := correlation.Add(1)
	if pid == 0 {
		pid = correlation.Add(1)
	}
	if _, err = native.Nat44EiHaResync(ctx, &natapi.Nat44EiHaResync{WantResyncEvent: 1, PID: pid}); err != nil {
		return Observation{}, err
	}
	for {
		select {
		case <-ctx.Done():
			return Observation{}, ctx.Err()
		case message, ok := <-watcher.Events():
			if !ok {
				return Observation{}, fmt.Errorf("HA completion watcher closed before matching event")
			}
			event, ok := message.(*natapi.Nat44EiHaResyncCompletedEvent)
			if !ok || event.PID != pid {
				continue
			}
			r.mu.Lock()
			r.latest.CompletedAt = time.Now().UTC()
			r.latest.MissedCount = event.MissedCount
			r.latest.Completed++
			out := r.latest
			r.mu.Unlock()
			if event.MissedCount > 0 {
				return out, fmt.Errorf("%w: %d", ErrMissed, event.MissedCount)
			}
			return out, nil
		}
	}
}
