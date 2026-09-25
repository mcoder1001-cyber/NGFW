// Package ipsec holds the reconciler descriptors for VPP's IPsec data plane (ipsec.api): SPDs,
// SPD↔interface bindings, SPD policies, SAs, tunnel protection, ipsec interfaces, the crypto
// backend selection and the async-crypto switch. Desired-state types are in
// internal/descriptors/vpn/pb; the secret contract is in internal/descriptors/vpn (doc.go).
// Object ↔ message table: docs/agent/descriptors/ipsec.md.
//
// Ownership on a shared VPP (D-071): ipsec/tunnel interfaces by owner tag; SPDs and SAs (which
// carry only a numeric id that VPP reuses across restarts) and SPD bindings on untagged interfaces
// by ownership records written after our own successful add and bound to the VPP boot identity
// (vpn.Records, D-080); SPD policies through their SPD; protections through the tag of the tunnel
// interface. Config.IDs additionally confines SPD/SA ids to the slot's range on the shared host.
// The backend and async-mode singletons are VPP-globals: set only by the globals owner
// (WithGlobalsOwner), required (checked, never set) by everybody else.
package ipsec

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/binapi/tunnel_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	SpdName           = "ipsec.spd"
	SpdInterfaceName  = "ipsec.spd-interface"
	SpdEntryName      = "ipsec.spd-entry"
	SaName            = "ipsec.sa"
	TunnelProtectName = "ipsec.tunnel-protect"
	ItfName           = "ipsec.itf"
	BackendName       = "ipsec.backend"
	AsyncModeName     = "ipsec.async-mode"
)

// Config is what every descriptor of the package is constructed with.
type Config struct {
	Client  vpp.Client
	Owner   string       // VRX_OWNER; tests pass their VRX_TEST_PREFIX
	Secrets vpn.Resolver // resolves SA key references; nil = every secret reference fails
	IDs     vpn.IDRange  // owned SPD/SA ids; zero value = all
	// Keys computes the keyed secret fingerprints (D-096; vpn.LoadOrCreateKeyFile in the agent
	// state dir). Required: without it every secret reference fails with vpn.ErrNoKeyer.
	Keys *vpn.Keyer
	// Boot is the owner's persisted record store (ownership records of SPDs, SAs and SPD
	// bindings; vpn.Records). Register defaults to an in-memory store.
	Boot dfkit.BootStore
	// GlobalsOwner registers the setters of the backend / async-mode globals (D-071).
	GlobalsOwner bool
}

func (c Config) records() vpn.Records { return vpn.Records{Client: c.Client, Store: c.Boot} }

// Option tunes Register.
type Option func(*Config)

// WithKeyer sets the agent-local fingerprint key (D-096).
func WithKeyer(k *vpn.Keyer) Option { return func(c *Config) { c.Keys = k } }

// WithSecrets sets the secret resolver.
func WithSecrets(r vpn.Resolver) Option { return func(c *Config) { c.Secrets = r } }

// WithIDRange restricts ownership of SPD/SA ids to lo..hi.
func WithIDRange(lo, hi uint32) Option {
	return func(c *Config) { c.IDs = vpn.IDRange{Lo: lo, Hi: hi} }
}

// WithBootStore sets the owner's persisted record store (P05/P08: dfkit.NewFileBootStore in the
// agent state dir, shared by every DF-5 package of the owner).
func WithBootStore(s dfkit.BootStore) Option { return func(c *Config) { c.Boot = s } }

// WithGlobalsOwner marks this agent as the globals owner (agent config globalsOwner: true).
func WithGlobalsOwner(on bool) Option { return func(c *Config) { c.GlobalsOwner = on } }

// Register constructs every descriptor of the plugin and registers it (P05 wires this).
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...Option) {
	cfg := Config{Client: c, Owner: owner}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.Boot == nil {
		cfg.Boot = dfkit.NewMemoryBootStore()
	}
	for _, d := range All(cfg) {
		r.Register(d)
	}
}

