package hoststack

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"ngfw/agent/binapi/session"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestHostStackOnHost is the ONE host check of F-host-stack: namespaces + session rules with this
// slot's prefixed ids, only when the host session layer is ALREADY on (it is never enabled,
// disabled or re-engined here, D-012/D-071). Retrieve == desired, `vppctl show session rules`
// lists the rules, an agent-restart simulation (new descriptors over the same boot store) finds
// them again, rollback clears them.
func TestHostStackOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, "session", &session.SessionRuleAddDel{}, &session.SessionRulesV2Dump{}, &session.AppNamespaceAddDelV4{})
	c := h.Client()
	ctx := context.Background()
	on, err := Probe(ctx, c)
	if err != nil || !on {
		t.Skipf("host session layer is not on (probe: on=%v err=%v): never enabled by tests (D-012); fake-client tests cover the path", on, err)
	}
	slot := vpptest.Slot(t)
	owner := h.Owner
	boot := dfkit.NewMemoryBootStore()
	r := scheduler.NewRegistry()
	Register(r, c, owner, WithBootStore(boot))
	st, _ := lookupState(owner)
	nd, rd := newNamespace(c, owner, st), newRule(c, owner, st)
	n := Namespace{ID: owner + "-ns", Vrf: 0}
	rules := []Rule{
		{Tag: owner + "-r4", Scope: "global", Transport: "tcp", Local: fmt.Sprintf("10.%d.1.0/24", slot), LocalPort: port(slot, 0),
			Remote: fmt.Sprintf("10.%d.2.0/24", slot), Action: "deny"},
		{Tag: owner + "-r6", Scope: "local", Transport: "udp", Local: fmt.Sprintf("fd00:%d::/48", slot), Remote: "::/0",
			RemotePort: port(slot, 1), Action: "allow", AppNamespace: n.ID},
	}
	cleanup := func() {
		for _, x := range rules {
			_ = rd.Delete(context.Background(), x.Proto(), nil)
		}
		_ = nd.Delete(context.Background(), n.Proto(), nil)
	}
	t.Cleanup(cleanup)
	for range 2 {
		if _, err := nd.Create(ctx, n.Proto()); err != nil {
			t.Fatalf("namespace: %v", err)
		}
		for _, x := range rules {
			if _, err := rd.Create(ctx, x.Proto()); err != nil {
				t.Fatalf("rule %s: %v", x.Tag, err)
			}
		}
	}
	kvs := []scheduler.KV{dfkittest.KV(rd, rules[0].Proto()), dfkittest.KV(rd, rules[1].Proto())}
	dfkittest.AssertEmptyPlan(t, rd, kvs...)
	if out, err := exec.Command("vppctl", "show", "session", "rules", "tcp").CombinedOutput(); err == nil {
		t.Logf("vppctl show session rules tcp:\n%s", out)
		if !strings.Contains(string(out), rules[0].Tag) {
			t.Errorf("vppctl does not list %s", rules[0].Tag)
		}
	}
	// agent restart simulation
	Register(scheduler.NewRegistry(), c, owner, WithBootStore(boot))
	st2, _ := lookupState(owner)
	rd2 := newRule(c, owner, st2)
	dfkittest.AssertEmptyPlan(t, rd2, kvs...)
	cleanup()
	if left := dfkittest.MustRetrieve(t, rd2); len(left) != 0 {
		t.Fatalf("rollback left %v", left)
	}
}

// port is this slot's rule port 3<slot>90+i (envelope: ports 3<SLOT>90–3<SLOT>99).
func port(slot, i int) uint32 {
	var p uint32
	_, _ = fmt.Sscanf(fmt.Sprintf("3%d9%d", slot, i), "%d", &p)
	return p
}
