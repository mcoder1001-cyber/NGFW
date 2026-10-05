package keadhcprelay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const appliedRelay = `{"items":[{"name":"to-kea","state":"applied","config":{"servers":["10.27.2.2"]},"retrieved":{"servers":["10.27.2.2"]}}]}`

func TestRelayReadinessThroughLiveAPI(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/state/dhcp/relays" || r.Header.Get("authorization") != "Bearer NGFW_TEST_PSK_readiness" {
			t.Errorf("readiness must use authenticated read-only relay RPC endpoint: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":503}`))
			return
		}
		_, _ = w.Write([]byte(appliedRelay))
	}))
	defer server.Close()
	a := &api{t: t, base: server.URL, token: "NGFW_TEST_PSK_readiness"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !a.waitRelayReady(ctx, "to-kea") || requests.Load() != 2 {
		t.Fatal("API recovery was not observed after unavailable RPC")
	}
}

func TestRelayReadinessRefusesUnreadyAndBoundsRequests(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unavailable", `{"status":503}`, 503},
		{"foreign-name", `{"items":[{"name":"foreign","state":"applied","config":{"servers":[]},"retrieved":{"servers":[]}}]}`, 200},
		{"drift", `{"items":[{"name":"to-kea","state":"drift","config":{"servers":[]},"retrieved":{"servers":[]}}]}`, 200},
		{"cached-config-only", `{"items":[{"name":"to-kea","state":"applied","config":{"servers":[]},"retrieved":null}]}`, 200},
		{"empty-retrieval", `{"items":[{"name":"to-kea","state":"applied","config":{"servers":[]},"retrieved":{}}]}`, 200},
		{"malformed", `{"items":"ready"}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if (&api{t: t, base: server.URL}).waitRelayReady(ctx, "to-kea") {
				t.Fatal("unready relay accepted")
			}
			if ctx.Err() == nil {
				t.Fatal("unready polling did not preserve its deadline")
			}
		})
	}
	t.Run("blocked-request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if (&api{t: t, base: server.URL}).waitRelayReady(ctx, "to-kea") {
			t.Fatal("unanswered request accepted")
		}
		if ctx.Err() == nil {
			t.Fatal("HTTP request exceeded its shared recovery deadline")
		}
	})
}
