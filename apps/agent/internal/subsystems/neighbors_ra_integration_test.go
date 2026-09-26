package subsystems

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"ngfw/agent/binapi/ethernet_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/vlib"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

func cli(t *testing.T, c vpp.Client, cmd string) string {
	t.Helper()
	r, err := vlib.NewServiceClient(c).CliInband(df2test.Ctx(t), &vlib.CliInband{Cmd: cmd})
	if err != nil {
		t.Fatalf("cli_inband %q: %v", cmd, err)
	}
	return r.Reply
}

// TestNeighborWatchOnHost (VRX_INTEGRATION=1, host VPP): the watcher subscribes to want_ip_neighbor_events_v2 one
// interface at a time — the watcher table (`show ip neighbor-watcher`) shows our pid on our loopback's sw_if_index, never
// on ~0 — and a neighbour learned on (here: added to) that interface arrives as ONE coalesced EVENT_KIND_NEIGHBOR_CHANGED
// with the interface's logical name; its removal as another. The product agent publishes these once TD-8 wires
// Env.Publish (questions Q3); this test drives the same RunNeighborWatch with its own sink.
func TestNeighborWatchOnHost(t *testing.T) {
	c := df2test.Connect(t) // skips unless VRX_INTEGRATION=1; shared lab lock for the run
	owner := vpptest.Prefix(t)
	name, idx := df2test.Loopback(t, c, 71)
	df2test.AddAddress(t, c, idx, fmt.Sprintf("10.%d.71.1/24", vpptest.Slot(t)))
	var got events
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	const pid = 0x6e7261 // "nra"
	go func() {
		defer close(done)
		RunNeighborWatch(ctx, NeighborWatchConfig{Client: c, Owner: owner, Publish: got.publish, Tick: 250 * time.Millisecond, Rescan: time.Hour, PID: pid})
	}()
	t.Cleanup(func() { cancel(); <-done })

	watchLine := func() []string {
		var ours []string
		for _, l := range strings.Split(cli(t, c, "show ip neighbor-watcher"), "\n") {
			if strings.Contains(l, fmt.Sprint(pid)) || strings.Contains(l, name) {
				ours = append(ours, strings.TrimSpace(l))
			}
		}
		return ours
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(watchLine()) == 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if l := watchLine(); len(l) > 0 {
		t.Logf("show ip neighbor-watcher (lines naming %s or pid %d):\n%s", name, pid, strings.Join(l, "\n"))
	} else {
		t.Logf("show ip neighbor-watcher (whole table; loop sw_if_index %d):\n%s", idx, cli(t, c, "show ip neighbor-watcher"))
	}

	hw, _ := ethernet_types.ParseMacAddress("02:00:00:00:97:01")
	ip, _ := df2.ParseAddr(fmt.Sprintf("10.%d.71.9", vpptest.Slot(t)))
	nb := ip_neighbor.IPNeighbor{SwIfIndex: interface_types.InterfaceIndex(idx), MacAddress: hw, IPAddress: df2.ToAddress(ip)}
	svc := ip_neighbor.NewServiceClient(c)
	if _, err := svc.IPNeighborAddDel(df2test.Ctx(t), &ip_neighbor.IPNeighborAddDel{IsAdd: true, Neighbor: nb}); err != nil {
		t.Fatal(err)
	}
	wait := func(what string, cond func([]*vrxv1.Event) bool) []*vrxv1.Event {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if evs := got.snapshot(); cond(evs) {
				return evs
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s: %v", what, got.snapshot())
		return nil
	}
	evs := wait("the ADDED event", func(evs []*vrxv1.Event) bool { return len(evs) >= 1 })
	if e := evs[0]; e.GetInterface() != name || e.GetAttributes()["added"] != "1" || e.GetKind() != vrxv1.EventKind_EVENT_KIND_NEIGHBOR_CHANGED {
		t.Fatalf("event %v", e)
	}
	t.Logf("event: kind=%s interface=%s message=%q attributes=%v", evs[0].GetKind(), evs[0].GetInterface(), evs[0].GetMessage(), evs[0].GetAttributes())
	if _, err := svc.IPNeighborAddDel(df2test.Ctx(t), &ip_neighbor.IPNeighborAddDel{IsAdd: false, Neighbor: nb}); err != nil {
		t.Fatal(err)
	}
	evs = wait("the REMOVED event", func(evs []*vrxv1.Event) bool {
		for _, e := range evs {
			if e.GetAttributes()["removed"] == "1" {
				return true
			}
		}
		return false
	})
	t.Logf("event: kind=%s interface=%s message=%q attributes=%v", evs[len(evs)-1].GetKind(), evs[len(evs)-1].GetInterface(), evs[len(evs)-1].GetMessage(), evs[len(evs)-1].GetAttributes())
	for _, l := range watchLine() {
		if strings.Contains(l, "4294967295") {
			t.Fatalf("a watcher on every interface: %q", l)
		}
	}
}
