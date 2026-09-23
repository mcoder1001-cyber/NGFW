package df6test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/mpls"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// EnvSocket overrides the VPP API socket for integration tests (default socketclient's).
const EnvSocket = "VRX_VPP_API_SOCKET"

// hostClient wraps *core.Connection as a vpp.Client until P05's client exists.
type hostClient struct {
	*core.Connection
}

func (hostClient) Connected() bool { return true }

var _ vpp.Client = hostClient{}

// Host is an integration-test session on the shared VPP: the connection, the slot's scope
// and the fixtures created so far (deleted in reverse order by t.Cleanup).
type Host struct {
	T      testing.TB
	Client vpp.Client
	Owner  string
	Slot   int
	Ctx    context.Context
}

// Connect skips unless VRX_INTEGRATION=1, takes the shared lab lock and connects to the host
// VPP. Everything the test creates must carry the slot prefix / ranges from Scope.
func Connect(t testing.TB) *Host {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	sock := os.Getenv(EnvSocket)
	if sock == "" {
		sock = socketclient.DefaultSocketName
	}
	conn, err := core.Connect(socketclient.NewVppClient(sock))
	if err != nil {
		t.Fatalf("connect to VPP at %s: %v", sock, err)
	}
	t.Cleanup(conn.Disconnect)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return &Host{
		T:      t,
		Client: hostClient{conn},
		Owner:  owner,
		Slot:   slot,
		Ctx:    ctx,
	}
}

// Reconnect opens a second, independent API connection (a restarted agent's), closed in
// Cleanup.
func (h *Host) Reconnect() vpp.Client {
	h.T.Helper()
	sock := os.Getenv(EnvSocket)
	if sock == "" {
		sock = socketclient.DefaultSocketName
	}
	conn, err := core.Connect(socketclient.NewVppClient(sock))
	if err != nil {
		h.T.Fatalf("reconnect to VPP at %s: %v", sock, err)
	}
	h.T.Cleanup(conn.Disconnect)
	return hostClient{conn}
}

// Table returns table id base+i of the slot.
func (h *Host) Table(i uint32) uint32 { return vpptest.TableBase(h.T) + i }

// IP4 returns 10.<slot>.<net>.<host>.
func (h *Host) IP4(network, host int) string {
	return fmt.Sprintf("10.%d.%d.%d", h.Slot, network, host)
}

// IP6 returns fd<slot>:<net>::<host>.
func (h *Host) IP6(network, host int) string {
	return fmt.Sprintf("fd%02d:%x::%x", h.Slot, network, host)
}

// Name returns "<owner>-<suffix>".
func (h *Host) Name(suffix string) string { return h.Owner + "-" + suffix }

// Loopback creates loop<slot><ii> tagged "<owner>:loop<slot><ii>" with the given address
// (prefix string, "" for none), admin up, and deletes it in Cleanup. It returns the name
// and sw_if_index.
func (h *Host) Loopback(i int, prefix string) (string, uint32) {
	h.T.Helper()
	inst := vpptest.LoopbackInstance(h.T, i)
	name := fmt.Sprintf("loop%d", inst)
	svc := interfaces.NewServiceClient(h.Client)
	rep, err := svc.CreateLoopbackInstance(h.Ctx, &interfaces.CreateLoopbackInstance{IsSpecified: true, UserInstance: inst})
	if err != nil {
		h.T.Fatalf("create_loopback_instance %d: %v", inst, err)
	}
	idx := rep.SwIfIndex
	h.T.Cleanup(func() {
		if _, err := svc.DeleteLoopback(context.Background(), &interfaces.DeleteLoopback{SwIfIndex: idx}); err != nil {
			h.T.Errorf("cleanup delete_loopback %s: %v", name, err)
		}
	})
	tag, err := vpp.OwnerTag(h.Owner, name)
	if err != nil {
		h.T.Fatal(err)
	}
	if _, err := svc.SwInterfaceTagAddDel(h.Ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: idx, Tag: tag}); err != nil {
		h.T.Fatalf("tag %s: %v", name, err)
	}
	if prefix != "" {
		p, err := ip_types.ParseAddressWithPrefix(prefix)
		if err != nil {
			h.T.Fatal(err)
		}
		if _, err := svc.SwInterfaceAddDelAddress(h.Ctx, &interfaces.SwInterfaceAddDelAddress{SwIfIndex: idx, IsAdd: true, Prefix: p}); err != nil {
			h.T.Fatalf("address %s on %s: %v", prefix, name, err)
		}
	}
	if _, err := svc.SwInterfaceSetFlags(h.Ctx, &interfaces.SwInterfaceSetFlags{SwIfIndex: idx, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP}); err != nil {
		h.T.Fatalf("admin up %s: %v", name, err)
	}
	return name, uint32(idx)
}

