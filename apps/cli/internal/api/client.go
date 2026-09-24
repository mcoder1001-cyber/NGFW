// Package api is the CLI's REST client for vrx-api (/api/v1). Requests are made only by operationId through the
// generated Operations table (operations_gen.go, from the OpenAPI document), never with hand-written URLs.
//
// Credentials are an API key (`Authorization: ApiKey …`) or a bearer token from POST /auth/login. They are never
// written to logs: Debug output shows method, path and status only.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Operation is one REST operation of the OpenAPI document.
type Operation struct {
	ID          string
	Method      string
	Path        string
	Summary     string
	PathParams  []string
	QueryParams []string
	Body        bool
}

// Credential produces the Authorization header value.
type Credential interface {
	Authorization() string
	Kind() string
}

// Key authenticates with a vrxk_… key.
type Key string

// Authorization implements Credential.
func (k Key) Authorization() string { return "ApiKey " + string(k) }

// Kind implements Credential.
func (Key) Kind() string { return "apikey" }

// Bearer authenticates with an access token from /auth/login.
type Bearer string

// Authorization implements Credential.
func (b Bearer) Authorization() string { return "Bearer " + string(b) }

// Kind implements Credential.
func (Bearer) Kind() string { return "jwt" }

// Client talks to one API instance.
type Client struct {
	Base  *url.URL
	HTTP  *http.Client
	Cred  Credential
	Debug io.Writer
	// Refresh is called once on a 401 when set (interactive sessions); it returns a fresh credential.
	Refresh func(ctx context.Context) (Credential, error)
	// Timeout bounds a request, ApplyTimeout a commit/rollback/confirm/validate (0 = DefaultTimeout/ApplyTimeout).
	Timeout, ApplyTimeout time.Duration
}

const (
	// DefaultTimeout bounds every request that does not wait for the data plane.
	DefaultTimeout = 90 * time.Second
	// ApplyTimeout bounds commit, rollback, confirm and validate (TD-10a, review 2.4a): ABOVE the server's commit
	// budget (111 s worst case, apps/api/src/commit/budget.ts), so the CLI never gives up on a commit the server is
	// still finishing. If it does run out, the client looks the outcome up (see Outcome).
	ApplyTimeout = 150 * time.Second
)

// applyOps wait for the agent (validate: DryRun; the others: Apply).
var applyOps = map[string]bool{"Config_commit": true, "Config_rollback": true, "Config_confirm": true, "Config_validate": true}

// outcomeOps change the data plane: a timeout leaves their outcome open, so the client asks the API what happened.
var outcomeOps = map[string]bool{"Config_commit": true, "Config_rollback": true, "Config_confirm": true}

// New returns a client for base (e.g. http://127.0.0.1:3000).
func New(base string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid API URL %q (want http(s)://host:port)", base)
	}
	// review 5.7b: redirects are never followed. vrx-api does not redirect; a 3xx comes from something in between
	// (nginx :80 → https, a proxy) and following it would carry the Authorization header to another scheme or port
	// of the same host, or replay a 307/308 body (passwords) to wherever it points.
	return &Client{Base: u, HTTP: &http.Client{CheckRedirect: noRedirect}}, nil
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// Call describes one request.
type Call struct {
	Op     string            // operationId
	Params map[string]string // path parameters; "path" is a JSON pointer without the leading "/", already escaped per segment
	Query  url.Values
	Body   any    // marshalled as JSON when non-nil
	NoAuth bool   // send no Authorization header (login)
	Cookie string // Cookie header (the refresh cookie for POST /auth/refresh)
}

// Response is a raw API answer.
type Response struct {
	Status  int
	Header  http.Header
	Body    []byte
	Cookies []*http.Cookie
}

// Problem is an RFC 9457 problem document.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	Errors []struct {
		Pointer string `json:"pointer"`
		Message string `json:"message"`
		Rule    string `json:"rule,omitempty"`
	} `json:"errors,omitempty"`
	Raw map[string]any `json:"-"`
}

// Error is a non-2xx answer or a transport failure.
type Error struct {
	Op      Operation
	Status  int // 0 = no answer (connection refused, timeout)
	Problem *Problem
	Err     error
	// Outcome is what the API reported after a commit/rollback/confirm timed out (nil: not looked up or unknown).
	Outcome *Outcome
}

// Outcome is the state looked up after a commit, rollback or confirm got no answer in time (TD-10a, review 2.4a):
// the server may well have finished it.
type Outcome struct {
	Sent time.Time
	// Pending is the commit waiting for confirmation, if any.
	Pending *struct {
		TxnID     string `json:"txnId"`
		Deadline  string `json:"deadline"`
		CreatedAt string `json:"createdAt"`
	}
	// Sync is whether running and the data plane agree (in-sync / unknown / degraded).
	Sync *struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	// Newest is the newest revision.
	Newest *struct {
		ID        int     `json:"id"`
		TxnID     *string `json:"txnId"`
		Kind      string  `json:"kind"`
		CreatedAt string  `json:"createdAt"`
	}
	Err error // the lookup itself failed
}

