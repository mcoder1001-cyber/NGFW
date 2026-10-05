package keadhcprelay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

// Exercise the same HTTP client and acceptance validator used by the real fixture.
func TestCommitAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		reject         bool
	}{
		{"fully-applied", `{"status":"applied","revision":{"id":7},"notApplied":[],"warnings":[]}`, false},
		{"changed-DHCP-unsupported", `{"status":"applied","revision":{"id":7},"notApplied":[],"warnings":[{"rule":"agent.unsupported-field","pointer":"/services/dhcp/relays/to-kea"}]}`, true},
		{"partial-apply", `{"status":"applied","revision":{"id":7},"notApplied":["services.dhcp"]}`, true},
		{"unsupported-result", `{"status":"applied","revision":{"id":7},"results":[{"reason":"agent.unsupported-field"}]}`, true},
		{"missing-revision", `{"status":"applied","notApplied":[]}`, true},
		{"non-applied", `{"status":"unchanged","revision":{"id":7},"notApplied":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/api/v1/config/commit" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			a := &api{t: t, base: server.URL}
			result := a.call("POST", "/api/v1/config/commit?comment=acceptance-test", nil)
			if result.status != 200 {
				t.Fatalf("fixture response status %d", result.status)
			}
			if err := appliedResponse(result); (err != nil) != tc.reject {
				t.Fatalf("reject=%v, got %v", tc.reject, err)
			}
		})
	}
}

func TestConfigDigestCanonical(t *testing.T) {
	var a, b map[string]any
	_ = json.Unmarshal([]byte(`{"services":{"dhcp":{}},"vrfs":{}}`), &a)
	_ = json.Unmarshal([]byte(`{"vrfs":{},"services":{"dhcp":{}}}`), &b)
	if configDigest(a) != configDigest(b) {
		t.Fatal("object order changed candidate digest")
	}
	b["services"] = map[string]any{"dhcp": map[string]any{"relays": map[string]any{"new": true}}}
	if configDigest(a) == configDigest(b) {
		t.Fatal("changed candidate digest not detected")
	}
}

func TestBaselineWarningGuard(t *testing.T) {
	warning := map[string]any{"rule": "agent.unsupported-field", "pointer": "/services/ntp", "message": "disabled"}
	var ntp any
	_ = json.Unmarshal([]byte(inactiveDefaults["/services/ntp"]), &ntp)
	document := map[string]any{"services": map[string]any{"ntp": ntp}}
	response := func(w any) resp {
		return resp{status: 200, body: map[string]any{"status": "applied", "revision": map[string]any{"id": float64(2)}, "notApplied": []any{}, "warnings": []any{w}}}
	}
	a := &api{t: t}
	if err := a.baselineResponse(response(warning), document, document, true); err != nil {
		t.Fatal(err)
	}
	if err := a.baselineResponse(response(warning), document, document, false); err != nil {
		t.Fatal(err)
	}
	for _, altered := range []map[string]any{
		{"rule": "agent.unsupported-field", "pointer": "/services/dhcp/relays/to-kea", "message": "disabled"},
		{"rule": "agent.unsupported-field", "pointer": "/services/ntp", "message": "new unsupported setting"},
	} {
		if err := a.baselineResponse(response(altered), document, document, false); err == nil {
			t.Fatalf("accepted changed/new warning %v", altered)
		}
	}
	changed := map[string]any{"services": map[string]any{"ntp": map[string]any{"enabled": true}}}
	if err := a.baselineResponse(response(warning), document, changed, false); err == nil {
		t.Fatal("accepted changed baseline field")
	}
}

func TestInactiveBaselineRequiresRecognizedExplicitDefault(t *testing.T) {
	for pointer, encoded := range inactiveDefaults {
		t.Run(pointer, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(encoded), &value); err != nil {
				t.Fatal(err)
			}
			if !inactiveDefault(pointer, value) {
				t.Fatal("recognized inactive schema default refused")
			}
			for _, altered := range []any{nil, map[string]any{}, map[string]any{"enabled": true}} {
				if inactiveDefault(pointer, altered) {
					t.Fatalf("missing/active/unrecognized value accepted: %v", altered)
				}
			}
			object := value.(map[string]any)
			object["unrecognizedControl"] = true
			if inactiveDefault(pointer, object) {
				t.Fatal("unrecognized control accepted")
			}
		})
	}
}

// Exercise the whole production fixture helper, including the actual numeric
// LockOut wire contract, candidate readback and post-commit persistence.
func TestCommitNumericOwner(t *testing.T) {
	const before = `{"vrfs":{"default":{"id":0}},"interfaces":{}}`
	const candidate = `{"vrfs":{"default":{"id":0},"fixture":{"id":123}},"interfaces":{}}`
	var committed atomic.Bool
	var requests []string
	var requestsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var response string
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/config":
			response = before
			if committed.Load() {
				response = candidate
			}
		case "GET /api/v1/config/lock":
			response = `{"locked":true,"owner":"admin","ownerId":1,"ownerKeyId":null}`
		case "GET /api/v1/config/candidate":
			response = candidate
		case "POST /api/v1/config/commit":
			if r.URL.Query().Get("comment") != "kea-base" {
				t.Error("commit comment changed")
			}
			committed.Store(true)
			response = `{"status":"applied","revision":{"id":42},"notApplied":[],"warnings":[]}`
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	a := &api{t: t, base: server.URL}
	result := a.commit("kea-base")
	if result["status"] != "applied" || !committed.Load() {
		t.Fatal("numeric-owner commit did not apply")
	}
	expected := []string{"GET /api/v1/config", "GET /api/v1/config/lock", "GET /api/v1/config/candidate", "GET /api/v1/config/lock", "POST /api/v1/config/commit", "GET /api/v1/config"}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	if !reflect.DeepEqual(requests, expected) {
		t.Fatalf("unexpected lifecycle %v", requests)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(candidate), &document); err != nil {
		t.Fatal(err)
	}
	if configDigest(a.baselineDocument) != configDigest(document) {
		t.Fatal("baseline did not preserve committed candidate")
	}
}

func TestCandidateOwnerWireContract(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		valid          bool
	}{
		{"numeric", `{"locked":true,"ownerId":1}`, true},
		{"display-name", `{"locked":true,"ownerId":"admin"}`, false},
		{"boolean", `{"locked":true,"ownerId":true}`, false},
		{"fractional", `{"locked":true,"ownerId":1.5}`, false},
		{"zero", `{"locked":true,"ownerId":0}`, false},
		{"negative", `{"locked":true,"ownerId":-1}`, false},
		{"unsafe", `{"locked":true,"ownerId":9007199254740992}`, false},
		{"missing", `{"locked":true}`, false},
		{"null", `{"locked":true,"ownerId":null}`, false},
		{"unlocked", `{"locked":false,"ownerId":1}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			a := &api{t: t, base: server.URL}
			owner, err := candidateOwner(a.call("GET", "/api/v1/config/lock", nil))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v got %v", tc.valid, err)
			}
			if tc.valid && owner != 1 {
				t.Fatalf("wrong owner %v", owner)
			}
		})
	}
}
