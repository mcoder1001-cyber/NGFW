package pki

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/secretchannel"
	"os"
	"path/filepath"
	"testing"
)

type activeSource struct{ store *secretchannel.Store }

func (s activeSource) Resolve(_ context.Context, ref string) ([]byte, error) {
	return s.store.Text(ref)
}

func TestRollbackWithCandidateOnlySealedSecrets(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(fmt.Sprint(rename), func(t *testing.T) {
			old, rotated := newTestPKI(t), newTestPKI(t)
			cache, err := secretchannel.Open(t.TempDir(), "pki-rollback")
			if err != nil {
				t.Fatal(err)
			}
			activate := func(values map[string][]byte) string {
				id, err := cache.Stage(values)
				if err != nil {
					t.Fatal(err)
				}
				if err = cache.Activate(id); err != nil {
					t.Fatal(err)
				}
				return id
			}
			activate(map[string][]byte{"cert/w5-gw": old.leafCert, "key/w5-gw": old.leafKey})
			root := t.TempDir()
			m := newMat(t, root, t.TempDir(), activeSource{cache})
			t.Cleanup(m.Close)
			failure := &failing{}
			sched := newSched(t, NewDescriptor(m), failure)
			files := []File{{Kind: KindCert, Name: "w5-gw", Ref: "cert/w5-gw"}, {Kind: KindKey, Name: "w5-gw", Ref: "key/w5-gw"}}
			first, errs := m.Plan(context.Background(), files)
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			apply(t, sched, []scheduler.KV{{Key: Key, Value: first}}, scheduler.Only(Name))
			name := "w5-gw"
			if rename {
				name = "w5-new"
			}
			candidate := activate(map[string][]byte{"cert/" + name: rotated.leafCert, "key/" + name: rotated.leafKey})
			next, errs := m.Plan(context.Background(), []File{{Kind: KindCert, Name: name, Ref: "cert/" + name}, {Kind: KindKey, Name: name, Ref: "key/" + name}})
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			failure.fail = true
			result := sched.Apply(context.Background(), []scheduler.KV{{Key: Key, Value: next}, {Key: scheduler.Join("test.fail", "x"), Value: &structpb.Struct{}}}, scheduler.Only(Name, "test.fail"))
			if result.Outcome != scheduler.OutcomeRolledBack {
				t.Fatalf("rollback: %+v", result)
			}
			got, err := m.Retrieve(context.Background())
			if err != nil || !proto.Equal(got, first) {
				t.Fatalf("previous generation not restored: %v %v", got, err)
			}
			if cache.Active() != candidate {
				t.Fatal("descriptor changed secret selection")
			}
			key, err := os.ReadFile(filepath.Join(root, "private", "w5-gw.pem"))
			if err != nil || string(key) != string(old.leafKey) {
				t.Fatal("old key bytes not restored")
			}
			clear(key)
		})
	}
}

func TestRejectUnsafeDirectoriesAndSigningCertificates(t *testing.T) {
	p := newTestPKI(t)
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "private")); err != nil {
		t.Fatal(err)
	}
	m := newMat(t, root, t.TempDir(), p.source())
	t.Cleanup(m.Close)
	set, errs := m.Plan(context.Background(), gwFiles())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if err := m.Apply(context.Background(), set); err == nil {
		t.Fatal("symlink directory accepted")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("wrote outside owned root")
	}
	if _, err := check(KindCert, p.caCert); err == nil {
		t.Fatal("CA signing cert accepted as operational leaf")
	}
}
