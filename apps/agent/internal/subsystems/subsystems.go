// Package subsystems wires the descriptors of the product agent into one registry: which
// descriptors exist, which configuration domain (Health.subsystems) each belongs to, the
// persisted record stores they need, and the hooks the agent calls on VPP (re)connect.
//
// Domains implemented by this build:
//
//	interfaces  core: interface.loopback, interface-ip.table, interface-ip
//	            DF-1: interface (alias, observe-only), interface.subinterface, interface.admin-state,
//	                  interface.mtu, interface.mac-address, interface.promisc, interface.rx-mode
//	            DF-1: af-packet.host-interface (lab path, D-010)
//	            DF-8: dhcp.client
//	vrfs        core: vrf
//	routing     core: ip.route
//
// interface.rx-placement is registered (its interface kind is known) but belongs to no domain: the
// configuration has no leaf for it, so it is never planned.
package subsystems

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/descriptors/ikev2"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// Domain names (ROOT_KEYS of packages/schema).
const (
	Interfaces = "interfaces"
	VRFs       = "vrfs"
	Routing    = "routing"
	// New domain constants: one line under the feature's anchor (wave-A-hotspots A1).
	// wave-A: F-loopback-bvi-gso-lldp-span
	// wave-A: F-rpf-adl-pbr
	// wave-A: F-object-model
	// wave-A: F-acl
	// wave-A: F-host-acl-nftables
	// wave-A: F-nat44-ed-sessions
	Nat = "nat"
	// wave-A: P11
	// wave-A: F-wireguard
	// wave-A: F-kea-dhcp-relay
	// wave-A: F-unbound-chrony-syslog
)

// Domains maps each implemented configuration domain to the descriptors that realise it.
var Domains = map[string][]string{
	Interfaces: {
		core.LoopbackName,
		afpacket.HostInterfaceName,
		iface.SubinterfaceName,
		iface.AliasName,
		iface.AdminStateName,
		iface.MtuName,
		iface.MacAddressName,
		iface.PromiscName,
		iface.RxModeName,
		core.InterfaceTableName,
		core.InterfaceAddrName,
		dhcp.NameClient,
		// wave-A: F-bonding
		// wave-A: F-bridge-l2
		// wave-A: F-loopback-bvi-gso-lldp-span
		// wave-A: F-neighbors-ra
		// wave-A: F-rpf-adl-pbr
		// wave-A: P12
	},
	VRFs: {
		core.VRFName,
		// wave-A: F-vrf-static-ecmp
		// wave-A: F-neighbors-ra
	},
	Routing: {
		core.RouteName,
		// wave-A: F-vrf-static-ecmp
		// wave-A: F-neighbors-ra
		// wave-A: F-rpf-adl-pbr
		// wave-A: P12
	},
	// New domain entries: one `<Const>: {…}` entry under the feature's anchor (wave-A-hotspots A1).
	// wave-A: F-loopback-bvi-gso-lldp-span
	// wave-A: F-rpf-adl-pbr
	// wave-A: F-object-model
	// wave-A: F-acl
	// wave-A: F-host-acl-nftables
	// wave-A: F-nat44-ed-sessions
	Nat: natDomain(
		nat44EDDescriptors,
		// wave-A: F-nat44-ei-64-66-nptv6
		nat44EI6466NptDescriptors,
		// wave-BC: F-det44-map-dslite-cnat
	),
	// wave-A: P11
	// wave-A: F-wireguard
	// wave-A: F-kea-dhcp-relay
	// wave-A: F-unbound-chrony-syslog
}

// DomainOf returns the domain a descriptor belongs to ("" when none).
func DomainOf(descriptor string) string {
	for d, ds := range Domains {
		for _, n := range ds {
			if n == descriptor {
				return d
			}
		}
	}
	return ""
}

