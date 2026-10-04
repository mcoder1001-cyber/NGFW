package ldp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ngfw/agent/internal/descriptors/mpls"
	"ngfw/agent/internal/renderers/frr"
)

func TestReadFRRShape(t *testing.T) {
	fixtures := map[frr.ShowCommand]string{
		ShowBindings:                          `{"bindings":[{"addressFamily":"ipv4","prefix":"198.51.100.0/24","neighborId":"192.0.2.9","localLabel":"16000","remoteLabel":"imp-null","inUse":1}]}`,
		ShowNeighbors:                         `{"neighbors":[{"addressFamily":"ipv4","neighborId":"192.0.2.9","transportAddress":"192.0.2.9","state":"OPERATIONAL"}]}`,
		ShowDiscovery:                         `{"interfaces":{"tap":{"adjacencies":[{"lsrId":"192.0.2.9","sourceAddress":"192.0.2.1"}]},"wrong":{"adjacencies":[{"lsrId":"192.0.2.9","sourceAddress":"192.0.2.2"}]}}}`,
		frr.ShowCommand("show ip route json"): `{"198.51.100.0/24":[{"prefix":"198.51.100.0/24","selected":true,"nexthops":[{"ip":"192.0.2.1","interfaceName":"tap","active":true}]}]}`,
	}
	show := func(_ context.Context, c frr.ShowCommand) (json.RawMessage, error) {
		return json.RawMessage(fixtures[c]), nil
	}
	got, err := Read(context.Background(), show)
	if err != nil || len(got.Bindings) != 1 || got.Bindings[0].NextHop != "192.0.2.1" {
		t.Fatalf("%+v %v", got, err)
	}
	fixtures[ShowBindings] = `null`
	if _, err := Read(context.Background(), show); err == nil {
		t.Fatal("null accepted")
	}
}
func TestHoldDownWithdraw(t *testing.T) {
	c := &Cache{}
	now := time.Unix(100, 0)
	routes := []mpls.Route{{Label: 16000}}
	if !c.Update(now, Observation{}, routes, nil) {
		t.Fatal("first sync missing")
	}
	if c.Update(now, Observation{}, nil, errors.New("down")) {
		t.Fatal("premature withdrawal")
	}
	if c.Update(now.Add(HoldDown-time.Second), Observation{}, nil, errors.New("down")) {
		t.Fatal("hold-down shortened")
	}
	if !c.Update(now.Add(HoldDown), Observation{}, nil, errors.New("down")) || len(c.Routes()) != 0 {
		t.Fatal("not flushed")
	}
	c.Update(now, Observation{}, routes, nil)
	if !c.Update(now, Observation{}, nil, nil) {
		t.Fatal("successful withdrawal not immediate")
	}
}
func TestFailedApplyRetriesUnchangedObservation(t *testing.T) {
	c := &Cache{}
	now := time.Unix(100, 0)
	routes := []mpls.Route{{Label: 16000}}
	c.Update(now, Observation{}, routes, nil)
	c.Applied(now, errors.New("scheduler unavailable"))
	if !c.Update(now, Observation{}, routes, nil) {
		t.Fatal("unchanged routes did not retry failed apply")
	}
	c.Applied(now, nil)
	if c.Update(now, Observation{}, routes, nil) {
		t.Fatal("clean state reapplied")
	}
}
func TestReadRejectsWrongSchemasAndBounds(t *testing.T) {
	for _, payload := range []string{`{"error":"unknown command"}`, `{"bindings":null}`, `{"bindings":{}}`} {
		show := func(_ context.Context, c frr.ShowCommand) (json.RawMessage, error) {
			if c == ShowBindings {
				return json.RawMessage(payload), nil
			}
			return json.RawMessage(`{}`), nil
		}
		if _, err := Read(context.Background(), show); err == nil {
			t.Fatalf("accepted %s", payload)
		}
	}
	show := func(_ context.Context, _ frr.ShowCommand) (json.RawMessage, error) {
		return make([]byte, MaxOutput+1), nil
	}
	if _, err := Read(context.Background(), show); err == nil {
		t.Fatal("oversized output accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	show = func(ctx context.Context, _ frr.ShowCommand) (json.RawMessage, error) { return nil, ctx.Err() }
	if _, err := Read(ctx, show); err == nil {
		t.Fatal("cancel ignored")
	}
}
func TestReadRowCountLimit(t *testing.T) {
	rows := make([]map[string]string, MaxRows+1)
	for i := range rows {
		rows[i] = map[string]string{"addressFamily": "ipv4"}
	}
	raw, err := json.Marshal(map[string]any{"neighbors": rows})
	if err != nil {
		t.Fatal(err)
	}
	show := func(_ context.Context, c frr.ShowCommand) (json.RawMessage, error) {
		if c == ShowNeighbors {
			return raw, nil
		}
		return json.RawMessage(`{}`), nil
	}
	if _, err := Read(context.Background(), show); err == nil {
		t.Fatal("neighbor flood accepted")
	}
}
func TestRouteLimitPreservesLastGoodCache(t *testing.T) {
	now := time.Unix(100, 0)
	cache := &Cache{}
	good := []mpls.Route{{Label: 16000}}
	cache.Update(now, Observation{}, good, nil)
	cache.Applied(now, nil)
	bindings := make([]Binding, MaxRoutes+1)
	for i := range bindings {
		bindings[i] = Binding{FEC: "198.51.100.0/24", LocalLabel: uint32(16000 + i), RemoteLabel: 17000, NextHop: "192.0.2.1", LinuxInterface: "tap"}
	}
	routes, err := Translate(bindings, 7000, map[string]string{"tap": "wan"})
	if err == nil {
		t.Fatal("route flood accepted")
	}
	if cache.Update(now, Observation{}, routes, err) || len(cache.Routes()) != 1 {
		t.Fatal("oversized snapshot withdrew last good state")
	}
}
