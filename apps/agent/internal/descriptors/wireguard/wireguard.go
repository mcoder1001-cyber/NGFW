// Package wireguard holds the reconciler descriptors for VPP's WireGuard plugin (wireguard.api):
// interfaces, peers and the async-crypto switch, plus the peer up/down event source (events.go)
// that StreamEvents consumes. Desired-state types are in internal/descriptors/vpn/pb; the secret
// contract (private key and preshared key references) is in internal/descriptors/vpn. Object ↔
// message table: docs/agent/descriptors/wireguard.md.
//
// Ownership: interfaces carry the owner tag "<owner>:wg<instance>"; peers belong to the owner of
// the interface they are attached to. The async-mode singleton is unowned.
package wireguard

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	InterfaceName = vpn.WireguardItfDescriptor // "wireguard.interface"
	PeerName      = "wireguard.peer"
	AsyncModeName = "wireguard.async-mode"
)

// Config is what every descriptor of the package is constructed with.
type Config struct {
	Client  vpp.Client
	Owner   string       // VRX_OWNER; tests pass their VRX_TEST_PREFIX
	Secrets vpn.Resolver // resolves private/preshared key references; nil = every reference fails
}

// Option tunes Register.
type Option func(*Config)

// WithSecrets sets the secret resolver.
func WithSecrets(r vpn.Resolver) Option { return func(c *Config) { c.Secrets = r } }

// Register constructs every descriptor of the plugin and registers it (P05 wires this). It returns
// the peer descriptor, which is also the plugin's event source (Peer.Events).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...Option) *Peer {
	cfg := Config{Client: c, Owner: owner}
	for _, o := range opts {
		o(&cfg)
	}
	itf, peer, async := NewInterface(cfg), NewPeer(cfg), NewAsyncMode(cfg)
	for _, d := range []scheduler.Descriptor{itf, peer, async} {
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

func sortKVs(kvs []scheduler.KV) {
	for i := 1; i < len(kvs); i++ {
		for j := i; j > 0 && kvs[j-1].Key > kvs[j].Key; j-- {
			kvs[j-1], kvs[j] = kvs[j], kvs[j-1]
		}
	}
}
