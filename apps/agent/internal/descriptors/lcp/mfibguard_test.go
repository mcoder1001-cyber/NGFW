package lcp

import (
	"context"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lcp"
	"ngfw/agent/binapi/mfib_types"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// mfibModel is linux-cp's (*,224.0.0.0/24) in table 0: the sw_if_indexes with an Accept path.
type mfibModel struct {
	accept map[uint32]bool
	log    []string
}

func (m *mfibModel) install(f *dfkittest.FakeVPP) {
	f.On("ip_mroute_dump", func(api.Message) ([]api.Message, error) {
		r := ip.IPMroute{Prefix: ip_types.Mprefix{Af: ip_types.ADDRESS_IP4, GrpAddressLength: 24,
			GrpAddress: ip_types.AddressUnionIP4(ip_types.IP4Address{224, 0, 0, 0})}}
		r.Paths = append(r.Paths, mfib_types.MfibPath{ItfFlags: mfib_types.MFIB_API_ITF_FLAG_FORWARD,
			Path: fib_types.FibPath{SwIfIndex: ^uint32(0)}})
		for i := range m.accept {
			r.Paths = append(r.Paths, mfib_types.MfibPath{ItfFlags: mfib_types.MFIB_API_ITF_FLAG_ACCEPT,
				Path: fib_types.FibPath{SwIfIndex: i}})
		}
		return []api.Message{&ip.IPMrouteDetails{Route: r}}, nil
	})
}

// flusher is linux-cp hearing RTM_DELADDR while the pair exists: it drops the Accept path of phy.
type flusher struct {
	m       *mfibModel
	pairs   map[uint32]bool
	phy     uint32
	works   bool
	ifindex int
}

func (f *flusher) FlushIPv4(ifindex int, name string) (int, error) {
	f.m.log = append(f.m.log, "flush "+name)
	f.ifindex = ifindex
	if f.works && f.pairs[f.phy] {
		delete(f.m.accept, f.phy)
	}
	return 1, nil
}

func setup(t *testing.T, works bool, netns string) (*mfibModel, *ItfPairDescriptor, *model) {
	t.Helper()
	f, m := newFake()
	mm := &mfibModel{accept: map[uint32]bool{7: true, 1: true}} // 1: another slot's stale path
	mm.install(f)
	pairsNow := map[uint32]bool{}
	fl := &flusher{m: mm, pairs: pairsNow, phy: 7, works: works}
	t.Cleanup(func() {
		if fl.ifindex != 0 && fl.ifindex != 121 { // model: first pair's vif_index is 100+21
			t.Errorf("flush by ifindex %d, want the pair's vif_index 121", fl.ifindex)
		}
	})
	d := NewItfPair(f, "w5", WithHostAddrFlusher(fl), WithMfibWait(100*time.Millisecond))
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: netns}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	for k := range m.pairs {
		pairsNow[k] = true
	}
	// record the pair delete in the order log and drop the pair from the flusher's view
	f.On("lcp_itf_pair_add_del_v3", func(api.Message) ([]api.Message, error) {
		mm.log = append(mm.log, "pair-del")
		delete(pairsNow, 7)
		delete(m.pairs, 7)
		return []api.Message{&lcp.LcpItfPairAddDelV3Reply{}}, nil
	})
	return mm, d, m
}

func TestDeleteDropsStaleAccept(t *testing.T) {
	mm, d, _ := setup(t, true, "")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
	if err := d.Delete(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	if mm.accept[7] {
		t.Fatal("Accept on sw_if_index 7 left after pair delete")
	}
	if !mm.accept[1] {
		t.Fatal("another interface's path was touched")
	}
	if len(mm.log) != 2 || mm.log[0] != "flush w5-lcp0" || mm.log[1] != "pair-del" {
		t.Fatalf("order %v, want flush before pair-del", mm.log)
	}
	ok, err := StaleAccept(context.Background(), d.client, 7)
	if err != nil || ok {
		t.Fatal(ok, err)
	}
}

func TestDeleteStillDeletesWhenAcceptStays(t *testing.T) {
	mm, d, m := setup(t, false, "")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
	if err := d.Delete(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.pairs[7]; ok || mm.log[len(mm.log)-1] != "pair-del" {
		t.Fatalf("pair not deleted: %v", mm.log)
	}
}

func TestDeleteNetnsPairSkipsGuard(t *testing.T) {
	mm, d, _ := setup(t, true, "ns-w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	if err := d.Delete(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	if len(mm.log) != 1 || mm.log[0] != "pair-del" {
		t.Fatalf("netns pair: %v, want no flush", mm.log)
	}
}
