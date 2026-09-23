package pnat_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	pnatapi "ngfw/agent/binapi/pnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/descriptors/pnat"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type flowKey struct {
	sw    uint32
	point pnatapi.PnatAttachmentPoint
	match pnatapi.PnatMatchTuple
}

// fakePnat models pnat's pool (with holes), the cursor semantics of pnat_bindings_get,
// the flow hash and the crash conditions of VPP 26.06.
type fakePnat struct {
	*fake.Client
	pool     []*pnatapi.PnatBindingsDetails // nil = free slot
	flows    map[flowKey]uint32
	ifaces   map[uint32]int // sw_if_index → refcount
	eagainAt int            // split a get reply after this many details (0 = never)
}

func newFakePnat(t *testing.T) *fakePnat {
	f := &fakePnat{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), flows: map[flowKey]uint32{}, ifaces: map[uint32]int{}}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop950", Tag: "w9:loop950"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 2, InterfaceName: "loop951", Tag: "w9:loop951"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop350", Tag: "w3:loop350"})
	f.On("pnat_binding_add_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*pnatapi.PnatBindingAddV2)
		d := &pnatapi.PnatBindingsDetails{Match: r.Match, Rewrite: r.Rewrite}
		for i, s := range f.pool {
			if s == nil {
				f.pool[i] = d
				return []api.Message{&pnatapi.PnatBindingAddV2Reply{BindingIndex: uint32(i)}}, nil //nolint:gosec // test
			}
		}
		f.pool = append(f.pool, d)
		return []api.Message{&pnatapi.PnatBindingAddV2Reply{BindingIndex: uint32(len(f.pool) - 1)}}, nil //nolint:gosec // test
	})
	f.On("pnat_binding_del", func(req api.Message) ([]api.Message, error) {
		i := req.(*pnatapi.PnatBindingDel).BindingIndex
		if int(i) >= len(f.pool) || f.pool[i] == nil {
			return []api.Message{&pnatapi.PnatBindingDelReply{Retval: -1}}, nil
		}
		f.pool[i] = nil
		return []api.Message{&pnatapi.PnatBindingDelReply{}}, nil
	})
	f.On("pnat_bindings_get", func(req api.Message) ([]api.Message, error) {
		c := int(req.(*pnatapi.PnatBindingsGet).Cursor)
		var out []api.Message
		n := 0
		for i := c; i < len(f.pool); i++ {
			if f.pool[i] == nil {
				continue
			}
			if f.eagainAt > 0 && n == f.eagainAt {
				return append(out, &pnatapi.PnatBindingsGetReply{Retval: int32(api.EAGAIN), Cursor: uint32(i)}), nil //nolint:gosec // test
			}
			out = append(out, f.pool[i])
			n++
		}
		rep := &pnatapi.PnatBindingsGetReply{Cursor: ^uint32(0)}
		if n == 0 {
			rep.Retval = int32(api.INVALID_VALUE)
		}
		return append(out, rep), nil
	})
	f.On("pnat_interfaces_get", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for sw := uint32(0); sw < 10; sw++ {
			if f.ifaces[sw] > 0 {
				out = append(out, &pnatapi.PnatInterfacesDetails{SwIfIndex: interface_types.InterfaceIndex(sw), Enabled: []bool{true, false}})
			}
		}
		return append(out, &pnatapi.PnatInterfacesGetReply{Cursor: ^uint32(0)}), nil
	})
	f.On("pnat_binding_attach", func(req api.Message) ([]api.Message, error) {
		r := req.(*pnatapi.PnatBindingAttach)
		if int(r.BindingIndex) >= len(f.pool) || f.pool[r.BindingIndex] == nil {
			return []api.Message{&pnatapi.PnatBindingAttachReply{Retval: -1}}, nil
		}
		k := flowKey{uint32(r.SwIfIndex), r.Attachment, f.pool[r.BindingIndex].Match}
		if _, dup := f.flows[k]; dup {
			return []api.Message{&pnatapi.PnatBindingAttachReply{Retval: -3}}, nil
		}
		f.flows[k] = r.BindingIndex
		f.ifaces[uint32(r.SwIfIndex)]++
		return []api.Message{&pnatapi.PnatBindingAttachReply{}}, nil
	})
	f.On("pnat_binding_detach", func(req api.Message) ([]api.Message, error) {
		if len(f.ifaces) == 0 {
			t.Fatal("pnat_binding_detach with an uninitialised flow hash would crash VPP 26.06")
		}
		r := req.(*pnatapi.PnatBindingDetach)
		k := flowKey{uint32(r.SwIfIndex), r.Attachment, f.pool[r.BindingIndex].Match}
		if _, ok := f.flows[k]; !ok {
			return []api.Message{&pnatapi.PnatBindingDetachReply{Retval: -2}}, nil
		}
		delete(f.flows, k)
		if f.ifaces[uint32(r.SwIfIndex)]--; f.ifaces[uint32(r.SwIfIndex)] == 0 {
			delete(f.ifaces, uint32(r.SwIfIndex))
		}
		return []api.Message{&pnatapi.PnatBindingDetachReply{}}, nil
	})
	f.On("pnat_flow_lookup", func(req api.Message) ([]api.Message, error) {
		if len(f.ifaces) == 0 {
			t.Fatal("pnat_flow_lookup with an uninitialised flow hash would crash VPP 26.06")
		}
		r := req.(*pnatapi.PnatFlowLookup)
		if i, ok := f.flows[flowKey{uint32(r.SwIfIndex), r.Attachment, r.Match}]; ok {
			return []api.Message{&pnatapi.PnatFlowLookupReply{BindingIndex: i}}, nil
		}
		return []api.Message{&pnatapi.PnatFlowLookupReply{Retval: -1, BindingIndex: ^uint32(0)}}, nil
	})
	return f
}

