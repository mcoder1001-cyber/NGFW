package pki

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/scheduler"
)

// failing is a daemon-stage descriptor registered after pki.files whose Create fails on demand: it makes a
// transaction roll back after the files were written.
type failing struct{ fail bool }

func (*failing) Name() string                                      { return "test.fail" }
func (*failing) KeyOf(proto.Message) scheduler.Key                 { return scheduler.Join("test.fail", "x") }
func (*failing) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (*failing) Stage() scheduler.Stage                            { return scheduler.StageDaemon }
func (f *failing) Create(context.Context, proto.Message) (any, error) {
	if f.fail {
		return nil, errors.New("injected failure after the PKI files")
	}
	return nil, nil
}
func (f *failing) Update(ctx context.Context, _, n proto.Message, _ any) (any, error) {
	return f.Create(ctx, n)
}
func (*failing) Delete(context.Context, proto.Message, any) error { return nil }
func (*failing) Retrieve(context.Context) ([]scheduler.KV, error) { return nil, nil }

func newSched(t *testing.T, d *Descriptor, extra ...scheduler.Descriptor) *scheduler.Scheduler {
	t.Helper()
	reg := scheduler.NewRegistry()
	reg.Register(d)
	for _, x := range extra {
		reg.Register(x)
	}
	return scheduler.New(reg, slog.New(slog.DiscardHandler))
}

func apply(t *testing.T, s *scheduler.Scheduler, kvs []scheduler.KV, scope scheduler.Scope) *scheduler.TxnResult {
	t.Helper()
	res := s.Apply(context.Background(), kvs, scope)
	if res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply: %s: %v", res.Outcome, res.Err)
	}
	return res
}

func TestDescriptorDeclaresPersistentManifest(t *testing.T) {
	d := NewDescriptor(newMat(t, t.TempDir(), t.TempDir(), nil))
	if err := persist.Declared(d); err != nil {
		t.Fatal(err)
	}
	if err := persist.Check(d); err != nil {
		t.Fatal(err)
	}
}

// TestRestartRematerialises is the restart-safety check of the reduced DoD: apply, stop the "agent", delete the
// files (simulated loss), start a fresh materialiser + scheduler over the same state dir, resync the stored desired
// state → every file is back (Retrieve == desired) well within 30 s.
func TestRestartRematerialises(t *testing.T) {
	p := newTestPKI(t)
	root, state := t.TempDir(), t.TempDir()
	src := p.source()
	m1 := newMat(t, root, state, src)
	set, errs := m1.Plan(context.Background(), gwFiles())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	desired := []scheduler.KV{{Key: Key, Value: set}}
	scope := scheduler.Only(Name)
	s1 := newSched(t, NewDescriptor(m1))
	apply(t, s1, desired, scope)
	if p, _ := s1.Plan(context.Background(), desired, scope); !p.Empty() {
		t.Fatalf("second plan not empty: %+v", p.Ops)
	}

	// agent stopped; the files are lost
	for _, rel := range []string{"x509/w5-gw.pem", "private/w5-gw.pem", "x509ca/w5-ca.pem", "x509crl/w5-ca.pem"} {
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	// agent started again: new runtime over the same manifest, then the start-up resync
	start := time.Now()
	m2 := newMat(t, root, state, src)
	s2 := newSched(t, NewDescriptor(m2))
	plan, err := s2.Plan(context.Background(), desired, scope)
	if err != nil || plan.Empty() {
		t.Fatalf("plan after the loss: %v empty=%v", err, plan.Empty())
	}
	apply(t, s2, desired, scope)
	took := time.Since(start)
	got, err := m2.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, set) {
		t.Fatalf("after the resync Retrieve != desired:\n got %v\nwant %v", got, set)
	}
	if took > 30*time.Second {
		t.Fatalf("re-materialised in %v, want < 30 s", took)
	}
	t.Logf("restart: 4 files re-materialised in %v; Retrieve == desired (%d files)", took.Round(time.Millisecond), len(got.GetFiles()))
}

