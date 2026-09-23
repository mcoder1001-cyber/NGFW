package strongswan

import (
	"cmp"
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Model is the typed, validated view of what the templates render. Every string in it has
// passed its validator (names, identities, addresses, selectors, proposals); secrets are held
// as bytes and only ever reach the secrets file as base64.
type Model struct {
	// Conns are the IKE connections, sorted by name.
	Conns []Conn
	// Secrets are the IKE pre-shared keys, sorted by name.
	Secrets []Secret
	// Pools are virtual-IP pools (remote access; not mapped from the document yet).
	Pools []Pool
	// Authorities are CA sections (F-pki-basic; rendered shape only).
	Authorities []Authority
}

// Conn is one `connections.<name>` section.
type Conn struct {
	// Name is the strongSwan section name (ConnName of the tunnel name).
	Name string
	// Description is rendered as a comment.
	Description string
	// Version is the IKE version (1 or 2).
	Version int
	// LocalAddrs / RemoteAddrs are IPs, hostnames or %any.
	LocalAddrs, RemoteAddrs []string
	// Proposals are IKE proposal keywords.
	Proposals []string
	// Local / Remote are the authentication rounds.
	Local, Remote Auth
	// DPDDelay is the DPD interval in seconds (0 = off); DPDTimeout the IKEv1 timeout.
	DPDDelay, DPDTimeout uint32
	// Mobike is "" (default), "yes" or "no".
	Mobike string
	// Encap forces UDP encapsulation.
	Encap bool
	// Fragmentation is "" (default) or yes|no|force|accept.
	Fragmentation string
	// RekeyTime / ReauthTime are IKE SA lifetimes in seconds (0 = not rendered);
	// NoRekey renders rekey_time = 0s (re-authentication instead of rekeying).
	RekeyTime, ReauthTime uint32
	NoRekey               bool
	// Pools are names of `pools` sections assigned to clients.
	Pools []string
	// Children are the CHILD_SA configs, sorted by name.
	Children []Child
}

// Auth is one local or remote authentication round.
type Auth struct {
	// Method is "psk" or "pubkey".
	Method string
	// ID is the identity ("" = not rendered).
	ID string
	// Certs / CACerts are file names in x509/ and x509ca/ (pubkey auth; F-pki-basic).
	Certs, CACerts []string
}

// Child is one `children.<name>` section.
type Child struct {
	Name string
	// LocalTS / RemoteTS are masked CIDRs (empty = dynamic).
	LocalTS, RemoteTS []string
	// ESPProposals / AHProposals are child proposal keywords (exactly one list is set).
	ESPProposals, AHProposals []string
	// Mode is "tunnel" or "transport".
	Mode string
	// StartAction / CloseAction are none|trap|start ("" = none, not rendered);
	// DPDAction is clear|trap|restart ("" = not rendered).
	StartAction, CloseAction, DPDAction string
	// RekeyTime in seconds, RekeyBytes / RekeyPackets (0 = not rendered).
	RekeyTime                uint32
	RekeyBytes, RekeyPackets uint64
	// ReplayWindow is rendered when set (0 disables anti-replay).
	ReplayWindow *uint32
	// IfIDIn / IfIDOut are XFRM interface ids / kernel-vpp tunnel ids (0 = not rendered).
	IfIDIn, IfIDOut uint32
}

// Secret is one `secrets.ike-<conn>` section.
type Secret struct {
	// Name is the section name ("ike-" + conn name); also the VICI unique id.
	Name string
	// Ref is the D-051 reference it was resolved from (never rendered).
	Ref string
	// IDs are the owner identities (id-local, id-remote).
	LocalID, RemoteID string
	// Value is the plaintext; the template sees only its base64 form.
	Value []byte
}

// Pool is one `pools.<name>` section.
type Pool struct {
	Name  string
	Addrs string   // CIDR
	DNS   []string // addresses
}

// Authority is one `authorities.<name>` section.
type Authority struct {
	Name   string
	CACert string // file name in x509ca/
}

// IfIDMapper maps a route-based tunnel's ipip interface name to the (in, out) if_id pair
// charon hands to the kernel plugin. P11 provides it (kernel-vpp); without one, route-based
// tunnels are rejected.
type IfIDMapper func(ipipInterface string) (in, out uint32, ok bool)

// Desired extracts the typed desired state from a *vrxv1.DesiredState or (D-055 stand-in) a
// *structpb.Struct holding the configuration document.
func Desired(msg proto.Message) (*vrxv1.DesiredState, error) {
	switch m := msg.(type) {
	case nil:
		return &vrxv1.DesiredState{}, nil
	case *vrxv1.DesiredState:
		if m == nil {
			return &vrxv1.DesiredState{}, nil
		}
		return m, nil
	case *structpb.Struct:
		if m == nil {
			return &vrxv1.DesiredState{}, nil
		}
		raw, err := protojson.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("%w: encode document: %v", ErrInput, err)
		}
		ds := &vrxv1.DesiredState{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, ds); err != nil {
			return nil, fmt.Errorf("%w: decode document: %v", ErrInput, err)
		}
		return ds, nil
	default:
		return nil, fmt.Errorf("%w: unsupported input type %T (want *vrxv1.DesiredState or *structpb.Struct)", ErrInput, msg)
	}
}