func binding(src, dst string, dport uint32, rwDst string) *pnat.BindingSpec {
	return &pnat.BindingSpec{Match: pnat.MatchSpec{Src: src, Dst: dst, Proto: "udp", DstPort: dport}, Rewrite: pnat.RewriteSpec{Dst: rwDst}}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	pnat.Register(reg, newFakePnat(t), "w9")
	if reg.Len() != 2 {
		t.Fatalf("registered %d", reg.Len())
	}
}

func TestBindingIndexRecovery(t *testing.T) {
	f := newFakePnat(t)
	p := pnat.New(f, "w9")
	// pool with holes and a foreign binding: [w9, free, w3, free, free, w9, w9]
	add := func(b *pnat.BindingSpec) {
		m, _ := p.Binding.Create(context.Background(), natcommon.MustEncode(b))
		_ = m
	}
	for i := 0; i < 7; i++ {
		add(binding("10.9.51.1", "10.9.52.1", uint32(1000+i), "10.9.53.1"))
	}
	f.pool[2] = &pnatapi.PnatBindingsDetails{Match: pnatapi.PnatMatchTuple{Src: [4]uint8{10, 3, 0, 1}, Mask: pnatapi.PNAT_SA}, Rewrite: pnatapi.PnatRewriteTuple{Src: [4]uint8{10, 3, 0, 2}, Mask: pnatapi.PNAT_SA}}
	f.pool[1], f.pool[3], f.pool[4] = nil, nil, nil
	for _, eagain := range []int{0, 2} {
		f.eagainAt = eagain
		kvs, err := p.Binding.Retrieve(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]uint32{"udp/10.9.51.1/any/10.9.52.1/1000": 0, "udp/10.9.51.1/any/10.9.52.1/1005": 5, "udp/10.9.51.1/any/10.9.52.1/1006": 6}
		if len(kvs) != len(want) {
			t.Fatalf("eagain=%d: %d bindings %v", eagain, len(kvs), kvs)
		}
		for _, kv := range kvs {
			id := string(kv.Key)[len("pnat.binding/"):]
			if kv.Meta.(pnat.BindingMeta).Index != want[id] {
				t.Fatalf("eagain=%d: %s index %d, want %d", eagain, id, kv.Meta.(pnat.BindingMeta).Index, want[id])
			}
		}
	}
}

