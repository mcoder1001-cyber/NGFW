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
}

// New returns a client for base (e.g. http://127.0.0.1:3000).
func New(base string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid API URL %q (want http(s)://host:port)", base)
	}
	return &Client{Base: u, HTTP: &http.Client{Timeout: 90 * time.Second}}, nil
}

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
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("API unreachable (%s %s): %v", e.Op.Method, e.Op.Path, e.Err)
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

// Do sends call; a non-2xx status is returned as *Error (the response is returned too).
func (c *Client) Do(ctx context.Context, call Call) (*Response, error) {
	resp, err := c.do(ctx, call)
	if err != nil {
		var ae *Error
		if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized && c.Refresh != nil && !call.NoAuth {
			cred, rerr := c.Refresh(ctx)
			if rerr == nil {
				c.Cred = cred
				return c.do(ctx, call)
			}
		}
	}
	return resp, err
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
