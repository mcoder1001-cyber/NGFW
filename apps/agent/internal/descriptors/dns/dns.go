// Package dns holds the reconciler descriptors for VPP's caching DNS resolver plugin (task DF-8,
// WBS D7.3): the global enable switch and the upstream name servers, plus the resolve action
// helpers. Message names come only from apps/agent/binapi/dns; docs/agent/descriptors/dns.md is
// the object ↔ message table.
//
// The dns plugin of VPP 26.06 has no dump and no getter (dns.api: enable_disable,
// name_server_add_del, resolve_name, resolve_ip): both descriptors are write-only — Retrieve
// returns ErrRetrieveUnsupported (D-063) and every Create is idempotent.
//
// Ordering: VPP refuses dns_enable_disable(enable=1) with NO_NAME_SERVERS while no name server
// is configured, so dns.enable depends on nothing and dns.name-server is registered first (the
// scheduler breaks ties by registration order); the DF-8 prompt's "name-server → enable" arrow
// is reversed on purpose.
package dns

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/dns"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameEnable     = "dns.enable"
	NameNameServer = "dns.name-server"
)

// Enable is the dns.enable singleton: whether VPP's DNS resolver/proxy answers on UDP 53.
type Enable struct {
	Enabled bool `json:"enabled"`
}

// NameServer is one upstream name server (canonical IPv4 or IPv6 address).
type NameServer struct {
	Address string `json:"address"`
}

// Proto returns the canonical structpb document.
func (s Enable) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s NameServer) Proto() *structpb.Struct { return dfkit.Encode(s) }

// EnableID is the object id of the singleton (key dns.enable/global).
const EnableID = "global"

// KeyEnable is the key of the singleton.
var KeyEnable = scheduler.Join(NameEnable, EnableID)

// Option configures Register.
type Option func(*dfkit.Globals)

// WithGlobals sets the D-071 role (default: not the globals owner). The resolver and its name
// servers are VPP-global: only the globals owner sets them; any other agent's Create fails with
// ErrNotGlobalsOwner (VPP has no getter to check a requirement) and its Delete is a no-op.
func WithGlobals(g dfkit.Globals) Option { return func(o *dfkit.Globals) { *o = g } }

// Register constructs and registers the dns descriptors (name servers first, see package doc).
func Register(r scheduler.Registry, client vpp.Client, opts ...Option) {
	var g dfkit.Globals
	for _, o := range opts {
		o(&g)
	}
	r.Register(NewNameServer(client, g))
	r.Register(NewEnable(client, g))
}

// ---- dns.enable -----------------------------------------------------------------------------

// EnableDescriptor manages the dns.enable singleton. Enabled=false (or Delete) sends
// dns_enable_disable(enable=0).
type EnableDescriptor struct {
	client  vpp.Client
	globals dfkit.Globals
}

var _ scheduler.Descriptor = (*EnableDescriptor)(nil)

// NewEnable returns the dns.enable descriptor.
func NewEnable(client vpp.Client, g dfkit.Globals) *EnableDescriptor {
	return &EnableDescriptor{client: client, globals: g}
}

// Name implements scheduler.Descriptor.
func (*EnableDescriptor) Name() string { return NameEnable }

// KeyOf implements scheduler.Descriptor.
func (*EnableDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyEnable }

// Dependencies implements scheduler.Descriptor: none (see package doc on ordering).
func (*EnableDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *EnableDescriptor) set(ctx context.Context, on bool) error {
	var v uint8
	if on {
		v = 1
	}
	if _, err := dns.NewServiceClient(d.client).DNSEnableDisable(ctx, &dns.DNSEnableDisable{Enable: v}); err != nil {
		return fmt.Errorf("dns_enable_disable(enable=%d): %w", v, err)
	}
	return nil
}

func (d *EnableDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s Enable
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if !d.globals.Owner() {
		return d.globals.Require(ctx, NameEnable, obj, nil)
	}
	return d.set(ctx, s.Enabled)
}