// TestRollbackKeepsPreviousFiles: a transaction that fails after the PKI files were updated is rolled back to the
// previous file set; a commit that drops the tunnel removes the unreferenced files (Retrieve shows none).
func TestRollbackKeepsPreviousFiles(t *testing.T) {
	p := newTestPKI(t)
	root, state := t.TempDir(), t.TempDir()
	src := p.source()
	src.Put("cert/w5-gw2", p.leafCert)
	src.Put("key/w5-gw2", p.leafKey)
	m := newMat(t, root, state, src)
	f := &failing{}
	s := newSched(t, NewDescriptor(m), f)
	scope := scheduler.Only(Name, "test.fail")
	marker := scheduler.KV{Key: scheduler.Join("test.fail", "x"), Value: &structpb.Struct{}}

	first, errs := m.Plan(context.Background(), gwFiles())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	apply(t, s, []scheduler.KV{{Key: Key, Value: first}}, scope)

	// second revision: certificate renamed w5-gw → w5-gw2, and the transaction fails after the files
	second, errs := m.Plan(context.Background(), []File{{Kind: KindCert, Name: "w5-gw2", Ref: "cert/w5-gw2"}, {Kind: KindKey, Name: "w5-gw2", Ref: "key/w5-gw2"}})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	f.fail = true
	res := s.Apply(context.Background(), []scheduler.KV{{Key: Key, Value: second}, marker}, scope)
	if res.Outcome != scheduler.OutcomeRolledBack {
		t.Fatalf("outcome %s (%v), want ROLLED_BACK: %+v", res.Outcome, res.Err, res)
	}
	got, _ := m.Retrieve(context.Background())
	if !proto.Equal(got, first) {
		t.Fatalf("after the rollback Retrieve != the previous set:\n got %v\nwant %v", got, first)
	}
	if _, err := os.Stat(filepath.Join(root, "x509", "w5-gw2.pem")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("x509/w5-gw2.pem survived the rollback")
	}

	// the tunnel is removed: the object is no longer desired → every file goes
	f.fail = false
	apply(t, s, nil, scheduler.Only(Name))
	if got, _ := m.Retrieve(context.Background()); len(got.GetFiles()) != 0 {
		t.Fatalf("files left after the removal: %v", got)
	}
	if kvs, _ := s.Retrieve(context.Background(), scheduler.Only(Name)); len(kvs) != 0 {
		t.Fatalf("scheduler Retrieve after the removal: %v", kvs)
	}
}

// TestEvidenceTree writes the files of a cert-auth tunnel into $NGFW_PKI_EVIDENCE_DIR (the slot's /run/ngfw-test/w5/…)
// and leaves them for `stat` (the acceptance's "file tree + modes"); the caller deletes the directory afterwards.
func TestEvidenceTree(t *testing.T) {
	dir := os.Getenv("NGFW_PKI_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("NGFW_PKI_EVIDENCE_DIR not set (evidence run only)")
	}
	p := newTestPKI(t)
	m := newMat(t, filepath.Join(dir, "swanctl"), filepath.Join(dir, "state"), p.source())
	set, errs := m.Plan(context.Background(), gwFiles())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	s := newSched(t, NewDescriptor(m))
	apply(t, s, []scheduler.KV{{Key: Key, Value: set}}, scheduler.Only(Name))
	kvs, err := s.Retrieve(context.Background(), scheduler.Only(Name))
	if err != nil || len(kvs) != 1 {
		t.Fatalf("retrieve: %v %v", kvs, err)
	}
	for _, f := range kvs[0].Value.(*ngfwv1.PkiFileStateSet).GetFiles() {
		t.Logf("Retrieve %s/%s ref=%s mode=%#o fingerprint=%s", Kind(f.GetKind()).Dir(), f.GetName()+".pem", f.GetRef(), f.GetMode(), f.GetFingerprint())
	}
}
