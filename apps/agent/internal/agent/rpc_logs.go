package agent

// F-unbound-chrony-syslog: the remote-syslog export state and the log explorer.

import (
	"context"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	ucsaction "ngfw/agent/internal/actions/unbound-chrony-syslog"
	"ngfw/agent/internal/renderers"
)

var (
	journalRunnerOnce sync.Once
	journalRunner     renderers.Runner
)

// logRunner is the log explorer's runner (journalctl only; tests replace journalRunner before the first call).
func logRunner() renderers.Runner {
	journalRunnerOnce.Do(func() {
		if journalRunner == nil {
			journalRunner = ucsaction.NewRunner()
		}
	})
	return journalRunner
}

// SyslogState implements the SyslogState RPC.
func (g *server) SyslogState(ctx context.Context, req *vrxv1.SyslogStateRequest) (*vrxv1.SyslogStateResponse, error) {
	hs, err := g.hostServices(req.GetOwner())
	if err != nil {
		return nil, err
	}
	release, err := syslogWalk.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	r := hs.Rsyslog.Renderer()
	resp := &vrxv1.SyslogStateResponse{Owner: g.svc.owner, RetrievedAt: timestamppb.New(g.svc.now()), ConfigPath: r.Paths().ConfFile, Inputs: map[string]int64{}}
	st, err := r.State(ctx)
	switch {
	case err != nil:
		resp.Error = err.Error()
	default:
		resp.Error = st.Error
		for k, v := range st.Inputs {
			resp.Inputs[k] = v
		}
		for i, t := range st.Targets {
			resp.Targets = append(resp.Targets, &vrxv1.SyslogTargetState{
				Index: uint32(i), Action: t.Name, Target: t.Target, Protocol: t.Protocol, Reported: t.Reported, //nolint:gosec // ≤ 16 targets
				Processed: t.Processed, Failed: t.Failed, Suspended: t.Suspended, SuspendedDuration: t.SuspendedDuration,
				Resumed: t.Resumed, QueueSize: t.QueueSize, Enqueued: t.Enqueued, Full: t.Full,
				DiscardedFull: t.DiscardedFull, DiscardedNf: t.DiscardedNF, MaxQueueSize: t.MaxQueueSize,
			})
		}
	}
	for _, p := range hs.Rsyslog.Pending(ctx) {
		resp.PendingActions = append(resp.PendingActions, &vrxv1.ServiceDaemonAction{Daemon: p.Daemon, Unit: p.Unit, Action: p.Action, Reason: p.Reason})
	}
	return resp, nil
}

// SyslogEntries implements the SyslogEntries RPC (the log explorer).
func (g *server) SyslogEntries(ctx context.Context, req *vrxv1.SyslogEntriesRequest) (*vrxv1.SyslogEntriesResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	q, err := ucsaction.ParseRequest(req, g.svc.now())
	if err != nil {
		return nil, err
	}
	release, err := journalWalk.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	resp, err := ucsaction.Run(ctx, logRunner(), q)
	if err != nil {
		return nil, err
	}
	resp.Owner, resp.RetrievedAt = g.svc.owner, timestamppb.New(g.svc.now())
	return resp, nil
}
