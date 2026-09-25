package scheduler_test

// TD-13 (D-125 ARCH-02): the tier-3 Validator and the VPP→daemon stage, exercised with an example
// adopter — fakeDaemon, a daemon configuration descriptor that declares StageDaemon and implements
// Validator (its "checker" is a Go func, it shells nothing) — next to the worked-example loopback
// descriptor over the fake VPP (example_descriptor_test.go).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

const daemonName = "daemon.fake"

// daemonConf is a daemon object: its name, the loopback it listens on ("" = none; an optional
// dependency, as Kea's interfaces are) and free extra fields (secret leaves in the redaction test).
func daemonConf(name, listen string, extra map[string]any) *structpb.Struct {
	m := map[string]any{"name": name, "listen": listen}
	for k, v := range extra {
		m[k] = v
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		panic(err)
	}
	return s
}

// fakeDaemon is the example adopter. Validate runs check (the stand-in for `kea-dhcp4 -t` on a
// staged copy) and records when it ran; Create and Update check again before they "write" (defence
// in depth — what the renderers do today, D-109 d) and record the write.
type fakeDaemon struct {
	vpp *fakeVPP

	mu     sync.Mutex
	conf   map[scheduler.Key]*structpb.Struct // the daemon's running configuration
	events []string                           // "validate <key> @<VPP writes so far>", "create <key> @<n>", …
	bad    error                              // the configuration the daemon refuses (Validate and Create)
	check  func(ctx context.Context, key scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error
}

func newFakeDaemon(v *fakeVPP) *fakeDaemon {
	return &fakeDaemon{vpp: v, conf: map[scheduler.Key]*structpb.Struct{}}
}

func (*fakeDaemon) Name() string { return daemonName }
func (*fakeDaemon) KeyOf(obj proto.Message) scheduler.Key {
	return scheduler.Join(daemonName, field(obj, "name").GetStringValue())
}
func (*fakeDaemon) Dependencies(obj proto.Message) []scheduler.Dependency {
	if l := field(obj, "listen").GetStringValue(); l != "" {
		return []scheduler.Dependency{{Key: scheduler.Join(loopbackName, l), Optional: true}}
	}
	return nil
}
func (*fakeDaemon) Stage() scheduler.Stage { return scheduler.StageDaemon }

func (d *fakeDaemon) record(what string, k scheduler.Key) {
	d.events = append(d.events, fmt.Sprintf("%s %s @%d", what, k, len(vppWrites(d.vpp))))
}

func (d *fakeDaemon) Validate(ctx context.Context, key scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error {
	d.mu.Lock()
	d.record("validate", key)
	bad, check := d.bad, d.check
	d.mu.Unlock()
	if bad != nil {
		return bad
	}
	if check != nil {
		return check(ctx, key, value, view)
	}
	return nil
}

func (d *fakeDaemon) write(what string, obj proto.Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	k := d.KeyOf(obj)
	if d.bad != nil {
		return d.bad // refused before anything is written
	}
	d.record(what, k)
	d.conf[k] = proto.Clone(obj).(*structpb.Struct)
	return nil
}

func (d *fakeDaemon) Create(_ context.Context, obj proto.Message) (any, error) {
	return nil, d.write("create", obj)
}
func (d *fakeDaemon) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.write("update", newObj)
}
func (d *fakeDaemon) Delete(_ context.Context, obj proto.Message, _ any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	k := d.KeyOf(obj)
	d.record("delete", k)
	delete(d.conf, k)
	return nil
}
func (d *fakeDaemon) Retrieve(context.Context) ([]scheduler.KV, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []scheduler.KV
	for k, v := range d.conf {
		out = append(out, scheduler.KV{Key: k, Value: proto.Clone(v)})
	}
	return out, nil
}

func (d *fakeDaemon) log() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.events...)
}

func (d *fakeDaemon) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = nil
}

func (d *fakeDaemon) kv(name, listen string, extra map[string]any) scheduler.KV {
	v := daemonConf(name, listen, extra)
	return scheduler.KV{Key: d.KeyOf(v), Value: v}
}

