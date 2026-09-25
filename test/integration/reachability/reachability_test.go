package reachability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeAPI answers the three calls Check makes with canned bodies.
func fakeAPI(t *testing.T, commit, drift string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t" {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/config/"):
			var v any
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			_, _ = w.Write([]byte(`{"pointer":"x","before":null,"after":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/config/commit":
			_, _ = w.Write([]byte(commit))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state/drift":
			_, _ = w.Write([]byte(drift))
		default:
			http.NotFound(w, r)
		}
	}))
}

const (
	okCommit = `{"status":"applied","results":[],"warnings":[],"notApplied":[]}`
	okDrift  = `{"subsystems":["interfaces","vrfs","routing"],"changes":[],"ignored":[{"pointer":"/routing","rule":"agent.unsupported-field"}]}`
)

func TestCheck(t *testing.T) {
	cases := []struct {
		name, commit, drift, want string
	}{
		{"reachable", okCommit, okDrift, ""},
		{"partially applied", `{"status":"partially-applied","results":[],"warnings":[],"notApplied":["nat"]}`, okDrift, `status "partially-applied"`},
		{"notApplied", `{"status":"applied","results":[],"warnings":[],"notApplied":["system"]}`, okDrift, "notApplied [system]"},
		{"unsupported field", `{"status":"applied","results":[],"warnings":[{"pointer":"/interfaces/loop1/unnumbered","message":"m","rule":"agent.unsupported-field"}],"notApplied":[]}`, okDrift, "agent.unsupported-field"},
		{"drift", okCommit, `{"subsystems":["interfaces"],"changes":[{"op":"remove","pointer":"/interfaces/loop1/mtu"}],"ignored":[]}`, "Retrieve() != desired"},
		{"domain not retrieved", okCommit, `{"subsystems":["vrfs"],"changes":[],"ignored":[]}`, `domain "interfaces" is not retrieved`},
		{"leaf ignored", okCommit, `{"subsystems":["interfaces"],"changes":[],"ignored":[{"pointer":"/interfaces/loop1","rule":"agent.unsupported-field"}]}`, "drift ignores"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeAPI(t, tc.commit, tc.drift)
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, Authorization: "Bearer t"}
			_, err := c.Check(context.Background(), "/interfaces/loop1", map[string]any{"enabled": true, "mtu": 1400})
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("error %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestEscapePointer(t *testing.T) {
	if got := escapePointer("/interfaces/a b?c"); got != "/interfaces/a%20b%3Fc" {
		t.Fatalf("escapePointer = %q", got)
	}
}

func TestCheckRejectsRoot(t *testing.T) {
	if _, err := (&Client{}).Check(context.Background(), "", nil); err == nil {
		t.Fatal("root pointer accepted")
	}
}

func TestHTTPErrorSurfaces(t *testing.T) {
	srv := fakeAPI(t, okCommit, okDrift)
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL, Authorization: "Bearer wrong"}).Check(context.Background(), "/vrfs/blue", map[string]any{"table": 7})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("got %v, want HTTP 401", err)
	}
}

// TestReachabilityLoopback is the live DoD check against a running API + agent + VPP. It needs
// VRX_INTEGRATION=1, VRX_API_URL and VRX_API_TOKEN (a bearer token; never printed) and edits the
// running configuration of that box, so it runs only in a lab slot.
func TestReachabilityLoopback(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("VRX_INTEGRATION != 1: live API reachability check not run (this is a skip, not a pass)")
	}
	base, tok := os.Getenv("VRX_API_URL"), os.Getenv("VRX_API_TOKEN")
	if base == "" || tok == "" {
		t.Skip("VRX_API_URL / VRX_API_TOKEN unset: no API to check against")
	}
	n := os.Getenv("VRX_REACH_LOOPBACK") // loopbacks are named loop<N> (desired/interfaces.go); pick a slot-free N
	if n == "" {
		n = "9099"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := &Client{BaseURL: base, Authorization: "Bearer " + tok}
	ptr := "/interfaces/loop" + n
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), time.Minute)
		defer ccancel()
		if err := c.Delete(cctx, ptr); err != nil {
			t.Errorf("cleanup %s: %v", ptr, err)
		}
	})
	if _, err := c.Check(ctx, ptr, map[string]any{"enabled": true, "ipv4": []string{"198.18.99.1/32"}}); err != nil {
		t.Fatal(err)
	}
}