// IPTable creates IP table id (v4 or v6) named "<owner>-t<id>" and deletes it in Cleanup.
func (h *Host) IPTable(id uint32, ip6 bool) {
	h.T.Helper()
	svc := ip.NewServiceClient(h.Client)
	tbl := ip.IPTable{TableID: id, IsIP6: ip6, Name: fmt.Sprintf("%s-t%d", h.Owner, id)}
	if _, err := svc.IPTableAddDel(h.Ctx, &ip.IPTableAddDel{IsAdd: true, Table: tbl}); err != nil {
		h.T.Fatalf("ip_table_add_del %d ip6=%v: %v", id, ip6, err)
	}
	h.T.Cleanup(func() {
		if _, err := svc.IPTableAddDel(context.Background(), &ip.IPTableAddDel{IsAdd: false, Table: tbl}); err != nil {
			h.T.Errorf("cleanup ip_table_add_del %d: %v", id, err)
		}
	})
}

// MPLSTable takes a reference on MPLS table id (mpls_table_add_del is reference counted in
// VPP, so a table another worker holds is not disturbed) and releases it in Cleanup. This is
// the fixture standing in for DF-7's mpls-table descriptor.
func (h *Host) MPLSTable(id uint32) {
	h.T.Helper()
	svc := mpls.NewServiceClient(h.Client)
	tbl := mpls.MplsTable{MtTableID: id, MtName: fmt.Sprintf("%s-mpls%d", h.Owner, id)}
	if _, err := svc.MplsTableAddDel(h.Ctx, &mpls.MplsTableAddDel{MtIsAdd: true, MtTable: tbl}); err != nil {
		h.T.Fatalf("mpls_table_add_del %d: %v", id, err)
	}
	h.T.Cleanup(func() {
		if _, err := svc.MplsTableAddDel(context.Background(), &mpls.MplsTableAddDel{MtIsAdd: false, MtTable: tbl}); err != nil {
			h.T.Errorf("cleanup mpls_table_add_del %d: %v", id, err)
		}
	})
}

// Invoke is a raw request/reply for fixtures without a generated client.
func (h *Host) Invoke(req, reply api.Message) error { return h.Client.Invoke(h.Ctx, req, reply) }

// EnvHold makes Hold pause (seconds) so the operator can run `vppctl show …` while the test's
// objects exist (evidence for the status file). Unset = no pause.
const EnvHold = "VRX_DF6_HOLD"

// Hold pauses for VRX_DF6_HOLD seconds when set.
func (h *Host) Hold() {
	if s := os.Getenv(EnvHold); s != "" {
		if d, err := time.ParseDuration(s + "s"); err == nil {
			h.T.Logf("holding %s for evidence capture", d)
			time.Sleep(d)
		}
	}
}

// PlanFor computes the plan the reconciler would make for desired against d.Retrieve(), with
// the semantics documented in internal/scheduler (absent → Create, !proto.Equal → Update,
// retrieved and not desired → Delete). P05's reconciler is not in this branch; this is the
// idempotency check of the DF-6 acceptance ("applying the same desired state twice yields an
// empty plan").
func PlanFor(ctx context.Context, d scheduler.Descriptor, desired ...proto.Message) (scheduler.Plan, error) {
	actual, err := d.Retrieve(ctx)
	if err != nil {
		return scheduler.Plan{}, err
	}
	have := map[scheduler.Key]scheduler.KV{}
	for _, kv := range actual {
		have[kv.Key] = kv
	}
	var p scheduler.Plan
	want := map[scheduler.Key]bool{}
	for _, v := range desired {
		k := d.KeyOf(v)
		want[k] = true
		a, ok := have[k]
		switch {
		case !ok:
			p.Create = append(p.Create, scheduler.KV{Key: k, Value: v})
		case !proto.Equal(a.Value, v):
			p.Update = append(p.Update, scheduler.KV{Key: k, Value: v})
		}
	}
	for k, kv := range have {
		if !want[k] {
			p.Delete = append(p.Delete, kv)
		}
	}
	return p, nil
}

// AssertEmptyPlan fails unless desired (already applied) plans to nothing, and logs the plan
// sizes as evidence.
func (h *Host) AssertEmptyPlan(d scheduler.Descriptor, desired ...proto.Message) {
	h.T.Helper()
	p, err := PlanFor(h.Ctx, d, desired...)
	if err != nil {
		h.T.Fatalf("plan %s: %v", d.Name(), err)
	}
	h.T.Logf("re-apply plan %s: create=%d update=%d delete=%d (empty=%v)", d.Name(), len(p.Create), len(p.Update), len(p.Delete), p.Empty())
	if !p.Empty() {
		h.T.Errorf("re-applying %s is not a no-op: %+v", d.Name(), p)
	}
}

// Msgs converts a typed slice to []proto.Message.
func Msgs[T proto.Message](xs []T) []proto.Message {
	out := make([]proto.Message, 0, len(xs))
	for _, x := range xs {
		out = append(out, x)
	}
	return out
}