// buildOptions are the renderer inputs BuildModel needs besides the document.
type buildOptions struct {
	resolve func(ctx context.Context, ref string) ([]byte, error)
	ifID    IfIDMapper
}

// BuildModel validates vpn.ipsec and returns the model. Tunnels with enabled=false or
// engine "vpp-ikev2" are not rendered (the latter is a VPP descriptor's). Secrets are resolved
// through resolve; errors never contain secret values.
func BuildModel(ctx context.Context, ds *vrxv1.DesiredState, opt buildOptions) (*Model, error) {
	ipsec := ds.GetVpn().GetIpsec()
	m := &Model{}
	names := slices.Sorted(mapKeys(ipsec.GetTunnels()))
	type idPair struct{ l, r string }
	pskOwner := map[idPair]string{}
	for _, name := range names {
		t := ipsec.GetTunnels()[name]
		if t == nil || (t.Enabled != nil && !t.GetEnabled()) {
			continue
		}
		if e := t.GetEngine(); e != "" && e != "strongswan" {
			if e != "vpp-ikev2" {
				return nil, fmt.Errorf("%w: vpn.ipsec.tunnels.%s.engine %q is not strongswan|vpp-ikev2", ErrInput, name, clip(e))
			}
			continue
		}
		c, sec, err := buildConn(ctx, name, t, ipsec.GetProposals(), opt)
		if err != nil {
			return nil, err
		}
		if sec != nil {
			key := idPair{sec.LocalID, sec.RemoteID}
			if other, dup := pskOwner[key]; dup {
				return nil, fmt.Errorf("%w: vpn.ipsec.tunnels.%s and %s use the same identities (%s ↔ %s) with pre-shared keys; charon could not tell their keys apart", ErrInput, other, name, key.l, key.r)
			}
			pskOwner[key] = name
			m.Secrets = append(m.Secrets, *sec)
		}
		m.Conns = append(m.Conns, *c)
	}
	return m, m.normalize()
}