// Env is what the wiring needs.
type Env struct {
	Client vpp.Client
	Owner  string
	// StateDir holds the persisted stores (the agent's VRX_AGENT_STATE_DIR).
	StateDir string
	// Owned is the route owner table (P05 core).
	Owned ownertable.Set
	// GlobalsOwner is D-071's flag: only the product agent on a real box sets VPP-wide singletons.
	// This build registers no global descriptor; the flag is recorded for the families that do.
	GlobalsOwner bool
	Log          *slog.Logger
	// NetdevKind looks up Linux netdevs for the af_packet veth guard (D-105); nil = LinuxNetdevKind.
	NetdevKind NetdevKind
	// Publish is the agent's event sink (A5 seam): families that observe asynchronous changes
	// (neighbours, FQDN objects, IPsec SAs, WireGuard peers, routing) publish through
	// Wiring.Publish. nil = events are dropped (the default).
	Publish func(*vrxv1.Event)
	// Resync asks the agent for a full resync of its stored desired state (A5 seam, F-acl);
	// Wiring.RequestResync calls it. nil = no-op (the default).
	Resync func()
}

// Wiring is the result of Register: the stores and the hooks the agent calls.
type Wiring struct {
	env        Env
	identity   *Identity
	index      *IndexCache
	ifaceClaim *IfaceClaims
	boot       *dfkit.FileBootStore
	dhcpClient *dhcp.ClientDescriptor

	storesMu sync.Mutex
	keyed    map[string]*KeyedClaims
	classify *classify.FileStore
	vpnKeys  *vpn.Keyer
}

// Register builds every store (persisted in env.StateDir), installs the process-wide ones for
// env.Owner, and registers the descriptors of this build with r in dependency-friendly order (the
// scheduler's tie breaker): VRFs, interface creators, alias, attributes, addresses, routes.
func Register(r scheduler.Registry, env Env) (*Wiring, error) {
	if env.Log == nil {
		env.Log = slog.Default()
	}
	if env.NetdevKind == nil {
		env.NetdevKind = LinuxNetdevKind
	}
	w := &Wiring{env: env, identity: &Identity{}, keyed: map[string]*KeyedClaims{}}
	w.index = NewIndexCache(time.Second, func() (map[string]uint32, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		t, err := iface.Dump(ctx, env.Client, env.Owner)
		if err != nil {
			return nil, err
		}
		m := map[string]uint32{}
		for _, idx := range t.Indexes() {
			m[t.VPPName(idx)] = idx
		}
		return m, nil
	})
	var err error
	if w.ifaceClaim, err = OpenIfaceClaims(env.StateDir, env.Owner, w.identity, w.index); err != nil {
		return nil, err
	}
	iface.SetClaimStore(env.Owner, w.ifaceClaim) // D-075: persisted, never the in-memory default
	if w.boot, err = dfkit.NewFileBootStore(filepath.Join(env.StateDir, "boot-"+env.Owner+".json")); err != nil {
		return nil, err
	}
	df7.SetBootStore(env.Owner, w.boot) // D-076/D-080 applied-once records of the DF-7 families

	c, owner := env.Client, env.Owner
	core.Register(r, core.Env{Client: c, Owner: owner, Owned: env.Owned, IfRef: core.AliasInterfaceRef}) // D-065/D-073a
	r.Register(&vethOnly{Descriptor: afpacket.New(c, owner), kind: env.NetdevKind})                      // D-105: veth only
	// DF-1, in iface.Register's order, with the MTU/rx-mode "value equal to the default" tolerance
	r.Register(iface.NewSubinterface(c, owner))
	r.Register(iface.NewAdminState(c, owner))
	r.Register(newDefaultTolerant(iface.NewMtu(c, owner), iface.ErrMtuDefault, mtuInEffect(c, owner)))
	r.Register(iface.NewMacAddress(c, owner))
	r.Register(iface.NewPromisc(c, owner))
	r.Register(newDefaultTolerant(iface.NewRxMode(c, owner), iface.ErrRxModeDefault, rxModeInEffect(c, owner)))
	r.Register(iface.NewRxPlacement(c, owner))
	r.Register(iface.NewAlias(c, owner))
	w.dhcpClient = dhcp.NewClient(c, owner, dhcp.WithInterfaceKey(dfkit.DefaultInterfaceKey))
	r.Register(w.dhcpClient)
	// Feature families: one `<pkg>.Register(r, c, owner, opts…)` line under the feature's anchor; store
	// options only through the Wiring methods (wave-A-hotspots A1).
	// wave-A: F-bonding
	// wave-A: F-bridge-l2
	// wave-A: F-loopback-bvi-gso-lldp-span
	// wave-A: F-vrf-static-ecmp
	// wave-A: F-neighbors-ra
	// wave-A: F-rpf-adl-pbr
	// wave-A: F-object-model
	// wave-A: F-acl
	// wave-A: F-host-acl-nftables
	// wave-A: F-nat44-ed-sessions
	if err := w.registerNat44ED(r); err != nil {
		return nil, err
	}
	// wave-A: F-nat44-ei-64-66-nptv6
	if err := w.registerNat44EI6466Nptv6(r); err != nil {
		return nil, err
	}
	// wave-A: P11
	// wave-A: F-wireguard
	// wave-A: P12
	// wave-A: F-kea-dhcp-relay
	// wave-A: F-unbound-chrony-syslog
	return w, nil
}

