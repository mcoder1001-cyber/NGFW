package desired

import (
	"google.golang.org/protobuf/proto"
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	"testing"
	"time"
)

func delegationFixture() (*ngfwv1.DesiredState, PppoeDelegationLease, time.Time) {
	now := time.Unix(2000000000, 0)
	doc := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{
		"wan": {Enabled: proto.Bool(true), Pppoe: &ngfwv1.Pppoe{Enabled: proto.Bool(true), Ipv6: proto.String("dhcpv6"), Parent: proto.String("raw"), DelegationTargets: []*ngfwv1.PppoeDelegationTarget{{Interface: "lan", SubnetId: 2}}}},
		"lan": {Enabled: proto.Bool(true)}, "raw": {Enabled: proto.Bool(true)},
	}}
	return doc, PppoeDelegationLease{Logical: "wan", Generation: "current", Ready: true, Delegated: netip.MustParsePrefix("2001:db8:100::/56"), ValidUntil: now.Add(time.Hour), PreferredUntil: now.Add(30 * time.Minute)}, now
}
func TestPppoeDelegationLeaseLifecycle(t *testing.T) {
	doc, lease, now := delegationFixture()
	kvs := PppoeDelegation(doc, []PppoeDelegationLease{lease}, now)
	if len(kvs) != 3 {
		t.Fatalf("objects=%v", kvs)
	}
	if got := kvs[0].Value.(*core.InterfaceAddress).Prefix; got != "2001:db8:100:2::1/64" {
		t.Fatal(got)
	}
	if got := kvs[1].Value.(*ip6nd.RaPrefix); got.ValidLifetime != 3600 || got.PreferredLifetime != 1800 {
		t.Fatal(got)
	}
	lease.Delegated = netip.MustParsePrefix("2001:db8:200::/56")
	renewed := PppoeDelegation(doc, []PppoeDelegationLease{lease}, now)
	if renewed[0].Key != kvs[0].Key || proto.Equal(renewed[0].Value, kvs[0].Value) {
		t.Fatal("renewal must replace value at stable owned key")
	}
	for name, change := range map[string]func(*ngfwv1.DesiredState, *PppoeDelegationLease){
		"not-ready":     func(_ *ngfwv1.DesiredState, l *PppoeDelegationLease) { l.Ready = false },
		"no-generation": func(_ *ngfwv1.DesiredState, l *PppoeDelegationLease) { l.Generation = "" },
		"expired":       func(_ *ngfwv1.DesiredState, l *PppoeDelegationLease) { l.ValidUntil = now },
		"deprecated":    func(_ *ngfwv1.DesiredState, l *PppoeDelegationLease) { l.PreferredUntil = now },
		"removed":       func(d *ngfwv1.DesiredState, _ *PppoeDelegationLease) { delete(d.Interfaces, "wan") },
		"disabled": func(d *ngfwv1.DesiredState, _ *PppoeDelegationLease) {
			d.Interfaces["wan"].Pppoe.Enabled = proto.Bool(false)
		},
		"rollback-targets": func(d *ngfwv1.DesiredState, _ *PppoeDelegationLease) {
			d.Interfaces["wan"].Pppoe.DelegationTargets = nil
		},
		"static-conflict": func(d *ngfwv1.DesiredState, _ *PppoeDelegationLease) {
			d.Interfaces["lan"].Ipv6 = []string{"2001:db8::1/64"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := proto.Clone(doc).(*ngfwv1.DesiredState)
			l := lease
			change(d, &l)
			if got := PppoeDelegation(d, []PppoeDelegationLease{l}, now); len(got) != 0 {
				t.Fatalf("stale objects retained: %v", got)
			}
		})
	}
}

func TestPppoeDelegationRejectsCrossInterfaceOverlap(t *testing.T) {
	doc, lease, now := delegationFixture()
	doc.Interfaces["other"] = &ngfwv1.Interface{Enabled: proto.Bool(true), Ipv6: []string{"2001:db8:100:2::55/64"}}
	if got := PppoeDelegation(doc, []PppoeDelegationLease{lease}, now); len(got) != 0 {
		t.Fatal("overlapping static LAN accepted", got)
	}
	delete(doc.Interfaces, "other")
	doc.Interfaces["wan2"] = &ngfwv1.Interface{Enabled: proto.Bool(true), Pppoe: &ngfwv1.Pppoe{Enabled: proto.Bool(true), Ipv6: proto.String("dhcpv6"), Parent: proto.String("raw2"), DelegationTargets: []*ngfwv1.PppoeDelegationTarget{{Interface: "lan2", SubnetId: 2}}}}
	doc.Interfaces["lan2"] = &ngfwv1.Interface{Enabled: proto.Bool(true)}
	other := lease
	other.Logical = "wan2"
	if got := PppoeDelegation(doc, []PppoeDelegationLease{lease, other}, now); len(got) != 0 {
		t.Fatal("overlapping WAN delegations accepted", got)
	}
}
func TestPppoeDelegationLifetimeBudgetNeverExtendsLease(t *testing.T) {
	doc, lease, now := delegationFixture()
	for _, elapsed := range []time.Duration{time.Second, 17 * time.Second, 31 * time.Second, 1790 * time.Second} {
		when := now.Add(elapsed)
		for _, kv := range PppoeDelegation(doc, []PppoeDelegationLease{lease}, when) {
			if prefix, ok := kv.Value.(*ip6nd.RaPrefix); ok {
				if when.Add(time.Duration(prefix.ValidLifetime)*time.Second).After(lease.ValidUntil) || when.Add(time.Duration(prefix.PreferredLifetime)*time.Second).After(lease.PreferredUntil) {
					t.Fatal("advertisement exceeds lease", prefix)
				}
			}
		}
	}
}
