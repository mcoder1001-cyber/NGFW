package agent

// F-mpls-srmpls host check (VRX_INTEGRATION=1, shared lab lock, slot prefix): the in-process agent with the product
// wiring against the VPP on this host — the ONE integration check of the feature.
//
//	always:            a slot MPLS table (<base>+1), label routes in it (swap/push on a loopback next hop, pop and
//	                   look up in a slot VRF), an MPLS tunnel; apply → Retrieve == canonical, `vppctl show mpls fib
//	                   table <t>` / `show mpls tunnel`; idempotent re-apply; restart simulation (stop the agent, delete
//	                   the label routes and the tunnel behind its back — routes before tables, V15 —, start: back within
//	                   30 s); MplsState pages; rollback: routes, then tunnel, then table removed, no stray entry.
//	VRX_DF7_GLOBALS=1  (a manager window: D-071/D-082, `flock -x /run/lock/vrx-globals.lock`) additionally needs MPLS
//	                   table 0: it is created for the test only if VPP has none (and deleted again), and the check adds
//	                   MPLS on the loopback, a label binding, a table-0 label route, an SR-MPLS policy + steering, and
//	                   the SR policy in the restart simulation (`show sr mpls policies`).
//
// The agent runs with GlobalsOwner=false (test slots are never the globals owner): table 0 is only required. No packet
// is sent (no rig, no af_packet). VPP is never restarted; NRestarts is logged before and after.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	mplsapi "ngfw/agent/binapi/mpls"
	srmplsapi "ngfw/agent/binapi/sr_mpls"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

