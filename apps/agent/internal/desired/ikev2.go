package desired

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/ipip"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// IKEv2Env resolves configuration references to keyed descriptor references. No
// plaintext key enters the desired object or its persisted metadata.
type IKEv2Env struct {
	SecretRef  func(context.Context, string) (string, error)
	CheckReady func(context.Context) error
}

func IKEv2(s Sink, ds *ngfwv1.DesiredState, in map[string]bool, env IKEv2Env) {
	if !in["vpn"] {
		return
	}
	ip := ds.GetVpn().GetIpsec()
	// Validate ownership across the entire native set before resolving secrets or
	// projecting any profile. Direct agent RPCs also require the API's invariants.
	ipipUsers := map[string]string{}
	type peerTuple struct{ vrf, local, remote string }
	peerUsers := map[peerTuple]string{}
	conflicting := false
	for _, name := range sortedKeys(ip.GetTunnels()) {
		t := ip.GetTunnels()[name]
		if !tunnelEnabled(t) {
			continue
		}
		pt := Ptr("vpn", "ipsec", "tunnels", name)
		if itf := t.GetRouteBased().GetIpipInterface(); itf != "" {
			if previous, exists := ipipUsers[itf]; exists {
				s.Errorf(pt+"/routeBased/ipipInterface", "vpn.route-based-ipip", "IPIP tunnel %q is already protected by IPsec tunnel %q", itf, previous)
				conflicting = true
			} else {
				ipipUsers[itf] = name
			}
		}
		key := peerTuple{vrfName(t.GetUnderlayVrf()), t.GetLocalAddr(), t.GetRemoteAddr()}
		if previous, exists := peerUsers[key]; exists {
			s.Errorf(pt+"/remoteAddr", "vpn.ipsec-peer-unique", "native peer endpoints are already used by IPsec tunnel %q", previous)
			conflicting = true
		} else {
			peerUsers[key] = name
		}
	}
	if conflicting {
		return
	}
	for _, name := range sortedKeys(ip.GetTunnels()) {
		t := ip.GetTunnels()[name]
		if !tunnelEnabled(t) {
			continue
		}
		pt := Ptr("vpn", "ipsec", "tunnels", name)
		fail := func(field, msg string) { s.Errorf(pt+"/"+field, "vpn.ipsec-native", "%s", msg) }
		if env.CheckReady != nil {
			if err := env.CheckReady(context.Background()); err != nil {
				fail("engine", err.Error())
				continue
			}
		}
		if t.GetEngine() != "vpp-ikev2" {
			fail("engine", "IPsec supports only the native VPP route-based engine")
			continue
		}
		if t.GetIkeVersion() != 0 && t.GetIkeVersion() != 2 {
			fail("ikeVersion", "native IPsec supports IKEv2 only")
			continue
		}
		if (t.GetMode() != "" && t.GetMode() != "tunnel") || (t.GetProtocol() != "" && t.GetProtocol() != "esp") {
			fail("mode", "native route-based IPsec requires tunnel-mode ESP")
			continue
		}
		tn := ds.GetTunnels().GetIpip()[t.GetRouteBased().GetIpipInterface()]
		if tn == nil || tn.Instance == nil {
			fail("routeBased/ipipInterface", "a configured IPIP tunnel with an instance is required")
			continue
		}
		if tn.GetSrc() != t.GetLocalAddr() || tn.GetDst() != t.GetRemoteAddr() || vrfName(tn.GetVrf()) != vrfName(t.GetVrf()) || vrfName(tn.GetUnderlayVrf()) != vrfName(t.GetUnderlayVrf()) {
			fail("routeBased/ipipInterface", "IPIP endpoints and overlay/underlay VRFs must match the IPsec tunnel")
			continue
		}
		if _, err := netip.ParseAddr(t.GetRemoteAddr()); err != nil {
			fail("remoteAddr", "native route-based IPsec requires a fixed IP peer; hostname and wildcard peers are unavailable")
			continue
		}
		local, localErr := netip.ParseAddr(t.GetLocalAddr())
		remote, remoteErr := netip.ParseAddr(t.GetRemoteAddr())
		if localErr != nil || remoteErr != nil || !local.Is4() || !remote.Is4() {
			fail("localAddr", "native IPsec currently supports IPv4 underlay only")
			continue
		}
		if vrfName(t.GetUnderlayVrf()) != "default" {
			fail("underlayVrf", "native IPsec currently supports the default underlay VRF only")
			continue
		}
		if t.GetDpd() != nil {
			fail("dpd", "per-tunnel DPD is unavailable; native VPP uses global liveness defaults")
			continue
		}
		if r := t.GetRekey(); r != nil && r.IkeSec != nil {
			fail("rekey/ikeSec", "IKE lifetime is unavailable; only CHILD lifetime is configurable")
			continue
		}
		if len(tn.GetIpv4())+len(tn.GetIpv6()) == 0 {
			fail("routeBased/ipipInterface", "the protected IPIP interface requires an overlay address to enable IP forwarding")
			continue
		}
		if t.GetStartAction() != "" && t.GetStartAction() != "none" {
			fail("startAction", "native initiation is an explicit runtime action; use none")
			continue
		}
		if t.GetMobike() || t.GetEsn() || t.GetFragmentation() != "" || t.GetRekey().GetReauth() || t.GetRekey().GetEspPackets() != 0 {
			fail("proposal", "MOBIKE, ESN, explicit fragmentation, reauthentication and packet lifetimes are unavailable in the native API")
			continue
		}
		p := ip.GetProposals()[t.GetProposal()]
		if p == nil {
			fail("proposal", "proposal does not exist")
			continue
		}
		v, err := nativeProfile(name, t, p, ipip.InterfaceName(tn.GetInstance()))
		if err != nil {
			fail("proposal", err.Error())
			continue
		}
		for _, iname := range sortedKeys(ds.GetInterfaces()) {
			i := ds.GetInterfaces()[iname]
			if vrfName(i.GetVrf()) != vrfName(t.GetUnderlayVrf()) {
				continue
			}
			for _, cidr := range append(append([]string{}, i.GetIpv4()...), i.GetIpv6()...) {
				prefix, e := netip.ParsePrefix(cidr)
				if e == nil && prefix.Addr().String() == t.GetLocalAddr() {
					v.Responder = &vpnpb.Ikev2Responder{Interface: iname, Address: t.GetRemoteAddr()}
				}
			}
		}
		if t.GetAuth().GetMethod() != "psk" {
			fail("auth", "native certificate authentication requires peer-certificate trust mapping and plugin-global private-key provisioning, which are unavailable")
			continue
		}
		if env.SecretRef == nil {
			fail("auth/secretRef", "native IPsec secret resolver is unavailable")
			continue
		}
		ref, err := env.SecretRef(context.Background(), t.GetAuth().GetSecretRef())
		if err != nil {
			fail("auth/secretRef", "native IPsec secret reference cannot be resolved")
			continue
		}
		v.Auth = &vpnpb.Ikev2Auth{Method: ikev2.AuthPSK, Psk: ref}
		if err := ikev2.ValidateProfile(v); err != nil {
			fail("localId", err.Error())
			continue
		}
		doc := &ngfwv1.DesiredState{Vpn: &ngfwv1.VpnConfig{Ipsec: &ngfwv1.IpsecConfig{Tunnels: map[string]*ngfwv1.IpsecTunnel{name: t}, Proposals: map[string]*ngfwv1.IpsecProposal{t.GetProposal(): p}}}}
		raw, _ := protojson.Marshal(doc)
		profile, _ := protojson.Marshal(v)
		s.Add(scheduler.Join(ikev2.ProfileName, name), v, pt)
		s.Add(ikev2.MetaKey(name), ikev2.MetaValue(ikev2.MetaSpec{ID: name, Name: name, Document: string(raw), Profile: string(profile)}), pt)
	}
}

