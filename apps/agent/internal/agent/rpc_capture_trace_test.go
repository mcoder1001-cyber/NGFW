package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	capturetrace "ngfw/agent/internal/actions/capture-trace"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
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

func TestCaptureRecoveryAtConnectWithoutRPC(t *testing.T) {
	dir, vppDir := t.TempDir(), t.TempDir()
	t.Setenv("NGFW_CAPTURE_DIR", dir)
	t.Setenv("NGFW_CAPTURE_VPP_DIR", vppDir)
	t.Setenv("NGFW_GLOBALS_OWNER", "false")
	record := capturetrace.Record{ID: "w5-interrupted", State: "running"}
	data, _ := json.Marshal(record)
	if err := os.WriteFile(filepath.Join(dir, record.ID+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	f := dfkittest.NewFake()
	s := &Service{owner: "w5", vpp: f, captureConfig: capturetrace.Config{GlobalsOwner: true, Boot: dfkit.NewMemoryBootStore()}}
	t.Cleanup(func() { captureManagers.Delete(s) })
	if err := s.recoverCaptures(context.Background()); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Read only generated fixture filenames under this test's private state directory.
	data, err := os.ReadFile(filepath.Join(dir, record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.State != "interrupted" {
		t.Fatalf("not recovered before RPC: %+v", record)
	}
	m, err := (&server{svc: s}).captures()
	if err != nil || m == nil {
		t.Fatal(err)
	}
}
