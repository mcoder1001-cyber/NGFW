// Package bfd holds the reconciler descriptors for VPP BFD (task DF-7, WBS D3.8): the
// authentication keys (bfd_auth_set_key / bfd_auth_del_key, retrieved with bfd_auth_keys_dump),
// single-hop UDP sessions (bfd_udp_add / _mod / _del, bfd_udp_session_set_flags,
// bfd_udp_auth_activate / _deactivate, retrieved with bfd_udp_session_dump) and the echo
// source interface (bfd_udp_set_echo_source / _del_echo_source, bfd_udp_get_echo_source).
// WatchEvents turns want_bfd_events + bfd_udp_session_event into session state changes.
//
// Secrets: a key's secret never enters a Value (it would be diffed, logged and persisted as
// desired state). The Value carries the conf-key id and type; the secret is resolved at Create
// time through the Secrets function the agent passes to NewAuthKey / Register (P05 wires the
// encrypted store). VPP never reports key material, and refuses to modify a key in use, so a secret is
// immutable per conf-key id: rotate by creating a new key id and moving the sessions to it
// (RFC 5880 §6.7 key ids).
//
// Messages come only from apps/agent/binapi/bfd. Ownership: sessions and the echo source
// belong to the owner of their interface; keys are attributed by conf-key id
// (df7.WithIDRange). docs/agent/descriptors/bfd.md is the object ↔ message table.
package bfd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/bfd"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameAuthKey    = "bfd.auth-key"
	NameSession    = "bfd.udp-session"
	NameEchoSource = "bfd.echo-source"
)

// EchoSourceID is the object id of the echo-source singleton.
const EchoSourceID = "global"

// Authentication types VPP 26.06 supports (bfd_auth_type_supported: SHA1 variants only).
const (
	AuthKeyedSHA1           = "keyed-sha1"
	AuthMeticulousKeyedSHA1 = "meticulous-keyed-sha1"
)

var authTypes = map[string]uint8{AuthKeyedSHA1: 4, AuthMeticulousKeyedSHA1: 5}

// MaxKeyLen is the SHA1 key length limit (bfd_auth_set_key key[20]).
const MaxKeyLen = 20

// ErrNoSecret is returned when the secret of a key cannot be resolved.
var ErrNoSecret = errors.New("bfd: no secret for auth key")

// Secrets resolves the secret of a conf-key id. The returned bytes are sent to VPP once and
// never logged or stored in a Value.
type Secrets func(ctx context.Context, confKeyID uint32) ([]byte, error)

// ---- specs ------------------------------------------------------------------------------------

// AuthKey is the desired state of one bfd.auth-key object.
type AuthKey struct {
	ID   uint32 `json:"id,omitempty"`
	Type string `json:"type,omitempty"`
}

// Validate checks k.
func (k AuthKey) Validate() error {
	if _, ok := authTypes[k.Type]; !ok {
		return df7.Specf("bfd auth type %q: VPP 26.06 supports keyed-sha1 and meticulous-keyed-sha1", k.Type)
	}
	return nil
}

// SessionAuth selects the key a session authenticates with: the conf-key (bfd.auth-key id)
// and the BFD key id sent on the wire.
type SessionAuth struct {
	ConfKeyID uint32 `json:"conf_key_id,omitempty"`
	BFDKeyID  uint8  `json:"bfd_key_id,omitempty"`
}

// Session is the desired state of one bfd.udp-session object (single hop). Timers are in
// microseconds. AdminDown holds the session in AdminDown (bfd_udp_session_set_flags).
type Session struct {
	Interface     string       `json:"interface,omitempty"`
	Local         string       `json:"local,omitempty"`
	Peer          string       `json:"peer,omitempty"`
	DesiredMinTx  uint32       `json:"desired_min_tx,omitempty"`
	RequiredMinRx uint32       `json:"required_min_rx,omitempty"`
	DetectMult    uint8        `json:"detect_mult,omitempty"`
	AdminDown     bool         `json:"admin_down,omitempty"`
	Auth          *SessionAuth `json:"auth,omitempty"`
}

