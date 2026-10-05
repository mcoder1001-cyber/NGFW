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
	ravpn "ngfw/agent/internal/ra_vpn"
	"path/filepath"
	"sort"
	"sync"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/descriptors/hoststack"
	"ngfw/agent/internal/descriptors/ikev2"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/lb"
	"ngfw/agent/internal/descriptors/lisp"
	"ngfw/agent/internal/descriptors/policer"
	"ngfw/agent/internal/descriptors/qos"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/renderers/kea"
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
	// wave-BC: F-lisp
	Tunnels = "tunnels"
	// wave-A: F-loopback-bvi-gso-lldp-span
	// wave-A: F-rpf-adl-pbr
	Services = "services"
	// wave-A: F-object-model
	Objects = "objects"
	// wave-A: F-acl
	ACL = "acl" // shared with F-host-acl-nftables (one key, both families)
	// wave-A: F-host-acl-nftables (shares the ACL key/const above)
	// wave-A: F-nat44-ed-sessions
	Nat = "nat"
	// wave-A: P11
	// wave-A: F-wireguard
	VPN = "vpn"
	// wave-A: F-kea-dhcp-relay (shares F-rpf-adl-pbr's Services key/const above)
	// wave-A: F-unbound-chrony-syslog (shares F-rpf-adl-pbr's Services key/const above)
	Management = "management"
)

