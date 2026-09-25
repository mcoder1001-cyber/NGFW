package agent

// IpfixState RPC (F-ipfix-sflow, docs/contracts/proto.md §11): the live flow-export state behind
// GET /api/v1/state/ipfix. Dumps and getters only — never mutates VPP.

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/adapter/statsclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	fpapi "ngfw/agent/binapi/flowprobe"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/flowprobe"
	"ngfw/agent/internal/descriptors/ipfix"
	"ngfw/agent/internal/descriptors/sflow"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// sflowCounterPattern selects the sFlow node's error counters in the stats segment.
const sflowCounterPattern = "^/err/sflow/"

// counterSource reads named counters (the stats segment; a stub in tests).
type counterSource func(pattern string) ([]*vrxv1.IpfixCounter, error)

// IpfixState implements the IpfixState RPC.
func (g *server) IpfixState(ctx context.Context, req *vrxv1.IpfixStateRequest) (*vrxv1.IpfixStateResponse, error) {
	var counters counterSource
	if r, ok := g.stats.(*statsReader); ok {
		counters = statsSegmentCounters(r.path)
	}
	return g.svc.IpfixState(ctx, req, counters)
}

// statsSegmentCounters reads counters from the VPP stats segment at path (one short-lived
// connection per call: the RPC is polled by a UI, not streamed).
func statsSegmentCounters(path string) counterSource {
	return func(pattern string) ([]*vrxv1.IpfixCounter, error) {
		c := statsclient.NewStatsClient(path)
		if err := c.Connect(); err != nil {
			return nil, fmt.Errorf("stats segment %s: %w", path, err)
		}
		defer func() { _ = c.Disconnect() }()
		entries, err := c.DumpStats(pattern)
		if err != nil {
			return nil, fmt.Errorf("stats segment: %w", err)
		}
		return countersOf(entries), nil
	}
}

// countersOf sums scalar/error counters over workers, sorted by name.
func countersOf(entries []adapter.StatEntry) []*vrxv1.IpfixCounter {
	var out []*vrxv1.IpfixCounter
	for _, e := range entries {
		var sum uint64
		switch d := e.Data.(type) {
		case adapter.ErrorStat:
			for _, v := range d {
				sum += uint64(v)
			}
		case adapter.ScalarStat:
			sum = uint64(d)
		case adapter.SimpleCounterStat:
			for _, w := range d {
				for _, v := range w {
					sum += uint64(v)
				}
			}
		default:
			continue
		}
		out = append(out, &vrxv1.IpfixCounter{Name: string(e.Name), Value: sum})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].GetName() < out[b].GetName() })
	return out
}