// Validate checks s: canonical addresses of one family, non-zero timers and multiplier.
func (s Session) Validate() error {
	if s.Interface == "" {
		return df7.Specf("bfd session needs an interface")
	}
	l, err := df7.ParseAddr(s.Local)
	if err != nil {
		return err
	}
	p, err := df7.ParseAddr(s.Peer)
	if err != nil {
		return err
	}
	if l.String() != s.Local || p.String() != s.Peer {
		return df7.Specf("bfd addresses must be canonical (%s, %s)", l, p)
	}
	if l.Is4() != p.Is4() {
		return df7.Specf("bfd local %s and peer %s are different families", l, p)
	}
	if s.DesiredMinTx == 0 || s.RequiredMinRx == 0 || s.DetectMult == 0 {
		return df7.Specf("bfd desired_min_tx, required_min_rx and detect_mult must be > 0")
	}
	return nil
}

// EchoSource is the desired state of the bfd.echo-source singleton.
type EchoSource struct {
	Interface string `json:"interface,omitempty"`
}

// Validate checks e.
func (e EchoSource) Validate() error {
	if e.Interface == "" {
		return df7.Specf("bfd echo source needs an interface")
	}
	return nil
}

// ---- keys -------------------------------------------------------------------------------------

// KeyAuthKey is "bfd.auth-key/<conf key id>".
func KeyAuthKey(id uint32) scheduler.Key {
	return scheduler.Join(NameAuthKey, strconv.FormatUint(uint64(id), 10))
}

// KeySession is "bfd.udp-session/<interface>/<local>/<peer>".
func KeySession(ifName, local, peer string) scheduler.Key {
	return scheduler.Join(NameSession, ifName, local, peer)
}

// KeyEchoSource is "bfd.echo-source/global".
func KeyEchoSource() scheduler.Key { return scheduler.Join(NameEchoSource, EchoSourceID) }

func mustAddr(s string) ip_types.Address {
	a, _ := df7.ParseAddr(s) // validated by Session.Validate
	return df7.ToAddress(a)
}

// ---- bfd.auth-key -----------------------------------------------------------------------------

// AuthKeyDescriptor manages bfd.auth-key objects.
type AuthKeyDescriptor struct {
	df7.Base
	secrets Secrets
}

var _ scheduler.Descriptor = (*AuthKeyDescriptor)(nil)

// NewAuthKey returns the bfd.auth-key descriptor.
func NewAuthKey(c vpp.Client, owner string, secrets Secrets, opts ...df7.Option) *AuthKeyDescriptor {
	return &AuthKeyDescriptor{Base: df7.NewBase(NameAuthKey, c, owner, opts), secrets: secrets}
}

// KeyOf implements scheduler.Descriptor.
func (d *AuthKeyDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	k, _ := df7.Decode[AuthKey](obj)
	return KeyAuthKey(k.ID)
}

