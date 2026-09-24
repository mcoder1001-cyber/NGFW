// Package client is the provider's small HTTP client for the VRX REST API: API-key auth, TLS verification on by
// default, RFC 9457 problem+json → *APIError with pointers, and the candidate → commit(confirm) → confirm workflow.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Options configure a Client.
type Options struct {
	URL       string
	APIKey    string // sent as `Authorization: ApiKey <key>`; never logged
	Insecure  bool   // skip TLS verification (lab boxes with self-signed certificates only)
	CAFile    string // extra CA bundle (PEM)
	Timeout   time.Duration
	UserAgent string
	AllowHTTP bool // permit plain http:// to a non-loopback host (the API key then crosses the network in clear)
}

// Client talks to one appliance. Mutations are serialised by Committer (one candidate per appliance).
type Client struct {
	base   string
	key    string
	http   *http.Client
	ua     string
	commit sync.Mutex
}

// FieldError is one `errors[]` entry of a problem.
type FieldError struct {
	Pointer string `json:"pointer"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
}

// APIError is a non-2xx answer (RFC 9457 problem+json).
type APIError struct {
	Status  int
	Method  string
	Path    string
	Title   string          `json:"title"`
	Detail  string          `json:"detail"`
	Errors  []FieldError    `json:"errors"`
	Lock    json.RawMessage `json:"lock,omitempty"`
	Results json.RawMessage `json:"results,omitempty"`
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s → %d %s", e.Method, e.Path, e.Status, e.Title)
	if e.Detail != "" {
		b.WriteString(": " + e.Detail)
	}
	for _, fe := range e.Errors {
		fmt.Fprintf(&b, "\n  %s: %s", orRoot(fe.Pointer), fe.Message)
	}
	if len(e.Lock) > 0 && string(e.Lock) != "null" {
		b.WriteString("\n  lock: " + string(e.Lock))
	}
	return b.String()
}

func orRoot(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

// IsNotFound reports a 404 answer.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// New builds a Client.
func New(o Options) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(o.URL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("url must be http(s)://host[:port], got %q", o.URL)
	}
	if u.User != nil {
		return nil, errors.New("credentials in the URL are not accepted; use api_key")
	}
	if u.Scheme == "http" && !o.AllowHTTP && !IsLoopback(u.Hostname()) {
		return nil, fmt.Errorf("refusing plain http:// to %s: the API key would cross the network in clear — use https:// (or set allow_http = true for an isolated lab)", u.Hostname())
	}
	if o.APIKey == "" {
		return nil, errors.New("no API key (api_key or VRX_API_KEY)")
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if o.CAFile != "" {
		pem, err := os.ReadFile(o.CAFile)
		if err != nil {
			return nil, fmt.Errorf("ca_file: %w", err)
		}
		pool, _ := x509.SystemCertPool()
		if pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_file %s: no PEM certificates", o.CAFile)
		}
		tlsCfg.RootCAs = pool
	}
	tlsCfg.InsecureSkipVerify = o.Insecure //nolint:gosec // explicit opt-in (insecure = true), default false
	timeout := o.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	ua := o.UserAgent
	if ua == "" {
		ua = "terraform-provider-vrx"
	}
	return &Client{
		base: u.String(),
		key:  o.APIKey,
		ua:   ua,
		http: &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg, Proxy: http.ProxyFromEnvironment},
			// never follow redirects: the Authorization header must not travel to another origin
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// IsLoopback reports localhost / 127.0.0.0/8 / ::1.
func IsLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// String never shows the key.
func (c *Client) String() string { return "vrx.Client(" + c.base + ")" }

// Do performs one request; body nil = no body (and no content-type). out may be nil.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "ApiKey "+c.key)
	req.Header.Set("Accept", "application/json, application/problem+json")
	req.Header.Set("User-Agent", c.ua)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error carries the URL only (no headers) — safe to surface
		return fmt.Errorf("%s %s: %w", method, path, unwrapURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || len(data) == 0 {
			return nil
		}
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		return d.Decode(out)
	}
	ae := &APIError{Status: resp.StatusCode, Method: method, Path: path}
	if json.Unmarshal(data, ae) != nil || ae.Title == "" {
		ae.Title = http.StatusText(resp.StatusCode)
		if len(data) > 0 && ae.Detail == "" {
			ae.Detail = truncate(string(data), 200)
		}
	}
	return ae
}

func unwrapURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---- JSON pointers

// EscapeToken escapes one RFC 6901 token.
func EscapeToken(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// NormalizePointer accepts `/a/b`, `a/b` or `a/b/` and returns `/a/b`; validates `~` escapes.
func NormalizePointer(p string) (string, error) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return "", errors.New("a pointer below the document root is required (e.g. /interfaces/loop1)")
	}
	for _, seg := range strings.Split(p, "/") {
		for i := 0; i < len(seg); i++ {
			if seg[i] == '~' && (i+1 >= len(seg) || (seg[i+1] != '0' && seg[i+1] != '1')) {
				return "", fmt.Errorf("invalid JSON pointer escape in %q", seg)
			}
		}
		if seg == "" {
			return "", fmt.Errorf("empty token in pointer %q", "/"+p)
		}
	}
	return "/" + p, nil
}

// URLPath maps a normalized pointer to the `/api/v1/config/{path}` URL part (each token percent-encoded, `~` kept).
func URLPath(pointer string) string {
	segs := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i, s := range segs {
		segs[i] = strings.ReplaceAll(url.PathEscape(s), "%7E", "~")
	}
	return strings.Join(segs, "/")
}

// ---- configuration

// Running returns the running node at pointer (redacted by the API). 404 → IsNotFound.
func (c *Client) Running(ctx context.Context, pointer string) (any, error) {
	var v any
	return v, c.Do(ctx, http.MethodGet, "/api/v1/config/"+URLPath(pointer), nil, nil, &v)
}

// Put replaces (or creates) the candidate node at pointer.
func (c *Client) Put(ctx context.Context, pointer string, value any) error {
	if value == nil {
		value = json.RawMessage("null")
	}
	return c.Do(ctx, http.MethodPut, "/api/v1/config/"+URLPath(pointer), nil, value, nil)
}

// Delete removes the candidate node at pointer.
func (c *Client) Delete(ctx context.Context, pointer string) error {
	return c.Do(ctx, http.MethodDelete, "/api/v1/config/"+URLPath(pointer), nil, nil, nil)
}

// Change is one entry of GET /config/diff.
type Change struct {
	Op      string `json:"op"`
	Pointer string `json:"pointer"`
}

// Diff returns the candidate ↔ running changes.
func (c *Client) Diff(ctx context.Context) ([]Change, error) {
	var d struct {
		Changes []Change `json:"changes"`
	}
	return d.Changes, c.Do(ctx, http.MethodGet, "/api/v1/config/diff", nil, nil, &d)
}

// LockOwner returns the candidate lock owner ("" when unlocked).
func (c *Client) LockOwner(ctx context.Context) (string, error) {
	var l struct {
		Owner *string `json:"owner"`
	}
	if err := c.Do(ctx, http.MethodGet, "/api/v1/config/lock", nil, nil, &l); err != nil || l.Owner == nil {
		return "", err
	}
	return *l.Owner, nil
}

// Discard drops the candidate and releases the lock.
func (c *Client) Discard(ctx context.Context) error {
	return c.Do(ctx, http.MethodPost, "/api/v1/config/discard", nil, nil, nil)
}

// CommitResult is the answer of commit / confirm / rollback.
type CommitResult struct {
	Status          string `json:"status"`
	TxnID           string `json:"txnId"`
	ConfirmDeadline string `json:"confirmDeadline"`
	Revision        *struct {
		ID int64 `json:"id"`
	} `json:"revision"`
	Warnings   []FieldError `json:"warnings"`
	NotApplied []string     `json:"notApplied"`
	Sync       *SyncState   `json:"sync"`
}

// SyncState is the API's running ↔ data-plane agreement (P06 D-P06-14).
type SyncState struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

// NotEnforced reports a commit that was stored but not (fully) enforced by the agent (P06 D-P06-15).
func (r *CommitResult) NotEnforced() bool {
	return r != nil && (r.Status == "partially-applied" || r.Status == "not-applied" || len(r.NotApplied) > 0)
}

// Commit applies the candidate; confirmSec > 0 makes it a confirmed commit (status "pending").
func (c *Client) Commit(ctx context.Context, confirmSec int64, comment string) (*CommitResult, error) {
	q := url.Values{}
	if confirmSec > 0 {
		q.Set("confirm", strconv.FormatInt(confirmSec, 10))
	}
	if comment != "" {
		q.Set("comment", comment)
	}
	var r CommitResult
	return &r, c.Do(ctx, http.MethodPost, "/api/v1/config/commit", q, nil, &r)
}

// Confirm confirms the pending commit.
func (c *Client) Confirm(ctx context.Context) (*CommitResult, error) {
	var r CommitResult
	return &r, c.Do(ctx, http.MethodPost, "/api/v1/config/commit/confirm", nil, nil, &r)
}

// State reads /api/v1/state/<name>.
func (c *Client) State(ctx context.Context, name string, query url.Values) (any, error) {
	var v any
	return v, c.Do(ctx, http.MethodGet, "/api/v1/state/"+strings.Trim(name, "/"), query, nil, &v)
}

// ---- the one write path of the provider

// SystemState is the part of GET /state/system the write path needs.
type SystemState struct {
	Sync          *SyncState      `json:"sync"`
	PendingCommit json.RawMessage `json:"pendingCommit"`
}

// System reads /state/system.
func (c *Client) System(ctx context.Context) (*SystemState, error) {
	var s SystemState
	return &s, c.Do(ctx, http.MethodGet, "/api/v1/state/system", nil, nil, &s)
}

// Candidate returns the candidate node at pointer (redacted). 404 → IsNotFound.
func (c *Client) Candidate(ctx context.Context, pointer string) (any, error) {
	var v any
	return v, c.Do(ctx, http.MethodGet, "/api/v1/config/candidate/"+URLPath(pointer), nil, nil, &v)
}

var (
	// ErrDirtyCandidate: the candidate holds changes this run did not make.
	ErrDirtyCandidate = errors.New("dirty candidate")
	// ErrUnsynced: running and the data plane do not agree (sync unknown/degraded) or a commit is pending.
	ErrUnsynced = errors.New("appliance not in sync")
	// ErrConcurrent: the candidate changed under us (another run of the same user, D-093).
	ErrConcurrent = errors.New("candidate modified concurrently")
)

// ApplyOptions tune one Apply.
type ApplyOptions struct {
	ConfirmSec    int64
	Comment       string
	AllowUnsynced bool // edit/confirm even when /state/system reports sync ≠ in-sync
}

// Edit is one change confined to one subtree of the document.
type Edit struct {
	Pointer string                          // every change this edit makes is at or below this pointer
	Want    any                             // the node after the edit (nil + Absent = removed)
	Absent  bool                            // the edit removes the node
	Do      func(ctx context.Context) error // the candidate edit (PUT/DELETE)
}

func within(p, root string) bool { return p == root || strings.HasPrefix(p, root+"/") }

// ownChanges reports whether every candidate change lies inside root; it returns the first foreign one otherwise.
func ownChanges(changes []Change, root string) (bool, string) {
	for _, ch := range changes {
		if !within(ch.Pointer, root) {
			return false, ch.Op + " " + orRoot(ch.Pointer)
		}
	}
	return true, ""
}

// Apply runs one edit against the candidate and commits it, serialised per client (Terraform applies resources in
// parallel; the appliance has one candidate per user). Steps:
//  1. /state/system: sync must be in-sync and no confirmed commit pending (unless AllowUnsynced);
//  2. the candidate must be clean — or hold exactly this edit already (our own leftover after an auto-revert);
//  3. edit; the resulting changes must all lie inside e.Pointer (anything else = a concurrent run: fail, touch nothing);
//  4. commit (?confirm when ConfirmSec > 0); the pending answer and /state/system must say in-sync; confirm.
//
// A candidate is only ever discarded when every change in it lies inside e.Pointer (never someone else's edits).
func (c *Client) Apply(ctx context.Context, o ApplyOptions, e Edit) (*CommitResult, error) {
	c.commit.Lock()
	defer c.commit.Unlock()
	if !o.AllowUnsynced {
		if err := c.requireSynced(ctx, "before editing"); err != nil {
			return nil, err
		}
	}
	changes, err := c.Diff(ctx)
	if err != nil {
		return nil, err
	}
	if len(changes) > 0 && !c.ownLeftover(ctx, changes, e) {
		owner, _ := c.LockOwner(ctx)
		return nil, fmt.Errorf("%w: the candidate already has %d uncommitted change(s) (first %s %s, lock owner %q) — "+
			"commit or discard them (POST /api/v1/config/discard) before running again; nothing was changed", ErrDirtyCandidate,
			len(changes), changes[0].Op, orRoot(changes[0].Pointer), owner)
	}
	if err := e.Do(ctx); err != nil {
		c.discardIfOwn(e.Pointer)
		return nil, err
	}
	if changes, err = c.Diff(ctx); err != nil {
		return nil, err
	}
	if ok, foreign := ownChanges(changes, e.Pointer); !ok {
		return nil, fmt.Errorf("%w: the candidate now also contains %s — another run with the same user is editing "+
			"(use one service user per pipeline); nothing was committed and the candidate was left as it is", ErrConcurrent, foreign)
	}
	if len(changes) == 0 {
		c.discardIfOwn(e.Pointer)
		return &CommitResult{Status: "unchanged"}, nil
	}
	r, err := c.Commit(ctx, o.ConfirmSec, o.Comment)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && (ae.Status == http.StatusBadGateway || ae.Status == http.StatusGatewayTimeout) {
			// running-unknown: the API reconciles on its own and may still promote this candidate — do not discard it
			return nil, fmt.Errorf("the commit outcome is UNKNOWN (the API is reconciling; check /state/system and refresh before retrying): %w", err)
		}
		c.discardIfOwn(e.Pointer)
		return nil, err
	}
	if r.Status != "pending" {
		return r, nil
	}
	notConfirmed := func(why string) error {
		return fmt.Errorf("commit NOT confirmed (%s) — the appliance reverts it at %s. Afterwards the candidate still holds "+
			"this edit: re-running recognises and re-uses it, or POST /api/v1/config/discard", why, r.ConfirmDeadline)
	}
	if r.Sync != nil && r.Sync.State != "in-sync" && !o.AllowUnsynced {
		return r, notConfirmed("sync is " + r.Sync.State + ": " + r.Sync.Reason)
	}
	if !o.AllowUnsynced {
		if err := c.requireSynced(ctx, "after the commit"); err != nil && !errors.Is(err, errPending) {
			return r, notConfirmed(err.Error())
		}
	} else if _, err := c.System(ctx); err != nil {
		return r, notConfirmed("post-commit check failed: " + err.Error())
	}
	cr, err := c.Confirm(ctx)
	if err != nil {
		return r, notConfirmed("confirm failed: " + err.Error())
	}
	// the confirm answer carries no notApplied; the pending answer knows which domains are not enforced
	cr.NotApplied = append(cr.NotApplied, r.NotApplied...)
	return cr, nil
}

var errPending = errors.New("a confirmed commit is pending")

func (c *Client) requireSynced(ctx context.Context, when string) error {
	s, err := c.System(ctx)
	if err != nil {
		return fmt.Errorf("post-commit check failed: %w", err)
	}
	if s.Sync != nil && s.Sync.State != "in-sync" {
		return fmt.Errorf("%w %s: running ↔ data plane sync is %q (%s) — wait for the API's reconcile or set allow_unsynced",
			ErrUnsynced, when, s.Sync.State, s.Sync.Reason)
	}
	if len(s.PendingCommit) > 0 && string(s.PendingCommit) != "null" {
		return fmt.Errorf("%w (%w) %s — confirm or let it revert first", ErrUnsynced, errPending, when)
	}
	return nil
}

// ownLeftover: the candidate holds only changes inside e.Pointer and the node there already equals what this edit
// wants — the leftover of an earlier run of the same edit whose confirmed commit was reverted (L4).
func (c *Client) ownLeftover(ctx context.Context, changes []Change, e Edit) bool {
	if ok, _ := ownChanges(changes, e.Pointer); !ok {
		return false
	}
	got, err := c.Candidate(ctx, e.Pointer)
	if e.Absent {
		return IsNotFound(err)
	}
	if err != nil {
		return false
	}
	return jsonEquivalent(got, e.Want)
}

func jsonEquivalent(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	var x, y any
	_ = json.Unmarshal(ja, &x)
	_ = json.Unmarshal(jb, &y)
	return reflect.DeepEqual(x, y)
}

// discardIfOwn drops the candidate only when every change in it lies inside root (never another run's edits).
func (c *Client) discardIfOwn(root string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	changes, err := c.Diff(ctx)
	if err != nil {
		return
	}
	if ok, _ := ownChanges(changes, root); ok && len(changes) > 0 {
		_ = c.Discard(ctx)
	}
}
