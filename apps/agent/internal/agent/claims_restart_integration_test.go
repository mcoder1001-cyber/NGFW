package agent

// TD-11b host proof (VRX_INTEGRATION=1, shared lab lock, slot prefix): attributes of an UNTAGGED
// interface — a tap made directly through the binary API, standing in for a DPDK NIC (D-069, as in
// the DF-1 alias host test) — are ours only through the persisted claim store. A new agent process
// on the same state dir must still Retrieve them (its first resync creates nothing), and removing
// them through the agent releases the claims. VPP is never restarted; the tap is deleted in Cleanup.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	tapapi "ngfw/agent/binapi/tapv2"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

func TestUntaggedClaimsSurviveAgentRestartOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// the untagged "NIC": tap<slot>73, Linux side <prefix>-tap73
	inst := vpptest.LoopbackInstance(t, 73)
	svc := tapapi.NewServiceClient(raw)
	rep, err := svc.TapCreateV3(ctx, &tapapi.TapCreateV3{ID: inst, UseRandomMac: true, NumRxQueues: 1, NumTxQueues: 1,
		RxRingSz: 256, TxRingSz: 256, HostIfNameSet: true, HostIfName: vpptest.Name(t, "tap73")})
	if err != nil {
		t.Fatalf("tap_create_v3: %v", err)
	}
	tapIdx := uint32(rep.SwIfIndex)
	nic := fmt.Sprintf("tap%d", inst)
	t.Cleanup(func() {
		_ = ifsanitize.BeforeDelete(context.Background(), raw, tapIdx, "tap") // D-095 c
		if _, err := svc.TapDeleteV2(context.Background(), &tapapi.TapDeleteV2{SwIfIndex: interface_types.InterfaceIndex(tapIdx)}); err != nil {
			t.Errorf("cleanup tap_delete_v2: %v", err)
		}
	})
	t.Logf("untagged %s sw_if_index %d (no tag: ours only through a claim)", nic, tapIdx)

	bin := filepath.Join(t.TempDir(), "vrx-agent")
	build := exec.Command("go", "build", "-o", bin, "ngfw/agent/cmd/vrx-agent") //nolint:gosec // fixed argv, test only
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cfg := hostConfig(t, owner)
	logs := []string{}
	start := func(n int) *exec.Cmd {
		logPath := filepath.Join(filepath.Dir(cfg.StateDir), fmt.Sprintf("agent-%d.log", n))
		logs = append(logs, logPath)
		cmd := exec.Command(bin) //nolint:gosec // our own binary
		cmd.Env = append(os.Environ(),
			"VRX_AGENT_SOCKET="+cfg.Socket, "VRX_OWNER="+owner, "VRX_AGENT_STATE_DIR="+cfg.StateDir,
			"VRX_AGENT_VPP_API_SOCKET="+cfg.VPPAPISocket, "VRX_METRICS_ADDR=off", "VRX_SOCKET_GROUP=")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test temp dir
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout, cmd.Stderr = f, f
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
				_ = cmd.Wait()
			}
		})
		t.Logf("agent process %d: pid %d", n, cmd.Process.Pid)
		return cmd
	}
	stop := func(cmd *exec.Cmd) {
		t.Helper()
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil { // the PID we spawned, never by name
			t.Fatal(err)
		}
		_ = cmd.Wait()
	}
	retrieve := func(c vrxv1.DataplaneClient) *vrxv1.DesiredState {
		t.Helper()
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		r, err := c.Retrieve(rctx, &vrxv1.RetrieveRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return r.GetDesiredState()
	}
	claimsFile := filepath.Join(cfg.StateDir, "claims-iface-"+owner+".json")
	claims := func() []map[string]any {
		t.Helper()
		b, err := os.ReadFile(claimsFile) //nolint:gosec // test temp dir
		if err != nil {
			t.Fatal(err)
		}
		var recs []map[string]any
		if err := json.Unmarshal(b, &recs); err != nil {
			t.Fatal(err)
		}
		return recs
	}

	// 1. process 1 applies admin-state + MTU on the untagged NIC: both are claimed (bound to its index)
	p1 := start(1)
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)
	desired := doc(t, fmt.Sprintf(`{"system": {"hostname": "vrx-a"}, "interfaces": {%q: {"enabled": true, "mtu": 1400}}}`, nic))
	actx, acancel := context.WithTimeout(ctx, 60*time.Second)
	defer acancel()
	resp, err := c.Apply(actx, &vrxv1.ApplyRequest{TxnId: owner + "-claims-1", DesiredState: desired, Subsystems: []string{"interfaces"}})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %v", err, resp)
	}
	before := retrieve(c)
	itf := before.GetInterfaces()[nic]
	if !itf.GetEnabled() || itf.GetMtu() != 1400 {
		t.Fatalf("process 1 Retrieve of %s: %v", nic, protojson.Format(itf))
	}
	recs := claims()
	if len(recs) != 2 {
		t.Fatalf("claims after apply: %v", recs)
	}
	for _, r := range recs {
		if !strings.HasPrefix(r["key"].(string), nic+"|") || int64(r["sw_if_index"].(float64)) != int64(tapIdx) {
			t.Fatalf("claim %v not bound to %s/%d", r, nic, tapIdx)
		}
	}
	compact, _ := protojson.Marshal(itf)
	t.Logf("process 1: applied; Retrieve %s = %s; %d claims bound to sw_if_index %d: %v", nic, compact, len(recs), tapIdx, recs)

	// 2. a NEW agent process on the same state dir still retrieves them: its first resync creates nothing
	stop(p1)
	p2 := start(2)
	c = dialAgent(t, cfg.Socket)
	waitReady(t, c)
	after := retrieve(c)
	if !proto.Equal(before, after) {
		t.Fatalf("process 2 Retrieve differs:\nbefore %v\nafter  %v", protojson.Format(before), protojson.Format(after))
	}
	resync := ""
	if b, err := os.ReadFile(logs[1]); err == nil { //nolint:gosec // test temp dir
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "resync finished") || strings.Contains(l, "reconcile done") {
				t.Logf("process 2: %s", l)
				resync += l + "\n"
			}
		}
	}
	if resync == "" || strings.Contains(resync, "created:") || strings.Contains(resync, "updated:") {
		t.Fatalf("process 2's resync changed VPP (claims not retrieved from the persisted store):\n%s", resync)
	}
	t.Logf("process 2: Retrieve == process 1's, first resync created nothing")

	// 3. removing them through the agent restores the defaults and releases both claims
	resp, err = c.Apply(actx, &vrxv1.ApplyRequest{TxnId: owner + "-claims-2", Subsystems: []string{"interfaces"}})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || resp.GetSummary().GetDeleted() != 2 {
		t.Fatalf("remove: %v %v", err, resp)
	}
	if recs := claims(); len(recs) != 0 {
		t.Fatalf("claims left after the delete: %v", recs)
	}
	stop(p2)
	t.Logf("removed through the agent: summary %v, claims file empty", resp.GetSummary())
}
