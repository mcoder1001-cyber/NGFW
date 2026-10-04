package pim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mfib"
	"strings"
	"testing"
	"time"
)

const fixture = `{"239.1.2.3":{"10.0.0.2":{"source":"10.0.0.2","group":"239.1.2.3","installed":1,"iif":"w15in","oil":{"w15out":{"inboundInterface":"w15in","outboundInterface":"w15out","ttl":1}}}}}`

func ptr(s string) *string { return &s }
func doc() *ngfwv1.DesiredState {
	return &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{"in": {Lcp: &ngfwv1.InterfaceLcp{HostIfName: ptr("w15in")}}, "out": {Lcp: &ngfwv1.InterfaceLcp{HostIfName: ptr("w15out")}}}, Routing: &ngfwv1.RoutingConfig{Multicast: &ngfwv1.MulticastConfig{Pim: &ngfwv1.PimConfig{Interfaces: []string{"in", "out"}}}}}
}
func TestTranslateAndWithdraw(t *testing.T) {
	rows, e := Parse([]byte(fixture))
	if e != nil {
		t.Fatal(e)
	}
	kv := Translate(rows, doc())
	if len(kv) != 1 || kv[0].Key.Descriptor() != Descriptor {
		t.Fatalf("wrong dynamic key: %v", kv)
	}
	r, e := df7.Decode[mfib.Route](kv[0].Value)
	if e != nil {
		t.Fatal(e)
	}
	if r.Source != "10.0.0.2" || len(r.Paths) != 2 || r.Paths[0].Flags != "accept" || r.Paths[1].Flags != "forward" {
		t.Fatalf("wrong route %+v", r)
	}
	d := doc()
	delete(d.Interfaces, "out")
	if len(Translate(rows, d)) != 0 {
		t.Fatal("removed dependency retained")
	}
	d = doc()
	d.Routing.Multicast.Pim = nil
	if len(Translate(rows, d)) != 0 {
		t.Fatal("disabled PIM retained")
	}
	star, e := Parse([]byte(strings.ReplaceAll(fixture, "10.0.0.2", "0.0.0.0")))
	if e != nil {
		t.Fatal(e)
	}
	r, _ = df7.Decode[mfib.Route](Translate(star, doc())[0].Value)
	if r.Source != "" {
		t.Fatal("wildcard not normalized")
	}
}
func TestFailedReadPreservesSnapshot(t *testing.T) {
	raw := []byte(fixture)
	var readErr error
	s := &Source{Read: func(context.Context) ([]byte, error) { return raw, readErr }}
	if e := s.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"null", "[]", `{"error":"unknown command"}`, strings.ReplaceAll(fixture, `"ttl":1`, `"ttl":0`), strings.ReplaceAll(fixture, `"installed":1,`, ``)} {
		raw = []byte(bad)
		if e := s.Refresh(context.Background()); e == nil {
			t.Fatalf("accepted %s", bad)
		}
		if len(s.Desired(doc())) != 1 {
			t.Fatal("bad read withdrew route")
		}
	}
	readErr = errors.New("daemon down")
	if e := s.Refresh(context.Background()); e == nil || len(s.Desired(doc())) != 1 {
		t.Fatal("transport failure withdrew")
	}
	readErr = nil
	raw = []byte(`{}`)
	if e := s.Refresh(context.Background()); e != nil || len(s.Desired(doc())) != 0 {
		t.Fatal("empty successful read did not withdraw")
	}
}
func TestRunRetriesSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Source{Interval: time.Nanosecond, Read: func(context.Context) ([]byte, error) { return []byte(fixture), nil }}
	calls := 0
	s.Run(ctx, func(context.Context) error {
		calls++
		if len(s.Desired(doc())) != 1 {
			t.Fatal("cache unavailable from sync (lock order)")
		}
		if calls == 1 {
			return errors.New("transient VPP disconnect")
		}
		cancel()
		return nil
	})
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestMappingChangeSuppressesStaleObservation(t *testing.T) {
	rows, e := Parse([]byte(fixture))
	if e != nil {
		t.Fatal(e)
	}
	d := doc()
	d.Interfaces["out"].Lcp.HostIfName = ptr("w15new")
	if len(Translate(rows, d)) != 0 {
		t.Fatal("remapped old OIL retained")
	}
	d = doc()
	d.Interfaces["other"] = &ngfwv1.Interface{Lcp: &ngfwv1.InterfaceLcp{HostIfName: ptr("w15out")}}
	d.Routing.Multicast.Pim.Interfaces = append(d.Routing.Multicast.Pim.Interfaces, "other")
	if len(Translate(rows, d)) != 0 {
		t.Fatal("ambiguous mapping accepted")
	}
}

