package agent

// F-unbound-chrony-syslog: the read-only NTP state RPC.

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// NtpState implements the NtpState RPC.
func (g *server) NtpState(ctx context.Context, req *vrxv1.NtpStateRequest) (*vrxv1.NtpStateResponse, error) {
	hs, err := g.hostServices(req.GetOwner())
	if err != nil {
		return nil, err
	}
	r := hs.Chrony.Renderer()
	resp := &vrxv1.NtpStateResponse{Owner: g.svc.owner, RetrievedAt: timestamppb.New(g.svc.now()), ConfigPath: r.Paths().Conf()}
	st, err := r.State(ctx)
	if err != nil {
		resp.Error = err.Error()
	}
	resp.Running, resp.ServerStats = st.Running, st.ServerStats
	if t := st.Tracking; t != nil {
		resp.Tracking = &vrxv1.NtpTracking{
			RefId: t.RefID, RefName: t.RefName, Stratum: int32(t.Stratum), RefTime: t.RefTime, SystemTime: t.SystemTime, //nolint:gosec // stratum 0..16
			LastOffset: t.LastOffset, RmsOffset: t.RMSOffset, Frequency: t.Frequency, ResidualFreq: t.ResidualFreq,
			Skew: t.Skew, RootDelay: t.RootDelay, RootDispersion: t.RootDispersion, UpdateInterval: t.UpdateInterval, Leap: t.Leap,
		}
	}
	for _, s := range st.Sources {
		resp.Sources = append(resp.Sources, &vrxv1.NtpSource{
			Mode: s.Mode, State: s.State, Name: s.Name, Stratum: int32(s.Stratum), Poll: int32(s.Poll), Reach: s.Reach, //nolint:gosec // chrony ranges
			LastRx: s.LastRx, Offset: s.Offset, Measured: s.Measure, Error: s.Error,
		})
	}
	for _, s := range st.SourceStats {
		resp.SourceStats = append(resp.SourceStats, &vrxv1.NtpSourceStats{
			Name: s.Name, Np: int32(s.NP), Nr: int32(s.NR), Span: int32(s.Span), Frequency: s.Frequency, //nolint:gosec // chrony ranges
			FreqSkew: s.FreqSkew, Offset: s.Offset, StdDev: s.StdDev,
		})
	}
	for _, p := range hs.Chrony.Pending(ctx) {
		resp.PendingActions = append(resp.PendingActions, &vrxv1.ServiceDaemonAction{Daemon: p.Daemon, Unit: p.Unit, Action: p.Action, Reason: p.Reason})
	}
	return resp, nil
}
