package agent

import (
	"context"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io/fs"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/autoblock"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/scheduler"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Unit-only memory host descriptor: real VPP ACL messages use coretest, and the
// exact nftables desired table is asserted. This is not a kernel packet test.
type autoBlockHostMemory struct {
	mu    sync.Mutex
	value proto.Message
	fail  bool
}

func (*autoBlockHostMemory) Name() string                                      { return nftables.DescriptorName }
func (*autoBlockHostMemory) KeyOf(proto.Message) scheduler.Key                 { return nftables.Key }
func (*autoBlockHostMemory) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (d *autoBlockHostMemory) Create(_ context.Context, v proto.Message) (any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.value = proto.Clone(v)
	return nil, nil
}
func (d *autoBlockHostMemory) Update(ctx context.Context, _, v proto.Message, _ any) (any, error) {
	if d.fail {
		return nil, errors.New("injected host enforcement rejection")
	}
	return d.Create(ctx, v)
}
func (d *autoBlockHostMemory) Delete(context.Context, proto.Message, any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.value = nil
	return nil
}
func (d *autoBlockHostMemory) Retrieve(context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.value == nil {
		return nil, nil
	}
	return []scheduler.KV{{Key: nftables.Key, Value: proto.Clone(d.value)}}, nil
}
func TestAutoBlockRuntimeLifecycleOnFake(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s, _ := newACLSvc(t, v, dir, false)
	host := &autoBlockHostMemory{}
	reg := scheduler.NewRegistry()
	for _, d := range s.sched.Registry().Descriptors() {
		if d.Name() != nftables.DescriptorName {
			reg.Register(d)
		}
	}
	reg.Register(host)
	s.sched = scheduler.New(reg, nil)
	s.sched.VerifyRetries = 0
	ds := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]}},"security":{"autoBlock":{"enabled":true,"maxEntries":100,"allowlist":["192.0.2.8"],"rules":[{"source":"webLogin","enabled":true,"threshold":10,"windowSec":60,"blockSec":60,"escalate":true,"maxBlockSec":300}]}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "auto-config", DesiredState: ds}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	before := proto.Clone(s.st.desired)
	now := time.Now()
	s.now = func() time.Time { return now }
	req := &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{{Source: "192.0.2.7", ExpiresAt: timestamppb.New(now.Add(time.Minute))}, {Source: "192.0.2.8", ExpiresAt: timestamppb.New(now.Add(time.Minute))}}}
	resp, err := s.AutoBlockSet(context.Background(), req)
	if err != nil || resp.GetActiveEntries() != 1 {
		t.Fatalf("set: %v %v", resp, err)
	}
	if !proto.Equal(before, s.st.desired) || s.Health().GetLastTxnId() != "auto-config" {
		t.Fatal("runtime mutation changed running config/revision")
	}
	if len(v.ACL().ACLs()) != 3 {
		t.Fatalf("ACL runtime set not installed: %v", v.ACL().ACLs())
	}
	if host.value == nil {
		t.Fatal("local-in projection missing")
	}
	table := host.value.(*nftables.HostTable)
	if len(table.Sets) != 1 || len(table.Sets[0].Elements) != 1 || table.Sets[0].Elements[0] != "192.0.2.7/32" {
		t.Fatalf("local-in set unsafe: %v", table)
	}
	// A host rejection rolls back the VPP change and retains the old live set;
	// the new authoritative desired snapshot stays dirty for a bounded retry.
	oldRules := v.ACL().Rules(ownedACL(t, v, "_gb.auto-block.i00"))
	changed := proto.Clone(req).(*ngfwv1.AutoBlockSetRequest)
	changed.Entries[0].Source = "192.0.2.9"
	host.fail = true
	if _, err := s.AutoBlockSet(context.Background(), changed); status.Code(err) != codes.Unavailable {
		t.Fatal("failed enforcement claimed success", err)
	}
	if !s.autoBlock.dirty || !reflect.DeepEqual(oldRules, v.ACL().Rules(ownedACL(t, v, "_gb.auto-block.i00"))) || !proto.Equal(host.value, table) {
		t.Fatal("failed update did not preserve old enforcement/dirty desired")
	}
	host.fail = false
	if _, err := s.reconcileAutoBlockLocked(context.Background()); err != nil {
		t.Fatal("retry failed", err)
	}
	if _, err := s.AutoBlockSet(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	// Security-only updates apply enforcement immediately, with confirm revert
	// restoring it from the independent runtime snapshot rather than config data.
	disabled := proto.Clone(ds.Security).(*ngfwv1.SecurityConfig)
	disabled.AutoBlock.Enabled = proto.Bool(false)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "disable", DesiredState: &ngfwv1.DesiredState{Security: disabled}, Subsystems: []string{"security"}, ConfirmTimeoutSec: 30}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.ACL().ACLs()) != 0 || host.value != nil {
		t.Fatal("security-only disable left blocks")
	}
	s.revertLocked(context.Background(), "disable")
	if len(v.ACL().ACLs()) != 3 || host.value == nil {
		t.Fatal("confirm revert lost runtime set")
	}
	// Partial ACL updates must keep runtime security and interface context.
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "acl-only", DesiredState: &ngfwv1.DesiredState{Acl: &ngfwv1.AclConfig{}}, Subsystems: []string{"acl"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.ACL().ACLs()) != 3 {
		t.Fatal("ACL-only commit bypassed runtime blocks")
	}
	got, err := s.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"acl", "security"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetDesiredState().GetAcl().GetGlobalBlocking().GetLists()[autoblock.ListName] != nil || got.GetDesiredState().GetSecurity() == nil {
		t.Fatal("runtime set leaked into config retrieval or security missing")
	}
	// Cache replay does not use config for runtime state.
	s.Close()
	s2, err := NewService(ServiceConfig{Owner: testOwner, VPP: v, Scheduler: s.sched, StateDir: dir, BeforeTxn: s.beforeTxn, Now: s.now})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if len(s2.autoBlock.entries) != 2 {
		t.Fatal("restart lost cached snapshot")
	}
	// Simulate owned dataplane loss without restarting VPP or another daemon.
	v.ACL().Bind(loopIndex(t, v, "loop711"), 0)
	for idx := range v.ACL().ACLs() {
		v.ACL().DeleteACL(idx)
	}
	if err := host.Delete(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.reconcileAutoBlockLocked(context.Background()); err != nil {
		t.Fatal("cache replay after dataplane loss", err)
	}
	if len(v.ACL().ACLs()) != 3 || host.value == nil {
		t.Fatal("resync failed to recreate runtime enforcement")
	}
	// Agent-clock expiry removes VPP and local-in without an API sweep.
	now = now.Add(2 * time.Minute)
	if _, err := s2.reconcileAutoBlockLocked(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(v.ACL().ACLs()) != 0 || host.value != nil {
		t.Fatal("expiry left enforcement")
	}
	if _, err := s2.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: "foreign"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("owner guard: %v", err)
	}
	if _, err := s2.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{{Source: "192.0.2.0/24"}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("snapshot guard: %v", err)
	}
	if _, err := s2.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: testOwner}); err != nil {
		t.Fatal(err)
	}
	legacy := &ngfwv1.DesiredState{Security: disabled, Acl: &ngfwv1.AclConfig{GlobalBlocking: &ngfwv1.GlobalBlocking{Lists: map[string]*ngfwv1.GlobalBlockingList{autoblock.ListName: {Enabled: proto.Bool(true), AllInterfaces: proto.Bool(true), ProtectHost: proto.Bool(true), Entries: []string{"192.0.2.10/32"}}}}}}
	mustStatus(t, apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "legacy-user-list", DesiredState: legacy, Subsystems: []string{"acl", "security"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	legacyResp, err := s2.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: testOwner})
	if err != nil || legacyResp.GetActiveEntries() != 0 {
		t.Fatal("disabled user list counted as runtime block", err)
	}
	legacyState, err := s2.Retrieve(context.Background(), &ngfwv1.RetrieveRequest{Subsystems: []string{"acl"}})
	if err != nil || legacyState.GetDesiredState().GetAcl().GetGlobalBlocking().GetLists()[autoblock.ListName] == nil {
		t.Fatal("disabled user's list stripped from retrieval", err)
	}

}