func mapKeys[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func buildConn(ctx context.Context, name string, t *vrxv1.IpsecTunnel, proposals map[string]*vrxv1.IpsecProposal, opt buildOptions) (*Conn, *Secret, error) {
	at := func(field string) string { return "vpn.ipsec.tunnels." + name + "." + field }
	errf := func(field, format string, a ...any) error {
		return fmt.Errorf("%w: %s: %s", ErrInput, at(field), fmt.Sprintf(format, a...))
	}
	cname, err := ConnName(name)
	if err != nil {
		return nil, nil, err
	}
	c := &Conn{Name: cname, Version: 2}
	if t.Description != nil {
		if len(t.GetDescription()) > 255 {
			return nil, nil, errf("description", "longer than 255 bytes")
		}
		c.Description = t.GetDescription()
	}
	if t.IkeVersion != nil {
		switch t.GetIkeVersion() {
		case 1, 2:
			c.Version = int(t.GetIkeVersion())
		default:
			return nil, nil, errf("ikeVersion", "%d is not 1|2", t.GetIkeVersion())
		}
	}

	// Addresses.
	local, err := netip.ParseAddr(t.GetLocalAddr())
	if err != nil || local.Zone() != "" {
		return nil, nil, errf("localAddr", "%q is not an IP address", clip(t.GetLocalAddr()))
	}
	remote, err := Host(t.GetRemoteAddr())
	if err != nil {
		return nil, nil, errf("remoteAddr", "%v", err)
	}
	remoteIP, remoteErr := netip.ParseAddr(remote)
	if remoteErr == nil {
		if remoteIP == local {
			return nil, nil, errf("remoteAddr", "equals localAddr")
		}
		if remoteIP.Is4() != local.Is4() {
			return nil, nil, errf("remoteAddr", "address family differs from localAddr")
		}
	}
	c.LocalAddrs, c.RemoteAddrs = []string{local.String()}, []string{remote}

	// Identities: explicit, else the addresses (an explicit remote id stops charon from
	// accepting any identity the peer presents).
	lid, rid := local.String(), "%any"
	if remoteErr == nil {
		rid = remoteIP.String()
	}
	if t.LocalId != nil {
		if lid, err = Identity(t.GetLocalId()); err != nil {
			return nil, nil, errf("localId", "%v", err)
		}
	}
	if t.RemoteId != nil {
		if rid, err = Identity(t.GetRemoteId()); err != nil {
			return nil, nil, errf("remoteId", "%v", err)
		}
	}
	c.Local.ID, c.Remote.ID = lid, rid
	if rid == "%any" {
		c.Remote.ID = ""
	}

	// Proposal.
	pname := t.GetProposal()
	p, ok := proposals[pname]
	if !ok || p == nil {
		return nil, nil, errf("proposal", "%q is not in vpn.ipsec.proposals", clip(pname))
	}
	ike, err := IKEProposal(p.GetIke().GetEncr(), p.GetIke().GetInteg(), p.GetIke().GetPrf(), p.GetIke().GetDh())
	if err != nil {
		return nil, nil, fmt.Errorf("vpn.ipsec.proposals.%s: %w", pname, err)
	}
	if c.Version == 1 && isAEAD(p.GetIke().GetEncr()) {
		return nil, nil, fmt.Errorf("%w: vpn.ipsec.proposals.%s: IKEv1 does not support AEAD ciphers in IKE (%s)", ErrInput, pname, p.GetIke().GetEncr())
	}
	c.Proposals = []string{ike}

	// Authentication.
	var sec *Secret
	auth := t.GetAuth()
	switch auth.GetMethod() {
	case "psk":
		ref := auth.GetSecretRef()
		if !secretRefRe.MatchString(ref) || !strings.HasPrefix(ref, "psk/") {
			// The value is not echoed: a malformed reference may be a pasted secret.
			return nil, nil, errf("auth.secretRef", "must be a psk/<name> reference (D-051); the value is not shown")
		}
		if opt.resolve == nil {
			return nil, nil, fmt.Errorf("%s: %w", at("auth.secretRef"), ErrNoSecretResolver)
		}
		val, err := opt.resolve(ctx, ref)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: resolve %s: %w", at("auth.secretRef"), ref, err)
		}
		if err := checkPSK(val); err != nil {
			return nil, nil, errf("auth.secretRef", "secret %s: %v", ref, err)
		}
		c.Local.Method, c.Remote.Method = "psk", "psk"
		sec = &Secret{Name: "ike-" + cname, Ref: ref, LocalID: lid, RemoteID: rid, Value: val}
	case "cert":
		cert := auth.GetCertificate()
		if !objectNameRe.MatchString(cert) {
			return nil, nil, errf("auth.certificate", "%q is not an object name", clip(cert))
		}
		c.Local.Method, c.Remote.Method = "pubkey", "pubkey"
		c.Local.Certs = []string{cert + ".pem"}
		if auth.RemoteCa != nil {
			ca := auth.GetRemoteCa()
			if !objectNameRe.MatchString(ca) {
				return nil, nil, errf("auth.remoteCa", "%q is not an object name", clip(ca))
			}
			c.Remote.CACerts = []string{ca + ".pem"}
		}
	default:
		return nil, nil, errf("auth.method", "%q is not psk|cert", clip(auth.GetMethod()))
	}

	// Connection options.
	if d := t.GetDpd(); d != nil && (d.Enabled == nil || d.GetEnabled()) {
		c.DPDDelay = orDefault(d.DelaySec, 30)
		if c.DPDDelay < 1 || c.DPDDelay > 86400 {
			return nil, nil, errf("dpd.delaySec", "%d not in 1..86400", c.DPDDelay)
		}
		if c.Version == 1 {
			c.DPDTimeout = orDefault(d.TimeoutSec, 150)
			if c.DPDTimeout < 1 || c.DPDTimeout > 86400 {
				return nil, nil, errf("dpd.timeoutSec", "%d not in 1..86400", c.DPDTimeout)
			}
		}
	}
	if t.Mobike != nil {
		if c.Version != 2 && t.GetMobike() {
			return nil, nil, errf("mobike", "MOBIKE requires IKEv2")
		}
		if c.Version == 2 {
			c.Mobike = yesNo(t.GetMobike())
		}
	}
	if t.Fragmentation != nil {
		if err := oneOf(at("fragmentation"), t.GetFragmentation(), []string{"yes", "no", "force", "accept"}); err != nil {
			return nil, nil, err
		}
		c.Fragmentation = t.GetFragmentation()
	}
	rk := t.GetRekey()
	if rk == nil {
		rk = &vrxv1.IpsecRekey{}
	}
	ikeLife := orDefault(rk.IkeSec, 14400)
	if ikeLife < 60 || ikeLife > 604800 {
		return nil, nil, errf("rekey.ikeSec", "%d not in 60..604800", ikeLife)
	}
	if rk.GetReauth() {
		c.ReauthTime, c.NoRekey = ikeLife, true
	} else {
		c.RekeyTime = ikeLife
	}

	// The one CHILD_SA of a document tunnel carries the tunnel's name.
	child, err := buildChild(name, cname, t, p, opt, errf)
	if err != nil {
		return nil, nil, err
	}
	c.Children = []Child{*child}
	return c, sec, nil
}

