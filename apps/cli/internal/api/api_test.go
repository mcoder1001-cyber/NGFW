package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"ngfw/cli/internal/api/opgen"
)

// The committed table must be what vrx-opgen makes of the current OpenAPI document (written by `pnpm gen`; the
// test skips when it has not been generated in this checkout).
func TestOperationsTableMatchesOpenAPI(t *testing.T) {
	doc, err := os.ReadFile("../../../../packages/api-client/openapi.json")
	if err != nil {
		t.Skipf("packages/api-client/openapi.json not generated here (%v) — run `pnpm gen`", err)
	}
	want, err := opgen.Generate(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("operations_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("internal/api/operations_gen.go is stale: run `make -C apps/cli gen`")
	}
}

func TestGenerateFromFixture(t *testing.T) {
	doc, err := os.ReadFile("../testdata/openapi-min.json")
	if err != nil {
		t.Fatal(err)
	}
	src, err := opgen.Generate(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Config_commit":`, `Method: "POST", Path: "/api/v1/config/commit"`, `QueryParams: []string{"comment", "confirm"}`, `"Health_health":`} {
		if !strings.Contains(string(src), want) {
			t.Errorf("generated source lacks %s:\n%s", want, src)
		}
	}
}

func TestURLEncodesPointerSegmentsAndChecksQuery(t *testing.T) {
	c, err := New("http://127.0.0.1:3000/")
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := c.URL(Call{Op: "Config_putAt", Params: map[string]string{"path": "interfaces/TenGigabitEthernet0~10~10/description"}})
	if err != nil || u != "http://127.0.0.1:3000/api/v1/config/interfaces/TenGigabitEthernet0~10~10/description" {
		t.Errorf("URL = %s, %v", u, err)
	}
	u, _, _ = c.URL(Call{Op: "Config_putAt", Params: map[string]string{"path": "objects/addresses/a b?c"}})
	if !strings.HasSuffix(u, "/objects/addresses/a%20b%3Fc") {
		t.Errorf("segment escaping: %s", u)
	}
	u, _, _ = c.URL(Call{Op: "Config_commit", Query: url.Values{"confirm": {"5"}, "comment": {"a b"}}})
	if u != "http://127.0.0.1:3000/api/v1/config/commit?comment=a+b&confirm=5" {
		t.Errorf("query: %s", u)
	}
	if _, _, err := c.URL(Call{Op: "Config_commit", Query: url.Values{"force": {"1"}}}); err == nil {
		t.Error("an undocumented query parameter must be refused")
	}
	if _, _, err := c.URL(Call{Op: "No_such"}); err == nil {
		t.Error("unknown operation must be refused")
	}
	if _, _, err := c.URL(Call{Op: "Config_rollback"}); err == nil {
		t.Error("missing path parameter must be refused")
	}
	for _, bad := range []string{"127.0.0.1:3000", "ftp://x", "http://"} {
		if _, err := New(bad); err == nil {
			t.Errorf("New(%q) must fail", bad)
		}
	}
}

func TestProblemAndCredentialsNotLogged(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("content-type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"Bad Request","status":400,"detail":"invalid candidate","errors":[{"pointer":"/interfaces/x/mtu","message":"too big"}]}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL)
	var dbg bytes.Buffer
	c.Debug = &dbg
	c.Cred = Key("vrxk_" + strings.Repeat("s", 20))
	_, err := c.Do(context.Background(), Call{Op: "Config_validate"})
	ae, ok := err.(*Error)
	if !ok || ae.Status != 400 || ae.Problem == nil || ae.Problem.Errors[0].Pointer != "/interfaces/x/mtu" {
		t.Fatalf("problem not decoded: %#v", err)
	}
	if gotAuth != "ApiKey vrxk_"+strings.Repeat("s", 20) {
		t.Errorf("Authorization header %q", gotAuth)
	}
	if strings.Contains(dbg.String(), "vrxk_") || !strings.Contains(dbg.String(), "POST /api/v1/config/validate → 400") {
		t.Errorf("debug output: %q", dbg.String())
	}
}
