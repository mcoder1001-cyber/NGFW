package e2e

// P13 §6 e2e against the dev stack on the worker slot (apps/cli/test/devstack.sh: real vrx-api + real vrx-agent +
// the host VPP): log in through the REPL, Tab/`?` completion from the live schema, set MTU (+ an address the agent
// does apply) → `commit confirm 5` → observe the automatic revert, then an RBAC denial as readonly.
//
//	eval "$(tools/lab env 3)"; tools/lab lock shared apps/cli/test/devstack.sh start
//	VRX_INTEGRATION=1 go -C apps/cli test -count=1 -v ./test/e2e/
//	apps/cli/test/devstack.sh stop
//
// Objects: loop<slot>01 and 10.<slot>.101.0/24 only (shared-host rules); removed again at the end.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

type stack struct {
	api, pwFile, iface, net string
	bin, dir                string
}

func setup(t *testing.T) *stack {
	t.Helper()
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("e2e: VRX_INTEGRATION is not 1 (needs the slot dev stack: apps/cli/test/devstack.sh start)")
	}
	prefix := env("VRX_TEST_PREFIX", "w3")
	slot := strings.TrimPrefix(prefix, "w")
	s := &stack{
		api:    env("VRX_E2E_API", "http://127.0.0.1:"+env("VRX_HTTP_PORT", "3300")),
		pwFile: env("VRX_E2E_ADMIN_PASSWORD_FILE", "/run/vrx-test/"+prefix+"/admin.pw"),
		iface:  "loop" + slot + "01",
		net:    "10." + slot + ".101.",
		dir:    t.TempDir(),
	}
	if _, err := os.Stat(s.pwFile); err != nil {
		t.Skipf("e2e: no admin password file %s (start the dev stack first)", s.pwFile)
	}
	// session and history must live in a 0700 directory (review M1); t.TempDir() is 0755
	if err := os.Mkdir(filepath.Join(s.dir, "private"), 0o700); err != nil {
		t.Fatal(err)
	}
	s.bin = filepath.Join(s.dir, "vrx")
	build := exec.Command("go", "build", "-o", s.bin, "../../cmd/vrx")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return s
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (s *stack) env() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + s.dir, "TERM=xterm",
		"VRX_API_URL=" + s.api,
		"VRX_SESSION_FILE=" + filepath.Join(s.dir, "private", "session.json"),
		"VRX_HISTORY_FILE=" + filepath.Join(s.dir, "private", "history"),
	}
}

// oneShot runs `vrx args…` without a terminal and returns stdout, stderr and the exit code.
func (s *stack) oneShot(t *testing.T, extraEnv []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(s.bin, args...)
	cmd.Env = append(s.env(), extraEnv...)
	var so, se strings.Builder
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run vrx: %v", err)
		}
	}
	return so.String(), se.String(), code
}

