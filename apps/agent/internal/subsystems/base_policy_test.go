package subsystems

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/renderers/basepolicy"
	"ngfw/agent/internal/scheduler"
	"testing"
)

type puntSink struct {
	values   []scheduler.KV
	failures int
}

func (s *puntSink) Add(k scheduler.Key, v proto.Message, _ string) {
	s.values = append(s.values, scheduler.KV{Key: k, Value: v})
}
func (s *puntSink) Errorf(string, string, string, ...any) { s.failures++ }
func (*puntSink) Warnf(string, string, string, ...any)    {}
func TestBasePolicyProjectionActivationAndDefaultNamespace(t *testing.T) {
	owner := "punt-projection-test"
	defer basePolicyRuntimes.Delete(owner)
	pair := lcp.ItfPair{Interface: "loop1", HostIfName: "tap1", HostIfType: "tap"}
	kvs := []scheduler.KV{{Key: scheduler.Join(lcp.NameItfPair, pair.Interface), Value: pair.Proto()}}
	sink := &puntSink{}
	ProjectBasePolicy(context.Background(), owner, sink, kvs)
	if len(sink.values) != 0 || sink.failures != 0 {
		t.Fatal("disabled agent did work")
	}
	for _, namespace := range []string{"", "other"} {
		calls := 0
		basePolicyRuntimes.Store(owner, &basePolicyRuntime{config: basepolicy.Config{Management: "mgmt0"}, namespace: func(context.Context) (string, error) { calls++; return namespace, nil }})
		sink = &puntSink{}
		ProjectBasePolicy(context.Background(), owner, sink, kvs)
		want := 1
		if namespace != "" {
			want = 0
		}
		if len(sink.values) != want || calls != 1 || sink.failures != 0 {
			t.Fatalf("namespace %q: %+v calls%d", namespace, sink, calls)
		}
	}
	basePolicyRuntimes.Store(owner, &basePolicyRuntime{config: basepolicy.Config{Management: "mgmt0"}, namespace: func(context.Context) (string, error) { return "", errors.New("ambiguous readback") }})
	sink = &puntSink{}
	ProjectBasePolicy(context.Background(), owner, sink, kvs)
	if len(sink.values) != 0 || sink.failures != 1 {
		t.Fatal("unknown namespace admitted", sink)
	}
}
func TestBasePolicyActivationRequiresProductGlobalsOwner(t *testing.T) {
	t.Setenv(EnvBasePolicy, "1")
	for _, env := range []Env{{Owner: "lab", GlobalsOwner: true}, {Owner: "ngfw", GlobalsOwner: false}} {
		if err := registerBasePolicy(scheduler.NewRegistry(), &Wiring{env: env}); err == nil {
			t.Fatal("nonproduct activation accepted")
		}
	}
}

func TestBasePolicyProjectionUsesFinalDesiredNamespace(t *testing.T) {
	owner := "punt-final-namespace-test"
	defer basePolicyRuntimes.Delete(owner)
	calls := 0
	basePolicyRuntimes.Store(owner, &basePolicyRuntime{config: basepolicy.Config{Management: "mgmt0"}, namespace: func(context.Context) (string, error) { calls++; return "", nil }})
	pair := lcp.ItfPair{Interface: "loop1", HostIfName: "tap1", HostIfType: "tap"}
	kvs := []scheduler.KV{{Key: scheduler.Join(lcp.NameItfPair, pair.Interface), Value: pair.Proto()}, {Key: lcp.KeyDefaultNetns, Value: (lcp.DefaultNetns{Netns: "other"}).Proto()}}
	sink := &puntSink{}
	ProjectBasePolicy(context.Background(), owner, sink, kvs)
	if len(sink.values) != 0 || sink.failures != 0 || calls != 0 {
		t.Fatalf("old root namespace used %+v calls%d", sink, calls)
	}
	kvs[1].Value = (lcp.DefaultNetns{Netns: ""}).Proto()
	ProjectBasePolicy(context.Background(), owner, sink, kvs)
	if len(sink.values) != 1 || calls != 0 {
		t.Fatal("final root namespace not admitted")
	}
}
