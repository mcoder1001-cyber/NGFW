package desired

import (
	"context"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	ravpn "ngfw/agent/internal/ra_vpn"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

func TestRemoteAccessFailsClosedWithoutObjectsOrCredentials(t *testing.T) {
	for _, enabled := range []*bool{nil, proto.Bool(true), proto.Bool(false)} {
		ds := &ngfwv1.DesiredState{Vpn: &ngfwv1.VpnConfig{RemoteAccess: map[string]*ngfwv1.RemoteAccessProfile{
			"road/warrior~one": {Enabled: enabled},
		}}}
		s := &sink{}
		RemoteAccess(s, ds, map[string]bool{"vpn": true})
		if len(s.kvs) != 0 {
			t.Fatal("unavailable RA projected objects")
		}
		wanted := enabled == nil || *enabled
		if (len(s.errs) != 0) != wanted {
			t.Fatalf("enabled=%v errors=%v", enabled, s.errs)
		}
		if wanted && !strings.Contains(s.errs[0], "/vpn/remoteAccess/road~1warrior~0one/enabled") {
			t.Fatal(s.errs)
		}
	}
}

func TestRemoteAccessUnmanagedDomainUnchanged(t *testing.T) {
	s := &sink{}
	RemoteAccess(s, &ngfwv1.DesiredState{}, map[string]bool{"routing": true})
	if len(s.errs) != 0 || len(s.kvs) != 0 {
		t.Fatal("unmanaged VPN changed")
	}
}

func TestRemoteAccessEnabledProjectionRequiresPrivateOwnedRangeAndPolicies(t *testing.T) {
	ds := new(ngfwv1.DesiredState)
	data := `{"vpn":{"ipsec":{"proposals":{"modern":{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}}},"pki":{"certificates":{"server":{"certificateRef":"cert/server","privateKeyRef":"key/server"}}},"remoteAccess":{"road":{"localAddr":"192.0.2.19","certificate":"server","auth":"eap-mschapv2","proposal":"modern","pools":[{"name":"clients","prefix":"10.19.200.0/24"}],"users":[{"username":"client","passwordRef":"password/client"}],"transport":{"outer":{"vpp":"198.18.19.0/31","namespace":"198.18.19.1/31"},"inner":{"vpp":"198.18.19.2/31","namespace":"198.18.19.3/31"}},"outerPolicy":{"ingress":["public-in"],"egress":["public-out"]},"accessPolicy":{"ingress":["client-in"],"egress":["client-out"]}}}},"acl":{"lists":{"public-in":{"rules":[]},"public-out":{"rules":[]},"client-in":{"rules":[]},"client-out":{"rules":[]}}}}`
	if protojson.Unmarshal([]byte(data), ds) != nil {
		t.Fatal("fixture")
	}
	env := RAEnv{Owner: "w19", IDs: TunnelIDSpan{Lo: 2432, Hi: 2440}, Ready: func(context.Context) error { return nil }, SecretRef: func(context.Context, string) (string, error) { return "hmac:" + strings.Repeat("a", 64), nil }, VRF: func(string) (uint32, bool) { return 0, true }}
	s := &sink{}
	RemoteAccess(s, ds, map[string]bool{"vpn": true}, env)
	if len(s.errs) != 0 || len(s.kvs) < 10 {
		t.Fatal("enabled projection", s.errs)
	}
	found := false
	for _, kv := range s.kvs {
		if kv.Key.Descriptor() == ravpn.EngineName {
			found = true
		}
		if kv.Key.Descriptor() == "ip.route" || kv.Key.Descriptor() == "acl.interface-binding" {
			t.Fatal("private object entered ordinary authority")
		}
	}
	if !found {
		t.Fatal("engine absent")
	}
	for _, ids := range []TunnelIDSpan{{Lo: 8190, Hi: 8192}, {All: true}} {
		env.IDs = ids
		boundary := &sink{}
		RemoteAccess(boundary, ds, map[string]bool{"vpn": true}, env)
		if len(boundary.errs) != 0 {
			t.Fatal("compatible last/full range refused", boundary.errs)
		}
		for _, kv := range boundary.kvs {
			if kv.Key.Descriptor() == ravpn.EngineName {
				spec, err := ravpn.DecodeEngine(kv.Value.(*structpb.Struct))
				if err != nil || spec.OuterID > 8191 || spec.InnerID > 8191 {
					t.Fatal("out-of-range projected")
				}
			}
		}
	}
	env.IDs = TunnelIDSpan{Lo: 8191, Hi: 8192}
	exhausted := &sink{}
	RemoteAccess(exhausted, ds, map[string]bool{"vpn": true}, env)
	if len(exhausted.kvs) != 0 || len(exhausted.errs) == 0 {
		t.Fatal("single compatible ID accepted")
	}
	env.IDs = TunnelIDSpan{Lo: 19000, Hi: 19999}
	bad := &sink{}
	RemoteAccess(bad, ds, map[string]bool{"vpn": true}, env)
	if len(bad.kvs) != 0 || len(bad.errs) == 0 {
		t.Fatal("unsupported slot silently remapped")
	}
	env.IDs = TunnelIDSpan{Lo: 2432, Hi: 2440}
	ds.Vpn.RemoteAccess["road"].OuterPolicy.Egress = nil
	bad = &sink{}
	RemoteAccess(bad, ds, map[string]bool{"vpn": true}, env)
	if len(bad.kvs) != 0 || len(bad.errs) == 0 {
		t.Fatal("one-sided policy projected")
	}
}
