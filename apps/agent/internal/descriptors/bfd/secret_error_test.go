package bfd

import (
	"context"
	"errors"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"strings"
	"testing"
)

func TestResolverErrorNeverLeaksMaterial(t *testing.T) {
	f := df7test.NewFake()
	d := NewAuthKey(f, df7test.Owner, func(context.Context, uint32) ([]byte, error) { return nil, errors.New("private-resolver-material") })
	_, e := d.Create(context.Background(), df7.Encode(AuthKey{ID: 1, Type: AuthKeyedSHA1}))
	if e == nil || strings.Contains(e.Error(), "private-resolver-material") {
		t.Fatalf("unsafe error %v", e)
	}
}