func loopKV(name string) scheduler.KV {
	v := loopback(name, 9000)
	return scheduler.KV{Key: scheduler.Join(loopbackName, name), Value: v}
}

// vppWrites is every request the fake VPP received except the reads a plan makes (Retrieve's dump).
func vppWrites(v *fakeVPP) []string {
	var out []string
	for _, m := range v.Calls() {
		if n := m.GetMessageName(); n != "sw_interface_dump" && n != "control_ping" {
			out = append(out, n)
		}
	}
	return out
}

// td13Fixture: the daemon descriptor is registered FIRST, so only the stage puts its objects after
// the loopbacks. Owner "w2" as in the worked example.
func td13Fixture(t *testing.T) (*scheduler.Scheduler, *fakeVPP, *fakeDaemon) {
	t.Helper()
	v := newFakeVPP()
	d := newFakeDaemon(v)
	reg := scheduler.NewRegistry()
	reg.Register(d)
	reg.Register(loopbackDescriptor{client: v, owner: "w2"})
	s := scheduler.New(reg, nil)
	s.VerifyRetries = 0
	return s, v, d
}

func mustNoVPPWrite(t *testing.T, v *fakeVPP) {
	t.Helper()
	if w := vppWrites(v); len(w) != 0 {
		t.Fatalf("VPP was written before (or despite) the validator's verdict: %v", w)
	}
	for _, m := range v.Calls() {
		if n := m.GetMessageName(); n != "sw_interface_dump" && n != "control_ping" {
			t.Fatalf("unexpected VPP call %s", n)
		}
	}
}

func onlyIssue(t *testing.T, p *scheduler.TxnPlan) scheduler.Issue {
	t.Helper()
	if p == nil || len(p.Issues) != 1 {
		t.Fatalf("want exactly one issue, plan %+v", p)
	}
	return p.Issues[0]
}

// A validator that rejects the daemon's configuration fails the transaction before ANY write: the
// fake VPP receives no write call at all (only the Retrieve dump every plan makes), the daemon is
// not written, and the answer is FAILED (not ROLLED_BACK) with a finding that names the key and the
// pointer the validator gave.
func TestFailingValidatorLeavesVPPUntouched(t *testing.T) {
	s, v, d := td13Fixture(t)
	d.setBad(scheduler.InvalidAt("/services/dhcp/servers/lan/pools/0", errors.New("kea-dhcp4 -t: pool 10.0.0.300 is not an address")))
	before := len(v.ifaces)

	r := s.Apply(context.Background(), []scheduler.KV{loopKV("loop200"), d.kv("lan", "loop200", nil)}, nil)

	mustNoVPPWrite(t, v)
	if r.Outcome != scheduler.OutcomeFailed {
		t.Fatalf("outcome %s, want FAILED: err %v, results %+v", r.Outcome, r.Err, r.Results)
	}
	if len(v.ifaces) != before {
		t.Fatalf("VPP interfaces %d, want %d", len(v.ifaces), before)
	}
	if got := d.log(); len(got) != 1 || got[0] != "validate daemon.fake/lan @0" {
		t.Fatalf("daemon events %v, want only the validation", got)
	}
	is := onlyIssue(t, r.Plan)
	if is.Key != "daemon.fake/lan" || is.Code != scheduler.CodeInvalid || scheduler.IssueRule(is) != scheduler.RuleValidator ||
		scheduler.IssuePointer(is) != "/services/dhcp/servers/lan/pools/0" {
		t.Fatalf("issue %+v rule %q pointer %q", is, scheduler.IssueRule(is), scheduler.IssuePointer(is))
	}
	if !strings.Contains(is.String(), "daemon.fake/lan: validator: kea-dhcp4 -t: pool 10.0.0.300") {
		t.Fatalf("issue text %q", is.String())
	}
	if len(r.Results) != 1 || r.Results[0].Key != "daemon.fake/lan" || r.Results[0].Code != scheduler.CodeInvalid {
		t.Fatalf("results %+v", r.Results)
	}
	if r.Err == nil || !strings.Contains(r.Err.Error(), "validation failed: daemon.fake/lan") {
		t.Fatalf("err %v", r.Err)
	}
}

