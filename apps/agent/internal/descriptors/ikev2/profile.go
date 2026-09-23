package ikev2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/binapi/ikev2_types"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// Profile is the composite IKEv2 profile: ikev2_profile_add_del plus every per-profile setter
// (ikev2_profile_set_auth, _set_id ×2, _set_ts ×2, ikev2_set_responder / _responder_hostname,
// ikev2_set_ike_transforms, ikev2_set_esp_transforms, ikev2_set_sa_lifetime,
// ikev2_profile_set_udp_encap, ikev2_profile_set_ipsec_udp_port, ikev2_set_tunnel_interface,
// ikev2_profile_disable_natt); Retrieve decodes ikev2_profile_dump.
//
// Update re-issues only the setters whose part changed. VPP cannot unset a part once set, so
// removing auth / an id / a selector / the responder / transforms / lifetime / the tunnel
// interface, or switching udp_encap or natt_disabled back off, returns ErrRecreate.
//
// Secrets: the PSK is a reference; ikev2_profile_dump returns the PSK in clear, Retrieve hashes it
// into the reference and zeroes the buffer.
//
// Retrieve reports only what VPP dumps (D-063: no cached desired state). Two consequences, see
// docs/agent/descriptors/ikev2.md: a responder given by hostname is the separate write-only
// descriptor ikev2.responder-hostname (VPP does not dump the hostname), and ip4/ip6 ids whose
// bytes contain a zero followed by a non-zero byte (10.4.0.1) are refused, because govpp decodes
// id.data (string[64]) only up to the first NUL and such an id could never be retrieved exactly.
type Profile struct{ cfg Config }

// ProfileMeta is the runtime handle of a profile: its VPP name.
type ProfileMeta struct{ VPPName string }

// NewProfile returns the descriptor.
func NewProfile(cfg Config) *Profile { return &Profile{cfg: cfg} }

// VPPName is the VPP profile name of the desired profile name: "<owner>-<name>".
func (d *Profile) VPPName(name string) string { return vppName(d.cfg.Owner, name) }

func vppName(owner, name string) string { return owner + "-" + name }

// Name implements scheduler.Descriptor.
func (*Profile) Name() string { return ProfileName }

// KeyOf implements scheduler.Descriptor: ikev2.profile/<name>.
func (*Profile) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.Ikev2Profile)
	return scheduler.Join(ProfileName, o.GetName())
}

// Dependencies implements scheduler.Descriptor: the responder and tunnel interfaces (Optional)
// and, for rsa-sig auth, the local key (Optional).
func (*Profile) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.Ikev2Profile)
	var deps []scheduler.Dependency
	if i := o.GetResponder().GetInterface(); i != "" {
		deps = append(deps, scheduler.Dependency{Key: vpn.InterfaceKey(i), Optional: true})
	}
	if i := o.GetTunnelInterface(); i != "" {
		deps = append(deps, scheduler.Dependency{Key: vpn.InterfaceKey(i), Optional: true})
	}
	if o.GetAuth().GetMethod() == AuthRSASig {
		deps = append(deps, scheduler.Dependency{Key: LocalKeyKey, Optional: true})
	}
	return deps
}

// Create implements scheduler.Descriptor. A failing setter deletes the half-built profile.
func (d *Profile) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.Ikev2Profile)
	if !ok {
		return nil, typeErr(ProfileName, obj)
	}
	name, err := d.checkName(o.GetName())
	if err != nil {
		return nil, err
	}
	if err := validate(o); err != nil {
		return nil, err
	}
	svc := ikev2.NewServiceClient(d.cfg.Client)
	if _, err := svc.Ikev2ProfileAddDel(ctx, &ikev2.Ikev2ProfileAddDel{Name: name, IsAdd: true}); err != nil {
		return nil, fmt.Errorf("ikev2_profile_add_del (%s): %w", name, err)
	}
	if err := d.apply(ctx, name, nil, o); err != nil {
		_, _ = svc.Ikev2ProfileAddDel(ctx, &ikev2.Ikev2ProfileAddDel{Name: name, IsAdd: false})
		return nil, err
	}
	return ProfileMeta{VPPName: name}, nil
}

