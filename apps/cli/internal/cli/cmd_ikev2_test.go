package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestIPsecStateUsesRESTPagingAndPreservesJSON(t *testing.T) {
	f := newFake(t)
	sas := `{"daemonVersion":"vpp-ikev2","total":75,"sas":[{"tunnel":"branch","children":[{"spiIn":"01020304","bytesIn":1048576}]}]}`
	f.override["GET /api/v1/state/ipsec/sas"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tunnel") != "branch" || r.URL.Query().Get("limit") != "25" || r.URL.Query().Get("offset") != "50" {
			t.Errorf("wrong state selector/paging: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sas))
	}
	result := f.ngfw(t, nil, "", "--json", "show", "ipsec", "sa", "branch", "offset", "50", "limit", "25")
	if result.code != 0 || strings.TrimSpace(result.stdout) != sas {
		t.Fatalf("native SA state: %d %s %s", result.code, result.stdout, result.stderr)
	}
	f.override["GET /api/v1/state/ipsec/tunnels"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tunnel") != "branch" {
			t.Errorf("missing tunnel selector")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tunnels":[{"tunnel":"branch","status":"up"}]}`))
	}
	result = f.ngfw(t, nil, "", "show", "ipsec", "tunnels", "branch")
	if result.code != 0 || !strings.Contains(result.stdout, `"status": "up"`) {
		t.Fatalf("tunnel state: %d %s %s", result.code, result.stdout, result.stderr)
	}
	if len(f.requests()) != 2 || len(f.mutations()) != 0 {
		t.Fatal("state commands must make only their REST reads", f.requests())
	}
	if strings.Contains(result.stdout, "ngfwk_test") {
		t.Fatal("credential echoed")
	}
}

func TestIPsecInvalidPagingAndSPIHaveNoNetworkSideEffect(t *testing.T) {
	f := newFake(t)
	for _, args := range [][]string{
		{"show", "ipsec", "sa", "limit", "0"}, {"show", "ipsec", "sa", "limit", "1001"},
		{"show", "ipsec", "sa", "offset", "1000001"}, {"show", "ipsec", "sa", "offset", "-1"},
		{"show", "ipsec", "sa", "limit", "1", "limit", "2"}, {"show", "ipsec", "sa", "offset"},
		{"ipsec", "rekey", "branch", "4294967296"}, {"ipsec", "delete-sa", "branch", "18446744073709551616"},
		{"ipsec", "rekey", "branch", "0"}, {"ipsec", "delete-sa", "branch", "-1"},
	} {
		r := f.ngfw(t, nil, "", args...)
		if r.code != ExitUsage {
			t.Fatalf("invalid %v: %d %s", args, r.code, r.stderr)
		}
	}
	if len(f.requests()) != 0 {
		t.Fatal("invalid input reached API", f.requests())
	}
}

func TestIPsecActionsPreserveSPIWidthsAndAPIFailures(t *testing.T) {
	f := newFake(t)
	path := "POST /api/v1/actions/ipsec/ikev2/branch/rekey"
	f.override[path] = func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChildSPI uint32 `json:"childSpi"`
		}
		if e := json.NewDecoder(strings.NewReader(strings.SplitN(f.requests()[len(f.requests())-1], " ", 3)[2])).Decode(&body); e != nil || body.ChildSPI != 10 {
			t.Errorf("decimal leading-zero SPI changed: %#v %v", body, e)
		}
		problem(w, http.StatusForbidden, "administrator required")
	}
	result := f.ngfw(t, nil, "", "ipsec", "rekey", "branch", "010")
	if result.code != ExitForbidden || result.stdout != "" {
		t.Fatalf("RBAC refusal: %d %s %s", result.code, result.stdout, result.stderr)
	}
	f.override["POST /api/v1/actions/ipsec/ikev2/branch/delete-sa"] = func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IkeSPI string `json:"ikeSpi"`
		}
		if e := json.NewDecoder(strings.NewReader(strings.SplitN(f.requests()[len(f.requests())-1], " ", 3)[2])).Decode(&body); e != nil || body.IkeSPI != "18446744073709551615" {
			t.Errorf("64-bit SPI lost precision: %#v %v", body, e)
		}
		problem(w, http.StatusServiceUnavailable, "required native plugin capability unavailable")
	}
	result = f.ngfw(t, nil, "", "ipsec", "delete-sa", "branch", "0xffffffffffffffff")
	if result.code != ExitUnavailable || !strings.Contains(result.stderr, "capability unavailable") {
		t.Fatalf("native unavailable: %d %s", result.code, result.stderr)
	}
	if len(f.mutations()) != 2 {
		t.Fatal("unexpected action requests", f.mutations())
	}
}
