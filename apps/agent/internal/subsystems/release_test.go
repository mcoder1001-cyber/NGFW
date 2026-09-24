package subsystems

import (
	"context"
	"testing"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// TD-3 Q2: after a full resync the agent releases its own quarantine holders whose index is clean
// again; another owner's holder is left alone.
func TestAfterResyncReleasesQuarantine(t *testing.T) {
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "w1")
	if err != nil {
		t.Fatal(err)
	}
	v := coretest.New()
	w, err := Register(scheduler.NewRegistry(), Env{Client: v, Owner: "w1", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	v.AddInterface("loop16383", "Loopback", ifsanitize.QuarantineTagPrefix+"w1")
	v.AddInterface("loop16382", "Loopback", ifsanitize.QuarantineTagPrefix+"w2")
	w.AfterResync(context.Background())
	if _, ok := v.InterfaceByName("loop16383"); ok {
		t.Fatalf("own clean quarantine holder not released: %s", v.Snapshot())
	}
	if _, ok := v.InterfaceByName("loop16382"); !ok {
		t.Fatal("another owner's quarantine holder was released")
	}
}