// Update implements scheduler.Descriptor.
func (d *Profile) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, ok := oldObj.(*vpnpb.Ikev2Profile)
	n, ok2 := newObj.(*vpnpb.Ikev2Profile)
	if !ok || !ok2 {
		return nil, typeErr(ProfileName, newObj)
	}
	name, err := d.checkName(n.GetName())
	if err != nil {
		return nil, err
	}
	if m, ok := meta.(ProfileMeta); ok && m.VPPName != "" {
		name = m.VPPName
	}
	if err := validate(n); err != nil {
		return nil, err
	}
	if removed(o, n) {
		return nil, scheduler.ErrRecreate
	}
	if err := d.apply(ctx, name, o, n); err != nil {
		return nil, err
	}
	return ProfileMeta{VPPName: name}, nil
}

// Delete implements scheduler.Descriptor.
func (d *Profile) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*vpnpb.Ikev2Profile)
	if !ok {
		return typeErr(ProfileName, obj)
	}
	name := d.VPPName(o.GetName())
	if m, ok := meta.(ProfileMeta); ok && m.VPPName != "" {
		name = m.VPPName
	}
	if _, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2ProfileAddDel(ctx, &ikev2.Ikev2ProfileAddDel{Name: name, IsAdd: false}); err != nil {
		return fmt.Errorf("ikev2_profile_add_del (%s, del): %w", name, err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: every profile whose VPP name carries the owner prefix.
func (d *Profile) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2ProfileDump(ctx, &ikev2.Ikev2ProfileDump{})
	if err != nil {
		return nil, fmt.Errorf("ikev2_profile_dump: %w", err)
	}
	var dumped []ikev2_types.Ikev2Profile
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ikev2_profile_dump: %w", err)
		}
		p := det.Profile
		if !strings.HasPrefix(p.Name, d.cfg.Owner+"-") {
			vpn.Zero(p.Auth.Data) // another owner's PSK: never keep it
			continue
		}
		dumped = append(dumped, p)
	}
	var tbl *vpn.Interfaces
	for _, p := range dumped {
		if p.Responder.SwIfIndex != interface_types.InterfaceIndex(noInterface) || p.TunItf != noInterface {
			if tbl, err = vpn.DumpInterfaces(ctx, d.cfg.Client); err != nil {
				for i := range dumped {
					vpn.Zero(dumped[i].Auth.Data)
				}
				return nil, err
			}
			break
		}
	}
	out := make([]scheduler.KV, 0, len(dumped))
	for i := range dumped {
		v := decodeProfile(&dumped[i], strings.TrimPrefix(dumped[i].Name, d.cfg.Owner+"-"), tbl)
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: ProfileMeta{VPPName: dumped[i].Name}})
	}
	sortKVs(out)
	return out, nil
}

func (d *Profile) checkName(n string) (string, error) {
	if n == "" || strings.ContainsAny(n, "/\x00") {
		return "", fmt.Errorf("ikev2: profile name %q must be non-empty without '/' or NUL", n)
	}
	name := d.VPPName(n)
	if len(name) > 63 {
		return "", fmt.Errorf("ikev2: profile name %q is longer than 63 bytes", name)
	}
	return name, nil
}

