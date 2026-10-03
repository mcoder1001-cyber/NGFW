package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/secretchannel"
)

func nativeBundle(material string) *ngfwv1.SecretBundle {
	return &ngfwv1.SecretBundle{Values: map[string][]byte{"psk/site": []byte(material)}}
}
func TestSecretSnapshotsFollowApplyConfirmRevertAndRestart(t *testing.T) {
	dir := t.TempDir()
	s := newSvc(t, coretest.New(), dir)
	cache, e := secretchannel.Open(dir, s.owner)
	if e != nil {
		t.Fatal(e)
	}
	s.secrets = cache
	ds := doc(t, `{"interfaces":{}}`)
	first := &ngfwv1.ApplyRequest{TxnId: "secrets-first", DesiredState: ds, SecretBundle: nativeBundle("NGFW_TEST_PSK_FIRST")}
	mustStatus(t, apply(t, s, first), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(first.SecretBundle.Values) != 0 {
		t.Fatal("RPC plaintext retained")
	}
	old := s.st.meta.SecretBundle
	restarted, restartErr := NewService(ServiceConfig{Owner: s.owner, VPP: s.vpp, Scheduler: s.sched, StateDir: dir, SecretCache: cache})
	if restartErr != nil || restarted.secrets.Active() != old {
		t.Fatal("restart did not restore selected cache", restartErr)
	}
	if _, restartErr = NewService(ServiceConfig{Owner: s.owner, VPP: s.vpp, Scheduler: s.sched, StateDir: dir}); restartErr == nil {
		t.Fatal("restart accepted missing secret cache")
	}
	if old == "" || s.st.meta.ConfirmedSecretBundle != old {
		t.Fatal("snapshot not atomically confirmed")
	}
	pending := &ngfwv1.ApplyRequest{TxnId: "secrets-pending", DesiredState: ds, ConfirmTimeoutSec: 600, SecretBundle: nativeBundle("NGFW_TEST_PSK_NEXT")}
	mustStatus(t, apply(t, s, pending), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	next := s.st.meta.SecretBundle
	if next == old || s.st.meta.ConfirmedSecretBundle != old {
		t.Fatal("pending key overwrote confirmed binding")
	}
	reloaded, e := loadState(dir, s.owner)
	if e != nil {
		t.Fatal(e)
	}
	if reloaded.meta.SecretBundle != next || reloaded.meta.ConfirmedSecretBundle != old {
		t.Fatal("crash atomic state bindings missing")
	}
	_, e = s.Apply(context.Background(), &ngfwv1.ApplyRequest{TxnId: "secrets-pending", DesiredState: ds, ConfirmTimeoutSec: 600, SecretBundle: nativeBundle("NGFW_TEST_PSK_DIFFERENT")})
	if status.Code(e) != codes.Aborted {
		t.Fatalf("reused txn accepted changed secret: %v", e)
	}
	s.revert("secrets-pending")
	if s.st.meta.SecretBundle != old || cache.Active() != old {
		t.Fatal("confirm timeout did not restore old key")
	}
	restored, e := secretchannel.Open(dir, s.owner)
	if e != nil {
		t.Fatal(e)
	}
	if e = restored.Activate(old); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"agent-state.json", "desired.pb"} {
		raw, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), "NGFW_TEST_PSK_") {
			t.Fatal("plaintext in desired/txn state", name)
		}
	}
}
func TestDryRunSecretsRestoreConfirmedBindings(t *testing.T) {
	dir := t.TempDir()
	s := newSvc(t, coretest.New(), dir)
	cache, e := secretchannel.Open(dir, s.owner)
	if e != nil {
		t.Fatal(e)
	}
	s.secrets = cache
	id, e := cache.Stage(map[string][]byte{"psk/site": []byte("NGFW_TEST_PSK_CONFIRMED")})
	if e != nil {
		t.Fatal(e)
	}
	_ = cache.Activate(id)
	req := &ngfwv1.DryRunRequest{DesiredState: doc(t, `{"interfaces":{}}`), SecretBundle: nativeBundle("NGFW_TEST_PSK_PREVIEW")}
	r, e := s.DryRun(context.Background(), req)
	if e != nil || !r.GetOk() {
		t.Fatal(r, e)
	}
	if cache.Active() != id || len(req.SecretBundle.Values) != 0 {
		t.Fatal("validation mutated active secrets or retained plaintext")
	}
}
