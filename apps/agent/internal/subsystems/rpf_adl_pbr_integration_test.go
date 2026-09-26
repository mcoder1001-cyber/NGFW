package subsystems_test

// Host integration test of F-rpf-adl-pbr (VRX_INTEGRATION=1, shared lab lock, slot prefix): the agent
// service with the product registry against the VPP on this host — loopbacks loop<slot>01/02 and VRFs in
// the slot's table range, created by the agent through the same document; an ACL "<prefix>:lan-b" created
// through binapi (F-acl creates ACLs in the product). No packets are sent; no af_packet interface is used.
//
//	apply → Retrieve == desired (readable types) → idempotent re-apply sends no write
//	restart simulation: agent down, uRPF/ADL/ABF deleted via binapi (attachments first), agent up →
//	  converged; the write-only allow-list is re-applied by the reconciler and skipped by its applied-once
//	  record (same VPP instance: VPP still has it)
//	rollback to the document without the feature → nothing of it left (Retrieve + dumps)
//	V23 (a): a fresh loopback never reads as "ADL on", whatever the raw feature_is_enabled says
//
// VRX_RPF_VPPCTL=1 logs `vppctl show abf policy` / `show abf attach` / `show interface features` at
// each stage (evidence; read-only CLI, test code only).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	abfapi "ngfw/agent/binapi/abf"
	adlapi "ngfw/agent/binapi/adl"
	featureapi "ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	urpfapi "ngfw/agent/binapi/urpf"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/adl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// countingClient counts the request/reply messages sent through it (per message name).
type countingClient struct {
	vpp.Client
	mu sync.Mutex
	n  map[string]int
}

func (c *countingClient) Invoke(ctx context.Context, req, reply api.Message) error {
	c.mu.Lock()
	c.n[req.GetMessageName()]++
	c.mu.Unlock()
	return c.Client.Invoke(ctx, req, reply)
}

func (c *countingClient) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[name]
}

func vppctl(t *testing.T, args ...string) {
	t.Helper()
	if os.Getenv("VRX_RPF_VPPCTL") != "1" {
		return
	}
	out, err := exec.Command("vppctl", args...).CombinedOutput() //nolint:gosec // fixed evidence commands, test only
	t.Logf("$ vppctl %s\n%s(err=%v)", strings.Join(args, " "), out, err)
}

func evidence(t *testing.T, stage string, ifs ...string) {
	t.Helper()
	t.Logf("---- vppctl evidence: %s ----", stage)
	vppctl(t, "show", "abf", "policy")
	for _, i := range ifs {
		vppctl(t, "show", "abf", "attach", i)
		vppctl(t, "show", "interface", "features", i)
	}
}

// ownedIndex returns the sw_if_index of the owner's interface name.
func ownedIndex(t *testing.T, c vpp.Client, owner, name string) uint32 {
	t.Helper()
	ifs, err := df2.DumpInterfaces(context.Background(), c, owner)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := ifs.Index(name)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(idx)
}

// deleteFeatureBehindBack removes the owner's uRPF checks, adl-input and ABF objects via binapi
// (attachments before policies, D-095 c) and returns how many objects it removed. The write-only
// allow-list binding is left alone: nothing can see it (the agent's record says VPP still has it).
func deleteFeatureBehindBack(t *testing.T, c vpp.Client, lo, hi uint32, idxs ...uint32) int {
	t.Helper()
	ctx := context.Background()
	mine := map[uint32]bool{}
	for _, i := range idxs {
		mine[i] = true
	}
	n := 0
	abf := abfapi.NewServiceClient(c)
	st, err := abf.AbfItfAttachDump(ctx, &abfapi.AbfItfAttachDump{})
	if err != nil {
		t.Fatal(err)
	}
	var attaches []abfapi.AbfItfAttach
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if d.Attach.PolicyID >= lo && d.Attach.PolicyID <= hi && mine[uint32(d.Attach.SwIfIndex)] {
			attaches = append(attaches, d.Attach)
		}
	}
	for _, a := range attaches {
		if _, err := abf.AbfItfAttachAddDel(ctx, &abfapi.AbfItfAttachAddDel{IsAdd: false, Attach: a}); err != nil {
			t.Fatalf("abf_itf_attach_add_del: %v", err)
		}
		n++
	}
	ps, err := abf.AbfPolicyDump(ctx, &abfapi.AbfPolicyDump{})
	if err != nil {
		t.Fatal(err)
	}
	var policies []abfapi.AbfPolicy
	for {
		d, err := ps.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if d.Policy.PolicyID >= lo && d.Policy.PolicyID <= hi {
			policies = append(policies, d.Policy)
		}
	}
	for _, p := range policies {
		if _, err := abf.AbfPolicyAddDel(ctx, &abfapi.AbfPolicyAddDel{IsAdd: false, Policy: p}); err != nil {
			t.Fatalf("abf_policy_add_del: %v", err)
		}
		n++
	}
	us, err := urpfapi.NewServiceClient(c).UrpfInterfaceDump(ctx, &urpfapi.UrpfInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		t.Fatal(err)
	}
	var checks []*urpfapi.UrpfInterfaceDetails
	for {
		d, err := us.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if mine[uint32(d.SwIfIndex)] && d.Mode != urpfapi.URPF_API_MODE_OFF {
			checks = append(checks, d)
		}
	}
	for _, d := range checks {
		if _, err := urpfapi.NewServiceClient(c).UrpfUpdateV2(ctx, &urpfapi.UrpfUpdateV2{IsInput: d.IsInput, Mode: urpfapi.URPF_API_MODE_OFF, Af: d.Af, SwIfIndex: d.SwIfIndex, TableID: d.TableID}); err != nil {
			t.Fatalf("urpf_update_v2 off: %v", err)
		}
		n++
	}
	for _, i := range idxs {
		rep, err := featureapi.NewServiceClient(c).FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: "device-input", FeatureName: "adl-input", SwIfIndex: interface_types.InterfaceIndex(i)})
		ctl, cerr := featureapi.NewServiceClient(c).FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: "device-input", FeatureName: "ethernet-input", SwIfIndex: interface_types.InterfaceIndex(i)})
		if err == nil && cerr == nil && rep.IsEnabled && !ctl.IsEnabled {
			if _, err := adlapi.NewServiceClient(c).AdlInterfaceEnableDisable(ctx, &adlapi.AdlInterfaceEnableDisable{SwIfIndex: interface_types.InterfaceIndex(i), EnableDisable: false}); err != nil {
				t.Fatalf("adl_interface_enable_disable off: %v", err)
			}
			n++
		}
	}
	return n
}

func TestRpfAdlPbrOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner, slot, base := vpptest.Prefix(t), vpptest.Slot(t), vpptest.TableBase(t)
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	cc := &countingClient{Client: raw, n: map[string]int{}}
	df2test.ACL(t, raw, "lan-b") // deleted last (t.Cleanup is LIFO): the policies reference it

	var logs syncBuffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	dir := t.TempDir()
	l1 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 1))
	l2 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 2))
	desired, canonical := rpfDocs(t, slot, base, l1, l2)
	ctx := context.Background()
	domains := []string{"interfaces", "vrfs", "routing"}
	svc := rpfService(t, cc, owner, dir, log)
	t.Cleanup(func() {
		// everything goes: feature objects first (the scheduler orders them), then interfaces and VRFs
		s := rpfService(t, cc, owner, dir, log)
		resp, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: owner + "-rpf-cleanup", Subsystems: domains, DesiredState: &vrxv1.DesiredState{}})
		if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Errorf("cleanup apply: %v %s", err, protojson.Format(resp))
		}
	})

	// 1. apply → Retrieve == desired
	start := time.Now()
	resp, err := svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-rpf-1", Subsystems: domains, DesiredState: desired})
	mustApplied(t, resp, err)
	t.Logf("apply: %s in %v", protojson.Format(resp.GetSummary()), time.Since(start))
	for _, r := range resp.GetResults() {
		t.Logf("  %s %s %s", r.GetOp(), r.GetKey(), r.GetCode())
	}
	got := retrieveDomains(t, svc)
	if !proto.Equal(got, canonical) {
		t.Fatalf("Retrieve:\n got %s\nwant %s", protojson.Format(got), protojson.Format(canonical))
	}
	t.Logf("Retrieve == desired (canonical): routing.pbr=%s", protojson.Format(got.GetRouting().GetPbr()))
	t.Logf("Retrieve interfaces.%s.urpf=%s adl=%s (allow-list binding write-only)", l1, protojson.Format(got.GetInterfaces()[l1].GetUrpf()), protojson.Format(got.GetInterfaces()[l1].GetAdl()))
	evidence(t, "after commit", l1, l2)
	df2test.Hold(t)

	// 2. idempotent: nothing but reads
	before := cc.count("adl_allowlist_enable_disable")
	resp, err = svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-rpf-2", Subsystems: domains, DesiredState: desired})
	mustApplied(t, resp, err)
	if len(resp.GetResults()) != 0 || cc.count("adl_allowlist_enable_disable") != before {
		t.Fatalf("idempotent apply changed something: %s", protojson.Format(resp))
	}
	t.Logf("idempotent apply: %s", protojson.Format(resp.GetSummary()))

	// 3. restart simulation: agent down, our objects deleted behind its back, agent up
	svc.Close()
	i1, i2 := ownedIndex(t, raw, owner, l1), ownedIndex(t, raw, owner, l2)
	n := deleteFeatureBehindBack(t, raw, base, base+999, i1, i2)
	t.Logf("simulated loss: %d objects deleted via binapi (ABF attachments, policies, uRPF checks, adl-input)", n)
	evidence(t, "after the simulated loss", l1)
	before = cc.count("adl_allowlist_enable_disable")
	logs.Reset()
	start = time.Now()
	svc = rpfService(t, cc, owner, dir, log)
	resp = svc.Resync(ctx)
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("resync: %s", protojson.Format(resp))
	}
	got = retrieveDomains(t, svc)
	took := time.Since(start)
	if !proto.Equal(got, canonical) {
		t.Fatalf("Retrieve after restart:\n got %s\nwant %s", protojson.Format(got), protojson.Format(canonical))
	}
	sent := cc.count("adl_allowlist_enable_disable") - before
	t.Logf("restart: converged in %v (%s); allow-list re-applied by the reconciler, adl_allowlist_enable_disable calls sent: %d (applied-once record, same VPP instance)", took, protojson.Format(resp.GetSummary()), sent)
	if took > 30*time.Second || sent != 0 {
		t.Fatalf("restart: %v, %d allow-list calls", took, sent)
	}
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "adl.allowlist") || strings.Contains(line, "created") || strings.Contains(line, "reconcile") {
			t.Logf("agent log: %s", line)
		}
	}
	evidence(t, "after the restart", l1, l2)

	// 4. rollback to the document without the feature
	plain := proto.Clone(desired).(*vrxv1.DesiredState)
	for _, itf := range plain.GetInterfaces() {
		itf.Urpf, itf.Adl = nil, nil
	}
	plain.Routing = &vrxv1.RoutingConfig{}
	before = cc.count("adl_allowlist_enable_disable")
	resp, err = svc.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-rpf-3", Subsystems: domains, DesiredState: plain})
	mustApplied(t, resp, err)
	for _, r := range resp.GetResults() {
		t.Logf("  rollback %s %s %s", r.GetOp(), r.GetKey(), r.GetCode())
	}
	got = retrieveDomains(t, svc)
	if got.GetRouting().GetPbr() != nil || got.GetInterfaces()[l1].GetUrpf() != nil || got.GetInterfaces()[l1].GetAdl() != nil || got.GetInterfaces()[l2].GetUrpf() != nil {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if left := deleteFeatureBehindBack(t, raw, base, base+999, i1, i2); left != 0 {
		t.Fatalf("%d objects left in VPP after the rollback", left)
	}
	t.Logf("rollback: Retrieve has no urpf/adl/pbr, the binapi dumps find nothing of the slot; allow-list remove sequence sent %d calls", cc.count("adl_allowlist_enable_disable")-before)
	evidence(t, "after the rollback", l1, l2)
}

