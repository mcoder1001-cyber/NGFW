package agent

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// TD-13: a Validator's finding reaches the existing DryRun answer (ValidationReport) and a FAILED
// Apply's validation, with rule agent.validator, the key in the message and the pointer the
// validator named (else the object's own); VPP gets no write.

type checkFunc func(ctx context.Context, key scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error

// validating adds a Validator to a product descriptor (core.LoopbackDescriptor has no other
// optional extension, so wrapping it hides nothing).
type validating struct {
	scheduler.Descriptor
	check checkFunc
}

func (v *validating) Validate(ctx context.Context, key scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error {
	return v.check(ctx, key, value, view)
}

// validatingRegistry wraps the descriptor named name as subsystems.Register registers it (the
// ownership guard still sees the original).
type validatingRegistry struct {
	*scheduler.MapRegistry
	name  string
	check checkFunc
}

func (r *validatingRegistry) Register(d scheduler.Descriptor) {
	if d.Name() == r.name {
		d = &validating{Descriptor: d, check: r.check}
	}
	r.MapRegistry.Register(d)
}

// newSvcValidating is newSvc with check as the Validator of the descriptor named name.
func newSvcValidating(t *testing.T, v *coretest.VPP, dir, name string, check checkFunc) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := &validatingRegistry{MapRegistry: scheduler.NewRegistry(), name: name, check: check}
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg.MapRegistry, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	t.Cleanup(svc.Close)
	return svc
}

// onlyReads fails when the fake VPP received anything but the reads a plan makes.
func onlyReads(t *testing.T, v *coretest.VPP) {
	t.Helper()
	for _, c := range v.Calls() {
		if n := c.GetMessageName(); !strings.HasSuffix(n, "_dump") && n != "control_ping" && n != "sw_interface_get_table" {
			t.Fatalf("VPP received %s: a rejected configuration was written", n)
		}
	}
}

// validatorFindings returns "pointer|message" of every agent.validator issue, sorted.
func validatorFindings(rep *vrxv1.ValidationReport) []string {
	var out []string
	for _, e := range rep.GetErrors() {
		if e.GetRule() == "agent.validator" && e.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			out = append(out, e.GetPointer()+"|"+e.GetMessage())
		}
	}
	sort.Strings(out)
	return out
}

func TestDryRunAndApplyReportValidatorFindings(t *testing.T) {
	v := coretest.New()
	var mu sync.Mutex
	var validated []string
	check := func(_ context.Context, key scheduler.Key, _ proto.Message, _ scheduler.ReadOnlyView) error {
		mu.Lock()
		validated = append(validated, string(key))
		mu.Unlock()
		switch key.ID() {
		case "loop701":
			return scheduler.InvalidAt("/interfaces/loop701/description", errors.New("checker: line 3: bad description"))
		case "loop702":
			return errors.New("checker: rejected")
		}
		return nil
	}
	s := newSvcValidating(t, v, t.TempDir(), core.LoopbackName, check)
	ds := doc(t, `{"interfaces": {"loop701": {}, "loop702": {}, "loop703": {}}}`)
	want := []string{
		"/interfaces/loop701/description|interface.loopback/loop701: validator: checker: line 3: bad description",
		"/interfaces/loop702|interface.loopback/loop702: validator: checker: rejected",
	}

	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: ds})
	if err != nil {
		t.Fatal(err)
	}
	if got := validatorFindings(rep); rep.GetOk() || len(rep.GetPlan()) != 0 || strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("DryRun: ok=%v plan=%v findings:\n%s\nwant:\n%s\nreport %v", rep.GetOk(), rep.GetPlan(), strings.Join(got, "\n"), strings.Join(want, "\n"), rep)
	}
	mu.Lock()
	sort.Strings(validated)
	got := strings.Join(validated, ",")
	validated = nil
	mu.Unlock()
	if got != "interface.loopback/loop701,interface.loopback/loop702,interface.loopback/loop703" {
		t.Fatalf("validated %s", got)
	}
	onlyReads(t, v)

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "a1", DesiredState: ds})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	if got := validatorFindings(resp.GetValidation()); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Apply validation findings:\n%s", strings.Join(got, "\n"))
	}
	codes := map[string]vrxv1.ObjectResultCode{}
	for _, r := range resp.GetResults() {
		codes[r.GetKey()] = r.GetCode()
	}
	for _, k := range []string{"interface.loopback/loop701", "interface.loopback/loop702"} {
		if codes[k] != vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_INVALID {
			t.Fatalf("result of %s: %v (all %v)", k, codes[k], resp.GetResults())
		}
	}
	onlyReads(t, v)
	if len(v.CallsNamed("create_loopback_instance")) != 0 {
		t.Fatal("a rejected transaction created a loopback")
	}
}
