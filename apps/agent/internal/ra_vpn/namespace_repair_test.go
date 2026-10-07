package ravpn

import (
	"context"
	"os"
	"testing"

	"ngfw/agent/internal/vpp/bootid"
)

func TestStoppedRepairRequiresRuntimeGuard(t *testing.T) {
	h := &FixedNamespaceHandoff{}
	if h.ExportExistingRepair(context.Background(), networkFixture()) == nil {
		t.Fatal("repair accepted missing runtime barrier")
	}
	calls := 0
	h.Guard = func(context.Context, *NetworkPlan) error { calls++; return ErrBoundary }
	if h.ExportExistingRepair(context.Background(), networkFixture()) == nil || calls != 1 {
		t.Fatal("repair bypassed failed runtime barrier")
	}
}

func TestStoppedRepairRequiresStableManagerAndGoneGeneration(t *testing.T) {
	manager := (bootid.Reader{}).ForPID(os.Getpid())
	oldVPP := manager
	oldVPP.PID += 100
	oldVPP.StartTime += 100
	newVPP := oldVPP
	newVPP.PID++
	newVPP.StartTime++
	old := []MountTarget{{Boot: oldVPP, MountInode: 42}, {Boot: manager, MountInode: 42}}
	current := []MountTarget{{Boot: newVPP, MountInode: 42}, {Boot: manager, MountInode: 42}}
	calls := 0
	stable, err := repairStableMountTargets(old, current, func(identity bootid.Identity) bool { calls++; return identity.Equal(oldVPP) })
	if err != nil || len(stable) != 1 || !stable[0].Boot.Equal(manager) || calls != 1 {
		t.Fatal("failed safe stopped generation proof", err)
	}
	if _, err := repairStableMountTargets(old, current, func(bootid.Identity) bool { return false }); err == nil {
		t.Fatal("retired live old generation")
	}
	changed := append([]MountTarget(nil), current...)
	changed[0].MountInode = 43
	if _, err := repairStableMountTargets(old, changed, func(bootid.Identity) bool { return true }); err == nil {
		t.Fatal("adopted inaccessible changed mount namespace")
	}
	changed = append([]MountTarget(nil), current...)
	changed[1].Boot.StartTime++
	if _, err := repairStableMountTargets(old, changed, func(bootid.Identity) bool { return true }); err == nil {
		t.Fatal("adopted targets without unchanged manager role")
	}
}
