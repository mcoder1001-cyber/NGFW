package desired

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/tapv2"
	"strings"
	"testing"
)

func carrierInterfaces() map[string]*ngfwv1.Interface {
	return map[string]*ngfwv1.Interface{"pppwan": {Pppoe: &ngfwv1.Pppoe{Parent: proto.String("wanraw")}}, "wanraw": {Enabled: proto.Bool(true)}}
}
func TestCarrierProjectionSeparatesRawAndTransit(t *testing.T) {
	s := &pppoeSink{}
	PppoeCarriers(s, carrierInterfaces(), "ngfw")
	if len(s.errors) != 0 {
		t.Fatal(s.errors)
	}
	taps := map[string]*tapv2.Tap{}
	for _, kv := range s.kvs {
		if tap, ok := kv.Value.(*tapv2.Tap); ok {
			taps[tap.Name] = tap
		}
	}
	if len(taps) != 2 || taps["pppwan"] == nil {
		t.Fatalf("taps=%v", taps)
	}
	transit := taps["pppwan"]
	if transit.HostIp4Prefix == "" || transit.HostIp6Prefix == "" || !strings.HasPrefix(transit.HostNamespace, "ngp-") {
		t.Fatal(transit)
	}
	for name, raw := range taps {
		if name == "pppwan" {
			continue
		}
		if raw.HostIp4Prefix != "" || raw.HostIp6Prefix != "" || raw.Id == transit.Id || raw.HostNamespace != transit.HostNamespace {
			t.Fatal(raw)
		}
	}
}
func TestCarrierProjectionRejectsBeforeEmitting(t *testing.T) {
	for name, mutate := range map[string]func(map[string]*ngfwv1.Interface){
		"legacy":          func(m map[string]*ngfwv1.Interface) { m["pppwan"].Pppoe.Parent = nil },
		"lcp":             func(m map[string]*ngfwv1.Interface) { m["wanraw"].Lcp = &ngfwv1.InterfaceLcp{} },
		"logical-address": func(m map[string]*ngfwv1.Interface) { m["pppwan"].Ipv4 = []string{"192.0.2.1/24"} },
		"mtu":             func(m map[string]*ngfwv1.Interface) { m["pppwan"].Mtu = proto.Uint32(1500) },
	} {
		t.Run(name, func(t *testing.T) {
			m := carrierInterfaces()
			mutate(m)
			s := &pppoeSink{}
			PppoeCarriers(s, m, "ngfw")
			if len(s.errors) == 0 || len(s.kvs) != 0 {
				t.Fatalf("errors=%v objects=%d", s.errors, len(s.kvs))
			}
		})
	}
}

func TestCarrierVLANCompleteProjectionHasRewriteAndManifest(t *testing.T) {
	ifs := carrierInterfaces()
	ifs["wanraw"].Subinterfaces = map[string]*ngfwv1.Subinterface{"100": {Enabled: proto.Bool(true), VlanId: proto.Uint32(100)}, "200": {Enabled: proto.Bool(true), VlanId: proto.Uint32(200)}}
	ifs["pppwan"].Pppoe.Parent = proto.String("wanraw.100")
	sink := &pppoeSink{}
	PppoeCarriers(sink, ifs, "ngfw")
	Pppoe(sink, ifs, true, "ngfw")
	if len(sink.errors) != 0 {
		t.Fatal(sink.errors)
	}
	rewrite, manifest := false, false
	for _, kv := range sink.kvs {
		if kv.Key == "l2.vlan-tag-rewrite/wanraw.100" {
			rewrite = true
		}
		if kv.Key == PppoeClientKey {
			doc := kv.Value.(*ngfwv1.DesiredState)
			manifest = doc.Interfaces["wanraw"].Subinterfaces["100"] != nil && doc.Interfaces["wanraw"].Subinterfaces["200"] == nil
		}
	}
	if !rewrite || !manifest {
		t.Fatalf("rewrite=%v manifest=%v", rewrite, manifest)
	}
	ifs["wanraw"].Mtu = proto.Uint32(1400)
	rejected := &pppoeSink{}
	PppoeCarriers(rejected, ifs, "ngfw")
	if len(rejected.errors) == 0 || len(rejected.kvs) != 0 {
		t.Fatal("insufficient parent MTU emitted partial carrier")
	}
}
