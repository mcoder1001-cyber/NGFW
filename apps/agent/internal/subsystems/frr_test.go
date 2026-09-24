package subsystems

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/bgp"
)

// fakeFRR is a recording runner playing vtysh and frr-reload.py: diff is what `frr-reload.py --test` prints,
// running what `show running-config` prints.
type fakeFRR struct {
	*renderers.RecordingRunner
	mu      sync.Mutex
	diff    string
	running string
}

func newFakeFRR() *fakeFRR {
	f := &fakeFRR{RecordingRunner: renderers.NewRecordingRunner()}
	f.On(frr.VtyshBin, func(c renderers.Command) (renderers.Output, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if slices.Contains(c.Args, "-c") && slices.Contains(c.Args, string(frr.ShowRunningConfig)) {
			return renderers.Output{Stdout: []byte(f.running)}, nil
		}
		return renderers.Output{}, nil
	})
	f.On(frr.ReloadBin, func(c renderers.Command) (renderers.Output, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if slices.Contains(c.Args, "--test") {
			return renderers.Output{Stdout: []byte(f.diff)}, nil
		}
		return renderers.Output{}, nil
	})
	return f
}

func (f *fakeFRR) set(diff, running string) {
	f.mu.Lock()
	f.diff, f.running = diff, running
	f.mu.Unlock()
}

func (f *fakeFRR) count(sub string) int {
	n := 0
	for _, c := range f.Calls() {
		if strings.Contains(strings.Join(append([]string{c.Path}, c.Args...), " "), sub) {
			n++
		}
	}
	return n
}

