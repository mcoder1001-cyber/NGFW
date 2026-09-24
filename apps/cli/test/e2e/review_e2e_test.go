package e2e

// Review fix round (docs/status/tasks/P13-review.md) on the slot dev stack: H1 terminal-control injection through a
// real commit comment, H2 Ctrl-C cancels only the running command (then `confirm` still works), M3 pending-commit
// reminders and exit warning, M6 a pasted block stops at its first failing line.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewFixesOnTheRealStack(t *testing.T) {
	s := setup(t)
	const long = 30 * time.Second
	If := strings.Replace(s.iface, "01", "02", 1) // loop302: its own object
	pwFile := s.pwFile

	// ---- H1: an ESC/OSC/CSI commit comment through the real API is shown visibly, never raw
	if _, e, c := s.oneShot(t, nil, "--password-file", pwFile, "login", "admin"); c != 0 {
		t.Fatalf("login: %d %s", c, e)
	}
	evil := "cli review\x1b]0;PWNED\x07\x1b[2K\rinnocuous"
	if _, e, c := s.oneShot(t, nil, "set", "interfaces", If, "ipv4", `["`+s.net[:len(s.net)-4]+`102.1/24"]`); c != 0 {
		t.Fatalf("set: %d %s", c, e)
	}
	out, errOut, c := s.oneShot(t, nil, "commit", "comment", evil)
	if c != 0 {
		t.Fatalf("commit: %d %s", c, errOut)
	}
	rev, _, _ := s.oneShot(t, nil, "show", "revisions", "1")
	for name, text := range map[string]string{"commit output": out, "show revisions": rev} {
		if bytes.ContainsAny([]byte(text), "\x1b\x07\r") {
			t.Errorf("%s: raw control bytes reached the terminal: %q", name, text)
		}
		if !strings.Contains(text, `cli review\x1b]0;PWNED\x07\x1b[2K\x0dinnocuous`) {
			t.Errorf("%s: comment not rendered visibly: %q", name, text)
		}
	}
	if p := os.Getenv("VRX_E2E_TRANSCRIPT"); p != "" {
		_ = os.WriteFile(p+".h1", []byte("$ vrx commit comment <ESC]0;PWNED BEL ESC[2K CR …>\n"+out+"$ vrx show revisions 1\n"+rev), 0o600)
	}

	// ---- a proxy in front of the real API that holds GET /state/drift for 20 s (a slow command to interrupt)
	api, _ := url.Parse(s.api)
	rp := httputil.NewSingleHostReverseProxy(api)
	arrived := make(chan struct{}, 4)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/state/drift" {
			arrived <- struct{}{}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Second):
			}
		}
		rp.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	env := append(s.env(), "VRX_API_URL="+proxy.URL, "VRX_SESSION_FILE="+filepath.Join(s.dir, "private", "repl-session.json"))
	r := spawn(t, env, s.bin)
	pw, _ := os.ReadFile(pwFile)
	r.expect(`Username: `, long)
	r.line("admin")
	r.expect(`Password: `, long)
	r.line(strings.TrimSpace(string(pw)))
	r.command("configure")
	r.command("set interfaces " + If + " mtu 9000")

	// ---- H2 + M3: commit confirm, Ctrl-C a slow command, the shell still confirms
	r.command(`commit confirm 60 comment "review H2"`)
	r.expect(`NOT confirmed: it reverts automatically at \d\d:\d\d:\d\d [+-]\d{4} \(in \d+ s\)`, long)
	r.expect(`! commit \w+ is applied but NOT confirmed: it reverts at [^\n]*type .confirm. to keep it`, long)
	r.expect(`admin@vrx\[!\d+s\]# `, long)
	r.command("run show drift")
	select {
	case <-arrived:
	case <-time.After(long):
		t.Fatal("the slow request never reached the proxy")
	}
	r.send("\x03") // Ctrl-C while the command runs (tty in cooked mode → SIGINT)
	r.expect(`error: interrupted`, long)
	r.command("run show whoami")
	r.expect(`admin \(role admin`, long) // the shell is still usable
	r.command("confirm")
	r.expect(`commit \S+ confirmed — revision \d+`, long)

	// ---- M6: a pasted block stops at its first failing line; the trailing commit does not run
	r.prompt()
	r.send("set interfaces " + If + " description paste-test\rset interfaces " + If + " mtu 70000\rcommit\r")
	r.expect(`must be ≤ 9216`, long)
	r.expect(`\(stopped: \d+ byte\(s\) of pasted/typed-ahead input after the failing line were discarded\)`, long)
	r.command("compare")
	r.expect(`\+ set interfaces \S+ description paste-test`, long) // still uncommitted
	r.command("discard")
	r.expect(`candidate discarded`, long)

	// ---- M3: exit warnings while a confirmed commit is pending, then the revert is reported
	r.command("set interfaces " + If + " mtu 1400")
	r.command(`commit confirm 5 comment "review M3"`)
	r.expect(`NOT confirmed`, long)
	r.command("exit")
	r.expect("note: commit \\S+ is applied but NOT confirmed[^\\n]*`confirm` to keep it", long)
	r.command("exit")
	r.expect(`warning: commit \S+ is applied but NOT confirmed[^\n]*\n[^\n]*The shell stays open`, long)
	time.Sleep(8 * time.Second)
	r.line("") // a new prompt: the shell notices the revert
	r.expect(`note: commit \S+ was NOT confirmed and has been reverted automatically`, long)
	r.command("exit")
	if code := r.wait(long); code != 0 {
		t.Fatalf("REPL exit code %d", code)
	}
	if p := os.Getenv("VRX_E2E_TRANSCRIPT"); p != "" {
		_ = os.WriteFile(p+".review", []byte(r.transcript()+"\n"), 0o600)
	}
	mtu, _, _ := s.oneShot(t, nil, "show", "configuration", "interfaces", If, "mtu")
	if strings.TrimSpace(mtu) != "9000" {
		t.Errorf("after the revert running mtu = %q, want the confirmed 9000", mtu)
	}

	// cleanup
	if _, e, c := s.oneShot(t, nil, "delete", "interfaces", If); c != 0 {
		t.Fatalf("cleanup delete: %d %s", c, e)
	}
	if _, e, c := s.oneShot(t, nil, "commit", "comment", "cli review e2e cleanup"); c != 0 {
		t.Fatalf("cleanup commit: %d %s", c, e)
	}
}
