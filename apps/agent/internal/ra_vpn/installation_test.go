package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"ngfw/agent/internal/renderers/strongswan"
	"os"
	"path/filepath"
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
