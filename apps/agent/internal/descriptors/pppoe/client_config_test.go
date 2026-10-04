package pppoe

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureResolver struct {
	value string
	fail  bool
}

func (r *fixtureResolver) Resolve(context.Context, string) ([]byte, error) {
	if r.fail {
		return nil, errors.New("resolver failed")
	}
	return []byte(r.value), nil
}

type clientStub struct {
	calls    int
	sessions []ren.Session
}

func (r *clientStub) Apply(_ context.Context, s []ren.Session) error {
	r.calls++
	r.sessions = s
	return nil
}
func clientDocument() *ngfwv1.DesiredState {
	return &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{"wan": {Pppoe: &ngfwv1.Pppoe{Parent: proto.String("parent"), Username: proto.String("user"), PasswordRef: proto.String("password/test")}}, "parent": {Lcp: &ngfwv1.InterfaceLcp{HostIfName: proto.String("tap0")}}}}
}
func TestClientValidateStagesExactlyOnceAndRedactsChecker(t *testing.T) {
	stub := &clientStub{}
	dir := t.TempDir()
	d := NewClientConfig(stub, ren.New(ren.WithPaths(ren.PathsUnder(dir))), filepath.Join(dir, "applied.pb"))
	fixture := "NGFW_TEST_PSK_F-pppoe-client-wiring"
	d.SetResolver(&fixtureResolver{value: fixture})
	calls := 0
	d.Check = func(ctx context.Context, r *ren.Renderer, s []ren.Session) error {
		calls++
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		files, err := r.Render(s)
		if err != nil {
			t.Fatal(err)
		}
		for path := range files {
			if !strings.Contains(path, "ngfw-pppoe-validate-") {
				t.Fatal("checker did not receive staged paths")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("staged file missing", err)
			}
		}
		if err := r.Validate(s); err != nil {
			t.Fatal(err)
		}
		if s[0].Password != fixture {
			t.Fatal("not resolved")
		}
		return errors.New("bad config " + fixture)
	}
	err := d.Validate(context.Background(), ClientConfigKey, clientDocument(), nil)
	if err == nil || strings.Contains(err.Error(), fixture) || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatal(err)
	}
	if calls != 1 || stub.calls != 0 {
		t.Fatal("checker must precede apply", calls, stub.calls)
	}
	if _, err := os.Stat(d.manifest); !os.IsNotExist(err) {
		t.Fatal("validation wrote product manifest")
	}
}
func TestClientMissingSecretDryRunPassesApplyNamesPointer(t *testing.T) {
	stub := &clientStub{}
	dir := t.TempDir()
	d := NewClientConfig(stub, ren.New(ren.WithPaths(ren.PathsUnder(dir))), filepath.Join(dir, "applied.pb"))
	if err := d.Validate(context.Background(), ClientConfigKey, clientDocument(), nil); err != nil {
		t.Fatal(err)
	}
	_, err := d.Create(context.Background(), clientDocument())
	var validation *scheduler.ValidationError
	if !errors.As(err, &validation) || validation.Pointer != "/interfaces/wan/pppoe/passwordRef" || stub.calls != 0 {
		t.Fatal(err, stub.calls)
	}
	if _, err := os.Stat(d.manifest); !os.IsNotExist(err) {
		t.Fatal("missing secret wrote manifest")
	}
}
func TestClientManifestNeverContainsResolvedPassword(t *testing.T) {
	stub := &clientStub{}
	dir := t.TempDir()
	d := NewClientConfig(stub, ren.New(ren.WithPaths(ren.PathsUnder(dir))), filepath.Join(dir, "applied.pb"))
	fixture := "NGFW_TEST_PSK_F-pppoe-client-wiring"
	d.SetResolver(&fixtureResolver{value: fixture})
	if _, err := d.Create(context.Background(), clientDocument()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(d.manifest)
	if err != nil || strings.Contains(string(b), fixture) {
		t.Fatal("secret leaked into recovery state")
	}
	got := &ngfwv1.DesiredState{}
	if proto.Unmarshal(b, got) != nil || got.Interfaces["wan"].Pppoe.GetPasswordRef() != "password/test" {
		t.Fatal("reference lost")
	}
	if stub.sessions[0].MTU != 1492 || !stub.sessions[0].DefaultRoute || !stub.sessions[0].MSSClamp || stub.sessions[0].HoldoffSec != 5 {
		t.Fatal("proto defaults lost")
	}
}
