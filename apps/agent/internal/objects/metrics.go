package objects

import (
	"fmt"
	"io"
	"sync/atomic"
)

// Process-wide counters of the objects domain, rendered by the agent's /metrics (WriteMetrics).
var (
	storeCorrupt       atomic.Int64 // corrupt store files moved aside at start (review F2)
	storePersistErrors atomic.Int64 // failed store writes (the changes stay in memory)
	fqdnExpired        atomic.Int64 // last-good FQDN answers dropped after the maximum staleness (D-129)
)

// WriteMetrics renders the counters in the Prometheus text format (called by the agent's metrics handler).
func WriteMetrics(w io.Writer) {
	p := func(name, help string, v int64) {
		_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, v)
	}
	p("vrx_agent_objects_store_corrupt_total", "Corrupt objects store files moved aside at start (the store starts empty, the next resync re-applies).", storeCorrupt.Load())
	p("vrx_agent_objects_store_persist_errors_total", "Failed writes of the objects store (the applied objects stay in memory).", storePersistErrors.Load())
	p("vrx_agent_objects_fqdn_stale_expired_total", "Last-good FQDN answers dropped after the maximum staleness (the object expands to nothing).", fqdnExpired.Load())
}