// Plan (the DryRun RPC) runs the validators too and reports the finding; nothing is written, and a
// plan with a finding lists no operation. A valid configuration plans in stage order.
func TestPlanRunsValidators(t *testing.T) {
	s, v, d := td13Fixture(t)
	ctx := context.Background()
	desired := []scheduler.KV{d.kv("dns", "", nil), loopKV("loop200")}
	// a pointer that is not an RFC 6901 pointer ("/…") is ignored: the finding keeps the object's own
	d.setBad(scheduler.InvalidAt("services/dns", errors.New("unbound-checkconf: syntax error")))
	p, err := s.Plan(ctx, desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	is := onlyIssue(t, p)
	if is.Key != "daemon.fake/dns" || scheduler.IssueRule(is) != scheduler.RuleValidator || scheduler.IssuePointer(is) != "" || len(p.Ops) != 0 {
		t.Fatalf("issue %+v, ops %+v", is, p.Ops)
	}
	d.setBad(nil)
	d.reset()
	if p, err = s.Plan(ctx, desired, nil); err != nil || len(p.Issues) != 0 {
		t.Fatalf("valid plan: %v %v", err, p.Issues)
	}
	var ops []string
	for _, op := range p.Ops {
		ops = append(ops, op.Op+" "+string(op.Key))
	}
	if strings.Join(ops, ",") != "create interface.loopback/loop200,create daemon.fake/dns" {
		t.Fatalf("plan order %v", ops)
	}
	if got := d.log(); !reflect.DeepEqual(got, []string{"validate daemon.fake/dns @0"}) {
		t.Fatalf("daemon events %v", got)
	}
	mustNoVPPWrite(t, v)
}

// On success the validator runs once per Create/Update, before the first VPP write; the daemon's
// create runs after the VPP objects (the stage), though its descriptor was registered first.
// Unchanged objects and deletes are not validated.
func TestValidatorRunsBeforeAnyWriteAndOnlyForCreateUpdate(t *testing.T) {
	s, _, d := td13Fixture(t)
	ctx := context.Background()
	desired := []scheduler.KV{d.kv("dns", "", nil), loopKV("loop200")}
	mustApply(t, s.Apply(ctx, desired, nil))
	// 3 VPP writes: create_loopback_instance, sw_interface_tag_add_del, sw_interface_set_mtu
	if got := d.log(); !reflect.DeepEqual(got, []string{"validate daemon.fake/dns @0", "create daemon.fake/dns @3"}) {
		t.Fatalf("daemon events %v", got)
	}

	d.reset()
	mustApply(t, s.Apply(ctx, desired, nil))
	if got := d.log(); len(got) != 0 {
		t.Fatalf("unchanged object validated or written: %v", got)
	}

	d.reset()
	mustApply(t, s.Apply(ctx, []scheduler.KV{d.kv("dns", "loop200", nil), loopKV("loop200")}, nil))
	if got := d.log(); !reflect.DeepEqual(got, []string{"validate daemon.fake/dns @3", "update daemon.fake/dns @3"}) {
		t.Fatalf("daemon events %v", got)
	}

	d.reset()
	mustApply(t, s.Apply(ctx, []scheduler.KV{loopKV("loop200")}, nil))
	if got := d.log(); !reflect.DeepEqual(got, []string{"delete daemon.fake/dns @3"}) {
		t.Fatalf("daemon events %v", got)
	}
}

func mustApply(t *testing.T, r *scheduler.TxnResult) {
	t.Helper()
	if r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("outcome %s, err %v, results %+v", r.Outcome, r.Err, r.Results)
	}
}

