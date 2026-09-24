package provider_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"ngfw/sdk/terraform/internal/client"
	"ngfw/sdk/terraform/internal/provider"
	"ngfw/sdk/terraform/internal/tfharness"
)

// TestLive drives the provider against a REAL VRX API + vrx-agent + VPP on the caller's slot, through the plugin
// protocol exactly as Terraform core would (the terraform CLI is not installed on this host):
//
//	eval "$(tools/lab env 5)"
//	test/topology/sdk-terraform-ansible/live.sh run go -C sdk/terraform test -count=1 -v -run TestLive ./internal/provider
//
// Skipped unless VRX_INTEGRATION=1 and VRX_SDK_URL / VRX_SDK_API_KEY_FILE are set. Objects: loop<slot>21/22 on
// 10.<slot>.12{1,2}.0/24 and the user <prefix>tf; all removed at the end.
func TestLive(t *testing.T) {
	url, keyFile := os.Getenv("VRX_SDK_URL"), os.Getenv("VRX_SDK_API_KEY_FILE")
	if os.Getenv("VRX_INTEGRATION") != "1" || url == "" || keyFile == "" {
		t.Skip("live run: VRX_INTEGRATION=1 + VRX_SDK_URL + VRX_SDK_API_KEY_FILE (test/topology/sdk-terraform-ansible/live.sh run …)")
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	prefix := os.Getenv("VRX_TEST_PREFIX")
	slot := regexp.MustCompile(`\d+`).FindString(prefix)
	if slot == "" {
		t.Fatal("VRX_TEST_PREFIX must be w<N>")
	}
	h, err := tfharness.New(provider.New("live"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Configure(map[string]any{"url": url, "api_key": strings.TrimSpace(string(key)), "confirm_timeout": 30,
		"commit_comment": "terraform live " + prefix}); err != nil {
		t.Fatal(err)
	}
	say := func(format string, a ...any) { fmt.Printf("[tf-live] "+format+"\n", a...) }
	step := func(res string, prior tftypes.Value, attrs map[string]any) tftypes.Value {
		t.Helper()
		var cfg tftypes.Value
		if attrs == nil {
			cfg = h.Null(res)
		} else if cfg, err = h.Config(res, attrs); err != nil {
			t.Fatal(err)
		}
		plan, err := h.PlanChange(res, prior, cfg)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		fmt.Printf("$ terraform plan   (%s)\n%s\n", res, plan.Text)
		st, err := h.Apply(res, plan, cfg)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if attrs == nil {
			say("destroy applied (confirmed commit)")
			return st
		}
		say("apply → revision %v", tfharness.Attr(st, "revision"))
		if st, err = h.Read(res, st); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		again, err := h.PlanChange(res, st, cfg)
		if err != nil {
			t.Fatalf("re-plan: %v", err)
		}
		fmt.Printf("$ terraform plan   (again, after refresh)\n%s\n", again.Text)
		if !again.NoChanges() {
			t.Fatalf("second plan is not empty")
		}
		return st
	}
	ifA, ifB := "loop"+slot+"21", "loop"+slot+"22"
	ptrA := "/interfaces/" + ifA
	user := prefix + "tf"
	// leftovers of an interrupted run are removed before and after (shared-host rules: clean up in Cleanup)
	cleanup := func() {
		c, err := client.New(client.Options{URL: url, APIKey: strings.TrimSpace(string(key))})
		if err != nil {
			t.Log(err)
			return
		}
		ctx := context.Background()
		for _, p := range []string{ptrA, "/interfaces/" + ifB} {
			if _, err := c.Running(ctx, p); client.IsNotFound(err) {
				continue
			}
			if _, err := c.Apply(ctx, client.ApplyOptions{Comment: "terraform live cleanup"}, client.Edit{Pointer: p, Absent: true,
				Do: func(ctx context.Context) error { return c.Delete(ctx, p) }}); err != nil {
				t.Logf("cleanup %s: %v", p, err)
			}
		}
		if u, err := c.Running(ctx, "/management/users"); err == nil && strings.Contains(fmt.Sprint(u), user) {
			want := []any{map[string]any{"username": "admin", "role": "admin"}}
			if _, err := c.Apply(ctx, client.ApplyOptions{Comment: "terraform live cleanup"}, client.Edit{Pointer: "/management/users", Want: want,
				Do: func(ctx context.Context) error { return c.Put(ctx, "/management/users", want) }}); err != nil {
				t.Logf("cleanup users: %v", err)
			}
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	cidr := func(n, host int) string { return fmt.Sprintf("10.%s.%d.%d/24", slot, n, host) }

	// 1. vrx_config: create → no-diff → update → no-diff
	stA := step("vrx_config", h.Null("vrx_config"), map[string]any{"pointer": ptrA,
		"value": fmt.Sprintf(`{"enabled":true,"ipv4":[%q]}`, cidr(121, 1))})
	stA = step("vrx_config", stA, map[string]any{"pointer": ptrA,
		"value": fmt.Sprintf(`{"enabled":true,"ipv4":[%q]}`, cidr(121, 2))})

	// 2. data "vrx_state" sees it (agent Retrieve) and VPP has the address
	ds, err := h.ReadData("vrx_state", map[string]any{"path": "interfaces"})
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Items []struct {
			Name   string         `json:"name"`
			Config map[string]any `json:"config"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(tfharness.Attr(ds, "json").(string)), &st)
	found := false
	for _, it := range st.Items {
		if it.Name == ifA {
			found = true
			say("data.vrx_state interfaces: %s config=%v", it.Name, it.Config)
			if fmt.Sprint(it.Config["ipv4"]) != fmt.Sprint([]any{cidr(121, 2)}) {
				t.Fatalf("state ipv4 = %v", it.Config["ipv4"])
			}
		}
	}
	if !found {
		t.Fatalf("%s not in /state/interfaces", ifA)
	}
	say("vppctl show int addr: %s", vppAddr(ifA))

	// 3. import by pointer into a fresh state = what `terraform import vrx_config.x <pointer>` does
	imp, err := h.Import("vrx_config", ptrA)
	if err != nil {
		t.Fatal(err)
	}
	say("import %s → value=%s", ptrA, tfharness.Attr(imp, "value"))

	// 4. vrx_interface (typed, generated from the interfaces JSON Schema)
	stB := step("vrx_interface", h.Null("vrx_interface"), map[string]any{"name": ifB, "enabled": true,
		"ipv4": []any{cidr(122, 1)}, "mtu": 1500, "description": "terraform " + prefix})
	stB = step("vrx_interface", stB, map[string]any{"name": ifB, "enabled": true,
		"ipv4": []any{cidr(122, 1)}, "mtu": 9000, "description": "terraform " + prefix})
	say("vppctl show int addr: %s", vppAddr(ifB))

	// 5. secrets: a user's passwordHash only through the write-only sensitive_value
	hashAdmin, hash := randomPHC(), randomPHC()
	users, err := h.Import("vrx_config", "/management/users")
	if err != nil {
		t.Fatal(err)
	}
	usersCfg := map[string]any{"pointer": "/management/users",
		// the API's semantic rule: at least one enabled admin with a password must remain in management.users
		"value":                   fmt.Sprintf(`[{"username":"admin","role":"admin"},{"username":%q,"role":"operator"}]`, user),
		// matched by username (keyed array), never by position
		"sensitive_value":         fmt.Sprintf(`[{"username":%q,"passwordHash":%q},{"username":"admin","passwordHash":%q}]`, user, hash, hashAdmin),
		"sensitive_value_version": 1}
	users = step("vrx_config", users, usersCfg)
	cfgNoHash, _ := h.Config("vrx_config", map[string]any{"pointer": "/management/users", "value": fmt.Sprintf(`[{"username":%q,"role":"operator","passwordHash":%q}]`, user, hash)})
	verr := h.Validate("vrx_config", cfgNoHash)
	say("hash inside value → validation: %v", firstLine(verr))
	if verr == nil {
		t.Fatal("a write-only member in value was accepted")
	}
	if strings.Contains(users.String(), hash) || strings.Contains(users.String(), hashAdmin) || strings.Contains(verr.Error(), hash) {
		t.Fatal("the password hash reached the state or a diagnostic")
	}
	say("state after apply: sensitive_value=%v (write-only), hash in state: %v", tfharness.Attr(users, "sensitive_value"), strings.Contains(users.String(), hash))
	say("app_user %s has a password hash: %s", user, appUserHasHash(prefix, user))
	// remove the test user again (admin keeps its hash: an omitted passwordHash is "unchanged", D-046)
	users = step("vrx_config", users, map[string]any{"pointer": "/management/users", "value": `[{"username":"admin","role":"admin"}]`, "sensitive_value_version": 1})
	say("app_user %s still present: %s", user, appUserHasHash(prefix, user))

	// 6. destroy everything
	step("vrx_interface", stB, nil)
	step("vrx_config", stA, nil)
	_ = users
	say("after destroy: %s %s", vppAddr(ifA), vppAddr(ifB))
}

func vppAddr(ifname string) string {
	out, err := exec.Command("vppctl", "show", "int", "addr").Output()
	if err != nil {
		return "(vppctl unavailable)"
	}
	lines := strings.Split(string(out), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, ifname+" ") {
			addr := ""
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") {
				addr = strings.TrimSpace(lines[i+1])
			}
			return fmt.Sprintf("%s %s", strings.TrimSpace(l), addr)
		}
	}
	return "(no " + ifname + " in vppctl show int addr)"
}

// randomPHC is a syntactically valid argon2id PHC string with random salt and digest — generated per run, never
// written anywhere; it is not the hash of any password.
func randomPHC() string {
	b := make([]byte, 48)
	_, _ = rand.Read(b)
	enc := base64.RawStdEncoding
	return "$argon2id$v=19$m=65536,t=3,p=4$" + enc.EncodeToString(b[:16]) + "$" + enc.EncodeToString(b[16:])
}

// appUserHasHash asks PostgreSQL (the slot database, credentials from /run) whether the user got a hash — prints
// only a boolean.
func appUserHasHash(prefix, user string) string {
	env, err := os.ReadFile("/run/vrx-test/" + prefix + "/pg.env")
	if err != nil {
		return "(no pg.env)"
	}
	dsn := ""
	for _, l := range strings.Split(string(env), "\n") {
		if strings.HasPrefix(l, "VRX_PG_DSN=") {
			dsn = strings.TrimPrefix(l, "VRX_PG_DSN=")
		}
	}
	out, err := exec.Command("psql", dsn, "-XAtc",
		"select coalesce(bool_or(password_hash is not null and password_hash <> ''), false) from app_user where username = '"+user+"'").CombinedOutput()
	if err != nil {
		return "(psql: " + firstLine(err) + ")"
	}
	return strings.TrimSpace(string(out))
}

func firstLine(err error) string {
	if err == nil {
		return "<nil>"
	}
	return strings.SplitN(err.Error(), "\n", 2)[0]
}