func TestAutoBlockJournalAuthority(t *testing.T) {
	enabled := map[string]bool{"ssh": true, "portScan": true, "vpnAuth": true}
	if journalObservationEnabled("vpnAuth", enabled) {
		t.Fatal("test-peer charon journal authorized a native product block")
	}
	if !journalObservationEnabled("ssh", enabled) || !journalObservationEnabled("portScan", enabled) {
		t.Fatal("owned host observations disabled")
	}
}

// The API republishes even an empty, disabled runtime every five seconds. Once
// cleanup was confirmed, that publication must not enqueue another ACL pass or
// rewrite the cache while unrelated configuration transactions need the gate.
func TestAutoBlockConfirmedInactivePublicationDoesNotSchedule(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s, _ := newACLSvc(t, v, dir, false)
	ds := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "ordinary-interface", DesiredState: ds}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	baseline := v.ACL().ACLs()
	var transactions atomic.Int64
	beforeTxn := s.beforeTxn
	s.beforeTxn = func() { transactions.Add(1); beforeTxn() }
	req := &ngfwv1.AutoBlockSetRequest{Owner: testOwner}
	if _, err := s.AutoBlockSet(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if transactions.Load() != 1 || s.autoBlock.dirty || s.autoBlock.fingerprint == "" {
		t.Fatal("initial unknown runtime did not confirm cleanup")
	}
	cacheRoot := os.DirFS(dir)
	cache, err := cacheRoot.Open("auto-block.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	original, err := cache.Stat()
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		resp, callErr := s.AutoBlockSet(context.Background(), req)
		if callErr != nil || resp.GetActiveEntries() != 0 {
			t.Fatalf("confirmed empty publication: %v %v", resp, callErr)
		}
	}
	if transactions.Load() != 1 {
		t.Fatalf("repeated inactive publications scheduled %d ACL transactions", transactions.Load()-1)
	}
	after, err := fs.Stat(cacheRoot, "auto-block.json")
	if err != nil || !os.SameFile(original, after) {
		t.Fatal("no-op publication rewrote the confirmed cache", err)
	}
	if !reflect.DeepEqual(baseline, v.ACL().ACLs()) || !proto.Equal(ds, s.st.desired) {
		t.Fatal("empty runtime publication changed ordinary ACLs or desired state")
	}
	// A persisted empty cache is dirty after restart; it must still execute its
	// first cleanup rather than inheriting the old process's confirmation.
	s.Close()
	s2, err := NewService(ServiceConfig{Owner: testOwner, VPP: v, Scheduler: s.sched, StateDir: dir, BeforeTxn: s.beforeTxn})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if !s2.autoBlock.dirty {
		t.Fatal("cache replay lost the cleanup obligation")
	}
	if _, err := s2.AutoBlockSet(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if transactions.Load() != 2 || s2.autoBlock.dirty {
		t.Fatal("dirty replay was incorrectly treated as a confirmed no-op")
	}
}

func TestAutoBlockInactivePublicationStillClearsRetainedEntries(t *testing.T) {
	v := coretest.New()
	s, _ := newACLSvc(t, v, t.TempDir(), false)
	host := &autoBlockHostMemory{}
	reg := scheduler.NewRegistry()
	for _, d := range s.sched.Registry().Descriptors() {
		if d.Name() != nftables.DescriptorName {
			reg.Register(d)
		}
	}
	reg.Register(host)
	s.sched = scheduler.New(reg, nil)
	s.sched.VerifyRetries = 0
	ds := doc(t, `{"interfaces":{"loop711":{"ipv4":["10.71.1.1/24"]}},"security":{"autoBlock":{"enabled":true,"maxEntries":100}}}`)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "enabled", DesiredState: ds}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	entry := &ngfwv1.AutoBlockRuntimeEntry{Source: "192.0.2.7", ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))}
	if _, err := s.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: testOwner, Entries: []*ngfwv1.AutoBlockRuntimeEntry{entry}}); err != nil {
		t.Fatal(err)
	}
	disabled := proto.Clone(ds.Security).(*ngfwv1.SecurityConfig)
	disabled.AutoBlock.Enabled = proto.Bool(false)
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "disabled", DesiredState: &ngfwv1.DesiredState{Security: disabled}, Subsystems: []string{"security"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var transactions atomic.Int64
	beforeTxn := s.beforeTxn
	s.beforeTxn = func() { transactions.Add(1); beforeTxn() }
	empty := &ngfwv1.AutoBlockSetRequest{Owner: testOwner}
	if _, err := s.AutoBlockSet(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
	if transactions.Load() != 1 || len(s.autoBlock.entries) != 0 || s.autoBlock.dirty || host.value != nil || len(v.ACL().ACLs()) != 0 {
		t.Fatal("disabled retained entries were not explicitly cleared")
	}
	// Enabling protection restores authority even though both caches are empty.
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "reenabled", DesiredState: &ngfwv1.DesiredState{Security: ds.Security}, Subsystems: []string{"security"}}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	count := transactions.Load()
	if _, err := s.AutoBlockSet(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
	if transactions.Load() != count+1 {
		t.Fatal("enabled empty protection lost its reconciliation authority")
	}
}

// The API may omit owner on an empty authoritative publication. Restart must
// accept the persisted effective owner without accepting foreign ownership.
func TestAutoBlockOmittedOwnerSurvivesRestart(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s, _ := newACLSvc(t, v, dir, false)
	req := &ngfwv1.AutoBlockSetRequest{}
	if _, err := s.AutoBlockSet(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if req.GetOwner() != "" {
		t.Fatal("caller request mutated")
	}
	cacheFS := os.DirFS(dir)
	before, err := fs.ReadFile(cacheFS, "auto-block.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AutoBlockSet(context.Background(), &ngfwv1.AutoBlockSetRequest{Owner: "foreign"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner accepted: %v", err)
	}
	after, err := fs.ReadFile(cacheFS, "auto-block.json")
	if err != nil || string(before) != string(after) {
		t.Fatal("rejected request changed cache", err)
	}
	s.Close()
	s2, err := NewService(ServiceConfig{Owner: testOwner, VPP: v, Scheduler: s.sched, StateDir: dir, BeforeTxn: s.beforeTxn, Now: s.now})
	if err != nil {
		t.Fatal("ownerless publication prevented restart", err)
	}
	defer s2.Close()
	if len(s2.autoBlock.entries) != 0 || !s2.autoBlock.dirty {
		t.Fatal("empty snapshot not replayed")
	}
	if err := os.WriteFile(dir+"/auto-block.json", []byte(`{"owner":"foreign"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s2.loadAutoBlock(); err == nil {
		t.Fatal("foreign persisted owner accepted")
	}
}
