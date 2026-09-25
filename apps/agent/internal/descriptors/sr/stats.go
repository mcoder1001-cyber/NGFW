package sr

import (
	"context"
	"fmt"

	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
)

// Counters are a local SID's traffic counters (sr_localsids_with_packet_stats_details): "good"
// traffic was processed by the SID's behaviour, "bad" traffic was dropped by it. F-srv6
// (Srv6State); read-only status, never part of Retrieve.
type Counters struct {
	GoodPackets, GoodBytes, BadPackets, BadBytes uint64
}

// LocalSidCounters reads the counters of every local SID VPP has, keyed by canonical SID text
// (every owner's SIDs: the caller filters by claim).
func LocalSidCounters(ctx context.Context, c vpp.Client) (map[string]Counters, error) {
	stream, err := srapi.NewServiceClient(c).SrLocalsidsWithPacketStatsDump(ctx, &srapi.SrLocalsidsWithPacketStatsDump{})
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("sr_localsids_with_packet_stats_dump: %w", err))
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, df6.PluginError(Plugin, fmt.Errorf("sr_localsids_with_packet_stats_dump: %w", err))
	}
	out := make(map[string]Counters, len(recs))
	for _, r := range recs {
		sid := df6.IP6String(r.Addr)
		if sid == "" {
			continue
		}
		out[sid] = Counters{
			GoodPackets: r.GoodTrafficPktCount, GoodBytes: r.GoodTrafficBytes,
			BadPackets: r.BadTrafficPktCount, BadBytes: r.BadTrafficBytes,
		}
	}
	return out, nil
}
