package detectors

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	"testing"
	"time"
)

func TestNativePeerCorrelationAndSPIDedup(t *testing.T) {
	ds := &ngfwv1.DesiredState{Vpn: &ngfwv1.VpnConfig{Ipsec: &ngfwv1.IpsecConfig{Tunnels: map[string]*ngfwv1.IpsecTunnel{"site": {Engine: proto.String("vpp-ikev2"), LocalAddr: proto.String("192.0.2.1"), RemoteAddr: proto.String("192.0.2.7")}}}}}
	n := NewNative()
	now := time.Now()
	sa := ikev2.SAState{Profile: "site", State: "AUTH_FAILED", ISPI: 1, RSPI: 2, IAddr: "192.0.2.7", RAddr: "192.0.2.1"}
	if got := n.Observe([]ikev2.SAState{sa}, ds, now); len(got) != 1 || got[0].Source != "192.0.2.7" {
		t.Fatalf("wrong peer: %v", got)
	}
	if len(n.Observe([]ikev2.SAState{sa}, ds, now)) != 0 {
		t.Fatal("one failed SA counted repeatedly")
	}
	sa.ISPI++
	sa.IAddr, sa.RAddr = sa.RAddr, sa.IAddr
	if len(n.Observe([]ikev2.SAState{sa}, ds, now)) != 1 {
		t.Fatal("initiator role not resolved")
	}
	sa.ISPI++
	sa.IAddr = "198.51.100.9"
	if len(n.Observe([]ikev2.SAState{sa}, ds, now)) != 0 {
		t.Fatal("unconfigured peer accepted")
	}
	sa.IAddr = "192.0.2.1"
	sa.Profile = "foreign"
	if len(n.Observe([]ikev2.SAState{sa}, ds, now)) != 0 {
		t.Fatal("foreign profile accepted")
	}
}
