package desired

import (
	"context"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"sort"
	"strings"
	"time"
)

type RAEnv struct {
	Owner     string
	IDs       TunnelIDSpan
	Ready     func(context.Context) error
	SecretRef func(context.Context, string) (string, error)
	VRF       func(string) (uint32, bool)
}

// Enabled profiles project an independent verified engine. Absent readiness
// retains the original hard refusal; disabled drafts require no credentials.
func RemoteAccess(s Sink, ds *ngfwv1.DesiredState, in map[string]bool, options ...RAEnv) {
	if !in["vpn"] {
		return
	}
	env := RAEnv{}
	if len(options) > 0 {
		env = options[0]
	}
	profiles := ds.GetVpn().GetRemoteAccess()
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	enabled := []string{}
	for _, n := range names {
		p := profiles[n]
		if p == nil {
			s.Errorf(Ptr("vpn", "remoteAccess", n), "vpn.remote-access-value", "remote-access profile is invalid")
			continue
		}
		if p.Enabled == nil || p.GetEnabled() {
			enabled = append(enabled, n)
		}
	}
	if len(enabled) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fail := func(name, leaf string) {
		s.Errorf(Ptr("vpn", "remoteAccess", name, leaf), "vpn.remote-access-native-capability", "remote-access VPN requires the verified independent engine, owned transport and sealed credential references")
	}
	if env.Ready == nil || env.Ready(ctx) != nil || env.SecretRef == nil || env.VRF == nil || env.Owner == "" {
		for _, n := range enabled {
			fail(n, "enabled")
		}
		return
	}
	lo, hi := env.IDs.Lo, env.IDs.Hi
	if env.IDs.All {
		lo, hi = 0, 8192
	}
	hi = min(hi, 8192)
	if lo > hi || uint64(len(enabled))*2 > uint64(hi)-uint64(lo)+1 {
		for _, n := range enabled {
			fail(n, "transport")
		}
		return
	}
	// Deterministic sorted allocation refuses explicit TAP instance collisions.
	used := map[uint32]bool{}
	for i, name := range enabled {
		p := profiles[name]
		proposal := ds.GetVpn().GetIpsec().GetProposals()[p.GetProposal()]
		cert := ds.GetVpn().GetPki().GetCertificates()[p.GetCertificate()]
		ca := ds.GetVpn().GetPki().GetCas()[p.GetClientCa()]
		if proposal == nil || cert == nil || cert.GetAcme() != nil {
			fail(name, "certificate")
			continue
		}
		outer, ok := env.VRF(p.GetUnderlayVrf())
		if !ok {
			fail(name, "underlayVrf")
			continue
		}
		inner, ok := env.VRF(p.GetVrf())
		if !ok {
			fail(name, "vrf")
			continue
		}
		outerID := lo + uint32(i*2)
		innerID := outerID + 1
		if used[outerID] || used[innerID] {
			fail(name, "transport")
			continue
		}
		used[outerID] = true
		used[innerID] = true
		spec := ravpn.EngineSpec{Owner: env.Owner, Profile: name, Instance: ravpn.InstanceID(env.Owner, name), Configuration: proto.Clone(p).(*ngfwv1.RemoteAccessProfile), Proposal: proto.Clone(proposal).(*ngfwv1.IpsecProposal), ServerCertificate: &ngfwv1.PkiCertificate{CertificateRef: cert.CertificateRef, PrivateKeyRef: cert.PrivateKeyRef, Ca: cert.Ca}, OuterID: outerID, InnerID: innerID, OuterTable: outer, InnerTable: inner, Fingerprints: map[string]string{}}
		refs := []string{cert.GetCertificateRef(), cert.GetPrivateKeyRef()}
		if p.GetAuth() == "eap-tls" || p.GetAuth() == "pubkey" {
			if ca == nil {
				fail(name, "clientCa")
				continue
			}
			spec.ClientCA = &ngfwv1.PkiCa{CertificateRef: ca.CertificateRef}
			refs = append(refs, ca.GetCertificateRef(), "cert/"+p.GetClientCa()+".crl")
		}
		for _, u := range p.GetUsers() {
			refs = append(refs, u.GetPasswordRef())
		}
		for _, server := range p.GetRadius().GetServers() {
			refs = append(refs, server.GetSecretRef())
		}
		valid := true
		for _, ref := range refs {
			f, e := env.SecretRef(ctx, ref)
			if e != nil || !strings.HasPrefix(f, "hmac:") {
				valid = false
				break
			}
			spec.Fingerprints[ref] = f
		}
		if !valid {
			fail(name, "users")
			continue
		}
		for _, policy := range []*ngfwv1.RemoteAccessPolicy{p.GetOuterPolicy(), p.GetAccessPolicy()} {
			for _, ref := range append(append([]string{}, policy.GetIngress()...), policy.GetEgress()...) {
				if ds.GetAcl().GetLists()[ref] == nil {
					valid = false
				}
			}
		}
		if !valid {
			fail(name, "accessPolicy")
			continue
		}
		value, e := spec.Proto()
		if e != nil {
			fail(name, "transport")
			continue
		}
		objects, e := ravpn.TransportObjects(spec)
		if e != nil {
			fail(name, "transport")
			continue
		}
		for _, kv := range objects {
			s.Add(kv.Key, kv.Value, Ptr("vpn", "remoteAccess", name))
		}
		s.Add(scheduler.Join(ravpn.EngineName, spec.Instance), value, Ptr("vpn", "remoteAccess", name))
	}
}
func AssembleRA(ds *ngfwv1.DesiredState, kvs []scheduler.KV, in map[string]bool) {
	if !in["vpn"] {
		return
	}
	for _, kv := range kvs {
		if kv.Key.Descriptor() != ravpn.EngineName {
			continue
		}
		v, _ := kv.Value.(*structpb.Struct)
		spec, e := ravpn.DecodeEngine(v)
		if e != nil {
			continue
		}
		if ds.Vpn == nil {
			ds.Vpn = &ngfwv1.VpnConfig{}
		}
		if ds.Vpn.RemoteAccess == nil {
			ds.Vpn.RemoteAccess = map[string]*ngfwv1.RemoteAccessProfile{}
		}
		ds.Vpn.RemoteAccess[spec.Profile] = proto.Clone(spec.Configuration).(*ngfwv1.RemoteAccessProfile)
	}
}
