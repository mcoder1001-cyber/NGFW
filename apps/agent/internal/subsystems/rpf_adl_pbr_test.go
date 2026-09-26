package subsystems_test

// F-rpf-adl-pbr through the whole agent service (projection → plan → DF-2 descriptors → Retrieve →
// assemble) on the coretest model: apply, Retrieve == desired, idempotent re-apply, validation
// failures with pointers, agent restart after loss, a VPP restart re-applying the write-only
// allow-list exactly once, rollback deleting attachments before policies.

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/agent"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// Reset drops what was logged so far.
func (s *syncBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// rpfService builds the product registry (subsystems.Register) plus DF-4's acl.acl descriptor — F-acl
// registers it in the product; ABF policies depend on acl.acl/<name> — and the agent service over it.
// It stands for one agent process: a second call on the same dir is an agent restart.
func rpfService(t *testing.T, c vpp.Client, owner, dir string, log *slog.Logger) *agent.Service {
	t.Helper()
	return rpfServiceWith(t, c, owner, dir, log, true)
}

// rpfServiceWith: withACL=false is this build's product registry as it is (no acl.acl descriptor until
// F-acl): the pbr.acl-ref bridge resolves the ACL references.
func rpfServiceWith(t *testing.T, c vpp.Client, owner, dir string, log *slog.Logger, withACL bool) *agent.Service {
	t.Helper()
	owned, err := ownertable.Open(dir, owner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	scope, _ := subsystems.ResolveIDScope() // VRX_VPP_TABLE_BASE the tests set; empty (fail-closed) without it
	w, err := subsystems.Register(reg, subsystems.Env{Client: c, Owner: owner, StateDir: dir, Owned: owned, Log: log, IDs: scope})
	if err != nil {
		t.Fatal(err)
	}
	// F-acl's registration in the product (test harness only): only while the name is free — once F-acl registers
	// acl.acl in subsystems.Register, a second registration would panic (review M2)
	if _, registered := reg.Get(acl.NameACL); withACL && !registered {
		reg.Register(acl.NewACL(c, owner))
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg, log.With("component", "scheduler"))
	svc, err := agent.NewService(agent.ServiceConfig{Owner: owner, Version: "test", Logger: log, VPP: c, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return svc
}

func parseDoc(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatalf("doc: %v", err)
	}
	return ds
}

// rpfDocs returns the test document and its canonical Retrieve form for owner prefix p (slot n,
// tables base+1/base+2, loopbacks loop<n>01/loop<n>02).
func rpfDocs(t *testing.T, n int, base uint32, l1, l2 string) (desired, canonical *vrxv1.DesiredState) {
	t.Helper()
	js := fmt.Sprintf(`{
	  "vrfs": {"red": {"id": %[2]d}, "allow": {"id": %[3]d}},
	  "interfaces": {
	    %[4]q: {"vrf": "red", "ipv4": ["10.%[1]d.1.1/24"], "ipv6": ["2001:db8:%[1]d::1/64"],
	            "urpf": {"ipv4": "strict", "ipv6": "loose", "direction": "rx"},
	            "adl": {"ipv4": true, "ipv6": false, "allowVrf": "allow", "defaultAllow": true}},
	    %[5]q: {"ipv4": ["10.%[1]d.2.1/24"], "urpf": {"ipv4": "loose", "direction": "tx"}}
	  },
	  "routing": {"pbr": {
	    "policies": {
	      "via-l2": {"acl": "lan-b", "priority": 10, "paths": [{"address": "10.%[1]d.2.254", "interface": %[5]q, "vrf": "default", "weight": 1}]},
	      "lookup-red": {"acl": "lan-b", "priority": 20, "paths": [{"vrf": "red", "weight": 1}]}
	    },
	    "attachments": [
	      {"policy": "lookup-red", "interface": %[5]q, "family": "ipv4"},
	      {"policy": "via-l2", "interface": %[4]q, "family": "ipv4"}
	    ]
	  }}
	}`, n, base+1, base+2, l1, l2)
	canon := fmt.Sprintf(`{
	  "vrfs": {"red": {"id": %[2]d}, "allow": {"id": %[3]d}},
	  "interfaces": {
	    %[4]q: {"enabled": false, "promiscuous": false, "vrf": "red", "ipv4": ["10.%[1]d.1.1/24"], "ipv6": ["2001:db8:%[1]d::1/64"],
	            "urpf": {"ipv4": "strict", "ipv6": "loose", "direction": "rx"}, "adl": {}},
	    %[5]q: {"enabled": false, "promiscuous": false, "vrf": "default", "ipv4": ["10.%[1]d.2.1/24"], "urpf": {"ipv4": "loose", "direction": "tx"}}
	  },
	  "routing": {"pbr": {
	    "policies": {
	      "via-l2": {"acl": "lan-b", "priority": 10, "paths": [{"address": "10.%[1]d.2.254", "interface": %[5]q, "vrf": "default", "weight": 1}]},
	      "lookup-red": {"acl": "lan-b", "priority": 20, "paths": [{"vrf": "red", "weight": 1}]}
	    },
	    "attachments": [
	      {"policy": "lookup-red", "interface": %[5]q, "family": "ipv4"},
	      {"policy": "via-l2", "interface": %[4]q, "family": "ipv4"}
	    ]
	  }}
	}`, n, base+1, base+2, l1, l2)
	return parseDoc(t, js), parseDoc(t, canon)
}

// retrieveDomains Retrieves the three domains the test document uses.
func retrieveDomains(t *testing.T, svc *agent.Service) *vrxv1.DesiredState {
	t.Helper()
	got, err := svc.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

func mustApplied(t *testing.T, resp *vrxv1.ApplyResponse, err error) {
	t.Helper()
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %s", err, protojson.Format(resp))
	}
}

// fakeIdentity makes dfkit's boot identity (applied-once records) follow *pid (the coretest model's
// control_ping carries no real VPP PID); restored in t.Cleanup.
func fakeIdentity(t *testing.T, pid *int) {
	prev := dfkit.IdentitySource
	dfkit.IdentitySource = func(context.Context, vpp.Client) (bootid.Identity, error) {
		return bootid.Identity{BootID: "fake", PID: *pid, StartTime: 1}, nil
	}
	t.Cleanup(func() { dfkit.IdentitySource = prev })
}

func TestRpfAdlPbrOnFake(t *testing.T) {
	t.Setenv("VRX_VPP_TABLE_BASE", "3000")
	pid := 1000
	fakeIdentity(t, &pid)
	var logs syncBuffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	v := coretest.New()
	v.AddACL("w3:lan-b")
	v.AddACL("w9:lan-b") // another owner's ACL of the same name: never used
	dir := t.TempDir()
	svc := rpfService(t, v, "w3", dir, log)
	desired, canonical := rpfDocs(t, 3, 3000, "loop301", "loop302")
	ctx := context.Background()

	resp, err := svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: desired})
	mustApplied(t, resp, err)
	urpfs, adls, policies, attach := v.RpfAdlPbrState()
	if len(urpfs) != 3 || len(adls) != 1 || len(policies) != 2 || len(attach) != 2 {
		t.Fatalf("VPP model after apply: urpf %v adl %v policies %v attach %v", urpfs, adls, policies, attach)
	}
	for id := range policies {
		if id < 3000 || id > 3999 {
			t.Fatalf("policy id %d outside the slot range", id)
		}
	}
	for k, prio := range attach {
		if prio != 10 && prio != 20 {
			t.Fatalf("attachment %+v priority %d", k, prio)
		}
	}
	if got := retrieveDomains(t, svc); !proto.Equal(got, canonical) {
		t.Fatalf("Retrieve:\n got %s\nwant %s", protojson.Format(got), protojson.Format(canonical))
	}
	// idempotent: nothing sent but reads; the write-only allow-list is not re-added
	calls := len(v.CallsNamed("adl_allowlist_enable_disable"))
	resp, err = svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "t2", DesiredState: desired})
	mustApplied(t, resp, err)
	if len(resp.GetResults()) != 0 || len(v.CallsNamed("adl_allowlist_enable_disable")) != calls {
		t.Fatalf("second apply changed something: %s", protojson.Format(resp))
	}
	// DryRun: the write-only ADL leaves are marked, nothing else is flagged
	rep, err := svc.DryRun(ctx, &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: desired, Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil || !rep.GetOk() {
		t.Fatalf("dry run: %v %s", err, protojson.Format(rep))
	}
	var writeOnly []string
	for _, is := range rep.GetErrors() {
		if is.GetRule() == "agent.write-only" {
			writeOnly = append(writeOnly, is.GetPointer())
		}
	}
	if strings.Join(writeOnly, " ") != "/interfaces/loop301/adl/allowVrf /interfaces/loop301/adl/defaultAllow /interfaces/loop301/adl/ipv4 /interfaces/loop301/adl/ipv6" {
		t.Fatalf("write-only notes = %v", writeOnly)
	}

	// agent restart after loss: uRPF, ADL and ABF deleted behind the agent's back
	svc.Close()
	v.DeleteRpfAdlPbr()
	calls = len(v.CallsNamed("adl_allowlist_enable_disable"))
	svc = rpfService(t, v, "w3", dir, log)
	resp = svc.Resync(ctx)
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || resp.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync: %s", protojson.Format(resp))
	}
	if got := retrieveDomains(t, svc); !proto.Equal(got, canonical) {
		t.Fatalf("Retrieve after restart:\n got %s\nwant %s", protojson.Format(got), protojson.Format(canonical))
	}
	if n := len(v.CallsNamed("adl_allowlist_enable_disable")) - calls; n != 0 {
		t.Fatalf("agent restart re-sent the allow-list (%d calls): VPP still has it (same instance)", n)
	}
	// VPP restart (new identity): the write-only allow-list is applied exactly once more, and a
	// second resync adds nothing
	pid++
	calls = len(v.CallsNamed("adl_allowlist_enable_disable"))
	mustApplied(t, svc.Resync(ctx), nil)
	mustApplied(t, svc.Resync(ctx), nil)
	if n := len(v.CallsNamed("adl_allowlist_enable_disable")) - calls; n != 2 {
		t.Fatalf("after a VPP restart the allow-list add sequence ran %d calls, want exactly one sequence (2)", n)
	}

	// validation failures carry pointers and change nothing
	bad := proto.Clone(desired).(*vrxv1.DesiredState)
	bad.Interfaces["loop301"].Adl.DefaultAllow = proto.Bool(false)
	bad.Routing.Pbr.Attachments = append(bad.Routing.Pbr.Attachments, &vrxv1.PbrAttachment{Policy: proto.String("nope"), Interface: proto.String("loop301"), Family: proto.String("ipv4")})
	resp, err = svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "bad", DesiredState: bad})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_FAILED {
		t.Fatalf("invalid apply: %v %s", err, protojson.Format(resp))
	}
	var ptrs []string
	for _, is := range resp.GetValidation().GetErrors() {
		if is.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			ptrs = append(ptrs, is.GetPointer()+" "+is.GetRule())
		}
	}
	if strings.Join(ptrs, "; ") != "/interfaces/loop301/adl/defaultAllow interfaces.rpf-adl-pbr-adl-non-ip; /routing/pbr/attachments/2/policy routing.rpf-adl-pbr-attachment" {
		t.Fatalf("validation errors = %v", ptrs)
	}

	// rollback to a document without the feature: attachments go before policies, uRPF/ADL cleared
	plain := proto.Clone(desired).(*vrxv1.DesiredState)
	for _, itf := range plain.GetInterfaces() {
		itf.Urpf, itf.Adl = nil, nil
	}
	plain.Routing = &vrxv1.RoutingConfig{}
	calls = len(v.CallsNamed("adl_allowlist_enable_disable"))
	resp, err = svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "t3", DesiredState: plain})
	mustApplied(t, resp, err)
	urpfs, adls, policies, attach = v.RpfAdlPbrState()
	if len(urpfs)+len(adls)+len(policies)+len(attach) != 0 {
		t.Fatalf("after rollback: urpf %v adl %v policies %v attach %v", urpfs, adls, policies, attach)
	}
	if n := len(v.CallsNamed("adl_allowlist_enable_disable")) - calls; n != 2 {
		t.Fatalf("allow-list remove sequence: %d calls, want 2", n)
	}
	lastAttach, firstPolicy, adlOff, allowOff := -1, 1<<30, -1, -1
	for i, r := range resp.GetResults() {
		switch d := strings.SplitN(r.GetKey(), "/", 2)[0]; d {
		case "abf.attach":
			lastAttach = max(lastAttach, i)
		case "abf.policy":
			firstPolicy = min(firstPolicy, i)
		case "adl.interface":
			adlOff = i
		case "adl.allowlist":
			allowOff = i
		}
	}
	if lastAttach < 0 || lastAttach > firstPolicy || adlOff < 0 || adlOff > allowOff {
		t.Fatalf("delete order (attachments before policies, adl-input before the allow-list): %s", protojson.Format(resp))
	}
	if got := retrieveDomains(t, svc); got.GetRouting().GetPbr() != nil || got.GetInterfaces()["loop301"].GetUrpf() != nil || got.GetInterfaces()["loop301"].GetAdl() != nil {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if !strings.Contains(logs.String(), "adl.allowlist/loop301") {
		t.Fatalf("scheduler log does not mention the allow-list:\n%s", logs.String())
	}
}

