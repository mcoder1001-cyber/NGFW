package hoststack

import (
	"context"
	"errors"
	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/session"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/secretchannel"
	"strings"
	"testing"
)

func TestNamespaceSealedGenerationRotationRollbackRestartAndRevocation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cache, err := secretchannel.Open(root, "namespace-generation")
	if err != nil {
		t.Fatal(err)
	}
	stage := func(value string) string {
		id, e := cache.Stage(map[string][]byte{"key/ns": []byte(value)})
		if e != nil {
			t.Fatal(e)
		}
		if e = cache.Activate(id); e != nil {
			t.Fatal(e)
		}
		ref, e := cache.Ref(ctx, "key/ns")
		if e != nil {
			t.Fatal(e)
		}
		return ref
	}
	old := stage("18446744073709551614")
	newer := stage("18446744073709551613")
	f, _ := newFake(true)
	var observed []uint64
	f.On("app_namespace_add_del_v4", func(msg api.Message) ([]api.Message, error) {
		req := msg.(*session.AppNamespaceAddDelV4)
		observed = append(observed, req.Secret)
		return []api.Message{&session.AppNamespaceAddDelV4Reply{AppnsIndex: 1}}, nil
	})
	st := stateFor("w13", f, dfkit.NewMemoryBootStore())
	create := func() *NamespaceDescriptor {
		return newNamespace(f, "w13", st, func(ctx context.Context, ref string) ([]byte, error) { return cache.Resolve(ctx, ref) })
	}
	d := create()
	oldValue := Namespace{ID: "w13-secret", SecretGeneration: old}.Proto()
	newValue := Namespace{ID: "w13-secret", SecretGeneration: newer}.Proto()
	if strings.Contains(oldValue.String(), "18446744073709551614") {
		t.Fatal("material in scheduler value")
	}
	if _, err = d.Create(ctx, oldValue); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Update(ctx, oldValue, newValue, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Update(ctx, newValue, oldValue, nil); err != nil {
		t.Fatal(err)
	}
	cache, err = secretchannel.Open(root, "namespace-generation")
	if err != nil {
		t.Fatal(err)
	}
	d = create()
	if _, err = d.Create(ctx, oldValue); err != nil {
		t.Fatal(err)
	}
	want := []uint64{18446744073709551614, 18446744073709551613, 18446744073709551614, 18446744073709551614}
	if len(observed) != len(want) {
		t.Fatal("wrong call count")
	}
	for i := range want {
		if observed[i] != want[i] {
			t.Fatal("wrong generation at VPP boundary")
		}
	}
	d.secrets = func(context.Context, string) ([]byte, error) { return nil, errors.New("unavailable") }
	if err = d.Validate(ctx, KeyNamespace("w13-secret"), oldValue, nil); err == nil {
		t.Fatal("missing generation validated")
	}
	if _, err = d.Create(ctx, oldValue); err == nil || len(observed) != len(want) {
		t.Fatal("missing generation reached VPP")
	}
	if err = d.Delete(ctx, oldValue, nil); err != nil {
		t.Fatal(err)
	}
	if observed[len(observed)-1] != 0 {
		t.Fatal("removal resolved unnecessary credential")
	}
	for _, bad := range []string{"0", "01", "-1", "18446744073709551616", "secret text"} {
		d.secrets = func(context.Context, string) ([]byte, error) { return []byte(bad), nil }
		if err = d.Validate(ctx, KeyNamespace("w13-secret"), oldValue, nil); err == nil || strings.Contains(err.Error(), bad) && bad != "0" {
			t.Fatal("invalid material accepted or echoed")
		}
	}
}
