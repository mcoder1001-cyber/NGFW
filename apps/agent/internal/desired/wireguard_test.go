package desired

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

type sink struct {
	keys   []string
	issues []string
}

func (s *sink) Add(k scheduler.Key, _ proto.Message, pointer string) {
	s.keys = append(s.keys, string(k)+"@"+pointer)
}
func (s *sink) Errorf(pointer, rule, format string, a ...any) {
	s.issues = append(s.issues, "E "+rule+" "+pointer)
}
func (s *sink) Warnf(pointer, rule, format string, a ...any) {
	s.issues = append(s.issues, "W "+rule+" "+pointer)
}

func wgState(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func vrfs(name string) (uint32, bool) {
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
	s := &sink{}
	refs := WireguardEnv{SecretRef: func(r string) (string, error) { return "x25519:" + r, nil }}
	Wireguard(s, ds, map[string]bool{"vpn": true}, vrfs, refs)
	got := strings.Join(s.issues, "\n")
	for _, want := range []string{
		"W agent.unsupported-field /vpn/ipsec",
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
	s2 := &sink{}
	Wireguard(s2, ds, map[string]bool{"interfaces": true}, vrfs, refs)
	if len(s2.keys)+len(s2.issues) != 0 {
		t.Fatalf("projected without vpn: %v %v", s2.keys, s2.issues)
	}
}
