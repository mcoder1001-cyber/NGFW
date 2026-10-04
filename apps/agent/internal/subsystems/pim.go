package subsystems

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mfib"
	syncpim "ngfw/agent/internal/frrsync/pim"
	_ "ngfw/agent/internal/renderers/frr/pim" // Register section and S2 lines before FRR runtime construction.
	"ngfw/agent/internal/scheduler"
	"os"
	"path/filepath"
	"sync"
)

// registerPim owns an exclusive dynamic descriptor, outside configuration Domains.
// The existing PIM contract is default-VRF only; numbered lab ranges cannot own
// table zero, so they register no dynamic source and never touch global forwarding.
func registerPim(r scheduler.Registry, w *Wiring) error {
	rt := FRRRuntime(w.env.Owner)
	if !rt.Enabled() {
		return nil
	}
	ids, e := w.IDRange()
	if e != nil {
		if errors.Is(e, ErrNoIDRange) {
			return nil
		}
		return e
	}
	if ids != nil && (ids.Empty() || ids.Lo > 0) {
		return nil
	}
	r.Register(mfib.NewNamed(syncpim.Descriptor, w.env.Client, w.env.Owner, w.BootStore(), df7.WithIDs(ids.DF7())))
	source := &syncpim.Source{Read: func(ctx context.Context) ([]byte, error) {
		if _, e := os.Stat(filepath.Join(rt.paths.SocketDir(), "pimd.vty")); e != nil {
			return nil, fmt.Errorf("PIM daemon socket unavailable")
		}
		return syncpim.Reader(rt.r.Show)(ctx)
	}, OnError: pimStatusReporter(rt.log, w.Publish)}
	return w.AddDynamicSource(DynamicSource{Name: "pim", Descriptors: []string{syncpim.Descriptor}, Desired: source.Desired, Run: func(ctx context.Context, syncState SyncFunc) { source.Run(ctx, syncState) }})
}

// pimStatusReporter emits one operator-visible diagnostic per failure transition,
// and clears the condition only after a complete successful observation + sync.
// Raw daemon/parser/scheduler error payloads are deliberately not logged/published.
func pimStatusReporter(log *slog.Logger, publish func(*ngfwv1.Event)) func(error) {
	var mu sync.Mutex
	failed := false
	return func(err error) {
		nowFailed := err != nil
		mu.Lock()
		changed := nowFailed != failed
		failed = nowFailed
		mu.Unlock()
		if !changed {
			return
		}
		status := "recovered"
		message := "PIM synchronization recovered"
		kind := ngfwv1.EventKind_EVENT_KIND_ROUTING_CHANGED
		if nowFailed {
			status = "degraded"
			message = "PIM synchronization unavailable; retaining last supported snapshot"
			kind = ngfwv1.EventKind_EVENT_KIND_ERROR
			if log != nil {
				log.Warn("PIM synchronization unavailable", "component", "pim", "status", status, "reason", "observation-or-sync-failed")
			}
		} else if log != nil {
			log.Info("PIM synchronization recovered", "component", "pim", "status", status)
		}
		if publish != nil {
			publish(&ngfwv1.Event{Kind: kind, Message: message, Attributes: map[string]string{"source": "pim", "component": "pim", "status": status, "reason": "observation-or-sync-state"}})
		}
	}
}