// TestRpfAdlPbrPolicyNamesPersist: the pbr.policy name records survive an agent restart, so Retrieve
// names the policies VPP holds; a leftover policy of this owner without a record shows as "#<id>".
func TestRpfAdlPbrPolicyNamesPersist(t *testing.T) {
	t.Setenv("VRX_VPP_TABLE_BASE", "3000")
	pid := 1
	fakeIdentity(t, &pid)
	log := slog.New(slog.DiscardHandler)
	v := coretest.New()
	v.AddACL("w3:lan-b")
	dir := t.TempDir()
	svc := rpfService(t, v, "w3", dir, log)
	desired, canonical := rpfDocs(t, 3, 3000, "loop301", "loop302")
	resp, err := svc.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: "t1", DesiredState: desired})
	mustApplied(t, resp, err)
	svc.Close()
	svc = rpfService(t, v, "w3", dir, log)
	// the projection gets the recorded ids back (sticky policy ids, review L2)
	if rec := subsystems.RpfAdlPbrEnv().RecordedIDs; len(rec) != 2 || rec["via-l2"] < 3000 || rec["lookup-red"] > 3999 {
		t.Fatalf("recorded ids after restart = %v", rec)
	}
	if got := retrieveDomains(t, svc); !proto.Equal(got.GetRouting(), canonical.GetRouting()) {
		t.Fatalf("names after restart: %s", protojson.Format(got.GetRouting()))
	}
	// a fresh state dir (records lost): the owned policies are reported by id
	fresh := rpfService(t, v, "w3", t.TempDir(), log)
	names := []string{}
	for n := range retrieveDomains(t, fresh).GetRouting().GetPbr().GetPolicies() {
		names = append(names, n)
	}
	if len(names) != 2 || !strings.HasPrefix(names[0], "#") || !strings.HasPrefix(names[1], "#") {
		t.Fatalf("policies without records = %v", names)
	}
}

