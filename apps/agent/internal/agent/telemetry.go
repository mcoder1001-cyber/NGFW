package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"go.fd.io/govpp/adapter/statsclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
	"google.golang.org/protobuf/types/known/timestamppb"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/vpe"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/vpp"
)

// ---- stats segment ------------------------------------------------------------------------------

// statsSource yields one consistent snapshot of the interface counters.
type statsSource interface {
	InterfaceStats() ([]api.InterfaceCounters, error)
}

// statsReader reads the VPP stats segment (AD-5: shared memory, no API round trip). It connects
// lazily and reconnects after errors (VPP restart remaps the segment).
type statsReader struct {
	path string
	log  *slog.Logger
	mu   sync.Mutex
	conn *core.StatsConnection
}

func newStatsReader(path string, log *slog.Logger) *statsReader {
	return &statsReader{path: path, log: log}
}

// InterfaceStats implements statsSource.
func (r *statsReader) InterfaceStats() ([]api.InterfaceCounters, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		c, err := core.ConnectStats(statsclient.NewStatsClient(r.path))
		if err != nil {
			return nil, fmt.Errorf("stats segment %s: %w", r.path, err)
		}
		r.conn = c
	}
	var st api.InterfaceStats
	if err := r.conn.GetInterfaceStats(&st); err != nil {
		r.conn.Disconnect()
		r.conn = nil
		return nil, fmt.Errorf("stats segment: %w", err)
	}
	return st.Interfaces, nil
}

func (r *statsReader) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		r.conn.Disconnect()
		r.conn = nil
	}
}

// statsBatch builds one StatsBatch from a snapshot, filtered and sorted by name.
func statsBatch(snap []api.InterfaceCounters, want map[string]bool, seq uint64, interval uint32, ts time.Time) *vrxv1.StatsBatch {
	b := &vrxv1.StatsBatch{Ts: timestamppb.New(ts), Seq: seq, IntervalMs: interval}
	for _, c := range snap {
		name := c.InterfaceName
		if name == "" {
			name = strconv.FormatUint(uint64(c.InterfaceIndex), 10)
		}
		if len(want) > 0 && !want[name] {
			continue
		}
		b.InterfaceCounters = append(b.InterfaceCounters, &vrxv1.InterfaceCounters{
			Name: name, SwIfIndex: c.InterfaceIndex,
			RxPackets: c.Rx.Packets, RxBytes: c.Rx.Bytes, TxPackets: c.Tx.Packets, TxBytes: c.Tx.Bytes,
			Drops: c.Drops, Errors: c.RxErrors + c.TxErrors, Punts: c.Punts, RxMisses: c.RxNoBuf + c.RxMiss,
		})
	}
	sort.Slice(b.InterfaceCounters, func(i, j int) bool { return b.InterfaceCounters[i].Name < b.InterfaceCounters[j].Name })
	return b
}

// streamStats runs one StreamStats subscription: a sampler at the requested interval feeding a
// small drop-oldest buffer, drained by send (the gRPC stream). Returns when ctx ends or send fails.
func streamStats(ctx context.Context, src statsSource, req *vrxv1.StreamStatsRequest, send func(*vrxv1.StatsBatch) error, log *slog.Logger) error {
	interval := req.GetIntervalMs()
	if interval == 0 {
		interval = 1000
	}
	want := map[string]bool{}
	for _, n := range req.GetInterfaces() {
		want[n] = true
	}
	const depth = 4
	buf := make(chan *vrxv1.StatsBatch, depth)
	go func() {
		defer close(buf)
		t := time.NewTicker(time.Duration(interval) * time.Millisecond)
		defer t.Stop()
		var seq uint64
		for {
			snap, err := src.InterfaceStats()
			seq++
			if err != nil {
				log.Warn("stats segment read failed", "err", err)
			} else {
				b := statsBatch(snap, want, seq, interval, time.Now())
				select {
				case buf <- b:
				default: // slow consumer: drop the oldest, keep the newest (seq shows the gap)
					select {
					case <-buf:
					default:
					}
					buf <- b
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	for b := range buf {
		if err := send(b); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// ---- link events ------------------------------------------------------------------------------

// watchLinks subscribes to sw_interface_event (want_interface_events) and publishes
// LINK_UP/LINK_DOWN until ctx ends or the connection drops.
func watchLinks(ctx context.Context, c vpp.Client, b *bus, log *slog.Logger) error {
	svc := interfaces.NewServiceClient(c)
	w, err := c.WatchEvent(ctx, &interfaces.SwInterfaceEvent{})
	if err != nil {
		return fmt.Errorf("watch sw_interface_event: %w", err)
	}
	defer w.Close()
	pid := uint32(os.Getpid()) //nolint:gosec // pid fits
	if _, err := svc.WantInterfaceEvents(ctx, &interfaces.WantInterfaceEvents{EnableDisable: 1, PID: pid}); err != nil {
		return fmt.Errorf("want_interface_events: %w", err)
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_, _ = svc.WantInterfaceEvents(cctx, &interfaces.WantInterfaceEvents{EnableDisable: 0, PID: pid})
	}()
	names := map[uint32]string{}
	lookup := func(idx uint32) string {
		if n, ok := names[idx]; ok {
			return n
		}
		st, err := svc.SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if err == nil {
			for {
				d, err := st.Recv()
				if err != nil {
					if !errors.Is(err, io.EOF) {
						log.Debug("sw_interface_dump for link event", "err", err)
					}
					break
				}
				if uint32(d.SwIfIndex) == idx {
					names[idx] = trimNul(d.InterfaceName)
				}
			}
		}
		if n, ok := names[idx]; ok {
			return n
		}
		return strconv.FormatUint(uint64(idx), 10)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case m, ok := <-w.Events():
			if !ok {
				return errors.New("event watcher closed")
			}
			ev, ok := m.(*interfaces.SwInterfaceEvent)
			if !ok {
				continue
			}
			idx := uint32(ev.SwIfIndex)
			name := lookup(idx)
			kind := vrxv1.EventKind_EVENT_KIND_LINK_DOWN
			if ev.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0 {
				kind = vrxv1.EventKind_EVENT_KIND_LINK_UP
			}
			admin := "down"
			if ev.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0 {
				admin = "up"
			}
			attrs := map[string]string{"sw_if_index": strconv.FormatUint(uint64(idx), 10), "admin": admin}
			if ev.Deleted {
				attrs["deleted"] = "true"
				kind = vrxv1.EventKind_EVENT_KIND_LINK_DOWN
				delete(names, idx)
			}
			n := name
			b.publish(&vrxv1.Event{Kind: kind, Interface: &n, Attributes: attrs, Message: fmt.Sprintf("%s link %s admin %s", name, map[bool]string{true: "up", false: "down"}[kind == vrxv1.EventKind_EVENT_KIND_LINK_UP], admin)})
		}
	}
}

// vppVersion asks show_version.
func vppVersion(ctx context.Context, c vpp.Client) (string, error) {
	rep, err := vpe.NewServiceClient(c).ShowVersion(ctx, &vpe.ShowVersion{})
	if err != nil {
		return "", err
	}
	return trimNul(rep.Version), nil
}

func trimNul(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return s[:i]
		}
	}
	return s
}
