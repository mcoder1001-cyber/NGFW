package agent

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	capturetrace "ngfw/agent/internal/actions/capture-trace"
)

// TestCaptureStatus pins the gRPC codes the API maps to problem+json (busy → 409, invalid → 400 with a pointer).
func TestCaptureStatus(t *testing.T) {
	for err, want := range map[error]codes.Code{
		fmt.Errorf("%w: bpf: x", capturetrace.ErrInvalid): codes.InvalidArgument,
		capturetrace.ErrBusy:                              codes.Aborted,
		capturetrace.ErrGlobals:                           codes.FailedPrecondition,
		capturetrace.ErrRunning:                           codes.FailedPrecondition,
		capturetrace.ErrNotFound:                          codes.NotFound,
	} {
		if got := status.Code(captureStatus(err)); got != want {
			t.Errorf("%v: %v, want %v", err, got, want)
		}
	}
	if captureStatus(nil) != nil {
		t.Fatal("nil")
	}
}