// Domains maps each implemented configuration domain to the descriptors that realise it.
var Domains = map[string][]string{
	"security": {}, // defensive policy persisted by the agent; enforcement depends on ACL objects
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
		bondingBond,
		bondingMember,
		bondingWeight,
		// wave-A: F-bridge-l2
		bridgeL2Domain,
		bridgeL2Member,
		bridgeL2Xconnect,
		bridgeL2FibEntry,
		bridgeL2Flags,
		bridgeL2TagRewrite,
		bridgeL2L3xc,
		bridgeL2MacRange,
		bridgeL2MacEnable,
		// wave-A: F-loopback-bvi-gso-lldp-span
		loopbackGso,
		loopbackSpanMirror,
		// wave-A: F-neighbors-ra
		neighborsRaRaConfig,
		neighborsRaRaPrefix,
		neighborsRaProxyNd,
		neighborsRaProxyArpIf,
		// wave-A: F-rpf-adl-pbr
		rpfAdlPbrURPF,
		rpfAdlPbrADL,
		rpfAdlPbrADLAllow,
		// wave-A: P12
		lcpItfPairName,
	},
	VRFs: {
		core.VRFName,
		// wave-A: F-vrf-static-ecmp
		svsTableName,
		svsInterfaceName,
		svsRouteName,
		// wave-A: F-neighbors-ra
		neighborsRaProxyRange,
	},
	Routing: {
		core.RouteName,
		// wave-BC: F-bfd-redistribution
		// wave-BC: F-mpls-srmpls
		mplsTableName,
		mplsInterfaceName,
		mplsRouteName,
		mplsIPBindName,
		mplsTunnelName,
		srMplsPolicyName,
		srMplsSteeringName,
		// wave-BC: F-igmp-mfib
		// wave-BC: F-srv6
		srLocalSidName,      // F-srv6: sr.localsid (srv6.go)
		srPolicyName,        // F-srv6: sr.policy
		srSteeringName,      // F-srv6: sr.steering
		srEncapSourceName,   // F-srv6: sr.encap-source (VPP global, globals owner only)
		srEncapHopLimitName, // F-srv6: sr.encap-hop-limit (VPP global, globals owner only)
		// wave-A: F-vrf-static-ecmp
		// wave-A: F-neighbors-ra
		neighborsRaNeighbor,
		neighborsRaConfig,
		neighborsRaDad,
		// wave-A: F-rpf-adl-pbr
		rpfAdlPbrABFPolicy,
		rpfAdlPbrABFAttach,
		rpfAdlPbrPolicyName,
		// wave-A: P12
		frrConfigName,
		"lcp.osi-proto", // F-isis-rip globals-only descriptor
	},
	// New domain entries: one `<Const>: {…}` entry under the feature's anchor (wave-A-hotspots A1).
	// wave-BC: F-det44-map-dslite-cnat
	// wave-BC: F-tunnels (gre/ipip/vxlan tunnels + tunnels.meta are appended to F-lisp's Tunnels entry by tunnels.go init)
	// wave-BC: F-vrrp-config-sync (ha: vrrp.* + vrrp.meta + keepalived.config are added by vrrp.go init)
	// wave-BC: F-pki
	// wave-BC: F-ikev2-native
	// wave-BC: F-lb (services: lb.* are in the one services entry below)
	// wave-BC: F-qos-flat
	// wave-BC: F-host-stack
	// wave-BC: F-snmp
	// wave-BC: F-ipfix-sflow
	"services": append(append(append(append([]string{}, ipfixSflowDescriptors...), // other services families: extend ipfixSflowDescriptors' slice here
		hoststack.NameSession, hoststack.NameNamespace, hoststack.NameSessionRule, hoststack.NameTCPSrc, hoststack.NameHTTPStatic, // F-host-stack
		desired.SnmpDescriptorName,                 // F-snmp
		policer.NamePolicer, policer.NameInterface, // F-qos-flat
		qos.NameEgressMap, qos.NameRecord, qos.NameStore, qos.NameMark, qos.NameMeta, // F-qos-flat
		kea.NameDhcp4, kea.NameDhcp6, dhcp.NameProxy, dhcp.NameProxyVSS, dhcp.NameRelay), // F-kea-dhcp-relay
		servicesDescriptors...), // F-unbound-chrony-syslog (unbound, chrony, dns.*: unbound.go)
		lb.NameConf, lb.NameVIP, lb.NameAS, lb.NameIntfNat), // F-lb
	// wave-BC: F-lisp
	Tunnels: {
		lisp.EnableName, lisp.GpeEnableName, lisp.LocatorSetName, lisp.LocatorName, lisp.LocalEidName,
		lisp.MapResolverName, lisp.MapServerName, lisp.RemoteMappingName, lisp.AdjacencyName,
		lisp.EidTableMapName, lisp.PitrName, lisp.GpeFwdEntryName,
	},
	// wave-BC: F-dashboard-prom-alarms
	// wave-A: F-loopback-bvi-gso-lldp-span
	// (services: lldp.* / nsim.* are appended to F-rpf-adl-pbr's Services entry by loopback_bvi_gso_lldp_span.go init)
	// wave-A: F-rpf-adl-pbr (services: autosdl is appended to the services entry by rpf_adl_pbr.go init)
	// wave-A: F-object-model
	Objects: objectModelDescriptors(), // agent-local objects.* family (object_model.go)
	// wave-A: F-acl + F-host-acl-nftables (one ACL entry, both families)
	ACL: append(append([]string{}, aclDescriptors()...), hostACLDescriptors()...), // DF-4 acl plugin (acl.go) + host-acl.nftables (host_acl.go)
	// wave-A: F-nat44-ed-sessions
	Nat: natDomain(
		nat44EDDescriptors,
		// wave-A: F-nat44-ei-64-66-nptv6
		nat44EI6466NptDescriptors,
		// wave-BC: F-det44-map-dslite-cnat
		det44MapDsliteCnatDescriptors,
	),
	// wave-A: P11
	// wave-A: F-wireguard
	VPN: wireguardDescriptors(), // wireguard.interface, wireguard.peer, wireguard.meta (wireguard.go); P11 / F-ikev2-native append
	// wave-A: F-kea-dhcp-relay (services: kea.dhcp4/6, dhcp.proxy/proxy-vss/relay are in the one services entry above)
	// wave-A: F-unbound-chrony-syslog (services: unbound, chrony, dns.* are in the one services entry above)
	Management: append(append([]string{}, managementDescriptors...), desired.PrometheusDescriptorName),
	// F-system-identity (unanchored: no anchor seeded for this row)
	System: systemDescriptors, // system.identity (system_identity.go)
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
	// RA is a trusted construction seam for disposable private fixtures; production leaves it nil.
	RA     *RAControllerOptions
	Client vpp.Client
	Owner  string
	// StateDir holds the persisted stores (the agent's NGFW_AGENT_STATE_DIR).
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
	Publish func(*ngfwv1.Event)
	// Resync asks the agent for a full resync of its stored desired state (A5 seam, F-acl);
	// Wiring.RequestResync calls it. nil = no-op (the default).
	Resync func()
	// Exclusive runs fn while no transaction runs (the agent's transaction lock; F-lb review M2): background work
	// that reads VPP state and then acts on it (lb garbage collection) must not interleave with a transaction.
	// nil = fn runs directly (tests, the wiring before the service exists).
	Exclusive func(ctx context.Context, fn func(context.Context) error) error
	// IDs is this agent's VPP numeric id range (TD-8; the agent resolves it once with ResolveIDScope).
	// Families read it through Wiring.IDRange, which fails closed: the zero value owns no id.
	IDs IDScope
}

// Wiring is the result of Register: the stores and the hooks the agent calls.
type Wiring struct {
	env        Env
	identity   *Identity
	index      *IndexCache
	ifaceClaim *IfaceClaims
	boot       *dfkit.FileBootStore
	dhcpClient *dhcp.ClientDescriptor
	raStartup  *raStartupInitialization
	raRepair   ravpn.NamespaceHandoffStoppedRepair

	storesMu sync.Mutex
	keyed    map[string]*KeyedClaims
	classify *classify.FileStore
	vpnKeys  *vpn.Keyer

	seams seamRegistry // TD-8: dynamic desired sources and metrics collectors (seams.go)
}