func buildChild(name, cname string, t *vrxv1.IpsecTunnel, p *vrxv1.IpsecProposal, opt buildOptions, errf func(field, format string, a ...any) error) (*Child, error) {
	ch := &Child{Name: cname, Mode: "tunnel"}
	if t.Mode != nil {
		if err := oneOf("mode", t.GetMode(), []string{"tunnel", "transport"}); err != nil {
			return nil, errf("mode", "%v", err)
		}
		ch.Mode = t.GetMode()
	}
	proto := "esp"
	if t.Protocol != nil {
		proto = t.GetProtocol()
	}
	esp, err := ChildProposal(proto, p.GetEsp().GetEncr(), p.GetEsp().GetInteg(), p.GetEsp().GetDh(), t.GetEsn())
	if err != nil {
		return nil, fmt.Errorf("vpn.ipsec.proposals.%s (tunnel %s): %w", t.GetProposal(), name, err)
	}
	if proto == "ah" {
		ch.AHProposals = []string{esp}
	} else {
		ch.ESPProposals = []string{esp}
	}
	for i, ts := range t.GetLocalTs() {
		s, err := TrafficSelector(ts)
		if err != nil {
			return nil, errf(fmt.Sprintf("localTs[%d]", i), "%v", err)
		}
		ch.LocalTS = append(ch.LocalTS, s)
	}
	for i, ts := range t.GetRemoteTs() {
		s, err := TrafficSelector(ts)
		if err != nil {
			return nil, errf(fmt.Sprintf("remoteTs[%d]", i), "%v", err)
		}
		ch.RemoteTS = append(ch.RemoteTS, s)
	}
	if len(ch.LocalTS) > 64 || len(ch.RemoteTS) > 64 {
		return nil, errf("localTs", "at most 64 traffic selectors per side")
	}
	if rb := t.GetRouteBased(); rb != nil {
		if ch.Mode != "tunnel" {
			return nil, errf("routeBased", "a route-based tunnel requires tunnel mode")
		}
		if opt.ifID == nil {
			return nil, errf("routeBased", "route-based tunnels need the if_id mapping of the kernel-vpp integration (P11); none is configured")
		}
		in, out, ok := opt.ifID(rb.GetIpipInterface())
		if !ok || in == 0 || out == 0 {
			return nil, errf("routeBased.ipipInterface", "%q has no if_id", clip(rb.GetIpipInterface()))
		}
		ch.IfIDIn, ch.IfIDOut = in, out
		if len(ch.LocalTS) == 0 {
			ch.LocalTS = []string{"0.0.0.0/0", "::/0"}
		}
		if len(ch.RemoteTS) == 0 {
			ch.RemoteTS = []string{"0.0.0.0/0", "::/0"}
		}
	} else if ch.Mode == "tunnel" && (len(ch.LocalTS) == 0 || len(ch.RemoteTS) == 0) {
		return nil, errf("localTs", "a policy-based tunnel needs at least one local and one remote traffic selector")
	}
	var err2 error
	if ch.StartAction, err2 = action(t.StartAction, "start"); err2 != nil {
		return nil, errf("startAction", "%v", err2)
	}
	if ch.CloseAction, err2 = action(t.CloseAction, "none"); err2 != nil {
		return nil, errf("closeAction", "%v", err2)
	}
	if t.GetRemoteAddr() == "%any" && (ch.StartAction == "start") {
		return nil, errf("startAction", "a tunnel to %%any cannot be initiated; use trap or none")
	}
	if d := t.GetDpd(); d != nil && (d.Enabled == nil || d.GetEnabled()) {
		a := "restart"
		if d.Action != nil {
			a = d.GetAction()
		}
		if err := oneOf("dpd.action", a, []string{"clear", "trap", "restart"}); err != nil {
			return nil, errf("dpd.action", "%v", err)
		}
		ch.DPDAction = a
	}
	rk := t.GetRekey()
	if rk == nil {
		rk = &vrxv1.IpsecRekey{}
	}
	ch.RekeyTime = orDefault(rk.EspSec, 3600)
	if ch.RekeyTime < 60 || ch.RekeyTime > 604800 {
		return nil, errf("rekey.espSec", "%d not in 60..604800", ch.RekeyTime)
	}
	ch.RekeyBytes, ch.RekeyPackets = rk.GetEspBytes(), rk.GetEspPackets()
	if t.AntiReplay != nil && !t.GetAntiReplay() {
		zero := uint32(0)
		ch.ReplayWindow = &zero
	}
	return ch, nil
}

