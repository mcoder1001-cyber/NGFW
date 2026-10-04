package lcp_osi

import (
	"context"
	"go.fd.io/govpp/api"
	lcpapi "ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp/fake"
	"testing"
)

func TestOwnerAndOptInBoundary(t *testing.T) {
	for _, tc := range []struct{ owner, opt bool }{{false, false}, {false, true}, {true, false}} {
		f := fake.New()
		d := New(f, dfkit.GlobalsOwner(tc.owner), tc.opt)
		if _, e := d.Create(context.Background(), Value()); e == nil {
			t.Fatal("unauthorized create")
		}
		if _, e := d.Update(context.Background(), Value(), Value(), nil); e == nil {
			t.Fatal("unauthorized update")
		}
		if len(f.Calls()) != 0 {
			t.Fatal("unauthorized API")
		}
	}
}
func TestRetrieveIdempotenceAndNoDisable(t *testing.T) {
	f := fake.New()
	on := false
	f.On("lcp_osi_proto_get", func(api.Message) ([]api.Message, error) {
		r := &lcpapi.LcpOsiProtoGetReply{}
		if on {
			r.Count = 1
			r.OsiProtos = []byte{ProtocolISIS}
		}
		return []api.Message{r}, nil
	})
	f.On("lcp_osi_proto_enable", func(req api.Message) ([]api.Message, error) {
		if req.(*lcpapi.LcpOsiProtoEnable).OsiProto != ProtocolISIS {
			t.Fatal("wrong protocol")
		}
		on = true
		return []api.Message{&lcpapi.LcpOsiProtoEnableReply{}}, nil
	})
	d := New(f, dfkit.GlobalsOwner(true), true)
	ctx := context.Background()
	for range 2 {
		if _, e := d.Create(ctx, Value()); e != nil {
			t.Fatal(e)
		}
	}
	if len(f.CallsNamed("lcp_osi_proto_enable")) != 1 {
		t.Fatal("idempotence")
	}
	rows, e := d.Retrieve(ctx)
	if e != nil || len(rows) != 1 || rows[0].Key != Key {
		t.Fatal(rows, e)
	}
	before := len(f.Calls())
	if e := d.Delete(ctx, Value(), nil); e != nil {
		t.Fatal(e)
	}
	if !on || before != len(f.Calls()) {
		t.Fatal("delete cannot disable")
	}
}