// Dependencies implements scheduler.Descriptor: none.
func (*AuthKeyDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor: bfd_auth_set_key with the resolved secret.
func (d *AuthKeyDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	k, err := df7.DecodeValid[AuthKey](obj)
	if err != nil {
		return nil, err
	}
	if err := d.Opts.CheckID("bfd conf key", k.ID); err != nil {
		return nil, err
	}
	if d.secrets == nil {
		return nil, fmt.Errorf("%w %d (no secret resolver configured)", ErrNoSecret, k.ID)
	}
	secret, err := d.secrets(ctx, k.ID)
	if err != nil {
		return nil, fmt.Errorf("%w %d: %v", ErrNoSecret, k.ID, err)
	}
	if len(secret) == 0 || len(secret) > MaxKeyLen {
		return nil, df7.Specf("bfd key %d secret must be 1..%d bytes", k.ID, MaxKeyLen)
	}
	req := &bfd.BfdAuthSetKey{ConfKeyID: k.ID, AuthType: authTypes[k.Type], KeyLen: uint8(len(secret)), Key: make([]byte, MaxKeyLen)} //nolint:gosec // ≤ 20
	copy(req.Key, secret)
	_, err = bfd.NewServiceClient(d.Client).BfdAuthSetKey(ctx, req)
	for i := range req.Key {
		req.Key[i] = 0
	}
	return nil, d.Wrap(fmt.Sprintf("bfd_auth_set_key %d", k.ID), err) // the secret is never part of an error
}

// Update implements scheduler.Descriptor: the type of a key cannot change while sessions use
// it; a new type is a new key (ErrRecreate).
func (*AuthKeyDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: bfd_auth_del_key (sessions depend on the key and are
// moved or deleted first; VPP refuses a key in use with BFD_EINUSE).
func (d *AuthKeyDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	k, err := df7.Decode[AuthKey](obj)
	if err != nil {
		return err
	}
	_, err = bfd.NewServiceClient(d.Client).BfdAuthDelKey(ctx, &bfd.BfdAuthDelKey{ConfKeyID: k.ID})
	return d.Wrap(fmt.Sprintf("bfd_auth_del_key %d", k.ID), err)
}

// Retrieve implements scheduler.Descriptor: bfd_auth_keys_dump, ids in the owned range.
func (d *AuthKeyDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := bfd.NewServiceClient(d.Client).BfdAuthKeysDump(ctx, &bfd.BfdAuthKeysDump{})
	if err != nil {
		return nil, d.Wrap("bfd_auth_keys_dump", err)
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("bfd_auth_keys_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		if !d.Opts.IDs.Owns(det.ConfKeyID) {
			continue
		}
		k := AuthKey{ID: det.ConfKeyID, Type: fmt.Sprintf("#%d", det.AuthType)}
		for n, v := range authTypes {
			if v == det.AuthType {
				k.Type = n
			}
		}
		out = append(out, df7.KV(KeyAuthKey(k.ID), k, KeyMeta{UseCount: det.UseCount}))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// KeyMeta reports how many sessions use a key (read-only, from the dump).
type KeyMeta struct{ UseCount uint32 }

// ---- bfd.udp-session --------------------------------------------------------------------------

// SessionMeta is the interface index of a session.
type SessionMeta struct{ SwIfIndex uint32 }

// SessionDescriptor manages bfd.udp-session objects.
type SessionDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*SessionDescriptor)(nil)

// NewSession returns the bfd.udp-session descriptor.
func NewSession(c vpp.Client, owner string, opts ...df7.Option) *SessionDescriptor {
	return &SessionDescriptor{df7.NewBase(NameSession, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *SessionDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, _ := df7.Decode[Session](obj)
	return KeySession(s.Interface, s.Local, s.Peer)
}

// Dependencies implements scheduler.Descriptor: the interface and the auth key when set.
// (VPP 26.06 does not require the local address on the interface, so no interface-ip
// dependency is declared — docs/agent/descriptors/bfd.md.)
func (d *SessionDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, _ := df7.Decode[Session](obj)
	deps := []scheduler.Dependency{d.Opts.IfaceDep(s.Interface)}
	if s.Auth != nil {
		deps = append(deps, scheduler.Dependency{Key: KeyAuthKey(s.Auth.ConfKeyID)})
	}
	return deps
}

func (d *SessionDescriptor) setFlags(ctx context.Context, idx uint32, s Session) error {
	flags := interface_types.IF_STATUS_API_FLAG_ADMIN_UP
	if s.AdminDown {
		flags = 0
	}
	l, p := mustAddr(s.Local), mustAddr(s.Peer)
	_, err := bfd.NewServiceClient(d.Client).BfdUDPSessionSetFlags(ctx, &bfd.BfdUDPSessionSetFlags{
		SwIfIndex: interface_types.InterfaceIndex(idx), LocalAddr: l, PeerAddr: p, Flags: flags})
	return d.Wrap(fmt.Sprintf("bfd_udp_session_set_flags %s→%s admin_down=%v", s.Local, s.Peer, s.AdminDown), err)
}

// Create implements scheduler.Descriptor: bfd_udp_add (with the key when set), then
// bfd_udp_session_set_flags when the session is to be held AdminDown.
func (d *SessionDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := df7.DecodeValid[Session](obj)
	if err != nil {
		return nil, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.OwnedIndex(s.Interface)
	if err != nil {
		return nil, err
	}
	req := &bfd.BfdUDPAdd{SwIfIndex: interface_types.InterfaceIndex(idx), DesiredMinTx: s.DesiredMinTx, RequiredMinRx: s.RequiredMinRx,
		LocalAddr: mustAddr(s.Local), PeerAddr: mustAddr(s.Peer), DetectMult: s.DetectMult}
	if s.Auth != nil {
		req.IsAuthenticated, req.BfdKeyID, req.ConfKeyID = true, s.Auth.BFDKeyID, s.Auth.ConfKeyID
	}
	if _, err := bfd.NewServiceClient(d.Client).BfdUDPAdd(ctx, req); err != nil {
		return nil, d.Wrap(fmt.Sprintf("bfd_udp_add %s %s→%s", s.Interface, s.Local, s.Peer), err)
	}
	if s.AdminDown {
		if err := d.setFlags(ctx, idx, s); err != nil {
			return nil, err
		}
	}
	return SessionMeta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor in place: timers (bfd_udp_mod), authentication
// (bfd_udp_auth_activate / _deactivate, immediate) and admin state (set_flags). Interface and
// addresses are the key.
func (d *SessionDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, err := df7.Decode[Session](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[Session](newObj)
	if err != nil {
		return nil, err
	}
	if o.Interface != n.Interface || o.Local != n.Local || o.Peer != n.Peer {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(SessionMeta)
	if !ok {
		return nil, df7.BadMeta(NameSession, meta)
	}
	svc := bfd.NewServiceClient(d.Client)
	idx := interface_types.InterfaceIndex(m.SwIfIndex)
	l, p := mustAddr(n.Local), mustAddr(n.Peer)
	if o.DesiredMinTx != n.DesiredMinTx || o.RequiredMinRx != n.RequiredMinRx || o.DetectMult != n.DetectMult {
		if _, err := svc.BfdUDPMod(ctx, &bfd.BfdUDPMod{SwIfIndex: idx, DesiredMinTx: n.DesiredMinTx, RequiredMinRx: n.RequiredMinRx,
			LocalAddr: l, PeerAddr: p, DetectMult: n.DetectMult}); err != nil {
			return nil, d.Wrap("bfd_udp_mod", err)
		}
	}
	switch {
	case n.Auth != nil && (o.Auth == nil || *o.Auth != *n.Auth):
		if _, err := svc.BfdUDPAuthActivate(ctx, &bfd.BfdUDPAuthActivate{SwIfIndex: idx, LocalAddr: l, PeerAddr: p,
			BfdKeyID: n.Auth.BFDKeyID, ConfKeyID: n.Auth.ConfKeyID}); err != nil {
			return nil, d.Wrap("bfd_udp_auth_activate", err)
		}
	case n.Auth == nil && o.Auth != nil:
		if _, err := svc.BfdUDPAuthDeactivate(ctx, &bfd.BfdUDPAuthDeactivate{SwIfIndex: idx, LocalAddr: l, PeerAddr: p}); err != nil {
			return nil, d.Wrap("bfd_udp_auth_deactivate", err)
		}
	}
	if o.AdminDown != n.AdminDown {
		if err := d.setFlags(ctx, m.SwIfIndex, n); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// Delete implements scheduler.Descriptor: bfd_udp_del.
func (d *SessionDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	s, err := df7.Decode[Session](obj)
	if err != nil {
		return err
	}
	m, ok := meta.(SessionMeta)
	if !ok {
		return df7.BadMeta(NameSession, meta)
	}
	_, err = bfd.NewServiceClient(d.Client).BfdUDPDel(ctx, &bfd.BfdUDPDel{SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex),
		LocalAddr: mustAddr(s.Local), PeerAddr: mustAddr(s.Peer)})
	return d.Wrap(fmt.Sprintf("bfd_udp_del %s %s→%s", s.Interface, s.Local, s.Peer), err)
}

// Retrieve implements scheduler.Descriptor: bfd_udp_session_dump, single-hop sessions on owned
// interfaces. AdminDown is decoded from the AdminDown state; the live state (down/init/up) is
// read-only and reported through WatchEvents / Sessions, not in the Value.
func (d *SessionDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	dets, err := dumpSessions(ctx, d.Client)
	if err != nil {
		return nil, d.Wrap("bfd_udp_session_dump", err)
	}
	var out []scheduler.KV
	for _, det := range dets {
		if uint32(det.SwIfIndex) == df7.NoIndex {
			continue // multihop
		}
		name, ok := ifs.OwnedName(uint32(det.SwIfIndex))
		if !ok {
			continue
		}
		s := Session{Interface: name, Local: df7.FromAddress(det.LocalAddr).String(), Peer: df7.FromAddress(det.PeerAddr).String(),
			DesiredMinTx: det.DesiredMinTx, RequiredMinRx: det.RequiredMinRx, DetectMult: det.DetectMult,
			AdminDown: det.State == bfd.BFD_STATE_API_ADMIN_DOWN}
		if det.IsAuthenticated {
			s.Auth = &SessionAuth{ConfKeyID: det.ConfKeyID, BFDKeyID: det.BfdKeyID}
		}
		out = append(out, df7.KV(KeySession(s.Interface, s.Local, s.Peer), s, SessionMeta{SwIfIndex: uint32(det.SwIfIndex)}))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func dumpSessions(ctx context.Context, c vpp.Client) ([]*bfd.BfdUDPSessionDetails, error) {
	stream, err := bfd.NewServiceClient(c).BfdUDPSessionDump(ctx, &bfd.BfdUDPSessionDump{})
	if err != nil {
		return nil, err
	}
	return df7.Collect(stream.Recv)
}

// ---- bfd.echo-source --------------------------------------------------------------------------

// EchoMeta is the interface index of the echo source.
type EchoMeta struct{ SwIfIndex uint32 }

// EchoSourceDescriptor manages the bfd.echo-source singleton.
type EchoSourceDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*EchoSourceDescriptor)(nil)

// NewEchoSource returns the bfd.echo-source descriptor.
func NewEchoSource(c vpp.Client, owner string, opts ...df7.Option) *EchoSourceDescriptor {
	return &EchoSourceDescriptor{df7.NewBase(NameEchoSource, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (*EchoSourceDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyEchoSource() }

// Dependencies implements scheduler.Descriptor: the interface.
func (d *EchoSourceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	e, _ := df7.Decode[EchoSource](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(e.Interface)}
}

// Create implements scheduler.Descriptor: bfd_udp_set_echo_source.
func (d *EchoSourceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	e, err := df7.DecodeValid[EchoSource](obj)
	if err != nil {
		return nil, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.OwnedIndex(e.Interface)
	if err != nil {
		return nil, err
	}
	_, err = bfd.NewServiceClient(d.Client).BfdUDPSetEchoSource(ctx, &bfd.BfdUDPSetEchoSource{SwIfIndex: interface_types.InterfaceIndex(idx)})
	if err != nil {
		return nil, d.Wrap("bfd_udp_set_echo_source "+e.Interface, err)
	}
	return EchoMeta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: set the new interface in place.
func (d *EchoSourceDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: bfd_udp_del_echo_source.
func (d *EchoSourceDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	_, err := bfd.NewServiceClient(d.Client).BfdUDPDelEchoSource(ctx, &bfd.BfdUDPDelEchoSource{})
	return d.Wrap("bfd_udp_del_echo_source", err)
}

// Retrieve implements scheduler.Descriptor: bfd_udp_get_echo_source; reported when set on an
// owned interface.
func (d *EchoSourceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	rep, err := bfd.NewServiceClient(d.Client).BfdUDPGetEchoSource(ctx, &bfd.BfdUDPGetEchoSource{})
	if err != nil {
		return nil, d.Wrap("bfd_udp_get_echo_source", err)
	}
	if !rep.IsSet {
		return nil, nil
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	name, ok := ifs.OwnedName(uint32(rep.SwIfIndex))
	if !ok {
		return nil, nil
	}
	return []scheduler.KV{df7.KV(KeyEchoSource(), EchoSource{Interface: name}, EchoMeta{SwIfIndex: uint32(rep.SwIfIndex)})}, nil
}

// Register constructs every bfd descriptor (keys before sessions).
func Register(r scheduler.Registry, c vpp.Client, owner string, secrets Secrets, opts ...df7.Option) {
	r.Register(NewAuthKey(c, owner, secrets, opts...))
	r.Register(NewSession(c, owner, opts...))
	r.Register(NewEchoSource(c, owner, opts...))
}
