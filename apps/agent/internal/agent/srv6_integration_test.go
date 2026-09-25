package agent

// F-srv6 host integration check (VRX_INTEGRATION=1, shared lab lock, slot prefix; docs/status/tasks/F-srv6.md):
//
//	TestSrv6OnHost          in-process agent (owner <prefix>sr, VRX_GLOBALS_OWNER=0): apply routing.srv6 →
//	                        Retrieve == desired, `vppctl show sr …` lists it → Srv6State reports only our
//	                        objects → validation failure with pointer → stop, delete our SR objects via binapi
//	                        (steering → policies → SIDs, each checked first, D-074), start → recreated ≤ 30 s
//	                        → converged restart re-adds nothing → rollback removes steering → policies → SIDs
//	                        → VRF with no SR route left (`show ip6 fib table`, `show ip fib`).
//	TestSrv6GlobalsOnHost   opt-in VRX_FSRV6_GLOBALS=1 (manager window, D-082): the globals owner sets the
//	                        encap source / hop limit under flock -x /run/lock/vrx-globals.lock and restores the
//	                        values it recorded before (write-only: read with `vppctl show sr encaps …`).
//
// Addresses: SIDs/BSIDs/segments in fd00:<slot hex>::/48, the steered prefixes 10.<slot>.160.0/24 and
// fd00:<slot hex>:160::/48, tables base+60, loopbacks loop<slot>60/61. No packet is sent (no V19 pre-flight
// needed); no table is flushed; VPP is never restarted.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/binapi/sr_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// srv6Addrs are the slot's SRv6 test addresses.
type srv6Addrs struct {
	h         string // "fd00:<slot hex>"
	slot      int
	table     uint32
	vrf       string
	l1, l2    string
	sids      []string
	bsids     []string
	v4, v6pfx string
}

func srv6HostAddrs(t *testing.T, owner string) srv6Addrs {
	t.Helper()
	slot := vpptest.Slot(t)
	h := fmt.Sprintf("fd00:%x", slot)
	return srv6Addrs{
		h: h, slot: slot, table: vpptest.TableBase(t) + 60, vrf: owner + "-cust",
		l1: fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 60)), l2: fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 61)),
		sids:  []string{h + ":ff::1", h + ":ff::2", h + ":ff::a", h + ":ff::b", h + ":ff::c"},
		bsids: []string{h + ":bb::1", h + ":bb::2"},
		v4:    fmt.Sprintf("10.%d.160.0/24", slot), v6pfx: h + ":160::/48",
	}
}

// srv6HostDoc is the slot's SRv6 document (canonical form, steering in Retrieve order).
func srv6HostDoc(t *testing.T, a srv6Addrs) *vrxv1.DesiredState {
	t.Helper()
	return doc(t, fmt.Sprintf(`{
	  "vrfs": {%[1]q: {"id": %[2]d}},
	  "interfaces": {%[3]q: {"ipv6": ["%[5]s:1::1/64"]}, %[4]q: {}},
	  "routing": {"srv6": {
	    "localSids": {
	      "%[5]s:ff::1": {"behavior": "end", "psp": true, "vrf": "default"},
	      "%[5]s:ff::2": {"behavior": "end.x", "psp": false, "vrf": "default", "interface": %[3]q, "nextHop": "%[5]s:1::2"},
	      "%[5]s:ff::a": {"behavior": "end.dt4", "psp": false, "vrf": "default", "lookupVrf": %[1]q},
	      "%[5]s:ff::b": {"behavior": "end.dt6", "psp": false, "vrf": %[1]q, "lookupVrf": %[1]q},
	      "%[5]s:ff::c": {"behavior": "end.dx2", "psp": false, "vrf": "default", "interface": %[4]q}
	    },
	    "policies": {
	      "%[5]s:bb::1": {"type": "default", "encap": true, "vrf": "default", "encapSource": "%[5]s::1",
	        "sidLists": [{"sids": ["%[5]s:ee::1", "%[5]s:ee::a"], "weight": 1}, {"sids": ["%[5]s:ee::2", "%[5]s:ee::a"], "weight": 3}]},
	      "%[5]s:bb::2": {"type": "spray", "encap": false, "vrf": %[1]q, "sidLists": [{"sids": ["%[5]s:ee::3"], "weight": 1}]}
	    },
	    "steering": [
	      {"type": "l3", "prefix": %[6]q, "vrf": %[1]q, "bsid": "%[5]s:bb::1"},
	      {"type": "l3", "prefix": %[7]q, "vrf": %[1]q, "bsid": "%[5]s:bb::2"},
	      {"type": "l2", "interface": %[4]q, "bsid": "%[5]s:bb::1"}
	    ]
	  }}
	}`, a.vrf, a.table, a.l1, a.l2, a.h, a.v4, a.v6pfx))
}

