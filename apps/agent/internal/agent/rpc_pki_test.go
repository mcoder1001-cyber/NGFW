package agent

import (
	"context"
	"net"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/pki"
	"ngfw/agent/internal/subsystems"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

var pkiStateTime = time.Date(2026, time.October, 3, 12, 34, 56, 123000000, time.UTC)

func assertPkiUnavailable(t *testing.T, got *ngfwv1.PkiFileStateResponse) {
	t.Helper()
	want := &ngfwv1.PkiFileStateResponse{
		Owner: "pki-owner", RetrievedAt: timestamppb.New(pkiStateTime),
		Unavailable: "PKI file materializer is not wired in this agent build",
	}
	if !proto.Equal(got, want) {
		t.Fatalf("unexpected unwired PKI state: got %v, want %v", got, want)
	}
	if err := got.GetRetrievedAt().CheckValid(); err != nil {
		t.Fatalf("invalid observed timestamp: %v", err)
	}
}

func TestPkiFileStateOwnerAndUnavailable(t *testing.T) {
	clockCalls := 0
	// Deliberately no state, scheduler, VPP client or secret provider: observation
	// must not access any of them or require a working materializer.
	g := &server{svc: &Service{owner: "pki-owner", now: func() time.Time {
		clockCalls++
		return pkiStateTime
	}}}
	if got, err := g.PkiFileState(context.Background(), &ngfwv1.PkiFileStateRequest{Owner: "foreign"}); got != nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner response %v, error %v", got, err)
	}
	if clockCalls != 0 {
		t.Fatal("foreign owner reached observation before authorization")
	}
	for _, owner := range []string{"pki-owner", ""} {
		got, err := g.PkiFileState(context.Background(), &ngfwv1.PkiFileStateRequest{Owner: owner})
		if err != nil {
			t.Fatalf("owner %q: %v", owner, err)
		}
		assertPkiUnavailable(t, got)
	}
	if clockCalls != 2 {
		t.Fatalf("expected one timestamp per observation, got %d", clockCalls)
	}
}

func TestPkiFileStateRegisteredRPC(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })
	g := grpc.NewServer()
	ngfwv1.RegisterDataplaneServer(g, &server{svc: &Service{
		owner: "pki-owner", now: func() time.Time { return pkiStateTime },
	}})
	go func() { _ = g.Serve(listener) }()
	t.Cleanup(g.Stop)
	conn, err := grpc.NewClient("passthrough:///pki-offline",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := ngfwv1.NewDataplaneClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, owner := range []string{"pki-owner", ""} {
		got, err := client.PkiFileState(ctx, &ngfwv1.PkiFileStateRequest{Owner: owner})
		if err != nil {
			t.Fatalf("registered RPC owner %q returned %v (must override inherited UNIMPLEMENTED)", owner, err)
		}
		assertPkiUnavailable(t, got)
	}
	got, err := client.PkiFileState(ctx, &ngfwv1.PkiFileStateRequest{Owner: "foreign"})
	if got != nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("registered RPC foreign owner response %v, error %v", got, err)
	}
}

func TestPkiFileStateAvailableAndRedactedErrors(t *testing.T) {
	keyer, err := vpn.NewKeyer(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	m, err := pki.New(pki.Config{Root: dir, Manifest: manifest, Keyer: keyer})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	restore := subsystems.SetPKIRuntimeForTest("pki-observed", m, "")
	defer restore()
	g := &server{svc: &Service{owner: "pki-observed", now: func() time.Time { return pkiStateTime }}}
	got, err := g.PkiFileState(context.Background(), &ngfwv1.PkiFileStateRequest{Owner: "pki-observed"})
	if err != nil || got.GetRoot() != dir || got.GetUnavailable() != "" || len(got.GetFiles()) != 0 {
		t.Fatalf("observed state: %v %v", got, err)
	}
	if err := os.WriteFile(manifest, []byte("PRIVATE_DIAGNOSTIC_CANARY"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = g.PkiFileState(context.Background(), &ngfwv1.PkiFileStateRequest{})
	if got != nil || status.Code(err) != codes.Unavailable || strings.Contains(err.Error(), "CANARY") {
		t.Fatalf("unsafe error: %v %v", got, err)
	}
}
