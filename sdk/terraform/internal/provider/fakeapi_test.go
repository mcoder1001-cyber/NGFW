package provider_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeKey is a stand-in credential: the literal marker the repository's secret scanner allows in fixtures.
const fakeKey = "VRX_TEST_PSK_tf"

// fakeAPI is an in-memory VRX API: running + candidate documents, diff, commit (?confirm) / confirm / discard,
// schema defaults for /interfaces/<name> (like the real API), one validation rule (ipv4 must be a CIDR) and
// `passwordHash` redaction on reads (write-only, D-046).
type fakeAPI struct {
	t         *testing.T
	mu        sync.Mutex
	running   map[string]any
	candidate map[string]any
	pending   bool
	rev       int64
	calls     []string
	bodies    []string
	failState bool // /state/system answers 503 (the post-commit check fails)
	srv       *httptest.Server
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{t: t, running: map[string]any{}, candidate: map[string]any{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) problem(w http.ResponseWriter, status int, title string, errs ...map[string]string) {
	w.Header().Set("content-type", "application/problem+json")
	w.WriteHeader(status)
	doc := map[string]any{"type": "about:blank", "title": title, "status": status}
	if len(errs) > 0 {
		doc["errors"] = errs
	}
	_ = json.NewEncoder(w).Encode(doc)
}

func (f *fakeAPI) ok(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		call += "?" + r.URL.RawQuery
	}
	f.calls = append(f.calls, call)
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		f.bodies = append(f.bodies, string(body))
		if r.Header.Get("content-type") != "application/json" {
			f.problem(w, 400, "content-type")
			return
		}
	} else if r.Header.Get("content-type") != "" {
		f.problem(w, 400, "Body cannot be empty when content-type is set to 'application/json'")
		return
	}
	if r.Header.Get("authorization") != "ApiKey "+fakeKey {
		f.problem(w, 401, "Unauthorized")
		return
	}
	p := r.URL.Path
	switch {
	case p == "/api/v1/config/diff":
		changes := []any{}
		if !reflect.DeepEqual(f.running, f.candidate) {
			changes = append(changes, map[string]any{"op": "replace", "pointer": ""})
		}
		f.ok(w, map[string]any{"changes": changes})
	case p == "/api/v1/config/lock":
		f.ok(w, map[string]any{"locked": false, "owner": nil})
	case p == "/api/v1/config/discard":
		f.candidate = cloneDoc(f.running)
		f.ok(w, map[string]any{"discarded": true})
	case p == "/api/v1/config/commit":
		if sec := r.URL.Query().Get("confirm"); sec != "" {
			f.pending = true
			f.ok(w, map[string]any{"status": "pending", "txnId": "t", "confirmDeadline": "soon", "results": []any{}, "warnings": []any{}, "notApplied": []any{}})
			return
		}
		f.promote()
		f.ok(w, map[string]any{"status": "applied", "revision": map[string]any{"id": f.rev}})
	case p == "/api/v1/config/commit/confirm":
		if !f.pending {
			f.problem(w, 409, "nothing pending")
			return
		}
		f.pending = false
		f.promote()
		f.ok(w, map[string]any{"status": "confirmed", "revision": map[string]any{"id": f.rev}})
	case p == "/api/v1/state/system":
		if f.failState {
			f.problem(w, 503, "agent unreachable")
			return
		}
		f.ok(w, map[string]any{"sync": map[string]any{"state": "in-sync"}})
	case p == "/api/v1/state/interfaces":
		items := []any{}
		ifs, _ := f.running["interfaces"].(map[string]any)
		for _, n := range sortedKeys(ifs) {
			items = append(items, map[string]any{"name": n, "config": ifs[n]})
		}
		f.ok(w, map[string]any{"items": items})
	case strings.HasPrefix(p, "/api/v1/config/"):
		f.pointerRoute(w, r, body)
	default:
		f.problem(w, 404, "Not found")
	}
}

