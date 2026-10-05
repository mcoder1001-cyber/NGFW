package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGlobalsHelperCannotGrantItsOwnWindow(t *testing.T) {
	t.Setenv("NGFW_TRAFFIC_C_GLOBALS", "")
	if err := run("snapshot", filepath.Join(t.TempDir(), "snapshot.json")); err == nil || !strings.Contains(err.Error(), "leased") {
		t.Fatalf("expected lease refusal, got %v", err)
	}
}

func TestGlobalsHelperRejectsForeignLockBeforeConnecting(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-globals-lock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	t.Setenv("NGFW_TRAFFIC_C_GLOBALS", "1")
	t.Setenv("NGFW_GLOBAL_LOCK_FD", strconv.Itoa(int(f.Fd())))
	if err = run("snapshot", filepath.Join(t.TempDir(), "snapshot.json")); err == nil || !strings.Contains(err.Error(), "foreign lock") {
		t.Fatalf("expected foreign lock refusal, got %v", err)
	}
}
