package vpntest

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/scheduler"
)

// Plan computes the scheduler's diff for the given descriptors exactly as
// internal/scheduler/descriptor.go specifies step 3 (P05 implements it; until it merges the DF-5
// host checks use this): actual = union of Retrieve(); desired key absent → Create; present and
// proto.Equal → nothing; present and different → Update; owned actual key absent from desired →
// Delete. Write-only descriptors (Retrieve → vpn.ErrRetrieveUnsupported, D-063) are skipped: the
// reconciler re-applies them on resync instead of diffing.
func Plan(t testing.TB, ds []scheduler.Descriptor, desired []proto.Message) scheduler.Plan {
	t.Helper()
	actual := map[scheduler.Key]scheduler.KV{}
	writeOnly := map[string]bool{}
	for _, d := range ds {
		kvs, err := d.Retrieve(Context(t))
		if errors.Is(err, vpn.ErrRetrieveUnsupported) {
			writeOnly[d.Name()] = true
			continue
		}
		if err != nil {
			t.Fatalf("%s Retrieve: %v", d.Name(), err)
		}
		for _, kv := range kvs {
			actual[kv.Key] = kv
		}
	}
	var plan scheduler.Plan
	want := map[scheduler.Key]bool{}
	for _, v := range desired {
		var d scheduler.Descriptor
		for _, c := range ds {
			if descriptorMessages[c.Name()] == string(proto.MessageName(v)) {
				d = c
				break
			}
		}
		if d == nil {
			t.Fatalf("no descriptor for %T", v)
		}
		if writeOnly[d.Name()] {
			continue
		}
		k := d.KeyOf(v)
		want[k] = true
		switch a, ok := actual[k]; {
		case !ok:
			plan.Create = append(plan.Create, scheduler.KV{Key: k, Value: v})
		case !proto.Equal(a.Value, v):
			plan.Update = append(plan.Update, scheduler.KV{Key: k, Value: v, Meta: a.Meta})
		}
	}
	for k, a := range actual {
		if !want[k] {
			plan.Delete = append(plan.Delete, a)
		}
	}
	return plan
}

// descriptorMessages maps each DF-5 descriptor to its desired-state message (every DF-5
// descriptor has its own message type, so a desired value finds its descriptor by type).
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

// MustEmptyPlan fails t unless applying desired again would plan nothing, and logs the result
// (the "same desired state twice → empty plan" evidence).
func MustEmptyPlan(t testing.TB, ds []scheduler.Descriptor, desired []proto.Message) {
	t.Helper()
	p := Plan(t, ds, desired)
	if !p.Empty() {
		t.Fatalf("second apply of the same desired state is not a no-op: %d create %v, %d update %v, %d delete %v",
			len(p.Create), keys(p.Create), len(p.Update), keys(p.Update), len(p.Delete), keys(p.Delete))
	}
	t.Logf("idempotency: same desired state (%d objects) again → plan: 0 create, 0 update, 0 delete (Empty=%v)", len(desired), p.Empty())
}

func keys(kvs []scheduler.KV) []string {
	out := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		out = append(out, fmt.Sprint(kv.Key))
	}
	return out
}