func (o *Outcome) String() string {
	if o.Err != nil {
		return fmt.Sprintf("the outcome could not be looked up either (%v): check `show system` and `show revisions` before trying again", o.Err)
	}
	var parts []string
	if o.Pending != nil {
		parts = append(parts, fmt.Sprintf("commit %s IS pending (applied, reverts at %s unless confirmed — created %s)", o.Pending.TxnID, o.Pending.Deadline, o.Pending.CreatedAt))
	} else {
		parts = append(parts, "no commit is pending")
	}
	if o.Newest != nil {
		txn := "-"
		if o.Newest.TxnID != nil {
			txn = *o.Newest.TxnID
		}
		newer := ""
		if t, err := time.Parse(time.RFC3339Nano, o.Newest.CreatedAt); err == nil && !t.Before(o.Sent.Add(-2*time.Second)) {
			newer = ", created after this request was sent: it was probably applied"
		}
		parts = append(parts, fmt.Sprintf("newest revision %d (%s, txn %s, %s%s)", o.Newest.ID, o.Newest.Kind, txn, o.Newest.CreatedAt, newer))
	}
	if o.Sync != nil {
		s := "sync " + o.Sync.State
		if o.Sync.Reason != "" {
			s += " (" + o.Sync.Reason + ")"
		}
		parts = append(parts, s)
	}
	return "the API reports: " + strings.Join(parts, "; ")
}

func (e *Error) Error() string {
	if e.Status == 0 && e.Outcome != nil {
		return fmt.Sprintf("no answer to %s %s in time (%v) — the server may still have finished it; %s", e.Op.Method, e.Op.Path, e.Err, e.Outcome)
	}
	if e.Status == 0 {
		return fmt.Sprintf("API unreachable (%s %s): %v", e.Op.Method, e.Op.Path, e.Err)
	}
	if e.Problem == nil && e.Err != nil {
		return fmt.Sprintf("%d %s: %v", e.Status, http.StatusText(e.Status), e.Err)
	}
	if e.Problem != nil {
		msg := e.Problem.Title
		if e.Problem.Detail != "" {
			msg = e.Problem.Detail
		}
		return fmt.Sprintf("%d %s", e.Status, msg)
	}
	return fmt.Sprintf("%d %s", e.Status, http.StatusText(e.Status))
}

// Unwrap exposes the transport error.
func (e *Error) Unwrap() error { return e.Err }

// Lookup returns the operation for id; unknown ids are programming errors caught by the table test.
func Lookup(id string) (Operation, error) {
	op, ok := Operations[id]
	if !ok {
		return Operation{}, fmt.Errorf("operation %q is not in the OpenAPI document (regenerate: make -C apps/cli gen)", id)
	}
	return op, nil
}

// URL builds the request URL of c.
func (c *Client) URL(call Call) (string, Operation, error) {
	op, err := Lookup(call.Op)
	if err != nil {
		return "", op, err
	}
	p := op.Path
	for _, name := range op.PathParams {
		v, ok := call.Params[name]
		if !ok {
			return "", op, fmt.Errorf("%s: missing path parameter %q", op.ID, name)
		}
		p = strings.Replace(p, "{"+name+"}", escapePathParam(name, v), 1)
	}
	for name := range call.Query {
		if !contains(op.QueryParams, name) {
			return "", op, fmt.Errorf("%s: %q is not a query parameter of %s %s", op.ID, name, op.Method, op.Path)
		}
	}
	u := c.Base.String() + p
	if len(call.Query) > 0 {
		u += "?" + call.Query.Encode()
	}
	return u, op, nil
}

