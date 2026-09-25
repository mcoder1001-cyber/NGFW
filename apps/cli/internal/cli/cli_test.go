package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"ngfw/cli/internal/api"
)

// fakeAPI is a scripted vrx-api: it serves the testdata OpenAPI document at /api/docs-json, a candidate document,
// and records every request.
type fakeAPI struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	reqs      []string // "METHOD escaped-path?query body"
	auth      []string
	candidate map[string]any
	override  map[string]func(w http.ResponseWriter, r *http.Request)
}

func newFake(t *testing.T) *fakeAPI {
	t.Helper()
	spec, err := os.ReadFile("../testdata/openapi-min.json")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAPI{t: t, override: map[string]func(http.ResponseWriter, *http.Request){}}
	f.candidate = map[string]any{"interfaces": map[string]any{"eth0": map[string]any{"enabled": true, "ipv4": []any{"10.0.0.1/24"}}}, "system": map[string]any{"hostname": "vrx"}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.EscapedPath()
		f.mu.Lock()
		line := key
		if r.URL.RawQuery != "" {
			line += "?" + r.URL.RawQuery
		}
		if len(body) > 0 {
			line += " " + string(body)
		}
		f.reqs = append(f.reqs, line)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		h := f.override[key]
		f.mu.Unlock()
		if h != nil {
			h(w, r)
			return
		}
		w.Header().Set("content-type", "application/json")
		switch {
		case key == "GET /api/docs-json":
			_, _ = w.Write(spec)
		case key == "GET /api/v1/config/candidate":
			_ = json.NewEncoder(w).Encode(f.candidate)
		case strings.HasPrefix(key, "GET /api/v1/config/candidate/"):
			v := valueAt(f.candidate, strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/config/candidate/"), "/"))
			if v == nil {
				problem(w, 404, "nothing there")
				return
			}
			_ = json.NewEncoder(w).Encode(v)
		case r.Method == "PUT" || r.Method == "PATCH" || r.Method == "DELETE":
			_, _ = w.Write([]byte(`{"pointer":"/x","before":null,"after":null}`))
		case key == "POST /api/v1/config/commit":
			if r.URL.Query().Get("confirm") != "" {
				_, _ = w.Write([]byte(`{"status":"pending","txnId":"t1","confirmDeadline":"2026-01-01T00:00:05Z","results":[],"warnings":[],"notApplied":[],"sync":{"state":"in-sync","reason":"","txnId":null,"since":"x"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"applied","txnId":"t1","revision":{"id":7,"createdAt":"x","authorId":1,"author":"admin","comment":"c","parentId":6,"hash":"abc","txnId":"t1","kind":"commit"},"results":[{"key":"interface.loopback/eth0","op":"create","code":"ok","message":"","pointer":"/interfaces/eth0","subsystem":"interfaces"}],"warnings":[{"pointer":"/interfaces/eth0/mtu","message":"interfaces.mtu is not implemented by this agent build (DF-1/P08)"}],"notApplied":[],"sync":{"state":"in-sync","reason":"","txnId":null,"since":"x"}}`))
		case key == "GET /api/v1/state/system":
			_, _ = w.Write([]byte(`{"api":{"version":"1","startedAt":"s","wsClients":0},"agent":{"reachable":false,"error":"down"},"runningRevision":3,"pendingCommit":null,"sync":{"state":"unknown","reason":"apply answer lost","txnId":"t9","since":"s"}}`))
		case key == "POST /api/v1/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "vrx_refresh", Value: "r1", Path: "/api/v1/auth"})
			_, _ = w.Write([]byte(`{"accessToken":"tok-123","tokenType":"Bearer","expiresIn":900,"user":{"id":1,"username":"admin","role":"admin"}}`))
		default:
			problem(w, 404, "no fake for "+key)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func problem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("content-type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "title": http.StatusText(status), "status": status, "detail": detail})
}

func (f *fakeAPI) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.reqs...)
}

func (f *fakeAPI) mutations() []string {
	var out []string
	for _, r := range f.requests() {
		if !strings.HasPrefix(r, "GET ") {
			out = append(out, r)
		}
	}
	return out
}

type run struct {
	code           int
	stdout, stderr string
}

func (f *fakeAPI) vrx(t *testing.T, env map[string]string, stdin string, args ...string) run {
	t.Helper()
	in, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = in.WriteString(stdin)
	_, _ = in.Seek(0, 0)
	defer func() { _ = in.Close() }()
	var so, se bytes.Buffer
	e := map[string]string{"VRX_API_URL": f.srv.URL, "VRX_API_KEY": "vrxk_test", "VRX_SESSION_FILE": filepath.Join(t.TempDir(), "s.json")}
	for k, v := range env {
		e[k] = v
	}
	a := &App{Stdin: in, Stdout: &so, Stderr: &se, Getenv: func(k string) string { return e[k] }}
	code := a.Main(args)
	return run{code, so.String(), se.String()}
}

func TestSetSendsPUTAtTheEscapedPointer(t *testing.T) {
	f := newFake(t)
	r := f.vrx(t, nil, "", "set", "interfaces", "TenGigabitEthernet0/0/0", "mtu", "9000")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	want := []string{"PUT /api/v1/config/interfaces/TenGigabitEthernet0~10~10/mtu 9000"}
	if got := f.mutations(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests %q, want %q", got, want)
	}
	for _, a := range f.auth {
		if a != "ApiKey vrxk_test" {
			t.Errorf("Authorization %q", a)
		}
	}
	// the pointer form is equivalent, strings stay strings, booleans are typed
	f.vrx(t, nil, "", "set", "/interfaces/eth0/description", "1500")
	f.vrx(t, nil, "", "set", "interfaces", "eth0", "enabled", "false")
	got := f.mutations()
	if got[1] != `PUT /api/v1/config/interfaces/eth0/description "1500"` || got[2] != "PUT /api/v1/config/interfaces/eth0/enabled false" {
		t.Errorf("typed values: %q", got)
	}
}

func TestClientSideValidationSendsNothing(t *testing.T) {
	f := newFake(t)
	for _, args := range [][]string{
		{"set", "interfaces", "eth0", "mtu", "70000"},
		{"set", "interfaces", "eth0", "rxMode", "turbo"},
		{"set", "interfaces", "eth0", "speed", "10g"},
		{"set", "interfaces", "9bad", "mtu", "1500"},
		{"set", "interfaces", "eth0", "ipv4", "10.0.0.300/24"},
		{"merge", "interfaces", "eth0", `{"mtu": 10}`},
		{"commit", "confirm", "0"},
		{"rollback", "abc"},
	} {
		r := f.vrx(t, nil, "", args...)
		if r.code != ExitUsage {
			t.Errorf("%q: exit %d, want %d (%s)", args, r.code, ExitUsage, r.stderr)
		}
	}
	if m := f.mutations(); len(m) != 0 {
		t.Errorf("rejected values reached the API: %q", m)
	}
}

func TestLeafListAppendAndDeleteItem(t *testing.T) {
	f := newFake(t)
	if r := f.vrx(t, nil, "", "set", "interfaces", "eth0", "ipv4", "10.0.0.2/24"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if r := f.vrx(t, nil, "", "set", "interfaces", "eth0", "ipv4", "10.0.0.1/24"); r.code != 0 || !strings.Contains(r.stdout, "already contains") {
		t.Fatalf("duplicate append: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if r := f.vrx(t, nil, "", "delete", "interfaces", "eth0", "ipv4", "10.0.0.1/24"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if r := f.vrx(t, nil, "", "delete", "interfaces", "eth0", "ipv4", "10.9.9.9/24"); r.code != ExitNotFound {
		t.Fatalf("absent item: exit %d", r.code)
	}
	want := []string{`PUT /api/v1/config/interfaces/eth0/ipv4 ["10.0.0.1/24","10.0.0.2/24"]`, "DELETE /api/v1/config/interfaces/eth0/ipv4/0"}
	if got := f.mutations(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests %q, want %q", got, want)
	}
}

func TestMergeAndDelete(t *testing.T) {
	f := newFake(t)
	if r := f.vrx(t, nil, "", "merge", "interfaces", "eth0", `{"description":"uplink","ipv4":null}`); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if r := f.vrx(t, nil, "", "delete", "interfaces", "eth0"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	want := []string{`PATCH /api/v1/config/interfaces/eth0 {"description":"uplink","ipv4":null}`, "DELETE /api/v1/config/interfaces/eth0"}
	if got := f.mutations(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests %q, want %q", got, want)
	}
}

func TestCommitConfirmCommentAndOutput(t *testing.T) {
	f := newFake(t)
	r := f.vrx(t, nil, "", "commit", "confirm", "5", "comment", "mtu 9000")
	if r.code != 0 || !strings.Contains(r.stdout, "NOT confirmed") {
		t.Fatalf("exit %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if got := f.mutations(); len(got) != 1 || got[0] != "POST /api/v1/config/commit?comment=mtu+9000&confirm=5" {
		t.Errorf("requests %q", got)
	}
	r = f.vrx(t, nil, "", "commit")
	for _, want := range []string{"applied — revision 7", "create interface.loopback/eth0: ok", "stored but not enforced by this agent build (1): /interfaces/eth0/mtu", "sync: in-sync"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("commit output lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestExitCodesFollowHTTPStatus(t *testing.T) {
	f := newFake(t)
	for status, code := range map[int]int{400: ExitInvalid, 401: ExitAuth, 403: ExitForbidden, 404: ExitNotFound, 409: ExitConflict, 422: ExitCommitFailed, 429: ExitRateLimited, 501: ExitNotImplemented, 502: ExitUnavailable, 503: ExitUnavailable, 504: ExitUnavailable} {
		st := status
		f.override["POST /api/v1/config/commit/confirm"] = func(w http.ResponseWriter, _ *http.Request) { problem(w, st, "x") }
		if r := f.vrx(t, nil, "", "confirm"); r.code != code {
			t.Errorf("HTTP %d: exit %d, want %d", status, r.code, code)
		}
	}
	if r := f.vrx(t, map[string]string{"VRX_API_URL": "http://127.0.0.1:1"}, "", "show", "system"); r.code != ExitUnavailable {
		t.Errorf("unreachable API: exit %d", r.code)
	}
	if r := f.vrx(t, map[string]string{"VRX_API_KEY": ""}, "", "show", "system"); r.code != ExitAuth || !strings.Contains(r.stderr, "not logged in") {
		t.Errorf("no credentials: exit %d %s", r.code, r.stderr)
	}
	if r := f.vrx(t, nil, "", "show", "bgp", "summary"); r.code != ExitNotImplemented {
		t.Errorf("show bgp summary: exit %d", r.code)
	}
	if r := f.vrx(t, nil, "", "shwo", "system"); r.code != ExitUsage || !strings.Contains(r.stderr, "unknown command") {
		t.Errorf("typo: exit %d %s", r.code, r.stderr)
	}
}

func TestJSONMode(t *testing.T) {
	f := newFake(t)
	r := f.vrx(t, nil, "", "--json", "show", "system")
	var doc map[string]any
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &doc) != nil || doc["sync"].(map[string]any)["state"] != "unknown" {
		t.Fatalf("--json show system: %d %q", r.code, r.stdout)
	}
	f.override["POST /api/v1/config/commit"] = func(w http.ResponseWriter, _ *http.Request) { problem(w, 403, "role 'readonly' may not do this") }
	r = f.vrx(t, nil, "", "--json", "commit")
	var e struct {
		Error struct {
			ExitCode int            `json:"exitCode"`
			Status   int            `json:"status"`
			Problem  map[string]any `json:"problem"`
		} `json:"error"`
	}
	if r.code != ExitForbidden || r.stdout != "" || json.Unmarshal([]byte(r.stderr), &e) != nil || e.Error.ExitCode != 4 || e.Error.Status != 403 || e.Error.Problem["detail"] == nil {
		t.Fatalf("--json error: %d stdout=%q stderr=%q", r.code, r.stdout, r.stderr)
	}
	// human output of an unknown sync state is explicit, never "unchanged"
	r = f.vrx(t, nil, "", "show", "system")
	if !strings.Contains(r.stdout, "sync       unknown — apply answer lost") || !strings.Contains(r.stdout, "agent      UNREACHABLE") {
		t.Errorf("show system:\n%s", r.stdout)
	}
}

func TestLoginWithPasswordFileNeverPrintsSecrets(t *testing.T) {
	f := newFake(t)
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw")
	secret := "s3cret-" + strings.Repeat("x", 12)
	if err := os.WriteFile(pw, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sess := filepath.Join(dir, "sess", "session.json")
	env := map[string]string{"VRX_API_KEY": "", "VRX_SESSION_FILE": sess}
	r := f.vrx(t, env, "", "--password-file", pw, "login", "admin")
	if r.code != 0 {
		t.Fatalf("login: %d %s", r.code, r.stderr)
	}
	if strings.Contains(r.stdout+r.stderr, secret) || strings.Contains(r.stdout+r.stderr, "tok-123") {
		t.Errorf("secret on the terminal: %q %q", r.stdout, r.stderr)
	}
	reqs := f.requests()
	if len(reqs) != 1 || !strings.Contains(reqs[0], `"password":"`+secret+`"`) {
		t.Errorf("login request %q", reqs)
	}
	st, err := os.Stat(sess)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("session file: %v %v", st, err)
	}
	// the session is used by the next invocation
	f.vrx(t, env, "", "show", "system")
	if f.auth[len(f.auth)-1] != "Bearer tok-123" {
		t.Errorf("session not used: %q", f.auth)
	}
	// a world-readable password file is refused before anything is sent
	loose := filepath.Join(dir, "loose")
	_ = os.WriteFile(loose, []byte(secret), 0o600)
	_ = os.Chmod(loose, 0o644)
	if r := f.vrx(t, env, "", "--password-file", loose, "login", "admin"); r.code != ExitUsage || !strings.Contains(r.stderr, "chmod 600") {
		t.Errorf("loose password file: %d %s", r.code, r.stderr)
	}
	if strings.Contains(r.stderr, secret) {
		t.Error("the password leaked into an error")
	}
}

// TD-4 (D-100 (2)): api-key create from a login session sends the current password (--password-file, or a prompt
// without echo on a terminal); without a terminal it stops before any request; an API-key credential sends none.
func TestAPIKeyCreateStepUp(t *testing.T) {
	f := newFake(t)
	f.override["POST /api/v1/auth/api-keys"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"k1","name":"ci","role":"operator","expiresAt":null,"key":"vrxk_` + strings.Repeat("A", 43) + `"}`))
	}
	// the bodies of the key requests, as the fake recorded them (it has read the request body before the override)
	sent := func() []map[string]any {
		var out []map[string]any
		for _, line := range f.requests() {
			if rest, ok := strings.CutPrefix(line, "POST /api/v1/auth/api-keys "); ok {
				var b map[string]any
				if err := json.Unmarshal([]byte(rest), &b); err != nil {
					t.Fatalf("key request body %q: %v", rest, err)
				}
				out = append(out, b)
			}
		}
		return out
	}
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw")
	secret := "s3cret-" + strings.Repeat("y", 12)
	if err := os.WriteFile(pw, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// an API key: no password, no prompt
	if r := f.vrx(t, nil, "", "api-key", "create", "ci"); r.code != 0 {
		t.Fatalf("with an API key: %d %s", r.code, r.stderr)
	}
	if bodies := sent(); len(bodies) != 1 || bodies[0]["current"] != nil {
		t.Fatalf("API-key caller sent %v", bodies)
	}
	// a login session with --password-file: the current password goes in the body, never on the terminal
	env := map[string]string{"VRX_API_KEY": "", "VRX_SESSION_FILE": filepath.Join(dir, "sess", "session.json")}
	if r := f.vrx(t, env, "", "--password-file", pw, "login", "admin"); r.code != 0 {
		t.Fatalf("login: %d %s", r.code, r.stderr)
	}
	r := f.vrx(t, env, "", "--password-file", pw, "api-key", "create", "ci", "role", "operator")
	if r.code != 0 {
		t.Fatalf("with a session and --password-file: %d %s", r.code, r.stderr)
	}
	if bodies := sent(); len(bodies) != 2 || bodies[1]["current"] != secret || bodies[1]["role"] != "operator" {
		t.Errorf("session caller sent %v", bodies)
	}
	if f.auth[len(f.auth)-1] != "Bearer tok-123" {
		t.Errorf("the session was not used: %q", f.auth[len(f.auth)-1])
	}
	if strings.Contains(r.stdout+r.stderr, secret) {
		t.Error("the password reached the terminal")
	}
	// a login session, no terminal, no --password-file: a usage error naming the ways out, nothing sent, no key file
	keyFile := filepath.Join(dir, "k")
	r = f.vrx(t, env, "", "api-key", "create", "ci", "file", keyFile)
	if r.code != ExitUsage {
		t.Fatalf("no terminal: exit %d (%s)", r.code, r.stderr)
	}
	for _, want := range []string{"terminal", "--password-file", "API key"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("no-terminal error does not mention %q: %s", want, r.stderr)
		}
	}
	if bodies := sent(); len(bodies) != 2 {
		t.Errorf("a request was sent without the password: %v", bodies)
	}
	if _, err := os.Stat(keyFile); !os.IsNotExist(err) {
		t.Errorf("key file created although nothing was minted: %v", err)
	}
}

func TestScriptOnStdinStopsAtFirstError(t *testing.T) {
	f := newFake(t)
	r := f.vrx(t, nil, "set interfaces eth0 mtu 9000\n# a comment\nset interfaces eth0 mtu 1\ncommit\n")
	if r.code != ExitUsage {
		t.Fatalf("script: exit %d (%s)", r.code, r.stderr)
	}
	for _, m := range f.mutations() {
		if strings.Contains(m, "commit") {
			t.Errorf("the script went on after a failure: %q", f.mutations())
		}
	}
}

func TestEveryCommandMapsToDocumentedREST(t *testing.T) {
	for _, c := range Commands() {
		if len(c.Ops) == 0 && c.NoREST == "" {
			t.Errorf("%s: neither REST operations nor a reason for having none", c.Name())
		}
		for _, id := range c.Ops {
			if _, ok := api.Operations[id]; !ok {
				t.Errorf("%s: operation %s is not in the OpenAPI document", c.Name(), id)
			}
		}
	}
}

func TestReferenceDocIsCurrent(t *testing.T) {
	b, err := os.ReadFile("../../../../docs/user/cli/reference.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != Markdown() {
		t.Fatal("docs/user/cli/reference.md is stale: run `make -C apps/cli docs`")
	}
}

func TestCompletionFromTheLiveSchema(t *testing.T) {
	f := newFake(t)
	a := &App{Stdin: os.Stdin, Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }}
	c, _ := api.New(f.srv.URL)
	c.Cred = api.Key("vrxk_test")
	a.client = c
	texts := func(line string) []string {
		cs, _ := a.candidates(context.Background(), line)
		out := make([]string, len(cs))
		for i, x := range cs {
			out[i] = x.Text
		}
		return out
	}
	has := func(line string, want ...string) {
		t.Helper()
		got := texts(line)
		for _, w := range want {
			found := false
			for _, g := range got {
				if g == w {
					found = true
				}
			}
			if !found {
				t.Errorf("%q: %q not in %q", line, w, got)
			}
		}
	}
	has("sh", "show")
	has("show conf", "configuration")
	has("show configuration ", "interfaces", "system", "candidate", "diff", "json", "set")
	a.mode = ModeConfig
	has("set ", "interfaces", "system")
	has("set interfaces ", "eth0", "<interface>") // existing key from the candidate + a hint for a new one
	has("set interfaces eth0 ", "enabled", "mtu", "rxMode", "ipv4")
	has("set interfaces eth0 rxMode ", "polling", "interrupt", "adaptive")
	has("set interfaces eth0 enabled ", "true", "false")
	has("set interfaces eth0 mtu ", "<integer68..9216>")
	has("delete interfaces eth0 ipv4 ", "10.0.0.1/24")
	has("commit ", "confirm", "comment")
	has("run sh", "show")
	if got := texts("set interfaces eth0 mtu 9000 "); len(got) != 0 {
		t.Errorf("nothing follows a value: %q", got)
	}
	// Tab never inserts a <placeholder>
	cs, _ := a.complete("set interfaces eth0 mtu ")
	if len(cs) != 0 {
		t.Errorf("Tab candidates %v", cs)
	}
	if h := a.help("set interfaces eth0 mt"); !strings.Contains(h, "L3 MTU in bytes") {
		t.Errorf("? help: %q", h)
	}
}

// Interactive sessions keep the refresh cookie in memory and renew the access token once on a 401.
func TestInteractiveRefreshOn401(t *testing.T) {
	f := newFake(t)
	var cookies []string
	me := 0
	f.override["POST /api/v1/auth/refresh"] = func(w http.ResponseWriter, r *http.Request) {
		cookies = append(cookies, r.Header.Get("Cookie"))
		http.SetCookie(w, &http.Cookie{Name: "vrx_refresh", Value: "r2", Path: "/api/v1/auth"})
		_, _ = w.Write([]byte(`{"accessToken":"tok-456","tokenType":"Bearer","expiresIn":900,"user":{"id":1,"username":"admin","role":"admin"}}`))
	}
	f.override["GET /api/v1/auth/me"] = func(w http.ResponseWriter, r *http.Request) {
		me++
		if r.Header.Get("Authorization") != "Bearer tok-456" {
			problem(w, 401, "expired")
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"username":"admin","role":"admin","lastLogin":null,"effectiveRole":"admin","via":"jwt"}`))
	}
	a := &App{Stdin: os.Stdin, Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }, interactive: true, noSession: true}
	a.client, _ = api.New(f.srv.URL)
	if err := a.login(context.Background(), "admin", "pw", false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.call(context.Background(), api.Call{Op: "Auth_me"}, nil); err != nil {
		t.Fatalf("after refresh: %v", err)
	}
	if me != 2 || len(cookies) != 1 || cookies[0] != "vrx_refresh=r1" || a.refreshCookie != "vrx_refresh=r2" {
		t.Errorf("me calls %d, refresh cookies %q, kept %q", me, cookies, a.refreshCookie)
	}
}

// review H1: nothing the server sends can put a control sequence on the terminal.
func TestServerStringsCannotDriveTheTerminal(t *testing.T) {
	f := newFake(t)
	evil := "cli review\x1b]0;PWNED\x07\x1b[2K\rinnocuous\x9b31m" + string(rune(0x202e)) + "x"
	f.candidate["interfaces"] = map[string]any{"eth0": map[string]any{"description": evil}}
	f.override["GET /api/v1/config/revisions"] = func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "items": []any{map[string]any{"id": 1, "createdAt": "x", "authorId": 1, "author": evil, "comment": evil, "parentId": nil, "hash": "abc", "txnId": "t", "kind": "commit"}}})
	}
	f.override["POST /api/v1/config/validate"] = func(w http.ResponseWriter, _ *http.Request) { problem(w, 400, evil) }
	check := func(name string, r run) {
		t.Helper()
		out := r.stdout + r.stderr
		for _, bad := range []string{"\x1b", "\x07", "\r", "\xc2\x9b", string(rune(0x202e))} {
			if strings.Contains(out, bad) {
				t.Errorf("%s: raw %q reached the terminal:\n%q", name, bad, out)
			}
		}
		if !strings.Contains(out, `\x1b]0;PWNED\x07\x1b[2K`) {
			t.Errorf("%s: the sequence is not shown visibly: %q", name, out)
		}
	}
	check("show revisions", f.vrx(t, nil, "", "show", "revisions"))
	check("show configuration candidate", f.vrx(t, nil, "", "show", "configuration", "candidate"))
	check("show configuration candidate set", f.vrx(t, nil, "", "show", "configuration", "candidate", "set"))
	check("problem detail", f.vrx(t, nil, "", "validate"))
	// --json passes the API document through (JSON escapes controls itself)
	if r := f.vrx(t, nil, "", "--json", "show", "revisions"); strings.Contains(r.stdout, "\x1b") {
		t.Errorf("--json: raw ESC %q", r.stdout)
	}
}

// review M2: on a list of integers the last word of `delete` is a value, never a position.
func TestDeleteIntegerListByValue(t *testing.T) {
	f := newFake(t)
	f.candidate["dataplane"] = map[string]any{"corelist": []any{5.0, 7.0, 1.0}}
	if r := f.vrx(t, nil, "", "delete", "dataplane", "corelist", "1"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if r := f.vrx(t, nil, "", "delete", "dataplane", "corelist", "index", "0"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if r := f.vrx(t, nil, "", "delete", "dataplane", "corelist", "9"); r.code != ExitNotFound {
		t.Errorf("absent value: exit %d", r.code)
	}
	want := []string{"DELETE /api/v1/config/dataplane/corelist/2", "DELETE /api/v1/config/dataplane/corelist/0"}
	if got := f.mutations(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests %q, want %q (value 1 is at index 2)", got, want)
	}
	if r := f.vrx(t, nil, "", "delete"); r.code != ExitUsage {
		t.Errorf("bare delete: exit %d", r.code)
	}
}

// review M1: the session file never follows a symlink, never keeps a loose mode, never lands in a shared directory.
func TestSessionFileIsPrivate(t *testing.T) {
	f := newFake(t)
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw")
	_ = os.WriteFile(pw, []byte("pw-for-test\n"), 0o600)
	sessDir := filepath.Join(dir, "s")
	_ = os.Mkdir(sessDir, 0o700)
	sess := filepath.Join(sessDir, "session.json")
	victim := filepath.Join(dir, "victim")
	_ = os.WriteFile(victim, []byte("keep"), 0o644)
	_ = os.Symlink(victim, sess) // planted symlink at the final name
	env := map[string]string{"VRX_API_KEY": "", "VRX_SESSION_FILE": sess}
	if r := f.vrx(t, env, "", "--password-file", pw, "login", "admin"); r.code != 0 {
		t.Fatalf("login: %s", r.stderr)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatal("the symlink target was overwritten")
	}
	fi, err := os.Lstat(sess)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("session file: %v %v", fi.Mode(), err)
	}
	// a session file with a loose mode, or a symlink, is not used
	_ = os.Chmod(sess, 0o644)
	if r := f.vrx(t, env, "", "show", "system"); r.code != ExitAuth {
		t.Errorf("0644 session used: exit %d", r.code)
	}
	// a shared (group/other-writable) directory is refused
	shared := filepath.Join(dir, "shared")
	_ = os.Mkdir(shared, 0o777)
	_ = os.Chmod(shared, 0o777)
	env["VRX_SESSION_FILE"] = filepath.Join(shared, "session.json")
	r := f.vrx(t, env, "", "--password-file", pw, "login", "admin")
	if !strings.Contains(r.stderr, "must be 0700") {
		t.Errorf("shared dir: %q", r.stderr)
	}
	// no XDG_RUNTIME_DIR and no /run/user/<uid>: nothing is persisted, the login is refused up front
	a := &App{Getenv: func(string) string { return "" }}
	if p := a.sessionPath(); p != "" && !strings.HasPrefix(p, "/run/user/") {
		t.Errorf("fallback session path %q", p)
	}
}

// review M4: no cleartext credentials to a remote host.
func TestPlainHTTPOnlyToLoopback(t *testing.T) {
	f := newFake(t)
	if r := f.vrx(t, map[string]string{"VRX_API_URL": "http://10.0.0.1:3000"}, "", "show", "system"); r.code != ExitUsage || !strings.Contains(r.stderr, "cleartext") {
		t.Errorf("remote http: %d %s", r.code, r.stderr)
	}
	if r := f.vrx(t, map[string]string{"VRX_API_URL": "http://127.0.0.1:1"}, "", "--insecure-http", "show", "system"); r.code != ExitUnavailable {
		t.Errorf("loopback: %d", r.code)
	}
	for h, want := range map[string]bool{"localhost": true, "127.0.0.9": true, "::1": true, "10.0.0.1": false, "vrx.example": false} {
		if isLoopback(h) != want {
			t.Errorf("isLoopback(%s) != %v", h, want)
		}
	}
}

// review M3: a pending commit is tracked, shown in the prompt and reported when it stops being pending.
func TestPendingCommitIsTracked(t *testing.T) {
	f := newFake(t)
	var so bytes.Buffer
	a := &App{Stdin: os.Stdin, Stdout: &so, Stderr: io.Discard, Getenv: func(string) string { return "" }, username: "adm"}
	a.client, _ = api.New(f.srv.URL)
	a.client.Cred = api.Key("vrxk_test")
	deadline := time.Now().Add(42 * time.Second).UTC().Format(time.RFC3339Nano)
	a.setPending("pending", "d97486aa-1", deadline, "")
	if p := a.prompt(); !strings.Contains(p, "[!4") || !strings.HasSuffix(p, "> ") {
		t.Errorf("prompt %q", p)
	}
	if n := a.pendingNote(); !strings.Contains(n, "commit d97486aa") || !strings.Contains(n, "(in 4") || !strings.Contains(n, "`confirm`") {
		t.Errorf("note %q", n)
	}
	// the API says nothing is pending any more and the newest revision is another txn → reverted
	f.override["GET /api/v1/config/commit/pending"] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"pending":null}`)) }
	f.override["GET /api/v1/config/revisions"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"total":1,"items":[{"id":3,"createdAt":"x","authorId":1,"author":"a","comment":"","parentId":null,"hash":"h","txnId":"other","kind":"commit"}]}`))
	}
	a.refreshPending(context.Background())
	if a.pending != nil || !strings.Contains(so.String(), "NOT confirmed and has been reverted automatically") {
		t.Errorf("revert not reported: %q", so.String())
	}
}

