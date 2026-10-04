package natcommon_test

import (
	"context"
	"testing"

	"ngfw/agent/internal/descriptors/natcommon"
)

func TestInstanceRefusesForeignObjectAndRetrievesOnlyClaimed(t *testing.T) {
	type spec struct {
		Name string `json:"name"`
	}
	claims := natcommon.NewMemoryClaimStore()
	exists := false
	writes := 0
	base := natcommon.New(natcommon.Ops[spec]{Name: "nat.test", ID: func(s spec) string { return s.Name }, Claims: claims,
		Create: func(context.Context, spec) (any, error) { exists = true; writes++; return nil, nil }, Delete: func(context.Context, spec, any) error { exists = false; return nil },
		Retrieve: func(context.Context) ([]natcommon.Item[spec], error) {
			if !exists {
				return nil, nil
			}
			return []natcommon.Item[spec]{{Spec: spec{Name: "wan1"}}}, nil
		}})
	instance := base.Instance("nat.test.wan")
	obj, _ := natcommon.Encode(&spec{Name: "wan1"})
	ctx := context.Background()
	exists = true
	if _, err := instance.Create(ctx, obj); err == nil || writes != 0 {
		t.Fatal("foreign object claimed", err, writes)
	}
	if rows, err := instance.Retrieve(ctx); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	exists = false
	if _, err := instance.Create(ctx, obj); err != nil {
		t.Fatal(err)
	}
	if rows, err := instance.Retrieve(ctx); err != nil || len(rows) != 1 || rows[0].Key.Descriptor() != "nat.test.wan" {
		t.Fatal(rows, err)
	}
}
