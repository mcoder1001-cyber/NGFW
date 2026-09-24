package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"ngfw/cli/internal/api"
	"ngfw/cli/internal/safe"
)

// Exit codes of vrx (documented in docs/user/cli/reference.md, generated from ExitCodes).
const (
	ExitOK             = 0
	ExitError          = 1   // unexpected/internal error
	ExitUsage          = 2   // bad command, arguments or value (rejected before any request)
	ExitAuth           = 3   // not authenticated: no/expired credentials (HTTP 401)
	ExitForbidden      = 4   // authenticated but not allowed (HTTP 403, RBAC)
	ExitNotFound       = 5   // nothing at that path / revision (HTTP 404)
	ExitConflict       = 6   // candidate locked by another user, nothing pending to confirm… (HTTP 409)
	ExitInvalid        = 7   // the API rejected the value/candidate (HTTP 400: schema or semantic validation)
	ExitCommitFailed   = 8   // commit/rollback not applied (HTTP 422) — running unchanged
	ExitUnavailable    = 9   // API unreachable, agent unavailable or outcome unknown (HTTP 5xx except 501)
	ExitNotImplemented = 10  // the API/agent does not implement this yet (HTTP 501, or no REST endpoint)
	ExitRateLimited    = 11  // too many login attempts (HTTP 429)
	ExitInterrupted    = 130 // the command was interrupted (Ctrl-C / SIGTERM)
)

// ExitCodes documents every exit code (the reference page is generated from it).
var ExitCodes = []struct {
	Code    int
	Meaning string
}{
	{ExitOK, "success"},
	{ExitError, "unexpected error (bug, malformed answer)"},
	{ExitUsage, "usage error: unknown command, bad arguments, or a value rejected by the client-side schema check (nothing was sent)"},
	{ExitAuth, "not authenticated: no credentials, wrong password, expired session (HTTP 401)"},
	{ExitForbidden, "permission denied by RBAC (HTTP 403)"},
	{ExitNotFound, "not found: nothing at that path, no such revision (HTTP 404)"},
	{ExitConflict, "conflict: candidate locked by another user, no pending commit to confirm, … (HTTP 409)"},
	{ExitInvalid, "rejected by the API's validation (HTTP 400, problem+json with pointers)"},
	{ExitCommitFailed, "commit or rollback failed and was rolled back; running unchanged (HTTP 422)"},
	{ExitUnavailable, "API unreachable, agent unavailable, or outcome unknown — check `show system` sync state (HTTP 502/503/504/500)"},
	{ExitNotImplemented, "not implemented yet by the API/agent (HTTP 501 or no REST endpoint)"},
	{ExitRateLimited, "login rate limit (HTTP 429)"},
	{ExitInterrupted, "interrupted by Ctrl-C / SIGTERM (in the interactive shell only the running command is cancelled; the shell stays usable)"},
}

// ExitErr carries an exit code and, for API failures, the problem document.
type ExitErr struct {
	Code    int
	Err     error
	Problem *api.Problem
	Status  int
}

func (e *ExitErr) Error() string { return e.Err.Error() }
func (e *ExitErr) Unwrap() error { return e.Err }

func usagef(format string, a ...any) error {
	return &ExitErr{Code: ExitUsage, Err: fmt.Errorf(format, a...)}
}

func notImplemented(format string, a ...any) error {
	return &ExitErr{Code: ExitNotImplemented, Err: fmt.Errorf(format, a...)}
}

// codeForStatus maps an HTTP status to an exit code.
func codeForStatus(status int) int {
	switch {
	case status == 0:
		return ExitUnavailable
	case status == http.StatusBadRequest:
		return ExitInvalid
	case status == http.StatusUnauthorized:
		return ExitAuth
	case status == http.StatusForbidden:
		return ExitForbidden
	case status == http.StatusNotFound:
		return ExitNotFound
	case status == http.StatusConflict:
		return ExitConflict
	case status == http.StatusUnprocessableEntity:
		return ExitCommitFailed
	case status == http.StatusTooManyRequests:
		return ExitRateLimited
	case status == http.StatusNotImplemented:
		return ExitNotImplemented
	case status >= 500:
		return ExitUnavailable
	}
	return ExitError
}

// classify turns any error into an ExitErr.
func classify(err error) *ExitErr {
	var ee *ExitErr
	if errors.As(err, &ee) {
		return ee
	}
	if errors.Is(err, context.Canceled) {
		return &ExitErr{Code: ExitInterrupted, Err: errors.New("interrupted")}
	}
	var ae *api.Error
	if errors.As(err, &ae) {
		return &ExitErr{Code: codeForStatus(ae.Status), Err: ae, Problem: ae.Problem, Status: ae.Status}
	}
	return &ExitErr{Code: ExitError, Err: err}
}

// report prints an error: human text, or one JSON document {"error":{…}} in --json mode.
func report(w io.Writer, err error, asJSON bool) int {
	e := classify(err)
	if asJSON {
		doc := map[string]any{"exitCode": e.Code, "message": e.Error()}
		if e.Status != 0 {
			doc["status"] = e.Status
		}
		if e.Problem != nil {
			doc["problem"] = e.Problem.Raw
		}
		b, _ := json.Marshal(map[string]any{"error": doc})
		fmt.Fprintln(w, string(b))
		return e.Code
	}
	fmt.Fprintf(w, "error: %s\n", safe.String(humanMessage(e)))
	if e.Problem != nil {
		for _, i := range e.Problem.Errors {
			if i.Pointer != "" {
				fmt.Fprintf(w, "  at %s (%s): %s\n", one(i.Pointer), one(pointerWords(i.Pointer)), one(i.Message))
			} else {
				fmt.Fprintf(w, "  %s\n", one(i.Message))
			}
		}
		if r, ok := e.Problem.Raw["results"].([]any); ok {
			for _, x := range r {
				if m, ok := x.(map[string]any); ok && m["code"] != "OK" {
					fmt.Fprintf(w, "  %s %s: %s %s\n", one(m["op"]), one(m["key"]), one(m["code"]), one(m["message"]))
				}
			}
		}
		if s, ok := e.Problem.Raw["sync"].(map[string]any); ok {
			fmt.Fprintf(w, "  sync: %s (%s)\n", one(s["state"]), one(s["reason"]))
		}
		if l, ok := e.Problem.Raw["lock"].(map[string]any); ok && l["owner"] != nil {
			fmt.Fprintf(w, "  candidate locked by %s since %s\n", one(l["owner"]), one(l["lockedAt"]))
		}
	}
	return e.Code
}

// one renders a server-supplied value as one visible line (review H1).
func one(v any) string { return safe.String(fmt.Sprint(v)) }

func humanMessage(e *ExitErr) string {
	switch e.Code {
	case ExitAuth:
		if e.Status == http.StatusUnauthorized {
			return e.Error() + " — log in with `vrx login` or use an API key (VRX_API_KEY / --api-key-file)"
		}
	case ExitForbidden:
		return e.Error() + " — your role does not allow this"
	}
	return e.Error()
}
