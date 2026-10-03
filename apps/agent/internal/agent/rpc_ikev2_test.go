package agent

import (
	"ngfw/agent/internal/descriptors/ikev2"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"testing"
)

func TestNativeIPsecStateDirections(t *testing.T) {
	sa := ikev2.SAState{IID: &ikev2.IDState{Type: "fqdn", Value: "initiator.test"}, RID: &ikev2.IDState{Type: "fqdn", Value: "responder.test"}}
	child := ikev2.ChildSAState{ISPI: 0x10203040, RSPI: 0x50607080}
	for _, tc := range []struct {
		id                 string
		initiator          bool
		incoming, outgoing uint32
	}{{"initiator.test", true, child.ISPI, child.RSPI}, {"responder.test", false, child.RSPI, child.ISPI}} {
		p := &vpnpb.Ikev2Profile{LocalId: &vpnpb.Ikev2Id{Type: "fqdn", Value: tc.id}}
		role := nativeLocalInitiator(p, sa)
		if role != tc.initiator {
			t.Fatal("incorrect local IKE role")
		}
		incoming, outgoing := nativeChildDirection(role, child)
		if incoming != tc.incoming || outgoing != tc.outgoing {
			t.Fatal("incorrect local ESP direction")
		}
	}
}

func TestNativeRoleUsesPeerAddressForEqualIdentities(t *testing.T) {
	p := &vpnpb.Ikev2Profile{LocalId: &vpnpb.Ikev2Id{Type: "fqdn", Value: "same.test"}, Responder: &vpnpb.Ikev2Responder{Address: "198.18.8.2"}}
	sa := ikev2.SAState{IAddr: "198.18.8.1", RAddr: "198.18.8.2", IID: &ikev2.IDState{Type: "fqdn", Value: "same.test"}, RID: &ikev2.IDState{Type: "fqdn", Value: "same.test"}}
	if !nativeLocalInitiator(p, sa) {
		t.Fatal("fixed peer identifies native initiator")
	}
	sa.IAddr, sa.RAddr = sa.RAddr, sa.IAddr
	if nativeLocalInitiator(p, sa) {
		t.Fatal("fixed peer identifies native responder")
	}
}