// The view is the state after the transaction (objects it deletes are gone, objects outside its
// scope are there), values are copies, and the value handed to Validate is a copy too: nothing a
// validator does changes what is applied.
func TestValidatorViewIsTheStateAfterAndReadOnly(t *testing.T) {
	s, _, d := td13Fixture(t)
	ctx := context.Background()
	mustApply(t, s.Apply(ctx, []scheduler.KV{loopKV("loop200"), d.kv("old", "", nil)}, nil))

	var listed []string
	var loopSeen bool
	d.setCheck(func(_ context.Context, key scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error {
		if key != "daemon.fake/a" {
			return nil
		}
		for _, kv := range view.List(daemonName) {
			listed = append(listed, string(kv.Key)+"="+field(kv.Value, "listen").GetStringValue())
			kv.Value.(*structpb.Struct).Fields["listen"] = structpb.NewStringValue("evil")
		}
		if lv, ok := view.Get(scheduler.Join(loopbackName, "loop200")); ok {
			loopSeen = true
			lv.(*structpb.Struct).Fields["mtu"] = structpb.NewNumberValue(1)
		}
		if _, ok := view.Get(scheduler.Join(daemonName, "old")); ok {
			return errors.New("the view shows an object the transaction deletes")
		}
		if bv, ok := view.Get(scheduler.Join(daemonName, "b")); ok {
			bv.(*structpb.Struct).Fields["listen"] = structpb.NewStringValue("evil")
		}
		value.(*structpb.Struct).Fields["listen"] = structpb.NewStringValue("evil")
		return nil
	})
	// Scope: only the daemon — the loopback is out of scope and stays; "old" is deleted.
	r := s.Apply(ctx, []scheduler.KV{d.kv("a", "loop200", nil), d.kv("b", "", nil)}, scheduler.Only(daemonName))
	mustApply(t, r)
	if want := []string{"daemon.fake/a=loop200", "daemon.fake/b="}; !reflect.DeepEqual(listed, want) {
		t.Fatalf("view.List %v, want %v", listed, want)
	}
	if !loopSeen {
		t.Fatal("the out-of-scope loopback is missing from the view")
	}
	d.mu.Lock()
	gotA := field(d.conf["daemon.fake/a"], "listen").GetStringValue()
	gotB := field(d.conf["daemon.fake/b"], "listen").GetStringValue()
	d.mu.Unlock()
	if gotA != "loop200" || gotB != "" {
		t.Fatalf("the daemon received listen=%q / %q: a validator changed what is applied", gotA, gotB)
	}
	actual, err := s.Retrieve(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range actual {
		if kv.Key == "interface.loopback/loop200" && field(kv.Value, "mtu").GetNumberValue() != 9000 {
			t.Fatalf("loopback changed through the view: %v", kv.Value)
		}
	}
}

// Secret leaves of the value never reach the finding: secret-named fields, D-051 references and
// everything under a secret-named key are masked as whole tokens; a huge checker output is cut.
func TestValidatorFindingRedactsSecretLeaves(t *testing.T) {
	s, v, d := td13Fixture(t)
	// *_ref fields (a well-formed and a malformed reference, one nested) and a D-051 reference in a
	// field of another name are masked; a BGP community and the name "psk0" are not secrets (D-040).
	extra := map[string]any{ //nolint:gosec // G101: test placeholders (VRX_TEST_PSK_<id>, 00-CONTEXT), no credential
		"secret_ref":   "psk/site-a",
		"password_ref": "VRX_TEST_PSK_TD13",
		"auth":         map[string]any{"private_key_ref": "VRX_TEST_PSK_TD13_pk", "psk0": "10.0.0.9"},
		"ca":           "cert/lab-ca",
		"community":    "65000:70000",
		"peer":         "10.0.0.2",
	}
	d.setCheck(func(context.Context, scheduler.Key, proto.Message, scheduler.ReadOnlyView) error {
		return errors.New(`line 7: "secret psk/site-a; password VRX_TEST_PSK_TD13; key VRX_TEST_PSK_TD13_pk; ca cert/lab-ca" near router community 65000:70000 peer 10.0.0.2 psk0 10.0.0.9`)
	})
	r := s.Apply(context.Background(), []scheduler.KV{d.kv("ipsec", "", extra)}, nil)
	msg := onlyIssue(t, r.Plan).Message
	for _, leak := range []string{"psk/site-a", "VRX_TEST_PSK_TD13", "VRX_TEST_PSK_TD13_pk", "cert/lab-ca"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("finding leaks %q: %s", leak, msg)
		}
	}
	for _, keep := range []string{"router", "community 65000:70000", "peer 10.0.0.2", "psk0 10.0.0.9", "secret <redacted>;", "ca <redacted>\""} {
		if !strings.Contains(msg, keep) {
			t.Fatalf("finding lost %q: %s", keep, msg)
		}
	}
	if r.Err == nil || strings.Contains(r.Err.Error(), "VRX_TEST_PSK_TD13") {
		t.Fatalf("err %v", r.Err)
	}

	d.setCheck(func(context.Context, scheduler.Key, proto.Message, scheduler.ReadOnlyView) error {
		return errors.New(strings.Repeat("x", 10000))
	})
	r = s.Apply(context.Background(), []scheduler.KV{d.kv("ipsec", "", extra)}, nil)
	if msg = onlyIssue(t, r.Plan).Message; len(msg) > 2200 || !strings.Contains(msg, "more bytes") {
		t.Fatalf("unbounded finding: %d bytes", len(msg))
	}
	mustNoVPPWrite(t, v)
}

// A validator is bounded: one that ignores ctx is abandoned at the deadline (a finding, nothing
// written); one that honours it sees the deadline; a panic is a finding, not a crash; a caller that
// cancels gets a FAILED transaction with its ctx error, not a finding.
func TestValidatorIsBounded(t *testing.T) {
	s, v, d := td13Fixture(t)
	ctx := context.Background()
	desired := []scheduler.KV{loopKV("loop200"), d.kv("dns", "loop200", nil)}

	release := make(chan struct{})
	defer close(release)
	d.setCheck(func(context.Context, scheduler.Key, proto.Message, scheduler.ReadOnlyView) error {
		<-release // ignores ctx
		return nil
	})
	scheduler.SetValidateTimeout(s, 50*time.Millisecond)
	start := time.Now()
	r := s.Apply(ctx, desired, nil)
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("Apply took %s", took)
	}
	if r.Outcome != scheduler.OutcomeFailed || !strings.Contains(onlyIssue(t, r.Plan).Message, "validator did not return within 50ms") {
		t.Fatalf("outcome %s, plan %+v", r.Outcome, r.Plan)
	}
	mustNoVPPWrite(t, v)

	scheduler.SetValidateTimeout(s, 0)
	var deadline time.Duration
	d.setCheck(func(ctx context.Context, _ scheduler.Key, _ proto.Message, _ scheduler.ReadOnlyView) error {
		if dl, ok := ctx.Deadline(); ok {
			deadline = time.Until(dl)
		}
		return nil
	})
	if p, err := s.Plan(ctx, desired, nil); err != nil || len(p.Issues) != 0 {
		t.Fatalf("plan %v %v", err, p)
	}
	if deadline <= 0 || deadline > scheduler.DefaultValidateTimeout {
		t.Fatalf("validator deadline in %s, want (0, %s]", deadline, scheduler.DefaultValidateTimeout)
	}

	d.setCheck(func(context.Context, scheduler.Key, proto.Message, scheduler.ReadOnlyView) error {
		panic("validator bug")
	})
	r = s.Apply(ctx, desired, nil)
	if r.Outcome != scheduler.OutcomeFailed || !strings.Contains(onlyIssue(t, r.Plan).Message, "validator panicked") {
		t.Fatalf("outcome %s, plan %+v", r.Outcome, r.Plan)
	}

	cctx, cancel := context.WithCancel(ctx)
	d.setCheck(func(ctx context.Context, _ scheduler.Key, _ proto.Message, _ scheduler.ReadOnlyView) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	})
	r = s.Apply(cctx, desired, nil)
	if r.Outcome != scheduler.OutcomeFailed || !errors.Is(r.Err, context.Canceled) || r.Plan != nil {
		t.Fatalf("cancelled: outcome %s err %v plan %+v", r.Outcome, r.Err, r.Plan)
	}
	mustNoVPPWrite(t, v)
}

