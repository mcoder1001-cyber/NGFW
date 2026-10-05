package strongswan

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"strings"
	"testing"
)

func raFixture(t *testing.T) (*ngfwv1.RemoteAccessProfile, *ngfwv1.IpsecProposal) {
	t.Helper()
	ds := doc(t, `{"vpn":{"ipsec":{"proposals":{"modern":{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}}},"remoteAccess":{"road":{"localAddr":"192.0.2.19","localId":"vpn.example.test","auth":"eap-mschapv2","certificate":"server","pools":[{"name":"clients","prefix":"10.19.200.0/24","dns":["10.19.0.53"]}],"splitTunnel":["10.19.0.0/16"],"users":[{"username":"client","passwordRef":"password/client"}]}}}}`)
	return ds.GetVpn().GetRemoteAccess()["road"], ds.GetVpn().GetIpsec().GetProposals()["modern"]
}
func TestRAIndependentRendererAndSecretRedaction(t *testing.T) {
	p, proposal := raFixture(t)
	secret := "NGFW_TEST_PASSWORD_RA19"
	files, err := BuildRAFiles(context.Background(), "road", p, proposal, "/run/ngfw/ra/fixture", testResolver(map[string]string{"password/client": secret}))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"remote_addrs = %any", "eap_id = %any", "local_ts = 10.19.0.0/16", "remote_ts = dynamic", "if_id_in = 1", "rekey_time = 3600s", "dns = 10.19.0.53"} {
		if !strings.Contains(string(files.Connection), expected) {
			t.Errorf("missing %s", expected)
		}
	}
	for _, expected := range []string{"kernel-netlink", "install_routes = no", "install_routes_xfrmi = no", "install_virtual_ip = no", "unix:///run/ngfw/ra/fixture/vici.sock"} {
		if !strings.Contains(string(files.Daemon), expected) {
			t.Errorf("missing %s", expected)
		}
	}
	if strings.Contains(string(files.Daemon), "kernel-vpp") {
		t.Fatal("retired plugin rendered")
	}
	if strings.Contains(string(files.Connection), secret) || strings.Contains(string(files.Daemon), secret) {
		t.Fatal("secret outside private secrets file")
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(secret))
	if !strings.Contains(string(files.Secrets), "0s"+encoded) {
		t.Fatal("EAP secret not encoded")
	}
	state, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	for _, representation := range []string{string(state), fmt.Sprintf("%v", files), fmt.Sprintf("%+v", *files), fmt.Sprintf("%#v", files)} {
		if strings.Contains(representation, secret) || strings.Contains(representation, encoded) {
			t.Fatal("file bytes leaked through generic serialization")
		}
	}
}
func TestRARendererTLSAndRADIUS(t *testing.T) {
	p, proposal := raFixture(t)
	p.Auth = proto.String("eap-tls")
	p.ClientCa = proto.String("clients-ca")
	files, err := BuildRAFiles(context.Background(), "road", p, proposal, "/run/ngfw/ra/fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files.Connection), "revocation = strict") || !strings.Contains(string(files.Connection), "clients-ca.pem") {
		t.Fatal("TLS client trust/revocation not explicit")
	}
	if strings.Contains(string(files.Daemon), "eap-tls tls") {
		t.Fatal("libtls is a linked library, not a loadable plugin")
	}
	p.Auth = proto.String("eap-radius")
	p.Radius = &ngfwv1.RemoteAccessProfile_Radius{Servers: []*ngfwv1.RemoteAccessProfile_Radius_Server{{Address: proto.String("192.0.2.20"), SecretRef: proto.String("psk/radius")}}}
	secret := "NGFW_TEST_PSK_RADIUS19"
	files, err = BuildRAFiles(context.Background(), "road", p, proposal, "/run/ngfw/ra/fixture", testResolver(map[string]string{"psk/radius": secret}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files.Daemon), "eap-radius { servers {") || !strings.Contains(string(files.Daemon), "port = 1812") || !strings.Contains(string(files.Daemon), secret) {
		t.Fatal("RADIUS private settings missing")
	}
	if strings.Contains(string(files.Connection), secret) {
		t.Fatal("RADIUS secret in public connection config")
	}
}
func TestRARendererRefusesBadInputsWithoutResolverDetails(t *testing.T) {
	p, proposal := raFixture(t)
	resolver := SecretResolverFunc(func(context.Context, string) ([]byte, error) {
		return nil, errors.New("NGFW_TEST_PASSWORD_RESOLVER_DETAIL")
	})
	_, err := BuildRAFiles(context.Background(), "road", p, proposal, "/run/ngfw/ra/fixture", resolver)
	if err == nil || strings.Contains(err.Error(), "RESOLVER_DETAIL") {
		t.Fatal("resolver refusal leaked or missing")
	}
	p.Users[0].Username = proto.String("client\nunsafe")
	if _, err := BuildRAFiles(context.Background(), "road", p, proposal, "/run/ngfw/ra/fixture", testResolver(nil)); err == nil {
		t.Fatal("unsafe username accepted")
	}
	p.Auth = proto.String("eap-tls")
	p.ClientCa = nil
	if _, err := BuildRAFiles(context.Background(), "road", p, proposal, "/run/ngfw/ra/fixture", nil); err == nil {
		t.Fatal("TLS without trust accepted")
	}
}
