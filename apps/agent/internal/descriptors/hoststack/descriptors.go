package hoststack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	httpstatic "ngfw/agent/binapi/http_static"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/session"
	"ngfw/agent/binapi/tcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ---- hoststack.session ----------------------------------------------------------------------

// SessionDescriptor manages the hoststack.session singleton. VPP has no getter: the layer counts
// as on when session_rules_v2_dump answers (the probe). The owner enables the rule-table engine
// only when the probe fails; nobody ever disables or re-engines it (Delete is a no-op).
type SessionDescriptor struct {
	client  vpp.Client
	globals dfkit.Globals
}

var _ scheduler.Descriptor = (*SessionDescriptor)(nil)

// NewSession returns the hoststack.session descriptor with the given D-071 role.
func NewSession(client vpp.Client, g dfkit.Globals) *SessionDescriptor {
	return &SessionDescriptor{client: client, globals: g}
}

// Name implements scheduler.Descriptor.
func (*SessionDescriptor) Name() string { return NameSession }

// KeyOf implements scheduler.Descriptor.
func (*SessionDescriptor) KeyOf(proto.Message) scheduler.Key { return KeySession }

// Dependencies implements scheduler.Descriptor.
func (*SessionDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Probe reports whether the session layer answers (session_rules_v2_dump succeeds).
func Probe(ctx context.Context, c vpp.Client) (bool, error) {
	_, err := dumpRules(ctx, c)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, dfkit.ErrPluginNotLoaded) {
		return false, err
	}
	var apiErr api.VPPApiError
	if errors.As(err, &apiErr) {
		return false, nil // VPP refuses the dump while the layer is disabled
	}
	return false, err
}

