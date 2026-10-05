package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"strings"
	"testing"
)

func TestRARPCOwnerFirstUnavailableAndMalformedBoundaries(t *testing.T) {
	svc := &Service{owner: "w19-rpc-test"}
	ctx := context.Background()
	for _, call := range []func() error{func() error {
		_, e := svc.RemoteAccessCapabilities(ctx, &ngfwv1.RemoteAccessCapabilitiesRequest{Owner: "foreign"})
		return e
	}, func() error {
		_, e := svc.RemoteAccessSessions(ctx, &ngfwv1.RemoteAccessSessionsRequest{Owner: "foreign", Profile: "../bad", Limit: 101})
		return e
	}, func() error {
		_, e := svc.RemoteAccessDisconnect(ctx, &ngfwv1.RemoteAccessDisconnectRequest{Owner: "foreign", Id: "../bad"})
		return e
	}} {
		if e := call(); status.Code(e) != codes.PermissionDenied || !strings.Contains(e.Error(), "owner") || strings.Contains(e.Error(), svc.owner) || strings.Contains(e.Error(), "foreign") {
			t.Fatalf("owner first: %v", e)
		}
	}
	cap, e := svc.RemoteAccessCapabilities(ctx, &ngfwv1.RemoteAccessCapabilitiesRequest{Owner: svc.owner})
	if e != nil || cap.Operational || !cap.EditableDisabledDrafts || cap.Engine != "strongswan-ra" || len(cap.Reason) > 128 {
		t.Fatal("invented readiness", cap, e)
	}
	for _, r := range []*ngfwv1.RemoteAccessSessionsRequest{{Owner: svc.owner, Profile: "road", Limit: 101}, {Owner: svc.owner, Profile: "../road"}, {Owner: svc.owner, Profile: "road", Cursor: strings.Repeat("A", 64)}} {
		if _, e := svc.RemoteAccessSessions(ctx, r); status.Code(e) != codes.InvalidArgument {
			t.Fatal(e)
		}
	}
	if _, e := svc.RemoteAccessDisconnect(ctx, &ngfwv1.RemoteAccessDisconnectRequest{Owner: svc.owner, Profile: "road", Id: strings.Repeat("a", 64)}); status.Code(e) != codes.Unavailable {
		t.Fatal("missing engine claimed removal", e)
	}
}
