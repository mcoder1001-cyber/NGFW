package subsystems

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/secretchannel"
)

func TestFRRSecretGenerationRotationAndHistoricalRollback(t *testing.T) {
	ctx := context.Background()
	cache, err := secretchannel.Open(t.TempDir(), "generation-test")
	if err != nil {
		t.Fatal(err)
	}
	secret := func() []byte {
		b := make([]byte, 6)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		return []byte(hex.EncodeToString(b))
	}
	oldSecret, newSecret := secret(), secret()
	activate := func(value []byte) {
		id, err := cache.Stage(map[string][]byte{"password/ospf": value})
		if err != nil {
			t.Fatal(err)
		}
		if err = cache.Activate(id); err != nil {
			t.Fatal(err)
		}
	}
	activate(oldSecret)
	paths := tempPaths(t)
	if err = os.WriteFile(filepath.Join(paths.SocketDir(), "zebra.vty"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	runner := newFakeFRR()
	rt := newFRRAt(Env{Owner: "generation-test"}, runner, paths, true)
	defer rt.Close()
	rt.secretSource, rt.secretFingerprint, rt.secretHistory = cache.Text, cache.Ref, cache.Resolve
	descriptor := &frrConfigDescriptor{rt: rt}
	doc := frrDoc(t, `{"interfaces":{"host-w8l0":{"ipv4":["10.8.1.1/24"],"lcp":{"hostIfName":"w8-l0"}}},"routing":{"ospf":{"routerId":"10.8.0.1","areas":{"0":{}},"interfaces":{"host-w8l0":{"area":"0","auth":{"type":"md5","keyId":1,"keyRef":"password/ospf"}}}}}}`)
	original := proto.Clone(doc)
	value := func() *structpb.Struct {
		fingerprint, err := cache.Ref(ctx, "password/ospf")
		if err != nil {
			t.Fatal(err)
		}
		v, err := desired.FRRValueWithSecretBindings(doc, desired.FRRApplied, map[string]string{"password/ospf": fingerprint})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	oldValue := value()
	if _, err = descriptor.Create(ctx, oldValue); err != nil {
		t.Fatal(err)
	}
	assertRendered := func(want, absent []byte) {
		t.Helper()
		raw, err := os.ReadFile(paths.ConfFile())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), string(want)) || strings.Contains(string(raw), string(absent)) {
			t.Fatal("rendered credential generation mismatch")
		}
	}
	assertRendered(oldSecret, newSecret)
	activate(newSecret)
	newValue := value()
	if proto.Equal(oldValue, newValue) {
		t.Fatal("same-reference rotation did not change internal desired identity")
	}
	kvs, err := descriptor.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, oldValue) {
		t.Fatal("retrieval inferred candidate instead of last applied generation")
	}
	if _, err = descriptor.Update(ctx, oldValue, newValue, nil); err != nil {
		t.Fatal(err)
	}
	assertRendered(newSecret, oldSecret)
	// Restore old state, then force an actual scheduler transaction to fail after
	// rotating FRR. Compensation must use old history while new remains active.
	if _, err = descriptor.Update(ctx, newValue, oldValue, nil); err != nil {
		t.Fatal(err)
	}
	registry := scheduler.NewRegistry()
	registry.Register(descriptor)
	registry.Register(&frrGenerationFailure{frrConfigDescriptor: descriptor})
	reconciler := scheduler.New(registry, nil)
	reconciler.VerifyRetries = 0
	failed := reconciler.Apply(ctx, []scheduler.KV{{Key: desired.FRRConfigKey, Value: newValue}, {Key: scheduler.Join("generation.fail", "one"), Value: structpb.NewStructValue(&structpb.Struct{})}}, nil)
	if failed.Outcome != scheduler.OutcomeRolledBack {
		t.Fatal("credential failure did not roll back", failed.Outcome, failed.Err)
	}
	assertRendered(oldSecret, newSecret)
	if runner.count("--reload") != 5 {
		t.Fatal("rotation and rollback must execute real renderer reloads")
	}
	if !proto.Equal(doc, original) {
		t.Fatal("secret generation mutated user desired document")
	}
	kvs, err = descriptor.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, oldValue) {
		t.Fatal("rollback retrieval lost historical generation")
	}
	// An unavailable bound snapshot cannot fall back to the active candidate.
	rt.secretHistory = func(context.Context, string) ([]byte, error) { return nil, errors.New("unavailable") }
	before := runner.count("--reload")
	if _, err = descriptor.Update(ctx, oldValue, newValue, nil); err == nil {
		t.Fatal("missing bound history accepted")
	}
	if runner.count("--reload") != before {
		t.Fatal("unresolvable bound generation reached reload")
	}
	assertRendered(oldSecret, newSecret)
	if _, err = descriptor.Create(ctx, desired.FRRValue(doc, desired.FRRApplied)); err == nil {
		t.Fatal("generation-enabled runtime accepted unbound credentials")
	}
}

// A later dependent failure exercises the real transaction journal and rollback.
type frrGenerationFailure struct{ *frrConfigDescriptor }

func (*frrGenerationFailure) Name() string { return "generation.fail" }
func (*frrGenerationFailure) KeyOf(proto.Message) scheduler.Key {
	return scheduler.Join("generation.fail", "one")
}
func (*frrGenerationFailure) Dependencies(proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: desired.FRRConfigKey}}
}
func (*frrGenerationFailure) Create(context.Context, proto.Message) (any, error) {
	return nil, errors.New("injected later operation failure")
}
func (*frrGenerationFailure) Retrieve(context.Context) ([]scheduler.KV, error) { return nil, nil }