var _ api.Message = (*swInterfaceDump)(nil) // the fake VPP's messages come from example_descriptor_test.go

func (d *fakeDaemon) setBad(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bad = err
}

func (d *fakeDaemon) setCheck(f func(ctx context.Context, key scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.check = f
}

// RedactLeaves on a real configuration document (review M1): by D-040 only *_ref references carry
// secrets, so user object names (an interface named "psk0"), descriptions, addresses and BGP
// communities stay readable in a finding; the SNMP community's secret reference is masked.
func TestRedactLeavesKeepsNonSecrets(t *testing.T) {
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{
	  "interfaces": {"psk0": {"description": "to branch", "ipv4": ["10.0.0.1/24"]}},
	  "routing": {"policy": {"routeMaps": {"rm1": {"entries": [{"set": {"community": ["65000:70000"]}}]}}}},
	  "services": {"snmp": {"communities": {"ro-lab": {"secretRef": "password/snmp-ro"}}}}
	}`), ds); err != nil {
		t.Fatal(err)
	}
	in := "interface psk0: address 10.0.0.1/24 overlaps (description to branch); route-map rm1: set community 65000:70000: malformed; community ro-lab uses password/snmp-ro"
	want := "interface psk0: address 10.0.0.1/24 overlaps (description to branch); route-map rm1: set community 65000:70000: malformed; community ro-lab uses <redacted>"
	if got := scheduler.RedactLeaves(in, ds); got != want {
		t.Fatalf("RedactLeaves\n got %s\nwant %s", got, want)
	}
}

// A validator's panic text is masked and bounded like a finding before it is logged (review L2).
func TestValidatorPanicTextIsMaskedInTheLog(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	v := newFakeVPP()
	d := newFakeDaemon(v)
	reg := scheduler.NewRegistry()
	reg.Register(d)
	s := scheduler.New(reg, slog.New(slog.NewTextHandler(&lockedWriter{w: &buf, mu: &mu}, nil)))
	d.setCheck(func(context.Context, scheduler.Key, proto.Message, scheduler.ReadOnlyView) error {
		panic(fmt.Sprintf("bad line %q", "secret psk/site-a"))
	})
	ref := map[string]any{"secret_ref": "psk/site-a"} //nolint:gosec // G101: a D-051 reference, not a credential
	r := s.Apply(context.Background(), []scheduler.KV{d.kv("ipsec", "", ref)}, nil)
	if r.Outcome != scheduler.OutcomeFailed {
		t.Fatalf("outcome %s", r.Outcome)
	}
	mu.Lock()
	logged := buf.String()
	mu.Unlock()
	if !strings.Contains(logged, "validator panicked") || strings.Contains(logged, "psk/site-a") || !strings.Contains(logged, "<redacted>") {
		t.Fatalf("log:\n%s", logged)
	}
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// The periodic drift check (TD-9) plans with SkipValidators (review M2): a drifted daemon object
// whose checker would now reject it counts as one operation, not one issue that hides every other
// drifted operation, and no checker runs. Plan (DryRun) still validates.
func TestPlanWithSkipValidators(t *testing.T) {
	s, v, d := td13Fixture(t)
	ctx := context.Background()
	desired := []scheduler.KV{loopKV("loop200"), d.kv("dns", "", nil)}
	mustApply(t, s.Apply(ctx, desired, nil))
	// drift: the daemon's running configuration and a VPP interface changed behind the agent's back
	d.mu.Lock()
	d.conf["daemon.fake/dns"] = daemonConf("dns", "elsewhere", nil)
	d.mu.Unlock()
	for _, i := range v.ifaces {
		if i.Tag == "w2:loop200" {
			i.Mtu = 1500
		}
	}
	d.setBad(errors.New("kea-dhcp4 -t: rejected"))
	d.reset()
	v.Reset() // forget the first Apply's calls: the plans below must write nothing

	p, err := s.PlanWith(ctx, desired, nil, scheduler.PlanOptions{SkipValidators: true})
	if err != nil || len(p.Issues) != 0 || len(p.Ops) != 2 {
		t.Fatalf("drift plan: err %v issues %v ops %+v", err, p.Issues, p.Ops)
	}
	if got := d.log(); len(got) != 0 {
		t.Fatalf("the drift plan ran the validator: %v", got)
	}
	if p, err = s.Plan(ctx, desired, nil); err != nil || len(p.Issues) != 1 || len(p.Ops) != 0 {
		t.Fatalf("Plan must still validate: err %v issues %v ops %+v", err, p.Issues, p.Ops)
	}
	mustNoVPPWrite(t, v)
}
