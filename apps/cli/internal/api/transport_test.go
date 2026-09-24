package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TD-10a (review 5.7b): a redirect is never followed — not to another scheme or port of the same host (the
// Authorization header would go along: Go only strips it for another hostname), and a 307/308 never replays the body
// (passwords) anywhere.

type seen struct {
	mu     sync.Mutex
	auth   []string
	bodies []string
}

func (s *seen) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		s.bodies = append(s.bodies, string(b))
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
}

func (s *seen) requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.auth)
}

func TestRedirectToOtherSchemeAndPortOfSameHostIsNotFollowed(t *testing.T) {
	var target seen
	plain := httptest.NewServer(target.handler()) // http://127.0.0.1:<p2>
	defer plain.Close()
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+r.URL.Path, http.StatusFound) // https → http, same host, other port
	}))
	defer api.Close()
	c, err := New(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP.Transport = api.Client().Transport
	c.Cred = Key("vrxk_VRX_TEST_KEY_TD10A")
	_, err = c.Do(context.Background(), Call{Op: "Config_running"})
	if n := target.requests(); n != 0 {
		t.Fatalf("redirect followed: the http:// target got %d request(s), Authorization %q", n, target.auth)
	}
	var ae *Error
	if !errors.As(err, &ae) || ae.Status != http.StatusFound || !strings.Contains(err.Error(), plain.URL) {
		t.Fatalf("want a 302 error naming %s, got %v", plain.URL, err)
	}
}

func TestRedirect307And308NeverReplayTheBody(t *testing.T) {
	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		var target seen
		other := httptest.NewServer(target.handler())
		// another hostname for the same listener: Go would drop Authorization, but not the body
		otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, otherURL+r.URL.Path, code)
		}))
		c, err := New(api.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Do(context.Background(), Call{Op: "Auth_login", NoAuth: true, Body: map[string]string{"username": "admin", "password": "VRX_TEST_PSK_TD10A"}})
		if n := target.requests(); n != 0 {
			t.Errorf("%d: body replayed to %s: %q", code, otherURL, target.bodies)
		}
		if err == nil || !strings.Contains(err.Error(), otherURL) {
			t.Errorf("%d: want an error naming %s, got %v", code, otherURL, err)
		}
		api.Close()
		other.Close()
	}
}

// TD-10a (review 2.4a): commit-like calls wait longer than the server's commit budget (111 s), other calls as before.
func TestApplyDeadlineIsAboveTheServerBudget(t *testing.T) {
	deadlines := map[string]time.Duration{}
	var mu sync.Mutex
	c, err := New("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		d, ok := r.Context().Deadline()
		mu.Lock()
		if ok {
			deadlines[r.URL.Path] = time.Until(d)
		} else {
			deadlines[r.URL.Path] = -1
		}
		mu.Unlock()
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
	})
	for _, op := range []string{"Config_commit", "Config_confirm", "Config_validate", "Config_running"} {
		if _, err := c.Do(context.Background(), Call{Op: op}); err != nil {
			t.Fatal(op, err)
		}
	}
	for _, p := range []string{"/api/v1/config/commit", "/api/v1/config/commit/confirm", "/api/v1/config/validate"} {
		if d := deadlines[p]; d < 140*time.Second {
			t.Errorf("%s: deadline in %v, want ≥ 140 s (server budget 111 s)", p, d)
		}
	}
	if d := deadlines["/api/v1/config"]; d <= 0 || d > 91*time.Second {
		t.Errorf("GET /config: deadline in %v, want ≤ 90 s", d)
	}
}

// TD-10a (review 2.4a): a commit that gets no answer in time is not reported as a plain failure — the client asks
// the API what happened (pending commit, sync state, newest revision).
func TestCommitTimeoutLooksUpTheOutcome(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/config/commit", func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	mux.HandleFunc("GET /api/v1/state/system", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pendingCommit":{"txnId":"txn-td10a","deadline":"2026-09-24T20:00:00.000Z","createdAt":"2026-09-24T19:59:00.000Z"},"sync":{"state":"in-sync","reason":""}}`))
	})
	mux.HandleFunc("GET /api/v1/config/revisions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":7,"txnId":"txn-before","kind":"commit","createdAt":"2026-01-01T00:00:00.000Z"}],"total":7}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP.Timeout = 300 * time.Millisecond // stands in for the commit deadline
	_, err = c.Do(context.Background(), Call{Op: "Config_commit"})
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	for _, want := range []string{"txn-td10a", "IS pending", "newest revision 7", "sync in-sync"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