// TestRpfAdlPbrWithoutFAcl: the product registry of this build has no acl.acl descriptor (F-acl adds it);
// the observe-only pbr.acl-ref bridge resolves a policy's ACL from the owner-tagged ACLs in VPP, and an ACL
// VPP does not have is a dependency error with the policy's pointer.
func TestRpfAdlPbrWithoutFAcl(t *testing.T) {
	t.Setenv("VRX_VPP_TABLE_BASE", "3000")
	pid := 1
	fakeIdentity(t, &pid)
	log := slog.New(slog.DiscardHandler)
	v := coretest.New()
	svc := rpfServiceWith(t, v, "w3", t.TempDir(), log, false)
	desired, canonical := rpfDocs(t, 3, 3000, "loop301", "loop302")
	ctx := context.Background()
	resp, err := svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "no-acl", DesiredState: desired})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_FAILED {
		t.Fatalf("apply without the ACL in VPP: %v %s", err, protojson.Format(resp))
	}
	var missing []string
	for _, is := range resp.GetValidation().GetErrors() {
		if is.GetRule() == "agent.dependency-missing" {
			missing = append(missing, is.GetPointer())
		}
	}
	if strings.Join(missing, " ") != "/routing/pbr/policies/lookup-red /routing/pbr/policies/via-l2" {
		t.Fatalf("dependency errors = %v (%s)", missing, protojson.Format(resp))
	}
	v.AddACL("w3:lan-b")
	resp, err = svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: "acl", DesiredState: desired})
	mustApplied(t, resp, err)
	if got := retrieveDomains(t, svc); !proto.Equal(got, canonical) {
		t.Fatalf("Retrieve:\n got %s\nwant %s", protojson.Format(got), protojson.Format(canonical))
	}
}

