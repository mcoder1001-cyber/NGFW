package mfib

import (
	"context"
	"ngfw/agent/internal/descriptors/df7"
	"testing"
)

func TestNamedDescriptorIsolatedOwnership(t *testing.T) {
	static, client, _, _ := fixture(t)
	dynamic := NewNamed("mfib.route.pim", client, static.Owner, static.Store, df7.WithIDRange(1000, 1999))
	ctx := context.Background()
	r := sample()
	if dynamic.KeyOf(df7.Encode(r)) != NamedKey("mfib.route.pim", r) {
		t.Fatal("wrong named key")
	}
	if _, e := dynamic.Create(ctx, df7.Encode(r)); e != nil {
		t.Fatal(e)
	}
	got, e := dynamic.Retrieve(ctx)
	if e != nil || len(got) != 1 || got[0].Key != dynamic.KeyOf(df7.Encode(r)) {
		t.Fatalf("dynamic retrieval %v %v", got, e)
	}
	got, e = static.Retrieve(ctx)
	if e != nil || len(got) != 0 {
		t.Fatalf("static adopted dynamic %v %v", got, e)
	}
	if _, e := static.Create(ctx, df7.Encode(r)); e == nil {
		t.Fatal("static overwrote dynamic route")
	}
	if e := static.Delete(ctx, df7.Encode(r), nil); e != nil {
		t.Fatal(e)
	}
	got, _ = dynamic.Retrieve(ctx)
	if len(got) != 1 {
		t.Fatal("static deleted dynamic")
	}
	if e := dynamic.Delete(ctx, df7.Encode(r), nil); e != nil {
		t.Fatal(e)
	}
	got, e = dynamic.Retrieve(ctx)
	if e != nil || len(got) != 0 {
		t.Fatal("dynamic withdrawal failed", e)
	}
}
