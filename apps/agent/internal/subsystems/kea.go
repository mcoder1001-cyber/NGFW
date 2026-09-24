package subsystems

// F-kea-dhcp-relay: the `services` domain's DHCP families — the Kea singletons (kea.dhcp4, kea.dhcp6: RF-3's renderer
// wrapped in one scheduler descriptor per daemon, D-109 d), the VPP relay (dhcp.proxy, dhcp.proxy-vss) and the relay
// records (dhcp.relay) — plus the read-only DHCP runtime of the DhcpLeases RPC. subsystems.go carries only the domain
// constant, the Domains entry and one registerKea call.
//
// Environment (read once at start):
//
//	VRX_KEA_MODE      "product" (default): ProductPaths (/etc/kea, /run/kea sockets, /var/lib/kea), the Kea binaries of
//	                  NewRunner, no interface mapper until linux-cp (P12) provides one — a server interface is refused
//	                  (RF-3 review L7). "test": kea.TestPaths(<owner>) under /run/vrx-test/<owner>/kea, the checkers run
//	                  in VRX_KEA_NETNS through `ip netns exec` (test runner only), interfaces mapped by VRX_KEA_IFMAP,
//	                  no "<if>/<addr>" bindings (the mapped netdev does not carry the VPP address). "off": no Kea
//	                  descriptors (the relay families stay).
//	VRX_KEA_NETNS     test mode: the network namespace the Kea daemons run in (default ns-<owner>-a).
//	VRX_KEA_IFMAP     test mode: "<vpp-if>=<linux-if>[,…]" — the only interfaces a server may name (the lab stand-in for
//	                  the linux-cp mapping); any other name is refused.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/kea"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Environment of the DHCP families.
const (
	EnvKeaMode  = "VRX_KEA_MODE"
	EnvKeaNetns = "VRX_KEA_NETNS"
	EnvKeaIfMap = "VRX_KEA_IFMAP"
)

// DHCPRuntime is what the DhcpLeases RPC reads: the Kea renderer (nil when VRX_KEA_MODE=off), the DHCPv4 client
// descriptor (leases per interface) and the VPP connection.
type DHCPRuntime struct {
	Kea    *kea.Renderer
	Client *dhcp.ClientDescriptor
	VPP    vpp.Client
}

var (
	dhcpMu       sync.Mutex
	dhcpRuntimes = map[string]*DHCPRuntime{}
)

// DHCPFor returns the DHCP runtime registered for an agent (state dir + owner); nil when none.
func DHCPFor(stateDir, owner string) *DHCPRuntime {
	dhcpMu.Lock()
	defer dhcpMu.Unlock()
	return dhcpRuntimes[stateDir+"\x00"+owner]
}

// DHCP returns this agent's DHCP runtime.
func (w *Wiring) DHCP() *DHCPRuntime { return DHCPFor(w.env.StateDir, w.env.Owner) }

// registerKea registers the Kea singletons (unless off), the relay families scoped to the slot's table range, and
// the DHCP runtime of this agent.
func (w *Wiring) registerKea(r scheduler.Registry) error {
	c, owner := w.env.Client, w.env.Owner
	rng, err := SlotIDRange()
	if err != nil {
		return err
	}
	var opts []dhcp.Option
	if rng != nil {
		lo, hi := rng.Lo, rng.Hi
		opts = append(opts, dhcp.WithVRFScope(func(v uint32) bool { return v >= lo && v <= hi }))
	}
	rt := &DHCPRuntime{Client: w.dhcpClient, VPP: c}
	mode := strings.TrimSpace(os.Getenv(EnvKeaMode))
	switch mode {
	case "", "product", "test":
		kr, desc, err := keaRenderer(owner, mode == "test")
		if err != nil {
			return err
		}
		rt.Kea = kr
		log := w.env.Log.With("family", "kea")
		r.Register(kea.NewDescriptor(kr, 4, kea.WithDescriptorInterfaceKey(dfkit.DefaultInterfaceKey), kea.WithLogger(log)))
		r.Register(kea.NewDescriptor(kr, 6, kea.WithDescriptorInterfaceKey(dfkit.DefaultInterfaceKey), kea.WithLogger(log)))
		w.env.Log.Info("kea wired", "mode", desc, "conf", kr.Paths().ConfDir, "sockets", kr.Paths().RunDir)
	case "off":
		w.env.Log.Info("kea descriptors off (" + EnvKeaMode + "=off)")
	default:
		return fmt.Errorf("%s=%q: want product, test or off", EnvKeaMode, mode)
	}
	r.Register(dhcp.NewProxy(c, opts...))
	r.Register(dhcp.NewProxyVSS(c, opts...))
	store := &dhcp.FileRelayStore{Path: filepath.Join(w.env.StateDir, "dhcp-relays-"+owner+".json")}
	r.Register(dhcp.NewRelay(c, store, opts...))
	dhcpMu.Lock()
	dhcpRuntimes[w.env.StateDir+"\x00"+owner] = rt
	dhcpMu.Unlock()
	return nil
}

var ifMapRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./:-]{0,62}=[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}$`)

// keaRenderer builds the renderer of the mode (product or test) and describes it for the start-up log.
func keaRenderer(owner string, test bool) (*kea.Renderer, string, error) {
	if !test {
		p := kea.ProductPaths()
		return kea.New(kea.NewRunner(p), kea.WithPaths(p)), "product", nil
	}
	p := kea.TestPaths(owner)
	p.InterfacePrefix = "" // the explicit VRX_KEA_IFMAP is the scope
	if ns := strings.TrimSpace(os.Getenv(EnvKeaNetns)); ns != "" {
		p.Netns = ns
	}
	if err := p.Validate(); err != nil {
		return nil, "", err
	}
	m := map[string]string{}
	if s := strings.TrimSpace(os.Getenv(EnvKeaIfMap)); s != "" {
		for _, pair := range strings.Split(s, ",") {
			pair = strings.TrimSpace(pair)
			if !ifMapRe.MatchString(pair) {
				return nil, "", fmt.Errorf("%s: %q is not <vpp-if>=<linux-if>", EnvKeaIfMap, pair)
			}
			k, v, _ := strings.Cut(pair, "=")
			m[k] = v
		}
	}
	mapper := func(name string) (string, error) {
		if ln, ok := m[name]; ok {
			return ln, nil
		}
		return "", fmt.Errorf("interface %q is not in %s (test mode maps only the listed interfaces)", name, EnvKeaIfMap)
	}
	runner := kea.NewRunner(p)
	runner.Allow = renderers.NewAllowlist(append(kea.Binaries(), kea.IPBin)...) // test only: checkers inside the netns
	r := kea.New(runner, kea.WithPaths(p), kea.WithInterfaceMapper(mapper), kea.WithAddressBinding(false))
	return r, fmt.Sprintf("test (netns %s, %d mapped interfaces)", p.Netns, len(m)), nil
}
