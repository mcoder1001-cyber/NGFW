//go:build vrxtestsecrets

package subsystems

// Test builds only (go build -tags vrxtestsecrets): the slot-local WireGuard secret fixture of the
// F-wireguard host checks. The product agent has no way to receive secret material yet
// (PENDING-secret-channel); this file is the envelope's "slot-local fixture resolver", never part of a
// product build.
//
// VRX_TEST_WG_SECRETS names a JSON file {"key/<name>": "<wg genkey text>", "psk/<name>": "…"}: a
// regular file, mode 0600 or stricter, owned by the agent's user. Its values are test vectors derived
// from VRX_TEST_PSK_F-wireguard_* labels; nothing here logs them.

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"syscall"
)

// EnvWireguardTestSecrets names the fixture file.
const EnvWireguardTestSecrets = "VRX_TEST_WG_SECRETS"

func init() { wireguardFixture = loadWireguardFixture }

func loadWireguardFixture(s *WireguardSecrets, log *slog.Logger) error {
	path := os.Getenv(EnvWireguardTestSecrets)
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // test fixture path from the slot env
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !ok || int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("%s must be a regular 0600 file owned by this user", path)
	}
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return err
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("%s: not a JSON object of reference → key text", path)
	}
	for ref, text := range m {
		if err := s.PutBase64(ref, text); err != nil {
			return err
		}
	}
	log.Warn("TEST BUILD: WireGuard secret fixture loaded (never in a product build)", "file", path, "secrets", s.Len())
	return nil
}