// Create implements scheduler.Descriptor (idempotent).
func (d *EnableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor.
func (d *EnableDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: disables the resolver (globals owner only; a no-op
// for everyone else).
func (d *EnableDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if !d.globals.Owner() {
		return nil
	}
	return d.set(ctx, false)
}

// Retrieve implements scheduler.Descriptor: no getter in VPP (write-only, D-063).
func (*EnableDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameEnable)
}

// ---- dns.name-server ------------------------------------------------------------------------

// NameServerDescriptor manages dns.name-server objects: key dns.name-server/<address>.
type NameServerDescriptor struct {
	client  vpp.Client
	globals dfkit.Globals
}

var _ scheduler.Descriptor = (*NameServerDescriptor)(nil)

// NewNameServer returns the dns.name-server descriptor.
func NewNameServer(client vpp.Client, g dfkit.Globals) *NameServerDescriptor {
	return &NameServerDescriptor{client: client, globals: g}
}

// Name implements scheduler.Descriptor.
func (*NameServerDescriptor) Name() string { return NameNameServer }

// KeyOf implements scheduler.Descriptor.
func (*NameServerDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s NameServer
	if err := dfkit.Decode(obj, &s); err != nil {
		return scheduler.Join(NameNameServer, "invalid")
	}
	if a, err := dfkit.ParseAddr(s.Address); err == nil {
		return scheduler.Join(NameNameServer, a.String())
	}
	return scheduler.Join(NameNameServer, s.Address)
}

// Dependencies implements scheduler.Descriptor: none.
func (*NameServerDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *NameServerDescriptor) set(ctx context.Context, obj proto.Message, add bool) error {
	var s NameServer
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	a, err := dfkit.ParseAddr(s.Address)
	if err != nil {
		return err
	}
	if a.String() != s.Address || a.IsUnspecified() {
		return dfkit.Specf("name server %q must be a canonical, specified address", s.Address)
	}
	if !d.globals.Owner() {
		if !add {
			return nil
		}
		return d.globals.Require(ctx, NameNameServer, obj, nil)
	}
	req := &dns.DNSNameServerAddDel{ServerAddress: make([]byte, 16)}
	if a.Is6() {
		req.IsIP6 = 1
	}
	copy(req.ServerAddress, a.AsSlice())
	if add {
		req.IsAdd = 1
	}
	if _, err := dns.NewServiceClient(d.client).DNSNameServerAddDel(ctx, req); err != nil {
		return fmt.Errorf("dns_name_server_add_del(%s, add=%t): %w", a, add, err)
	}
	return nil
}

// Create implements scheduler.Descriptor (VPP ignores a duplicate add).
func (d *NameServerDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.set(ctx, obj, true)
}

// Update implements scheduler.Descriptor: the address is the key; never differs — re-apply.
func (d *NameServerDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.set(ctx, newObj, true)
}

// Delete implements scheduler.Descriptor; NAME_SERVER_NOT_FOUND counts as deleted.
func (d *NameServerDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	err := d.set(ctx, obj, false)
	if dfkit.IsVPPError(err, api.NAME_SERVER_NOT_FOUND) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: no dump in VPP (write-only, D-063).
func (*NameServerDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameNameServer)
}

// ---- action helpers (Actions API, never desired state) --------------------------------------

// MaxNameLen is the longest name dns_resolve_name carries (u8[256], NUL-terminated).
const MaxNameLen = 253

// ResolveName asks VPP's resolver for name (dns_resolve_name). The name is validated as a DNS
// name (letters, digits, "-", "_", "."); it is never passed to a shell. VPP replies only once the
// upstream answers or its retries give up, so ctx must carry a deadline (a host run without one
// blocked for minutes against unreachable upstreams).
func ResolveName(ctx context.Context, c vpp.Client, name string) (ip4, ip6 netip.Addr, err error) {
	if err := ValidateName(name); err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	buf := make([]byte, 256)
	copy(buf, name)
	rep, err := dns.NewServiceClient(c).DNSResolveName(ctx, &dns.DNSResolveName{Name: buf})
	if err != nil {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("dns_resolve_name(%s): %w", name, err)
	}
	if rep.IP4Set != 0 && len(rep.IP4Address) == 4 {
		ip4 = netip.AddrFrom4([4]byte(rep.IP4Address))
	}
	if rep.IP6Set != 0 && len(rep.IP6Address) == 16 {
		ip6 = netip.AddrFrom16([16]byte(rep.IP6Address))
	}
	return ip4, ip6, nil
}

// ResolveIP asks VPP's resolver for the PTR name of addr (dns_resolve_ip).
func ResolveIP(ctx context.Context, c vpp.Client, addr netip.Addr) (string, error) {
	req := &dns.DNSResolveIP{Address: make([]byte, 16)}
	addr = addr.Unmap()
	if addr.Is6() {
		req.IsIP6 = 1
	}
	copy(req.Address, addr.AsSlice())
	rep, err := dns.NewServiceClient(c).DNSResolveIP(ctx, req)
	if err != nil {
		return "", fmt.Errorf("dns_resolve_ip(%s): %w", addr, err)
	}
	name := string(rep.Name)
	if i := strings.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	return name, nil
}

// ValidateName checks a DNS name for the resolve action.
func ValidateName(name string) error {
	if name == "" || len(name) > MaxNameLen {
		return dfkit.Specf("dns name %q: length must be 1..%d", name, MaxNameLen)
	}
	for _, r := range name {
		ok := r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return dfkit.Specf("dns name %q: invalid character %q", name, r)
		}
	}
	return nil
}