func nativeProfile(name string, t *ngfwv1.IpsecTunnel, p *ngfwv1.IpsecProposal, itf string) (*vpnpb.Ikev2Profile, error) {
	ikeAlg, ikeBits, err := nativeEncryption(p.GetIke().GetEncr())
	if err != nil {
		return nil, err
	}
	espAlg, espBits, err := nativeEncryption(p.GetEsp().GetEncr())
	if err != nil {
		return nil, err
	}
	integ := map[string]string{"": "none", "sha1": "sha1-96", "sha256": "hmac-sha2-256-128", "sha384": "hmac-sha2-384-192", "sha512": "hmac-sha2-512-256", "md5": "md5-96", "aesxcbc": "aes-xcbc-96", "aescmac": "cmac-96"}
	prfs := map[string]string{"prfsha1": "hmac-sha1", "prfsha256": "hmac-sha2-256", "prfsha384": "hmac-sha2-384", "prfsha512": "hmac-sha2-512", "prfmd5": "hmac-md5", "prfaesxcbc": "aes128-xcbc", "prfaescmac": "aes128-cmac"}
	prf := p.GetIke().GetPrf()
	if prf == "" {
		prf = "prf" + p.GetIke().GetInteg()
	}
	if prfs[prf] == "" {
		return nil, fmt.Errorf("native IKE requires a supported PRF")
	}
	dh := p.GetIke().GetDh()
	if strings.HasPrefix(dh, "modp") {
		dh = "modp-" + strings.TrimPrefix(dh, "modp")
	} else if strings.HasPrefix(dh, "ecp") {
		dh = "ecp-" + strings.TrimPrefix(dh, "ecp")
	}
	if p.GetEsp().GetDh() != "" {
		return nil, fmt.Errorf("native ESP API does not configure a separate PFS group")
	}
	local, err := nativeIdentity(t.GetLocalId(), t.GetLocalAddr())
	if err != nil {
		return nil, err
	}
	remote, err := nativeIdentity(t.GetRemoteId(), t.GetRemoteAddr())
	if err != nil {
		return nil, err
	}
	lts, err := nativeTS(t.GetLocalTs(), t.GetLocalAddr())
	if err != nil {
		return nil, err
	}
	rts, err := nativeTS(t.GetRemoteTs(), t.GetRemoteAddr())
	if err != nil {
		return nil, err
	}
	life := uint64(t.GetRekey().GetEspSec())
	if life == 0 {
		life = 3600
	}
	return &vpnpb.Ikev2Profile{Name: name, TunnelInterface: itf, LocalId: local, RemoteId: remote, LocalTs: lts, RemoteTs: rts,
		Ike:      &vpnpb.Ikev2IkeTransforms{CryptoAlg: ikeAlg, CryptoKeySize: ikeBits, IntegAlg: integ[p.GetIke().GetInteg()], PrfAlg: prfs[prf], DhGroup: dh},
		Esp:      &vpnpb.Ikev2EspTransforms{CryptoAlg: espAlg, CryptoKeySize: espBits, IntegAlg: integ[p.GetEsp().GetInteg()]},
		Lifetime: &vpnpb.Ikev2Lifetime{Seconds: life, Handover: 10, MaxData: t.GetRekey().GetEspBytes()}, NattDisabled: t.NatT != nil && !t.GetNatT()}, nil
}

