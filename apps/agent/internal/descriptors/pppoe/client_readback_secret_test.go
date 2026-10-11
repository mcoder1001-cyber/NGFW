package pppoe

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/secretchannel"
	"os"
	"path/filepath"
	"testing"
)

type readbackClient struct {
	clientStub
	remembered []ren.Session
}

func (r *readbackClient) Remember(s []ren.Session) { r.remembered = append([]ren.Session(nil), s...) }
func TestUnavailableAppliedSecretIsDriftAndWithdrawable(t *testing.T) {
	dir := t.TempDir()
	r := &readbackClient{}
	d := NewClientConfig(r, ren.New(ren.WithPaths(ren.PathsUnder(dir))), filepath.Join(dir, "applied.pb"))
	store, e := secretchannel.Open(filepath.Join(dir, "cache"), "w20")
	if e != nil {
		t.Fatal(e)
	}
	activate := func(values map[string][]byte) {
		t.Helper()
		id, e := store.Stage(values)
		if e != nil {
			t.Fatal(e)
		}
		if e = store.Activate(id); e != nil {
			t.Fatal(e)
		}
	}
	activate(map[string][]byte{"password/test": []byte("NGFW_TEST_PSK_readback-old")})
	d.SetSecretSource(store.Text)
	doc := clientDocument()
	if _, e = d.Create(t.Context(), doc); e != nil {
		t.Fatal(e)
	}
	files, e := d.renderer.Render(r.sessions)
	if e != nil {
		t.Fatal(e)
	}
	for path := range files {
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e = renderers.WriteFiles(files); e != nil {
		t.Fatal(e)
	}
	if rows, e := d.Retrieve(t.Context()); e != nil || len(rows) != 1 || !proto.Equal(rows[0].Value, doc) {
		t.Fatalf("healthy readback %v", e)
	}
	activate(map[string][]byte{})
	rows, e := d.Retrieve(t.Context())
	if e != nil || len(rows) != 1 {
		t.Fatalf("candidate withdrawal cannot retrieve old manifest: %v", e)
	}
	drift, ok := rows[0].Value.(*structpb.Struct)
	if !ok || drift.Fields["drift"].GetStringValue() == "" {
		t.Fatal("unverified files claimed healthy")
	}
	if len(r.remembered) != 1 || r.remembered[0].Password != "" || r.remembered[0].Iface != "wan" {
		t.Fatal("teardown metadata lost/retained credential")
	}
	before := r.calls
	if _, e = d.Create(t.Context(), doc); !errors.Is(e, errPasswordUnavailable) || r.calls != before {
		t.Fatal("missing committed credential accepted", e)
	}
	next := proto.Clone(doc).(*ngfwv1.DesiredState)
	next.Interfaces["wan"].Pppoe.PasswordRef = proto.String("password/new")
	if _, e = d.Create(t.Context(), next); !errors.Is(e, errPasswordUnavailable) {
		t.Fatal("missing new credential accepted", e)
	}
	if e = d.Delete(context.Background(), rows[0].Value, nil); e != nil {
		t.Fatal(e)
	}
	if len(r.sessions) != 0 {
		t.Fatal("withdrawal kept session")
	}
	if _, e = os.Stat(d.manifest); !os.IsNotExist(e) {
		t.Fatal("manifest retained")
	}
	activate(map[string][]byte{"password/test": []byte("NGFW_TEST_PSK_readback-old")})
	if _, e = d.Create(t.Context(), doc); e != nil {
		t.Fatal(e)
	}
	activate(map[string][]byte{"password/new": []byte("NGFW_TEST_PSK_readback-new")})
	if old, e := d.Retrieve(t.Context()); e != nil || len(old) != 1 {
		t.Fatalf("replacement cannot retrieve old manifest: %v", e)
	}
	if _, e = d.Update(t.Context(), doc, next, nil); e != nil {
		t.Fatal(e)
	}
	if r.sessions[0].Password != "NGFW_TEST_PSK_readback-new" {
		t.Fatal("replacement credential not selected")
	}
}
func TestUnavailableAppliedSecretDoesNotBypassStructure(t *testing.T) {
	dir := t.TempDir()
	d := NewClientConfig(&clientStub{}, ren.New(ren.WithPaths(ren.PathsUnder(dir))), filepath.Join(dir, "applied.pb"))
	doc := clientDocument()
	doc.Interfaces["wan"].Pppoe.Username = proto.String("unsafe\nusername")
	b, e := proto.Marshal(doc)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(d.manifest, b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = d.Retrieve(t.Context()); e == nil {
		t.Fatal("invalid username accepted")
	}
}
