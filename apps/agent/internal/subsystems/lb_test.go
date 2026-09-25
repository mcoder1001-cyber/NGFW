package subsystems

// F-lb wiring: the lb families in the product registry (lb.conf only in the globals owner, D-071), the TD-11b guard
// over them, and D-090 (2): the globals owner runs VPP's lb garbage collection once, lb.GCDelay after the last lb
// delete; a slot agent never sends it.

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lb"
	"ngfw/agent/binapi/lb_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	lbd "ngfw/agent/internal/descriptors/lb"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// lbWiring registers the product wiring (globals owner or not) over a fake VPP that answers the lb messages the
// descriptors and lb.GarbageCollect send, and counts the cli_inband calls.
func lbWiring(t *testing.T, owner string, globals bool) (*scheduler.MapRegistry, func() []string) {
	t.Helper()
	root := t.TempDir()
	if err := bootid.WriteFakeProc(root, "boot-lb", map[int]uint64{4732: 7}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bootid.SetProcRoot(root))
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, owner)
	if err != nil {
		t.Fatal(err)
	}
	v := ifacetest.New()
	v.Reply("control_ping", &memclnt.ControlPingReply{VpePID: 4732})
	var mu sync.Mutex
	var cli []string
	v.Reply("lb_add_del_vip_v2", &lb.LbAddDelVipV2Reply{})
	v.Reply("lb_add_del_as", &lb.LbAddDelAsReply{})
	v.Reply("lb_vip_dump", &lb.LbVipDetails{Vip: lb_types.LbVip{Pfx: df7ToPfx("10.2.250.1/32"), Port: 80}, Encap: lb_types.LbEncapType(lb_types.LB_API_VIP_TYPE_IP4_GRE4)})
	v.Reply("lb_as_dump")
	v.On("cli_inband", func(m api.Message) ([]api.Message, error) {
		mu.Lock()
		defer mu.Unlock()
		cli = append(cli, m.(*vlib.CliInband).Cmd)
		return []api.Message{&vlib.CliInbandReply{Retval: -1, Reply: "lb_vip_find_index error -6"}}, nil
	})
	reg := scheduler.NewRegistry()
	w, err := Register(reg, Env{Client: v, Owner: owner, StateDir: dir, Owned: owned, GlobalsOwner: globals,
		NetdevKind: func(string) (string, bool, error) { return "veth", true, nil }})
	if err != nil {
		t.Fatal(err) // includes the TD-11b guard over the lb descriptors
	}
	w.Connected(context.Background())
	return reg, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), cli...)
	}
}

func df7ToPfx(s string) ip_types.AddressWithPrefix {
	p := df7.ToAddressWithPrefix(netip.MustParsePrefix(s))
	p.Len += 96 // ip46 length of an IPv4 VIP
	return p
}

// shortGC makes the collection run d after the last delete for the duration of the test.
func shortGC(t *testing.T, d time.Duration) {
	prev := lbGCDelay
	lbGCDelay = d
	t.Cleanup(func() { lbGCDelay = prev })
}

// createDelete creates and deletes one VIP and one AS through the registered descriptors (a transaction that removed
// lb objects).
func createDelete(t *testing.T, reg *scheduler.MapRegistry) {
	t.Helper()
	ctx := context.Background()
	vip := lbd.VIP{Prefix: "10.2.250.1/32", Protocol: lbd.ProtoTCP, Port: 80}
	vd, _ := reg.ForKey(lbd.KeyVIP(vip))
	ad, _ := reg.ForKey(lbd.KeyAS(vip, "10.2.2.10"))
	v := df7.Encode(lbd.VIPSpec{VIP: vip, Encap: lbd.EncapGRE4})
	a := df7.Encode(lbd.AS{VIP: vip, Address: "10.2.2.10"})
	if _, err := vd.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := ad.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := ad.Delete(ctx, a, nil); err != nil {
		t.Fatal(err)
	}
	if err := vd.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
}

// Envelope obligation (D-090, D-071): a slot agent (VRX_GLOBALS_OWNER=0) never sends the collection and never
// registers lb.conf.
func TestLbSlotAgentNeverCollects(t *testing.T) {
	shortGC(t, 10*time.Millisecond)
	reg, cli := lbWiring(t, "w2s", false)
	if _, ok := reg.ForKey(lbd.KeyConf()); ok {
		t.Fatal("a slot agent registered lb.conf (D-071)")
	}
	for _, k := range []scheduler.Key{lbd.KeyVIP(lbd.VIP{Prefix: "10.2.250.1/32", Protocol: "tcp", Port: 80}), lbd.KeyIntfNat("loop0", "ip4")} {
		if _, ok := reg.ForKey(k); !ok {
			t.Fatalf("%s not registered", k)
		}
	}
	createDelete(t, reg)
	time.Sleep(200 * time.Millisecond)
	if got := cli(); len(got) != 0 {
		t.Fatalf("slot agent sent cli_inband %v", got)
	}
}

// The globals owner: lb.conf registered; a burst of lb deletes → exactly one lb.GCCommand, lbGCDelay after the last.
func TestLbGlobalsOwnerCollectsOnce(t *testing.T) {
	shortGC(t, 2*time.Second) // long enough that both bursts (fsynced records) land inside one window under CI load
	reg, cli := lbWiring(t, "w2g", true)
	if _, ok := reg.ForKey(lbd.KeyConf()); !ok {
		t.Fatal("the globals owner did not register lb.conf")
	}
	createDelete(t, reg)
	createDelete(t, reg) // a second delete inside the window re-arms the same timer
	if got := cli(); len(got) != 0 {
		t.Fatalf("collected before the delay: %v", got)
	}
	deadline := time.Now().Add(15 * time.Second)
	for len(cli()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)
	if got := cli(); len(got) != 1 || got[0] != lbd.GCCommand {
		t.Fatalf("want one %q, got %v", lbd.GCCommand, got)
	}
	// a create alone schedules nothing
	vip := lbd.VIP{Prefix: "10.2.250.1/32", Protocol: lbd.ProtoTCP, Port: 80}
	vd, _ := reg.ForKey(lbd.KeyVIP(vip))
	if _, err := vd.Create(context.Background(), df7.Encode(lbd.VIPSpec{VIP: vip, Encap: lbd.EncapGRE4})); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if got := cli(); len(got) != 1 {
		t.Fatalf("a create triggered a collection: %v", got)
	}
}

// The services domain of this build carries the lb descriptors (Health.subsystems).
func TestLbDomain(t *testing.T) {
	if DomainOf(lbd.NameVIP) != Services || DomainOf(lbd.NameAS) != Services || DomainOf(lbd.NameIntfNat) != Services || DomainOf(lbd.NameConf) != Services {
		t.Fatal(Domains[Services])
	}
	if LbEnv().GlobalsOwner != lbGlobalsOwner.Load() {
		t.Fatal("LbEnv")
	}
}
