package acl

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestACLEditorScale measures the rule editor's routes on a VRX_ACL_EDITOR_RULES-rule list (default 100 000) that stays
// in the CANDIDATE: CSV dry run + import through the streamed route, first/middle/last page latency, a quick-filter
// search, a bulk disable of 1 000 rules and a move to sequence — against the real API and slot database. Nothing is
// committed and nothing reaches VPP (the agent is up only because the API needs it for its health), so it needs no
// manager window.
//
//	VRX_ACL_EDITOR=1 run.sh -run TestACLEditorScale
func TestACLEditorScale(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" || os.Getenv("VRX_ACL_EDITOR") != "1" {
		t.Skip("editor scale run: set VRX_INTEGRATION=1 and VRX_ACL_EDITOR=1")
	}
	n := 100_000
	if v, err := strconv.Atoi(os.Getenv("VRX_ACL_EDITOR_RULES")); err == nil && v > 0 {
		n = v
	}
	s := slotFromEnv(t)
	sharedLock(t)
	st := newStack(t, s)
	a := st.api

	var csv strings.Builder
	csv.WriteString("sequence,action,source,destination,service,description\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&csv, "%d,permit,10.%d.%d.%d/32,172.%d.%d.%d/32,tcp:443,rule %d\n", (i+1)*10, s.num, 100+i/65536, i/256%256, 16+i/65536, i/256%256, i%256, i+1)
	}
	timed := func(what string, f func() resp) resp {
		t0 := time.Now()
		r := f()
		t.Logf("%-58s → %d in %7.3f s  %s", what, r.status, time.Since(t0).Seconds(), trunc(r.raw, 160))
		return r
	}
	timed(fmt.Sprintf("CSV dry run (%d rows, %d bytes)", n, csv.Len()), func() resp {
		return a.raw("POST", "/api/v1/actions/acl/import?list=big&dryRun=true", "text/csv", csv.String())
	})
	if r := timed("CSV import into the candidate", func() resp {
		return a.raw("POST", "/api/v1/actions/acl/import?list=big&dryRun=false", "text/csv", csv.String())
	}); r.status != 200 {
		t.Fatal("import failed")
	}
	for _, q := range []string{
		"page=1&pageSize=100", "page=1&pageSize=1000", fmt.Sprintf("page=%d&pageSize=100", n/200), fmt.Sprintf("page=%d&pageSize=100", n/100),
		"page=1&pageSize=100&filter=172.17.1.", "page=1&pageSize=100&filter=rule%2099999",
	} {
		r := timed("GET rules?"+q+" (candidate)", func() resp { return a.call("GET", "/api/v1/state/acl/lists/big/rules?"+q, nil) })
		if r.status != 200 {
			t.Fatalf("page %s: %s", q, trunc(r.raw, 400))
		}
	}
	seqs := make([]int, 1000)
	for i := range seqs {
		seqs[i] = (i + 1) * 10
	}
	timed("bulk disable of 1 000 rules", func() resp {
		return a.call("POST", "/api/v1/actions/acl/lists/big/rules/bulk", map[string]any{"op": "disable", "sequences": seqs})
	})
	timed("bulk move of 1 rule to sequence 15", func() resp {
		return a.call("POST", "/api/v1/actions/acl/lists/big/rules/bulk", map[string]any{"op": "move", "sequences": []int{n * 10}, "to": 15})
	})
	timed("GET rules?page=1&pageSize=100 (candidate, after the edits)", func() resp {
		return a.call("GET", "/api/v1/state/acl/lists/big/rules?page=1&pageSize=100", nil)
	})
	timed("GET state/acl/lists", func() resp { return a.call("GET", "/api/v1/state/acl/lists", nil) })
	timed("export.csv (candidate)", func() resp { return a.call("GET", "/api/v1/actions/acl/export.csv?list=big&source=candidate", nil) })
	a.must(200, "POST", "/api/v1/config/discard", nil)
}
