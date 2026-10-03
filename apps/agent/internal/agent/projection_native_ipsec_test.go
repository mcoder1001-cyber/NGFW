package agent

import (
	"testing"

	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

func TestNativeIPsecOwnsProtectedIPIPAdminState(t *testing.T) {
	ds := doc(t, `{"vpn":{"ipsec":{"tunnels":{"site":{"engine":"vpp-ikev2","routeBased":{"ipipInterface":"protected"}}}}},"tunnels":{"ipip":{"protected":{"instance":123},"plain":{"instance":124}}}}`)
	protected := scheduler.Join(iface.AdminStateName, "ipip123")
	plain := scheduler.Join(iface.AdminStateName, "ipip124")
	p := &projected{kvs: []scheduler.KV{
		{Key: protected, Value: &iface.AdminState{Interface: string(iface.AliasKey("ipip123"))}},
		{Key: plain, Value: &iface.AdminState{Interface: string(iface.AliasKey("ipip124"))}},
	}}
	suppressNativeIPsecAdmin(p, ds)
	if len(p.kvs) != 1 || p.kvs[0].Key != plain {
		t.Fatalf("admin-state desired keys: %v; protected IPIP must remain runtime-controlled", p.kvs)
	}
	if len(p.issues) != 1 || p.issues[0].pointer != "/tunnels/ipip/protected/enabled" || p.issues[0].rule != "agent.write-only-field" {
		t.Fatalf("coverage notices: %v", p.issues)
	}
}
