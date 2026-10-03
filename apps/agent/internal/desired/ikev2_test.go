package desired

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"strings"
	"testing"
)

func nativeDoc(t *testing.T) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	err := protojson.Unmarshal([]byte(`{"tunnels":{"ipip":{"site":{"instance":8001,"ipv4":["198.18.83.1/30"],"src":"198.18.8.1","dst":"198.18.8.2","vrf":"default","underlayVrf":"default"}}},"vpn":{"ipsec":{"proposals":{"p":{"ike":{"encr":"aes128","integ":"sha256","prf":"prfsha256","dh":"modp2048"},"esp":{"encr":"aes128gcm16"}}},"tunnels":{"site":{"engine":"vpp-ikev2","ikeVersion":2,"localAddr":"198.18.8.1","remoteAddr":"198.18.8.2","localId":"@local.test","remoteId":"@remote.test","auth":{"method":"psk","secretRef":"psk/site"},"proposal":"p","routeBased":{"ipipInterface":"site"}}}}}}`), ds)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestIKEv2NativeExclusiveOwnershipBeforeProjection(t *testing.T) {
	for _, separateIPIP := range []bool{false, true} {
		t.Run(fmt.Sprint("separate-ipip-", separateIPIP), func(t *testing.T) {
			ds := nativeDoc(t)
			other := proto.Clone(ds.Vpn.Ipsec.Tunnels["site"]).(*vrxv1.IpsecTunnel)
			if separateIPIP {
				i := proto.Clone(ds.Tunnels.Ipip["site"]).(*vrxv1.IpipTunnel)
				i.Instance = proto.Uint32(8002)
				ds.Tunnels.Ipip["other"] = i
				other.RouteBased.IpipInterface = proto.String("other")
			}
			ds.Vpn.Ipsec.Tunnels["other"] = other
			resolved := 0
			env := IKEv2Env{SecretRef: func(context.Context, string) (string, error) {
				resolved++
				return "hmac:" + strings.Repeat("a", 64), nil
			}}
			s := &sink{}
			IKEv2(s, ds, inVPN, env)
			if len(s.errs) == 0 || resolved != 0 || s.value(scheduler.Join(ikev2.ProfileName, "site")) != nil || s.value(scheduler.Join(ikev2.ProfileName, "other")) != nil {
				t.Fatal("duplicate ownership was not refused before projection", s.errs, resolved)
			}
			other.Enabled = proto.Bool(false)
			s = &sink{}
			IKEv2(s, ds, inVPN, env)
			if len(s.errs) != 0 || resolved != 1 || s.value(scheduler.Join(ikev2.ProfileName, "site")) == nil {
				t.Fatal("disabled profile reserved active ownership", s.errs, resolved)
			}
		})
	}
}

func TestIKEv2NativeProjectionAndLiveDrift(t *testing.T) {
	ds := nativeDoc(t)
	s := &sink{}
	IKEv2(s, ds, inVPN, IKEv2Env{SecretRef: func(context.Context, string) (string, error) { return "hmac:" + strings.Repeat("a", 64), nil }})
	if len(s.errs) > 0 {
		t.Fatal(s.errs)
	}
	profile := s.value(scheduler.Join(ikev2.ProfileName, "site"))
	if profile == nil {
		t.Fatal("no profile")
	}
	p := profile.(*vpnpb.Ikev2Profile)
	if p.TunnelInterface != "ipip8001" || p.LocalTs.StartAddr != "0.0.0.0" || p.LocalTs.EndAddr != "255.255.255.255" || p.Esp.CryptoAlg != "aes-gcm-16" {
		t.Fatal(p)
	}
	meta := s.value(ikev2.MetaKey("site"))
	kvs := []scheduler.KV{{Key: scheduler.Join(ikev2.ProfileName, "site"), Value: p}, {Key: ikev2.MetaKey("site"), Value: meta}}
	got := &vrxv1.DesiredState{}
	AssembleIKEv2(got, kvs)
	if !proto.Equal(ds.Vpn, got.Vpn) {
		t.Fatalf("read-back did not preserve native config: %v", got)
	}
	changed := proto.Clone(p).(*vpnpb.Ikev2Profile)
	changed.Esp.CryptoKeySize = 256
	kvs[0].Value = changed
	drift := &vrxv1.DesiredState{}
	AssembleIKEv2(drift, kvs)
	if drift.Vpn != nil {
		t.Fatal("metadata concealed live drift")
	}
}