// validate checks everything that can be checked without VPP.
func validate(o *vpnpb.Ikev2Profile) error {
	if a := o.GetAuth(); a != nil {
		switch a.GetMethod() {
		case AuthPSK:
			if a.GetPsk() == "" || a.GetCertFile() != "" {
				return errors.New("ikev2: auth psk needs psk (a secret reference) and no cert_file")
			}
		case AuthRSASig:
			if a.GetCertFile() == "" || a.GetPsk() != "" || len(a.GetCertFile()) > 1023 {
				return errors.New("ikev2: auth rsa-sig needs cert_file (< 1024 bytes) and no psk")
			}
		default:
			return fmt.Errorf("ikev2: auth method %q (want %s|%s)", a.GetMethod(), AuthPSK, AuthRSASig)
		}
	}
	for _, id := range []*vpnpb.Ikev2Id{o.GetLocalId(), o.GetRemoteId()} {
		if id == nil {
			continue
		}
		if _, err := encodeID(id); err != nil {
			return err
		}
	}
	for _, ts := range []*vpnpb.Ikev2Ts{o.GetLocalTs(), o.GetRemoteTs()} {
		if ts == nil {
			continue
		}
		if _, err := encodeTs(ts, false); err != nil {
			return err
		}
	}
	if r := o.GetResponder(); r != nil {
		a, err := vpn.ParseAddress(r.GetAddress())
		if err != nil {
			return fmt.Errorf("ikev2: responder needs an address (a hostname is ikev2.responder-hostname): %w", err)
		}
		if vpn.IsUnspecified(a) {
			return errors.New("ikev2: responder address must not be unspecified")
		}
	}
	if t := o.GetIke(); t != nil {
		if _, err := encodeIke(t); err != nil {
			return err
		}
	}
	if t := o.GetEsp(); t != nil {
		if _, err := encodeEsp(t); err != nil {
			return err
		}
	}
	if p := o.GetIpsecOverUdpPort(); p >= ipsecUDPPortNone {
		return fmt.Errorf("ikev2: ipsec_over_udp_port %d out of range", p)
	}
	return nil
}

// removed reports a part that is set in o but no longer in n (VPP cannot unset it).
func removed(o, n *vpnpb.Ikev2Profile) bool {
	gone := func(was, is bool) bool { return was && !is }
	return gone(o.GetAuth() != nil, n.GetAuth() != nil) ||
		gone(o.GetLocalId() != nil, n.GetLocalId() != nil) ||
		gone(o.GetRemoteId() != nil, n.GetRemoteId() != nil) ||
		gone(o.GetLocalTs() != nil, n.GetLocalTs() != nil) ||
		gone(o.GetRemoteTs() != nil, n.GetRemoteTs() != nil) ||
		gone(o.GetResponder() != nil, n.GetResponder() != nil) ||
		gone(o.GetIke() != nil, n.GetIke() != nil) ||
		gone(o.GetEsp() != nil, n.GetEsp() != nil) ||
		gone(o.GetLifetime() != nil, n.GetLifetime() != nil) ||
		gone(o.GetTunnelInterface() != "", n.GetTunnelInterface() != "") ||
		(o.GetUdpEncap() && !n.GetUdpEncap()) ||
		(o.GetNattDisabled() && !n.GetNattDisabled())
}

