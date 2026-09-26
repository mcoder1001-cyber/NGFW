package subsystems

// Close seam (review Q4, manager): a family with background work (F-object-model's FQDN resolver and
// store flush; later P11, P12, F-wireguard) registers a closer from its own file with w.OnClose(f);
// Agent.Stop calls w.Close() once, after the gRPC server and the service have stopped. Closers run in
// reverse registration order. They are kept beside the Wiring rather than in its struct, so the seam
// needs no edit of subsystems.go.

import "sync"

var (
	closersMu sync.Mutex
	closers   = map[*Wiring][]func(){}
)

// OnClose registers f to run when the agent stops (Wiring.Close).
func (w *Wiring) OnClose(f func()) {
	closersMu.Lock()
	defer closersMu.Unlock()
	closers[w] = append(closers[w], f)
}

// Close runs the registered closers, last registered first; a second call does nothing.
func (w *Wiring) Close() {
	closersMu.Lock()
	fs := closers[w]
	delete(closers, w)
	closersMu.Unlock()
	for i := len(fs) - 1; i >= 0; i-- {
		fs[i]()
	}
}
