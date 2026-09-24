package provider_test

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"ngfw/sdk/terraform/internal/provider"
	"ngfw/sdk/terraform/internal/tfharness"
)

func setup(t *testing.T, extra map[string]any) (*fakeAPI, *tfharness.H) {
	t.Helper()
	f := newFakeAPI(t)
	h, err := tfharness.New(provider.New("test"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{"url": f.srv.URL, "api_key": fakeKey}
	for k, v := range extra {
		cfg[k] = v
	}
	if err := h.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	return f, h
}

// applyConfig = terraform apply of one resource: plan prior → config (the harness enforces core's plan-validity and
// post-apply consistency rules), apply, then refresh + re-plan must be empty.
func applyConfig(t *testing.T, h *tfharness.H, res string, prior tftypes.Value, attrs map[string]any) (tftypes.Value, *tfharness.Plan) {
	t.Helper()
	st, plan, err := tryApply(h, res, prior, attrs)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := h.Config(res, attrs)
	again, err := h.PlanChange(res, st, cfg)
	if err != nil {
		t.Fatalf("re-plan: %v", err)
	}
	if !again.NoChanges() {
		t.Fatalf("second plan is not empty:\n%s", again.Text)
	}
	return st, plan
}

func tryApply(h *tfharness.H, res string, prior tftypes.Value, attrs map[string]any) (tftypes.Value, *tfharness.Plan, error) {
	cfg, err := h.Config(res, attrs)
	if err != nil {
		return tftypes.Value{}, nil, err
	}
	plan, err := h.PlanChange(res, prior, cfg)
	if err != nil {
		return tftypes.Value{}, nil, err
	}
	st, err := h.Apply(res, plan, cfg)
	if err != nil {
		return tftypes.Value{}, plan, err
	}
	st, err = h.Read(res, st)
	return st, plan, err
}

func destroy(t *testing.T, h *tfharness.H, res string, st tftypes.Value) {
	t.Helper()
	plan, err := h.PlanChange(res, st, h.Null(res))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Apply(res, plan, h.Null(res)); err != nil {
		t.Fatalf("destroy: %v", err)
	}
}

func noCall(t *testing.T, f *fakeAPI, prefixes ...string) {
	t.Helper()
	for _, c := range f.Calls() {
		for _, p := range prefixes {
			if strings.HasPrefix(c, p) {
				t.Fatalf("unexpected call %s\n%s", c, strings.Join(f.Calls(), "\n"))
			}
		}
	}
}

func TestConfigResourceLifecycle(t *testing.T) {
	f, h := setup(t, nil)
	st, plan := applyConfig(t, h, "vrx_config", h.Null("vrx_config"), map[string]any{
		"pointer": "/interfaces/loop1", "value": `{"enabled":true,"ipv4":["10.0.0.1/24"]}`,
	})
	if !strings.Contains(plan.Text, `+ pointer`) || !strings.Contains(plan.Text, "revision                 = (known after apply)") {
		t.Fatalf("create plan:\n%s", plan.Text)
	}
	if got := tfharness.Attr(st, "revision"); got != 1.0 {
		t.Fatalf("revision: %v", got)
	}
	want := []string{"GET /api/v1/state/system", "GET /api/v1/config/diff", "GET /api/v1/config/interfaces/loop1",
		"PUT /api/v1/config/interfaces/loop1", "GET /api/v1/config/diff",
		"POST /api/v1/config/commit?comment=terraform%3A+create+%2Finterfaces%2Floop1&confirm=60",
		"GET /api/v1/state/system", "POST /api/v1/config/commit/confirm"}
	if got := f.Calls()[:len(want)]; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s", strings.Join(got, "\n"))
	}

	// H1: a reformatted value is planned exactly as configured (core's rule) — a harmless update that commits nothing
	// and succeeds; afterwards the plan is empty again
	st, plan = applyConfig(t, h, "vrx_config", st, map[string]any{"pointer": "/interfaces/loop1", "value": `{ "ipv4": ["10.0.0.1/24"], "enabled": true }`})
	if !strings.Contains(plan.Text, "~ value") {
		t.Fatalf("reformat plan:\n%s", plan.Text)
	}
	if got := tfharness.Attr(st, "revision"); got != 0.0 {
		t.Fatalf("a reformat must not commit: revision %v", got)
	}

	// update
	st, plan = applyConfig(t, h, "vrx_config", st, map[string]any{"pointer": "/interfaces/loop1", "value": `{"enabled":true,"ipv4":["10.0.0.2/24"]}`})
	if !strings.Contains(plan.Text, `~ value`) || !strings.Contains(plan.Text, "10.0.0.2/24") {
		t.Fatalf("update plan:\n%s", plan.Text)
	}
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop1", "value": `{"enabled":true,"ipv4":["10.0.0.2/24"]}`})

	// drift of a configured member is detected on refresh
	f.setRunning([]string{"interfaces", "loop1", "enabled"}, false)
	st2, err := h.Read("vrx_config", st)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := h.PlanChange("vrx_config", st2, cfg); err != nil || p.NoChanges() || !strings.Contains(p.Text, `\"enabled\":false`) {
		t.Fatalf("changed member not planned: %v\n%v", err, p)
	}
	f.setRunning([]string{"interfaces", "loop1", "enabled"}, true)

	// M4: a member ADDED out of band is drift too (a PUT would remove it — the plan must show that)
	f.setRunning([]string{"interfaces", "loop1", "mtu"}, 9000)
	st2, err = h.Read("vrx_config", st)
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.PlanChange("vrx_config", st2, cfg)
	if err != nil || p.NoChanges() || !strings.Contains(p.Text, `\"mtu\":9000`) {
		t.Fatalf("added member not planned: %v\n%v", err, p)
	}
	st, err = h.Apply("vrx_config", p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := get(f.running, []string{"interfaces", "loop1", "mtu"}); n != nil {
		t.Fatal("mtu not removed by the (planned) replacement")
	}

	// pointer change → replace
	cfg, _ = h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop2", "value": `{}`})
	if p, err := h.PlanChange("vrx_config", st, cfg); err != nil || len(p.RequiresReplace) == 0 {
		t.Fatalf("pointer change must replace: %v", err)
	}
	destroy(t, h, "vrx_config", st)
	if _, ok := get(f.running, []string{"interfaces", "loop1"}); ok {
		t.Fatal("loop1 still in running after destroy")
	}
}

func TestNonCanonicalPointerIsRefused(t *testing.T) {
	_, h := setup(t, nil)
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "interfaces/loop1/", "value": `{}`})
	if err := h.Validate("vrx_config", cfg); err == nil || !strings.Contains(err.Error(), `write the pointer as "/interfaces/loop1"`) {
		t.Fatalf("want canonical-pointer error, got %v", err)
	}
}