// apply issues the setters for every part of n that differs from o (o == nil: every set part).
func (d *Profile) apply(ctx context.Context, name string, o, n *vpnpb.Ikev2Profile) error {
	if o == nil {
		o = &vpnpb.Ikev2Profile{}
	}
	changed := func(a, b proto.Message, bSet bool) bool { return bSet && !proto.Equal(a, b) }
	svc := ikev2.NewServiceClient(d.cfg.Client)

	var tbl *vpn.Interfaces
	index := func(ifName string) (interface_types.InterfaceIndex, error) {
		if ifName == "" {
			return interface_types.InterfaceIndex(noInterface), nil
		}
		if tbl == nil {
			var err error
			if tbl, err = vpn.DumpInterfaces(ctx, d.cfg.Client); err != nil {
				return 0, err
			}
		}
		return tbl.Index(ifName)
	}

	if changed(o.GetAuth(), n.GetAuth(), n.GetAuth() != nil) {
		if err := d.setAuth(ctx, name, n.GetAuth()); err != nil {
			return err
		}
	}
	for _, p := range []struct {
		local    bool
		old, new *vpnpb.Ikev2Id
	}{{true, o.GetLocalId(), n.GetLocalId()}, {false, o.GetRemoteId(), n.GetRemoteId()}} {
		if !changed(p.old, p.new, p.new != nil) {
			continue
		}
		data, _ := encodeID(p.new) // validated
		typ, _ := idTypes.value(p.new.GetType())
		if _, err := svc.Ikev2ProfileSetID(ctx, &ikev2.Ikev2ProfileSetID{
			Name: name, IsLocal: p.local, IDType: typ, DataLen: uint32(len(data)), Data: data, //nolint:gosec // ≤ 64
		}); err != nil {
			return fmt.Errorf("ikev2_profile_set_id (%s, local=%v): %w", name, p.local, err)
		}
	}
	for _, p := range []struct {
		local    bool
		old, new *vpnpb.Ikev2Ts
	}{{true, o.GetLocalTs(), n.GetLocalTs()}, {false, o.GetRemoteTs(), n.GetRemoteTs()}} {
		if !changed(p.old, p.new, p.new != nil) {
			continue
		}
		ts, _ := encodeTs(p.new, p.local) // validated
		if _, err := svc.Ikev2ProfileSetTs(ctx, &ikev2.Ikev2ProfileSetTs{Name: name, Ts: ts}); err != nil {
			return fmt.Errorf("ikev2_profile_set_ts (%s, local=%v): %w", name, p.local, err)
		}
	}
	if r := n.GetResponder(); changed(o.GetResponder(), r, r != nil) {
		idx, err := index(r.GetInterface())
		if err != nil {
			return err
		}
		addr, _ := vpn.ParseAddress(r.GetAddress()) // validated
		if _, err := svc.Ikev2SetResponder(ctx, &ikev2.Ikev2SetResponder{Name: name, Responder: ikev2_types.Ikev2Responder{SwIfIndex: idx, Addr: addr}}); err != nil {
			return fmt.Errorf("ikev2_set_responder (%s): %w", name, err)
		}
	}
	if t := n.GetIke(); changed(o.GetIke(), t, t != nil) {
		tr, _ := encodeIke(t) // validated
		if _, err := svc.Ikev2SetIkeTransforms(ctx, &ikev2.Ikev2SetIkeTransforms{Name: name, Tr: tr}); err != nil {
			return fmt.Errorf("ikev2_set_ike_transforms (%s): %w", name, err)
		}
	}
	if t := n.GetEsp(); changed(o.GetEsp(), t, t != nil) {
		tr, _ := encodeEsp(t) // validated
		if _, err := svc.Ikev2SetEspTransforms(ctx, &ikev2.Ikev2SetEspTransforms{Name: name, Tr: tr}); err != nil {
			return fmt.Errorf("ikev2_set_esp_transforms (%s): %w", name, err)
		}
	}
	if l := n.GetLifetime(); changed(o.GetLifetime(), l, l != nil) {
		if _, err := svc.Ikev2SetSaLifetime(ctx, &ikev2.Ikev2SetSaLifetime{
			Name: name, Lifetime: l.GetSeconds(), LifetimeJitter: l.GetJitter(), Handover: l.GetHandover(), LifetimeMaxdata: l.GetMaxData(),
		}); err != nil {
			return fmt.Errorf("ikev2_set_sa_lifetime (%s): %w", name, err)
		}
	}
	if n.GetUdpEncap() && !o.GetUdpEncap() {
		if _, err := svc.Ikev2ProfileSetUDPEncap(ctx, &ikev2.Ikev2ProfileSetUDPEncap{Name: name}); err != nil {
			return fmt.Errorf("ikev2_profile_set_udp_encap (%s): %w", name, err)
		}
	}
	if op, np := o.GetIpsecOverUdpPort(), n.GetIpsecOverUdpPort(); op != np {
		if op != 0 { // VPP refuses to set a port while one is set: unset first
			if _, err := svc.Ikev2ProfileSetIpsecUDPPort(ctx, &ikev2.Ikev2ProfileSetIpsecUDPPort{Name: name, IsSet: 0, Port: uint16(op)}); err != nil { //nolint:gosec // validated
				return fmt.Errorf("ikev2_profile_set_ipsec_udp_port (%s, unset): %w", name, err)
			}
		}
		if np != 0 {
			if _, err := svc.Ikev2ProfileSetIpsecUDPPort(ctx, &ikev2.Ikev2ProfileSetIpsecUDPPort{Name: name, IsSet: 1, Port: uint16(np)}); err != nil { //nolint:gosec // validated
				return fmt.Errorf("ikev2_profile_set_ipsec_udp_port (%s): %w", name, err)
			}
		}
	}
	if ti := n.GetTunnelInterface(); ti != "" && ti != o.GetTunnelInterface() {
		idx, err := index(ti)
		if err != nil {
			return err
		}
		if _, err := svc.Ikev2SetTunnelInterface(ctx, &ikev2.Ikev2SetTunnelInterface{Name: name, SwIfIndex: idx}); err != nil {
			return fmt.Errorf("ikev2_set_tunnel_interface (%s): %w", name, err)
		}
	}
	if n.GetNattDisabled() && !o.GetNattDisabled() {
		if _, err := svc.Ikev2ProfileDisableNatt(ctx, &ikev2.Ikev2ProfileDisableNatt{Name: name}); err != nil {
			return fmt.Errorf("ikev2_profile_disable_natt (%s): %w", name, err)
		}
	}
	return nil
}

