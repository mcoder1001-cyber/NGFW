package pppoe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/secretchannel"
)

// Regression (live PPPoE acceptance, docs/status/tasks/claude-pppoe-wip.md defect 1): the descriptor must resolve
// "password/<name>" through the REAL sealed secret channel wired exactly as internal/agent/agent.go wires it, not
// through a fixture that ignores the reference. Before the fix every real Apply rolled back with
// "PPPoE password reference is unavailable".
func TestClientResolvesPasswordFromRealSecretChannel(t *testing.T) {
	stub := &clientStub{}
	dir := t.TempDir()
	d := NewClientConfig(stub, ren.New(ren.WithPaths(ren.PathsUnder(dir))), filepath.Join(dir, "applied.pb"))
	store, err := secretchannel.Open(filepath.Join(dir, "cache"), "w9")
	if err != nil {
		t.Fatal(err)
	}
	const password = "NGFW_TEST_PSK_F-pppoe-secret-channel" //nolint:gosec // required non-production fixture marker
	id, err := store.Stage(map[string][]byte{"password/test": []byte(password)})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Activate(id); err != nil {
		t.Fatal(err)
	}
	wireProductionSecrets(d, store)

	if err = d.Validate(context.Background(), ClientConfigKey, clientDocument(), nil); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, err = d.Create(context.Background(), clientDocument()); err != nil {
		t.Fatalf("apply with sealed password: %v", err)
	}
	if stub.calls != 1 || len(stub.sessions) != 1 || stub.sessions[0].Password != password {
		t.Fatal("sealed password did not reach the runtime", stub.calls)
	}
	// Recovery manifest keeps only the reference.
	b, err := os.ReadFile(d.manifest)
	if err != nil || strings.Contains(string(b), password) || !strings.Contains(string(b), "password/test") {
		t.Fatal("manifest must hold the reference, never the password", err)
	}
	// Rendered config: the password appears only in the 0600 chap/pap secrets files.
	files, err := d.renderer.Render(stub.sessions)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for path, f := range files {
		if !strings.Contains(string(f.Content), password) {
			continue
		}
		base := filepath.Base(path)
		if f.Mode != 0o600 || !f.Secret || (base != "chap-secrets" && base != "pap-secrets") {
			t.Fatalf("password rendered into %s (mode %o, secret %v)", base, f.Mode, f.Secret)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("password missing from chap/pap secrets")
	}

	// A reference absent from the active snapshot still fails closed, naming only the pointer.
	doc := clientDocument()
	doc.Interfaces["wan"].Pppoe.PasswordRef = proto.String("password/absent")
	_, err = d.Create(context.Background(), doc)
	if err == nil || !strings.Contains(err.Error(), "password reference is unavailable") || strings.Contains(err.Error(), password) {
		t.Fatalf("absent reference: %v", err)
	}
}

// wireProductionSecrets mirrors internal/agent/agent.go: SetPppoeSecrets(cfg.Owner, cache.Text).
func wireProductionSecrets(d *ClientConfig, store *secretchannel.Store) {
	d.SetSecretSource(store.Text)
}
