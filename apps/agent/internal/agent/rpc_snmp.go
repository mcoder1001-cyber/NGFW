package agent

// F-snmp: the SnmpState RPC (docs/contracts/proto.md §11) — read-only state of the snmpd renderer stage
// (internal/subsystems/snmp.go). Credentials are named, never shown.

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/subsystems"
)

const snmpStateTimeout = 5 * time.Second

// SnmpState implements the SnmpState RPC.
func (g *server) SnmpState(ctx context.Context, req *vrxv1.SnmpStateRequest) (*vrxv1.SnmpStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	st, ok := subsystems.SnmpStageOf(g.svc.owner)
	if !ok {
		return nil, status.Error(codes.Unavailable, "this agent has no snmpd stage")
	}
	ctx, cancel := context.WithTimeout(ctx, snmpStateTimeout)
	defer cancel()
	v, err := st.State(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "snmpd state: %v", err)
	}
	return snmpStateResponse(v), nil
}

func snmpStateResponse(v *subsystems.SnmpStateView) *vrxv1.SnmpStateResponse {
	out := &vrxv1.SnmpStateResponse{Configured: v.Configured, EngineId: v.EngineID, PendingAction: v.PendingAction}
	if d := v.Daemon; d != nil {
		out.Reachable, out.Endpoint, out.Credential = d.Reachable, d.Endpoint, d.Credential
		out.SysName, out.SysDescr, out.SysLocation, out.SysContact = d.SysName, d.SysDescr, d.SysLocation, d.SysContact
		out.SysUpTime, out.Error = d.SysUpTime, d.Error
	}
	if s := v.Subagent; s != nil {
		out.SubagentRegistered, out.SubagentRegistrations, out.SubagentError = s.Registered, s.Registrations, s.LastError
	}
	return out
}
