package main

import (
	"context"
	ravpn "ngfw/agent/internal/ra_vpn"
	"os"
	"testing"
)

func TestBoundaryFailureNeverExecutesDaemon(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"ngfw-ra-daemon", ravpn.InstanceID("w19", "fixture")}
	executed, validated := false, false
	err := runWith(func(context.Context, string) error { return ravpn.ErrBoundary }, func(string, int64) error { validated = true; return nil }, func(string, []string, []string) error { executed = true; return nil })
	if err == nil || executed || validated {
		t.Fatal("boundary failure reached daemon/config")
	}
	os.Args = []string{"ngfw-ra-daemon", "--version"}
	called := false
	if runWith(func(context.Context, string) error { called = true; return nil }, func(string, int64) error { return nil }, func(string, []string, []string) error { called = true; return nil }) == nil || called {
		t.Fatal("daemon flag escaped fixed instance contract")
	}
}
