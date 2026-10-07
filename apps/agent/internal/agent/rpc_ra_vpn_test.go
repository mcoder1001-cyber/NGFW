package agent

import (
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
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
	capability, e := svc.RemoteAccessCapabilities(ctx, &ngfwv1.RemoteAccessCapabilitiesRequest{Owner: svc.owner})
	if e != nil || capability.Operational || !capability.EditableDisabledDrafts || capability.Engine != "strongswan-ra" || len(capability.Reason) > 128 {
		t.Fatal("invented readiness", capability, e)
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

func TestRAVerifiedSnapshotPaginationBeyondTwoHundred(t *testing.T) {
	for _, count := range []int{201, strongswan.MaxRASessions} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			rows := make([]strongswan.RASession, count)
			for i := range rows {
				rows[i] = strongswan.RASession{ID: fmt.Sprintf("%064x", count-i), Profile: "road", Identity: "client", BytesIn: ^uint64(0), BytesOut: 9007199254740993}
			}
			cursor := ""
			seen := map[string]bool{}
			pageSizes := []int{}
			for page := 0; page < 12; page++ {
				response, err := pageRemoteAccessSessions(rows, &ngfwv1.RemoteAccessSessionsRequest{Profile: "road", Cursor: cursor, Limit: 100})
				if err != nil || len(response.GetSessions()) > 100 || len(response.GetSessions()) == 0 {
					t.Fatal("bounded page lost", err)
				}
				pageSizes = append(pageSizes, len(response.GetSessions()))
				for _, row := range response.GetSessions() {
					if seen[row.GetId()] || row.GetId() <= cursor || row.GetProfile() != "road" || row.GetBytesIn() != ^uint64(0) || row.GetBytesOut() != 9007199254740993 {
						t.Fatal("membership, cursor or exact counter lost")
					}
					seen[row.GetId()] = true
				}
				if response.GetNextCursor() == "" {
					break
				}
				if response.GetNextCursor() != response.GetSessions()[len(response.GetSessions())-1].GetId() {
					t.Fatal("cursor does not bind last owned row")
				}
				cursor = response.GetNextCursor()
			}
			if len(seen) != count {
				t.Fatalf("truncated complete snapshot: %d/%d", len(seen), count)
			}
			if count == 201 && fmt.Sprint(pageSizes) != "[100 100 1]" {
				t.Fatal("201 sessions did not form three bounded pages", pageSizes)
			}
			if _, err := pageRemoteAccessSessions(rows, &ngfwv1.RemoteAccessSessionsRequest{Profile: "road", Cursor: strings.Repeat("f", 64), Limit: 100}); status.Code(err) != codes.FailedPrecondition {
				t.Fatal("stale cursor accepted", err)
			}
			if _, err := pageRemoteAccessSessions(rows, &ngfwv1.RemoteAccessSessionsRequest{Profile: "road", Limit: 101}); status.Code(err) != codes.InvalidArgument {
				t.Fatal("oversized page accepted", err)
			}
		})
	}
	if _, err := pageRemoteAccessSessions(make([]strongswan.RASession, strongswan.MaxRASessions+1), &ngfwv1.RemoteAccessSessionsRequest{Profile: "road", Limit: 100}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("oversized total snapshot accepted", err)
	}
}