// AfterResync runs after every full resync — the initial reconcile and the one on each VPP
// (re)connect: it releases this owner's quarantine holders whose parked sw_if_index is clean again
// (TD-3 Q2, ifsanitize.Release), so the index is free for the next creator. A holder that is still
// unclearable stays (admin down, owns the dirty index).
func (w *Wiring) AfterResync(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	n, err := ifsanitize.Release(rctx, w.env.Client, w.env.Owner)
	switch {
	case err != nil:
		w.env.Log.Warn("quarantine release after resync (VPP V19)", "released", n, "err", err)
	case n > 0:
		w.env.Log.Info("quarantine holders released after resync (VPP V19)", "released", n)
	}
}

// NetdevKind is the Linux netdev lookup of the af_packet veth guard; the projection uses the same one
// to refuse an existing non-veth netdev at validation time (D-105).
func (w *Wiring) NetdevKind() NetdevKind { return w.env.NetdevKind }

// Connected is P05's VPP (re)connect hook, called before the resync: it records the D-080 boot
// identity (claims of another VPP instance expire and are pruned from disk), invalidates cached
// interface indexes and tells DF-8's DHCP client that the API connection is new (its lease-event
// subscriptions must be re-made on this connection).
func (w *Wiring) Connected(ctx context.Context) {
	w.index.Invalidate()
	id, err := bootid.Current(ctx, w.env.Client)
	if err != nil {
		w.env.Log.Warn("VPP boot identity unavailable; claims on untagged interfaces are not trusted until it is", "err", err)
	} else if w.identity.Set(id) {
		n, perr := w.ifaceClaim.Prune()
		if perr != nil {
			w.env.Log.Error("prune interface claims", "err", perr)
		}
		w.storesMu.Lock()
		for fam, k := range w.keyed {
			if m, err := k.Prune(); err != nil {
				w.env.Log.Error("prune claims", "family", fam, "err", err)
			} else {
				n += m
			}
		}
		w.storesMu.Unlock()
		w.env.Log.Info("VPP boot identity", "identity", id.String(), "complete", id.Complete(), "expired_claims", n)
	}
	w.dhcpClient.Reconnected() // DF-8 (obligation): re-subscribe lease events on the new connection
}

// KeyedClaims opens (once per family) the persisted single-key claim store of a descriptor family
// for this owner: "acl" (acl.WithEtypeClaims, df2.WithClaims), "nat" (natcommon.WithClaims). The
// feature tasks that register those families pass it instead of the in-memory default.
func (w *Wiring) KeyedClaims(family string) (*KeyedClaims, error) {
	w.storesMu.Lock()
	defer w.storesMu.Unlock()
	if k, ok := w.keyed[family]; ok {
		return k, nil
	}
	k, err := OpenKeyedClaims(w.env.StateDir, family, w.env.Owner, w.identity)
	if err != nil {
		return nil, err
	}
	w.keyed[family] = k
	return k, nil
}

