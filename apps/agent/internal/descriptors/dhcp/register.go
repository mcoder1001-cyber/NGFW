// Package dhcp holds the reconciler descriptors for VPP's DHCP plugin (task DF-8, WBS D7.2):
// the DHCPv4/v6 relay (proxy) per VRF and its VSS option, the DHCPv4 client on an interface,
// the DHCPv6 IA_NA and prefix-delegation clients, addresses derived from a delegated prefix and
// the client DUID. Message names and fields come only from apps/agent/binapi/{dhcp,
// dhcp6_ia_na_client_cp,dhcp6_pd_client_cp}. docs/agent/descriptors/dhcp.md is the object ↔
// message table.
//
// Desired values are *structpb.Struct documents built from the typed specs in spec.go (D-055
// stand-in until P03b adds the domain messages).
//
// Ownership on the shared VPP: interface-bound objects are owned through the interface tag
// "<owner>:…" (vpp.OwnerTag); relay objects through their rx VRF, which must be inside the
// VRF scope given to Register (tests: the slot's table range). The DHCPv6 client objects have
// no dump in VPP 26.06: their Retrieve returns ErrRetrieveUnsupported (write-only, D-063).
package dhcp

import (
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names (scheduler.Descriptor.Name); the first segment of every key.
const (
	NameProxy         = "dhcp.proxy"
	NameProxyVSS      = "dhcp.proxy-vss"
	NameClient        = "dhcp.client"
	NameDHCP6Client   = "dhcp.dhcp6-client"
	NameDHCP6PDClient = "dhcp.dhcp6-pd-client"
	NameDHCP6PDAddr   = "dhcp.dhcp6-pd-address"
	NameDHCP6DUID     = "dhcp.dhcp6-duid"
)

// Option configures the descriptors of this package.
type Option func(*options)

type options struct {
	ifaceKey dfkit.KeyFunc
	vrfKey   dfkit.KeyFunc
	vrfScope func(vrf uint32) bool
	globals  dfkit.Globals
}

func buildOptions(opts []Option) options {
	o := options{
		ifaceKey: dfkit.DefaultInterfaceKey,
		vrfKey:   dfkit.DefaultVRFKey,
		vrfScope: func(uint32) bool { return true },
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithInterfaceKey sets the interface key scheme of Dependencies (default "interface/<name>", D-065).
func WithInterfaceKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.ifaceKey = f
		}
	}
}

// WithVRFKey sets the VRF key scheme of Dependencies (default "vrf/<id>").
func WithVRFKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.vrfKey = f
		}
	}
}

// WithVRFScope limits the relay objects this agent owns to rx VRFs for which in returns true
// (default: every VRF — a production agent owns the whole VPP). Tests pass their slot's table
// range so two workers never see each other's relays.
func WithVRFScope(in func(vrf uint32) bool) Option {
	return func(o *options) {
		if in != nil {
			o.vrfScope = in
		}
	}
}

// WithGlobals sets the D-071 role for the VPP-global dhcp.dhcp6-duid (default: not the globals
// owner — the DUID is then only required, never set).
func WithGlobals(g dfkit.Globals) Option { return func(o *options) { o.globals = g } }

// Register constructs every descriptor of the dhcp plugin with the shared client and owner and
// registers them in dependency-friendly order. This is the one entry point the agent (P05) wires.
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...Option) {
	r.Register(NewProxy(client, opts...))
	r.Register(NewProxyVSS(client, opts...))
	r.Register(NewClient(client, owner, opts...))
	r.Register(NewDHCP6DUID(client, buildOptions(opts).globals))
	r.Register(NewDHCP6Client(client, owner, opts...))
	r.Register(NewDHCP6PDClient(client, owner, opts...))
	r.Register(NewDHCP6PDAddress(client, owner, opts...))
}

func (o options) vrfDeps(vrfs ...uint32) []scheduler.Dependency {
	var deps []scheduler.Dependency
	seen := map[uint32]bool{}
	for _, v := range vrfs {
		if v == 0 || seen[v] { // table 0 always exists
			continue
		}
		seen[v] = true
		deps = append(deps, scheduler.Dependency{Key: o.vrfKey(uitoa(v)), Optional: true})
	}
	return deps
}

func (o options) ifaceDep(name string) scheduler.Dependency {
	return scheduler.Dependency{Key: o.ifaceKey(name)}
}