// All returns the package's descriptors in registration order; the globals are the owner's
// setters or the Require variants (D-071). cfg.Boot must be set.
func All(cfg Config) []scheduler.Descriptor {
	backend := NewBackend(cfg)
	return []scheduler.Descriptor{
		NewSpd(cfg), NewSpdInterface(cfg), NewSpdEntry(cfg), NewSa(cfg),
		NewTunnelProtect(cfg), NewItf(cfg),
		vpn.Global(cfg.GlobalsOwner, backend, backend.current),
		vpn.Global(cfg.GlobalsOwner, NewAsyncMode(cfg), nil),
	}
}

// ---- enum name tables derived from the generated bindings (never typed by hand) ------------

type enum[T ~uint8 | ~uint32] struct {
	names  map[T]string
	values map[string]T
}

// newEnum maps "IPSEC_API_CRYPTO_ALG_AES_GCM_128" ↔ "aes-gcm-128" using the binapi *_name table.
func newEnum[T ~uint8 | ~uint32, K ~uint8 | ~uint32](table map[K]string, prefix string) enum[T] {
	e := enum[T]{names: map[T]string{}, values: map[string]T{}}
	for v, n := range table {
		s := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(n, prefix), "_", "-"))
		e.names[T(v)] = s
		e.values[s] = T(v)
	}
	return e
}

func (e enum[T]) name(v T) string { return e.names[v] }

func (e enum[T]) value(kind, s string) (T, error) {
	v, ok := e.values[s]
	if !ok {
		return 0, fmt.Errorf("ipsec: unknown %s %q (want one of %s)", kind, s, strings.Join(e.sorted(), ", "))
	}
	return v, nil
}

func (e enum[T]) sorted() []string {
	out := make([]string, 0, len(e.values))
	for s := range e.values {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

var (
	cryptoAlgs = newEnum[ipsec_types.IpsecCryptoAlg](ipsec_types.IpsecCryptoAlg_name, "IPSEC_API_CRYPTO_ALG_")
	integAlgs  = newEnum[ipsec_types.IpsecIntegAlg](ipsec_types.IpsecIntegAlg_name, "IPSEC_API_INTEG_ALG_")
	protocols  = newEnum[ipsec_types.IpsecProto](ipsec_types.IpsecProto_name, "IPSEC_API_PROTO_")
	actions    = newEnum[ipsec_types.IpsecSpdAction](ipsec_types.IpsecSpdAction_name, "IPSEC_API_SPD_ACTION_")
	encapFlags = newEnum[tunnel_types.TunnelEncapDecapFlags](tunnel_types.TunnelEncapDecapFlags_name, "TUNNEL_API_ENCAP_DECAP_FLAG_")
)

// tunnel modes: the task vocabulary is p2p / p2mp (VPP: TUNNEL_API_MODE_P2P / _MP).
var tunnelModes = map[string]tunnel_types.TunnelMode{
	"p2p":  tunnel_types.TUNNEL_API_MODE_P2P,
	"p2mp": tunnel_types.TUNNEL_API_MODE_MP,
}

func tunnelModeName(m tunnel_types.TunnelMode) string {
	for s, v := range tunnelModes {
		if v == m {
			return s
		}
	}
	return ""
}

// encapFlagNames decodes a flag set into sorted names; encodeEncapFlags is the inverse.
func encapFlagNames(f tunnel_types.TunnelEncapDecapFlags) []string {
	var out []string
	for i := range 8 {
		if bit := tunnel_types.TunnelEncapDecapFlags(1) << i; f&bit != 0 {
			out = append(out, encapFlags.name(bit))
		}
	}
	sort.Strings(out)
	return out
}

func encodeEncapFlags(names []string) (tunnel_types.TunnelEncapDecapFlags, error) {
	var f tunnel_types.TunnelEncapDecapFlags
	for _, n := range names {
		v, err := encapFlags.value("encap/decap flag", n)
		if err != nil {
			return 0, err
		}
		f |= v
	}
	return f, nil
}

// noInterface is ~0 as an interface index.
const noInterface = ^uint32(0)

func typeErr(name string, obj proto.Message) error {
	return fmt.Errorf("%s: unexpected desired type %T", name, obj)
}

func metaErr(name string, meta any) error {
	return fmt.Errorf("%s: unexpected meta %T", name, meta)
}