func TestConfigImportAndExisting(t *testing.T) {
	f, h := setup(t, nil)
	f.setRunning([]string{"interfaces", "loop9"}, map[string]any{"enabled": true, "vrf": "default", "ipv4": []any{"192.0.2.9/32"}})
	st, err := h.Import("vrx_config", "/interfaces/loop9")
	if err != nil {
		t.Fatal(err)
	}
	if v := tfharness.Attr(st, "value"); v != `{"enabled":true,"ipv4":["192.0.2.9/32"],"vrf":"default"}` {
		t.Fatalf("imported value: %v", v)
	}
	// H1 regression (review probe): after import the user writes the same JSON differently — the plan must be valid
	// (planned == config), the update commits nothing and succeeds, and the next plan is empty
	applyConfig(t, h, "vrx_config", st, map[string]any{"pointer": "/interfaces/loop9", "value": `{"vrf":"default","ipv4":["192.0.2.9/32"],"enabled":true}`})

	// creating over an existing node is refused (import it instead)
	_, _, err = tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop9", "value": `{}`})
	if err == nil || !strings.Contains(err.Error(), "import it first") {
		t.Fatalf("want import hint, got %v", err)
	}
}

func TestConfigSecretsNeverInPlanOrState(t *testing.T) {
	f, h := setup(t, nil)
	hash := "$argon2id$v=19$m=65536,t=3,p=4$VRX_TEST_PSK_salt$VRX_TEST_PSK_hash"
	// a write-only member inside `value` is refused at validation time
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/management/users",
		"value": `[{"username":"ops","role":"operator","passwordHash":"` + hash + `"}]`})
	if err := h.Validate("vrx_config", cfg); err == nil || !strings.Contains(err.Error(), "/management/users/0/passwordHash is write-only") {
		t.Fatalf("want write-only refusal, got %v", err)
	}
	// through sensitive_value (merged by username, never by index) it is sent, but never planned, stored or rendered
	st, plan := applyConfig(t, h, "vrx_config", h.Null("vrx_config"), map[string]any{
		"pointer":         "/management/users",
		"value":           `[{"username":"admin","role":"admin"},{"username":"ops","role":"operator"}]`,
		"sensitive_value": `[{"username":"ops","passwordHash":"` + hash + `"}]`,
	})
	if !strings.Contains(strings.Join(f.bodies, "\n"), `"passwordHash":"`+hash+`","role":"operator","username":"ops"`) {
		t.Fatalf("the hash did not reach the ops user: %v", f.bodies)
	}
	if strings.Contains(plan.Text, hash) || !strings.Contains(plan.Text, "sensitive_value          = (write-only attribute)") {
		t.Fatalf("plan text:\n%s", plan.Text)
	}
	if tfharness.Attr(plan.Planned, "sensitive_value") != nil || tfharness.Attr(st, "sensitive_value") != nil {
		t.Fatal("write-only value reached plan or state")
	}
	if strings.Contains(st.String(), "argon2") || strings.Contains(plan.Planned.String(), "argon2") {
		t.Fatal("hash in state/plan")
	}
	if u, _ := get(f.running, []string{"management", "users", "1", "passwordHash"}); u != hash {
		t.Fatalf("running lost the hash: %v", u)
	}
	// L3: a sensitive element that matches no user is an error, not a hash on the wrong account
	_, _, err := tryApply(h, "vrx_config", st, map[string]any{"pointer": "/management/users",
		"value":           `[{"username":"admin","role":"admin"},{"username":"ops","role":"operator"}]`,
		"sensitive_value": `[{"username":"nobody","passwordHash":"` + hash + `"}]`, "sensitive_value_version": 2})
	if err == nil || !strings.Contains(err.Error(), "not in value") || strings.Contains(err.Error(), hash) {
		t.Fatalf("want an unmatched-key error without the hash, got %v", err)
	}
}