func action(v *string, def string) (string, error) {
	a := def
	if v != nil {
		a = *v
	}
	if err := oneOf("action", a, []string{"none", "start", "trap"}); err != nil {
		return "", err
	}
	if a == "none" {
		return "", nil
	}
	return a, nil
}

func orDefault(v *uint32, def uint32) uint32 {
	if v == nil {
		return def
	}
	return *v
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// normalize sorts and validates the model — also for models built by hand (tests, P11).
func (m *Model) normalize() error {
	slices.SortFunc(m.Conns, func(a, b Conn) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(m.Secrets, func(a, b Secret) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(m.Pools, func(a, b Pool) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(m.Authorities, func(a, b Authority) int { return cmp.Compare(a.Name, b.Name) })
	seen := map[string]bool{}
	for _, c := range m.Conns {
		if _, err := SectionName(c.Name); err != nil {
			return fmt.Errorf("%w: connection: %v", ErrInput, err)
		}
		if seen["c/"+c.Name] {
			return fmt.Errorf("%w: duplicate connection %s", ErrInput, c.Name)
		}
		seen["c/"+c.Name] = true
		if len(c.Children) == 0 {
			return fmt.Errorf("%w: connection %s has no children", ErrInput, c.Name)
		}
		slices.SortFunc(c.Children, func(a, b Child) int { return cmp.Compare(a.Name, b.Name) })
		for i, ch := range c.Children {
			if _, err := SectionName(ch.Name); err != nil {
				return fmt.Errorf("%w: connection %s child: %v", ErrInput, c.Name, err)
			}
			if i > 0 && c.Children[i-1].Name == ch.Name {
				return fmt.Errorf("%w: connection %s: duplicate child %s", ErrInput, c.Name, ch.Name)
			}
		}
	}
	for _, s := range m.Secrets {
		if _, err := SectionName(s.Name); err != nil || !strings.HasPrefix(s.Name, "ike-") {
			return fmt.Errorf("%w: secret section %q must be ike-<name>", ErrInput, clip(s.Name))
		}
		if err := checkPSK(s.Value); err != nil {
			return fmt.Errorf("%w: secret %s: %v", ErrInput, s.Name, err)
		}
	}
	for _, p := range m.Pools {
		if _, err := SectionName(p.Name); err != nil {
			return fmt.Errorf("%w: pool: %v", ErrInput, err)
		}
		if _, err := netip.ParsePrefix(p.Addrs); err != nil {
			return fmt.Errorf("%w: pool %s addrs %q: %v", ErrInput, p.Name, clip(p.Addrs), err)
		}
		for _, d := range p.DNS {
			if _, err := netip.ParseAddr(d); err != nil {
				return fmt.Errorf("%w: pool %s dns %q: %v", ErrInput, p.Name, clip(d), err)
			}
		}
	}
	for _, a := range m.Authorities {
		if _, err := SectionName(a.Name); err != nil {
			return fmt.Errorf("%w: authority: %v", ErrInput, err)
		}
	}
	return nil
}