// setAuth resolves the PSK (or takes the certificate path) and issues ikev2_profile_set_auth. The
// request buffer is zeroed afterwards; errors never include it.
func (d *Profile) setAuth(ctx context.Context, name string, a *vpnpb.Ikev2Auth) error {
	var method uint8
	var data []byte
	switch a.GetMethod() {
	case AuthPSK:
		method = authSharedKey
		mat, err := vpn.Resolve(ctx, d.cfg.Secrets, a.GetPsk())
		if err != nil {
			return fmt.Errorf("ikev2: profile %s psk: %w", name, err)
		}
		if len(mat) == 0 || len(mat) > 1024 {
			vpn.Zero(mat)
			return fmt.Errorf("ikev2: profile %s psk must be 1–1024 bytes", name)
		}
		data = mat
	default:
		method = authRSASig
		data = []byte(a.GetCertFile())
	}
	defer vpn.Zero(data)
	if _, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2ProfileSetAuth(ctx, &ikev2.Ikev2ProfileSetAuth{
		Name: name, AuthMethod: method, DataLen: uint32(len(data)), Data: data, //nolint:gosec // ≤ 1024
	}); err != nil {
		return fmt.Errorf("ikev2_profile_set_auth (%s, %s): %w", name, a.GetMethod(), err)
	}
	return nil
}

// ---- encoders / decoders --------------------------------------------------------------------

// encodeID returns the id payload VPP stores: 4/16 address bytes for ip4/ip6, the text otherwise.
func encodeID(id *vpnpb.Ikev2Id) ([]byte, error) {
	typ, err := idTypes.value(id.GetType())
	if err != nil {
		return nil, err
	}
	var data []byte
	switch typ {
	case idIP4, idIP6:
		a, err := netip.ParseAddr(id.GetValue())
		if err != nil {
			return nil, fmt.Errorf("ikev2: id %s %q: %w", id.GetType(), id.GetValue(), err)
		}
		a = a.Unmap()
		if (typ == idIP4) != a.Is4() {
			return nil, fmt.Errorf("ikev2: id %s %q: wrong address family", id.GetType(), id.GetValue())
		}
		data = a.AsSlice()
		if z := bytes.IndexByte(data, 0); z >= 0 && strings.Trim(string(data[z:]), "\x00") != "" {
			return nil, fmt.Errorf("ikev2: id %s %q cannot be retrieved exactly: VPP dumps id data as string[64] and the "+
				"decoder stops at the first zero byte (10.4.0.1 → 10.4); use an address without a zero byte "+
				"followed by a non-zero byte, or an fqdn id", id.GetType(), id.GetValue())
		}
	case idKeyID:
		return nil, errors.New("ikev2: id type key-id is not supported by VPP 26.06 (ikev2_profile_set_id accepts ip4, ip6, fqdn, rfc822)")
	default:
		data = []byte(id.GetValue())
	}
	if len(data) == 0 || len(data) > 63 || bytes.IndexByte(data, 0) >= 0 && typ != idIP4 && typ != idIP6 {
		return nil, fmt.Errorf("ikev2: id %s value must be 1–63 bytes without NUL", id.GetType())
	}
	return data, nil
}