// register is Register without the persistence guard (stores.go, TD-11b).
func register(r scheduler.Registry, env Env) (*Wiring, error) {
	if RARuntimeFor(env.Owner) != nil {
		return nil, errors.New("remote-access owner already registered")
	}
	r = &raFilteringRegistry{Registry: r, byName: map[string]scheduler.Descriptor{}}
	if env.Log == nil {
		env.Log = slog.Default()
	}
	if env.NetdevKind == nil {
		env.NetdevKind = LinuxNetdevKind
	}
	w := &Wiring{env: env, identity: &Identity{}, keyed: map[string]*KeyedClaims{}}
	w.index = NewIndexCache(time.Second, func(ctx context.Context) (map[string]uint32, error) { // the caller's deadline (R2-stores)
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
	// D-065/D-073a; TD-11c: addresses and VRF bindings on untagged NICs through the persisted claims (D-071/D-080)
	core.Register(r, core.Env{Client: c, Owner: owner, Owned: env.Owned, IfRef: core.AliasInterfaceRef, Claims: w.ifaceClaim})
	r.Register(&vethOnly{Descriptor: afpacket.New(c, owner), kind: env.NetdevKind}) // D-105: veth only
	// DF-1, in iface.Register's order, with the MTU/rx-mode "value equal to the default" tolerance
	r.Register(iface.NewSubinterface(c, owner))
	r.Register(&nativeAdminGuard{Descriptor: iface.NewAdminState(c, owner), owner: owner})
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
	// wave-BC: F-det44-map-dslite-cnat
	if err := w.registerDet44MapDsliteCnat(r); err != nil { // det44/dslite/map/cnat (det44_map_dslite_cnat.go)
		return nil, err
	}
	// wave-BC: F-tunnels
	if err := registerTunnels(r, w); err != nil { // DF-6 gre/ipip/vxlan tunnels + tunnels.meta (tunnels.go)
		return nil, err
	}
	registerTunnelsT1(r, w) // S-tunnels-contract: additional tunnel descriptors
	// wave-BC: F-vrrp-config-sync
	if err := registerVrrp(r, w); err != nil { // DF-7 vrrp family + vrrp.meta + keepalived stage (vrrp.go, keepalived.go)
		return nil, err
	}
	// wave-BC: F-pki
	if err := w.registerPKI(r); err != nil {
		return nil, err
	}
	if err := w.registerRATransport(r); err != nil {
		return nil, err
	}
	// wave-BC: F-ikev2-native
	if err := w.registerIKEv2(r); err != nil {
		return nil, err
	}
	// wave-BC: F-ospf
	// wave-BC: F-isis-rip
	registerIsisOSI(r, w)
	// wave-BC: F-mpls-srmpls
	if err := registerMplsSrmpls(r, w); err != nil {
		return nil, err
	}
	// wave-BC: F-srv6
	if err := w.registerSrv6(r); err != nil { // DF-6 sr family, df6 claims in PairClaims("df6") (srv6.go)
		return nil, err
	}
	// wave-BC: F-lisp
	if err := registerLisp(r, w); err != nil {
		return nil, err
	}
	// wave-BC: F-bfd-redistribution
	if err := w.registerBfd(r); err != nil {
		return nil, err
	}
	// wave-BC: F-mpls-ldp
	// wave-BC: F-igmp-mfib
	if err := w.registerIgmpMfib(r); err != nil {
		return nil, err
	}
	// wave-BC: F-ha-state-sync
	registerHaSync(r, w)
	registerSnmp(r, w) // F-snmp (unanchored: no wave-BC: F-snmp anchor in register())
	// wave-A: F-bonding
	if err := w.registerBonding(r); err != nil {
		return nil, err
	}
	// wave-A: F-bridge-l2
	w.registerBridgeL2(r)
	// wave-A: F-loopback-bvi-gso-lldp-span
	w.registerLoopbackBviGsoLldpSpan(r)
	// wave-A: F-vrf-static-ecmp
	registerVrfStaticEcmp(r, w)
	// wave-A: F-neighbors-ra
	if err := w.registerNeighborsRa(r); err != nil {
		return nil, err
	}
	// wave-A: F-rpf-adl-pbr
	if err := w.registerRpfAdlPbr(r); err != nil {
		return nil, err
	}
	// wave-A: F-object-model
	if err := w.registerObjectModel(r); err != nil { // objects.* store + FQDN resolver (object_model.go)
		return nil, err
	}
	// wave-A: F-acl
	if err := w.registerACL(r); err != nil { // acl.Register's six descriptors with KeyedClaims("acl") (acl.go)
		return nil, err
	}
	// wave-A: F-host-acl-nftables
	if err := w.registerHostACL(r); err != nil { // host firewall: nftables renderer as one descriptor (host_acl.go)
		return nil, err
	}
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
	if err := w.registerWireguard(r); err != nil { // DF-5 wireguard family + wireguard.meta + secret store (wireguard.go)
		return nil, err
	}
	// wave-A: P12
	registerP12(r, w)
	if err := registerPim(r, w); err != nil {
		return nil, err
	}
	// wave-BC: F-mpls-ldp
	if err := registerMplsLdp(r, w); err != nil {
		return nil, err
	}
	if err := registerBasePolicy(r, w); err != nil {
		return nil, err
	}
	// wave-A: F-kea-dhcp-relay
	if err := w.registerKea(r); err != nil {
		return nil, err
	}
	// wave-A: F-unbound-chrony-syslog
	if err := registerUnboundChronySyslog(r, env); err != nil {
		return nil, err
	}
	hoststack.Register(r, c, owner, hoststack.WithBootStore(w.boot), hoststack.WithGlobalsOwner(env.GlobalsOwner)) // F-host-stack (unanchored)
	if env.GlobalsOwner {
		hoststack.RegisterGlobals(r, c, hoststack.WithBootStore(w.boot)) // F-host-stack: D-071 session layer, opt-in http_static
	}
	// wave-BC: F-ipfix-sflow (unanchored)
	if err := registerIpfixSflow(r, w); err != nil {
		return nil, err
	}
	// F-qos-flat (unanchored: no `wave-BC: F-qos-flat` anchor in this block)
	if err := w.registerQoS(r); err != nil {
		return nil, err
	}
	// wave-BC: F-dashboard-prom-alarms (registration seam)
	if err := registerDashboardPromAlarms(r, w); err != nil {
		return nil, err
	}
	w.registerLb(r)
	w.registerRuleExpiry()         // F-rule-expiry (unanchored)
	registerSystemIdentity(r, env) // F-system-identity (unanchored)
	w.registerPppoe()
	if err := w.registerPppoeClient(r); err != nil {
		return nil, err
	}
	if err := w.registerRAController(r); err != nil {
		return nil, err
	}
	return w, nil
}

// AfterResync runs after every full resync — the initial reconcile and the one on each VPP
// (re)connect: it releases this owner's quarantine holders whose parked sw_if_index is clean again
// (TD-3 Q2, ifsanitize.Release), so the index is free for the next creator. A holder that is still
// unclearable stays (admin down, owns the dirty index).
func (w *Wiring) AfterResync(ctx context.Context) {
	w.bfdAfterResync(ctx)      // wave-BC: F-bfd-redistribution
	w.igmpMfibAfterResync(ctx) // wave-BC: F-igmp-mfib-host
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
	sourceErr := w.initializeRASource(ctx)
	if err := w.StopRA(ctx); err != nil {
		w.env.Log.Error("remote-access reconnect cleanup refused", "reason", "owned generation could not be stopped")
		return
	}
	targetsErr := sourceErr
	if sourceErr == nil {
		targetsErr = w.initializeRATargets(ctx)
	}
	if targetsErr != nil {
		w.env.Log.Warn("remote-access initialization unavailable", "reason", "engine-not-ready")
	}
	repair := w.raRepair
	if targetsErr != nil {
		repair = nil
	}
	if runtime := RARuntimeFor(w.env.Owner); runtime != nil {
		if w.raStartup != nil {
			runtime.SetInitializationReady(false)
		}
		if runtime.RepairStoppedExports(ctx, repair) != nil {
			runtime.SetInitializationReady(false)
			w.env.Log.Error("remote-access stopped export repair unavailable", "reason", "engine-not-ready")
			return
		}
		if w.raStartup != nil && targetsErr == nil {
			runtime.SetInitializationReady(true)
		}
	}
	w.classifySentinelConnected(ctx) // globals owner establishes table 0 before other reconnect work
	w.bfdConnected()                 // wave-BC: F-bfd-redistribution
	w.igmpMfibConnected()            // wave-BC: F-igmp-mfib-host
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
	// F-neighbors-ra (W-seed seeded no anchor in Connected, questions Q2): restart the neighbour-event watcher
	w.neighborsRaConnected(ctx)
	w.wireguardConnected(ctx) // F-wireguard: (re)start the peer-event watcher (wireguard.go; no anchor here, questions Q4)
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

// IfaceClaims returns the persisted DF-1 claim store (claims on untagged interfaces, bound to their
// sw_if_index). Not for df6.WithClaims: df6 keyed claim ids are not interface names — pass
// PairClaims("df6") (TD-11b; the agent refuses to start otherwise).
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
