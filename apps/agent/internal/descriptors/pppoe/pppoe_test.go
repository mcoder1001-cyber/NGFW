package pppoe_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	pppoeapi "ngfw/agent/binapi/pppoe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/scheduler"
)

type fakePPPoE struct {
	*df6test.FakeVPP
	sessions map[uint32]*pppoeapi.PppoeSessionDetails
	cp       map[uint32]int // device-input feature count per interface (VPP does not dedupe)
}

func newFakePPPoE() *fakePPPoE {
	f := &fakePPPoE{FakeVPP: df6test.NewFakeVPP(), sessions: map[uint32]*pppoeapi.PppoeSessionDetails{}, cp: map[uint32]int{}}
	n := 0
	f.On("pppoe_add_del_session", func(req api.Message) ([]api.Message, error) {
		r := req.(*pppoeapi.PppoeAddDelSession)
		if r.IsAdd {
			idx := f.AddInterface(fmt.Sprintf("pppoe_session%d", n), "")
			n++
			f.sessions[idx] = &pppoeapi.PppoeSessionDetails{SwIfIndex: interface_types.InterfaceIndex(idx), SessionID: r.SessionID, ClientIP: r.ClientIP, DecapVrfID: r.DecapVrfID, ClientMac: r.ClientMac, EncapIfIndex: 1}
			return []api.Message{&pppoeapi.PppoeAddDelSessionReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
		}
		for idx, s := range f.sessions {
			if s.SessionID == r.SessionID && s.ClientMac == r.ClientMac {
				delete(f.sessions, idx)
				f.RemoveInterface(idx)
				return []api.Message{&pppoeapi.PppoeAddDelSessionReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
		return []api.Message{&pppoeapi.PppoeAddDelSessionReply{Retval: -6}}, nil
	})
	f.On("pppoe_session_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 100; idx++ {
			if s, ok := f.sessions[idx]; ok {
				c := *s
				out = append(out, &c)
			}
		}
		return out, nil
	})
	f.On("pppoe_add_del_cp", func(req api.Message) ([]api.Message, error) {
		r := req.(*pppoeapi.PppoeAddDelCp)
		f.SetFeature("device-input", "pppoe-input", uint32(r.SwIfIndex), r.IsAdd == 1)
		if r.IsAdd == 1 {
			f.cp[uint32(r.SwIfIndex)]++
		} else if f.cp[uint32(r.SwIfIndex)] > 0 {
			f.cp[uint32(r.SwIfIndex)]--
		}
		return []api.Message{&pppoeapi.PppoeAddDelCpReply{}}, nil
	})
	return f
}

func TestSessionDescriptor(t *testing.T) {
	ctx := context.Background()
	f := newFakePPPoE()
	other := f.AddInterface("pppoe_session9", "w3:02:00:00:00:00:03/3")
	f.sessions[other] = &pppoeapi.PppoeSessionDetails{SwIfIndex: interface_types.InterfaceIndex(other), SessionID: 3, ClientIP: df6test.Addr("10.3.0.2")}

	reg := scheduler.NewRegistry()
	pppoe.Register(reg, f, "w11")
	if reg.Len() != 2 {
		t.Fatalf("registered %d", reg.Len())
	}
	d := pppoe.NewSession(f, "w11")
	desired := &pppoe.Session{SessionId: 1100, ClientIp: "10.11.7.2", ClientMac: "02:11:00:00:00:01", DecapVrfId: 11001}
	if k := d.KeyOf(desired); k != "pppoe.session/02:11:00:00:00:01/1100" {
		t.Fatalf("KeyOf = %s", k)
	}
	if k := d.KeyOf(&pppoe.Session{SessionId: 1100, ClientIp: "10.11.7.2", ClientMac: "02-11-00-00-00-01"}); k != "pppoe.session/02:11:00:00:00:01/1100" {
		t.Fatalf("MAC not canonicalised in key: %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "vrf/11001" {
		t.Fatalf("deps = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tag(meta.(df6.IfMeta).SwIfIndex) != "w11:02:11:00:00:00:01/1100" {
		t.Fatalf("tag = %q", f.Tag(meta.(df6.IfMeta).SwIfIndex))
	}
	req := f.CallsNamed("pppoe_add_del_session")[0].(*pppoeapi.PppoeAddDelSession)
	if req.SessionID != 1100 || req.DecapVrfID != 11001 || req.ClientIP != df6test.Addr("10.11.7.2") || req.ClientMac.String() != "02:11:00:00:00:01" {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v", actual)
	}
	if _, err := d.Update(ctx, desired, &pppoe.Session{SessionId: 1100, ClientIp: "10.11.7.3", ClientMac: "02:11:00:00:00:01"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update = %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if a, _ := d.Retrieve(ctx); len(a) != 0 {
		t.Fatalf("after delete: %+v", a)
	}
	if !f.Has(other) {
		t.Fatal("other owner's session touched")
	}
	for _, b := range []*pppoe.Session{{ClientIp: "10.11.7.2", ClientMac: "02:11:00:00:00:01"}, {SessionId: 70000, ClientIp: "10.11.7.2", ClientMac: "02:11:00:00:00:01"}, {SessionId: 1, ClientIp: "x", ClientMac: "02:11:00:00:00:01"}, {SessionId: 1, ClientIp: "10.11.7.2", ClientMac: "zz"}} {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}

	// cp: VPP-global (review M2), globals owner only, one enable per VPP boot (D-076).
	cp := pppoe.NewCp(f, "w11")
	idx := f.AddInterface("loop1101", "w11:loop1101")
	f.AddInterface("w3-tap1", "w3:w3-tap1")
	c := &pppoe.Cp{Interface: "loop1101"}
	if k := cp.KeyOf(c); k != "pppoe.cp/global" {
		t.Fatalf("KeyOf = %s", k)
	}
	f.SetBoot(100)
	for i := 0; i < 2; i++ { // two resyncs
		if _, err := cp.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if f.cp[idx] != 1 {
		t.Fatalf("after two resyncs the feature is enabled %d times, want 1", f.cp[idx])
	}
	f.SetBoot(101) // VPP restarted: the feature is gone, re-added once
	f.cp[idx] = 0
	f.ClearFeatures(idx)
	for i := 0; i < 2; i++ {
		if _, err := cp.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if f.cp[idx] != 1 {
		t.Fatalf("after restart + two resyncs: %d, want 1", f.cp[idx])
	}
	if _, err := cp.Create(ctx, &pppoe.Cp{Interface: "w3-tap1"}); !errors.Is(err, iface.ErrForeignInterface) {
		t.Fatalf("foreign interface = %v", err)
	}
	if err := cp.Delete(ctx, c, nil); err != nil || f.cp[idx] != 0 {
		t.Fatalf("cp delete: %v %v", err, f.cp)
	}
	if err := cp.Delete(ctx, c, nil); err != nil || f.cp[idx] != 0 {
		t.Fatalf("second cp delete: %v %v", err, f.cp)
	}
	// N5: moving the CP interface disables pppoe-input on the old one.
	idxB := f.AddInterface("loop1102", "w11:loop1102")
	if _, err := cp.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := cp.Update(ctx, c, &pppoe.Cp{Interface: "loop1102"}, nil); err != nil {
		t.Fatal(err)
	}
	if a, b := f.Feature("device-input", "pppoe-input", idx), f.Feature("device-input", "pppoe-input", idxB); a != 0 || b != 1 {
		t.Fatalf("after move: old=%d new=%d, want 0/1", a, b)
	}
	if err := cp.Delete(ctx, &pppoe.Cp{Interface: "loop1102"}, nil); err != nil || f.Feature("device-input", "pppoe-input", idxB) != 0 {
		t.Fatalf("delete after move: %v", err)
	}
	// Non-owners only get the require variant: never set, never reset.
	reg = scheduler.NewRegistry()
	pppoe.Register(reg, f, "w11")
	rd, _ := reg.Get(pppoe.CpName)
	if _, err := rd.Create(ctx, c); !errors.Is(err, df6.ErrNotGlobalsOwner) {
		t.Fatalf("non-owner create = %v", err)
	}
	if err := rd.Delete(ctx, c, nil); err != nil || f.cp[idx] != 0 {
		t.Fatalf("non-owner delete: %v", err)
	}
	if ad, ok := rd.(interface{ DeleteOnAbsence() bool }); !ok || ad.DeleteOnAbsence() {
		t.Fatal("require variant must not be deleted on absence")
	}
	reg2 := scheduler.NewRegistry()
	pppoe.Register(reg2, f, "w11", df6.WithGlobalsOwner(true))
	if od, _ := reg2.Get(pppoe.CpName); od == nil {
		t.Fatal("owner setter not registered")
	}
}