// H2 (review probe): `value` is unknown at validate time (it references another resource), known at plan/apply.
func TestWriteOnlyGuardWhenValueUnknownAtValidate(t *testing.T) {
	f, h := setup(t, nil)
	hash := "$argon2id$v=19$m=65536,t=3,p=4$VRX_TEST_PSK_salt$VRX_TEST_PSK_hash"
	withHash := `[{"username":"admin","role":"admin"},{"username":"ops","role":"operator","passwordHash":"` + hash + `"}]`
	unknown, _ := h.Config("vrx_config", map[string]any{"pointer": "/management/users",
		"value": tftypes.NewValue(tftypes.String, tftypes.UnknownValue)})
	if err := h.Validate("vrx_config", unknown); err != nil {
		t.Fatalf("validate with an unknown value: %v", err)
	}
	// plan with the value known → refused in ModifyPlan
	known, _ := h.Config("vrx_config", map[string]any{"pointer": "/management/users", "value": withHash})
	if _, err := h.PlanChange("vrx_config", h.Null("vrx_config"), known); err == nil || !strings.Contains(err.Error(), "write-only") {
		t.Fatalf("plan must refuse the hash, got %v", err)
	}
	// value unknown during plan too, known only at apply → refused in Create before any HTTP write
	plan, err := h.PlanChange("vrx_config", h.Null("vrx_config"), unknown)
	if err != nil {
		t.Fatal(err)
	}
	planned := setAttr(t, plan.Planned, "value", withHash)
	plan.Planned = planned
	st, err := h.Apply("vrx_config", plan, known)
	if err == nil || !strings.Contains(err.Error(), "must not be in value") {
		t.Fatalf("apply must refuse the hash, got %v", err)
	}
	if strings.Contains(st.String(), "argon2") {
		t.Fatal("hash reached the state")
	}
	noCall(t, f, "PUT ", "POST /api/v1/config/commit")
}

