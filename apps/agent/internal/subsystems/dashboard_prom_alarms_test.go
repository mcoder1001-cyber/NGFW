package subsystems

import (
	"context"
	"google.golang.org/protobuf/proto"
	"net"
	"net/http"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/promexport"
	"strconv"
	"testing"
)

type dashboardSource struct{}

func (dashboardSource) Read(context.Context) (promexport.Snapshot, error) {
	return promexport.Snapshot{}, nil
}
func TestPrometheusListenerLifecycle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := dashboardPort(t, ln)
	_ = ln.Close()
	s := NewPrometheusStage(dashboardSource{})
	defer s.Close()
	ctx := context.Background()
	v := &ngfwv1.ManagementPrometheus{Enabled: proto.Bool(true), Listen: proto.String("127.0.0.1"), Port: proto.Uint32(port)}
	if _, err := s.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	kvs, err := s.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, v) {
		t.Fatalf("retrieve: %v %v", kvs, err)
	}
	scrape := func(want int) {
		t.Helper()
		resp, err := http.Get("http://" + s.listener.Addr() + "/metrics")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("status %d want %d", resp.StatusCode, want)
		}
	}
	scrape(200)
	deny := proto.Clone(v).(*ngfwv1.ManagementPrometheus)
	deny.Allow = []string{"192.0.2.0/24"}
	if _, err := s.Update(ctx, v, deny, nil); err != nil {
		t.Fatal(err)
	}
	scrape(403)
	// Same-address update must replace the handler, and rollback restores it.
	if _, err := s.Update(ctx, deny, v, nil); err != nil {
		t.Fatal(err)
	}
	scrape(200)
	blocked, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocked.Close() }()
	bad := proto.Clone(v).(*ngfwv1.ManagementPrometheus)
	bad.Port = proto.Uint32(dashboardPort(t, blocked))
	if _, err := s.Update(ctx, v, bad, nil); err == nil {
		t.Fatal("occupied port accepted")
	}
	scrape(200)
	kvs, err = s.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, v) {
		t.Fatalf("failed bind changed retrieved config: %v %v", kvs, err)
	}
	if err := s.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	kvs, err = s.Retrieve(ctx)
	if err != nil || len(kvs) != 0 {
		t.Fatal(kvs, err)
	}
	if _, err := s.Create(ctx, v); err != nil {
		t.Fatal("restart:", err)
	}
	scrape(200)
}

func dashboardPort(t *testing.T, ln net.Listener) uint32 {
	t.Helper()
	_, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(p, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(port)
}
