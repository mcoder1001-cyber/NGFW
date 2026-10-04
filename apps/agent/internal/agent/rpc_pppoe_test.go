package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// On the globals owner the handler delegates and reports not-accepted (no applied session) — not an error.
func TestPppoeReconnectOwner(t *testing.T) {
	s := newOwnerSvc(t, coretest.New(), t.TempDir()) // GlobalsOwner=true wires the pppoe subsystem for testOwner
	g := &server{svc: s, log: s.log}

	resp, err := g.PppoeReconnect(context.Background(), &ngfwv1.PppoeReconnectRequest{Interface: "wan0"})
	if err != nil {
		t.Fatalf("PppoeReconnect(wan0): %v", err)
	}
	if resp.GetAccepted() || resp.GetMessage() == "" {
		t.Fatalf("no applied session must be not-accepted with a message, got %+v", resp)
	}

	// interface validation → InvalidArgument.
	for name, iface := range map[string]string{"empty": "", "too long": strings.Repeat("a", 65), "control char": "wan\x00"} {
		if _, err := g.PppoeReconnect(context.Background(), &ngfwv1.PppoeReconnectRequest{Interface: iface}); grpcCode(err) != codes.InvalidArgument {
			t.Fatalf("%s interface: want InvalidArgument, got %v", name, err)
		}
	}

	// owner mismatch → InvalidArgument.
	if _, err := g.PppoeReconnect(context.Background(), &ngfwv1.PppoeReconnectRequest{Owner: "someone-else", Interface: "wan0"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("owner mismatch: want InvalidArgument, got %v", err)
	}
}

// A slot agent does not drive the host's pppd units: reconnect is Unavailable.
func TestPppoeReconnectSlotUnavailable(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir()) // GlobalsOwner=false (a test slot)
	g := &server{svc: s, log: s.log}
	if _, err := g.PppoeReconnect(context.Background(), &ngfwv1.PppoeReconnectRequest{Interface: "wan0"}); grpcCode(err) != codes.Unavailable {
		t.Fatalf("slot reconnect: want Unavailable, got %v", err)
	}
}
