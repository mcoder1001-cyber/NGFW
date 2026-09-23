// Package df2test holds the shared-host integration fixtures of the DF-2 descriptor tests:
// a govpp connection to the host VPP (behind vpptest's VRX_INTEGRATION gate and the shared
// lab lock) and slot-prefixed fixtures — tagged loopbacks, VRFs, addresses and a permit-all
// ACL — that delete themselves in t.Cleanup. Every fixture message comes from
// apps/agent/binapi; nothing here shells out.
package df2test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/core"

	"ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/acl_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// Timeout bounds every fixture call.
const Timeout = 10 * time.Second

// Client wraps a live govpp connection as a vpp.Client.
type Client struct{ *core.Connection }

// Connected implements vpp.Client.
func (Client) Connected() bool { return true }

var _ vpp.Client = Client{}

// Connect skips unless VRX_INTEGRATION=1, takes the shared lab lock and connects to
// /run/vpp/api.sock; the connection closes in t.Cleanup.
func Connect(t testing.TB) vpp.Client {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	conn, err := core.Connect(socketclient.NewVppClient(socketclient.DefaultSocketName))
	if err != nil {
		t.Fatalf("connect to VPP: %v", err)
	}
	t.Cleanup(conn.Disconnect)
	return Client{conn}
}

// Ctx returns a context bounded by Timeout.
func Ctx(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	t.Cleanup(cancel)
	return ctx
}

// Loopback creates loop<slot><ii> tagged "<prefix>:loop<slot><ii>" and admin-up, and deletes
// it in t.Cleanup. A leftover of the same name from an earlier run is removed first.
func Loopback(t testing.TB, c vpp.Client, i int) (name string, swIfIndex uint32) {
	t.Helper()
	return loopback(t, c, i, true)
}

// UntaggedLoopback creates loop<slot><ii> WITHOUT an owner tag — the stand-in for a physical
// port (DPDK NICs carry no tag) in the claim-store tests — and deletes it in t.Cleanup. The
// name is still inside the slot's range; an existing interface of that name is never touched.
func UntaggedLoopback(t testing.TB, c vpp.Client, i int) (name string, swIfIndex uint32) {
	t.Helper()
	return loopback(t, c, i, false)
}

func loopback(t testing.TB, c vpp.Client, i int, tagged bool) (name string, swIfIndex uint32) {
	t.Helper()
	ctx := Ctx(t)
	inst := vpptest.LoopbackInstance(t, i)
	name = fmt.Sprintf("loop%d", inst)
	svc := interfaces.NewServiceClient(c)
	if ifs, err := df2.DumpInterfaces(ctx, c, vpptest.Prefix(t)); err == nil {
		if idx, err := ifs.Index(name); err == nil {
			if !tagged || !ifs.Owned(uint32(idx)) {
				t.Fatalf("%s exists and is not provably ours: refusing to touch it", name)
			}
			t.Logf("removing leftover %s (sw_if_index %d)", name, idx)
			_, _ = svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: idx})
		}
	}
	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		t.Fatalf("create_loopback_instance %d: %v", inst, err)
	}
	swIfIndex = uint32(rep.SwIfIndex)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()
		if _, err := svc.DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: rep.SwIfIndex}); err != nil {
			t.Errorf("cleanup delete_loopback %s: %v", name, err)
		}
	})
	if !tagged {
		if _, err := svc.SwInterfaceSetFlags(ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: rep.SwIfIndex, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP}); err != nil {
			t.Fatalf("sw_interface_set_flags %s: %v", name, err)
		}
		return name, swIfIndex
	}
	tag, err := vpp.OwnerTag(vpptest.Prefix(t), name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: rep.SwIfIndex, Tag: tag}); err != nil {
		t.Fatalf("sw_interface_tag_add_del %s: %v", name, err)
	}
	if _, err := svc.SwInterfaceSetFlags(ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: rep.SwIfIndex, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP}); err != nil {
		t.Fatalf("sw_interface_set_flags %s: %v", name, err)
	}
	return name, swIfIndex
}

// VRF creates IP table id (IPv4 or IPv6) named "<prefix>-<name>" and deletes it in t.Cleanup.
// The id must be inside the slot's range.
func VRF(t testing.TB, c vpp.Client, id uint32, ip6 bool, name string) {
	t.Helper()
	base := vpptest.TableBase(t)
	if id < base || id > base+999 {
		t.Fatalf("table id %d outside the slot range %d..%d", id, base, base+999)
	}
	ctx := Ctx(t)
	table := ip.IPTable{TableID: id, IsIP6: ip6, Name: vpptest.Prefix(t) + "-" + name}
	if _, err := ip.NewServiceClient(c).IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: table}); err != nil {
		t.Fatalf("ip_table_add_del %d: %v", id, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()
		if _, err := ip.NewServiceClient(c).IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: false, Table: table}); err != nil {
			t.Errorf("cleanup ip_table_add_del %d: %v", id, err)
		}
	})
}

