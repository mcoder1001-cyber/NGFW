package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/subsystems"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSystemIdentityStateOwnerAndWiredPaths(t *testing.T) {
	svc := newSvc(t, coretest.New(), t.TempDir())
	g := &server{svc: svc}
	if _, err := g.SystemIdentityState(context.Background(), &vrxv1.SystemIdentityStateRequest{Owner: "foreign"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("owner guard: %v", err)
	}
	d := subsystems.SystemIdentityOf(testOwner)
	if d == nil {
		t.Fatal("identity descriptor not wired")
	}
	p := d.Paths()
	if err := os.MkdirAll(filepath.Dir(p.Hostname), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Hostname, []byte("test-slot-identity\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := g.SystemIdentityState(context.Background(), &vrxv1.SystemIdentityStateRequest{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	if st.Owner != testOwner || st.Hostname != "test-slot-identity" || st.KernelHostname != nil || st.ResolverStatus != "slot-only" || st.RetrievedAt == nil {
		t.Fatalf("observed state %+v", st)
	}
	if st.RetrievedAt.AsTime().Before(time.Now().Add(-time.Minute)) {
		t.Fatal("stale retrieved time")
	}
}
