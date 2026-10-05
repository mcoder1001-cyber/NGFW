package nat44ed_test

import (
	"context"
	"go.fd.io/govpp/api"
	"net"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/internal/descriptors/nat44ed"
	"ngfw/agent/internal/descriptors/natcommon"
	"testing"
)

func TestWANPoolScopedNativeLifecycleAndExclusiveClaims(t *testing.T) {
	f := newFakeNAT()
	_, network, _ := net.ParseCIDR("10.9.2.1/24")
	network.IP = net.ParseIP("10.9.2.1")
	prefix := ip_types.AddressWithPrefix(ip_types.NewPrefix(*network))
	f.On("ip_address_dump", func(req api.Message) ([]api.Message, error) {
		if req.(*ip.IPAddressDump).SwIfIndex != 1 {
			return nil, nil
		}
		return []api.Message{&ip.IPAddressDetails{SwIfIndex: 1, Prefix: prefix}}, nil
	})
	claims := natcommon.NewMemoryClaimStore()
	p := nat44ed.New(f, "w9", natcommon.WithClaims(claims))
	d := p.WANPool("nat44-ed.static-address.wan", "nat44-ed.output-feature.wan")
	spec := natcommon.MustEncode(&nat44ed.WANPoolSpec{Interface: "loop900", Prefix: "10.9.2.1/24", VRF: 0})
	deps := d.Dependencies(spec)
	if len(deps) != 3 || deps[1].Key != "interface-ip/loop900/10.9.2.1/24" || deps[2].Key != "nat44-ed.output-feature.wan/loop900" {
		t.Fatalf("deps%v", deps)
	}
	f.addrs = []*nat44_ed.Nat44AddressDetails{{IPAddress: [4]uint8{10, 9, 2, 1}, VrfID: 0}}
	if _, err := d.Create(context.Background(), spec); err == nil {
		t.Fatal("adopted foreign pool")
	}
	if claims.Claimed("nat44-ed.static-address.wan/10.9.2.1-10.9.2.1/0") {
		t.Fatal("claim created on refusal")
	}
	if err := d.Delete(context.Background(), spec, nil); err == nil {
		t.Fatal("deleted foreign pool")
	}
	f.addrs = nil
	meta, err := d.Create(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	f.addrs = append(f.addrs, &nat44_ed.Nat44AddressDetails{IPAddress: [4]uint8{10, 9, 2, 2}, VrfID: 0})
	regular, err := p.AddressPool.Retrieve(context.Background())
	if err != nil || len(regular) != 1 || regular[0].Key != "nat44-ed.address-pool/10.9.2.2-10.9.2.2/0" {
		t.Fatalf("configuration retrieved WAN pool: %v %v", regular, err)
	}
	req := f.CallsNamed("nat44_add_del_address_range")[0].(*nat44_ed.Nat44AddDelAddressRange)
	if req.VrfID != 0 || req.FirstIPAddress != [4]uint8{10, 9, 2, 1} || req.LastIPAddress != req.FirstIPAddress {
		t.Fatal(req)
	}
	restarted := nat44ed.New(f, "w9", natcommon.WithClaims(claims)).WANPool("nat44-ed.static-address.wan", "nat44-ed.output-feature.wan")
	rows, err := restarted.Retrieve(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("retrieval %v %v", rows, err)
	}
	restored, err := natcommon.Decode[nat44ed.WANPoolSpec](rows[0].Value)
	if err != nil || restored.Prefix != "10.9.2.1/24" || restored.Interface != "loop900" || restored.VRF != 0 {
		t.Fatalf("spec%+v %v", restored, err)
	}
	if err := restarted.Delete(context.Background(), spec, meta); err != nil {
		t.Fatal(err)
	}
	if len(f.addrs) != 1 || f.addrs[0].IPAddress != [4]uint8{10, 9, 2, 2} || claims.Claimed("nat44-ed.static-address.wan/10.9.2.1-10.9.2.1/0") {
		t.Fatal("pool or claim remained")
	}
}
