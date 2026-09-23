// Package restarttest is the DF-8 restart-safety simulation against the host VPP (FAST MODE DoD
// 3, reviews since D-071): agent 1 applies a desired state, a fresh agent (new connection, new
// descriptor instances, no memory) must plan nothing; then the objects are deleted through the
// binary API (simulated loss) and the fresh agent must plan exactly their re-creation, apply it
// and plan nothing again. Only dump-backed descriptors take part (write-only ones cannot be
// retrieved by definition, D-063).
package restarttest

import (
	"context"
	"fmt"
	"net/netip"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/descriptors/flowprobe"
	"ngfw/agent/internal/descriptors/ipfix"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/descriptors/sflow"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

type step struct {
	d       scheduler.Descriptor
	desired []proto.Message
}

func (s step) kvs() []scheduler.KV {
	out := make([]scheduler.KV, 0, len(s.desired))
	for _, v := range s.desired {
		out = append(out, dfkittest.KV(s.d, v))
	}
	return out
}

// agent builds one "agent process": fresh descriptors on client c (no learned state).
func agent(t *testing.T, c vpp.Client, owner string, ifs [3]string) []step {
	t.Helper()
	slot := vpptest.Slot(t)
	base := vpptest.TableBase(t)
	pool := netip.MustParsePrefix(fmt.Sprintf("10.%d.96.0/24", slot)) // disjoint from the ipfix test's collectors
	ip := func(x, y int) string { return fmt.Sprintf("10.%d.%d.%d", slot, x, y) }
	vrfScope := dhcp.WithVRFScope(func(v uint32) bool { return v >= base+900 && v < base+1000 }) // disjoint from the dhcp test
	return []step{
		{dhcp.NewProxy(c, vrfScope), []proto.Message{
			dhcp.Proxy{RxVRF: base + 901, Server: ip(95, 1), Src: ip(95, 2)}.Proto(),
		}},
		{dhcp.NewClient(c, owner), []proto.Message{
			dhcp.Client{Interface: ifs[0], Hostname: owner + "-rst"}.Proto(),
		}},
		{ipfix.NewExporter(c, ipfix.WithCollectorScope(pool.Contains)), []proto.Message{
			ipfix.Exporter{Collector: ip(96, 1), CollectorPort: 4739, Src: ip(96, 2), PathMTU: 1400, TemplateInterval: 20}.Proto(),
		}},
		{flowprobe.NewParams(c, flowprobe.WithGlobals(dfkit.GlobalsOwner(true))), []proto.Message{
			flowprobe.Params{RecordL3: true, ActiveTimer: 15, PassiveTimer: 120}.Proto(),
		}},
		{flowprobe.NewInterface(c, owner), []proto.Message{
			flowprobe.Interface{Interface: ifs[1], Which: "ip4", Direction: "rx"}.Proto(),
		}},
		{sflow.NewInterface(c, owner), []proto.Message{
			sflow.Interface{Interface: ifs[1]}.Proto(),
		}},
		{lcp.NewItfPair(c, owner), []proto.Message{
			lcp.ItfPair{Interface: ifs[2], HostIfName: owner + "-rst0", HostIfType: "tap"}.Proto(),
		}},
	}
}

// reconcile plans every step and applies Creates (in step order) and Deletes (reverse order);
// it returns the number of planned operations.
func reconcile(t *testing.T, ctx context.Context, steps []step) int {
	t.Helper()
	total := 0
	plans := make([]scheduler.Plan, len(steps))
	for i, s := range steps {
		plans[i] = dfkittest.DiffPlan(s.kvs(), dfkittest.MustRetrieve(t, s.d))
		t.Logf("plan %-22s %s", s.d.Name(), oneLine(plans[i]))
		total += plans[i].Len()
	}
	for i, s := range steps {
		for _, kv := range plans[i].Create {
			if _, err := s.d.Create(ctx, kv.Value); err != nil {
				t.Fatalf("%s create: %v", kv.Key, err)
			}
		}
		for _, kv := range plans[i].Update {
			if _, err := s.d.Update(ctx, kv.Value, kv.Value, kv.Meta); err != nil {
				t.Fatalf("%s update: %v", kv.Key, err)
			}
		}
	}
	for i := len(steps) - 1; i >= 0; i-- {
		for _, kv := range plans[i].Delete {
			if err := steps[i].d.Delete(ctx, kv.Value, kv.Meta); err != nil {
				t.Fatalf("%s delete: %v", kv.Key, err)
			}
		}
	}
	return total
}

func oneLine(p scheduler.Plan) string {
	if p.Empty() {
		return "(empty)"
	}
	s := ""
	for _, kv := range p.Create {
		s += " create " + string(kv.Key)
	}
	for _, kv := range p.Update {
		s += " update " + string(kv.Key)
	}
	for _, kv := range p.Delete {
		s += " delete " + string(kv.Key)
	}
	return s
}

func deleteAll(ctx context.Context, t *testing.T, steps []step) {
	t.Helper()
	for i := len(steps) - 1; i >= 0; i-- {
		for _, v := range steps[i].desired {
			if err := steps[i].d.Delete(ctx, v, nil); err != nil {
				t.Errorf("%s delete: %v", steps[i].d.Name(), err)
			}
		}
	}
}

func TestRestartSimulationOnHost(t *testing.T) {
	h1 := dfkittest.ConnectHost(t)
	h1.LockGlobals(t)
	h1.Owner += "r" // its own owner: the per-package tests of the same slot run in parallel
	ctx := context.Background()
	if kvs := dfkittest.MustRetrieve(t, flowprobe.NewParams(h1.Client(), flowprobe.WithGlobals(dfkit.GlobalsOwner(true)))); len(kvs) != 0 {
		t.Skipf("flowprobe params are held by someone else (%v)", kvs[0].Value)
	}
	if ns, err := lcp.NewDefaultNetns(h1.Client()).Current(ctx); err != nil || ns != "" {
		t.Skipf("lcp default netns %q / %v: pairs would land elsewhere", ns, err)
	}
	var ifs [3]string
	for i := range ifs {
		ifs[i], _ = h1.Loopback(t, 93+i)
	}

	t.Log("== agent 1: apply the desired state")
	a1 := agent(t, h1.Client(), h1.Owner, ifs)
	t.Cleanup(func() { deleteAll(context.Background(), t, a1) })
	if n := reconcile(t, ctx, a1); n == 0 {
		t.Fatal("agent 1 planned nothing on an empty VPP")
	}
	if n := reconcile(t, ctx, a1); n != 0 {
		t.Fatalf("agent 1 re-apply planned %d op(s)", n)
	}

	t.Log("== agent restart: fresh connection, fresh descriptors → empty plan")
	h2 := dfkittest.ConnectHost(t)
	h2.Owner = h1.Owner
	a2 := agent(t, h2.Client(), h2.Owner, ifs)
	if n := reconcile(t, ctx, a2); n != 0 {
		t.Fatalf("fresh agent planned %d op(s) against unchanged VPP state", n)
	}

	t.Log("== simulated loss: delete the objects through the binary API")
	deleteAll(ctx, t, a2)

	t.Log("== fresh agent after the loss: plan = re-create everything, apply, then empty plan")
	a3 := agent(t, h2.Client(), h2.Owner, ifs)
	want := 0
	for _, s := range a3 {
		want += len(s.desired)
	}
	if n := reconcile(t, ctx, a3); n != want {
		t.Fatalf("after the loss the plan has %d op(s), want %d creates", n, want)
	}
	dfkittest.HoldForEvidence(t, "restart simulation: objects re-created")
	if n := reconcile(t, ctx, a3); n != 0 {
		t.Fatalf("after re-creation the plan has %d op(s)", n)
	}
}
