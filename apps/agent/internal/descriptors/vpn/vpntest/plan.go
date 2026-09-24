package vpntest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Agent is one simulated agent process for the host checks: P05's reconciler over the given
// DF-5 descriptors plus DF-1's observe-only interface alias (the interface/<name> dependency
// target, D-065). A restart simulation builds a second Agent (fresh descriptors, fresh scheduler,
// only the persisted record store shared) and plans the same desired state.
type Agent struct {
	S *scheduler.Scheduler
}

// NewAgent registers ds (and DF-1's alias descriptor for owner on c) with a new scheduler.
func NewAgent(c vpp.Client, owner string, ds ...scheduler.Descriptor) *Agent {
	reg := scheduler.NewRegistry()
	reg.Register(iface.NewAlias(c, owner))
	for _, d := range ds {
		reg.Register(d)
	}
	return &Agent{S: scheduler.New(reg, nil)}
}

// KVs turns desired values into scheduler KVs (each DF-5 descriptor has its own message type).
func (a *Agent) KVs(t testing.TB, desired []proto.Message) []scheduler.KV {
	t.Helper()
	out := make([]scheduler.KV, 0, len(desired))
	for _, v := range desired {
		var d scheduler.Descriptor
		for name, msg := range descriptorMessages {
			if msg == string(proto.MessageName(v)) {
				d, _ = a.S.Registry().Get(name)
				break
			}
		}
		if d == nil {
			t.Fatalf("no registered descriptor for %T", v)
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v})
	}
	return out
}

// Plan computes P05's plan for desired (every registered descriptor in scope) and fails t on
// validation issues.
func (a *Agent) Plan(ctx context.Context, t testing.TB, desired []proto.Message) *scheduler.TxnPlan {
	t.Helper()
	p, err := a.S.Plan(ctx, a.KVs(t, desired), scheduler.All)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(p.Issues) > 0 {
		t.Fatalf("plan issues: %v", p.Issues)
	}
	return p
}

// Apply applies desired through P05 and fails t unless the outcome is APPLIED.
func (a *Agent) Apply(ctx context.Context, t testing.TB, desired []proto.Message) *scheduler.TxnResult {
	t.Helper()
	r := a.S.Apply(ctx, a.KVs(t, desired), scheduler.All)
	if r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply: %s: %v (results %+v)", r.Outcome, r.Err, r.Results)
	}
	t.Logf("apply: %s %+v", r.Outcome, r.Summary)
	return r
}

// writeOnly reports whether descriptor name cannot be retrieved (D-063): P05 re-applies its
// objects in a fresh process, so they are not drift.
func (a *Agent) writeOnly(ctx context.Context, name string) bool {
	d, ok := a.S.Registry().Get(name)
	if !ok {
		return false
	}
	_, err := d.Retrieve(ctx)
	return scheduler.IsRetrieveUnsupported(err)
}

// MustEmptyPlan fails t unless planning desired again changes nothing retrievable, and logs the
// plan (the "same desired state twice → empty plan" evidence). Operations on write-only
// descriptors (re-applied by P05 in a process that has not applied them yet, D-063) are listed
// separately and do not count.
func (a *Agent) MustEmptyPlan(ctx context.Context, t testing.TB, what string, desired []proto.Message) {
	t.Helper()
	p := a.Plan(ctx, t, desired)
	var drift, reapply []string
	for _, op := range p.Ops {
		s := op.Op + " " + string(op.Key)
		if a.writeOnly(ctx, op.Key.Descriptor()) {
			reapply = append(reapply, s)
			continue
		}
		drift = append(drift, s)
	}
	if len(drift) > 0 {
		t.Fatalf("%s: plan is not empty: %s", what, strings.Join(drift, "; "))
	}
	t.Logf("%s: P05 plan of the same desired state (%d objects): 0 create, 0 update, 0 delete, %d unchanged; write-only re-apply: [%s]",
		what, len(desired), p.Unchanged, strings.Join(reapply, "; "))
}

// PlanString renders a plan on one line.
func PlanString(p *scheduler.TxnPlan) string {
	if p.Empty() {
		return "(empty)"
	}
	parts := make([]string, 0, len(p.Ops))
	for _, op := range p.Ops {
		parts = append(parts, fmt.Sprintf("%s %s", op.Op, op.Key))
	}
	return strings.Join(parts, "; ")
}

// descriptorMessages maps each DF-5 descriptor to its desired-state message.
var descriptorMessages = map[string]string{
	"ipsec.spd": "vrx.agent.vpn.v1.IpsecSpd", "ipsec.spd-interface": "vrx.agent.vpn.v1.IpsecSpdInterface",
	"ipsec.spd-entry": "vrx.agent.vpn.v1.IpsecSpdEntry", "ipsec.sa": "vrx.agent.vpn.v1.IpsecSa",
	"ipsec.tunnel-protect": "vrx.agent.vpn.v1.IpsecTunnelProtect", "ipsec.itf": "vrx.agent.vpn.v1.IpsecItf",
	"ipsec.backend": "vrx.agent.vpn.v1.IpsecBackend", "ipsec.async-mode": "vrx.agent.vpn.v1.IpsecAsyncMode",
	"ikev2.profile": "vrx.agent.vpn.v1.Ikev2Profile", "ikev2.local-key": "vrx.agent.vpn.v1.Ikev2LocalKey",
	"ikev2.sleep-interval": "vrx.agent.vpn.v1.Ikev2SleepInterval", "ikev2.liveness": "vrx.agent.vpn.v1.Ikev2Liveness",
	"ikev2.responder-hostname": "vrx.agent.vpn.v1.Ikev2ResponderHostname",
	"wireguard.interface":      "vrx.agent.vpn.v1.WireguardInterface", "wireguard.peer": "vrx.agent.vpn.v1.WireguardPeer",
	"wireguard.async-mode": "vrx.agent.vpn.v1.WireguardAsyncMode",
}
