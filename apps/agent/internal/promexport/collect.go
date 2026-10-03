package promexport

import (
	"context"
	"io"
	"sort"
)

// InterfaceStats is one interface's counters (absolute, from the stats segment).
type InterfaceStats struct {
	Name string
	// StatsOnly means link flags and directional drops are unavailable in the stats segment.
	StatsOnly            bool
	Drops                uint64
	RxBytes, TxBytes     uint64
	RxPackets, TxPackets uint64
	RxDrops, TxDrops     uint64
	RxErrors, TxErrors   uint64
	AdminUp, LinkUp      bool
}

// WorkerStats is one VPP worker thread's load (vpp_main plus each worker).
type WorkerStats struct {
	Name           string
	VectorsPerCall float64
	// Clocks is the per-vector clock cost (a load proxy on hosts with no worker threads).
	Clocks float64
}

// BufferStats is one buffer pool.
type BufferStats struct {
	Pool            string
	Used, Available uint64
}

// NodeError is one (node, reason) error counter (top-N by count).
type NodeError struct {
	Node, Reason string
	Count        uint64
}

// Snapshot is one read of the stats segment. A StatsSource returns it without blocking on the VPP binary API.
type Snapshot struct {
	Interfaces []InterfaceStats
	Workers    []WorkerStats
	Buffers    []BufferStats
	NodeErrors []NodeError
}

// StatsSource reads the data-plane stats. The box implements it over the VPP stats segment; tests use a fake.
type StatsSource interface {
	Read(ctx context.Context) (Snapshot, error)
}

// MaxNodeErrors bounds how many (node, reason) error counters the exporter emits (top-N by count).
const MaxNodeErrors = 50

// Collect writes every family from one snapshot in the text exposition format. `prefix` is prepended to the
// interface label (the slot prefix in tests, "" on the product). It returns the source's error unwritten.
func Collect(ctx context.Context, src StatsSource, prefix string, w io.Writer) error {
	snap, err := src.Read(ctx)
	if err != nil {
		return err
	}
	e := newWriter(w)

	e.family("ngfw_interface_rx_bytes_total", "counter", "Bytes received on the interface.")
	for _, i := range snap.Interfaces {
		e.counter("ngfw_interface_rx_bytes_total", ifl(prefix, i.Name), i.RxBytes)
	}
	e.family("ngfw_interface_tx_bytes_total", "counter", "Bytes transmitted on the interface.")
	for _, i := range snap.Interfaces {
		e.counter("ngfw_interface_tx_bytes_total", ifl(prefix, i.Name), i.TxBytes)
	}
	e.family("ngfw_interface_rx_packets_total", "counter", "Packets received on the interface.")
	for _, i := range snap.Interfaces {
		e.counter("ngfw_interface_rx_packets_total", ifl(prefix, i.Name), i.RxPackets)
	}
	e.family("ngfw_interface_tx_packets_total", "counter", "Packets transmitted on the interface.")
	for _, i := range snap.Interfaces {
		e.counter("ngfw_interface_tx_packets_total", ifl(prefix, i.Name), i.TxPackets)
	}
	e.family("ngfw_interface_drops_total", "counter", "Packets dropped on the interface (VPP aggregate).")
	for _, i := range snap.Interfaces {
		if i.StatsOnly {
			e.counter("ngfw_interface_drops_total", ifl(prefix, i.Name), i.Drops)
		}
	}
	e.family("ngfw_interface_rx_drops_total", "counter", "Packets dropped on receive.")
	for _, i := range snap.Interfaces {
		if i.StatsOnly {
			continue
		}
		e.counter("ngfw_interface_rx_drops_total", ifl(prefix, i.Name), i.RxDrops)
	}
	e.family("ngfw_interface_tx_drops_total", "counter", "Packets dropped on transmit.")
	for _, i := range snap.Interfaces {
		if i.StatsOnly {
			continue
		}
		e.counter("ngfw_interface_tx_drops_total", ifl(prefix, i.Name), i.TxDrops)
	}
	e.family("ngfw_interface_rx_errors_total", "counter", "Receive errors on the interface.")
	for _, i := range snap.Interfaces {
		e.counter("ngfw_interface_rx_errors_total", ifl(prefix, i.Name), i.RxErrors)
	}
	e.family("ngfw_interface_tx_errors_total", "counter", "Transmit errors on the interface.")
	for _, i := range snap.Interfaces {
		e.counter("ngfw_interface_tx_errors_total", ifl(prefix, i.Name), i.TxErrors)
	}
	e.family("ngfw_interface_admin_up", "gauge", "Interface administrative state (1 = up).")
	for _, i := range snap.Interfaces {
		if i.StatsOnly {
			continue
		}
		e.gauge("ngfw_interface_admin_up", ifl(prefix, i.Name), b2f(i.AdminUp))
	}
	e.family("ngfw_interface_link_up", "gauge", "Interface link/carrier state (1 = up).")
	for _, i := range snap.Interfaces {
		if i.StatsOnly {
			continue
		}
		e.gauge("ngfw_interface_link_up", ifl(prefix, i.Name), b2f(i.LinkUp))
	}

	e.family("ngfw_worker_vectors_per_call", "gauge", "Average vectors per graph-node call (VPP load indicator).")
	for _, wk := range snap.Workers {
		e.gauge("ngfw_worker_vectors_per_call", map[string]string{"worker": wk.Name}, wk.VectorsPerCall)
	}
	e.family("ngfw_worker_clocks_per_vector", "gauge", "Average clock cycles per vector.")
	for _, wk := range snap.Workers {
		e.gauge("ngfw_worker_clocks_per_vector", map[string]string{"worker": wk.Name}, wk.Clocks)
	}

	e.family("ngfw_buffer_used", "gauge", "Buffers in use per pool.")
	for _, bp := range snap.Buffers {
		e.gauge("ngfw_buffer_used", map[string]string{"pool": bp.Pool}, float64(bp.Used))
	}
	e.family("ngfw_buffer_available", "gauge", "Buffers available per pool.")
	for _, bp := range snap.Buffers {
		e.gauge("ngfw_buffer_available", map[string]string{"pool": bp.Pool}, float64(bp.Available))
	}
	e.family("ngfw_buffer_used_percent", "gauge", "Percent of the pool's buffers in use.")
	for _, bp := range snap.Buffers {
		e.gauge("ngfw_buffer_used_percent", map[string]string{"pool": bp.Pool}, usedPercent(bp))
	}

	e.family("ngfw_node_errors_total", "counter", "Data-plane node error counters (top-N by count).")
	for _, ne := range topErrors(snap.NodeErrors) {
		e.counter("ngfw_node_errors_total", map[string]string{"node": ne.Node, "reason": ne.Reason}, ne.Count)
	}
	return e.flush()
}

func ifl(prefix, name string) map[string]string {
	if prefix != "" {
		name = prefix + name
	}
	return map[string]string{"interface": name}
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func usedPercent(bp BufferStats) float64 {
	total := bp.Used + bp.Available
	if total == 0 {
		return 0
	}
	return float64(bp.Used) * 100 / float64(total)
}

// topErrors returns the highest-count node errors (deterministic: count desc, then node, then reason).
func topErrors(in []NodeError) []NodeError {
	out := append([]NodeError(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Node != out[j].Node {
			return out[i].Node < out[j].Node
		}
		return out[i].Reason < out[j].Reason
	})
	if len(out) > MaxNodeErrors {
		out = out[:MaxNodeErrors]
	}
	return out
}