// escapePathParam percent-encodes a parameter; the `path` parameter of the config routes is a pointer whose
// segments are escaped one by one (the "/" between them stays).
func escapePathParam(name, v string) string {
	if name != "path" {
		return url.PathEscape(v)
	}
	segs := strings.Split(v, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Do sends call; a non-2xx status is returned as *Error (the response is returned too). A commit, rollback or
// confirm that gets no answer in time comes back with Error.Outcome: what the API says happened meanwhile.
func (c *Client) Do(ctx context.Context, call Call) (*Response, error) {
	sent := time.Now()
	resp, err := c.do(ctx, call)
	if err != nil {
		var ae *Error
		if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized && c.Refresh != nil && !call.NoAuth {
			cred, rerr := c.Refresh(ctx)
			if rerr == nil {
				c.Cred = cred
				sent = time.Now()
				resp, err = c.do(ctx, call)
			}
		}
	}
	var ae *Error
	if err != nil && errors.As(err, &ae) && ae.Status == 0 && outcomeOps[ae.Op.ID] && isTimeout(ae.Err) && ctx.Err() == nil {
		ae.Outcome = c.lookupOutcome(ctx, sent)
	}
	return resp, err
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne interface{ Timeout() bool }
	return errors.As(err, &ne) && ne.Timeout()
}

// lookupOutcome asks the API what became of a commit-like request that timed out: the pending commit and sync
// state (GET /state/system) and the newest revision (GET /config/revisions?limit=1).
func (c *Client) lookupOutcome(ctx context.Context, sent time.Time) *Outcome {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	o := &Outcome{Sent: sent}
	var sys struct {
		PendingCommit *struct {
			TxnID     string `json:"txnId"`
			Deadline  string `json:"deadline"`
			CreatedAt string `json:"createdAt"`
		} `json:"pendingCommit"`
		Sync *struct {
			State  string `json:"state"`
			Reason string `json:"reason"`
		} `json:"sync"`
	}
	if _, err := c.JSON(ctx, Call{Op: "State_system"}, &sys); err != nil {
		o.Err = err
		return o
	}
	o.Pending, o.Sync = sys.PendingCommit, sys.Sync
	var revs struct {
		Items []struct {
			ID        int     `json:"id"`
			TxnID     *string `json:"txnId"`
			Kind      string  `json:"kind"`
			CreatedAt string  `json:"createdAt"`
		} `json:"items"`
	}
	if _, err := c.JSON(ctx, Call{Op: "Config_revisions", Query: url.Values{"limit": {"1"}}}, &revs); err != nil {
		o.Err = err
		return o
	}
	if len(revs.Items) > 0 {
		n := revs.Items[0]
		o.Newest = &n
	}
	return o
}

func (c *Client) timeoutFor(op Operation) time.Duration {
	if applyOps[op.ID] {
		if c.ApplyTimeout > 0 {
			return c.ApplyTimeout
		}
		return ApplyTimeout
	}
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

// redirectError: a 3xx is never followed (review 5.7b); name where it pointed so the operator can decide.
func redirectError(h http.Header) error {
	loc := h.Get("Location")
	if loc == "" {
		loc = "(no Location)"
	}
	return fmt.Errorf("the server answered a redirect to %s, which the CLI does not follow (credentials and bodies never go to a redirect target) — if that is the API, use it as --api / VRX_API_URL", loc)
}

func (c *Client) do(ctx context.Context, call Call) (*Response, error) {
	u, op, err := c.URL(call)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if call.Body != nil {
		b, err := json.Marshal(call.Body)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeoutFor(op))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, op.Method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, application/problem+json")
	req.Header.Set("User-Agent", "vrx-cli")
	if call.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Cred != nil && !call.NoAuth {
		req.Header.Set("Authorization", c.Cred.Authorization())
	}
	if call.Cookie != "" {
		req.Header.Set("Cookie", call.Cookie)
	}
	start := time.Now()
	res, err := c.HTTP.Do(req)
	if err != nil {
		if c.Debug != nil {
			_, _ = fmt.Fprintf(c.Debug, "debug: %s %s → error after %s\n", op.Method, req.URL.Path, time.Since(start).Round(time.Millisecond))
		}
		return nil, &Error{Op: op, Err: err}
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, &Error{Op: op, Err: err}
	}
	if c.Debug != nil {
		// never headers or bodies: they carry credentials
		_, _ = fmt.Fprintf(c.Debug, "debug: %s %s → %d (%s)\n", op.Method, req.URL.EscapedPath(), res.StatusCode, time.Since(start).Round(time.Millisecond))
	}
	r := &Response{Status: res.StatusCode, Header: res.Header, Body: data, Cookies: res.Cookies()}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return r, nil
	}
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return r, &Error{Op: op, Status: res.StatusCode, Err: redirectError(res.Header)}
	}
	e := &Error{Op: op, Status: res.StatusCode}
	var p Problem
	if json.Unmarshal(data, &p) == nil && (p.Title != "" || p.Status != 0) {
		_ = json.Unmarshal(data, &p.Raw)
		e.Problem = &p
	}
	return r, e
}

// JSON sends call and decodes a 2xx body into out (when out is non-nil).
func (c *Client) JSON(ctx context.Context, call Call, out any) (*Response, error) {
	r, err := c.Do(ctx, call)
	if err != nil {
		return r, err
	}
	if out != nil && len(r.Body) > 0 {
		if err := json.Unmarshal(r.Body, out); err != nil {
			return r, fmt.Errorf("decoding %s answer: %w", call.Op, err)
		}
	}
	return r, nil
}

// Raw fetches a non-operation document (the OpenAPI JSON at /api/docs-json) with the client's credential.
func (c *Client) Raw(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeoutFor(Operation{}))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base.String()+path, nil)
	if err != nil {
		return nil, err
	}
	if c.Cred != nil {
		req.Header.Set("Authorization", c.Cred.Authorization())
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &Error{Op: Operation{Method: "GET", Path: path}, Err: err}
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return nil, &Error{Op: Operation{Method: "GET", Path: path}, Status: res.StatusCode, Err: redirectError(res.Header)}
	}
	if res.StatusCode != http.StatusOK {
		e := &Error{Op: Operation{Method: "GET", Path: path}, Status: res.StatusCode}
		var p Problem
		if json.Unmarshal(data, &p) == nil && p.Title != "" {
			e.Problem = &p
		}
		return nil, e
	}
	return data, nil
}
