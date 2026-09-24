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

// applyConfig = terraform apply of one resource: plan prior → config, apply, then refresh + re-plan must be empty.
func applyConfig(t *testing.T, h *tfharness.H, res string, prior tftypes.Value, attrs map[string]any) (tftypes.Value, *tfharness.Plan) {
	t.Helper()
	cfg, err := h.Config(res, attrs)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := h.PlanChange(res, prior, cfg)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	st, err := h.Apply(res, plan, cfg)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	st, err = h.Read(res, st)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	again, err := h.PlanChange(res, st, cfg)
	if err != nil {
		t.Fatalf("re-plan: %v", err)
	}
	if !again.NoChanges() {
		t.Fatalf("second plan is not empty:\n%s", again.Text)
	}
	return st, plan
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

func TestConfigResourceLifecycle(t *testing.T) {
	f, h := setup(t, nil)
	st, plan := applyConfig(t, h, "vrx_config", h.Null("vrx_config"), map[string]any{
		"pointer": "interfaces/loop1/", "value": `{"enabled":true,"ipv4":["10.0.0.1/24"]}`,
	})
	if !strings.Contains(plan.Text, `+ pointer`) || !strings.Contains(plan.Text, "revision                 = (known after apply)") {
		t.Fatalf("create plan:\n%s", plan.Text)
	}
	if got := tfharness.Attr(st, "id"); got != "/interfaces/loop1" {
		t.Fatalf("id = normalized pointer: %v", got)
	}
	if got := tfharness.Attr(st, "revision"); got != 1.0 {
		t.Fatalf("revision: %v", got)
	}
	want := []string{"GET /api/v1/config/diff", "GET /api/v1/config/interfaces/loop1", "PUT /api/v1/config/interfaces/loop1",
		"GET /api/v1/config/diff", "POST /api/v1/config/commit?comment=terraform%3A+create+%2Finterfaces%2Floop1&confirm=60",
		"GET /api/v1/state/system", "POST /api/v1/config/commit/confirm"}
	if got := f.Calls()[:len(want)]; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s", strings.Join(got, "\n"))
	}

	// semantically equal JSON (order, whitespace) is not a change
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop1", "value": `{ "ipv4": ["10.0.0.1/24"], "enabled": true }`})
	if p, err := h.PlanChange("vrx_config", st, cfg); err != nil || !p.NoChanges() {
		t.Fatalf("semantic JSON: %v\n%v", err, p)
	}

	// update
	st, plan = applyConfig(t, h, "vrx_config", st, map[string]any{"pointer": "/interfaces/loop1", "value": `{"enabled":true,"ipv4":["10.0.0.2/24"]}`})
	if !strings.Contains(plan.Text, `~ value`) || !strings.Contains(plan.Text, "10.0.0.2/24") {
		t.Fatalf("update plan:\n%s", plan.Text)
	}

	// out-of-band drift of a configured member is detected on refresh
	f.setRunning([]string{"interfaces", "loop1", "enabled"}, false)
	st, err := h.Read("vrx_config", st)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ = h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop1", "value": `{"enabled":true,"ipv4":["10.0.0.2/24"]}`})
	p, err := h.PlanChange("vrx_config", st, cfg)
	if err != nil || p.NoChanges() || !strings.Contains(p.Text, `\"enabled\":false`) {
		t.Fatalf("drift not planned: %v\n%s", err, p.Text)
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
	// creating over an existing node is refused (import it instead); the candidate is discarded
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop9", "value": `{}`})
	plan, err := h.PlanChange("vrx_config", h.Null("vrx_config"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Apply("vrx_config", plan, cfg); err == nil || !strings.Contains(err.Error(), "terraform import") {
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
	// through sensitive_value it is sent, but never planned, stored or rendered
	st, plan := applyConfig(t, h, "vrx_config", h.Null("vrx_config"), map[string]any{
		"pointer":         "/management/users",
		"value":           `[{"username":"ops","role":"operator"}]`,
		"sensitive_value": `[{"passwordHash":"` + hash + `"}]`,
	})
	if !strings.Contains(strings.Join(f.bodies, "\n"), hash) {
		t.Fatal("the API never received the hash")
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
	// the stored user kept its hash in the fake's running doc; the API redacts it on reads → no drift
	if u, _ := get(f.running, []string{"management", "users", "0", "passwordHash"}); u != hash {
		t.Fatalf("running lost the hash: %v", u)
	}
}

func TestDirtyCandidateIsRefused(t *testing.T) {
	f, h := setup(t, nil)
	set(f.candidate, []string{"system", "hostname"}, "someone-elses-edit")
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
	plan, _ := h.PlanChange("vrx_config", h.Null("vrx_config"), cfg)
	_, err := h.Apply("vrx_config", plan, cfg)
	if err == nil || !strings.Contains(err.Error(), "uncommitted change") {
		t.Fatalf("want dirty-candidate refusal, got %v", err)
	}
	for _, c := range f.Calls() {
		if strings.HasPrefix(c, "PUT ") || strings.HasPrefix(c, "POST /api/v1/config/commit") {
			t.Fatalf("edited or committed a dirty candidate: %s", c)
		}
	}
}

func TestValidationErrorKeepsPointerAndDiscards(t *testing.T) {
	f, h := setup(t, nil)
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop1", "value": `{"ipv4":["bad"]}`})
	plan, _ := h.PlanChange("vrx_config", h.Null("vrx_config"), cfg)
	_, err := h.Apply("vrx_config", plan, cfg)
	if err == nil || !strings.Contains(err.Error(), "/interfaces/loop1/ipv4/0: Invalid IPv4 range") {
		t.Fatalf("want the pointer in the diagnostic, got %v", err)
	}
	if c := f.Calls(); c[len(c)-1] != "POST /api/v1/config/discard" {
		t.Fatalf("candidate not discarded: %v", c)
	}
}

func TestFailedPostCommitCheckDoesNotConfirm(t *testing.T) {
	f, h := setup(t, map[string]any{"confirm_timeout": 30})
	f.failState = true
	cfg, _ := h.Config("vrx_config", map[string]any{"pointer": "/interfaces/loop1", "value": `{}`})
	plan, _ := h.PlanChange("vrx_config", h.Null("vrx_config"), cfg)
	_, err := h.Apply("vrx_config", plan, cfg)
	if err == nil || !strings.Contains(err.Error(), "NOT confirming") {
		t.Fatalf("want a not-confirmed error, got %v", err)
	}
	for _, c := range f.Calls() {
		if c == "POST /api/v1/config/commit/confirm" {
			t.Fatal("confirmed although the check failed")
		}
	}
	if !strings.Contains(strings.Join(f.Calls(), "\n"), "confirm=30") {
		t.Fatal("confirm_timeout not used")
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
	// nested + renamed attributes (snake_case ↔ camelCase) round-trip
	st, _ = applyConfig(t, h, "vrx_interface", st, map[string]any{
		"name": "loop2", "enabled": true, "ipv4": []any{"10.0.2.1/24"}, "mtu": 9000, "rx_mode": "interrupt",
		"subinterfaces": map[string]any{"100": map[string]any{"vlan_id": 100, "ipv4": []any{"10.0.100.1/24"}}},
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
	if _, err := h.ReadData("vrx_state", map[string]any{"path": "../config"}); err == nil {
		t.Fatal("path traversal accepted")
	}
}