// IfaceClaims returns the persisted DF-1 claim store (also df6.WithClaims: df6.ClaimStore = iface.ClaimStore).
func (w *Wiring) IfaceClaims() *IfaceClaims { return w.ifaceClaim }

// BootStore returns the persisted D-076 applied-once store (pcap.Register, df6/df7 options).
func (w *Wiring) BootStore() dfkit.BootStore { return w.boot }

// ClassifyStore opens (once) the persisted DF-2 classify table store (<dir>/classify-<owner>.json)
// for classify.Register, ipfix.Register and ip_session_redirect.Register; it is bound to the VPP boot
// identity by the classify package itself (Instance/Reset, D-080).
func (w *Wiring) ClassifyStore() (classify.Store, error) {
	w.storesMu.Lock()
	defer w.storesMu.Unlock()
	if w.classify == nil {
		s, err := classify.OpenFileStore(filepath.Join(w.env.StateDir, "classify-"+w.env.Owner+".json"))
		if err != nil {
			return nil, err
		}
		w.classify = s
	}
	return w.classify, nil
}

// VPNKeyer opens (once) the agent-local HMAC key of DF-5's secret fingerprints (D-096):
// <state dir>/vpn-<owner>.key, 0600, created when missing.
func (w *Wiring) VPNKeyer() (*vpn.Keyer, error) {
	w.storesMu.Lock()
	defer w.storesMu.Unlock()
	if w.vpnKeys == nil {
		k, err := vpn.LoadOrCreateKeyFile(filepath.Join(w.env.StateDir, "vpn-"+w.env.Owner+".key"))
		if err != nil {
			return nil, err
		}
		w.vpnKeys = k
	}
	return w.vpnKeys, nil
}

// IPsecOptions are the persisted-store options DF-5's ipsec.Register must get from the product agent
// (D-096 / DF-5 Q12): the owner's file-backed BootStore (ownership records of SPDs, SAs, SPD bindings;
// the charon sweeper refuses an in-memory store), the D-071 globals flag and the D-096 keyer. P11 adds
// the secret resolver and the id range when it registers the family.
func (w *Wiring) IPsecOptions() ([]ipsec.Option, error) {
	k, err := w.VPNKeyer()
	if err != nil {
		return nil, err
	}
	return []ipsec.Option{ipsec.WithBootStore(w.boot), ipsec.WithKeyer(k), ipsec.WithGlobalsOwner(w.env.GlobalsOwner)}, nil
}

// IKEv2Options is IPsecOptions for ikev2.Register (D-076 applied-once responder hostname records).
func (w *Wiring) IKEv2Options() ([]ikev2.Option, error) {
	k, err := w.VPNKeyer()
	if err != nil {
		return nil, err
	}
	return []ikev2.Option{ikev2.WithBootStore(w.boot), ikev2.WithKeyer(k), ikev2.WithGlobalsOwner(w.env.GlobalsOwner)}, nil
}

// BeforeTxn is called before every transaction: interface indexes may have changed behind us.
func (w *Wiring) BeforeTxn() { w.index.Invalidate() }

// Identity returns the current VPP boot identity (zero before the first connect).
func (w *Wiring) Identity() bootid.Identity { return w.identity.Identity() }

// ImplementedDomains lists the implemented domains, sorted.
func ImplementedDomains() []string {
	out := make([]string, 0, len(Domains))
	for d := range Domains {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// ErrNoIdentity is returned by stores asked to record before the first VPP connect.
var ErrNoIdentity = errors.New("subsystems: VPP boot identity not known yet")

// String describes the wiring (start-up log).
func (w *Wiring) String() string {
	return fmt.Sprintf("stores in %s: claims-iface-%s.json, boot-%s.json; globals owner %v", w.env.StateDir, w.env.Owner, w.env.Owner, w.env.GlobalsOwner)
}