// p08Interfaces is GET /api/v1/state/interfaces as P08's controller builds it: a retrieved interface and
// its sub-interface (config = Retrieve), and three rows the agent did not retrieve (config null): only in
// the candidate, a live interface of another owner, and a configured one VPP does not have.
const p08Interfaces = `{"retrievedAt":"2026-09-24T12:00:00.000Z","countersAt":"2026-09-24T12:00:00.000Z","items":[
 {"name":"host-w1l0","kind":"interface","parent":null,
  "state":{"name":"host-w1l0","vppName":"host-w1l0","swIfIndex":5,"type":"af_packet","adminUp":true,"linkUp":true,"mtu":1400,"linkMtu":1500,"mac":"02:fe:00:00:00:05","ipv4":["10.1.1.1/24"],"ipv6":[],"vrf":"default","tableId":0,"parent":"","vlanId":0,"innerVlanId":0,"managed":true,"linkSpeedKbps":"0","rxMode":"interrupt","description":"lan"},
  "config":{"enabled":true,"promiscuous":false,"mtu":1400,"vrf":"default","ipv4":["10.1.1.1/24"],"description":"lan"},
  "running":{"enabled":true,"mtu":1400,"ipv4":["10.1.1.1/24"],"description":"lan"},
  "counters":{"name":"host-w1l0","swIfIndex":5,"rxPackets":"7","rxBytes":"700","txPackets":"9","txBytes":"900","drops":"0","errors":"0","punts":"0","rxMisses":"0"},
  "hasPendingChange":false},
 {"name":"host-w1l0.100","kind":"subinterface","parent":"host-w1l0","state":null,
  "config":{"vlanId":100,"dot1ad":false,"enabled":true,"vrf":"default","ipv4":["10.1.100.1/24"]},
  "running":{"vlanId":100,"enabled":true,"ipv4":["10.1.100.1/24"]},"counters":null,"hasPendingChange":false},
 {"name":"host-w1w0","kind":"interface","parent":null,"state":null,"config":null,
  "running":{"enabled":true,"mtu":1500,"ipv4":["10.1.2.1/24"]},"counters":null,"hasPendingChange":false},
 {"name":"host-w1w9","kind":"interface","parent":null,"state":null,"config":null,"running":null,"counters":null,"hasPendingChange":true},
 {"name":"host-w3l0","kind":"interface","parent":null,
  "state":{"name":"host-w3l0","vppName":"host-w3l0","swIfIndex":7,"type":"af_packet","adminUp":true,"linkUp":true,"mtu":9000,"linkMtu":9000,"mac":"02:fe:00:00:00:07","ipv4":[],"ipv6":[],"vrf":"","tableId":7,"parent":"","vlanId":0,"innerVlanId":0,"managed":false,"linkSpeedKbps":"0","rxMode":"interrupt","description":""},
  "config":null,"running":null,"counters":null,"hasPendingChange":false}]}`

