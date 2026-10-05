package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	pb "ngfw/agent/gen/ngfw/v1"
)

type agent struct {
	pb.UnimplementedDataplaneServer
	response *pb.HealthResponse
}

func (a *agent) Health(context.Context, *pb.HealthRequest) (*pb.HealthResponse, error) {
	return a.response, nil
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name                             string
		connected, degraded, reconciling bool
		status                           int
		body                             string
		ok                               bool
	}{
		{"healthy", true, false, false, 200, `{"status":"ok","service":"ngfw-api"}`, true},
		{"VPP disconnected", false, false, false, 200, `{"status":"ok","service":"ngfw-api"}`, false},
		{"agent degraded", true, true, false, 200, `{"status":"ok","service":"ngfw-api"}`, false},
		{"reconciling", true, false, true, 200, `{"status":"ok","service":"ngfw-api"}`, false},
		{"API failure", true, false, false, 503, `{"status":"ok","service":"ngfw-api"}`, false},
		{"wrong service", true, false, false, 200, `{"status":"ok","service":"other"}`, false},
		{"bad JSON", true, false, false, 200, `broken`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "agent.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			pb.RegisterDataplaneServer(server, &agent{response: &pb.HealthResponse{VppConnected: tc.connected, Degraded: tc.degraded, ReconcileInProgress: tc.reconciling}})
			t.Cleanup(server.Stop)
			go func() { _ = server.Serve(listener) }()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(api.Close)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err = check(ctx, socket, api.URL)
			if (err == nil) != tc.ok {
				t.Fatalf("health result=%v want healthy=%v", err, tc.ok)
			}
		})
	}
}