func (d *SessionDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s Session
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if !s.Enabled {
		return nil // never disabled by the agent (package doc)
	}
	current := func(ctx context.Context) (proto.Message, bool, error) {
		on, err := Probe(ctx, d.client)
		if err != nil {
			return nil, false, err
		}
		return Session{Enabled: on}.Proto(), true, nil
	}
	if !d.globals.Owner() {
		return d.globals.Require(ctx, NameSession, obj, current)
	}
	if on, err := Probe(ctx, d.client); err != nil {
		return err
	} else if on {
		return nil // already on (whatever engine startup.conf chose): never re-engined
	}
	_, err := session.NewServiceClient(d.client).SessionEnableDisableV2(ctx,
		&session.SessionEnableDisableV2{RtEngineType: session.RT_BACKEND_ENGINE_API_RULE_TABLE})
	if err != nil {
		return fmt.Errorf("session_enable_disable_v2(rule-table): %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *SessionDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor.
func (d *SessionDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a no-op — the session layer is left as found.
func (*SessionDescriptor) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: no getter (write-only, D-063).
func (d *SessionDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	if !d.globals.Owner() {
		return d.globals.NonOwnerRetrieve(NameSession)
	}
	return nil, dfkit.RetrieveUnsupported(NameSession)
}

// ---- hoststack.namespace --------------------------------------------------------------------

// NamespaceDescriptor manages application namespaces (key hoststack.namespace/<id>). VPP has no
// namespace dump: write-only, Create is an idempotent add (VPP updates an existing id in place);
// the returned appns_index is recorded per VPP boot for the session rules and HostStackState.
type NamespaceDescriptor struct {
	client vpp.Client
	owner  string
	st     *state
}

var _ scheduler.Descriptor = (*NamespaceDescriptor)(nil)

func newNamespace(c vpp.Client, owner string, st *state) *NamespaceDescriptor {
	return &NamespaceDescriptor{client: c, owner: owner, st: st}
}

// Name implements scheduler.Descriptor.
func (*NamespaceDescriptor) Name() string { return NameNamespace }

// KeyOf implements scheduler.Descriptor.
func (*NamespaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s Namespace
	if err := dfkit.Decode(obj, &s); err != nil {
		return KeyNamespace("invalid")
	}
	return KeyNamespace(s.ID)
}

// Dependencies implements scheduler.Descriptor: the interface (alias key) when bound to one.
func (*NamespaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s Namespace
	if err := dfkit.Decode(obj, &s); err != nil || s.Interface == "" {
		return nil
	}
	return []scheduler.Dependency{{Key: dfkit.DefaultInterfaceKey(s.Interface)}}
}

func (d *NamespaceDescriptor) set(ctx context.Context, obj proto.Message, add bool) (uint32, error) {
	var s Namespace
	if err := dfkit.Decode(obj, &s); err != nil {
		return 0, err
	}
	if err := ValidID(s.ID); err != nil {
		return 0, err
	}
	req := &session.AppNamespaceAddDelV4{IsAdd: add, NamespaceID: s.ID, IP4FibID: s.Vrf, IP6FibID: s.Vrf,
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0))}
	if s.Interface != "" && add {
		idx, err := dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner)
		if err != nil {
			return 0, err
		}
		req.SwIfIndex = interface_types.InterfaceIndex(idx)
	}
	rep, err := session.NewServiceClient(d.client).AppNamespaceAddDelV4(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("app_namespace_add_del_v4(%s, add=%t): %w", s.ID, add, dfkit.PluginError("session", err))
	}
	return rep.AppnsIndex, nil
}

// Create implements scheduler.Descriptor (idempotent).
func (d *NamespaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	idx, err := d.set(ctx, obj, true)
	if err != nil {
		return nil, err
	}
	var s Namespace
	_ = dfkit.Decode(obj, &s)
	if err := d.st.setIndex(ctx, s.ID, idx, false); err != nil {
		return nil, scheduler.PartialCreate(err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: re-add (VPP updates in place).
func (d *NamespaceDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor.
func (d *NamespaceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	var s Namespace
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if _, err := d.set(ctx, obj, false); err != nil && !dfkit.IsVPPError(err, vppNoSuchEntry...) {
		return err
	}
	return d.st.setIndex(ctx, s.ID, 0, true)
}

// Retrieve implements scheduler.Descriptor: no dump (write-only, D-063).
func (*NamespaceDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameNamespace)
}

// ---- hoststack.session-rule -----------------------------------------------------------------

// VPP action_index values (session_rules_table.h): ~0-1 drop, ~0-2 allow, else an app index.
const (
	actionDrop  = ^uint32(0) - 1
	actionAllow = ^uint32(0) - 2
)

// RuleDescriptor manages session rules (key hoststack.session-rule/<tag>). VPP carries the tag
// as "<owner>:<tag>"; Retrieve reports only this owner's rules.
type RuleDescriptor struct {
	client vpp.Client
	owner  string
	st     *state
}

var _ scheduler.Descriptor = (*RuleDescriptor)(nil)

func newRule(c vpp.Client, owner string, st *state) *RuleDescriptor {
	return &RuleDescriptor{client: c, owner: owner, st: st}
}

// Name implements scheduler.Descriptor.
func (*RuleDescriptor) Name() string { return NameSessionRule }

// KeyOf implements scheduler.Descriptor.
func (*RuleDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s Rule
	if err := dfkit.Decode(obj, &s); err != nil {
		return KeyRule("invalid")
	}
	return KeyRule(s.Tag)
}

// Dependencies implements scheduler.Descriptor: the rule's namespace.
func (*RuleDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s Rule
	if err := dfkit.Decode(obj, &s); err != nil || s.AppNamespace == "" {
		return nil
	}
	return []scheduler.Dependency{{Key: KeyNamespace(s.AppNamespace)}}
}

// VPPTag is the tag VPP carries for this owner's rule.
func VPPTag(owner, tag string) string { return owner + ":" + tag }

func toAPIPrefix(s string) (ip_types.Prefix, netip.Prefix, error) {
	p, err := dfkit.ParsePrefix(s)
	if err != nil {
		return ip_types.Prefix{}, p, err
	}
	return ip_types.Prefix{Address: dfkit.ToAPIAddress(p.Addr()), Len: uint8(p.Bits())}, p, nil //nolint:gosec // 0..128
}

func fromAPIPrefix(p ip_types.Prefix) string {
	a := dfkit.FromAPIAddress(p.Address)
	pf, err := a.Prefix(int(p.Len))
	if err != nil {
		return fmt.Sprintf("%s/%d", a, p.Len)
	}
	return pf.String()
}

func (d *RuleDescriptor) request(ctx context.Context, obj proto.Message, add bool) (*session.SessionRuleAddDel, error) {
	var s Rule
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil, err
	}
	if err := ValidID(s.Tag); err != nil {
		return nil, err
	}
	tag := VPPTag(d.owner, s.Tag)
	if len(tag) > 63 {
		return nil, dfkit.Specf("rule tag %q: owner-prefixed tag longer than 63", s.Tag)
	}
	req := &session.SessionRuleAddDel{IsAdd: add, Tag: tag, LclPort: uint16(s.LocalPort), RmtPort: uint16(s.RemotePort)} //nolint:gosec // ports ≤ 65535 (schema)
	switch s.Transport {
	case "tcp":
		req.TransportProto = session.TRANSPORT_PROTO_API_TCP
	case "udp":
		req.TransportProto = session.TRANSPORT_PROTO_API_UDP
	default:
		return nil, dfkit.Specf("rule %q: transport %q", s.Tag, s.Transport)
	}
	switch s.Scope {
	case "global":
		req.Scope = session.SESSION_RULE_SCOPE_API_GLOBAL
	case "local":
		req.Scope = session.SESSION_RULE_SCOPE_API_LOCAL
	default:
		return nil, dfkit.Specf("rule %q: scope %q", s.Tag, s.Scope)
	}
	switch s.Action {
	case "allow":
		req.ActionIndex = actionAllow
	case "deny":
		req.ActionIndex = actionDrop
	case "redirect":
		if s.RedirectAppIndex >= actionAllow {
			return nil, dfkit.Specf("rule %q: redirect app index %d reserved", s.Tag, s.RedirectAppIndex)
		}
		req.ActionIndex = s.RedirectAppIndex
	default:
		return nil, dfkit.Specf("rule %q: action %q", s.Tag, s.Action)
	}
	var lp, rp netip.Prefix
	var err error
	if req.Lcl, lp, err = toAPIPrefix(s.Local); err != nil {
		return nil, err
	}
	if req.Rmt, rp, err = toAPIPrefix(s.Remote); err != nil {
		return nil, err
	}
	if lp.Addr().Is4() != rp.Addr().Is4() {
		return nil, dfkit.Specf("rule %q: local and remote prefixes of different families", s.Tag)
	}
	if s.AppNamespace != "" {
		idx, err := d.st.currentIndexes(ctx)
		if err != nil {
			return nil, err
		}
		i, ok := idx[s.AppNamespace]
		if !ok {
			return nil, fmt.Errorf("rule %q: app namespace %q is not applied on this VPP instance", s.Tag, s.AppNamespace)
		}
		req.AppnsIndex = i
	}
	return req, nil
}

func (d *RuleDescriptor) send(ctx context.Context, req *session.SessionRuleAddDel) error {
	if _, err := session.NewServiceClient(d.client).SessionRuleAddDel(ctx, req); err != nil {
		return fmt.Errorf("session_rule_add_del(%s, add=%t): %w", req.Tag, req.IsAdd, dfkit.PluginError("session", err))
	}
	return nil
}

// Create implements scheduler.Descriptor. VPP replaces the action of an existing identical match,
// so a duplicate add is harmless.
func (d *RuleDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	req, err := d.request(ctx, obj, true)
	if err != nil {
		return nil, err
	}
	return nil, d.send(ctx, req)
}

// Update implements scheduler.Descriptor: delete the old match, add the new one.
func (d *RuleDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	if err := d.Delete(ctx, oldObj, nil); err != nil {
		return nil, err
	}
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor; an absent rule counts as deleted.
func (d *RuleDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	req, err := d.request(ctx, obj, false)
	if err != nil {
		var s Rule
		if dfkit.Decode(obj, &s) == nil && s.AppNamespace != "" && !errors.Is(err, dfkit.ErrSpec) {
			return nil // namespace gone with the VPP instance: so is the rule
		}
		return err
	}
	err = d.send(ctx, req)
	if dfkit.IsVPPError(err, vppNoSuchEntry...) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: session_rules_v2_dump, this owner's tags only. A dump
// VPP refuses (session layer off, plugin not loaded, message unknown) means no rule can exist: the
// result is empty, not an error — every other domain's Retrieve must not fail because of the host
// stack, and an absent rule only makes the reconciler re-add it (idempotent). A lost connection or
// a cancelled context is still an error.
func (d *RuleDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	details, err := dumpRules(ctx, d.client)
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) || ctx.Err() != nil {
			return nil, err
		}
		return nil, nil
	}
	idx, err := d.st.currentIndexes(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, r := range details {
		s, ok := ruleFromDetails(d.owner, r, idx)
		if !ok {
			continue
		}
		out = append(out, scheduler.KV{Key: KeyRule(s.Tag), Value: s.Proto()})
	}
	return out, nil
}

func ruleFromDetails(owner string, r *session.SessionRulesV2Details, idx map[string]uint32) (Rule, bool) {
	prefix := owner + ":"
	if !strings.HasPrefix(r.Tag, prefix) {
		return Rule{}, false
	}
	s := Rule{Tag: strings.TrimPrefix(r.Tag, prefix), Local: fromAPIPrefix(r.Lcl), Remote: fromAPIPrefix(r.Rmt),
		LocalPort: uint32(r.LclPort), RemotePort: uint32(r.RmtPort)}
	s.Transport = map[session.TransportProto]string{session.TRANSPORT_PROTO_API_TCP: "tcp", session.TRANSPORT_PROTO_API_UDP: "udp"}[r.TransportProto]
	if s.Transport == "" {
		s.Transport = r.TransportProto.String()
	}
	s.Scope = map[session.SessionRuleScope]string{session.SESSION_RULE_SCOPE_API_GLOBAL: "global", session.SESSION_RULE_SCOPE_API_LOCAL: "local"}[r.Scope]
	if s.Scope == "" {
		s.Scope = r.Scope.String()
	}
	s.Action, s.RedirectAppIndex = actionName(r.ActionIndex)
	// App namespace: the named one of this owner whose index the rule applies to; none = default.
	for _, name := range sortedKeys(idx) {
		for _, i := range r.AppnsIndex {
			if i == idx[name] && i != 0 {
				s.AppNamespace = name
			}
		}
		if s.AppNamespace != "" {
			break
		}
	}
	return s, true
}

func actionName(a uint32) (string, uint32) {
	switch a {
	case actionAllow:
		return "allow", 0
	case actionDrop:
		return "deny", 0
	default:
		return "redirect", a
	}
}

func dumpRules(ctx context.Context, c vpp.Client) ([]*session.SessionRulesV2Details, error) {
	stream, err := session.NewServiceClient(c).SessionRulesV2Dump(ctx, &session.SessionRulesV2Dump{})
	if err != nil {
		return nil, fmt.Errorf("session_rules_v2_dump: %w", dfkit.PluginError("session", err))
	}
	details, err := dfkit.Drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("session_rules_v2_dump: %w", dfkit.PluginError("session", err))
	}
	return details, nil
}

// ---- hoststack.tcp-src ----------------------------------------------------------------------

// TCPSrcDescriptor manages the TCP source-address pool of a fib (key hoststack.tcp-src/<fib>).
// tcp_configure_src_addresses has no dump and no delete: the add is recorded once per VPP boot
// (D-076) and Delete only forgets the record — the pool stays in VPP until it restarts.
type TCPSrcDescriptor struct {
	client vpp.Client
	boot   dfkit.BootStore
}

var _ scheduler.Descriptor = (*TCPSrcDescriptor)(nil)

// NewTCPSrc returns the hoststack.tcp-src descriptor.
func NewTCPSrc(c vpp.Client, boot dfkit.BootStore) *TCPSrcDescriptor {
	return &TCPSrcDescriptor{client: c, boot: boot}
}

// Name implements scheduler.Descriptor.
func (*TCPSrcDescriptor) Name() string { return NameTCPSrc }

// KeyOf implements scheduler.Descriptor.
func (*TCPSrcDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s TCPSrc
	if err := dfkit.Decode(obj, &s); err != nil {
		return KeyTCPSrc(^uint32(0))
	}
	return KeyTCPSrc(s.Vrf)
}

// Dependencies implements scheduler.Descriptor: the VRF (non-default).
func (*TCPSrcDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s TCPSrc
	if err := dfkit.Decode(obj, &s); err != nil || s.Vrf == 0 {
		return nil
	}
	return []scheduler.Dependency{{Key: dfkit.DefaultVRFKey(fmt.Sprint(s.Vrf))}}
}

// Create implements scheduler.Descriptor: applied once per VPP boot.
func (d *TCPSrcDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	var s TCPSrc
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil, err
	}
	first, err := dfkit.ParseAddr(s.First)
	if err != nil {
		return nil, err
	}
	last, err := dfkit.ParseAddr(s.Last)
	if err != nil {
		return nil, err
	}
	if first.Is4() != last.Is4() || last.Less(first) {
		return nil, dfkit.Specf("tcp source range %s-%s: same family, first ≤ last", first, last)
	}
	key := d.KeyOf(obj)
	value := first.String() + "-" + last.String()
	done, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.boot, key, value)
	if err != nil {
		return nil, err
	}
	if done {
		return nil, nil
	}
	if _, err := tcp.NewServiceClient(d.client).TCPConfigureSrcAddresses(ctx, &tcp.TCPConfigureSrcAddresses{
		VrfID: s.Vrf, FirstAddress: dfkit.ToAPIAddress(first), LastAddress: dfkit.ToAPIAddress(last)}); err != nil {
		return nil, fmt.Errorf("tcp_configure_src_addresses(%s, fib %d): %w", value, s.Vrf, dfkit.PluginError("tcp", err))
	}
	if err := d.boot.Put(dfkit.BootRecord{Key: string(key), Identity: id, Value: value}); err != nil {
		return nil, scheduler.PartialCreate(err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: VPP adds the new range (the old one stays until restart).
func (d *TCPSrcDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: no delete in VPP — only the record is dropped.
func (d *TCPSrcDescriptor) Delete(_ context.Context, obj proto.Message, _ any) error {
	return d.boot.Delete(string(d.KeyOf(obj)))
}

// Retrieve implements scheduler.Descriptor: no dump (write-only, D-063).
func (*TCPSrcDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameTCPSrc)
}

// ---- hoststack.http-static ------------------------------------------------------------------

// HTTPStaticDescriptor manages the http_static server (globals owner + opt-in only). No disable,
// no dump: applied once per VPP boot; Delete only forgets the record.
type HTTPStaticDescriptor struct {
	client vpp.Client
	boot   dfkit.BootStore
}

var _ scheduler.Descriptor = (*HTTPStaticDescriptor)(nil)

// NewHTTPStatic returns the hoststack.http-static descriptor.
func NewHTTPStatic(c vpp.Client, boot dfkit.BootStore) *HTTPStaticDescriptor {
	return &HTTPStaticDescriptor{client: c, boot: boot}
}

// Name implements scheduler.Descriptor.
func (*HTTPStaticDescriptor) Name() string { return NameHTTPStatic }

// KeyOf implements scheduler.Descriptor.
func (*HTTPStaticDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyHTTPStatic }

// Dependencies implements scheduler.Descriptor: the session layer.
func (*HTTPStaticDescriptor) Dependencies(proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: KeySession}}
}

// Create implements scheduler.Descriptor.
func (d *HTTPStaticDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	var s HTTPStatic
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil, err
	}
	if err := ValidWWWRoot(s.WWWRoot); err != nil {
		return nil, err
	}
	if s.URI == "" || len(s.URI) > 255 {
		return nil, dfkit.Specf("http_static uri %q", s.URI)
	}
	value := string(mustJSON(s))
	done, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.boot, KeyHTTPStatic, value)
	if err != nil {
		return nil, err
	}
	if done {
		return nil, nil
	}
	if _, err := httpstatic.NewServiceClient(d.client).HTTPStaticEnableV5(ctx, &httpstatic.HTTPStaticEnableV5{
		WwwRoot: s.WWWRoot, URI: s.URI, CacheSizeLimit: s.CacheSizeMB << 20,
		MaxAge: 600, KeepaliveTimeout: 60, MaxBodySize: 8192, RxBuffThresh: 1 << 20}); err != nil {
		return nil, fmt.Errorf("http_static_enable_v5: %w", dfkit.PluginError("http_static", err))
	}
	if err := d.boot.Put(dfkit.BootRecord{Key: string(KeyHTTPStatic), Identity: id, Value: value}); err != nil {
		return nil, scheduler.PartialCreate(err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor: VPP has no disable — a changed server needs a restart.
func (d *HTTPStaticDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	started, err := dfkit.StartedThisBoot(ctx, d.client, d.boot, KeyHTTPStatic)
	if err != nil {
		return nil, err
	}
	if started {
		return nil, errors.New("http_static is already running on this VPP instance and cannot be changed or disabled via the API; restart VPP")
	}
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: forgets the record (the server runs until VPP restarts).
func (d *HTTPStaticDescriptor) Delete(context.Context, proto.Message, any) error {
	return d.boot.Delete(string(KeyHTTPStatic))
}

// Retrieve implements scheduler.Descriptor: no dump (write-only, D-063).
func (*HTTPStaticDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameHTTPStatic)
}

// vppNoSuchEntry are the retvals of a delete of something that is not there.
var vppNoSuchEntry = []api.VPPApiError{api.NO_SUCH_ENTRY, api.INVALID_VALUE}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err) // plain struct of strings and numbers
	}
	return raw
}