// P08 re-review R1 (D-118): `show interfaces` keeps its pre-P08 meaning — the interfaces the agent retrieved
// from the data plane. Rows with a null config are not listed, a name lookup exits 5 "not in the data plane"
// for them (a script checking the exit code must not take an uncommitted interface as present), and
// completion does not offer them. --json prints the API's answer unchanged.
func TestShowInterfacesListsOnlyRetrievedRows(t *testing.T) {
	f := newFake(t)
	f.override["GET /api/v1/state/interfaces"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(p08Interfaces))
	}
	notRetrieved := []string{"host-w1w0", "host-w1w9", "host-w3l0"}

	r := f.vrx(t, nil, "", "show", "interfaces")
	if r.code != 0 || !strings.Contains(r.stdout, "host-w1l0 ") || !strings.Contains(r.stdout, "host-w1l0.100") || !strings.Contains(r.stdout, "1400") {
		t.Fatalf("table: %d %q %q", r.code, r.stdout, r.stderr)
	}
	for _, n := range notRetrieved {
		if strings.Contains(r.stdout, n) {
			t.Errorf("table lists %s (config null):\n%s", n, r.stdout)
		}
	}

	r = f.vrx(t, nil, "", "show", "interfaces", "host-w1l0")
	if r.code != 0 || !strings.Contains(r.stdout, "Interface host-w1l0 (retrieved") || !strings.Contains(r.stdout, "mtu 1400") {
		t.Errorf("show interfaces host-w1l0: %d %q %q", r.code, r.stdout, r.stderr)
	}
	for _, n := range notRetrieved {
		r = f.vrx(t, nil, "", "show", "interfaces", n)
		if r.code != ExitNotFound || !strings.Contains(r.stderr, "not in the data plane") {
			t.Errorf("show interfaces %s: exit %d %q %q, want %d not in the data plane", n, r.code, r.stdout, r.stderr, ExitNotFound)
		}
	}

	r = f.vrx(t, nil, "", "--json", "show", "interfaces")
	if r.code != 0 || !strings.Contains(r.stdout, `"host-w1w9"`) {
		t.Errorf("--json is the API's answer unchanged: %d %q", r.code, r.stdout)
	}

	a := &App{Stdin: os.Stdin, Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }}
	c, _ := api.New(f.srv.URL)
	c.Cred = api.Key("vrxk_test")
	a.client = c
	cs, _ := a.candidates(context.Background(), "show interfaces ")
	var got []string
	for _, x := range cs {
		got = append(got, x.Text)
	}
	if !slices.Contains(got, "host-w1l0") || !slices.Contains(got, "host-w1l0.100") {
		t.Errorf("completion %q lacks the retrieved interfaces", got)
	}
	for _, n := range notRetrieved {
		if slices.Contains(got, n) {
			t.Errorf("completion offers %s (config null): %q", n, got)
		}
	}
}