func TestSupportedSnapshotCapPreservesCache(t *testing.T) {
	makeSnapshot := func(n int) []byte {
		t.Helper()
		groups := map[string]map[string]Observation{}
		for i := 0; i < n; i++ {
			source := fmt.Sprintf("10.1.%d.%d", i/254, i%254+1)
			installed := 1
			ttl := 1
			groups["239.1.2.3"] = ensureSources(groups["239.1.2.3"])
			groups["239.1.2.3"][source] = Observation{Source: source, Group: "239.1.2.3", IIF: "w15in", Installed: &installed, Oil: map[string]Output{"w15out": {Inbound: "w15in", Outbound: "w15out", TTL: &ttl}}}
		}
		raw, e := json.Marshal(groups)
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	raw := makeSnapshot(MaxDynamicRoutes)
	s := &Source{Read: func(context.Context) ([]byte, error) { return raw, nil }}
	if e := s.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	previous := s.Desired(doc())
	if len(previous) != MaxDynamicRoutes {
		t.Fatalf("boundary snapshot routes=%d", len(previous))
	}
	raw = makeSnapshot(MaxDynamicRoutes + 1)
	// The parser's independent input guard still permits this shape; runtime
	// support rejects it before publication, not by pretending it is withdrawal.
	parsed, e := Parse(raw)
	if e != nil || len(parsed) != MaxDynamicRoutes+1 {
		t.Fatalf("parser boundary: %d %v", len(parsed), e)
	}
	if e := s.Refresh(context.Background()); e == nil {
		t.Fatal("accepted unsupported 257-route snapshot")
	}
	current := s.Desired(doc())
	if len(current) != len(previous) {
		t.Fatal("overflow replaced last snapshot")
	}
	for i := range previous {
		if current[i].Key != previous[i].Key {
			t.Fatal("overflow changed cached route keys")
		}
	}
}
func ensureSources(m map[string]Observation) map[string]Observation {
	if m == nil {
		return map[string]Observation{}
	}
	return m
}

func TestRunReportsRecoveryAndRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads, syncCalls := 0, 0
	var outcomes []bool
	source := &Source{Interval: time.Nanosecond, Read: func(context.Context) ([]byte, error) {
		reads++
		if reads <= 2 {
			return nil, errors.New("daemon unavailable")
		}
		return []byte(`{}`), nil
	}}
	source.OnError = func(err error) {
		outcomes = append(outcomes, err != nil)
		if len(outcomes) == 4 {
			cancel()
		}
	}
	source.Run(ctx, func(context.Context) error {
		syncCalls++
		if syncCalls == 2 {
			return errors.New("scheduler retry")
		}
		return nil
	})
	want := []bool{true, true, false, true}
	if len(outcomes) != len(want) {
		t.Fatalf("outcomes=%v", outcomes)
	}
	for i := range want {
		if outcomes[i] != want[i] {
			t.Fatalf("outcomes=%v", outcomes)
		}
	}
	if reads != 4 || syncCalls != 2 {
		t.Fatalf("read/sync retries %d/%d", reads, syncCalls)
	}
}
