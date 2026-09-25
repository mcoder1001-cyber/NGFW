// Package reachability is the DoD check of review items 6.1a/6.6b ("reachable from the desired
// state through the API"): it drives the product API the way an operator does — edit the
// candidate, commit — and then asks the API whether the agent really applied the change.
//
// Check passes only when, for one change:
//   - POST /api/v1/config/commit answers status "applied" and notApplied [];
//   - no warning of the commit carries rule "agent.unsupported-field" or "agent.unimplemented-domain"
//     at (or above/below) the edited pointer — the agent accepted the leaf but has no descriptor for it;
//   - GET /api/v1/state/drift (running vs the agent's Retrieve, proto.md §5) reports no change and
//     does not list the pointer as ignored, and the pointer's domain is one the agent retrieves:
//     Retrieve() == desired.
//
// It uses the standard library only, so any integration test (Go module under test/) can import it.
package reachability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client talks to the product API.
type Client struct {
	BaseURL string // e.g. https://127.0.0.1:8443
	// Authorization is the full Authorization header value ("Bearer …"); never logged.
	Authorization string
	HTTP          *http.Client // nil = http.DefaultClient
}

// Issue is a validation warning (config.controller.ts Issue).
type Issue struct {
	Pointer string `json:"pointer"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
}

// CommitResult is the subset of CommitOut the check reads.
type CommitResult struct {
	Status     string   `json:"status"`
	TxnID      string   `json:"txnId,omitempty"`
	Warnings   []Issue  `json:"warnings"`
	NotApplied []string `json:"notApplied"`
	Results    []struct {
		Key     string `json:"key"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Pointer string `json:"pointer"`
	} `json:"results"`
}

// Drift is DriftOut of GET /api/v1/state/drift.
type Drift struct {
	Subsystems []string `json:"subsystems"`
	Changes    []struct {
		Op      string `json:"op"`
		Pointer string `json:"pointer"`
	} `json:"changes"`
	Ignored []struct {
		Pointer string `json:"pointer"`
		Rule    string `json:"rule"`
	} `json:"ignored"`
}

// coverageRules are the agent's "not managed by this build" warnings (state.controller.ts COVERAGE_RULES).
var coverageRules = map[string]bool{"agent.unsupported-field": true, "agent.unimplemented-domain": true}

// Check sets pointer to value in the candidate (PUT, whole node), commits, and verifies the change
// reached the data plane (see the package comment). It returns the commit result for further checks.
func (c *Client) Check(ctx context.Context, pointer string, value any) (*CommitResult, error) {
	if !strings.HasPrefix(pointer, "/") || len(pointer) < 2 {
		return nil, fmt.Errorf("reachability: pointer %q must be a non-root JSON pointer", pointer)
	}
	if err := c.do(ctx, http.MethodPut, "/api/v1/config"+pointer, value, nil); err != nil {
		return nil, fmt.Errorf("edit candidate %s: %w", pointer, err)
	}
	var cr CommitResult
	if err := c.do(ctx, http.MethodPost, "/api/v1/config/commit?comment="+url.QueryEscape("reachability "+pointer), nil, &cr); err != nil {
		return nil, fmt.Errorf("commit %s: %w", pointer, err)
	}
	if err := VerifyCommit(pointer, &cr); err != nil {
		return &cr, err
	}
	var d Drift
	if err := c.do(ctx, http.MethodGet, "/api/v1/state/drift", nil, &d); err != nil {
		return &cr, fmt.Errorf("drift after %s: %w", pointer, err)
	}
	return &cr, VerifyDrift(pointer, &d)
}

// VerifyCommit is the commit half of Check.
func VerifyCommit(pointer string, cr *CommitResult) error {
	if cr.Status != "applied" {
		return fmt.Errorf("%s: commit status %q, want \"applied\" (results %+v)", pointer, cr.Status, cr.Results)
	}
	if len(cr.NotApplied) != 0 {
		return fmt.Errorf("%s: commit notApplied %v, want [] (the agent build does not implement that domain)", pointer, cr.NotApplied)
	}
	for _, w := range cr.Warnings {
		if coverageRules[w.Rule] && related(pointer, w.Pointer) {
			return fmt.Errorf("%s: commit warning %s at %s: %s (accepted but not applied)", pointer, w.Rule, w.Pointer, w.Message)
		}
	}
	return nil
}

// VerifyDrift is the Retrieve()==desired half of Check.
func VerifyDrift(pointer string, d *Drift) error {
	dom := domainOf(pointer)
	found := false
	for _, s := range d.Subsystems {
		found = found || s == dom
	}
	if !found {
		return fmt.Errorf("%s: domain %q is not retrieved by the agent (subsystems %v)", pointer, dom, d.Subsystems)
	}
	for _, i := range d.Ignored {
		if related(pointer, i.Pointer) && (i.Rule == "agent.unimplemented-domain" || depth(i.Pointer) >= 2) {
			return fmt.Errorf("%s: drift ignores %s (%s): Retrieve cannot prove it", pointer, i.Pointer, i.Rule)
		}
	}
	if len(d.Changes) != 0 {
		ps := make([]string, 0, len(d.Changes))
		for _, ch := range d.Changes {
			ps = append(ps, ch.Op+" "+ch.Pointer)
		}
		return fmt.Errorf("%s: Retrieve() != desired after commit: %v", pointer, ps)
	}
	return nil
}

// related reports whether a is b, or one is an ancestor of the other.
func related(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func domainOf(p string) string {
	seg := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 2)[0]
	return strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
}

func depth(p string) int { return strings.Count(p, "/") }

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, application/problem+json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Authorization != "" {
		req.Header.Set("Authorization", c.Authorization)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, truncate(string(b), 512))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
