package subsystems

// F-object-model: the `objects` domain (Domains["objects"]) — the agent-local objects.*
// descriptor family (internal/objects) and the lifecycle of its FQDN resolver. subsystems.go
// only carries the domain constant, the Domains entry and one registerObjectModel call.

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/scheduler"
)

// Environment of the objects domain (read once at start).
const (
	// EnvFQDNRefresh is the FQDN refresh interval in seconds (default 60; clamped to 30–3600).
	EnvFQDNRefresh = "VRX_OBJECTS_FQDN_REFRESH_SEC"
	// EnvFQDNMaxStale is how long a failing FQDN keeps its last good answers, in seconds (default 86400 =
	// 24 h, D-129; clamped to 60 s – 30 days); after that the object expands to nothing, with a warning.
	EnvFQDNMaxStale = "VRX_OBJECTS_FQDN_MAX_STALE_SEC"
	// EnvDNSServers lists "ip:port" DNS servers (comma-separated) the FQDN resolver asks instead of
	// the system configuration (/etc/resolv.conf) — for test slots and their in-process responder;
	// the product agent leaves it unset.
	EnvDNSServers = "VRX_OBJECTS_DNS_SERVERS"
)

// objectModelDescriptors is Domains["objects"].
func objectModelDescriptors() []string { return objects.DescriptorNames() }

// registerObjectModel opens the objects runtime of this owner (store + FQDN state in the state
// dir), registers the objects.* family with r and starts the resolver; Agent.Stop closes it through
// the close seam (w.OnClose: resolver stopped, pending store changes and FQDN state written).
func (w *Wiring) registerObjectModel(r scheduler.Registry) error {
	cfg, err := objectModelConfig(w.env)
	if err != nil {
		return err
	}
	rt, err := objects.Open(cfg)
	if err != nil {
		return err
	}
	objects.Register(r, rt)
	rt.Start()
	w.OnClose(rt.Close)
	servers := "system (/etc/resolv.conf)"
	if s := os.Getenv(EnvDNSServers); s != "" {
		servers = s
	}
	w.env.Log.Info("objects domain wired", "store", rt.Store().Path(), "fqdn_objects", len(rt.FQDNStates()),
		"fqdn_refresh", objects.ClampRefresh(cfg.Refresh).String(), "fqdn_max_stale", objects.ClampMaxStale(cfg.MaxStale).String(), "dns", servers)
	return nil
}

// ObjectModel returns this agent's objects runtime (applied objects document, FQDN resolver).
func (w *Wiring) ObjectModel() *objects.Runtime {
	return objects.RuntimeFor(w.env.StateDir, w.env.Owner)
}

// objectModelConfig reads EnvFQDNRefresh, EnvFQDNMaxStale and EnvDNSServers. A malformed value is an error, never
// a silent default.
func objectModelConfig(env Env) (objects.Config, error) {
	cfg := objects.Config{StateDir: env.StateDir, Owner: env.Owner, Log: env.Log.With("family", "objects")}
	if s := strings.TrimSpace(os.Getenv(EnvFQDNRefresh)); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return cfg, fmt.Errorf("%s=%q: want a number of seconds", EnvFQDNRefresh, s)
		}
		cfg.Refresh = time.Duration(n) * time.Second
		if c := objects.ClampRefresh(cfg.Refresh); c != cfg.Refresh {
			env.Log.Warn("FQDN refresh interval clamped", "env", EnvFQDNRefresh, "value", s, "used", c.String())
			cfg.Refresh = c
		}
	}
	if s := strings.TrimSpace(os.Getenv(EnvFQDNMaxStale)); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return cfg, fmt.Errorf("%s=%q: want a number of seconds", EnvFQDNMaxStale, s)
		}
		cfg.MaxStale = time.Duration(n) * time.Second
		if c := objects.ClampMaxStale(cfg.MaxStale); c != cfg.MaxStale {
			env.Log.Warn("FQDN maximum staleness clamped", "env", EnvFQDNMaxStale, "value", s, "used", c.String())
			cfg.MaxStale = c
		}
	}
	if s := strings.TrimSpace(os.Getenv(EnvDNSServers)); s != "" {
		var servers []string
		for _, f := range strings.Split(s, ",") {
			ap, err := netip.ParseAddrPort(strings.TrimSpace(f))
			if err != nil {
				return cfg, fmt.Errorf("%s=%q: want ip:port[,ip:port…]: %w", EnvDNSServers, s, err)
			}
			servers = append(servers, ap.String())
		}
		cfg.Lookup = objects.NetLookup(servers)
	}
	return cfg, nil
}
