package isis

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
	"testing"
)

type authFixture struct{ value string }

func (f authFixture) Secret(string) (string, error) { return f.value, nil }
func TestFamiliesAndAuthentication(t *testing.T) {
	o := &ngfwv1.IsisConfig{Net: proto.String("49.0001.1921.6800.1001.00"), Interfaces: map[string]*ngfwv1.IsisInterface{"loop0": {Ipv4: proto.Bool(false)}}}
	mapper := func(string) (string, bool) { return "eth0", true }
	lines, e := RenderInterfaces(o, mapper)
	if e != nil || len(lines["eth0"]) != 1 || lines["eth0"][0] != " ipv6 router isis ngfw" {
		t.Fatal(lines, e)
	}
	o.AreaPasswordRef = proto.String("password/area")
	if _, e := RenderWithSecrets(o, nil); e == nil {
		t.Fatal("missing resolver")
	}
	if _, e := RenderWithSecrets(o, authFixture{"NGFW_TEST_PSK_ISIS"}); e != nil {
		t.Fatal(e)
	}
	if _, e := RenderWithSecrets(o, authFixture{"key\nrouter bgp 1"}); e == nil {
		t.Fatal("injection")
	}
}
func TestMalformedAdjacencySnapshot(t *testing.T) {
	for _, raw := range []string{"", `{"unknown":1}`, `{"areas":null}`, `{"vrfs":[{"vrf":"default"}]}`, `{"areas":[{"area":"ngfw","circuits":[{"adj":"peer","state":"Up"}]}]}`} {
		if _, e := PollAdjacencies(context.Background(), func(context.Context, frr.ShowCommand) (json.RawMessage, error) { return json.RawMessage(raw), nil }); e == nil {
			t.Fatal("malformed read fabricates removal", raw)
		}
	}
}
