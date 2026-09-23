package policer

import (
	"errors"
	"sort"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/policer"
	"ngfw/agent/binapi/policer_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

// fakePolicers models VPP's policer pool on the fake client: policer_add / _update / _del and
// policer_dump_v2 (all, or one index — free slots answer nothing, like VPP).
func fakePolicers(t *testing.T) (*df7test.Fake, map[uint32]*policer.PolicerDetails) {
	t.Helper()
	f := df7test.NewFake()
	pool := map[uint32]*policer.PolicerDetails{}
	next := uint32(0)
	fromCfg := func(name string, c policer_types.PolicerConfig) *policer.PolicerDetails {
		return &policer.PolicerDetails{Name: name, Cir: c.Cir, Eir: c.Eir, Cb: c.Cb, Eb: c.Eb, RateType: c.RateType, RoundType: c.RoundType,
			Type: c.Type, ColorAware: c.ColorAware, ConformAction: c.ConformAction, ExceedAction: c.ExceedAction, ViolateAction: c.ViolateAction}
	}
	f.On("policer_add", func(m api.Message) ([]api.Message, error) {
		req := m.(*policer.PolicerAdd)
		for _, p := range pool {
			if p.Name == req.Name {
				return []api.Message{&policer.PolicerAddReply{Retval: int32(api.VALUE_EXIST)}}, nil
			}
		}
		idx := next
		next++
		pool[idx] = fromCfg(req.Name, req.Infos)
		return []api.Message{&policer.PolicerAddReply{PolicerIndex: idx}}, nil
	})
	f.On("policer_update", func(m api.Message) ([]api.Message, error) {
		req := m.(*policer.PolicerUpdate)
		p, ok := pool[req.PolicerIndex]
		if !ok {
			return []api.Message{&policer.PolicerUpdateReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		pool[req.PolicerIndex] = fromCfg(p.Name, req.Infos)
		return []api.Message{&policer.PolicerUpdateReply{}}, nil
	})
	f.On("policer_del", func(m api.Message) ([]api.Message, error) {
		req := m.(*policer.PolicerDel)
		if _, ok := pool[req.PolicerIndex]; !ok {
			return []api.Message{&policer.PolicerDelReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		delete(pool, req.PolicerIndex)
		return []api.Message{&policer.PolicerDelReply{}}, nil
	})
	f.On("policer_dump_v2", func(m api.Message) ([]api.Message, error) {
		req := m.(*policer.PolicerDumpV2)
		if req.PolicerIndex != df7.NoIndex {
			if p, ok := pool[req.PolicerIndex]; ok {
				return []api.Message{p}, nil
			}
			return nil, nil
		}
		idx := make([]uint32, 0, len(pool))
		for k := range pool {
			idx = append(idx, k)
		}
		sort.Slice(idx, func(a, b int) bool { return idx[a] < idx[b] })
		out := make([]api.Message, 0, len(idx))
		for _, k := range idx {
			out = append(out, pool[k])
		}
		return out, nil
	})
	return f, pool
}

var gold = Policer{Name: "gold", CIR: 1000, EIR: 2000, CB: 16000, EB: 32000, RateType: RateKbps, RoundType: RoundClosest,
	Type: Type2R3C2698, Conform: Action{Type: ActTransmit}, Exceed: Action{Type: ActMark, DSCP: 10}, Violate: Action{Type: ActDrop}}

func TestPolicerLifecycle(t *testing.T) {
	f, pool := fakePolicers(t)
	d := NewPolicer(f, df7test.Owner)
	ctx := t.Context()

	// another owner's policer and an unprefixed one exist and must never be reported
	pool[40] = &policer.PolicerDetails{Name: df7test.Other + ":gold", Cir: 1}
	pool[41] = &policer.PolicerDetails{Name: "legacy", Cir: 1}

	desired := df7test.Desired(d, df7.Encode(gold))
	if desired.Key != "policer.policer/gold" {
		t.Fatalf("key %s", desired.Key)
	}
	if deps := d.Dependencies(desired.Value); deps != nil {
		t.Fatalf("deps %v", deps)
	}
	meta, err := d.Create(ctx, desired.Value)
	if err != nil {
		t.Fatal(err)
	}
	req := df7test.Last[*policer.PolicerAdd](t, f, "policer_add")
	if req.Name != "w0:gold" || req.Infos.Cir != 1000 || req.Infos.Type != 2 || req.Infos.ExceedAction.Type != 2 || req.Infos.ExceedAction.Dscp != 10 {
		t.Fatalf("policer_add %+v", req)
	}
	df7test.AssertEmptyPlan(t, d, desired)

	// update in place keeps the index
	g2 := gold
	g2.CIR = 1500
	m2, err := d.Update(ctx, desired.Value, df7.Encode(g2), meta)
	if err != nil || m2 != meta {
		t.Fatalf("update: %v %v", m2, err)
	}
	if up := df7test.Last[*policer.PolicerUpdate](t, f, "policer_update"); up.PolicerIndex != meta.(Meta).Index || up.Infos.Cir != 1500 {
		t.Fatalf("policer_update %+v", up)
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, df7.Encode(g2)))
	renamed := g2
	renamed.Name = "silver"
	if _, err := d.Update(ctx, df7.Encode(g2), df7.Encode(renamed), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("rename: %v", err)
	}
	if _, err := d.Update(ctx, df7.Encode(g2), df7.Encode(g2), "bad"); !errors.Is(err, df7.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}

	// a second policer appears at a later index; Retrieve finds both indices by probing
	s := gold
	s.Name = "silver"
	ms, err := d.Create(ctx, df7.Encode(s))
	if err != nil {
		t.Fatal(err)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 2 {
		t.Fatalf("retrieve %v %v", df7test.Keys(kvs), err)
	}
	for _, kv := range kvs {
		if kv.Key == "policer.policer/silver" && kv.Meta != ms {
			t.Fatalf("silver meta %v want %v", kv.Meta, ms)
		}
	}
	if idx, ok, err := LookupIndex(ctx, f, df7test.Owner, "silver"); err != nil || !ok || idx != ms.(Meta).Index {
		t.Fatalf("LookupIndex %d %v %v", idx, ok, err)
	}

	// delete by the Meta
	if err := d.Delete(ctx, df7.Encode(s), ms); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, df7.Encode(g2), meta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("left: %v", df7test.Keys(kvs))
	}
	// VPP error surfaces
	if err := d.Delete(ctx, df7.Encode(s), ms); err == nil {
		t.Fatal("delete of a missing policer must fail")
	}
	f.Reply("policer_reset", &policer.PolicerResetReply{})
	if err := Reset(ctx, f, 0); err != nil {
		t.Fatal(err)
	}
}

func TestPolicerValidate(t *testing.T) {
	bad := []Policer{
		{},
		func() Policer { p := gold; p.RateType = "bps"; return p }(),
		func() Policer { p := gold; p.RoundType = ""; return p }(),
		func() Policer { p := gold; p.Type = "x"; return p }(),
		func() Policer { p := gold; p.EIR = 10; return p }(),
		func() Policer { p := gold; p.CIR = 0; return p }(),
		func() Policer { p := gold; p.Conform = Action{Type: ActDrop, DSCP: 4}; return p }(),
		func() Policer { p := gold; p.Exceed = Action{Type: ActMark, DSCP: 64}; return p }(),
		func() Policer { p := gold; p.CB = 1 << 60; return p }(),
	}
	for i, p := range bad {
		if err := p.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	f, _ := fakePolicers(t)
	if _, err := NewPolicer(f, df7test.Owner).Create(t.Context(), df7.Encode(bad[1])); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("create invalid: %v", err)
	}
	if len(f.CallsNamed("policer_add")) != 0 {
		t.Fatal("invalid spec reached VPP")
	}
}

func TestAttachments(t *testing.T) {
	f := df7test.NewFake()
	ctx := t.Context()
	f.Reply("policer_input", &policer.PolicerInputReply{})
	f.Reply("policer_output", &policer.PolicerOutputReply{})
	d := NewInterface(f, df7test.Owner)
	in := df7.Encode(Attachment{Interface: "loop0", Direction: DirInput, Policer: "gold"})
	if k := d.KeyOf(in); k != "policer.interface/loop0/input" {
		t.Fatalf("key %s", k)
	}
	deps := d.Dependencies(in)
	if len(deps) != 2 || deps[0].Key != "policer.policer/gold" || deps[1].Key != "interface/loop0" || deps[1].Optional {
		t.Fatalf("deps %v", deps)
	}
	meta, err := d.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*policer.PolicerInput](t, f, "policer_input"); r.Name != "w0:gold" || r.SwIfIndex != 1 || !r.Apply {
		t.Fatalf("policer_input %+v", r)
	}
	out := df7.Encode(Attachment{Interface: "loop0", Direction: DirOutput, Policer: "gold"})
	if _, err := d.Create(ctx, out); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*policer.PolicerOutput](t, f, "policer_output"); !r.Apply {
		t.Fatalf("policer_output %+v", r)
	}
	if _, err := d.Update(ctx, in, df7.Encode(Attachment{Interface: "loop0", Direction: DirInput, Policer: "silver"}), meta); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*policer.PolicerInput](t, f, "policer_input"); r.Name != "w0:silver" {
		t.Fatalf("update %+v", r)
	}
	if _, err := d.Update(ctx, in, out, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("direction change: %v", err)
	}
	f.Reset()
	if err := d.Delete(ctx, in, meta); err != nil {
		t.Fatal(err)
	}
	calls := f.CallsNamed("policer_input")
	if len(calls) != 2 || !calls[0].(*policer.PolicerInput).Apply || calls[1].(*policer.PolicerInput).Apply {
		t.Fatalf("delete = apply then un-apply, got %v", calls)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatalf("retrieve: %v", err)
	}
	// foreign and unknown interfaces are refused
	if _, err := d.Create(ctx, df7.Encode(Attachment{Interface: "loop9", Direction: DirInput, Policer: "gold"})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatalf("foreign: %v", err)
	}
	if _, err := d.Create(ctx, df7.Encode(Attachment{Interface: "eth0", Direction: DirInput, Policer: "gold"})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatalf("untagged without claim: %v", err)
	}
	if _, err := NewInterface(f, df7test.Owner, df7.WithClaimUntagged(true)).Create(ctx, df7.Encode(Attachment{Interface: "eth0", Direction: DirInput, Policer: "gold"})); err != nil {
		t.Fatalf("untagged with claim: %v", err)
	}
	if _, err := d.Create(ctx, df7.Encode(Attachment{Interface: "nope", Direction: DirInput, Policer: "gold"})); !errors.Is(err, df7.ErrNoSuchInterface) {
		t.Fatalf("missing: %v", err)
	}
}

func TestBind(t *testing.T) {
	f := df7test.NewFake()
	f.Reply("policer_bind", &policer.PolicerBindReply{})
	d := NewBind(f, df7test.Owner)
	v := df7.Encode(Bind{Policer: "gold", Worker: 1})
	if d.KeyOf(v) != "policer.bind/gold" || d.Dependencies(v)[0].Key != "policer.policer/gold" {
		t.Fatal("key/deps")
	}
	if _, err := d.Create(t.Context(), v); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*policer.PolicerBind](t, f, "policer_bind"); r.Name != "w0:gold" || r.WorkerIndex != 1 || !r.BindEnable {
		t.Fatalf("%+v", r)
	}
	if err := d.Delete(t.Context(), v, nil); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*policer.PolicerBind](t, f, "policer_bind"); r.BindEnable {
		t.Fatalf("%+v", r)
	}
	f.Reply("policer_bind", &policer.PolicerBindReply{Retval: int32(api.INVALID_WORKER)})
	if _, err := d.Create(t.Context(), v); !df7.IsVPPError(err, api.INVALID_WORKER) {
		t.Fatalf("no workers: %v", err)
	}
	if _, err := d.Retrieve(t.Context()); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
}

