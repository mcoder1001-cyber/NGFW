package autosdl

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	autosdlapi "ngfw/agent/binapi/auto_sdl"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// TestAutoSdlOnHost enables auto-SDL on the host VPP, re-applies it (no call: applied-once record)
// and disables it. auto_sdl_config is a getter-less VPP-global (D-071/D-082): the test runs only with
// VRX_AUTOSDL_GLOBALS=1 (a manager window), holds the globals lock exclusively and cannot restore a
// previous value — it leaves auto-SDL disabled, VPP's default. Without the session layer's SDL backend
// (startup.conf, handover-gated on vrx-a) VPP answers FEATURE_DISABLED and changes nothing: skip.
func TestAutoSdlOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	if os.Getenv("VRX_AUTOSDL_GLOBALS") != "1" {
		t.Skip("auto_sdl_config changes a getter-less VPP-global; set VRX_AUTOSDL_GLOBALS=1 in a manager window (D-071/D-082)")
	}
	h.SkipUnlessCompatible(t, "auto_sdl", &autosdlapi.AutoSdlConfig{}, &autosdlapi.AutoSdlConfigReply{})
	h.LockGlobals(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	boot := dfkit.NewMemoryBootStore()
	d := New(h.Client(), boot)
	c := Config{Enable: true, Threshold: DefaultThreshold + 1, RemoveTimeout: DefaultRemoveTimeout}
	if _, err := d.Create(ctx, c.Proto()); errors.Is(err, ErrSessionSDLDisabled) {
		t.Skipf("skip-unless-supported: %v", err)
	} else if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Delete(context.Background(), c.Proto(), nil) })
	if _, err := d.Create(ctx, c.Proto()); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if err := d.Delete(ctx, c.Proto(), nil); err != nil {
		t.Fatal(err)
	}
	t.Log("auto-sdl enabled (threshold 6, remove-timeout 300), re-applied without a call, disabled")
}
