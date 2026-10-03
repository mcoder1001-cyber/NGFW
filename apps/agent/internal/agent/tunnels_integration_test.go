package agent

// F-tunnels-host: private VPP, actual agent RPC, owned binary-API loss,
// canonical Retrieve equality, unchanged apply, restart and complete rollback.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/vpp/vpptest"
)

func TestTunnelsOnDisposableVPP(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv("NGFW_DISPOSABLE_VPP") != "1" {
		t.Skip("requires disposable VPP")
	}
	vpptest.LockLab(t)
	baseValue, err := strconv.ParseUint(os.Getenv("NGFW_VPP_TABLE_BASE"), 10, 16)
	base := int(baseValue)
	if err != nil || base < 1000 || base > 32000 || base%1000 != 0 {
		t.Fatal("invalid slot base")
	}
	slot := base / 1000
	owner := vpptest.Prefix(t) + "tu"
	src := fmt.Sprintf("10.%d.7.1", slot)
	replace := strings.NewReplacer("7001", strconv.Itoa(base+1), "7002", strconv.Itoa(base+2), "7003", strconv.Itoa(base+3), "7100", strconv.Itoa(base+100),
		"loop7001", fmt.Sprintf("loop%d071", slot), "to-dc", owner+"-gre", "ipip-a", owner+"-ipip", "vx-l3", owner+"-vxlan", "red", owner+"-red",
		"10.7.1.", fmt.Sprintf("10.%d.7.", slot), "10.254.0.1", fmt.Sprintf("10.%d.251.1", slot), "2001:db8:7::1", fmt.Sprintf("fd00:%x:252::1", slot), "2001:db8:9::1", fmt.Sprintf("fd00:%x:253::1", slot))
	desired := doc(t, replace.Replace(tunnelsDoc))
	// The delete helper expects the exact owned VXLAN instance/VNI tuple.
	desired.Tunnels.Vxlan[owner+"-vxlan"].Vni = proto.Uint32(uint32(baseValue + 3))
	want := &ngfwv1.TunnelsConfig{}
	if err := protojson.Unmarshal([]byte(replace.Replace(tunnelsRetrieved)), want); err != nil {
		t.Fatal(err)
	}
	want.Vxlan[owner+"-vxlan"].Vni = proto.Uint32(uint32(baseValue + 3))
	helper, err := filepath.Abs("../../../../.scratch/tunnels-delete-owned")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Fatal("build test/topology/tunnels/delete-owned.go first", err)
	}
	cfg := hostConfig(t, owner)
	a, err := Start(context.Background(), cfg, "tunnels-host", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := dialAgent(t, cfg.Socket)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_, _ = c.Apply(cleanCtx, &ngfwv1.ApplyRequest{TxnId: owner + "-cleanup", DesiredState: &ngfwv1.DesiredState{}, Subsystems: []string{"interfaces", "vrfs", "tunnels"}})
		a.Stop()
	})
	waitReady(t, c)
	retrieve := func() *ngfwv1.TunnelsConfig {
		t.Helper()
		r, e := c.Retrieve(ctx, &ngfwv1.RetrieveRequest{Subsystems: []string{"interfaces", "vrfs", "tunnels"}})
		if e != nil {
			t.Fatal(e)
		}
		return r.GetDesiredState().GetTunnels()
	}
	assertEqual := func() {
		t.Helper()
		got := retrieve()
		if !proto.Equal(got, want) {
			t.Fatalf("tunnels Retrieve mismatch\ngot %s\nwant %s", protojson.Format(got), protojson.Format(want))
		}
	}
	applyDesired := func(id string) *ngfwv1.ApplyResponse {
		t.Helper()
		r, e := c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + id, DesiredState: desired, Subsystems: []string{"interfaces", "vrfs", "tunnels"}})
		if e != nil || r.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Fatalf("apply %s: %v %s", id, e, protojson.Format(r))
		}
		return r
	}
	applyDesired("-first")
	assertEqual()
	for _, kind := range []string{"gre", "ipip", "vxlan"} {
		out := srv6Show(t, "show", kind, "tunnel")
		if !strings.Contains(out, src) {
			t.Fatal(kind, "missing from VPP dump", out)
		}
		t.Log(kind, out)
	}
	if r := applyDesired("-unchanged"); len(r.GetResults()) != 0 {
		t.Fatal("unchanged apply has operations", protojson.Format(r))
	}
	a.Stop()
	deleteCtx, stop := context.WithTimeout(ctx, 20*time.Second)
	//nolint:gosec // Disposable lab fixture uses an explicitly authorized local executable or evidence directory.
	out, e := exec.CommandContext(deleteCtx, helper, src, strconv.Itoa(base)).CombinedOutput()
	stop()
	if e != nil {
		t.Fatalf("owned loss injection: %v %s", e, out)
	}
	t.Log(string(out))
	start := time.Now()
	restarted, err := Start(context.Background(), cfg, "tunnels-host", nil)
	if err != nil {
		t.Fatal(err)
	}
	a = restarted
	c = dialAgent(t, cfg.Socket)
	for time.Since(start) < 30*time.Second {
		if proto.Equal(retrieve(), want) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assertEqual()
	if time.Since(start) >= 30*time.Second {
		t.Fatal("restart exceeded30s")
	}
	t.Log("restart recovery", time.Since(start))
	if r := applyDesired("-restart-unchanged"); len(r.GetResults()) != 0 {
		t.Fatal("recovered apply has operations", protojson.Format(r))
	}
	r, e := c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "-rollback", DesiredState: &ngfwv1.DesiredState{}, Subsystems: []string{"interfaces", "vrfs", "tunnels"}})
	if e != nil || r.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("rollback %v %s", e, protojson.Format(r))
	}
	got := retrieve()
	if len(got.GetGre())+len(got.GetIpip())+len(got.GetVxlan()) != 0 {
		t.Fatal("rollback metadata residue", protojson.Format(got))
	}
	raw, err := os.ReadFile(filepath.Join(cfg.StateDir, "tunnels-meta-"+owner+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.Entries) != 0 {
		t.Fatal("persisted tunnels.meta residue", string(raw))
	}
	t.Log("persisted tunnels.meta entries=0")
	for _, kind := range []string{"gre", "ipip", "vxlan"} {
		if out := srv6Show(t, "show", kind, "tunnel"); strings.Contains(out, src) {
			t.Fatal("rollback VPP residue", kind, out)
		}
	}
}
