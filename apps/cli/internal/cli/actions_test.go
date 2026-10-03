package cli

import (
	"net/http"
	"strings"
	"testing"
)

func TestActionsSendTargetJSON(t *testing.T) {
	for _, name := range []string{"ping", "traceroute"} {
		for _, target := range []string{"192.0.2.1", "2001:db8::1", "::ffff:192.0.2.1"} {
			t.Run(name+"/"+target, func(t *testing.T) {
				f := newFake(t)
				response := `{"action":"` + name + `","lines":["reply"],"done":{"summary":"complete","exitCode":0,"stats":{}}}`
				f.override["POST /api/v1/actions/"+name] = func(w http.ResponseWriter, r *http.Request) {
					if got := r.Header.Get("Content-Type"); got != "application/json" {
						t.Errorf("content type %q", got)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(response))
				}
				r := f.vrx(t, nil, "", "--json", name, target)
				if r.code != ExitOK {
					t.Fatalf("exit %d: %s", r.code, r.stderr)
				}
				want := "POST /api/v1/actions/" + name + ` {"target":"` + target + `"}`
				got := f.mutations()
				if len(got) != 1 || got[0] != want {
					t.Fatalf("wire request %q, want %q", got, want)
				}
				if strings.TrimSpace(r.stdout) != response {
					t.Errorf("response changed: %s", r.stdout)
				}
			})
		}
	}
}

func TestActionsRejectInvalidTargetsBeforeRequest(t *testing.T) {
	for _, name := range []string{"ping", "traceroute"} {
		for _, args := range [][]string{nil, {"192.0.2.1", "extra"}, {"example.com"}, {""}, {"999.0.0.1"}, {"192.0.2.1/24"}, {"[::1]"}, {"fe80::1%eth0"}, {"010.0.0.1"}} {
			t.Run(name+"/"+strings.Join(args, "/"), func(t *testing.T) {
				f := newFake(t)
				r := f.vrx(t, nil, "", append([]string{name}, args...)...)
				if r.code != ExitUsage {
					t.Fatalf("exit %d, want usage: %s", r.code, r.stderr)
				}
				if got := f.requests(); len(got) != 0 {
					t.Fatalf("invalid target sent requests %q", got)
				}
			})
		}
	}
}

func TestTracerouteRetainsNotImplementedExit(t *testing.T) {
	f := newFake(t)
	f.override["POST /api/v1/actions/traceroute"] = func(w http.ResponseWriter, _ *http.Request) { problem(w, 501, "no VPP traceroute API") }
	r := f.vrx(t, nil, "", "traceroute", "2001:db8::1")
	if r.code != ExitNotImplemented {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if got := f.mutations(); len(got) != 1 || got[0] != `POST /api/v1/actions/traceroute {"target":"2001:db8::1"}` {
		t.Fatalf("wire request %q", got)
	}
}
