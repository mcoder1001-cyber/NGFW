package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"ngfw/agent/internal/renderers/strongswan"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultMissingInstalledEnginePreflightFailsClosed(t *testing.T) {
	p := &SealedPreparation{Resolver: strongswan.SecretResolverFunc(func(context.Context, string) ([]byte, error) {
		t.Fatal("preflight resolves plaintext")
		return nil, ErrEngine
	}), Readiness: func(context.Context) error { return nil }}
	if p.Preflight(context.Background()) == nil {
		t.Fatal("uninstalled runtime advertised ready")
	}
	p.Readiness = nil
	if p.Preflight(context.Background()) == nil {
		t.Fatal("missing cache proof advertised ready")
	}
}
func TestInstallationUnitDigestMatchesSource(t *testing.T) {
	data, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "deploy", "systemd", "ngfw-ra@.service"))
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != expectedRAUnitSHA256 {
		t.Fatal("unit change requires paired preflight contract")
	}
}
func TestIntegrationActualArtifactReadonlyPreflight(t *testing.T) {
	prefix := os.Getenv("NGFW_RA_ENGINE_ROOT")
	helper := os.Getenv("NGFW_RA_HELPER")
	if os.Getenv("NGFW_INTEGRATION") != "1" || prefix == "" || helper == "" {
		t.Skip("requires authenticated owned engine artifact")
	}
	unit, e := filepath.Abs(filepath.Join("..", "..", "..", "..", "deploy", "systemd", "ngfw-ra@.service"))
	if e != nil {
		t.Fatal(e)
	}
	i := EngineInstallation{Prefix: prefix, Helper: helper, Unit: unit, OSRelease: "/etc/os-release", PackageStatus: "/var/lib/dpkg/status"}
	calls := 0
	p := &SealedPreparation{Installation: &i, Resolver: strongswan.SecretResolverFunc(func(context.Context, string) ([]byte, error) {
		t.Fatal("preflight resolves plaintext")
		return nil, ErrEngine
	}), Readiness: func(context.Context) error { calls++; return nil }}
	if e := p.Preflight(context.Background()); e != nil {
		t.Fatal("actual artifact readiness", e)
	}
	if calls != 1 {
		t.Fatal("cache readiness not verified")
	}
	changed := i
	changed.Helper = "/bin/true"
	p.Installation = &changed
	if p.Preflight(context.Background()) == nil {
		t.Fatal("foreign ELF without packaged helper digest accepted")
	}
	changed = i
	changed.Unit = "/etc/passwd"
	if p.Preflight(context.Background()) == nil {
		t.Fatal("foreign unit accepted")
	}
	p.Installation = &i
	p.Readiness = func(context.Context) error { return ErrEngine }
	if p.Preflight(context.Background()) == nil {
		t.Fatal("unopened cache advertised ready")
	}
}

func TestDaemonUnitDoesNotExposeHostNamespaceOrOwnershipReceipts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "deploy", "systemd", "ngfw-ra@.service"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"hostnetns", "namespace-exports", "snapshot-receipt", "BindReadOnlyPaths=/run/ngfw/ra/%i "} {
		if strings.Contains(text, forbidden) {
			t.Fatal("daemon unit exposes ownership-only material", forbidden)
		}
	}
	var readOnly []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "BindReadOnlyPaths=") {
			readOnly = append(readOnly, strings.Fields(strings.TrimPrefix(line, "BindReadOnlyPaths="))...)
		}
	}
	required := []string{"network.json", "netns", "strongswan.conf", "swanctl.conf", "private", "x509", "x509ca", "x509crl"}
	for _, name := range required {
		found := false
		for _, path := range readOnly {
			if path == "/run/ngfw/ra/%i/"+name {
				found = true
			}
		}
		if !found {
			t.Fatal("required exact daemon asset is missing", name)
		}
	}
	for _, path := range readOnly {
		if strings.HasPrefix(path, "/run/ngfw/ra/") && !strings.HasPrefix(path, "/run/ngfw/ra/%i/") {
			t.Fatal("broad/foreign instance bind", path)
		}
	}
	if !strings.Contains(text, "/run/ngfw/ra/%i:ro,mode=0700") || !strings.Contains(text, "BindPaths=/run/ngfw/ra/%i/daemon\n") {
		t.Fatal("private instance view and writable VICI child required")
	}
	if !strings.Contains(text, "CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_IPC_LOCK\n") || !strings.Contains(text, "PrivatePIDs=yes\n") || !strings.Contains(text, "SystemCallFilter=~@mount @reboot @swap @raw-io @debug\n") {
		t.Fatal("daemon hardening contract weakened")
	}
}
