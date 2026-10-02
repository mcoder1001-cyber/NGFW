package basepolicy

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/scheduler"
	"slices"
	"strings"
	"testing"
)

type pairBackend struct {
	pairs map[string]lcp.ItfPair
	log   *[]string
}

func (p *pairBackend) Name() string { return lcp.NameItfPair }
func (p *pairBackend) KeyOf(v proto.Message) scheduler.Key {
	var a lcp.ItfPair
	_ = dfkit.Decode(v, &a)
	return scheduler.Join(p.Name(), a.Interface)
}
func (*pairBackend) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (p *pairBackend) Create(_ context.Context, v proto.Message) (any, error) {
	var a lcp.ItfPair
	if err := dfkit.Decode(v, &a); err != nil {
		return nil, err
	}
	p.pairs[a.Interface] = a
	*p.log = append(*p.log, "pair:create:"+a.Interface)
	return nil, nil
}
func (*pairBackend) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}
func (p *pairBackend) Delete(_ context.Context, v proto.Message, _ any) error {
	var a lcp.ItfPair
	_ = dfkit.Decode(v, &a)
	delete(p.pairs, a.Interface)
	*p.log = append(*p.log, "pair:delete:"+a.Interface)
	return nil
}
func (p *pairBackend) Retrieve(context.Context) ([]scheduler.KV, error) {
	var out []scheduler.KV
	for _, a := range p.pairs {
		out = append(out, scheduler.KV{Key: scheduler.Join(p.Name(), a.Interface), Value: a.Proto()})
	}
	return out, nil
}

type lifecycleNft struct {
	*memberNft
	log *[]string
}

func (f *lifecycleNft) Run(ctx context.Context, c renderers.Command) (renderers.Output, error) {
	out, err := f.memberNft.Run(ctx, c)
	if len(c.Stdin) > 0 {
		parts := strings.Split(string(c.Stdin), `"`)
		*f.log = append(*f.log, "nft:"+strings.Fields(string(c.Stdin))[0]+":"+parts[1])
	}
	return out, err
}
func lifecycle(t *testing.T) (*scheduler.Scheduler, *pairBackend, *lifecycleNft, *[]string) {
	t.Helper()
	log := []string{}
	pairs := &pairBackend{pairs: map[string]lcp.ItfPair{}, log: &log}
	nft := &lifecycleNft{memberNft: &memberNft{mode: "success"}, log: &log}
	r, _ := New(nft, "mgmt0")
	d, err := NewDescriptor(r, Config{Management: "mgmt0"}, func(context.Context) ([]lcp.ItfPair, error) {
		var out []lcp.ItfPair
		for _, p := range pairs.pairs {
			out = append(out, p)
		}
		return out, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	reg.Register(pairs)
	reg.Register(d)
	engine := scheduler.New(reg, nil)
	engine.VerifyRetries = 0
	return engine, pairs, nft, &log
}
func desiredPairs(pairs ...lcp.ItfPair) []scheduler.KV {
	var out []scheduler.KV
	for _, p := range pairs {
		a := Admission{Host: p.HostIfName, Pair: string(scheduler.Join(lcp.NameItfPair, p.Interface))}
		out = append(out, scheduler.KV{Key: scheduler.Join(lcp.NameItfPair, p.Interface), Value: p.Proto()}, scheduler.KV{Key: scheduler.Join(DescriptorName, a.Host), Value: dfkit.Encode(a)})
	}
	return out
}
func apply(t *testing.T, e *scheduler.Scheduler, pairs ...lcp.ItfPair) {
	t.Helper()
	r := e.Apply(context.Background(), desiredPairs(pairs...), nil)
	if r.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("%s: %v %+v", r.Outcome, r.Err, r.Results)
	}
}
func TestSchedulerMembershipLifecycle(t *testing.T) {
	e, _, nft, log := lifecycle(t)
	a := lcp.ItfPair{Interface: "loop1", HostIfName: "tap1", HostIfType: "tap"}
	b := lcp.ItfPair{Interface: "loop2", HostIfName: "tap2", HostIfType: "tap"}
	c := lcp.ItfPair{Interface: "loop3", HostIfName: "tap3", HostIfType: "tap"}
	apply(t, e, a, b)
	*log = nil
	apply(t, e, b, c)
	want := []string{"nft:delete:tap1", "pair:delete:loop1", "pair:create:loop3", "nft:add:tap3"}
	if !slices.Equal(*log, want) {
		t.Fatalf("mixed trace %v want %v", *log, want)
	}
	if !slices.Equal(nft.state, []string{"tap2", "tap3"}) {
		t.Fatal(nft.state)
	}
	*log = nil
	b.HostIfName = "newtap"
	apply(t, e, b, c)
	want = []string{"nft:delete:tap2", "pair:delete:loop2", "pair:create:loop2", "nft:add:newtap"}
	if !slices.Equal(*log, want) {
		t.Fatalf("rename %v", *log)
	}
	*log = nil
	b.HostIfType = "tun"
	apply(t, e, b, c)
	want = []string{"nft:delete:newtap", "pair:delete:loop2", "pair:create:loop2", "nft:add:newtap"}
	if !slices.Equal(*log, want) {
		t.Fatalf("recreate %v", *log)
	}
}
func TestOrphanMembershipIsRetrievedAndRemoved(t *testing.T) {
	e, _, nft, log := lifecycle(t)
	nft.state = []string{"orphan"}
	apply(t, e)
	if len(nft.state) != 0 || fmt.Sprint(*log) != "[nft:delete:orphan]" {
		t.Fatalf("orphan retained %v %v", nft.state, *log)
	}
}