func setAttr(t *testing.T, v tftypes.Value, name string, s string) tftypes.Value {
	t.Helper()
	var m map[string]tftypes.Value
	if err := v.As(&m); err != nil {
		t.Fatal(err)
	}
	m[name] = tftypes.NewValue(tftypes.String, s)
	return tftypes.NewValue(v.Type(), m)
}

func TestDirtyCandidateIsRefusedAndNeverDiscarded(t *testing.T) {
	f, h := setup(t, nil)
	set(f.candidate, []string{"system", "hostname"}, "someone-elses-edit")
	_, _, err := tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
	if err == nil || !strings.Contains(err.Error(), "uncommitted change") {
		t.Fatalf("want dirty-candidate refusal, got %v", err)
	}
	noCall(t, f, "PUT ", "POST /api/v1/config/commit", "POST /api/v1/config/discard")
}

// M3: another run of the same user edits the shared candidate while we do.
func TestConcurrentEditIsNotCommittedNorDiscarded(t *testing.T) {
	f, h := setup(t, nil)
	f.onPut = func(toks []string) { set(f.candidate, []string{"system", "hostname"}, "other-run") }
	_, _, err := tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
	if err == nil || !strings.Contains(err.Error(), "candidate now also contains") {
		t.Fatalf("want concurrent-edit error, got %v", err)
	}
	noCall(t, f, "POST /api/v1/config/commit", "POST /api/v1/config/discard")
	if v, _ := get(f.candidate, []string{"system", "hostname"}); v != "other-run" {
		t.Fatal("the other run's edit was lost")
	}
}

// M3: the other run discarded the candidate between our edit and our commit → Create must not report success.
func TestCreateThatCommittedNothingFails(t *testing.T) {
	f, h := setup(t, nil)
	f.onPut = func(toks []string) { f.candidate = cloneDoc(f.running) }
	_, _, err := tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
	if err == nil || !strings.Contains(err.Error(), "nothing was committed") {
		t.Fatalf("want nothing-committed error, got %v", err)
	}
}

// L4: after an unconfirmed (reverted) commit the candidate still holds our edit; the next run recognises it.
func TestOwnLeftoverAfterRevertIsReused(t *testing.T) {
	f, h := setup(t, nil)
	set(f.candidate, []string{"interfaces", "loop1"}, map[string]any{"enabled": true, "ipv4": []any{}, "ipv6": []any{}, "vrf": "default", "promiscuous": false, "subinterfaces": map[string]any{}})
	applyConfig(t, h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{"enabled":true}`})
	if _, ok := get(f.running, []string{"interfaces", "loop1"}); !ok {
		t.Fatal("not committed")
	}
}

func TestValidationErrorKeepsPointer(t *testing.T) {
	f, h := setup(t, nil)
	_, _, err := tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{"ipv4":["bad"]}`})
	if err == nil || !strings.Contains(err.Error(), "/interfaces/loop1/ipv4/0: Invalid IPv4 range") {
		t.Fatalf("want the pointer in the diagnostic, got %v", err)
	}
	noCall(t, f, "POST /api/v1/config/commit")
}

