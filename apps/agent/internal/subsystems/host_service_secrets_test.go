package subsystems

import (
	"context"
	"errors"
	"ngfw/agent/internal/secretvalue"
	"strings"
	"testing"
)

func TestHostServiceSecretOwnerIsolationAndNoActiveFallback(t *testing.T) {
	ctx := context.Background()
	generation := "hmac:" + strings.Repeat("a", 64)
	calls := 0
	if err := SetHostServiceSecrets(t.Name(), func(context.Context, string) (string, error) { return generation, nil }, func(_ context.Context, got string) ([]byte, error) {
		calls++
		if got != generation {
			return nil, errors.New("private material must not escape")
		}
		return []byte("historical"), nil
	}); err != nil {
		t.Fatal(err)
	}
	source := hostServiceSecrets(t.Name())
	if _, err := source.resolve(ctx, "key/one"); err == nil || calls != 0 {
		t.Fatal("unbound lookup reached history")
	}
	bound := secretvalue.WithBindings(ctx, map[string]string{"key/one": generation})
	raw, err := source.resolve(bound, "key/one")
	if err != nil || string(raw) != "historical" || calls != 1 {
		t.Fatal("bound history failed")
	}
	if _, err = hostServiceSecrets(t.Name()+"-other").resolve(bound, "key/one"); err == nil {
		t.Fatal("owner isolation failed")
	}
	bad := secretvalue.WithBindings(ctx, map[string]string{"key/one": "hmac:" + strings.Repeat("b", 64)})
	if _, err = source.resolve(bad, "key/one"); err == nil || strings.Contains(err.Error(), "private material") {
		t.Fatal("missing history leaked or fell back")
	}
	if err = SetHostServiceSecrets(t.Name(), nil, nil); err == nil {
		t.Fatal("incomplete adapter accepted")
	}
}
