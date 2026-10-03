package lcp

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestLeftoverLocalClearedOnHost (TD-lcp-leftover-local-path): in this slot's own VRF (table base +
// 224, never table 0), the API (*,224.0.0.0/24) local path S-lcp-netns-224-accept Q2 can leave
// behind (no Accept) is removed when a default-namespace pair is added to the table.
func TestLeftoverLocalClearedOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, Plugin, &lcp.LcpItfPairAddDelV3{}, &lcp.LcpItfPairGet{}, &lcp.LcpDefaultNsGet{})
	c := h.Client()
	ctx := context.Background()
	lockGlobalsShared(t) // reads the VPP-global lcp default netns (shared-host-rules §7)
	if def, err := NewDefaultNetns(c).Current(ctx); err != nil {
		t.Fatal(err)
	} else if def != "" {
		t.Skipf("default netns %q set by someone else: a pair with netns \"\" would land there", def)
	}
	table := vpptest.TableBase(t) + 224
	ifName, idx := h.Loopback(t, 90)
	ipsvc := ip.NewServiceClient(c)
	if _, err := ipsvc.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: ip.IPTable{TableID: table, Name: h.Owner + ":lcp-leftover"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { // runs after the unbind; also flushes any API mroute left in the table
		if _, err := ipsvc.IPTableAddDel(context.Background(), &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: table}}); err != nil {
			t.Errorf("cleanup ip_table_add_del del %d: %v", table, err)
		}
	})
	ifsvc := interfaces.NewServiceClient(c)
	if _, err := ifsvc.SwInterfaceSetTable(ctx, &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(idx), VrfID: table}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := ifsvc.SwInterfaceSetTable(context.Background(), &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
			t.Errorf("cleanup sw_interface_set_table %s → 0: %v", ifName, err)
		}
	})
	t.Logf("%s (sw_if_index %d) in table %d; empty table: %s", ifName, idx, table, mfibShowIn(table))

	// the leftover: the API source's local path and no Accept (Q2 keeps it after a dump error)
	if _, err := ipsvc.IPMrouteAddDel(ctx, &ip.IPMrouteAddDel{IsAdd: true, IsMultipath: true, Route: acceptRoute(table, localPath)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = ipsvc.IPMrouteAddDel(context.Background(), &ip.IPMrouteAddDel{IsAdd: false, IsMultipath: true, Route: acceptRoute(table, localPath)})
	})
	paths, found, err := mroute224(ctx, c, table)
	if err != nil || !found {
		t.Fatalf("leftover not in table %d: found %v, %v", table, found, err)
	}
	t.Logf("ip_mroute_dump table %d (*,224.0.0.0/24) before: %+v", table, paths)
	before := mfibShowIn(table)
	t.Logf("before: %s", before)
	if !strings.Contains(before, "src:API") {
		t.Fatal("no API source in the table before the pair add")
	}

	pd := NewItfPair(c, h.Owner)
	v := ItfPair{Interface: ifName, HostIfName: h.Owner + "-lcp90", HostIfType: "tap"}.Proto()
	t.Cleanup(func() { _ = pd.Delete(context.Background(), v, nil) })
	meta, err := pd.Create(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pair %s ↔ %s-lcp90 created: %+v", ifName, h.Owner, meta)
	after := mfibShowIn(table)
	t.Logf("after: %s", after)
	if _, found, err := mroute224(ctx, c, table); err != nil || found {
		t.Fatalf("(*,224.0.0.0/24) still in table %d after the pair add: found %v, %v", table, found, err)
	}
	if strings.Contains(after, "src:API") {
		t.Fatal("API source still in the table after the pair add")
	}
}

// lockGlobalsShared holds dfkittest.GlobalsLock shared until the test ends: this test only reads a
// VPP-global (shared-host-rules §7); tests that set one hold it exclusively (Host.LockGlobals).
func lockGlobalsShared(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile(dfkittest.GlobalsLock, os.O_RDONLY|os.O_CREATE, 0o644) //nolint:gosec // shared lock file, no content
	if err != nil {
		t.Fatalf("globals lock: %v", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		t.Fatalf("flock -s %s: %v", dfkittest.GlobalsLock, err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	})
}
