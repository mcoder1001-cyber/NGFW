package ravpn

import (
	"context"
	"sync"
)

// observationGate keeps one adapter's two fresh socket activations together.
// Waiting consumes the existing caller budget; no capture or proof is cached.
type observationGate struct {
	once  sync.Once
	token chan struct{}
}

func (g *observationGate) acquire(ctx context.Context) bool {
	g.once.Do(func() { g.token = make(chan struct{}, 1) })
	if ctx.Err() != nil {
		return false
	}
	select {
	case g.token <- struct{}{}:
		if ctx.Err() != nil {
			g.release()
			return false
		}
		return true
	case <-ctx.Done():
		return false
	}
}
func (g *observationGate) release() { <-g.token }
