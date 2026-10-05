package desired

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/scheduler"
)

type wgSink struct {
	keys   []string
	issues []string
}

func (s *wgSink) Add(k scheduler.Key, _ proto.Message, pointer string) {
	s.keys = append(s.keys, string(k)+"@"+pointer)
}
func (s *wgSink) Errorf(pointer, rule, _ string, _ ...any) {
	s.issues = append(s.issues, "E "+rule+" "+pointer)
}
func (s *wgSink) Warnf(pointer, rule, _ string, _ ...any) {
	s.issues = append(s.issues, "W "+rule+" "+pointer)
}

func wgState(t *testing.T, js string) *ngfwv1.DesiredState {
	t.Helper()
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func wgVRFs(name string) (uint32, bool) {
	switch name {
	case "default":
		return 0, true
	case "red":
		return 7001, true
	}
	return 0, false
}

func TestWireguardRouteNextHop(t *testing.T) {
	for in, want := range map[string]string{"10.7.10.0/24": "10.7.10.0", "10.7.1.9/32": "10.7.1.9", "0.0.0.0/0": "0.0.0.1", "::/0": "::1", "fd00:7::/64": "fd00:7::"} {
		if got, err := WireguardRouteNextHop(in); err != nil || got != want {
			t.Errorf("%s → %s, %v (want %s)", in, got, err, want)
		}
	}
}

func TestWireguardBuilderFindings(t *testing.T) {
	const pub = "HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw="
	ds := wgState(t, `{"vpn": {
	  "ipsec": {"settings": {"asyncCrypto": false}},
	  "wireguard": {"interfaces": {
	    "a": {"instance": 7001, "listenAddress": "10.7.8.1", "privateKeyRef": "key/a", "address": ["10.7.9.1/24"], "routeAllowedIps": true, "vrf": "red",
	          "peers": {"dns": {"publicKey": "`+pub+`", "endpoint": {"address": "vpn.example.net", "port": 1}, "allowedIps": ["10.7.10.0/24"]}}},
	    "b": {"instance": 7002, "listenAddress": "10.7.8.1", "privateKeyRef": "key/b", "underlayVrf": "nope"}
	  }}}}`)
	s := &wgSink{}
	refs := WireguardEnv{SecretRef: func(r string) (string, error) { return "x25519:" + r, nil }}
	Wireguard(s, ds, map[string]bool{"vpn": true}, wgVRFs, refs)
	got := strings.Join(s.issues, "\n")
	for _, want := range []string{
		"W agent.cross-domain /vpn/wireguard/interfaces/a", // interfaces domain not in the transaction
		"E agent.unsupported-value /vpn/wireguard/interfaces/a/peers/dns/endpoint/address",
		"E vpn.vrf-exists /vpn/wireguard/interfaces/b/underlayVrf",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// the peer with a hostname endpoint is not projected; no route without `routing`
	for _, k := range s.keys {
		if strings.HasPrefix(k, "wireguard.peer/") || strings.HasPrefix(k, "ip.route/") || strings.HasPrefix(k, "interface-ip/") {
			t.Errorf("unexpected object %s", k)
		}
	}
	// without the vpn domain nothing happens
	s2 := &wgSink{}
	Wireguard(s2, ds, map[string]bool{"interfaces": true}, wgVRFs, refs)
	if len(s2.keys)+len(s2.issues) != 0 {
		t.Fatalf("projected without vpn: %v %v", s2.keys, s2.issues)
	}
}

// Review F1: routeAllowedIps with an allowed IP that contains a peer endpoint routed in the same VRF (the full-tunnel
// 0.0.0.0/0 peer with vrf == underlayVrf) would send the tunnel's own UDP into the tunnel. The builder refuses the route
// with an ERROR at the allowed IP (the schema's vpn.wireguard-route-loop is the API-side twin).
func TestWireguardRouteLoopRefused(t *testing.T) {
	const pub = "HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw="
	refs := WireguardEnv{SecretRef: func(r string) (string, error) { return "x25519:" + r, nil }}
	all := map[string]bool{"vpn": true, "interfaces": true, "routing": true}
	doc := func(vrf string) *ngfwv1.DesiredState {
		return wgState(t, `{"vpn": {"wireguard": {"interfaces": {"a": {"instance": 7001, "listenAddress": "10.7.8.1",
		  "privateKeyRef": "key/a", "routeAllowedIps": true, "vrf": "`+vrf+`",
		  "peers": {"hq": {"publicKey": "`+pub+`", "endpoint": {"address": "203.0.113.9", "port": 51820}, "allowedIps": ["0.0.0.0/0"]}}}}}}}`)
	}
	s := &wgSink{}
	Wireguard(s, doc("default"), all, wgVRFs, refs)
	if !strings.Contains(strings.Join(s.issues, "\n"), "E vpn.wireguard-route-loop /vpn/wireguard/interfaces/a/peers/hq/allowedIps/0") {
		t.Fatalf("no route-loop error: %v", s.issues)
	}
	for _, k := range s.keys {
		if strings.HasPrefix(k, "ip.route/") {
			t.Fatalf("the looping route was projected: %s", k)
		}
	}
	// routes in another VRF than the endpoint's underlay: no loop, the route is projected
	s = &wgSink{}
	Wireguard(s, doc("red"), all, wgVRFs, refs)
	var route bool
	for _, k := range s.keys {
		route = route || strings.HasPrefix(k, "ip.route/7001/0.0.0.0/0@")
	}
	if !route || strings.Contains(strings.Join(s.issues, "\n"), "route-loop") {
		t.Fatalf("overlay VRF red: keys %v issues %v", s.keys, s.issues)
	}
}

// A transaction can configure IPsec/PKI alongside WireGuard. Those implemented
// builders validate their own leaves; WireGuard must not mark their entire roots
// unsupported. Remote access is implemented by the registered RA controller.
func TestWireguardOtherVpnCapabilities(t *testing.T) {
	ds := wgState(t, `{"vpn":{"ipsec":{"settings":{"asyncCrypto":false}},"pki":{"certificates":{"site":{"certificateRef":"cert/site","privateKeyRef":"key/site"}}},"remoteAccess":{"road":{}}}}`)
	s := &wgSink{}
	Wireguard(s, ds, map[string]bool{"vpn": true}, wgVRFs, WireguardEnv{})
	got := strings.Join(s.issues, "\n")
	for _, obsolete := range []string{"W agent.unsupported-field /vpn/ipsec", "W agent.unsupported-field /vpn/pki", "W agent.unsupported-field /vpn/remoteAccess"} {
		if strings.Contains(got, obsolete) {
			t.Errorf("implemented VPN capability wrongly rejected: %s\n%s", obsolete, got)
		}
	}
}