func tempPaths(t *testing.T) frr.Paths {
	t.Helper()
	base := t.TempDir()
	p := frr.Paths{ConfDir: filepath.Join(base, "etc"), RunDir: filepath.Join(base, "run"), BinDir: "/usr/bin",
		ReloadLog: filepath.Join(base, "reload.log"), FileMode: 0o640}
	for _, d := range []string{p.ConfSubdir(), p.SocketDir()} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func frrDoc(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestFRRConfigLifecycle(t *testing.T) {
	ctx := context.Background()
	f := newFakeFRR()
	paths := tempPaths(t)
	rt := newFRRAt(Env{Owner: "w8"}, f, paths, true)
	defer rt.Close()
	d := &frrConfigDescriptor{rt: rt}
	doc := frrDoc(t, `{"interfaces":{"host-w8l0":{"ipv4":["10.8.1.1/24"],"lcp":{"hostIfName":"w8-l0"}}},
	  "routing":{"bgp":{"asn":65080,"neighbors":{"10.8.1.2":{"remoteAs":65081,"updateSource":"host-w8l0"}}}}}`)
	v := desired.FRRValue(doc, desired.FRRApplied)
	if d.KeyOf(v) != "frr.config/vrx" {
		t.Fatal(d.KeyOf(v))
	}
	if deps := d.Dependencies(v); len(deps) != 1 || deps[0].Key != "lcp.itf-pair/host-w8l0" || deps[0].Optional {
		t.Fatalf("deps %+v", deps)
	}

	// FRR not running: nothing to retrieve, a create fails without spawning anything
	if kvs, err := d.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("retrieve without FRR: %v %v", kvs, err)
	}
	if _, err := d.Create(ctx, v); !errors.Is(err, ErrFRRUnavailable) || len(f.Calls()) != 0 {
		t.Fatalf("create without FRR: %v, %d calls", err, len(f.Calls()))
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatalf("delete without FRR: %v", err)
	}

	// running: create = vtysh -C, frr-reload --reload, --test (convergence)
	if err := os.WriteFile(filepath.Join(paths.SocketDir(), "zebra.vty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if f.count(" -C -f ") != 1 || f.count("--reload") != 1 || f.count("--test") != 1 {
		t.Fatalf("create calls: %v", f.Calls())
	}
	conf, err := os.ReadFile(paths.ConfFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"interface w8-l0\n ip address 10.8.1.1/24\nexit", "router bgp 65080", " neighbor 10.8.1.2 update-source w8-l0"} {
		if !strings.Contains(string(conf), want) {
			t.Fatalf("frr.conf lacks %q:\n%s", want, conf)
		}
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, v) {
		t.Fatalf("retrieve after create: %v %v", kvs, err)
	}

	// drift → a different value (the reconciler re-applies it)
	f.set("Lines To Delete\n===============\n\nLines To Add\n============\n\nrouter bgp 65080\n neighbor 10.8.1.2 remote-as 65081\n", "")
	kvs, _ = d.Retrieve(ctx)
	if _, status, _ := desired.ParseFRRValue(kvs[0].Value); status != desired.FRRDrift || proto.Equal(kvs[0].Value, v) {
		t.Fatalf("drift not reported: %v", kvs)
	}
	f.set("", "")
	if _, err := d.Update(ctx, v, v, nil); err != nil {
		t.Fatal(err)
	}

	// delete = the framework-only configuration; afterwards an agent restart sees FRR content as "unknown"
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	conf, _ = os.ReadFile(paths.ConfFile())
	if strings.Contains(string(conf), "router bgp") || strings.Contains(string(conf), "interface") {
		t.Fatalf("frr.conf after delete:\n%s", conf)
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("retrieve after delete with a clean FRR: %v", kvs)
	}
	f.set("", "Building configuration...\nfrr version 10.7.1\nfrr defaults traditional\nhostname x\nlog syslog informational\nservice integrated-vtysh-config\n!\nrouter bgp 1\nexit\nend\n")
	kvs, _ = d.Retrieve(ctx)
	if len(kvs) != 1 {
		t.Fatalf("leftover FRR content not reported: %v", kvs)
	}
	if _, status, _ := desired.ParseFRRValue(kvs[0].Value); status != desired.FRRUnknown {
		t.Fatalf("status %q", status)
	}
}

func TestEventOf(t *testing.T) {
	ev := EventOf(frr.Event{Poller: bgp.PollerNeighbors, Key: "default|10.8.1.2", Old: "Established", New: "Idle"})
	if ev.GetKind() != vrxv1.EventKind_EVENT_KIND_BGP_NEIGHBOR_CHANGED || ev.GetAttributes()["peer"] != "10.8.1.2" ||
		ev.GetAttributes()["vrf"] != "default" || ev.GetAttributes()["old"] != "Established" || ev.GetAttributes()["new"] != "Idle" {
		t.Fatalf("%v", ev)
	}
	ev = EventOf(frr.Event{Poller: frr.PollerRoutes, Key: "ipv4/default/bgp", Old: "", New: "200"})
	if ev.GetKind() != vrxv1.EventKind_EVENT_KIND_ROUTING_CHANGED || ev.GetAttributes()["protocol"] != "bgp" ||
		ev.GetAttributes()["old"] != "0" || ev.GetAttributes()["new"] != "200" || ev.GetAttributes()["family"] != "ipv4" {
		t.Fatalf("%v", ev)
	}
	if EventOf(frr.Event{Poller: frr.PollerInterfaces, Key: "w8-l0", New: "down"}) != nil {
		t.Fatal("FRR interface events are VPP's to report")
	}
}

func TestEventsArePublished(t *testing.T) {
	old := pollInterval
	pollInterval = 20 * time.Millisecond
	defer func() { pollInterval = old }()
	f := newFakeFRR()
	summary := `{"default":{"ipv4Unicast":{"as":65080,"peers":{"10.8.1.2":{"remoteAs":65081,"state":"%s"}}}}}`
	state := "Active"
	var mu sync.Mutex
	f.On(frr.VtyshBin, func(c renderers.Command) (renderers.Output, error) {
		mu.Lock()
		defer mu.Unlock()
		if slices.Contains(c.Args, string(bgp.ShowSummary)) {
			return renderers.Output{Stdout: []byte(strings.Replace(summary, "%s", state, 1))}, nil
		}
		return renderers.Output{Stdout: []byte("{}")}, nil
	})
	paths := tempPaths(t)
	if err := os.WriteFile(filepath.Join(paths.SocketDir(), "zebra.vty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got := make(chan *vrxv1.Event, 16)
	rt := newFRRAt(Env{Owner: "w8", Publish: func(ev *vrxv1.Event) { got <- ev }}, f, paths, true)
	defer rt.Close()
	if err := rt.apply(context.Background(), frrDoc(t, `{"routing":{"bgp":{"asn":65080}}}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // baseline
	mu.Lock()
	state = "Established"
	mu.Unlock()
	select {
	case ev := <-got:
		if ev.GetKind() != vrxv1.EventKind_EVENT_KIND_BGP_NEIGHBOR_CHANGED || ev.GetAttributes()["new"] != "Established" {
			t.Fatalf("%v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no neighbour event")
	}
}

func TestHasOwnContent(t *testing.T) {
	if hasOwnContent("Building configuration...\n\nCurrent configuration:\n!\nfrr version 10.7.1\nfrr defaults traditional\nhostname h\nlog syslog informational\nservice integrated-vtysh-config\n!\nend\n") {
		t.Fatal("framework globals are not content")
	}
	if !hasOwnContent("frr version 10.7.1\nip prefix-list a seq 5 permit 10.0.0.0/8\n") {
		t.Fatal("a prefix list is content")
	}
}

func TestTapGatedPairs(t *testing.T) {
	v := coretest.New()
	d := &tapGatedPairs{client: v, owner: "w8"}
	kvs, err := d.Retrieve(context.Background())
	if err != nil || len(kvs) != 0 {
		t.Fatal(kvs, err)
	}
	for _, c := range v.Calls() {
		if c.GetMessageName() == "lcp_itf_pair_get" {
			t.Fatal("lcp_itf_pair_get sent while VPP has no tap interface")
		}
	}
}

func TestFRRPathsByOwner(t *testing.T) {
	t.Setenv(EnvFRR, "")
	t.Setenv(EnvFRRPathspace, "")
	if p, ok := frrPaths("vrx"); !ok || p.ConfDir != "/etc/frr" {
		t.Fatal("product owner → product paths", p)
	}
	if _, ok := frrPaths("w8"); ok {
		t.Fatal("a slot owner drives no FRR without VRX_FRR_PATHSPACE")
	}
	t.Setenv(EnvFRRPathspace, "w8")
	if p, ok := frrPaths("w8"); !ok || p.Namespace != "w8" || !strings.HasPrefix(p.ConfDir, "/run/vrx-test/w8/") {
		t.Fatal("slot pathspace", p)
	}
	if _, ok := frrPaths("vrx"); !ok {
		t.Fatal("the pathspace wins for any owner")
	}
	t.Setenv(EnvFRR, "off")
	if _, ok := frrPaths("vrx"); ok {
		t.Fatal("VRX_FRR=off")
	}
}