// TestACLBridgeRegistration: the pbr.acl-ref bridge and DF-4's acl.acl in either registration order (review M2).
// acl.acl first (F-acl's registration ran before this task's): the bridge is not registered and nothing panics.
// acl.acl after: the bridge is registered but inert — no aliases, no Retrieve result, no acl_dump sent.
func TestACLBridgeRegistration(t *testing.T) {
	t.Setenv("VRX_VPP_TABLE_BASE", "3000")
	log := slog.New(slog.DiscardHandler)
	env := func(t *testing.T, v *coretest.VPP) subsystems.Env {
		dir := t.TempDir()
		owned, err := ownertable.Open(dir, "w3")
		if err != nil {
			t.Fatal(err)
		}
		scope, _ := subsystems.ResolveIDScope()
		return subsystems.Env{Client: v, Owner: "w3", StateDir: dir, Owned: owned, Log: log, IDs: scope}
	}
	ctx := context.Background()

	// F-acl is in the product now: subsystems.Register wires acl.acl (registerACL), so the pbr.acl-ref bridge —
	// a shim that resolves ABF's acl.acl/<name> dependency only while no acl.acl descriptor is registered — is
	// registered but inert (its active() sees acl.acl and yields nothing). It self-activates only in a build
	// without F-acl (unit-tested in aclRefs' own package-internal tests).
	v := coretest.New()
	v.AddACL("w3:lan-b")
	reg := scheduler.NewRegistry()
	if _, err := subsystems.Register(reg, env(t, v)); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("acl.acl"); !ok {
		t.Fatal("acl.acl not registered by the product (F-acl)")
	}
	d, ok := reg.Get("pbr.acl-ref")
	if !ok {
		t.Fatal("bridge not registered")
	}
	dumps := len(v.CallsNamed("acl_dump"))
	kvs, err := d.Retrieve(ctx)
	if err != nil || kvs != nil {
		t.Fatalf("inert bridge Retrieve = %v, %v (acl.acl is registered)", kvs, err)
	}
	if n := len(v.CallsNamed("acl_dump")) - dumps; n != 0 {
		t.Fatalf("inert bridge sent %d acl_dump", n)
	}
}