func TestIKEv2NativeRefusalBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*vrxv1.DesiredState)
		env   IKEv2Env
	}{
		{name: "secret-unavailable"},
		{name: "policy", alter: func(ds *vrxv1.DesiredState) { ds.Vpn.Ipsec.Tunnels["site"].RouteBased = nil }},
		{name: "transport", alter: func(ds *vrxv1.DesiredState) { ds.Vpn.Ipsec.Tunnels["site"].Mode = proto.String("transport") }},
		{name: "wrong-underlay", alter: func(ds *vrxv1.DesiredState) { ds.Tunnels.Ipip["site"].UnderlayVrf = proto.String("wrong") }},
		{name: "unsupported-underlay-vrf", alter: func(ds *vrxv1.DesiredState) {
			ds.Tunnels.Ipip["site"].UnderlayVrf = proto.String("outer")
			ds.Vpn.Ipsec.Tunnels["site"].UnderlayVrf = proto.String("outer")
		}},
		{name: "ipv6-underlay", alter: func(ds *vrxv1.DesiredState) {
			ds.Tunnels.Ipip["site"].Src = proto.String("2001:db8::1")
			ds.Tunnels.Ipip["site"].Dst = proto.String("2001:db8::2")
			ds.Vpn.Ipsec.Tunnels["site"].LocalAddr = proto.String("2001:db8::1")
			ds.Vpn.Ipsec.Tunnels["site"].RemoteAddr = proto.String("2001:db8::2")
		}},
		{name: "custom-dpd", alter: func(ds *vrxv1.DesiredState) {
			ds.Vpn.Ipsec.Tunnels["site"].Dpd = &vrxv1.IpsecDpd{DelaySec: proto.Uint32(1)}
		}},
		{name: "custom-ike-lifetime", alter: func(ds *vrxv1.DesiredState) {
			ds.Vpn.Ipsec.Tunnels["site"].Rekey = &vrxv1.IpsecRekey{IkeSec: proto.Uint32(600)}
		}},
		{name: "many-selectors", alter: func(ds *vrxv1.DesiredState) {
			ds.Vpn.Ipsec.Tunnels["site"].LocalTs = []string{"10.0.0.0/24", "10.0.1.0/24"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := nativeDoc(t)
			if tc.alter != nil {
				tc.alter(ds)
			}
			s := &sink{}
			IKEv2(s, ds, inVPN, tc.env)
			if len(s.errs) == 0 || s.value(scheduler.Join(ikev2.ProfileName, "site")) != nil {
				t.Fatal("invalid native config was projected", s.errs)
			}
		})
	}
}

func TestNativeCertificateRefusedBeforeResolvingMaterial(t *testing.T) {
	ds := nativeDoc(t)
	ds.Vpn.Ipsec.Tunnels["site"].Auth = &vrxv1.IpsecAuth{Method: proto.String("cert"), Certificate: proto.String("local"), RemoteCa: proto.String("ca")}
	resolved := 0
	s := &sink{}
	IKEv2(s, ds, inVPN, IKEv2Env{SecretRef: func(context.Context, string) (string, error) { resolved++; return "unused", nil }})
	if resolved != 0 || s.value(scheduler.Join(ikev2.ProfileName, "site")) != nil {
		t.Fatal("unsupported certificate request reached material resolution or mutation plan")
	}
	if len(s.errs) == 0 {
		t.Fatal("certificate capability gap was hidden")
	}
}

var inVPN = map[string]bool{"vpn": true}