// BindVRF moves an interface into table id for one address family (back to 0 in t.Cleanup).
func BindVRF(t testing.TB, c vpp.Client, swIfIndex, id uint32, ip6 bool) {
	t.Helper()
	svc := interfaces.NewServiceClient(c)
	if _, err := svc.SwInterfaceSetTable(Ctx(t), &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(swIfIndex), IsIPv6: ip6, VrfID: id}); err != nil {
		t.Fatalf("sw_interface_set_table %d → %d: %v", swIfIndex, id, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()
		_, _ = svc.SwInterfaceSetTable(ctx, &interfaces.SwInterfaceSetTable{SwIfIndex: interface_types.InterfaceIndex(swIfIndex), IsIPv6: ip6, VrfID: 0})
	})
}

// AddAddress assigns prefix (e.g. "10.3.1.1/24") to an interface and removes it in t.Cleanup.
func AddAddress(t testing.TB, c vpp.Client, swIfIndex uint32, prefix string) {
	t.Helper()
	p, err := ip_types.ParseAddressWithPrefix(prefix)
	if err != nil {
		t.Fatalf("prefix %q: %v", prefix, err)
	}
	svc := interfaces.NewServiceClient(c)
	req := &interfaces.SwInterfaceAddDelAddress{SwIfIndex: interface_types.InterfaceIndex(swIfIndex), IsAdd: true, Prefix: p}
	if _, err := svc.SwInterfaceAddDelAddress(Ctx(t), req); err != nil {
		t.Fatalf("sw_interface_add_del_address %s: %v", prefix, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()
		del := *req
		del.IsAdd = false
		_, _ = svc.SwInterfaceAddDelAddress(ctx, &del)
	})
}

// ACL creates a permit-all IPv4 ACL tagged "<prefix>:<name>" (the tag DF-4's descriptor
// stamps, which abf resolves) and deletes it in t.Cleanup.
func ACL(t testing.TB, c vpp.Client, name string) uint32 {
	t.Helper()
	tag, err := vpp.OwnerTag(vpptest.Prefix(t), name)
	if err != nil {
		t.Fatal(err)
	}
	any4, _ := ip_types.ParsePrefix("0.0.0.0/0")
	rule := acl_types.ACLRule{IsPermit: acl_types.ACL_ACTION_API_PERMIT, SrcPrefix: any4, DstPrefix: any4, SrcportOrIcmptypeLast: 65535, DstportOrIcmpcodeLast: 65535}
	svc := acl.NewServiceClient(c)
	rep, err := svc.ACLAddReplace(Ctx(t), &acl.ACLAddReplace{ACLIndex: ^uint32(0), Tag: tag, Count: 1, R: []acl_types.ACLRule{rule}})
	if err != nil {
		t.Fatalf("acl_add_replace %s: %v", tag, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()
		if _, err := svc.ACLDel(ctx, &acl.ACLDel{ACLIndex: rep.ACLIndex}); err != nil {
			t.Errorf("cleanup acl_del %d: %v", rep.ACLIndex, err)
		}
	})
	return rep.ACLIndex
}

// SkipIfNotLoaded skips t when err says the plugin is not loaded on this host
// (skip-unless-plugin-loaded); other errors fail the test.
func SkipIfNotLoaded(t testing.TB, plugin string, err error) {
	t.Helper()
	if errors.Is(err, df2.ErrPluginNotLoaded) {
		t.Skipf("skip-unless-plugin-loaded: %s is not loaded on this VPP (%v)", plugin, err)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Slot exposes the numeric slot for table ids and addresses ("10.<slot>.x.y", "2001:db8:<slot>::").
func Slot(t testing.TB) int { return vpptest.Slot(t) }

// Hold pauses the test for VRX_DF2_HOLD (a Go duration, e.g. "20s") while the objects it
// created exist, so an operator can capture `vppctl show …` evidence from a shell. Unset in
// CI, it returns immediately.
func Hold(t testing.TB) {
	t.Helper()
	v := os.Getenv("VRX_DF2_HOLD")
	if v == "" {
		return
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		t.Fatalf("VRX_DF2_HOLD=%q: %v", v, err)
	}
	t.Logf("holding objects for %s (VRX_DF2_HOLD)", d)
	time.Sleep(d)
}
