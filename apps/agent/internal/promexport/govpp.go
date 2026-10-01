package promexport

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/adapter/statsclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
)

// GovppSource owns an independent, lazily connected stats segment. Failed reads
// discard the mapping so the next scrape reconnects after VPP restarts.
// No binary API call is made from a scrape.
type GovppSource struct {
	mu      sync.Mutex
	path    string
	conn    statsSession
	raw     workerReader
	stopCtx context.Context
	stop    context.CancelFunc
	connect func(string) (statsSession, workerReader, error)
}

type statsSession interface {
	api.StatsProvider
	Disconnect()
}

func connectGovpp(path string) (statsSession, workerReader, error) {
	raw := statsclient.NewStatsClient(path, statsclient.SetSocketRetryTimeout(time.Second))
	conn, err := core.ConnectStats(raw)
	return conn, raw, err
}

// NewGovppSource creates a lazy reader of the configured VPP stats socket.
func NewGovppSource(path string) *GovppSource {
	if path == "" {
		path = adapter.DefaultStatsSocket
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &GovppSource{path: path, stopCtx: ctx, stop: cancel, connect: connectGovpp}
}

// Close releases the mapped stats segment; later reads reconnect.
func (s *GovppSource) Close() { s.mu.Lock(); defer s.mu.Unlock(); s.closeLocked() }

// Stop permanently cancels reads and disposes the mapping after active reads exit.
// Unlike Close, Stop never permits a queued or later scrape to reconnect.
func (s *GovppSource) Stop() {
	s.stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}
func (s *GovppSource) closeLocked() {
	if s.conn != nil {
		s.conn.Disconnect()
	}
	s.conn = nil
	s.raw = nil
}

// Read obtains dataplane counters, aborting and discarding the mapping on errors.
func (s *GovppSource) Read(ctx context.Context) (Snapshot, error) {
	ctx, cancel := context.WithCancel(ctx)
	stopCancel := context.AfterFunc(s.stopCtx, cancel)
	defer func() { stopCancel(); cancel() }()
	if err := s.stopCtx.Err(); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.stopCtx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if s.conn == nil {
		c, raw, err := s.connect(s.path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("connect VPP stats: %w", err)
		}
		s.conn = c
		s.raw = raw
	}
	snap, err := readGovpp(ctx, s.conn, s.raw)
	if stopped := s.stopCtx.Err(); stopped != nil {
		err = stopped
		snap = Snapshot{}
	}
	if err != nil {
		s.closeLocked()
	}
	return snap, err
}

type workerReader interface {
	DumpStats(...string) ([]adapter.StatEntry, error)
}

func readGovpp(ctx context.Context, p api.StatsProvider, raw workerReader) (Snapshot, error) {
	var interfaces api.InterfaceStats
	var buffers api.BufferStats
	var errors api.ErrorStats
	for _, read := range []func() error{func() error { return p.GetInterfaceStats(&interfaces) }, func() error { return p.GetBufferStats(&buffers) }, func() error { return p.GetErrorStats(&errors) }} {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		if err := read(); err != nil {
			return Snapshot{}, err
		}
	}
	entries, err := raw.DumpStats(core.NodeStatsPrefix)
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{}
	for _, i := range interfaces.Interfaces {
		name := i.InterfaceName
		if name == "" {
			name = fmt.Sprint(i.InterfaceIndex)
		}
		snap.Interfaces = append(snap.Interfaces, InterfaceStats{Name: name, RxBytes: i.Rx.Bytes, TxBytes: i.Tx.Bytes, RxPackets: i.Rx.Packets, TxPackets: i.Tx.Packets, RxErrors: i.RxErrors, TxErrors: i.TxErrors, Drops: i.Drops, StatsOnly: true})
	}
	sort.Slice(snap.Interfaces, func(i, j int) bool { return snap.Interfaces[i].Name < snap.Interfaces[j].Name })
	for name, b := range buffers.Buffer {
		snap.Buffers = append(snap.Buffers, BufferStats{Pool: name, Used: uint64(b.Used), Available: uint64(b.Available)})
	}
	sort.Slice(snap.Buffers, func(i, j int) bool { return snap.Buffers[i].Pool < snap.Buffers[j].Pool })
	for _, e := range errors.Errors {
		name := strings.TrimPrefix(e.CounterName, "/err/")
		node, reason, ok := strings.Cut(name, "/")
		if !ok {
			reason = "unknown"
		}
		var count uint64
		for _, v := range e.Values {
			count += v
		}
		snap.NodeErrors = append(snap.NodeErrors, NodeError{Node: node, Reason: reason, Count: count})
	}
	snap.Workers = workerCounters(entries)
	return snap, nil
}
func workerCounters(entries []adapter.StatEntry) []WorkerStats {
	var calls, vectors, clocks adapter.SimpleCounterStat
	for _, e := range entries {
		v, ok := e.Data.(adapter.SimpleCounterStat)
		if !ok {
			continue
		}
		switch string(e.Name) {
		case core.NodeStats_Calls:
			calls = v
		case core.NodeStats_Vectors:
			vectors = v
		case core.NodeStats_Clocks:
			clocks = v
		}
	}
	n := max(len(calls), len(vectors), len(clocks))
	out := make([]WorkerStats, 0, n)
	sum := func(s adapter.SimpleCounterStat, i int) uint64 {
		var n uint64
		if i < len(s) {
			for _, v := range s[i] {
				n += uint64(v)
			}
		}
		return n
	}
	for i := 0; i < n; i++ {
		name := "vpp_main"
		if i > 0 {
			name = fmt.Sprintf("vpp_worker_%d", i)
		}
		w := WorkerStats{Name: name}
		c, v, k := sum(calls, i), sum(vectors, i), sum(clocks, i)
		if c > 0 {
			w.VectorsPerCall = float64(v) / float64(c)
		}
		if v > 0 {
			w.Clocks = float64(k) / float64(v)
		}
		out = append(out, w)
	}
	return out
}