func TestBindingAndAttachment(t *testing.T) {
	f := newFakePnat(t)
	p := pnat.New(f, "w9")
	ctx := context.Background()

	// nothing attached anywhere: Retrieve must not probe the flow hash (fake fails if it does)
	if len(nattest.Keys(t, p.Attachment)) != 0 {
		t.Fatal("attachments on an empty VPP")
	}
	b1 := natcommon.MustEncode(binding("10.9.51.1", "10.9.52.1", 53, "10.9.53.1"))
	b2 := natcommon.MustEncode(&pnat.BindingSpec{Match: pnat.MatchSpec{Dst: "10.9.52.2", Proto: "17", DstPort: 53},
		Rewrite: pnat.RewriteSpec{Dst: "10.9.53.2", DstPort: 5353, CopyByte: true, FromOffset: 1, ToOffset: 2}})
	if nattest.Apply(t, p.Binding, b1, b2) != 2 || nattest.Apply(t, p.Binding, b1, b2) != 0 {
		t.Fatal("bindings create / idempotent")
	}
	if keys := nattest.Keys(t, p.Binding); len(keys) != 2 || keys[0] != "pnat.binding/udp/10.9.51.1/any/10.9.52.1/53" || keys[1] != "pnat.binding/udp/any/any/10.9.52.2/53" {
		t.Fatalf("keys %v", keys)
	}
	req := f.CallsNamed("pnat_binding_add_v2")[1].(*pnatapi.PnatBindingAddV2)
	if req.Match.Mask != pnatapi.PNAT_DA|pnatapi.PNAT_PROTO|pnatapi.PNAT_DPORT || req.Rewrite.Mask != pnatapi.PNAT_DA|pnatapi.PNAT_DPORT|pnatapi.PNAT_COPY_BYTE || req.Rewrite.ToOffset != 2 {
		t.Fatalf("binding request %+v", req)
	}
	// rewrite change → recreate
	b1b := natcommon.MustEncode(binding("10.9.51.1", "10.9.52.1", 53, "10.9.53.9"))
	if _, err := p.Binding.Update(ctx, b1, b1b, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("update: %v", err)
	}
	for _, bad := range []*pnat.BindingSpec{
		{Match: pnat.MatchSpec{Dst: "10.9.52.1", DstPort: 53}, Rewrite: pnat.RewriteSpec{Dst: "10.9.53.1"}}, // ports without tcp/udp
		{Match: pnat.MatchSpec{Dst: "10.9.52.1"}},                                                           // empty rewrite
		{Rewrite: pnat.RewriteSpec{Dst: "10.9.53.1"}},                                                       // empty match
	} {
		if _, err := p.Binding.Create(ctx, natcommon.MustEncode(bad)); err == nil {
			t.Fatalf("invalid binding accepted: %+v", bad)
		}
	}

	a1 := natcommon.MustEncode(&pnat.AttachmentSpec{Interface: "loop950", Point: pnat.PointInput, Binding: "udp/10.9.51.1/any/10.9.52.1/53"})
	a2 := natcommon.MustEncode(&pnat.AttachmentSpec{Interface: "loop951", Point: pnat.PointOutput, Binding: "udp/any/any/10.9.52.2/53"})
	if deps := p.Attachment.Dependencies(a1); len(deps) != 2 || deps[0].Key != "pnat.binding/udp/10.9.51.1/any/10.9.52.1/53" || deps[1].Key != "interface/loop950" {
		t.Fatalf("deps %+v", deps)
	}
	if nattest.Apply(t, p.Attachment, a1, a2) != 2 || nattest.Apply(t, p.Attachment, a1, a2) != 0 {
		t.Fatal("attachments create / idempotent")
	}
	if keys := nattest.Keys(t, p.Attachment); len(keys) != 2 || keys[0] != "pnat.attachment/loop950/input/udp/10.9.51.1/any/10.9.52.1/53" {
		t.Fatalf("attachment keys %v", keys)
	}
	if _, err := p.Attachment.Create(ctx, natcommon.MustEncode(&pnat.AttachmentSpec{Interface: "loop950", Point: pnat.PointInput, Binding: "tcp/any/any/any/1"})); !errors.Is(err, pnat.ErrNoBinding) {
		t.Fatalf("unknown binding: %v", err)
	}
	// a w3 attachment on w3's interface is invisible
	f.flows[flowKey{3, pnatapi.PNAT_IP4_INPUT, f.pool[0].Match}] = 0
	f.ifaces[3]++
	if nattest.Apply(t, p.Attachment, a1) != 1 || nattest.Apply(t, p.Attachment) != 1 || len(f.flows) != 1 {
		t.Fatalf("attachment deletes, foreign kept: %v", f.flows)
	}
	if nattest.Apply(t, p.Binding) != 2 {
		t.Fatal("binding deletes")
	}
	// detach refused (not sent) when the flow hash can't exist
	delete(f.ifaces, 3)
	f.flows = map[flowKey]uint32{}
	if err := p.Attachment.Delete(ctx, a1, pnat.AttachMeta{SwIfIndex: 1}); !errors.Is(err, pnat.ErrFlowHashUninitialised) {
		t.Fatalf("detach guard: %v", err)
	}
}
