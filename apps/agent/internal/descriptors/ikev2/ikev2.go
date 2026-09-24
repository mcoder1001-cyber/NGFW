// Package ikev2 holds the reconciler descriptors for VPP's native IKEv2 plugin (ikev2.api): the
// composite responder/initiator profile and its write-only responder hostname, the plugin-wide
// local key, sleep interval and liveness singletons, plus a read-only SA state helper (state.go)
// and thin action helpers (actions.go) that the VPN F-* tasks call. Desired-state types are in internal/descriptors/vpn/pb; the secret
// contract (PSK references) is in internal/descriptors/vpn. Object ↔ message table:
// docs/agent/descriptors/ikev2.md.
//
// Ownership: a profile is named "<owner>-<name>" in VPP (ikev2_profile_add_del.name, ≤ 63 bytes),
// so Retrieve keeps only profiles with the owner's prefix and ownership survives an agent restart.
// The local key, sleep interval and liveness are VPP-globals (D-071): only the globals owner
// (WithGlobalsOwner) registers their setters; every other agent registers requirements
// (vpn.Require). Interfaces are named by their logical names (D-069).
package ikev2

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	ProfileName       = "ikev2.profile"
	LocalKeyName      = "ikev2.local-key"
	SleepIntervalName = "ikev2.sleep-interval"
	LivenessName      = "ikev2.liveness"
	// ResponderHostnameName is the write-only responder-by-hostname part of a profile.
	ResponderHostnameName = "ikev2.responder-hostname"
)

// Config is what every descriptor of the package is constructed with.
type Config struct {
	Client  vpp.Client
	Owner   string       // VRX_OWNER; tests pass their VRX_TEST_PREFIX
	Secrets vpn.Resolver // resolves PSK references; nil = every PSK reference fails
	// Boot is the owner's persisted record store (D-076 applied-once records of the responder
	// hostname; vpn.Records). Register defaults to an in-memory store.
	Boot dfkit.BootStore
	// GlobalsOwner registers the setters of the local key / sleep interval / liveness globals.
	GlobalsOwner bool
}

func (c Config) records() vpn.Records { return vpn.Records{Client: c.Client, Store: c.Boot} }

// WithBootStore sets the owner's persisted record store (shared by every DF-5 package).
func WithBootStore(s dfkit.BootStore) Option { return func(c *Config) { c.Boot = s } }

// WithGlobalsOwner marks this agent as the globals owner (agent config globalsOwner: true).
func WithGlobalsOwner(on bool) Option { return func(c *Config) { c.GlobalsOwner = on } }

// Option tunes Register.
type Option func(*Config)

// WithSecrets sets the secret resolver.
func WithSecrets(r vpn.Resolver) Option { return func(c *Config) { c.Secrets = r } }

// Register constructs every descriptor of the plugin and registers it (P05 wires this).
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

// All returns the package's descriptors in registration order (singletons first: a profile with
// rsa-sig auth depends on the local key; the responder hostname depends on its profile). The
// globals are the owner's setters or the Require variants (D-071). cfg.Boot must be set.
func All(cfg Config) []scheduler.Descriptor {
	sleep := NewSleepInterval(cfg)
	return []scheduler.Descriptor{
		vpn.Global(cfg.GlobalsOwner, NewLocalKey(cfg), nil),
		vpn.Global(cfg.GlobalsOwner, sleep, sleep.current),
		vpn.Global(cfg.GlobalsOwner, NewLiveness(cfg), nil),
		NewProfile(cfg), NewResponderHostname(cfg),
	}
}

// ---- value tables ---------------------------------------------------------------------------
//
// ikev2.api carries transforms, id types and the auth method as plain u8 (no binapi enum), so
// there is no generated name table. The values below are the IANA IKEv2 numbers exactly as VPP
// 26.06 defines them in src/plugins/ikev2/ikev2.h (foreach_ikev2_transform_{encr,prf,integ,dh}_type,
// foreach_ikev2_auth_method, foreach_ikev2_id_type), names spelled as VPP's own CLI spells them.
// Message names still come only from binapi.

type table struct {
	kind   string
	names  map[uint8]string
	values map[string]uint8
}

