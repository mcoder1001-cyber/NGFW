package agent

// F-default-vpp-nics (D-164): the HostNics RPC on this host (NGFW_INTEGRATION=1, shared lab lock). It enumerates the
// real host NICs read-only — it binds nothing, never touches /etc/vpp and never restarts VPP (D-012). The test
// asserts the reference host's shape (ens192 = management, the six data NICs = non-management, none bound to DPDK
// because startup.conf uses no-pci) and that VPP's restart count and startup.conf are unchanged before/after.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/vpp/vpptest"
)

func vppNRestarts(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts", "--value").CombinedOutput()
	v := strings.TrimSpace(string(out))
	if err != nil || v == "" {
		t.Fatalf("systemctl show vpp -p NRestarts: %v (%q)", err, out)
	}
	return v
}

func startupConfSha(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("/etc/vpp/startup.conf")
	if err != nil {
		t.Fatalf("read /etc/vpp/startup.conf: %v", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestHostNicsOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)

	restartsBefore := vppNRestarts(t)
	confBefore := startupConfSha(t)
	t.Logf("before: vpp NRestarts=%s /etc/vpp/startup.conf sha256=%s", restartsBefore, confBefore)

	cfg := hostConfig(t, owner)
	a, err := Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Stop)
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := c.HostNics(ctx, &ngfwv1.HostNicsRequest{})
	if err != nil {
		t.Fatalf("HostNics: %v", err)
	}

	t.Logf("HostNics: %d NIC(s)", len(resp.GetNics()))
	for _, n := range resp.GetNics() {
		t.Logf("  %-8s pci=%s driver=%-8s mac=%s management=%v bound_to_dpdk=%v link_up=%v",
			n.GetNetdev(), n.GetPci(), n.GetDriver(), n.GetMac(), n.GetIsManagement(), n.GetBoundToDpdk(), n.GetLinkUp())
	}
	for _, note := range resp.GetManagementNotes() {
		t.Logf("  note: %s", note)
	}

	var mgmt, data int
	var mgmtNames []string
	for _, n := range resp.GetNics() {
		if n.GetBoundToDpdk() {
			t.Errorf("%s reports bound_to_dpdk=true; startup.conf uses no-pci, nothing should be bound", n.GetNetdev())
		}
		if n.GetPci() == "" {
			t.Errorf("%s has no PCI address but was enumerated", n.GetNetdev())
		}
		if n.GetIsManagement() {
			mgmt++
			mgmtNames = append(mgmtNames, n.GetNetdev())
		} else {
			data++
		}
	}
	if mgmt < 1 {
		t.Errorf("expected at least one management NIC, got none (%v)", mgmtNames)
	}
	if data < 1 {
		t.Errorf("expected at least one non-management data NIC, got %d", data)
	}
	// on the reference host (docs/lab/host-ngfw-a.md, recognised by ens192 = 0000:0b:00.0) the exact set is asserted
	got := map[string]string{}
	for _, n := range resp.GetNics() {
		got[n.GetNetdev()] = n.GetPci()
	}
	if got["ens192"] == "0000:0b:00.0" {
		want := map[string]string{
			"ens161": "0000:04:00.0", "ens192": "0000:0b:00.0", "ens193": "0000:0c:00.0", "ens224": "0000:13:00.0",
			"ens225": "0000:14:00.0", "ens256": "0000:1b:00.0", "ens257": "0000:1c:00.0",
		}
		if !maps.Equal(got, want) {
			t.Errorf("ngfw-a NIC set = %v, want %v", got, want)
		}
		if !slices.Equal(mgmtNames, []string{"ens192"}) {
			t.Errorf("ngfw-a management = %v, want [ens192]", mgmtNames)
		}
	} else {
		t.Logf("not the reference host (no ens192 = 0000:0b:00.0): exact-set check skipped")
	}

	a.Stop()
	restartsAfter, confAfter := vppNRestarts(t), startupConfSha(t)
	t.Logf("after:  vpp NRestarts=%s /etc/vpp/startup.conf sha256=%s", restartsAfter, confAfter)
	if got := restartsAfter; got != restartsBefore {
		t.Errorf("VPP NRestarts changed: before=%q after=%q (HostNics must not restart VPP)", restartsBefore, got)
	}
	if got := confAfter; got != confBefore {
		t.Errorf("/etc/vpp/startup.conf changed: before=%s after=%s (HostNics must not touch /etc/vpp)", confBefore, got)
	}
}
