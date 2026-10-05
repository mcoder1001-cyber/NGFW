package bfd

import (
	"context"
	binbfd "ngfw/agent/binapi/bfd"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"testing"
	"time"
)

type testEndpointClaims map[[2]string]string

func (c testEndpointClaims) Claim(l, p, n string) error { c[[2]string{l, p}] = n; return nil }
func (c testEndpointClaims) Lookup(l, p string) (string, bool) {
	n, ok := c[[2]string{l, p}]
	return n, ok
}
func (c testEndpointClaims) Release(l, p, _ string) error { delete(c, [2]string{l, p}); return nil }
func TestMultihopExactTupleEventsAndRemoval(t *testing.T) {
	f, _, sessions := fakeBFD()
	claims := testEndpointClaims{}
	calls := 0
	SetMultihopEnvironment(df7test.Owner, &MultihopEnvironment{Claims: claims, Enable: func(context.Context) error { calls++; return nil }})
	t.Cleanup(func() { SetMultihopEnvironment(df7test.Owner, nil) })
	d := NewSession(f, df7test.Owner)
	s := Session{Interface: "loop0", Local: "10.0.0.1", Peer: "10.0.0.2", Multihop: true, DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3}
	if _, err := d.Create(t.Context(), df7.Encode(s)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || df7test.Last[*binbfd.BfdUDPAdd](t, f, "bfd_udp_add").SwIfIndex != interface_types.InterfaceIndex(df7.NoIndex) {
		t.Fatal("global activation or NoIndex missing")
	}
	duplicate := s
	duplicate.Interface = "loop1"
	if _, err := d.Create(t.Context(), df7.Encode(duplicate)); err == nil || calls != 1 {
		t.Fatal("duplicate tuple accepted")
	}
	sessions["foreign"] = &binbfd.BfdUDPSessionDetails{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex), LocalAddr: mustAddr(s.Local), PeerAddr: mustAddr("10.0.0.99")}
	kvs, err := d.Retrieve(t.Context())
	if err != nil || len(kvs) != 1 {
		t.Fatalf("retrieve %v %v", kvs, err)
	}
	observed, err := Sessions(t.Context(), f, df7test.Owner)
	if err != nil || len(observed) != 1 {
		t.Fatalf("state %v %v", observed, err)
	}
	f.Reply("want_bfd_events", &binbfd.WantBfdEventsReply{})
	ctx, cancel := context.WithCancel(t.Context())
	events, err := WatchEvents(ctx, f, df7test.Owner)
	if err != nil {
		t.Fatal(err)
	}
	f.Emit(&binbfd.BfdUDPSessionEvent{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex), LocalAddr: mustAddr(s.Local), PeerAddr: mustAddr("10.0.0.99"), State: binbfd.BFD_STATE_API_UP})
	f.Emit(&binbfd.BfdUDPSessionEvent{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex), LocalAddr: mustAddr(s.Local), PeerAddr: mustAddr(s.Peer), State: binbfd.BFD_STATE_API_UP})
	select {
	case e := <-events:
		if e.Peer != s.Peer || e.Interface != s.Interface {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no exact tuple event")
	}
	cancel()
	for event := range events {
		t.Logf("drained event while subscription closes: %v", event)
	}
	if df7test.Last[*binbfd.WantBfdEvents](t, f, "want_bfd_events").EnableDisable {
		t.Fatal("listener not released")
	}
	if err := d.Delete(t.Context(), df7.Encode(s), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := claims.Lookup(s.Local, s.Peer); ok {
		t.Fatal("claim leaked")
	}
	if kvs, err := d.Retrieve(t.Context()); err != nil || len(kvs) != 0 {
		t.Fatalf("foreign tuple must remain invisible %v %v", kvs, err)
	}
	SetMultihopEnvironment(df7test.Owner, &MultihopEnvironment{Claims: claims})
	if _, err := d.Create(t.Context(), df7.Encode(s)); err == nil {
		t.Fatal("non globals owner accepted")
	}
	if calls != 1 {
		t.Fatal("non owner enabled global capability")
	}
}

func TestMultihopFailedAddDoesNotAdopt(t *testing.T) {
	f, _, _ := fakeBFD()
	claims := testEndpointClaims{}
	SetMultihopEnvironment(df7test.Owner, &MultihopEnvironment{Claims: claims, Enable: func(context.Context) error { return nil }})
	t.Cleanup(func() { SetMultihopEnvironment(df7test.Owner, nil) })
	f.Reply("bfd_udp_add", &binbfd.BfdUDPAddReply{Retval: -1})
	s := Session{Interface: "loop0", Local: "10.0.0.1", Peer: "10.0.0.2", Multihop: true, DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3}
	if _, err := NewSession(f, df7test.Owner).Create(t.Context(), df7.Encode(s)); err == nil {
		t.Fatal("failed add accepted")
	}
	if _, ok := claims.Lookup(s.Local, s.Peer); ok {
		t.Fatal("failed add adopted tuple")
	}
}
