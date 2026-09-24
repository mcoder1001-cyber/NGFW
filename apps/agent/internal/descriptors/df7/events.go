package df7

import (
	"context"
	"fmt"
	"os"

	"go.fd.io/govpp/api"

	"ngfw/agent/internal/vpp"
)

// Watch is the common event plumbing of bfd/vrrp/igmp (StreamEvents): subscribe to the event
// message, enable the plugin's want_* registration, decode every event with decode (which
// returns ok=false for events to drop, e.g. other owners' interfaces) and deliver it on the
// returned channel until ctx ends; then the registration is disabled and the channel closed.
// want is called with true to register and false to unregister, with this process' PID.
func Watch[E any](ctx context.Context, c vpp.Client, event api.Message, want func(ctx context.Context, enable bool, pid uint32) error,
	decode func(ctx context.Context, m api.Message) (E, bool)) (<-chan E, error) {
	w, err := c.WatchEvent(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", event.GetMessageName(), err)
	}
	pid := uint32(os.Getpid()) //nolint:gosec // PIDs fit in u32
	if err := want(ctx, true, pid); err != nil {
		w.Close()
		return nil, err
	}
	out := make(chan E, 64)
	go func() {
		defer close(out)
		defer func() { _ = want(context.WithoutCancel(ctx), false, pid) }()
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-w.Events():
				if !ok {
					return
				}
				e, keep := decode(ctx, m)
				if !keep {
					continue
				}
				select {
				case out <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