// TestADLRetrieveV23OnHost: a fresh loopback of this owner never reads as ADL-on through
// adl.interface Retrieve, whatever the raw feature_is_enabled answers (V23 a).
func TestADLRetrieveV23OnHost(t *testing.T) {
	c := df2test.Connect(t)
	owner := vpptest.Prefix(t)
	name, idx := df2test.Loopback(t, c, 3) // created without the agent (no sanitizer run on it)
	ctx := df2test.Ctx(t)
	f := featureapi.NewServiceClient(c)
	adlRaw, err := f.FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: "device-input", FeatureName: "adl-input", SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatal(err)
	}
	ctl, err := f.FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: "device-input", FeatureName: "ethernet-input", SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatal(err)
	}
	bogus, err := f.FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: "device-input", FeatureName: "vrx-no-such-feature", SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fresh %s (sw_if_index %d): raw feature_is_enabled adl-input=%v, control ethernet-input=%v, unknown feature=%v", name, idx, adlRaw.IsEnabled, ctl.IsEnabled, bogus.IsEnabled)
	if !bogus.IsEnabled {
		t.Logf("note: this VPP answers false for an unknown feature (V23 a may be fixed upstream)")
	}
	d := adl.NewInterface(c, owner)
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range kvs {
		if kv.Key == d.KeyOf(&adl.Interface{Interface: name}) {
			t.Fatalf("fresh %s retrieved as ADL on (raw %v, control %v)", name, adlRaw.IsEnabled, ctl.IsEnabled)
		}
	}
	// enable for real: now it is reported, and gone again after Delete
	meta, err := d.Create(ctx, &adl.Interface{Interface: name})
	if err != nil {
		t.Fatal(err)
	}
	kvs, err = d.Retrieve(ctx)
	if err != nil || len(kvs) == 0 {
		t.Fatalf("enabled ADL not retrieved: %v %v", kvs, err)
	}
	if err := d.Delete(ctx, &adl.Interface{Interface: name}, meta); err != nil {
		t.Fatal(err)
	}
	if kvs, err = d.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("ADL retrieved after Delete: %v %v", kvs, err)
	}
	t.Logf("adl.interface Retrieve: fresh → absent, enabled → present, disabled → absent")
}

func vppSocket() string {
	if p := os.Getenv("VRX_VPP_API_SOCKET"); p != "" {
		return p
	}
	return "/run/vpp/api.sock"
}
