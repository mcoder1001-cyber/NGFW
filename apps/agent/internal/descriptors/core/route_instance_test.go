package core_test

import (
	"context"
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
)

func TestNamedRouteOwnershipRestartAndConflict(t *testing.T) {
	ctx := context.Background()
	v := coretest.New()
	owned := ownertable.NewMemory()
	config := &core.RouteDescriptor{Env: core.Env{Client: v, Owner: "w7", Owned: owned}}
	dynamic := &core.RouteDescriptor{Env: core.Env{Client: v, Owner: "w7", Owned: owned, RouteInstance: "ip.route.wan"}}
	route := &core.Route{Prefix: "0.0.0.0/0", Paths: []*core.RoutePath{{Address: "192.0.2.1", Weight: 3}, {Address: "198.51.100.1", Weight: 1}}}
	if _, err := dynamic.Create(ctx, route); err != nil {
		t.Fatal(err)
	}
	if owned.Has(string(config.KeyOf(route))) || !owned.Has(string(dynamic.KeyOf(route))) {
		t.Fatal("ownership overlaps")
	}
	if got, err := config.Retrieve(ctx); err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	if _, err := config.Create(ctx, route); !errors.Is(err, core.ErrRouteConflict) {
		t.Fatal("configuration stole dynamic FIB entry", err)
	}
	restarted := &core.RouteDescriptor{Env: dynamic.Env}
	if got, err := restarted.Retrieve(ctx); err != nil || len(got) != 1 || got[0].Key.Descriptor() != "ip.route.wan" {
		t.Fatal(got, err)
	}
	if err := restarted.Delete(ctx, route, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := restarted.Retrieve(ctx); err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	if _, err := config.Create(ctx, route); err != nil {
		t.Fatal(err)
	}
	if _, err := dynamic.Create(ctx, route); !errors.Is(err, core.ErrRouteConflict) {
		t.Fatal("dynamic stole configuration FIB entry", err)
	}
}