func TestClassify(t *testing.T) {
	f := df7test.NewFake()
	f.Reply("policer_classify_set_interface", &classify.PolicerClassifySetInterfaceReply{})
	tables := map[string]uint32{"cls4": 7, "cls6": 8}
	d := NewClassify(f, df7test.Owner, df7.WithClassifyTables(func(n string) (uint32, bool) { v, ok := tables[n]; return v, ok }))
	v := df7.Encode(Classify{Interface: "loop1", IP4Table: "cls4", IP6Table: "cls6"})
	deps := d.Dependencies(v)
	if len(deps) != 3 || deps[1].Key != "classify.table/cls4" || deps[2].Key != "classify.table/cls6" {
		t.Fatalf("deps %v", deps)
	}
	meta, err := d.Create(t.Context(), v)
	if err != nil {
		t.Fatal(err)
	}
	r := df7test.Last[*classify.PolicerClassifySetInterface](t, f, "policer_classify_set_interface")
	if r.SwIfIndex != 2 || r.IP4TableIndex != 7 || r.IP6TableIndex != 8 || r.L2TableIndex != df7.NoIndex || !r.IsAdd {
		t.Fatalf("%+v", r)
	}
	if _, err := d.Update(t.Context(), v, v, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(t.Context(), v, meta); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*classify.PolicerClassifySetInterface](t, f, "policer_classify_set_interface"); r.IsAdd {
		t.Fatal("delete must send is_add=0")
	}
	if _, err := d.Create(t.Context(), df7.Encode(Classify{Interface: "loop1", L2Table: "nope"})); !errors.Is(err, ErrNoTable) {
		t.Fatalf("unknown table: %v", err)
	}
	if _, err := NewClassify(f, df7test.Owner).Create(t.Context(), v); !errors.Is(err, ErrNoTable) {
		t.Fatalf("no resolver: %v", err)
	}
	if err := (Classify{Interface: "x"}).Validate(); !errors.Is(err, df7.ErrSpec) {
		t.Fatal("empty classify must be invalid")
	}
	if _, err := d.Retrieve(t.Context()); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
}

func TestRegister(t *testing.T) {
	r := scheduler.NewRegistry()
	Register(r, df7test.NewFake(), df7test.Owner)
	if got := r.Names(); len(got) != 4 || got[0] != NamePolicer {
		t.Fatalf("registered %v", got)
	}
}