// decodeID is the inverse of encodeID. govpp cuts id.data at the first NUL; encodeID admits only
// addresses whose bytes after the first zero are all zero, so zero-filling to data_len is exact.
func decodeID(id ikev2_types.Ikev2ID) *vpnpb.Ikev2Id {
	if id.Type == 0 {
		return nil
	}
	data := []byte(id.Data)
	if int(id.DataLen) < len(data) {
		data = data[:id.DataLen]
	}
	out := &vpnpb.Ikev2Id{Type: idTypes.name(id.Type)}
	switch id.Type {
	case idIP4, idIP6:
		want := 4
		if id.Type == idIP6 {
			want = 16
		}
		full := make([]byte, want)
		copy(full, data)
		a, _ := netip.AddrFromSlice(full)
		out.Value = a.String()
	default:
		out.Value = string(data)
	}
	return out
}

func encodeTs(t *vpnpb.Ikev2Ts, local bool) (ikev2_types.Ikev2Ts, error) {
	var ts ikev2_types.Ikev2Ts
	if t.GetProtocol() > 255 || t.GetStartPort() > 65535 || t.GetEndPort() > 65535 {
		return ts, errors.New("ikev2: ts protocol ≤ 255 and ports ≤ 65535")
	}
	start, err := vpn.ParseAddress(t.GetStartAddr())
	if err != nil {
		return ts, err
	}
	end, err := vpn.ParseAddress(t.GetEndAddr())
	if err != nil {
		return ts, err
	}
	if start.Af != end.Af {
		return ts, errors.New("ikev2: ts start and end address families differ")
	}
	return ikev2_types.Ikev2Ts{
		IsLocal: local, ProtocolID: uint8(t.GetProtocol()), //nolint:gosec // checked
		StartPort: uint16(t.GetStartPort()), EndPort: uint16(t.GetEndPort()), //nolint:gosec // checked
		StartAddr: start, EndAddr: end,
	}, nil
}

// decodeTs returns nil for VPP's untouched (all-zero) selector.
func decodeTs(t ikev2_types.Ikev2Ts) *vpnpb.Ikev2Ts {
	if t.ProtocolID == 0 && t.StartPort == 0 && t.EndPort == 0 && vpn.IsUnspecified(t.StartAddr) && vpn.IsUnspecified(t.EndAddr) {
		return nil
	}
	return &vpnpb.Ikev2Ts{
		Protocol: uint32(t.ProtocolID), StartPort: uint32(t.StartPort), EndPort: uint32(t.EndPort),
		StartAddr: vpn.AddressString(t.StartAddr), EndAddr: vpn.AddressString(t.EndAddr),
	}
}

func encodeIke(t *vpnpb.Ikev2IkeTransforms) (ikev2_types.Ikev2IkeTransforms, error) {
	var tr ikev2_types.Ikev2IkeTransforms
	var err error
	if tr.CryptoAlg, err = encrAlgs.value(t.GetCryptoAlg()); err != nil {
		return tr, err
	}
	if tr.IntegAlg, err = integAlgs.value(t.GetIntegAlg()); err != nil {
		return tr, err
	}
	if tr.PrfAlg, err = prfAlgs.value(t.GetPrfAlg()); err != nil {
		return tr, err
	}
	if tr.DhGroup, err = dhGroups.value(t.GetDhGroup()); err != nil {
		return tr, err
	}
	tr.CryptoKeySize = t.GetCryptoKeySize()
	return tr, nil
}

func decodeIke(t ikev2_types.Ikev2IkeTransforms) *vpnpb.Ikev2IkeTransforms {
	if t == (ikev2_types.Ikev2IkeTransforms{}) {
		return nil
	}
	return &vpnpb.Ikev2IkeTransforms{
		CryptoAlg: encrAlgs.name(t.CryptoAlg), CryptoKeySize: t.CryptoKeySize, IntegAlg: integAlgs.name(t.IntegAlg),
		PrfAlg: prfAlgs.name(t.PrfAlg), DhGroup: dhGroups.name(t.DhGroup),
	}
}