// srv6Show runs a read-only `vppctl show …` with fixed words (ALLOW: test-only evidence, show only).
func srv6Show(t *testing.T, args ...string) string {
	t.Helper()
	if len(args) == 0 || args[0] != "show" {
		t.Fatalf("vppctl %v: only show commands", args)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/vppctl", args...).CombinedOutput() //nolint:gosec // G204 ALLOW: fixed argv, show only
	if err != nil {
		t.Fatalf("vppctl %v: %v: %s", args, err, out)
	}
	return string(out)
}

// grepLines keeps the lines of out that contain any of the words (and the first line).
func grepLines(out string, words ...string) string {
	var keep []string
	for i, l := range strings.Split(out, "\n") {
		for _, w := range words {
			if i == 0 || strings.Contains(l, w) {
				keep = append(keep, l)
				break
			}
		}
	}
	return strings.Join(keep, "\n")
}

// srv6OnHost lists which of the slot's SIDs, BSIDs and steering entries VPP has (binapi dumps).
func srv6OnHost(t *testing.T, c vpp.Client, a srv6Addrs) (sids, bsids []string, steer []*srapi.SrSteeringPolDetails) {
	t.Helper()
	ctx := context.Background()
	collect := func(recv func() error) {
		for {
			if err := recv(); errors.Is(err, io.EOF) {
				return
			} else if err != nil {
				t.Fatal(err)
			}
		}
	}
	ls, err := srapi.NewServiceClient(c).SrLocalsidsDump(ctx, &srapi.SrLocalsidsDump{})
	if err != nil {
		t.Fatal(err)
	}
	collect(func() error {
		d, err := ls.Recv()
		if err == nil && strings.HasPrefix(df6.IP6String(d.Addr), a.h+":") {
			sids = append(sids, df6.IP6String(d.Addr))
		}
		return err
	})
	ps, err := srapi.NewServiceClient(c).SrPoliciesV2Dump(ctx, &srapi.SrPoliciesV2Dump{})
	if err != nil {
		t.Fatal(err)
	}
	collect(func() error {
		d, err := ps.Recv()
		if err == nil && strings.HasPrefix(df6.IP6String(d.Bsid), a.h+":") {
			bsids = append(bsids, df6.IP6String(d.Bsid))
		}
		return err
	})
	ss, err := srapi.NewServiceClient(c).SrSteeringPolDump(ctx, &srapi.SrSteeringPolDump{})
	if err != nil {
		t.Fatal(err)
	}
	collect(func() error {
		d, err := ss.Recv()
		if err == nil && strings.HasPrefix(df6.IP6String(d.Bsid), a.h+":") {
			steer = append(steer, d)
		}
		return err
	})
	return sids, bsids, steer
}

// deleteSrv6OnHost deletes the slot's SR objects via binapi in dependency order (steering → policies →
// SIDs), each only when the dump still lists it (D-074), and returns how many it deleted.
func deleteSrv6OnHost(t *testing.T, c vpp.Client, a srv6Addrs) int {
	t.Helper()
	ctx := context.Background()
	svc := srapi.NewServiceClient(c)
	sids, bsids, steer := srv6OnHost(t, c, a)
	n := 0
	for _, s := range steer {
		req := &srapi.SrSteeringAddDel{IsDel: true, BsidAddr: s.Bsid, SrPolicyIndex: ^uint32(0), TableID: s.FibTable, Prefix: s.Prefix,
			SwIfIndex: s.SwIfIndex, TrafficType: s.TrafficType}
		if s.TrafficType == sr_types.SR_STEER_API_L2 {
			req.TableID, req.Prefix = 0, ip_types.Prefix{}
		} else {
			req.SwIfIndex = interface_types.InterfaceIndex(^uint32(0))
		}
		if _, err := svc.SrSteeringAddDel(ctx, req); err != nil {
			t.Errorf("delete steering %v: %v", s, err)
		}
		n++
	}
	for _, b := range bsids {
		bsid, _ := df6.IP6Of(b)
		if _, err := svc.SrPolicyDel(ctx, &srapi.SrPolicyDel{BsidAddr: bsid, SrPolicyIndex: ^uint32(0)}); err != nil {
			t.Errorf("delete policy %s: %v", b, err)
		}
		n++
	}
	ls, err := svc.SrLocalsidsDump(ctx, &srapi.SrLocalsidsDump{})
	if err != nil {
		t.Fatal(err)
	}
	var dels []*srapi.SrLocalsidAddDel
	for {
		d, err := ls.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range sids {
			if df6.IP6String(d.Addr) == s {
				// the delete key is (sid, fib table); the table exists (VPP uses fib_table_find unchecked)
				dels = append(dels, &srapi.SrLocalsidAddDel{IsDel: true, Localsid: d.Addr, FibTable: d.FibTable, SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
			}
		}
	}
	for _, d := range dels {
		if err := df6.RequireTable(ctx, c, d.FibTable, true); err != nil {
			t.Errorf("sid %s: %v (not deleted)", df6.IP6String(d.Localsid), err)
			continue
		}
		if _, err := svc.SrLocalsidAddDel(ctx, d); err != nil {
			t.Errorf("delete sid %s: %v", df6.IP6String(d.Localsid), err)
		}
		n++
	}
	return n
}

// srv6Leftovers reports any FIB entry of the slot's SR addresses and steered prefixes in any table.
func srv6Leftovers(t *testing.T, a srv6Addrs) []string {
	t.Helper()
	var out []string
	pfx := regexp.MustCompile(`^(` + regexp.QuoteMeta(a.h) + `:(ff|bb|160)::[0-9a-f:/]*|` + regexp.QuoteMeta(strings.TrimSuffix(a.v4, ".0/24")) + `\.[0-9./]*)`)
	for _, fam := range []string{"ip6", "ip"} {
		for _, l := range strings.Split(srv6Show(t, "show", fam, "fib"), "\n") {
			if pfx.MatchString(strings.TrimSpace(l)) {
				out = append(out, fam+": "+strings.TrimSpace(l))
			}
		}
	}
	return out
}

func TestSrv6OnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t) + "sr"
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	a := srv6HostAddrs(t, owner)
	t.Cleanup(func() { deleteSrv6OnHost(t, raw, a); deleteOwned(t, raw, owner) })
	if s, b, st := srv6OnHost(t, raw, a); len(s)+len(b)+len(st) != 0 {
		t.Fatalf("slot SR range not clean before the run: %v %v %v", s, b, st)
	}

	cfg := hostConfig(t, owner)
	a1, err := Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)
	ctx := context.Background()
	desired := srv6HostDoc(t, a)
	want := desired.GetRouting().GetSrv6()
	retrieve := func() *vrxv1.Srv6Config {
		t.Helper()
		got, err := c.Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"routing"}})
		if err != nil {
			t.Fatal(err)
		}
		return got.GetDesiredState().GetRouting().GetSrv6()
	}
	waitSrv6 := func(since time.Time) time.Duration {
		t.Helper()
		var last *vrxv1.Srv6Config
		for time.Since(since) < 30*time.Second {
			if last = retrieve(); proto.Equal(last, want) {
				return time.Since(since)
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("routing.srv6 not converged within 30 s:\n got %s\nwant %s", protojson.Format(last), protojson.Format(want))
		return 0
	}

	// 1. apply → Retrieve == desired; VPP shows it.
	resp, err := c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-1", DesiredState: desired})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %s", err, protojson.Format(resp))
	}
	t.Logf("apply: %s", protojson.Format(resp.GetSummary()))
	waitSrv6(time.Now())
	t.Logf("Retrieve routing.srv6 == desired:\n%s", protojson.Format(retrieve()))
	t.Logf("vppctl show sr localsids:\n%s", grepLines(srv6Show(t, "show", "sr", "localsids"), a.h+":"))
	t.Logf("vppctl show sr policies:\n%s", srv6Show(t, "show", "sr", "policies"))
	t.Logf("vppctl show sr steering-policies:\n%s", grepLines(srv6Show(t, "show", "sr", "steering-policies"), a.h+":", a.l2))
	t.Logf("vppctl show ip6 fib table %d:\n%s", a.table, srv6Show(t, "show", "ip6", "fib", "table", fmt.Sprint(a.table)))
	t.Logf("vppctl show ip fib table %d:\n%s", a.table, srv6Show(t, "show", "ip", "fib", "table", fmt.Sprint(a.table)))

	// 2. Srv6State: exactly our objects.
	st, err := c.Srv6State(ctx, &vrxv1.Srv6StateRequest{})
	if err != nil || len(st.GetLocalSids()) != 5 || len(st.GetPolicies()) != 2 || len(st.GetSteering()) != 3 {
		t.Fatalf("Srv6State: %v %s", err, protojson.Format(st))
	}
	t.Logf("Srv6State: %s", protojson.Format(st))

	// 3. validation failure with a pointer (the agent's own gate).
	bad := proto.Clone(desired).(*vrxv1.DesiredState)
	bad.GetRouting().GetSrv6().GetPolicies()[a.bsids[0]].EncapSource = nil
	resp, err = c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-bad", DesiredState: bad})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_FAILED || resp.GetValidation().GetErrors()[0].GetPointer() != "/routing/srv6/policies/"+a.bsids[0]+"/encapSource" {
		t.Fatalf("encap without source: %v %s", err, protojson.Format(resp))
	}

	// 4. restart simulation: stop, delete our SR objects via binapi, start → recreated.
	a1.Stop()
	if n := deleteSrv6OnHost(t, raw, a); n != 10 {
		t.Fatalf("simulated loss deleted %d SR objects, want 10", n)
	}
	start := time.Now()
	a1, err = Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	c = dialAgent(t, cfg.Socket)
	t.Logf("restart after loss: routing.srv6 converged in %v", waitSrv6(start))

	// 5. converged restart: nothing re-added (claims persisted).
	a1.Stop()
	a1, err = Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	c = dialAgent(t, cfg.Socket)
	h := waitReady(t, c)
	resp, err = c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-2", DesiredState: desired})
	if err != nil || len(resp.GetResults()) != 0 {
		t.Fatalf("converged restart then re-apply changed something: %v %s", err, protojson.Format(resp))
	}
	t.Logf("converged restart: last reconcile %s, re-apply unchanged=%d", h.GetLastReconcileAt().AsTime().Format(time.RFC3339), resp.GetSummary().GetUnchanged())

	// 6. rollback: routing and vrfs authoritative and empty → steering → policies → SIDs → VRF, no SR route left.
	resp, err = c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-3", DesiredState: doc(t, fmt.Sprintf(`{"interfaces": {%q: {}, %q: {}}}`, a.l1, a.l2)), Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("rollback: %v %s", err, protojson.Format(resp))
	}
	var order []string
	for _, r := range resp.GetResults() {
		order = append(order, r.GetKey())
	}
	t.Logf("rollback order: %s", strings.Join(order, " → "))
	if got := retrieve(); got != nil {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if s, b, st := srv6OnHost(t, raw, a); len(s)+len(b)+len(st) != 0 {
		t.Fatalf("left in VPP: %v %v %v", s, b, st)
	}
	if l := srv6Leftovers(t, a); len(l) > 0 {
		t.Fatalf("leaked FIB entries: %v", l)
	}
	t.Logf("after rollback: vppctl show sr localsids (slot range):\n%s", grepLines(srv6Show(t, "show", "sr", "localsids"), a.h+":"))
	t.Logf("after rollback: vppctl show ip6 fib table %d: %s", a.table, strings.TrimSpace(srv6ShowMaybe(t, "show", "ip6", "fib", "table", fmt.Sprint(a.table))))
	a1.Stop()
}