func nativeEncryption(s string) (string, uint32, error) {
	for _, bits := range []uint32{128, 192, 256} {
		base := fmt.Sprintf("aes%d", bits)
		switch s {
		case base:
			return "aes-cbc", bits, nil
		case base + "ctr":
			return "aes-ctr", bits, nil
		case base + "gcm16":
			return "aes-gcm-16", bits, nil
		}
	}
	if s == "chacha20poly1305" {
		return "chacha20-poly1305", 256, nil
	}
	if s == "3des" {
		return "3des", 192, nil
	}
	if s == "null" {
		return "null", 0, nil
	}
	return "", 0, fmt.Errorf("unsupported native encryption transform")
}
func nativeIdentity(id, fallback string) (*vpnpb.Ikev2Id, error) {
	if id == "" {
		id = fallback
	}
	typ := "fqdn"
	if strings.HasPrefix(id, "@") {
		id = strings.TrimPrefix(id, "@")
	} else if a, e := netip.ParseAddr(id); e == nil {
		typ = "ip6"
		if a.Is4() {
			typ = "ip4"
		}
	} else if strings.Contains(id, "@") {
		typ = "rfc822"
	}
	return &vpnpb.Ikev2Id{Type: typ, Value: id}, nil
}
func nativeTS(ts []string, addr string) (*vpnpb.Ikev2Ts, error) {
	if len(ts) > 1 {
		return nil, fmt.Errorf("native VPP profile supports one selector per direction")
	}
	cidr := "0.0.0.0/0"
	if a, e := netip.ParseAddr(addr); e == nil && a.Is6() {
		cidr = "::/0"
	}
	if len(ts) == 1 {
		cidr = ts[0]
	}
	p, e := netip.ParsePrefix(cidr)
	if e != nil {
		return nil, fmt.Errorf("invalid traffic selector")
	}
	p = p.Masked()
	end := p.Addr().AsSlice()
	for bit := p.Bits(); bit < len(end)*8; bit++ {
		end[bit/8] |= 1 << uint(7-bit%8)
	}
	a, _ := netip.AddrFromSlice(end)
	return &vpnpb.Ikev2Ts{StartAddr: p.Addr().String(), EndAddr: a.String(), EndPort: 65535}, nil
}

