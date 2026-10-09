package agent

import (
	"context"
	"encoding/base64"
	"ngfw/agent/internal/subsystems"
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
		//nolint:gosec // Read only generated fixture filenames under this test's private state directory.
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

func TestWireguardSecretServiceDryRunApplyRevertRestart(t *testing.T) {
	dir := t.TempDir()
	s := newSvc(t, coretest.New(), dir)
	cache, err := secretchannel.Open(dir, s.owner)
	if err != nil {
		t.Fatal(err)
	}
	s.secrets = cache
	if err = subsystems.SetWireguardSecrets(s.owner, cache.WireguardRef, cache.ResolveWireguard); err != nil {
		t.Fatal(err)
	}
	bundle := func(label string) *ngfwv1.SecretBundle {
		return &ngfwv1.SecretBundle{Values: map[string][]byte{"key/w7-site-a": []byte(base64.StdEncoding.EncodeToString(wgVector(label))), "psk/w7-b1": []byte(base64.StdEncoding.EncodeToString(wgVector("psk" + label)))}}
	}
	ds := doc(t, wgDoc(t))
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "wg-sealed-first", DesiredState: ds, Subsystems: allDomains, SecretBundle: bundle("first")}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	old, err := subsystems.WireguardSecretsFor(s.owner).Ref("key/w7-site-a")
	if err != nil {
		t.Fatal(err)
	}
	dry, err := s.DryRun(context.Background(), &ngfwv1.DryRunRequest{DesiredState: ds, Subsystems: allDomains, SecretBundle: bundle("dry")})
	if err != nil || !dry.GetOk() {
		t.Fatal(dry, err)
	}
	after, _ := subsystems.WireguardSecretsFor(s.owner).Ref("key/w7-site-a")
	if after != old {
		t.Fatal("dry-run changed active WG key")
	}
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "wg-sealed-next", DesiredState: ds, Subsystems: allDomains, SecretBundle: bundle("next"), ConfirmTimeoutSec: 600}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	next, _ := subsystems.WireguardSecretsFor(s.owner).Ref("key/w7-site-a")
	if next == old {
		t.Fatal("rotation unchanged")
	}
	s.revert("wg-sealed-next")
	after, err = subsystems.WireguardSecretsFor(s.owner).Ref("key/w7-site-a")
	if err != nil || after != old {
		t.Fatal("confirm revert did not restore WG key", err)
	}
	restored, err := secretchannel.Open(dir, s.owner)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewService(ServiceConfig{Owner: s.owner, VPP: s.vpp, Scheduler: s.sched, StateDir: dir, SecretCache: restored})
	if err != nil {
		t.Fatal(err)
	}
	if err = subsystems.SetWireguardSecrets(s.owner, restored.WireguardRef, restored.ResolveWireguard); err != nil {
		t.Fatal(err)
	}
	after, err = subsystems.WireguardSecretsFor(s.owner).Ref("key/w7-site-a")
	if err != nil || after != old || restarted.secrets.Active() != s.st.meta.SecretBundle {
		t.Fatal("restart lost WG generation", err)
	}
}