// srv6ShowMaybe is srv6Show that returns the error text instead of failing (a deleted table).
func srv6ShowMaybe(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "/usr/bin/vppctl", args...).CombinedOutput() //nolint:gosec // G204 ALLOW: fixed argv, show only
	return string(out)
}

// TestSrv6GlobalsOnHost changes the two write-only VPP globals as the globals owner — only in a manager
// window (VRX_FSRV6_GLOBALS=1, D-082): exclusive globals lock, previous values recorded first and restored.
func TestSrv6GlobalsOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv("VRX_FSRV6_GLOBALS") != "1" {
		t.Skip("changes the getter-less SRv6 globals; set VRX_FSRV6_GLOBALS=1 in a manager window (D-082)")
	}
	vpptest.LockLab(t)
	f, err := os.OpenFile("/run/lock/vrx-globals.lock", os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // lock file
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() })
	readGlobals := func() (string, string) {
		src := regexp.MustCompile(`= (\S+)`).FindStringSubmatch(srv6Show(t, "show", "sr", "encaps", "source", "addr"))
		hl := regexp.MustCompile(`= (\d+)`).FindStringSubmatch(srv6Show(t, "show", "sr", "encaps", "hop-limit"))
		if src == nil || hl == nil {
			t.Fatal("cannot read the SRv6 globals")
		}
		return src[1], hl[1]
	}
	prevSrc, prevHL := readGlobals()
	t.Logf("recorded before: encap source %s, hop limit %s", prevSrc, prevHL)
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { // restore the recorded values, never VPP's defaults (D-071)
		a, _ := df6.IP6Of(prevSrc)
		var hl uint8
		_, _ = fmt.Sscan(prevHL, &hl)
		_, e1 := srapi.NewServiceClient(raw).SrSetEncapSource(context.Background(), &srapi.SrSetEncapSource{EncapsSource: a})
		_, e2 := srapi.NewServiceClient(raw).SrSetEncapHopLimit(context.Background(), &srapi.SrSetEncapHopLimit{HopLimit: hl})
		s, h := readGlobals()
		t.Logf("restored: encap source %s, hop limit %s (%v %v)", s, h, e1, e2)
	})
	owner := vpptest.Prefix(t) + "srg"
	cfg := hostConfig(t, owner)
	cfg.GlobalsOwner = true
	ag, err := Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ag.Stop()
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)
	src := fmt.Sprintf("fd00:%x::99", vpptest.Slot(t))
	resp, err := c.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: owner + "-g", DesiredState: doc(t, fmt.Sprintf(`{"routing": {"srv6": {"encapSource": %q, "encapHopLimit": 33}}}`, src))})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %s", err, protojson.Format(resp))
	}
	if s, h := readGlobals(); s != src || h != "33" {
		t.Fatalf("globals %s %s", s, h)
	}
	t.Logf("globals owner set: %s", strings.Join(strings.Fields(srv6Show(t, "show", "sr", "encaps", "source", "addr")+" "+srv6Show(t, "show", "sr", "encaps", "hop-limit")), " "))
}