func (f *fakeAPI) promote() {
	f.running = cloneDoc(f.candidate)
	f.rev++
}

func (f *fakeAPI) pointerRoute(w http.ResponseWriter, r *http.Request, body []byte) {
	raw := strings.TrimPrefix(r.URL.EscapedPath(), "/api/v1/config/")
	var toks []string
	for _, s := range strings.Split(raw, "/") {
		u, _ := url.PathUnescape(s)
		toks = append(toks, strings.ReplaceAll(strings.ReplaceAll(u, "~1", "/"), "~0", "~"))
	}
	ptr := "/" + raw
	switch r.Method {
	case http.MethodGet:
		v, ok := get(f.running, toks)
		if !ok {
			f.problem(w, 404, "nothing at "+ptr)
			return
		}
		f.ok(w, redactHashes(clone(v)))
	case http.MethodPut:
		var v any
		_ = json.Unmarshal(body, &v)
		if toks[0] == "interfaces" && len(toks) == 2 {
			m, _ := v.(map[string]any)
			if ips, ok := m["ipv4"].([]any); ok {
				for i, ip := range ips {
					if s, _ := ip.(string); !strings.Contains(s, "/") {
						f.problem(w, 400, "Validation failed", map[string]string{"pointer": fmt.Sprintf("%s/ipv4/%d", ptr, i), "message": "Invalid IPv4 range"})
						return
					}
				}
			}
			for k, d := range map[string]any{"enabled": false, "ipv4": []any{}, "ipv6": []any{}, "vrf": "default", "promiscuous": false, "subinterfaces": map[string]any{}} {
				if _, ok := m[k]; !ok {
					m[k] = d
				}
			}
		}
		set(f.candidate, toks, v)
		f.ok(w, map[string]any{"pointer": ptr})
	case http.MethodDelete:
		if _, ok := get(f.candidate, toks); !ok {
			f.problem(w, 404, "nothing at "+ptr)
			return
		}
		del(f.candidate, toks)
		f.ok(w, map[string]any{"pointer": ptr})
	default:
		f.problem(w, 405, "method")
	}
}

func (f *fakeAPI) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeAPI) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls, f.bodies = nil, nil
}

func clone(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	if m, ok := out.(map[string]any); ok && m == nil {
		return map[string]any{}
	}
	return out
}

func (f *fakeAPI) setRunning(toks []string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	set(f.running, toks, v)
	f.candidate = cloneDoc(f.running)
}

func cloneDoc(m map[string]any) map[string]any {
	out, _ := clone(m).(map[string]any)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func get(doc map[string]any, toks []string) (any, bool) {
	var cur any = doc
	for _, t := range toks {
		switch c := cur.(type) {
		case map[string]any:
			v, ok := c[t]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(t)
			if err != nil || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

func set(doc map[string]any, toks []string, v any) {
	cur := doc
	for _, t := range toks[:len(toks)-1] {
		n, ok := cur[t].(map[string]any)
		if !ok {
			n = map[string]any{}
			cur[t] = n
		}
		cur = n
	}
	cur[toks[len(toks)-1]] = v
}

func del(doc map[string]any, toks []string) {
	parent, ok := get(doc, toks[:len(toks)-1])
	if m, isMap := parent.(map[string]any); ok && isMap {
		delete(m, toks[len(toks)-1])
	}
}

func redactHashes(v any) any {
	switch x := v.(type) {
	case map[string]any:
		delete(x, "passwordHash")
		for k, e := range x {
			x[k] = redactHashes(e)
		}
	case []any:
		for i, e := range x {
			x[i] = redactHashes(e)
		}
	}
	return v
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	for i := range ks {
		for j := i + 1; j < len(ks); j++ {
			if ks[j] < ks[i] {
				ks[i], ks[j] = ks[j], ks[i]
			}
		}
	}
	return ks
}