func TestFailedPostCommitCheckDoesNotConfirm(t *testing.T) {
	f, h := setup(t, map[string]any{"confirm_timeout": 30})
	f.failState = true
	_, _, err := tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
	if err == nil || !strings.Contains(err.Error(), "NOT confirmed") || !strings.Contains(err.Error(), "config/discard") {
		t.Fatalf("want a not-confirmed error with the recovery hint, got %v", err)
	}
	noCall(t, f, "POST /api/v1/config/commit/confirm")
	if !strings.Contains(strings.Join(f.Calls(), "\n"), "confirm=30") {
		t.Fatal("confirm_timeout not used")
	}
}

// M2: running ↔ data plane not in sync → no edit (unless allow_unsynced).
func TestUnsyncedApplianceIsRefused(t *testing.T) {
	f, h := setup(t, nil)
	f.sync = "unknown"
	_, _, err := tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
	if err == nil || !strings.Contains(err.Error(), `sync is "unknown"`) {
		t.Fatalf("want an unsynced refusal, got %v", err)
	}
	noCall(t, f, "PUT ")

	f2, h2 := setup(t, map[string]any{"allow_unsynced": true})
	f2.sync = "degraded"
	applyConfig(t, h2, "vrx_config", h2.Null("vrx_config"), map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
}

// M1: stored but not enforced must not look like success.
func TestNotAppliedIsAnErrorWhenAsked(t *testing.T) {
	f, h := setup(t, map[string]any{"fail_on_not_applied": true})
	f.notApplied = []string{"nat"}
	_, _, err := tryApply(h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/nat/x", "value": `{"a":1}`})
	if err == nil || !strings.Contains(err.Error(), "NOT enforced") || !strings.Contains(err.Error(), "[nat]") {
		t.Fatalf("want a not-enforced error, got %v", err)
	}
}

func TestNotAppliedWarning(t *testing.T) {
	f, h := setup(t, nil)
	f.notApplied = []string{"nat"}
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/nat/x", "value": `{"a":1}`})
	plan, err := h.PlanChange("vrx_config", h.Null("vrx_config"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Apply("vrx_config", plan, cfg); err != nil {
		t.Fatal(err)
	}
	if w := strings.Join(h.Warnings, "\n"); !strings.Contains(w, "NOT enforced") || !strings.Contains(w, "[nat]") {
		t.Fatalf("want a not-enforced warning, got %q", w)
	}
}

func TestNoConfirmWhenTimeoutZero(t *testing.T) {
	f, h := setup(t, map[string]any{"confirm_timeout": 0})
	applyConfig(t, h, "vrx_config", h.Null("vrx_config"), map[string]any{"pointer": "/system/hostname", "value": `"vrx-a"`})
	for _, c := range f.Calls() {
		if strings.Contains(c, "confirm") {
			t.Fatalf("unexpected confirm call %s", c)
		}
	}
}

func TestProviderConfiguration(t *testing.T) {
	h, _ := tfharness.New(provider.New("test"))
	if err := h.Configure(map[string]any{"url": "https://vrx.test"}); err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("missing key: %v", err)
	}
	t.Setenv("VRX_API_KEY", fakeKey)
	if err := h.Configure(map[string]any{"url": "https://user:pw@vrx.test"}); err == nil || !strings.Contains(err.Error(), "credentials in the URL") {
		t.Fatalf("credentials in url: %v", err)
	}
	if err := h.Configure(map[string]any{"url": "https://vrx.test", "confirm_timeout": 4000}); err == nil {
		t.Fatal("confirm_timeout 4000 accepted")
	}
	if err := h.Configure(map[string]any{"url": "https://vrx.test"}); err != nil {
		t.Fatalf("key from env: %v", err)
	}
	// L2: plain http to a remote host needs allow_http (and warns); loopback is allowed
	if err := h.Configure(map[string]any{"url": "http://vrx.test"}); err == nil || !strings.Contains(err.Error(), "refusing plain http://") {
		t.Fatalf("http to a remote host: %v", err)
	}
	if err := h.Configure(map[string]any{"url": "http://vrx.test", "allow_http": true}); err != nil || !strings.Contains(strings.Join(h.Warnings, ""), "without TLS") {
		t.Fatalf("allow_http: %v %v", err, h.Warnings)
	}
	if err := h.Configure(map[string]any{"url": "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("loopback http: %v", err)
	}
}

func TestInterfaceResource(t *testing.T) {
	f, h := setup(t, nil)
	st, plan := applyConfig(t, h, "vrx_interface", h.Null("vrx_interface"), map[string]any{
		"name": "loop2", "enabled": true, "ipv4": []any{"10.0.2.1/24"}, "mtu": 1500,
	})
	if !strings.Contains(plan.Text, `+ vrf                      = "default"`) {
		t.Fatalf("defaults from the JSON Schema not planned:\n%s", plan.Text)
	}
	body := f.bodies[len(f.bodies)-1]
	for _, w := range []string{`"mtu":1500`, `"ipv4":["10.0.2.1/24"]`, `"vrf":"default"`, `"subinterfaces":{}`} {
		if !strings.Contains(body, w) {
			t.Fatalf("PUT body %s lacks %s", body, w)
		}
	}
	if strings.Contains(body, `"name"`) || strings.Contains(body, `"revision"`) {
		t.Fatalf("provider attributes leaked into the body: %s", body)
	}
	// nested + renamed attributes (snake_case ↔ camelCase) round-trip; nested defaults pass core's plan-validity rule
	st, _ = applyConfig(t, h, "vrx_interface", st, map[string]any{
		"name": "loop2", "enabled": true, "ipv4": []any{"10.0.2.1/24"}, "mtu": 9000, "rx_mode": "interrupt",
		"subinterfaces": map[string]any{"100": map[string]any{"vlan_id": 100, "ipv4": []any{"10.0.100.1/24"}}},
		"dhcp_client":   map[string]any{"hostname": "edge"},
	})
	if body := f.bodies[len(f.bodies)-1]; !strings.Contains(body, `"rxMode":"interrupt"`) || !strings.Contains(body, `"vlanId":100`) {
		t.Fatalf("JSON names not mapped: %s", body)
	}
	imp, err := h.Import("vrx_interface", "loop2")
	if err != nil {
		t.Fatal(err)
	}
	if tfharness.Attr(imp, "rx_mode") != "interrupt" || tfharness.Attr(imp, "mtu") != 9000.0 {
		t.Fatalf("import: %v", tfharness.Native(imp))
	}
	// M4: members the generated schema does not know are never silently deleted by an update
	f.setRunning([]string{"interfaces", "loop2", "futureField"}, true)
	_, _, err = tryApply(h, "vrx_interface", st, map[string]any{"name": "loop2", "enabled": false})
	if err == nil || !strings.Contains(err.Error(), "futureField") {
		t.Fatalf("want an unknown-member refusal, got %v", err)
	}
	f.mu.Lock()
	delete(f.running["interfaces"].(map[string]any)["loop2"].(map[string]any), "futureField")
	f.candidate = cloneDoc(f.running)
	f.mu.Unlock()
	destroy(t, h, "vrx_interface", st)
	if _, ok := get(f.running, []string{"interfaces", "loop2"}); ok {
		t.Fatal("loop2 still in running")
	}
}

func TestStateDataSource(t *testing.T) {
	f, h := setup(t, nil)
	f.setRunning([]string{"interfaces", "loop3"}, map[string]any{"ipv4": []any{"10.0.3.1/24"}})
	v, err := h.ReadData("vrx_state", map[string]any{"path": "interfaces"})
	if err != nil {
		t.Fatal(err)
	}
	if j, _ := tfharness.Attr(v, "json").(string); !strings.Contains(j, `"name":"loop3"`) {
		t.Fatalf("state json: %s", j)
	}
	for _, bad := range []string{"../config", "interfaces/../../config", "auth"} {
		if _, err := h.ReadData("vrx_state", map[string]any{"path": bad}); err == nil {
			t.Fatalf("state path %q accepted", bad)
		}
	}
}
