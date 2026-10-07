package vppstartup

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Lab instances (LAB-vpp-per-slot, D-125 option F): `tools/lab vpp up <N>` starts a small VPP of
// test slot N beside the shared one (vpp.service, /run/vpp). Everything that instance creates lives
// under <root>/w<N>/vpp and /dev/shm/w<N>-*, so it can never take over the shared VPP's sockets or
// API segment; the slot numbering is docs/lab/shared-host-rules.md §1 (1–12, 14–32; 13 is tools/app's).

// DefaultLabRoot is the per-slot runtime root of the shared host (/run/ngfw-test/w<N>/…).
const DefaultLabRoot = "/run/ngfw-test"

// Lab instance sizing: no workers, no DPDK, a small main heap on normal pages and buffers on
// normal pages, so an instance needs no hugepages (the host's 4096 × 2 MiB stay for vpp.service).
// No poll-sleep-usec: VPP then sleeps a fixed interval and polls epoll with timeout 0 on every
// main-loop turn (vlib/file.c vlib_file_poll), measured at ~17 % of a core idle with 100 µs; without
// it the idle main thread blocks in epoll for up to 10 ms.
const (
	LabMainHeapMB        = 512
	LabStatsegMB         = 32
	LabPageSize          = "4k"
	SharedRuntimeDir     = "/run/vpp"
	maxLabSlot           = 32
	reservedToolsAppSlot = 13
)

// ErrLabSlot is wrapped when a slot number or a lab rendering is not acceptable.
var ErrLabSlot = errors.New("vppstartup: lab slot instance")

// LabSlot checks a slot number: 1–12 (12 = CI) and 14–32; 13 does not exist (its id range is
// tools/app's).
func LabSlot(slot int) error {
	if slot < 1 || slot > maxLabSlot || slot == reservedToolsAppSlot {
		return fmt.Errorf("%w: slot %d must be 1..12 or 14..32 (13 is reserved for tools/app; docs/lab/shared-host-rules.md §1)", ErrLabSlot, slot)
	}
	return nil
}

// LabSlotDir is the runtime directory of slot N's instance: <root>/w<N>/vpp.
func LabSlotDir(root string, slot int) string {
	return filepath.Join(root, fmt.Sprintf("w%d", slot), "vpp")
}

// LabSlotSettings returns the Settings of slot N's lab instance under root (DefaultLabRoot on the
// host; tests pass a temp dir): runtime dir, log, CLI/API/stats sockets and startup.conf all in
// LabSlotDir, api-segment prefix w<N>, a 512M main heap and buffers on 4k pages.
func LabSlotSettings(root string, slot int) (Settings, error) {
	if err := LabSlot(slot); err != nil {
		return Settings{}, err
	}
	if _, err := PathToken(root); err != nil {
		return Settings{}, fmt.Errorf("%w: lab root: %v", ErrLabSlot, err)
	}
	dir := LabSlotDir(root, slot)
	if dir == SharedRuntimeDir || strings.HasPrefix(dir+"/", SharedRuntimeDir+"/") {
		return Settings{}, fmt.Errorf("%w: lab root %s would put slot %d inside the shared VPP's %s", ErrLabSlot, root, slot, SharedRuntimeDir)
	}
	return Settings{
		ConfPath:         filepath.Join(dir, "startup.conf"),
		LogFile:          filepath.Join(dir, "vpp.log"),
		CLISocket:        filepath.Join(dir, "cli.sock"),
		StatsSocket:      filepath.Join(dir, "stats.sock"),
		Group:            "vpp",
		RuntimeDir:       dir,
		APIPrefix:        fmt.Sprintf("w%d", slot),
		APISocket:        filepath.Join(dir, "api.sock"),
		StatsegMB:        LabStatsegMB,
		MainHeapMB:       LabMainHeapMB,
		MainHeapPageSize: LabPageSize,
		BuffersPageSize:  LabPageSize,
	}, nil
}

// CheckLabRendering fails closed unless rendered (a startup.conf produced with s =
// LabSlotSettings) can only ever touch the slot's own paths: no /run/vpp/ path anywhere, socksvr
// with its own socket-name (never `default`, which is /run/vpp/api.sock), the runtime dir and API
// prefix of s, and no dpdk section (a lab instance never probes PCI devices).
func CheckLabRendering(rendered []byte, s Settings) error {
	if s.RuntimeDir == "" || s.APISocket == "" || s.APIPrefix == "" {
		return fmt.Errorf("%w: settings are not a lab instance's (runtime dir, API socket and API prefix are required)", ErrLabSlot)
	}
	root, err := Parse(rendered)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLabSlot, err)
	}
	lines := root.Canonical()
	has := map[string]bool{}
	for _, l := range lines {
		has[l] = true
		switch {
		case strings.Contains(l, SharedRuntimeDir+"/") || strings.HasSuffix(l, " "+SharedRuntimeDir):
			return fmt.Errorf("%w: rendering refers to the shared VPP's %s: %q", ErrLabSlot, SharedRuntimeDir, l)
		case l == "socksvr > default":
			return fmt.Errorf("%w: `socksvr { default }` would listen on %s/api.sock", ErrLabSlot, SharedRuntimeDir)
		case l == "dpdk {}" || strings.HasPrefix(l, "dpdk > "):
			return fmt.Errorf("%w: a lab instance renders no dpdk section (disable dpdk_plugin.so): %q", ErrLabSlot, l)
		}
	}
	for _, want := range []string{
		"unix > runtime-dir " + s.RuntimeDir,
		"unix > cli-listen " + s.CLISocket,
		"socksvr > socket-name " + s.APISocket,
		"statseg > socket-name " + s.StatsSocket,
		"api-segment > prefix " + s.APIPrefix,
	} {
		if !has[want] {
			return fmt.Errorf("%w: rendering lacks %q", ErrLabSlot, want)
		}
	}
	return nil
}