func TestREPLConfirmedCommitAutoRevertAndRBAC(t *testing.T) {
	s := setup(t)
	pw, err := os.ReadFile(s.pwFile)
	if err != nil {
		t.Fatal(err)
	}
	password := strings.TrimSpace(string(pw))
	const long = 30 * time.Second
	If, a1, a9 := s.iface, s.net+"1/24", s.net+"9/24"

	r := spawn(t, s.env(), s.bin)
	r.expect(`Username: `, long)
	r.line("admin")
	r.expect(`Password: `, long)
	r.line(password)
	r.expect(`admin@\S+> `, long)

	// operational → configuration mode; Tab completes command words
	r.prompt()
	r.send("confi\tg\t") // "conf" is ambiguous (configure, confirm): Tab completes the common prefix "confi"
	r.expect(`configure `, long)
	r.line("")
	r.expect(`\[edit\]\s*admin@\S+# `, long)

	// baseline owned by this test: the interface with address .1 and MTU 1500
	r.command(fmt.Sprintf(`set interfaces %s ipv4 ["%s"]`, If, a1))
	r.command(fmt.Sprintf("set interfaces %s mtu 1500", If))
	r.command(`commit comment "cli e2e baseline"`)
	r.expect(`(applied|unchanged)[^\n]*`, long)

	// completion from the live JSON Schema: a config path element, then enum values with `?`
	r.prompt()
	r.send(fmt.Sprintf("set interfaces %s rxM\t", If))
	r.expect(`rxMode `, long)
	r.send("?")
	r.expect(`polling[\s\S]*interrupt[\s\S]*adaptive`, long) // the enum of rxMode, in schema order
	r.send("\x15")                                           // Ctrl-U: clear the line
	r.send("set interfaces " + If + " mt?")
	r.expect(`mtu\s+MTU — L3 MTU in bytes[^\n]*integer 68\.\.9216`, long)
	r.send("\x15")
	// a value the schema rejects never reaches the API (exit code 2 = usage)
	r.command(fmt.Sprintf("set interfaces %s mtu 70000", If))
	r.expect(`error: interfaces \S+ mtu: must be ≤ 9216`, long)

	// the change under test: MTU 9000 and address .1 → .9, committed with a 5-second confirm window
	r.command(fmt.Sprintf("set interfaces %s mtu 9000", If))
	r.command(fmt.Sprintf("delete interfaces %s ipv4 %s", If, a1))
	r.command(fmt.Sprintf("set interfaces %s ipv4 %s", If, a9))
	r.command("compare")
	r.expect(`\+ set interfaces \S+ mtu 9000`, long)
	r.command(`commit confirm 5 comment "cli e2e: mtu 9000, auto-revert"`)
	r.expect(`NOT confirmed: it reverts automatically`, long)
	r.command("run show commit pending")
	r.expect(`waiting for .confirm. until`, long)
	r.command("run show interfaces " + If)
	r.expect(regexp.QuoteMeta(a9), long) // applied in the data plane (agent Retrieve)

	time.Sleep(8 * time.Second) // deadline 5 s + the API's Health detection

	r.command("run show commit pending")
	r.expect(`no commit is waiting for confirmation`, long)
	r.command(fmt.Sprintf("run show configuration interfaces %s mtu", If))
	r.expect(`\n1500\r?\n`, long)
	r.command("run show interfaces " + If)
	r.expect(regexp.QuoteMeta(a1), long) // the data plane is back on the old address
	r.command("run show system")
	r.expect(`sync\s+in-sync`, long)
	r.command("discard")
	r.expect(`candidate discarded`, long)
	r.command("exit")
	r.expect(`admin@\S+> `, long)

	// a readonly API key for the RBAC check, written to a 0600 file (never printed)
	keyFile := filepath.Join(s.dir, "ro.key")
	r.command("api-key create cli-e2e-ro role readonly file " + keyFile)
	r.expect(`written to \S+ \(mode 0600\)`, long)
	r.command("exit")
	if code := r.wait(long); code != 0 {
		t.Fatalf("REPL exit code %d", code)
	}
	tr := r.transcript()
	if strings.Contains(tr, password) {
		t.Fatal("the password appeared on the terminal")
	}
	if p := os.Getenv("VRX_E2E_TRANSCRIPT"); p != "" {
		_ = os.WriteFile(p, []byte(tr+"\n"), 0o600)
	}

	// RBAC: readonly may read but not change the candidate (403 → exit 4), human and --json
	ro := []string{"VRX_API_KEY_FILE=" + keyFile}
	out, _, code := s.oneShot(t, ro, "show", "whoami")
	if code != 0 || !strings.Contains(out, "effective readonly") {
		t.Fatalf("readonly whoami: code %d, %q", code, out)
	}
	_, errOut, code := s.oneShot(t, ro, "set", "interfaces", If, "mtu", "1400")
	if code != 4 || !strings.Contains(errOut, "403") {
		t.Fatalf("readonly set: want exit 4 with 403, got %d %q", code, errOut)
	}
	_, errOut, code = s.oneShot(t, ro, "--json", "commit")
	var doc struct {
		Error struct {
			ExitCode int `json:"exitCode"`
			Status   int `json:"status"`
		} `json:"error"`
	}
	if code != 4 || json.Unmarshal([]byte(errOut), &doc) != nil || doc.Error.Status != 403 {
		t.Fatalf("readonly --json commit: code %d, %q", code, errOut)
	}
	if strings.Contains(errOut, "vrxk_") {
		t.Fatal("an API key appeared in an error message")
	}

	// cleanup with the admin session the REPL saved: revoke the key, remove the interface
	var keys []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	out, _, _ = s.oneShot(t, nil, "--json", "api-key", "list")
	_ = json.Unmarshal([]byte(out), &keys)
	for _, k := range keys {
		if k.Name == "cli-e2e-ro" {
			if _, e, c := s.oneShot(t, nil, "api-key", "delete", k.ID); c != 0 {
				t.Errorf("api-key delete: %d %s", c, e)
			}
		}
	}
	if _, e, c := s.oneShot(t, nil, "delete", "interfaces", If); c != 0 {
		t.Fatalf("cleanup delete: %d %s", c, e)
	}
	if _, e, c := s.oneShot(t, nil, "commit", "comment", "cli e2e cleanup"); c != 0 {
		t.Fatalf("cleanup commit: %d %s", c, e)
	}
	out, _, _ = s.oneShot(t, nil, "show", "interfaces")
	if strings.Contains(out, If) {
		t.Fatalf("%s still in the data plane after cleanup:\n%s", If, out)
	}
}