// AssembleIKEv2 joins persisted configuration references to actual live profiles.
// Missing or changed VPP objects remain absent so drift cannot be hidden by metadata.
func AssembleIKEv2(ds *ngfwv1.DesiredState, kvs []scheduler.KV) {
	live := map[string]*vpnpb.Ikev2Profile{}
	for _, kv := range kvs {
		if v, ok := kv.Value.(*vpnpb.Ikev2Profile); ok {
			live[v.GetName()] = v
		}
	}
	for _, kv := range kvs {
		if !strings.HasPrefix(string(kv.Key), ikev2.MetaName+"/") {
			continue
		}
		m, e := ikev2.DecodeMeta(kv.Value)
		if e != nil {
			continue
		}
		expected := &vpnpb.Ikev2Profile{}
		if protojson.Unmarshal([]byte(m.Profile), expected) != nil || !proto.Equal(expected, live[m.Name]) {
			continue
		}
		doc := &ngfwv1.DesiredState{}
		if protojson.Unmarshal([]byte(m.Document), doc) != nil {
			continue
		}
		if ds.Vpn == nil {
			ds.Vpn = &ngfwv1.VpnConfig{}
		}
		if ds.Vpn.Ipsec == nil {
			ds.Vpn.Ipsec = &ngfwv1.IpsecConfig{}
		}
		ip := ds.Vpn.Ipsec
		if ip.Tunnels == nil {
			ip.Tunnels = map[string]*ngfwv1.IpsecTunnel{}
		}
		if ip.Proposals == nil {
			ip.Proposals = map[string]*ngfwv1.IpsecProposal{}
		}
		for n, t := range doc.GetVpn().GetIpsec().GetTunnels() {
			ip.Tunnels[n] = t
		}
		for n, p := range doc.GetVpn().GetIpsec().GetProposals() {
			ip.Proposals[n] = p
		}
	}
}

func tunnelEnabled(t *ngfwv1.IpsecTunnel) bool {
	return t != nil && (t.Enabled == nil || t.GetEnabled())
}
