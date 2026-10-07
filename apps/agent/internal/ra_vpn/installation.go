package ravpn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

// EngineInstallation is configured only by trusted wiring. A private fixture
// may point at its owned offline artifact; profiles cannot choose these paths.
type EngineInstallation struct{ Prefix, Helper, Unit, OSRelease, PackageStatus string }

// DefaultEngineInstallation fixes the installed engine, helper and ABI evidence paths.
func DefaultEngineInstallation() EngineInstallation {
	return EngineInstallation{Prefix: "/opt/ngfw-ra", Helper: "/usr/lib/ngfw/ngfw-ra-daemon", Unit: "/usr/lib/systemd/system/ngfw-ra@.service", OSRelease: "/etc/os-release", PackageStatus: "/var/lib/dpkg/status"}
}

const expectedRAUnitSHA256 = "0ab102d0538360d947e48a437723a67d1abcd602605ed1bfb2974ee4e2ac9246"
const expectedEngineBuild = "version=6.1.0\nsource_sha256=fe6c97481298767213cfc2e9a1da29fdd8018d481ff4cb9cf0283099654f20d4\nrelease_fingerprint=948F158A4E76A27BF3D07532DF42C170B34DBA77\n"

func trustedInstallationFile(path string, limit int64, executable bool) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrEngine
	}
	var original unix.Stat_t
	if unix.Lstat(path, &original) != nil || original.Uid != 0 {
		return nil, ErrEngine
	}
	// Resolving a symlink cannot bypass ownership of its original directory.
	for parent := filepath.Dir(path); parent != "/"; parent = filepath.Dir(parent) {
		var s unix.Stat_t
		if unix.Lstat(parent, &s) != nil || s.Uid != 0 || s.Mode&unix.S_IFMT != unix.S_IFDIR || s.Mode&0022 != 0 && s.Mode&unix.S_ISVTX == 0 {
			return nil, ErrEngine
		}
	}
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return nil, ErrEngine
	}
	fd, e := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrEngine
	}
	defer func() { _ = unix.Close(fd) }()
	parts := strings.Split(strings.TrimPrefix(resolved, "/"), "/")
	for _, part := range parts[:len(parts)-1] {
		next, e := unix.Openat(fd, part, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return nil, ErrEngine
		}
		_ = unix.Close(fd)
		fd = next
		var s unix.Stat_t
		if unix.Fstat(fd, &s) != nil || s.Uid != 0 || s.Mode&0022 != 0 && s.Mode&unix.S_ISVTX == 0 {
			return nil, ErrEngine
		}
	}
	child, e := unix.Openat(fd, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrEngine
	}
	f := os.NewFile(uintptr(child), "private engine artifact")
	defer func() { _ = f.Close() }()
	var s unix.Stat_t
	if unix.Fstat(child, &s) != nil || s.Uid != 0 || s.Mode&unix.S_IFMT != unix.S_IFREG || s.Mode&0022 != 0 || s.Size > limit || executable && s.Mode&0111 == 0 {
		return nil, ErrEngine
	}
	data, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || len(data) > int(limit) {
		return nil, ErrEngine
	}
	return data, nil
}
func releaseFields(data []byte) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = strings.Trim(value, "\"'")
		}
	}
	return values
}
func runtimePackages(data []byte) map[string]string {
	values := map[string]string{}
	for _, paragraph := range strings.Split(string(data), "\n\n") {
		fields := map[string]string{}
		for _, line := range strings.Split(paragraph, "\n") {
			k, v, ok := strings.Cut(line, ": ")
			if ok {
				fields[k] = v
			}
		}
		if fields["Status"] == "install ok installed" && fields["Architecture"] == "amd64" {
			values[fields["Package"]] = fields["Version"]
		}
	}
	return values
}

