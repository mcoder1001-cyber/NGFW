package ldp

import (
	"errors"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

type contextFixture struct {
	mapped      string
	secretError bool
}

func (c contextFixture) MapInterface(string) (string, bool) { return c.mapped, c.mapped != "" }
func (c contextFixture) Secret(string) (string, error) {
	if c.secretError {
		return "", errors.New("resolver contains confidential detail")
	}
	return "NGFW_TEST_PSK_F_mpls_ldp", nil
}

func config() *ngfwv1.MplsLdp {
	return &ngfwv1.MplsLdp{RouterId: proto.String("192.0.2.1"), TransportAddress: proto.String("192.0.2.2"), Interfaces: []string{"wan"}}
}

func TestRender(t *testing.T) {
	c := config()
	c.Neighbors = map[string]*ngfwv1.LdpNeighbor{"192.0.2.3": {PasswordRef: proto.String("password/peer")}}
	c.LabelRange = &ngfwv1.LdpLabelRange{Min: proto.Uint32(16000), Max: proto.Uint32(17000)}
	got, err := Render(c, contextFixture{mapped: "host-w1wan"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mpls label dynamic-block 16000 17000", "mpls ldp", " router-id 192.0.2.1", " neighbor 192.0.2.3 password NGFW_TEST_PSK_F_mpls_ldp", " address-family ipv4", "  discovery transport-address 192.0.2.2", "  interface host-w1wan", "  exit", " exit-address-family", "exit"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInvalidConfigProducesNoOutput(t *testing.T) {
	for _, test := range []struct {
		name    string
		mutate  func(*ngfwv1.MplsLdp)
		context contextFixture
	}{
		{"router injection", func(c *ngfwv1.MplsLdp) { c.RouterId = proto.String("192.0.2.1\nend") }, contextFixture{mapped: "wan"}},
		{"IPv6 transport", func(c *ngfwv1.MplsLdp) { c.TransportAddress = proto.String("::1") }, contextFixture{mapped: "wan"}},
		{"unmapped", func(*ngfwv1.MplsLdp) {}, contextFixture{}},
		{"unsafe mapped", func(*ngfwv1.MplsLdp) {}, contextFixture{mapped: "wan\nend"}},
		{"duplicate mapping", func(c *ngfwv1.MplsLdp) { c.Interfaces = []string{"wan", "lan"} }, contextFixture{mapped: "wan"}},
		{"range", func(c *ngfwv1.MplsLdp) {
			c.LabelRange = &ngfwv1.LdpLabelRange{Min: proto.Uint32(3), Max: proto.Uint32(100)}
		}, contextFixture{mapped: "wan"}},
		{"secret failure", func(c *ngfwv1.MplsLdp) {
			c.Neighbors = map[string]*ngfwv1.LdpNeighbor{"192.0.2.3": {PasswordRef: proto.String("password/peer")}}
		}, contextFixture{mapped: "wan", secretError: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := config()
			test.mutate(c)
			got, err := Render(c, test.context)
			if err == nil || got != nil {
				t.Fatalf("got %q, err %v", got, err)
			}
		})
	}
}

func TestAbsent(t *testing.T) {
	got, err := Render(nil, contextFixture{})
	if got != nil || err != nil {
		t.Fatalf("got %q err %v", got, err)
	}
}
