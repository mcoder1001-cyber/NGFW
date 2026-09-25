package agent

import (
	"testing"

	"ngfw/agent/internal/renderers/snmpd"
	"ngfw/agent/internal/snmpagent"
	"ngfw/agent/internal/subsystems"
)

// TestSnmpStateResponse maps the stage view field by field (no credential value exists in the view).
func TestSnmpStateResponse(t *testing.T) {
	got := snmpStateResponse(&subsystems.SnmpStateView{
		Configured: true, EngineID: "8000", PendingAction: "snmpd needs restart",
		Daemon:   &snmpd.State{Reachable: true, Endpoint: "127.0.0.1:161", Credential: "v3 user noc", SysName: "vrx-a", SysUpTime: 42}, //nolint:gosec // fixture, no credential
		Subagent: &snmpagent.Status{Registered: true, Registrations: 2},
	})
	if !got.GetConfigured() || !got.GetReachable() || got.GetCredential() != "v3 user noc" || got.GetSysName() != "vrx-a" ||
		got.GetSysUpTime() != 42 || !got.GetSubagentRegistered() || got.GetSubagentRegistrations() != 2 || got.GetPendingAction() == "" {
		t.Fatalf("%v", got)
	}
}
