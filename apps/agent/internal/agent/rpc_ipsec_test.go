package agent

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"testing"
)

func TestIpsecStateNativeWithoutCharon(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	response, err := s.IpsecState(context.Background(), &vrxv1.IpsecStateRequest{})
	if err != nil || response.GetDaemonVersion() != "vpp-ikev2" || len(response.GetSas()) != 0 {
		t.Fatal("native empty state without charon", response, err)
	}
	if _, err = s.IpsecState(context.Background(), &vrxv1.IpsecStateRequest{Owner: "someone-else"}); status.Code(err) != codes.InvalidArgument && status.Code(err) != codes.PermissionDenied {
		t.Fatal("foreign owner accepted", err)
	}
}
