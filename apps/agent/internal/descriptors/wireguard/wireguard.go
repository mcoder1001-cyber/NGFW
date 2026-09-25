// Package wireguard holds the reconciler descriptors for VPP's WireGuard plugin (wireguard.api):
// interfaces, peers and the async-crypto switch, plus the peer up/down event source (events.go)
// that StreamEvents consumes. Desired-state types are in internal/descriptors/vpn/pb; the secret
// contract (private key and preshared key references) is in internal/descriptors/vpn. Object ↔
// message table: docs/agent/descriptors/wireguard.md.
//
// Ownership: interfaces carry the owner tag "<owner>:wg<instance>" (logical name = tag id = VPP's
// name, D-069); peers belong to the owner of the interface they are attached to. The async-mode
// singleton is a VPP-global (D-071): its setter is registered only for the globals owner
// (WithGlobalsOwner), everybody else registers a requirement that cannot be met (no getter).
package wireguard

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	InterfaceName = "wireguard.interface"
	PeerName      = "wireguard.peer"
	AsyncModeName = "wireguard.async-mode"
)

// Config is what every descriptor of the package is constructed with.
type Config struct {
	Client  vpp.Client
	Owner   string       // VRX_OWNER; tests pass their VRX_TEST_PREFIX
	Secrets vpn.Resolver // resolves private/preshared key references; nil = every reference fails
	// Keys computes the keyed secret fingerprints (D-096; vpn.LoadOrCreateKeyFile in the agent
	// state dir). Required: without it every secret reference fails with vpn.ErrNoKeyer.
	Keys *vpn.Keyer
	// GlobalsOwner registers the setter of the async-mode global (D-071).
	GlobalsOwner bool
}

// WithGlobalsOwner marks this agent as the globals owner (agent config globalsOwner: true).
func WithGlobalsOwner(on bool) Option { return func(c *Config) { c.GlobalsOwner = on } }

// Option tunes Register.
type Option func(*Config)

// WithKeyer sets the agent-local fingerprint key (D-096).
func WithKeyer(k *vpn.Keyer) Option { return func(c *Config) { c.Keys = k } }

// WithSecrets sets the secret resolver.
func WithSecrets(r vpn.Resolver) Option { return func(c *Config) { c.Secrets = r } }

// Register constructs every descriptor of the plugin and registers it (P05 wires this). It returns
// the peer descriptor, which is also the plugin's event source (Peer.Events).
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...Option) *Peer {
	cfg := Config{Client: c, Owner: owner}
	for _, o := range opts {
		o(&cfg)
	}
	itf, peer := NewInterface(cfg), NewPeer(cfg)
	for _, d := range []scheduler.Descriptor{itf, peer, vpn.Global(cfg.GlobalsOwner, NewAsyncMode(cfg), nil)} {
		r.Register(d)
	}
	return peer
}

// ItfName is the VPP name of the WireGuard interface with the given instance.
func ItfName(instance uint32) string { return "wg" + vpn.Uint(instance) }

// noInterface is ~0 as an interface index / "any" in dumps.
const noInterface = ^uint32(0)

func typeErr(name string, obj proto.Message) error {
	return fmt.Errorf("%s: unexpected desired type %T", name, obj)
}

func metaErr(name string, meta any) error {
	return fmt.Errorf("%s: unexpected meta %T", name, meta)
}

// sortKVs sorts by key and drops repeated keys (a Retrieve never reports one key twice).
func sortKVs(kvs []scheduler.KV) []scheduler.KV {
	sort.SliceStable(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
	return dfkit.Dedupe(kvs)
}