func newTable(kind string, pairs map[uint8]string) table {
	t := table{kind: kind, names: pairs, values: map[string]uint8{}}
	for v, n := range pairs {
		t.values[n] = v
	}
	return t
}

func (t table) name(v uint8) string {
	if n, ok := t.names[v]; ok {
		return n
	}
	return fmt.Sprintf("unknown-%d", v)
}

func (t table) value(s string) (uint8, error) {
	v, ok := t.values[s]
	if !ok {
		all := make([]string, 0, len(t.values))
		for n := range t.values {
			all = append(all, n)
		}
		sort.Strings(all)
		return 0, fmt.Errorf("ikev2: unknown %s %q (want one of %s)", t.kind, s, strings.Join(all, ", "))
	}
	return v, nil
}

var (
	encrAlgs = newTable("encryption algorithm", map[uint8]string{
		1: "des-iv64", 2: "des", 3: "3des", 4: "rc5", 5: "idea", 6: "cast", 7: "blowfish", 8: "3idea",
		9: "des-iv32", 11: "null", 12: "aes-cbc", 13: "aes-ctr", 20: "aes-gcm-16", 28: "chacha20-poly1305",
	})
	prfAlgs = newTable("prf algorithm", map[uint8]string{
		1: "hmac-md5", 2: "hmac-sha1", 3: "mac-tiger", 4: "aes128-xcbc", 5: "hmac-sha2-256",
		6: "hmac-sha2-384", 7: "hmac-sha2-512", 8: "aes128-cmac",
	})
	integAlgs = newTable("integrity algorithm", map[uint8]string{
		0: "none", 1: "md5-96", 2: "sha1-96", 3: "des-mac", 4: "kpdk-md5", 5: "aes-xcbc-96", 6: "md5-128",
		7: "sha1-160", 8: "cmac-96", 9: "aes-128-gmac", 10: "aes-192-gmac", 11: "aes-256-gmac",
		12: "hmac-sha2-256-128", 13: "hmac-sha2-384-192", 14: "hmac-sha2-512-256",
	})
	dhGroups = newTable("dh group", map[uint8]string{
		0: "none", 1: "modp-768", 2: "modp-1024", 5: "modp-1536", 14: "modp-2048", 15: "modp-3072",
		16: "modp-4096", 17: "modp-6144", 18: "modp-8192", 19: "ecp-256", 20: "ecp-384", 21: "ecp-521",
		22: "modp-1024-160", 23: "modp-2048-224", 24: "modp-2048-256", 25: "ecp-192", 26: "ecp-224",
		27: "brainpool-224", 28: "brainpool-256", 29: "brainpool-384", 30: "brainpool-512",
	})
)

// Auth methods (foreach_ikev2_auth_method) and their desired-state names.
const (
	authRSASig    uint8 = 1
	authSharedKey uint8 = 2
	AuthPSK             = "psk"
	AuthRSASig          = "rsa-sig"
)

// ID types (foreach_ikev2_id_type) and their desired-state names. VPP 26.06 accepts only ip4, ip6,
// fqdn and rfc822 in ikev2_profile_set_id (ikev2_is_id_supported); "key-id" is rejected by Create.
const (
	idIP4    uint8 = 1
	idFQDN   uint8 = 2
	idRFC822 uint8 = 3
	idIP6    uint8 = 5
	idKeyID  uint8 = 11
	IDIP4          = "ip4"
	IDIP6          = "ip6"
	IDFQDN         = "fqdn"
	IDRFC822       = "rfc822"
	IDKeyID        = "key-id"
)

var idTypes = newTable("id type", map[uint8]string{
	idIP4: IDIP4, idFQDN: IDFQDN, idRFC822: IDRFC822, idIP6: IDIP6, idKeyID: IDKeyID,
})

// noInterface is ~0 as an interface index.
const noInterface = ^uint32(0)

// ipsecUDPPortNone is VPP's "no ipsec-over-udp port" (IPSEC_UDP_PORT_NONE, (u16)~0).
const ipsecUDPPortNone = 0xffff

func typeErr(name string, obj proto.Message) error {
	return fmt.Errorf("%s: unexpected desired type %T", name, obj)
}
