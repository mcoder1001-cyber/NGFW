package core_test

import (
	"context"
	"path/filepath"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// lockIP6 makes the model behave like VPP 26.06 after nat64 locked table id's IPv6 family: a
// delete answers success and removes nothing (only the API lock goes).
func lockIP6(v *coretest.VPP, id uint32) {
	v.On("ip_table_add_del", func(m api.Message) ([]api.Message, error) {
		req := m.(*ip.IPTableAddDel)
		k, v6 := req.Table.TableID, req.Table.IsIP6
		switch {
		case req.IsAdd && !v.HasTable(k, v6):
			v.SetTableName(k, v6, req.Table.Name)
		case !req.IsAdd && (k != id || !v6):
			v.DeleteTable(k, v6)
		}
		return []api.Message{&ip.IPTableAddDelReply{}}, nil
	})
}

// TD-26: deleting a VRF whose IPv6 table VPP keeps locked applies (no verify failure, no
// rollback), the table stays in VPP, and Retrieve hides it until the VRF is created again or VPP
// restarts.
func TestVRFDeleteKeptLockedByVPP(t *testing.T) {
	restore := bootid.SetProcRoot(filepath.Join(t.TempDir(), "a"))
	defer restore()
	v := coretest.New()
	r := newRig(t, "w26", v)
	ctx := context.Background()
	vrf := []scheduler.KV{{Key: "vrf/4064", Value: &core.Table{Id: 4064, Vrf: "n64"}}}
	if res := r.s.Apply(ctx, vrf, nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	lockIP6(v, 4064)
	if res := r.s.Apply(ctx, nil, nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("delete of a VPP-locked VRF: outcome %s err %v", res.Outcome, res.Err)
	}
	if v.HasTable(4064, false) || !v.HasTable(4064, true) {
		t.Fatal("want the IPv4 family gone and the locked IPv6 family left")
	}
	kvs, err := r.desc(core.VRFName).Retrieve(ctx)
	if err != nil || len(kvs) != 0 {
		t.Fatalf("Retrieve must hide the kept table: %v %v", kvs, err)
	}
	// a resync with the same (empty) config plans nothing
	if res := r.s.Apply(ctx, nil, nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	// creating the VRF again re-asserts the table and it is reported again
	if res := r.s.Apply(ctx, vrf, nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	kvs, err = r.desc(core.VRFName).Retrieve(ctx)
	if err != nil || len(kvs) != 1 || kvs[0].Value.(*core.Table).GetMissingIp6() || kvs[0].Value.(*core.Table).GetMissingIp4() {
		t.Fatalf("recreated VRF: %v %v", kvs, err)
	}
	// deleted and kept again; a new VPP boot expires the record and the table shows up again
	if res := r.s.Apply(ctx, nil, nil); res.Outcome != scheduler.OutcomeApplied {
		t.Fatal(res.Err)
	}
	dir := filepath.Join(t.TempDir(), "b")
	if err := bootid.WriteFakeProc(dir, "boot-b", nil); err != nil {
		t.Fatal(err)
	}
	restore2 := bootid.SetProcRoot(dir)
	defer restore2()
	kvs, err = r.desc(core.VRFName).Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !kvs[0].Value.(*core.Table).GetMissingIp4() {
		t.Fatalf("after a boot change the table must be reported again: %v %v", kvs, err)
	}
}
