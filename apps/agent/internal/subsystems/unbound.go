package subsystems

// F-unbound-chrony-syslog: the host services the agent renders — Unbound (services.dns.resolvers), chrony
// (services.ntp) and the rsyslog export (management.syslog) — each as one singleton scheduler descriptor around its
// RF-3/RF-4 renderer (D-109 d: no renderer stage in the agent core), plus DF-8's VPP DNS cache descriptors (dns.go).
//
// Whose daemons (decision, see docs/status/tasks/F-unbound-chrony-syslog.md): only the globals owner (D-071: the
// product agent on a real box) renders into the product paths (/etc/unbound, /etc/chrony, /etc/rsyslog.d) — the
// box's resolver, clock and syslog are box-wide singletons like VPP's globals. Every other agent (a test slot, the
// dev host's main stack with VRX_GLOBALS_OWNER=0) renders into its own path space /run/vrx-test/<owner>/{unbound,
// chrony/agent,rsyslog} with loopback-only listeners, and never restarts or signals a daemon: start/restart requests
// are recorded (D-079) and reported by the state RPCs; the slot harness runs the slot instances. The host's own
// chrony.service and rsyslog.service are never touched by such an agent.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"ngfw/agent/internal/descriptors/dns"
	"ngfw/agent/internal/renderers/chrony"
	"ngfw/agent/internal/renderers/rsyslog"
	"ngfw/agent/internal/renderers/unbound"
	"ngfw/agent/internal/scheduler"
)

// HostServices are the host-service descriptors of one agent (owner): the state RPCs read their daemons.
type HostServices struct {
	Unbound *unbound.Descriptor
	Chrony  *chrony.Descriptor
	Rsyslog *rsyslog.Descriptor
	// Product is true when the renderers use the product paths (the globals owner).
	Product bool
	// GlobalsOwner is D-071's flag (the VPP DNS cache is applied, not only required).
	GlobalsOwner bool
}

var (
	hostServicesMu sync.Mutex
	hostServices   = map[string]*HostServices{}
)

// HostServicesOf returns the host services registered for owner (nil before Register).
func HostServicesOf(owner string) *HostServices {
	hostServicesMu.Lock()
	defer hostServicesMu.Unlock()
	return hostServices[owner]
}

var slotOwnerRe = regexp.MustCompile(`^w([0-9]{1,2})$`)

// slotOf is the slot number of a test-slot owner ("w10" → 10); 0 for any other owner.
func slotOf(owner string) int {
	if m := slotOwnerRe.FindStringSubmatch(owner); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// EnvHostServicesDir overrides the base directory of an agent that is not the globals owner (default
// /run/vrx-test/<owner>; unit tests point it at a temporary directory).
const EnvHostServicesDir = "VRX_HOST_SERVICES_DIR"

// slotRunDir is the base directory of a non-owner agent's host-service instances.
func slotRunDir(owner string) string {
	if d := os.Getenv(EnvHostServicesDir); d != "" && filepath.IsAbs(d) {
		return filepath.Clean(d)
	}
	return filepath.Join("/run/vrx-test", owner)
}

// mkdirShared creates dir and its missing parents 0755 (never re-modes an existing directory: the slot run dir
// must stay traversable for the _chrony test daemons, D-106/D-107).
func mkdirShared(dir string) error {
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", dir)
		}
		return nil
	}
	if err := mkdirShared(filepath.Dir(dir)); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) { //nolint:gosec // traversable run dir, no secrets
		return err
	}
	return os.Chmod(dir, 0o755) //nolint:gosec // G302: see above
}

// registerUnboundChronySyslog builds the three renderers for env (product or slot paths, see the file comment),
// registers their descriptors and DF-8's dns descriptors with r, and records them for the state RPCs. It touches no
// file: a non-owner's slot directories are prepared before the first write (WithPrepare).
func registerUnboundChronySyslog(r scheduler.Registry, env Env) error {
	log := env.Log.With("feature", "unbound-chrony-syslog")
	hs := &HostServices{Product: env.GlobalsOwner, GlobalsOwner: env.GlobalsOwner}
	ub, ubPrep := newUnbound(env)
	ch, chPrep := newChrony(env)
	rs, rsPrep, err := newRsyslog(env)
	if err != nil {
		return err
	}
	hs.Unbound = unbound.NewDescriptor(ub, log.With("daemon", "unbound")).WithPrepare(ubPrep)
	hs.Chrony = chrony.NewDescriptor(ch, log.With("daemon", "chronyd")).WithPrepare(chPrep)
	hs.Rsyslog = rsyslog.NewDescriptor(rs, log.With("daemon", "rsyslogd")).WithPrepare(rsPrep)
	r.Register(hs.Unbound)
	r.Register(hs.Chrony)
	r.Register(hs.Rsyslog)
	registerDNSCache(r, env)
	hostServicesMu.Lock()
	hostServices[env.Owner] = hs
	hostServicesMu.Unlock()
	log.Info("host services wired", "product_paths", hs.Product, "unbound_conf", ub.Paths().Conf(),
		"chrony_conf", ch.Paths().Conf(), "rsyslog_conf", rs.Paths().ConfFile, "vpp_dns_cache", map[bool]string{true: "applied (globals owner)", false: "required only"}[env.GlobalsOwner])
	return nil
}

// newUnbound returns the Unbound renderer of env: product paths for the globals owner, else the slot's
// (<slot dir>/unbound, loopback only, idle port 3<slot>53) and the hook that prepares that directory and seeds the
// trust anchor from dns-root-data.
func newUnbound(env Env) (*unbound.Renderer, func() error) {
	if env.GlobalsOwner {
		return unbound.New(unbound.NewRunner()), nil
	}
	p := unbound.PathsUnder(filepath.Join(slotRunDir(env.Owner), "unbound"), uint32(3000+slotOf(env.Owner)*100+53)) //nolint:gosec // slots 0–99
	prep := func() error {
		if err := mkdirShared(p.ConfDir); err != nil {
			return fmt.Errorf("unbound slot dir: %w", err)
		}
		if _, err := os.Stat(p.TrustAnchor); errors.Is(err, fs.ErrNotExist) {
			key, err := os.ReadFile(p.RootKey)
			if err != nil {
				return fmt.Errorf("unbound trust anchor (dns-root-data): %w", err)
			}
			if err := os.WriteFile(p.TrustAnchor, key, 0o644); err != nil { //nolint:gosec // public trust anchor, unbound updates it
				return fmt.Errorf("unbound trust anchor: %w", err)
			}
		}
		return nil
	}
	return unbound.New(unbound.NewRunner(), unbound.WithPaths(p)), prep
}

// servicesDescriptors / managementDescriptors are this feature's descriptors of the `services` and `management`
// domains (Domains, subsystems.go A1). F-kea-dhcp-relay (services) and F-dashboard-prom-alarms (management) add
// theirs to the same entries.
var (
	servicesDescriptors   = []string{unbound.Name, chrony.Name, dns.NameNameServer, dns.NameEnable}
	managementDescriptors = []string{rsyslog.Name}
)
