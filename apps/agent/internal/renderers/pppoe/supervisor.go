package pppoe

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"ngfw/agent/internal/renderers"
)

// Apply reconciles the on-disk pppd config and the running per-session units to `sessions` (F-pppoe-client,
// agent side). It writes the rendered files, removes the files and stops the units of sessions that are gone,
// runs `daemon-reload` when any unit file changed, and (re)starts a session's unit only when its peer file or
// unit changed — an unchanged session is left dialed so a no-op Apply never drops the link. Starting pppd and
// mirroring the negotiated address into VPP need /dev/ppp and VPP; this drives systemctl and is unit-tested
// with a recording runner. The live dial is proven on the lab host (F-pppoe-client-host).
//
// It never enables/disables units (shared-host rules §3) and only touches units it owns (`ngfw-pppoe-*`).
func (r *Renderer) Apply(ctx context.Context, runner renderers.Runner, sessions []Session) error {
	files, err := r.Render(sessions)
	if err != nil {
		return err
	}
	want := map[string]bool{} // host interfaces we should be running
	for _, s := range sessions {
		want[s.HostIf] = true
	}

	// unchanged peer/unit content per session before writing (to decide restarts)
	changed := map[string]bool{}
	for hostIf := range want {
		peer := r.paths.PeersDir + "/ngfw-" + hostIf
		unit := r.paths.UnitDir + "/ngfw-pppoe-" + hostIf + ".service"
		changed[hostIf] = !sameOnDisk(peer, files[peer]) || !sameOnDisk(unit, files[unit])
		if _, err := os.Stat(r.paths.StateDir + "/" + hostIf + ".ipv6.pending"); err == nil {
			changed[hostIf] = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("pppoe: transition evidence unavailable")
		}
		// dhcpv6 -> slaac leaves +ipv6 in the peer unchanged, but changes
		// the helper/config. Stop the old generation before replacing them.
		for _, path := range r.sessionFiles(hostIf) {
			if strings.HasPrefix(path, r.paths.StateDir+"/") {
				continue
			}
			if file, exists := files[path]; exists {
				changed[hostIf] = changed[hostIf] || !sameOnDisk(path, file)
			} else if _, err := os.Stat(path); err == nil {
				changed[hostIf] = true
			}
		}
	}

	stale := r.installedHostIfs()
	unitFilesChanged := false
	for hostIf := range stale {
		if want[hostIf] {
			continue
		}
		// gone: stop the unit, then remove its files
		if err := r.StopIPv6(ctx, hostIf); err != nil {
			return err
		}
		if err := r.systemctl(ctx, runner, "stop", unitName(hostIf)); err != nil {
			return err
		}
		for _, p := range r.sessionFiles(hostIf) {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("pppoe: remove %s: %w", p, err)
			}
		}
		unitFilesChanged = true
	}

	if len(want) == 0 {
		// no sessions left: the shared secrets files (not owned by any single session) go too
		for _, p := range []string{r.paths.ChapSecrets, r.paths.PapSecrets} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("pppoe: remove %s: %w", p, err)
			}
		}
	}
	// A kept session may have dropped an optional file (IPv6 turned off, dhcpv6 → slaac): remove what is no longer
	// rendered. Hook state files are the hooks' own and stay.
	for _, hostIf := range sortedKeys(want) {
		if changed[hostIf] {
			if err := r.StopIPv6(ctx, hostIf); err != nil {
				return err
			}
			if stale[hostIf] {
				if err := r.systemctl(ctx, runner, "stop", unitName(hostIf)); err != nil {
					return err
				}
			}
			for _, suffix := range []string{".state", ".state6", ".pd"} {
				if err := os.Remove(r.paths.StateDir + "/" + hostIf + suffix); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
		for _, p := range r.sessionFiles(hostIf) {
			if _, keep := files[p]; keep || strings.HasPrefix(p, r.paths.StateDir+"/") {
				continue
			}
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("pppoe: remove %s: %w", p, err)
			}
		}
	}
	if len(files) > 0 {
		for _, dir := range r.paths.Dirs() {
			if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // secrets are the 0600 files inside
				return fmt.Errorf("pppoe: mkdir %s: %w", dir, err)
			}
		}
		if err := renderers.WriteFiles(files); err != nil {
			return err
		}
	}
	for hostIf := range want {
		if changed[hostIf] {
			unitFilesChanged = true
		}
	}
	if unitFilesChanged {
		if err := r.systemctl(ctx, runner, "daemon-reload"); err != nil {
			return err
		}
	}
	// (re)start changed or new sessions, deterministic order
	for _, hostIf := range sortedKeys(want) {
		if changed[hostIf] {
			if err := r.ResumeIPv6(hostIf); err != nil {
				return err
			}
			if err := r.systemctl(ctx, runner, "restart", unitName(hostIf)); err != nil {
				return err
			}
			if err := r.CompleteIPv6Transition(hostIf); err != nil {
				return err
			}
		}
	}
	return nil
}

func unitName(hostIf string) string { return "ngfw-pppoe-" + hostIf + ".service" }

func (r *Renderer) systemctl(ctx context.Context, runner renderers.Runner, args ...string) error {
	_, err := runner.Run(ctx, renderers.Command{Path: SystemctlBin, Args: args})
	if err != nil {
		return fmt.Errorf("pppoe: systemctl %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// sessionFiles are every file that belongs to one session's host interface.
func (r *Renderer) sessionFiles(hostIf string) []string {
	return []string{
		r.paths.PeersDir + "/ngfw-" + hostIf,
		r.paths.IPUpDir + "/ngfw-" + hostIf,
		r.paths.IPDownDir + "/ngfw-" + hostIf,
		r.paths.IPv6UpDir + "/ngfw-" + hostIf,
		r.paths.IPv6DownDir + "/ngfw-" + hostIf,
		r.paths.dhcpcdConf(hostIf),
		r.paths.dhcp6Script(hostIf),
		r.paths.ipv6Helper(hostIf),
		r.paths.UnitDir + "/ngfw-pppoe-" + hostIf + ".service",
		r.paths.StateDir + "/" + hostIf + ".state",
		r.paths.StateDir + "/" + hostIf + ".state6",
		r.paths.StateDir + "/" + hostIf + ".pd",
		r.paths.StateDir + "/" + hostIf + ".ipv6.pid",
	}
}

// installedHostIfs are the host interfaces with a unit file on disk (what a previous Apply left).
func (r *Renderer) installedHostIfs() map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(r.paths.UnitDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		n := e.Name()
		if h, ok := strings.CutPrefix(n, "ngfw-pppoe-"); ok {
			if h, ok := strings.CutSuffix(h, ".service"); ok {
				out[h] = true
			}
		}
	}
	return out
}

func sameOnDisk(path string, want renderers.File) bool {
	b, err := os.ReadFile(path) //nolint:gosec // path is a fixed product location under the renderer's dirs
	return err == nil && string(b) == string(want.Content)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
