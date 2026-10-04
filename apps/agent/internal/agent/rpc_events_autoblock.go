package agent

import (
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/detectors"
)

// autoBlockObserved preserves the validated kernel destination port so the API
// can refresh repeated probes without counting one port multiple times.
func autoBlockObserved(ob detectors.Observation) *ngfwv1.Event {
	attrs := map[string]string{"source_ip": ob.Source, "detector": ob.Kind}
	if ob.Kind == "portScan" {
		attrs["destination_port"] = ob.Port
	}
	return &ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_AUTOBLOCK_OBSERVED, Attributes: attrs}
}
