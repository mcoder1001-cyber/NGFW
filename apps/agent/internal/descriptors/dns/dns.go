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
// is configured, and crashes on a DNS request while enabled without one (V-item of
// F-unbound-chrony-syslog). dns.enable depends on nothing and dns.name-server is registered first
// (the scheduler breaks ties by registration order), so servers are created before the switch; a
// server delete disables the switch first (NameServerDescriptor.Delete), and the switch carries the
// upstream set (Enable.Upstreams), so the same transaction re-enables it after the new servers exist.
// The DF-8 prompt's "name-server → enable" arrow is reversed on purpose.
package dns

import (
	"context"
	"errors"
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

// Enable is the dns.enable singleton: whether VPP's DNS resolver/proxy answers on UDP 53. Upstreams (canonical
// addresses) are the name servers the enabled resolver uses. They are not scheduler dependencies (see Dependencies):
// carrying them in the Value turns a changed set into an update that re-enables the switch after the new servers were
// created (a server delete disables it first, NameServerDescriptor.Delete). The globals owner enables only after an
// IPv4 server was added on the running VPP (Readiness, D-137).
type Enable struct {
	Enabled   bool     `json:"enabled"`
	Upstreams []string `json:"upstreams,omitempty"`
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

// RegisterGlobals registers this package's VPP-global singleton descriptors, constructed as the
// globals owner (D-071). Call it only in the designated globals owner's agent (config
// globalsOwner: true — never a test slot on the shared host), name servers first (see package doc).
func RegisterGlobals(r scheduler.Registry, client vpp.Client) {
	RegisterGlobalsReady(r, client)
}

// RegisterGlobalsReady is RegisterGlobals returning the D-137 readiness fact both descriptors maintain (the dns_lookup
// action asks it before anything reaches VPP).
func RegisterGlobalsReady(r scheduler.Registry, client vpp.Client) *Readiness {
	g := dfkit.GlobalsOwner(true)
	ready := NewReadiness(client)
	r.Register(NewNameServer(client, g).WithReadiness(ready))
	r.Register(NewEnable(client, g).WithReadiness(ready))
	return ready
}

// ---- dns.enable -----------------------------------------------------------------------------

// EnableDescriptor manages the dns.enable singleton. Enabled=false (or Delete) sends
// dns_enable_disable(enable=0).
type EnableDescriptor struct {
	client  vpp.Client
	globals dfkit.Globals
	ready   *Readiness
}

// WithReadiness makes the descriptor maintain (and consult) the D-137 readiness fact.
func (d *EnableDescriptor) WithReadiness(r *Readiness) *EnableDescriptor {
	d.ready = r
	return d
}

// ErrNoIPv4Upstream refuses an enable without an IPv4 name server: VPP 26.06 dereferences a NULL IPv4 server vector on
// every request otherwise (D-137, docs/vpp-code-track.md). Nothing is sent.
var ErrNoIPv4Upstream = errors.New("dns: VPP's DNS cache needs at least one IPv4 upstream added on the running VPP before it is enabled (VPP 26.06 defect, D-137)")

var _ scheduler.Descriptor = (*EnableDescriptor)(nil)

// NewEnable returns the dns.enable descriptor.
func NewEnable(client vpp.Client, g dfkit.Globals) *EnableDescriptor {
	return &EnableDescriptor{client: client, globals: g}
}

// Name implements scheduler.Descriptor.
func (*EnableDescriptor) Name() string { return NameEnable }

// KeyOf implements scheduler.Descriptor.
func (*EnableDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyEnable }

// Dependencies implements scheduler.Descriptor: none. A dependency on the name servers would make the scheduler
// delete-and-recreate the switch around every server delete, re-enabling it before the replacement server exists
// (VPP: NO_NAME_SERVERS). Order instead: name servers are registered first (the tie breaker creates them before the
// switch), a server delete disables the switch first (NameServerDescriptor.Delete), and the switch carries the
// upstream set, so a changed set updates — re-enables — it after the new servers were created.
func (*EnableDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *EnableDescriptor) set(ctx context.Context, on bool) error {
	var v uint8
	if on {
		v = 1
	}
	if _, err := dns.NewServiceClient(d.client).DNSEnableDisable(ctx, &dns.DNSEnableDisable{Enable: v}); err != nil {
		return fmt.Errorf("dns_enable_disable(enable=%d): %w", v, err)
	}
	d.ready.setEnabled(ctx, on)
	return nil
}

// hasIPv4 is the enable precondition: an IPv4 server added on the running VPP (the readiness fact), or — for a
// descriptor built without one — an IPv4 address among the Value's upstreams.
func (d *EnableDescriptor) hasIPv4(ctx context.Context, s Enable) bool {
	if d.ready != nil {
		return d.ready.hasIPv4(ctx)
	}
	for _, u := range s.Upstreams {
		if a, err := netip.ParseAddr(u); err == nil && a.Unmap().Is4() {
			return true
		}
	}
	return false
}

func (d *EnableDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s Enable
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if !d.globals.Owner() {
		return d.globals.Require(ctx, NameEnable, obj, nil)
	}
	if s.Enabled && !d.hasIPv4(ctx, s) {
		return ErrNoIPv4Upstream
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
	ready   *Readiness
}

// WithReadiness makes the descriptor maintain the D-137 readiness fact.
func (d *NameServerDescriptor) WithReadiness(r *Readiness) *NameServerDescriptor {
	d.ready = r
	return d
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
	if add {
		d.ready.serverAdded(ctx, a)
	} else {
		d.ready.serverRemoved(a)
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

// Delete implements scheduler.Descriptor; NAME_SERVER_NOT_FOUND counts as deleted. The globals owner disables the
// resolver first: VPP 26.06 dereferences a NULL name server (ip4_sas from vnet_send_dns4_request) when a DNS request —
// API or UDP 53 packet — arrives while it is enabled without a server (2026-09-25 04:27 crash, docs/vpp-code-track.md),
// and the scheduler runs deletes before creates, so replacing the last server would open exactly that window. The
// dns.enable object carries the upstream set, so the same transaction re-enables it after the new servers exist.
func (d *NameServerDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	if d.globals.Owner() {
		if _, err := dns.NewServiceClient(d.client).DNSEnableDisable(ctx, &dns.DNSEnableDisable{Enable: 0}); err != nil {
			return fmt.Errorf("dns_enable_disable(enable=0) before removing a name server: %w", err)
		}
		d.ready.setEnabled(ctx, false)
	}
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

// Ready is the caller's guarantee (D-137) that it added at least one name server (dns_name_server_add_del) and
// enabled the resolver (dns_enable_disable(1)) on this VPP, and that both calls succeeded. VPP 26.06 dereferences a
// NULL name server in vnet_send_dns4_request → ip4_sas when dns_resolve_* runs without one (the 2026-09-25 04:27
// crash of the shared VPP, docs/vpp-code-track.md): the helpers below refuse to send anything without it.
type Ready bool

// ErrResolverNotReady is returned by ResolveName / ResolveIP called without Ready (nothing was sent to VPP).
var ErrResolverNotReady = errors.New("dns: dns_resolve_* may only be sent after dns_name_server_add_del and dns_enable_disable(1) succeeded " +
	"(VPP 26.06 crashes without a name server, D-137): nothing was sent")

// ResolveName asks VPP's resolver for name (dns_resolve_name). May be called only after
// dns_name_server_add_del and dns_enable_disable(1) succeeded on this VPP — the caller says so with ready (D-137);
// without it nothing is sent and ErrResolverNotReady is returned. The name is validated as a DNS name (letters,
// digits, "-", "_", "."); it is never passed to a shell. VPP replies only once the upstream answers or its retries
// give up, so ctx must carry a deadline (a host run without one blocked for minutes against unreachable upstreams).
func ResolveName(ctx context.Context, c vpp.Client, name string, ready Ready) (ip4, ip6 netip.Addr, err error) {
	if err := ValidateName(name); err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	if !ready {
		return netip.Addr{}, netip.Addr{}, ErrResolverNotReady
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

// ResolveIP asks VPP's resolver for the PTR name of addr (dns_resolve_ip). Same precondition as ResolveName: may be
// called only after dns_name_server_add_del and dns_enable_disable(1) succeeded (ready, D-137).
func ResolveIP(ctx context.Context, c vpp.Client, addr netip.Addr, ready Ready) (string, error) {
	if !ready {
		return "", ErrResolverNotReady
	}
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