// Preflight performs read-only checks. It never invokes a daemon, modifies a
// namespace, starts a unit or writes credentials merely to advertise readiness.
func (p *SealedPreparation) Preflight(ctx context.Context) error {
	if p == nil || p.Resolver == nil || p.Readiness == nil || ctx.Err() != nil || p.Readiness(ctx) != nil {
		return ErrEngine
	}
	installation := DefaultEngineInstallation()
	if p.Installation != nil {
		installation = *p.Installation
	}
	return installation.Verify(ctx)
}

// Verify checks the installed artifact, ABI and fixed unit without running a daemon.
func (i EngineInstallation) Verify(ctx context.Context) error {
	if ctx.Err() != nil || runtime.GOARCH != "amd64" || !filepath.IsAbs(i.Prefix) || filepath.Clean(i.Prefix) != i.Prefix {
		return ErrEngine
	}
	build, e := trustedInstallationFile(filepath.Join(i.Prefix, "share/ngfw/engine-build.txt"), 4096, false)
	if e != nil || string(build) != expectedEngineBuild {
		return ErrEngine
	}
	data, e := trustedInstallationFile(filepath.Join(i.Prefix, "share/ngfw/engine-abi.json"), 16384, false)
	if e != nil {
		return ErrEngine
	}
	var abi struct {
		Format          int               `json:"format"`
		Architecture    string            `json:"architecture"`
		OS              map[string]string `json:"os"`
		RuntimePackages map[string]string `json:"runtimePackages"`
	}
	if json.Unmarshal(data, &abi) != nil || abi.Format != 1 || abi.Architecture != "x86_64" || abi.OS["ID"] != "ubuntu" || abi.OS["VERSION_ID"] != "26.04" || len(abi.RuntimePackages) != 3 {
		return ErrEngine
	}
	release, e := trustedInstallationFile(i.OSRelease, 16384, false)
	if e != nil {
		return ErrEngine
	}
	fields := releaseFields(release)
	if fields["ID"] != "ubuntu" || fields["VERSION_ID"] != "26.04" {
		return ErrEngine
	}
	status, e := trustedInstallationFile(i.PackageStatus, 64<<20, false)
	if e != nil {
		return ErrEngine
	}
	installed := runtimePackages(status)
	for _, name := range []string{"libc6", "libssl3t64", "libsystemd0"} {
		if abi.RuntimePackages[name] == "" || installed[name] != abi.RuntimePackages[name] {
			return ErrEngine
		}
	}
	unit, e := trustedInstallationFile(i.Unit, 16384, false)
	if e != nil {
		return ErrEngine
	}
	hash := sha256.Sum256(unit)
	if hex.EncodeToString(hash[:]) != expectedRAUnitSHA256 {
		return ErrEngine
	}
	for _, path := range []string{i.Helper, filepath.Join(i.Prefix, "sbin/charon-systemd"), filepath.Join(i.Prefix, "sbin/swanctl")} {
		data, e := trustedInstallationFile(path, 64<<20, true)
		if e != nil || len(data) < 4 || !bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}) {
			return ErrEngine
		}
		if path == i.Helper {
			receipt, e := trustedInstallationFile(path+".sha256", 128, false)
			digest := sha256.Sum256(data)
			if e != nil || strings.TrimSpace(string(receipt)) != hex.EncodeToString(digest[:]) {
				return ErrEngine
			}
		}
	}
	for _, plugin := range []string{"kernel-netlink", "socket-default", "vici", "openssl", "random", "nonce", "pem", "pkcs1", "pkcs8", "x509", "pubkey", "revocation", "constraints", "eap-identity", "eap-mschapv2", "md4", "eap-tls", "eap-radius"} {
		path := filepath.Join(i.Prefix, "lib/ipsec/plugins/libstrongswan-"+plugin+".so")
		resolved, e := filepath.EvalSymlinks(path)
		if e != nil || !strings.HasPrefix(resolved, i.Prefix+"/") {
			return ErrEngine
		}
		data, e := trustedInstallationFile(path, 16<<20, false)
		if e != nil || len(data) < 4 || !bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}) {
			return ErrEngine
		}
	}
	if ctx.Err() != nil {
		return ErrEngine
	}
	return nil
}
