package bond_test

import (
	"context"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func keyOf(kv scheduler.KV) string { return string(kv.Key) }

func has(kvs []scheduler.KV, key string) bool {
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return true
		}
	}
	return false
}

func create(t *testing.T, d scheduler.Descriptor, desired proto.Message) any {
	t.Helper()
	ctx := context.Background()
	key := string(d.KeyOf(desired))
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatalf("%s Create %s: %v", d.Name(), key, err)
	}
	t.Cleanup(func() {
		if err := d.Delete(ctx, desired, meta); err != nil {
			t.Errorf("%s Delete %s: %v", d.Name(), key, err)
			return
		}
		if kvs, _ := d.Retrieve(ctx); has(kvs, key) {
			t.Errorf("%s: %s still retrieved after Delete", d.Name(), key)
		}
	})
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	if got := ifacetest.Find(t, kvs, key, keyOf); !proto.Equal(got.Value, iface.Normalize(d, desired)) {
		t.Fatalf("%s Retrieve %s = %v, want %v", d.Name(), key, got.Value, desired)
	}
	t.Logf("%s: Retrieve == desired: %s", d.Name(), key)
	return meta
}

func TestBondOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	td := tapv2.New(c, owner)
	var members []string
	for _, i := range []int{20, 21} {
		name := vpptest.Name(t, "tap"+strconv.Itoa(i))
		tap := &tapv2.Tap{Name: name, Id: vpptest.LoopbackInstance(t, i), HostIfName: name, RxRingSize: 256, TxRingSize: 256}
		create(t, td, tap)
		members = append(members, string(td.KeyOf(tap)))
	}
	bd := bond.NewBond(c, owner)
	lacp := &bond.Bond{Name: vpptest.Name(t, "bond20"), Id: vpptest.LoopbackInstance(t, 20), Mode: bond.Mode_MODE_LACP, Lb: bond.LoadBalance_LOAD_BALANCE_L34}
	create(t, bd, lacp)
	// VPP forces the lb algorithm for round-robin: Retrieve must reflect that
	create(t, bd, &bond.Bond{Name: vpptest.Name(t, "bond21"), Id: vpptest.LoopbackInstance(t, 21), Mode: bond.Mode_MODE_ROUND_ROBIN, Lb: bond.LoadBalance_LOAD_BALANCE_ROUND_ROBIN})

	md := bond.NewMember(c, owner)
	create(t, md, &bond.Member{Bond: string(bd.KeyOf(lacp)), Interface: members[0]})
	create(t, md, &bond.Member{Bond: string(bd.KeyOf(lacp)), Interface: members[1], Passive: true, LongTimeout: true})
	t.Logf("bond %s (BondEthernet%d) with members %v configured; vppctl show bond details", lacp.Name, lacp.Id, members)
	ifacetest.Hold(t)
}