func encodeEsp(t *vpnpb.Ikev2EspTransforms) (ikev2_types.Ikev2EspTransforms, error) {
	var tr ikev2_types.Ikev2EspTransforms
	var err error
	if tr.CryptoAlg, err = encrAlgs.value(t.GetCryptoAlg()); err != nil {
		return tr, err
	}
	if tr.IntegAlg, err = integAlgs.value(t.GetIntegAlg()); err != nil {
		return tr, err
	}
	tr.CryptoKeySize = t.GetCryptoKeySize()
	return tr, nil
}

func decodeEsp(t ikev2_types.Ikev2EspTransforms) *vpnpb.Ikev2EspTransforms {
	if t == (ikev2_types.Ikev2EspTransforms{}) {
		return nil
	}
	return &vpnpb.Ikev2EspTransforms{CryptoAlg: encrAlgs.name(t.CryptoAlg), CryptoKeySize: t.CryptoKeySize, IntegAlg: integAlgs.name(t.IntegAlg)}
}

// decodeProfile turns a dumped profile into the desired shape. The PSK is hashed into its
// reference and the dump buffer zeroed before this function returns.
func decodeProfile(p *ikev2_types.Ikev2Profile, name string, tbl *vpn.Interfaces) *vpnpb.Ikev2Profile {
	defer vpn.Zero(p.Auth.Data)
	v := &vpnpb.Ikev2Profile{
		Name:         name,
		LocalId:      decodeID(p.LocID),
		RemoteId:     decodeID(p.RemID),
		LocalTs:      decodeTs(p.LocTs),
		RemoteTs:     decodeTs(p.RemTs),
		Ike:          decodeIke(p.IkeTs),
		Esp:          decodeEsp(p.EspTs),
		UdpEncap:     p.UDPEncap,
		NattDisabled: p.NattDisabled,
	}
	data := p.Auth.Data
	if int(p.Auth.DataLen) < len(data) {
		data = data[:p.Auth.DataLen]
	}
	switch p.Auth.Method {
	case authSharedKey:
		v.Auth = &vpnpb.Ikev2Auth{Method: AuthPSK, Psk: vpn.Ref(data)}
	case authRSASig:
		v.Auth = &vpnpb.Ikev2Auth{Method: AuthRSASig, CertFile: string(bytes.TrimRight(data, "\x00"))}
	}
	if p.Lifetime != 0 || p.LifetimeJitter != 0 || p.Handover != 0 || p.LifetimeMaxdata != 0 {
		v.Lifetime = &vpnpb.Ikev2Lifetime{Seconds: p.Lifetime, Jitter: p.LifetimeJitter, Handover: p.Handover, MaxData: p.LifetimeMaxdata}
	}
	if p.IpsecOverUDPPort != ipsecUDPPortNone {
		v.IpsecOverUdpPort = uint32(p.IpsecOverUDPPort)
	}
	if p.TunItf != noInterface && tbl != nil {
		v.TunnelInterface = tbl.Name(p.TunItf)
	}
	// the responder is reported when VPP has an address for it; a hostname responder (address
	// unspecified, only the interface set by ikev2_set_responder_hostname) belongs to the write-only
	// ikev2.responder-hostname descriptor and is not part of the profile's value
	if !vpn.IsUnspecified(p.Responder.Addr) {
		v.Responder = &vpnpb.Ikev2Responder{Address: vpn.AddressString(p.Responder.Addr)}
		if idx := uint32(p.Responder.SwIfIndex); idx != noInterface && tbl != nil {
			v.Responder.Interface = tbl.Name(idx)
		}
	}
	return v
}

func sortKVs(kvs []scheduler.KV) {
	for i := 1; i < len(kvs); i++ {
		for j := i; j > 0 && kvs[j-1].Key > kvs[j].Key; j-- {
			kvs[j-1], kvs[j] = kvs[j], kvs[j-1]
		}
	}
}