// IpfixState builds the flow-export snapshot. counters (nil: no stats segment) reads the sFlow
// counters.
func (s *Service) IpfixState(ctx context.Context, req *vrxv1.IpfixStateRequest, counters counterSource) (*vrxv1.IpfixStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	if !s.vpp.Connected() {
		return nil, status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	fail := func(err error) error {
		if errors.Is(err, vpp.ErrDisconnected) {
			return status.Error(codes.Unavailable, err.Error())
		}
		return status.Errorf(codes.Internal, "ipfix state: %v", err)
	}
	resp := &vrxv1.IpfixStateResponse{Owner: s.owner, GlobalsOwner: desired.IpfixGlobalsOwner(), RetrievedAt: timestamppb.New(s.now())}

	// exporter 0: read directly (for a non-owner its descriptor is write-only)
	e0, ok, err := ipfix.NewDefaultExporter(s.vpp).Current(ctx)
	if err != nil {
		return nil, fail(err)
	}
	if ok {
		resp.Exporters = append(resp.Exporters, s.exporterState(e0, true, nil))
	}
	kvs, err := s.sched.Retrieve(ctx, scheduler.Only(ipfix.NameExporter, flowprobe.NameInterface, sflow.NameInterface))
	if err != nil {
		return nil, fail(err)
	}
	var stats interface {
		StatIndex(string) (uint32, bool)
	}
	if d, ok := s.sched.Registry().Get(ipfix.NameExporter); ok {
		stats, _ = d.(*ipfix.ExporterDescriptor)
	}
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case ipfix.NameExporter:
			var e ipfix.Exporter
			if dfkit.Decode(kv.Value, &e) != nil {
				continue
			}
			var idx *uint32
			if stats != nil {
				if i, ok := stats.StatIndex(e.Collector); ok {
					idx = proto.Uint32(i)
				}
			}
			resp.Exporters = append(resp.Exporters, s.exporterState(e, false, idx))
		case flowprobe.NameInterface:
			var f flowprobe.Interface
			if dfkit.Decode(kv.Value, &f) == nil {
				resp.FlowprobeInterfaces = append(resp.FlowprobeInterfaces, &vrxv1.IpfixFlowprobeInterfaceState{Interface: f.Interface, Which: f.Which, Direction: f.Direction})
			}
		case sflow.NameInterface:
			var i sflow.Interface
			if dfkit.Decode(kv.Value, &i) == nil {
				st := &vrxv1.IpfixSflowInterfaceState{Interface: i.Interface}
				if m, ok := kv.Meta.(sflow.InterfaceMeta); ok {
					st.HwIfIndex = m.HwIfIndex
				}
				resp.SflowInterfaces = append(resp.SflowInterfaces, st)
			}
		}
	}
	sort.Slice(resp.FlowprobeInterfaces, func(a, b int) bool {
		return resp.FlowprobeInterfaces[a].GetInterface() < resp.FlowprobeInterfaces[b].GetInterface()
	})
	sort.Slice(resp.SflowInterfaces, func(a, b int) bool {
		return resp.SflowInterfaces[a].GetInterface() < resp.SflowInterfaces[b].GetInterface()
	})

	p, err := fpapi.NewServiceClient(s.vpp).FlowprobeGetParams(ctx, &fpapi.FlowprobeGetParams{})
	if err != nil {
		return nil, fail(fmt.Errorf("flowprobe_get_params: %w", err))
	}
	if p.RecordFlags != 0 {
		resp.FlowprobeParams = &vrxv1.IpfixFlowprobeParamsState{
			RecordL2: p.RecordFlags&fpapi.FLOWPROBE_RECORD_FLAG_L2 != 0, RecordL3: p.RecordFlags&fpapi.FLOWPROBE_RECORD_FLAG_L3 != 0,
			RecordL4: p.RecordFlags&fpapi.FLOWPROBE_RECORD_FLAG_L4 != 0, ActiveTimerSec: p.ActiveTimer, PassiveTimerSec: p.PassiveTimer,
		}
	}
	g, err := sflow.NewGlobal(s.vpp).Current(ctx)
	if err != nil {
		return nil, fail(err)
	}
	resp.SflowGlobal = &vrxv1.IpfixSflowGlobalState{SamplingN: g.SamplingRate, PollingIntervalSec: g.PollingInterval,
		HeaderBytes: g.HeaderBytes, Direction: g.Direction, DropMonitoring: g.DropMonitoring}

	if counters == nil {
		resp.Notes = append(resp.Notes, "sFlow counters unavailable: no stats segment reader")
	} else if cs, err := counters(sflowCounterPattern); err != nil {
		resp.Notes = append(resp.Notes, "sFlow counters unavailable: "+err.Error())
	} else {
		resp.SflowCounters = cs
	}
	if len(resp.SflowInterfaces) > 0 {
		resp.Notes = append(resp.Notes, "VPP samples on the sFlow interfaces; export to collectors needs hsflowd, which this build does not ship")
	}
	if !resp.GlobalsOwner {
		resp.Notes = append(resp.Notes, "this agent is not the VPP-globals owner (D-071): exporter 0, flowprobe and sFlow parameters are only required, never set")
	}
	return resp, nil
}

func (s *Service) exporterState(e ipfix.Exporter, def bool, statIndex *uint32) *vrxv1.IpfixExporterState {
	vrf := "none"
	if e.VRF != ipfix.NoVRF {
		vrf = s.tableName(e.VRF)
	}
	name := desired.IpfixExporterName(e.Collector)
	return &vrxv1.IpfixExporterState{
		Name: name, DefaultExporter: def, Collector: e.Collector, CollectorPort: uint32(e.CollectorPort),
		SourceAddress: e.Src, Vrf: vrf, PathMtu: e.PathMTU, TemplateIntervalSec: e.TemplateInterval,
		UdpChecksum: e.UDPChecksum, StatIndex: statIndex,
	}
}