func mplsHostLog(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("vppctl", args...).CombinedOutput() //nolint:gosec // fixed show commands of this test
	if err != nil {
		t.Logf("vppctl %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

func vppRestarts(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts").CombinedOutput()
	if err != nil {
		t.Logf("systemctl show vpp -p NRestarts: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// mplsTables dumps the MPLS tables (id → name).
func mplsTables(t *testing.T, c vpp.Client) map[uint32]string {
	t.Helper()
	st, err := mplsapi.NewServiceClient(c).MplsTableDump(context.Background(), &mplsapi.MplsTableDump{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint32]string{}
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out[d.MtTable.MtTableID] = strings.TrimRight(d.MtTable.MtName, "\x00")
	}
}

// mplsLabelsOf dumps the labels ≥ 16 of an MPLS table as "<label>/<eos|neos>" → the route.
func mplsLabelsOf(t *testing.T, c vpp.Client, table uint32) map[string]mplsapi.MplsRoute {
	t.Helper()
	st, err := mplsapi.NewServiceClient(c).MplsRouteDump(context.Background(), &mplsapi.MplsRouteDump{Table: mplsapi.MplsTable{MtTableID: table}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]mplsapi.MplsRoute{}
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if d.MrRoute.MrLabel >= 16 {
			e := "neos"
			if d.MrRoute.MrEos != 0 {
				e = "eos"
			}
			out[fmt.Sprintf("%d/%s", d.MrRoute.MrLabel, e)] = d.MrRoute
		}
	}
}

// ownedMplsTunnels dumps this owner's MPLS tunnels (tag "<owner>:<name>").
func ownedMplsTunnels(t *testing.T, c vpp.Client, owner string) []mplsapi.MplsTunnel {
	t.Helper()
	st, err := mplsapi.NewServiceClient(c).MplsTunnelDump(context.Background(), &mplsapi.MplsTunnelDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		t.Fatal(err)
	}
	var out []mplsapi.MplsTunnel
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := vpp.ParseOwnerTag(d.MtTunnel.MtTag, owner); ok {
			out = append(out, d.MtTunnel)
		}
	}
}

// deleteOwnedMpls removes, via binapi, this owner's MPLS objects in V15 order: label routes (in our tables and the
// given table-0 labels), tunnels, then tables. Returns the number of objects removed.
func deleteOwnedMpls(t *testing.T, c vpp.Client, owner string, zeroLabels []uint32) int {
	t.Helper()
	ctx := context.Background()
	svc := mplsapi.NewServiceClient(c)
	n := 0
	tables := mplsTables(t, c)
	for id, name := range tables {
		if tag, ok := vpp.ParseOwnerTag(name, owner); !ok || tag != fmt.Sprint(id) {
			continue
		}
		for _, r := range mplsLabelsOf(t, c, id) {
			if _, err := svc.MplsRouteAddDel(ctx, &mplsapi.MplsRouteAddDel{MrIsAdd: false, MrRoute: mplsapi.MplsRoute{MrTableID: id, MrLabel: r.MrLabel, MrEos: r.MrEos, MrEosProto: r.MrEosProto}}); err != nil {
				t.Errorf("delete label %d in table %d: %v", r.MrLabel, id, err)
			}
			n++
		}
	}
	if _, ok := tables[0]; ok {
		labels := mplsLabelsOf(t, c, 0)
		for _, l := range zeroLabels {
			for _, e := range []string{"eos", "neos"} {
				if r, ok := labels[fmt.Sprintf("%d/%s", l, e)]; ok {
					_, _ = svc.MplsRouteAddDel(ctx, &mplsapi.MplsRouteAddDel{MrIsAdd: false, MrRoute: mplsapi.MplsRoute{MrTableID: 0, MrLabel: r.MrLabel, MrEos: r.MrEos, MrEosProto: r.MrEosProto}})
					n++
				}
			}
		}
	}
	for _, tn := range ownedMplsTunnels(t, c, owner) {
		if _, err := svc.MplsTunnelAddDel(ctx, &mplsapi.MplsTunnelAddDel{MtIsAdd: false, MtTunnel: mplsapi.MplsTunnel{MtSwIfIndex: tn.MtSwIfIndex, MtNPaths: tn.MtNPaths, MtPaths: tn.MtPaths}}); err != nil {
			t.Errorf("delete tunnel %d: %v", tn.MtSwIfIndex, err)
		}
		n++
	}
	for id, name := range tables {
		if tag, ok := vpp.ParseOwnerTag(name, owner); ok && tag == fmt.Sprint(id) && id != 0 {
			if _, err := svc.MplsTableAddDel(ctx, &mplsapi.MplsTableAddDel{MtIsAdd: false, MtTable: mplsapi.MplsTable{MtTableID: id}}); err != nil {
				t.Errorf("delete MPLS table %d: %v", id, err)
			}
			n++
		}
	}
	return n
}

// globalsWindow takes `flock -x /run/lock/vrx-globals.lock` (D-082) for the rest of the test.
func globalsWindow(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile("/run/lock/vrx-globals.lock", os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // lock file
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
}

func TestMplsOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	base := vpptest.TableBase(t)
	globals := os.Getenv("VRX_DF7_GLOBALS") == "1"
	t.Logf("systemctl show vpp -p NRestarts (before): %s", vppRestarts(t))
	before := vppRestarts(t)
	t.Cleanup(func() {
		after := vppRestarts(t)
		t.Logf("systemctl show vpp -p NRestarts (after): %s", after)
		if after != before {
			t.Errorf("VPP restarted during the test: %s → %s", before, after)
		}
	})

	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	labels := func(i int) uint32 { return uint32(slot*10000 + i) } //nolint:gosec // slots are 1–11
	mplsTable, vrfTable := base+1, base+10
	loop := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 31))
	zeroLabels := []uint32{labels(20), labels(40), labels(100)}
	t.Cleanup(func() {
		n := deleteOwnedMpls(t, raw, owner, zeroLabels)
		m := deleteOwned(t, raw, owner)
		t.Logf("cleanup: %d MPLS objects, %d loopbacks/VRFs removed via binapi", n, m)
		t.Logf("cleanup: vppctl show mpls fib table %d:\n%s", mplsTable, mplsHostLog(t, "show", "mpls", "fib", "table", fmt.Sprint(mplsTable)))
	})

	createdZero := false
	if globals {
		globalsWindow(t)
		if _, ok := mplsTables(t, raw)[0]; !ok {
			if _, err := mplsapi.NewServiceClient(raw).MplsTableAddDel(context.Background(), &mplsapi.MplsTableAddDel{MtIsAdd: true, MtTable: mplsapi.MplsTable{MtTableID: 0, MtName: owner + ":globals-window"}}); err != nil {
				t.Fatal(err)
			}
			createdZero = true
			t.Log("VRX_DF7_GLOBALS=1: created MPLS table 0 for the window (deleted at the end)")
		}
		t.Cleanup(func() {
			if !createdZero {
				return
			}
			if _, err := mplsapi.NewServiceClient(raw).MplsTableAddDel(context.Background(), &mplsapi.MplsTableAddDel{MtIsAdd: false, MtTable: mplsapi.MplsTable{MtTableID: 0}}); err != nil {
				t.Errorf("delete the window's MPLS table 0: %v", err)
			}
		})
	}

	extra, extraCanon := "", ""
	if globals {
		extra = fmt.Sprintf(`,
	    "interfaces": [%[1]q],
	    "ipBindings": [{"label": %[2]d, "vrf": "red", "prefix": "10.%[3]d.40.0/24"}],
	    "sr": {"policies": {"%[4]d": {"segmentLists": [{"labels": [%[5]d, %[6]d], "weight": 1}], "spray": false}},
	           "steering": [{"vrf": "default", "prefix": "10.%[3]d.60.0/24", "bsid": %[4]d}]}`,
			loop, labels(40), slot, labels(100), labels(101), labels(102))
		extraCanon = fmt.Sprintf(`, "interfaces": [%q]`, loop)
	}
	zeroRoute, zeroCanon := "", ""
	if globals {
		zeroRoute = fmt.Sprintf(`{"table": 0, "label": %d, "eos": false, "paths": [{"interface": "t1", "outLabels": [%d], "weight": 1}]},`, labels(20), labels(21))
		zeroCanon = zeroRoute
	}
	js := fmt.Sprintf(`{
	  "vrfs": {"red": {"id": %[1]d}},
	  "interfaces": {%[2]q: {"ipv4": ["10.%[3]d.31.1/24"]}},
	  "routing": {"mpls": {
	    "tables": {"%[4]d": {}},
	    "labelRoutes": [
	      %[9]s
	      {"table": %[4]d, "label": %[5]d, "eos": true, "paths": [{"nextHop": "10.%[3]d.31.2", "interface": %[2]q, "outLabels": [%[6]d, %[7]d], "weight": 1}]},
	      {"table": %[4]d, "label": %[8]d, "eos": true, "paths": [{"vrf": "red", "weight": 1}]}
	    ],
	    "tunnels": {"t1": {"paths": [{"nextHop": "10.%[3]d.31.2", "interface": %[2]q, "outLabels": [%[10]d], "weight": 1}], "l2Only": false}}%[11]s
	  }}
	}`, vrfTable, loop, slot, mplsTable, labels(16), labels(17), labels(18), labels(30), zeroRoute, labels(50), extra)
	canon := fmt.Sprintf(`{
	  "tables": {"%[1]d": {}},
	  "labelRoutes": [
	    %[7]s
	    {"table": %[1]d, "label": %[2]d, "eos": true, "paths": [{"nextHop": "10.%[3]d.31.2", "interface": %[4]q, "outLabels": [%[5]d, %[6]d], "weight": 1}]},
	    {"table": %[1]d, "label": %[8]d, "eos": true, "paths": [{"vrf": "red", "weight": 1}]}
	  ],
	  "tunnels": {"t1": {"paths": [{"nextHop": "10.%[3]d.31.2", "interface": %[4]q, "outLabels": [%[9]d], "weight": 1}], "l2Only": false}}%[10]s
	}`, mplsTable, labels(16), slot, loop, labels(17), labels(18), zeroCanon, labels(30), labels(50), extraCanon)
	desired := doc(t, js)
	want := &vrxv1.MplsConfig{}
	if err := protojson.Unmarshal([]byte(canon), want); err != nil {
		t.Fatal(err)
	}

	cfg := hostConfig(t, owner)
	cfg.IDs = subsystems.IDScope{Range: &subsystems.IDRange{Lo: base, Hi: base + 999}}
	a, err := Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Stop() })
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)
	ctx := context.Background()
	retrieved := func() *vrxv1.MplsConfig {
		got, err := c.Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
		if err != nil {
			t.Fatal(err)
		}
		return got.GetDesiredState().GetRouting().GetMpls()
	}
	converged := func(since time.Time) time.Duration {
		for time.Since(since) < 30*time.Second {
			if proto.Equal(retrieved(), want) {
				return time.Since(since)
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("not converged within 30 s:\n got %s\nwant %s", protojson.Format(retrieved()), protojson.Format(want))
		return 0
	}

	// 1. apply → Retrieve == canonical; VPP shows it
	resp, err := c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-mpls-1", DesiredState: desired})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %s", err, protojson.Format(resp))
	}
	t.Logf("apply: %s", protojson.Format(resp.GetSummary()))
	converged(time.Now())
	t.Logf("vppctl show mpls fib table %d:\n%s", mplsTable, mplsHostLog(t, "show", "mpls", "fib", "table", fmt.Sprint(mplsTable)))
	t.Logf("vppctl show mpls tunnel:\n%s", mplsHostLog(t, "show", "mpls", "tunnel"))
	if got := mplsLabelsOf(t, raw, mplsTable); len(got) != 2 {
		t.Fatalf("labels in table %d: %v", mplsTable, got)
	}
	if globals {
		t.Logf("vppctl show sr mpls policies:\n%s", mplsHostLog(t, "show", "sr", "mpls", "policies"))
		t.Logf("vppctl show mpls interface:\n%s", mplsHostLog(t, "show", "mpls", "interface"))
		t.Logf("vppctl show mpls fib table 0 (our labels %v):\n%s", zeroLabels, mplsHostLog(t, "show", "mpls", "fib", "table", "0"))
	}
	resp, err = c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-mpls-2", DesiredState: desired})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("idempotent apply: %v %v", err, resp)
	}
	for _, r := range resp.GetResults() {
		if r.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_CREATE || (!strings.HasPrefix(r.GetKey(), "mpls-ip-bind") && !strings.HasPrefix(r.GetKey(), "sr-mpls")) {
			t.Errorf("idempotent apply changed %s (%s)", r.GetKey(), r.GetOp())
		}
	}
	t.Logf("idempotent apply: %s (write-only objects are re-applied, D-063)", protojson.Format(resp.GetSummary()))

	// 2. MplsState: one page of our table, our tunnel
	st, err := c.MplsState(ctx, &vrxv1.MplsStateRequest{View: "fib", TableId: mplsTable, Limit: 10})
	if err != nil || st.GetTotal() < 2 {
		t.Fatalf("MplsState fib: %v %v", err, st)
	}
	t.Logf("MplsState fib table %d: total %d, tables %v", mplsTable, st.GetTotal(), st.GetTables())
	tn, err := c.MplsState(ctx, &vrxv1.MplsStateRequest{View: "tunnels"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MplsState tunnels: %s", protojson.Format(tn))

	// 3. restart simulation: stop, lose label routes / tunnel (/ SR policy) behind the agent's back, start → back
	a.Stop()
	svc := mplsapi.NewServiceClient(raw)
	for _, r := range mplsLabelsOf(t, raw, mplsTable) {
		if _, err := svc.MplsRouteAddDel(ctx, &mplsapi.MplsRouteAddDel{MrIsAdd: false, MrRoute: mplsapi.MplsRoute{MrTableID: mplsTable, MrLabel: r.MrLabel, MrEos: r.MrEos, MrEosProto: r.MrEosProto}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tu := range ownedMplsTunnels(t, raw, owner) {
		if _, err := svc.MplsTunnelAddDel(ctx, &mplsapi.MplsTunnelAddDel{MtIsAdd: false, MtTunnel: mplsapi.MplsTunnel{MtSwIfIndex: tu.MtSwIfIndex, MtNPaths: tu.MtNPaths, MtPaths: tu.MtPaths}}); err != nil {
			t.Fatal(err)
		}
	}
	if globals {
		if _, err := srmplsapi.NewServiceClient(raw).SrMplsPolicyDel(ctx, &srmplsapi.SrMplsPolicyDel{Bsid: labels(100)}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("simulated loss: vppctl show mpls fib table %d:\n%s", mplsTable, mplsHostLog(t, "show", "mpls", "fib", "table", fmt.Sprint(mplsTable)))
	start := time.Now()
	a, err = Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	c = dialAgent(t, cfg.Socket)
	took := converged(start)
	t.Logf("restart after loss: Retrieve == canonical in %v", took)
	t.Logf("vppctl show mpls fib table %d (after restart):\n%s", mplsTable, mplsHostLog(t, "show", "mpls", "fib", "table", fmt.Sprint(mplsTable)))
	if globals {
		t.Logf("vppctl show sr mpls policies (after restart):\n%s", mplsHostLog(t, "show", "sr", "mpls", "policies"))
	}

	// 4. rollback: an empty routing domain removes the label routes before their table, the tunnel, (the SR objects)
	resp, err = c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-mpls-3", DesiredState: doc(t, `{"routing": {}}`), Subsystems: []string{"routing"}})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("rollback: %v %s", err, protojson.Format(resp))
	}
	t.Logf("rollback: %s", protojson.Format(resp.GetSummary()))
	if m := retrieved(); m != nil {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(m))
	}
	if name, ok := mplsTables(t, raw)[mplsTable]; ok {
		t.Fatalf("MPLS table %d still exists (%q)", mplsTable, name)
	}
	if n := len(ownedMplsTunnels(t, raw, owner)); n != 0 {
		t.Fatalf("%d tunnels left", n)
	}
	t.Logf("after rollback: vppctl show mpls fib table %d:\n%s", mplsTable, mplsHostLog(t, "show", "mpls", "fib", "table", fmt.Sprint(mplsTable)))
	if globals {
		zero := mplsLabelsOf(t, raw, 0)
		for _, l := range zeroLabels {
			if _, ok := zero[fmt.Sprintf("%d/eos", l)]; ok {
				t.Errorf("label %d left in MPLS table 0", l)
			}
		}
		t.Logf("after rollback: vppctl show sr mpls policies:\n%s", mplsHostLog(t, "show", "sr", "mpls", "policies"))
	}
}
