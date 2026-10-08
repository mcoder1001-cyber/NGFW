package subsystems

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	ip6api "ngfw/agent/binapi/ip6_nd"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	iface "ngfw/agent/internal/descriptors/interface"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

type pdFaultClient struct {
	vpp.Client
	reject atomic.Bool
}

func (c *pdFaultClient) Invoke(ctx context.Context, req, reply api.Message) error {
	if r, ok := req.(*ip6api.SwInterfaceIP6ndRaPrefix); ok && !r.IsNo && c.reject.CompareAndSwap(true, false) {
		return errors.New("injected PD prefix failure")
	}
	return c.Client.Invoke(ctx, req, reply)
}
func pdProduct(t *testing.T, c vpp.Client, dir string) (*scheduler.MapRegistry, *scheduler.Scheduler, func()) {
	t.Helper()
	owned, err := ownertable.Open(dir, "wpd")
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := registerMock(reg, Env{Client: c, Owner: "wpd", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	closeW := func() { once.Do(w.Close) }
	t.Cleanup(closeW)
	w.Connected(t.Context())
	if _, ok := reg.Get(desired.PppoeDelegationAddress); !ok {
		if err = w.registerPppoeDelegation(reg, func() []desired.PppoeDelegationLease { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	return reg, scheduler.New(reg, nil), closeW
}
func pdDocLease() (*ngfwv1.DesiredState, desired.PppoeDelegationLease, time.Time) {
	now := time.Unix(2000000000, 0)
	doc := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{
		"wan": {Enabled: proto.Bool(true), Pppoe: &ngfwv1.Pppoe{Enabled: proto.Bool(true), Ipv6: proto.String("dhcpv6"), Parent: proto.String("raw"), DelegationTargets: []*ngfwv1.PppoeDelegationTarget{{Interface: "loop701", SubnetId: 2}}}},
		"raw": {Enabled: proto.Bool(true)}, "loop701": {Enabled: proto.Bool(true)},
	}}
	return doc, desired.PppoeDelegationLease{Logical: "wan", Generation: "current", Ready: true, Delegated: netip.MustParsePrefix("2001:db8:100::/56"), ValidUntil: now.Add(time.Hour), PreferredUntil: now.Add(30 * time.Minute)}, now
}
func TestPppoeDelegationProductLifecycle(t *testing.T) {
	model := coretest.New()
	model.AddInterface("loop701", "Loopback", "wpd:loop701")
	client := &pdFaultClient{Client: model}
	dir := t.TempDir()
	reg, s, closeW := pdProduct(t, client, dir)
	doc, lease, now := pdDocLease()
	aliasDescriptor, _ := reg.Get(iface.AliasName)
	aliases, err := aliasDescriptor.Retrieve(t.Context())
	if err != nil || len(aliases) != 1 {
		t.Fatalf("LAN alias unavailable: %v %v", aliases, err)
	}
	alias := aliases[0]
	scope := scheduler.Only(iface.AliasName, desired.PppoeDelegationAddress, desired.PppoeDelegationPrefix, desired.PppoeDelegationRA)
	plan := func(l desired.PppoeDelegationLease) []scheduler.KV {
		return append([]scheduler.KV{alias}, desired.PppoeDelegation(doc, []desired.PppoeDelegationLease{l}, now)...)
	}
	check := func(result *scheduler.TxnResult) {
		t.Helper()
		if result.Outcome != scheduler.OutcomeApplied {
			t.Fatalf("transaction=%+v", result)
		}
	}
	check(s.Apply(t.Context(), plan(lease), scope))
	// Static descriptors must never retrieve or overwrite dynamic objects.
	for _, name := range []string{core.InterfaceAddrName, ip6nd.RaPrefixName, ip6nd.RaConfigName} {
		static, _ := reg.Get(name)
		live, err := static.Retrieve(t.Context())
		if err != nil || len(live) != 0 {
			t.Fatalf("static %s leaked %v %v", name, live, err)
		}
	}
	closeW()
	_, restarted, _ := pdProduct(t, client, dir)
	check(restarted.Apply(t.Context(), plan(lease), scope))
	// Replacing a delegation is one scheduler transaction. A failed new prefix
	// restores the original address/prefix/RA rather than leaving a partial LAN.
	next := lease
	next.Delegated = netip.MustParsePrefix("2001:db8:200::/56")
	client.reject.Store(true)
	failed := restarted.Apply(t.Context(), plan(next), scope)
	if failed.Outcome == scheduler.OutcomeApplied {
		t.Fatal("injected failure was ignored")
	}
	got, err := restarted.Retrieve(t.Context(), scheduler.Only(desired.PppoeDelegationAddress))
	if err != nil || len(got) != 1 || got[0].Value.(*core.InterfaceAddress).Prefix != "2001:db8:100:2::1/64" {
		t.Fatalf("rollback=%v err=%v outcome=%+v", got, err, failed)
	}
	check(restarted.Apply(t.Context(), plan(next), scope))
	next.Ready = false
	check(restarted.Apply(t.Context(), plan(next), scope))
	for _, name := range []string{desired.PppoeDelegationAddress, desired.PppoeDelegationPrefix, desired.PppoeDelegationRA} {
		got, err := restarted.Retrieve(t.Context(), scheduler.Only(name))
		if err != nil || len(got) != 0 {
			t.Fatalf("withdraw %s=%v %v", name, got, err)
		}
	}
	ledger, err := openPDOwnership(filepath.Join(dir, "pppoe-pd-wpd.json"))
	if err != nil || len(ledger.keys) != 0 {
		t.Fatalf("ownership not withdrawn: %v %v", ledger, err)
	}
}
func TestPppoeDelegationRefusesStaticAdoption(t *testing.T) {
	model := coretest.New()
	index := model.AddInterface("loop701", "Loopback", "wpd:loop701")
	model.Ifaces[index].Addrs["2001:db8:100:2::1/64"] = true
	reg, _, _ := pdProduct(t, model, t.TempDir())
	d, _ := reg.Get(desired.PppoeDelegationAddress)
	_, err := d.Create(t.Context(), &core.InterfaceAddress{Interface: "loop701", Prefix: "2001:db8:100:2::1/64"})
	if err == nil {
		t.Fatal("adopted preexisting static address")
	}
	static, _ := reg.Get(core.InterfaceAddrName)
	got, err := static.Retrieve(t.Context())
	if err != nil || len(got) != 1 {
		t.Fatalf("static address lost: %v %v", got, err)
	}
}
