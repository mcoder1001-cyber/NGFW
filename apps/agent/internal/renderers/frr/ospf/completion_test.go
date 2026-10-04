package ospf_test

import (
	"context"
	"encoding/json"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/ospf"
	"strings"
	"testing"
)

func TestOSPF6AndMD5Rendering(t *testing.T) {
	ds := parse(t, `{"interfaces":{"loop0":{"lcp":{"hostIfName":"w8-lo"}}},"routing":{"ospf":{"routerId":"10.8.0.1","areas":{"0":{}},"interfaces":{"loop0":{"area":"0","auth":{"type":"md5","keyId":7,"keyRef":"password/ospf"}}}},"ospf6":{"routerId":"10.8.0.1","areas":{"0":{},"1":{"type":"stub","noSummary":true}},"interfaces":{"loop0":{"area":"1","passive":true,"networkType":"point-to-point"}},"redistribute":{"connected":{},"rip":{}}}}}`)
	r := frr.New(renderers.NewRecordingRunner(), frr.WithPaths(frr.TestPaths("w8")), frr.WithSections(ospf.Section{}, ospf.Section6{}), frr.WithInterfaceMapper(mapIf), frr.WithSecretResolver(frr.SecretResolverFunc(func(context.Context, string) (string, error) { return "fixture-ospf-key", nil })), frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: "ospf", Fn: ospf.InterfaceLines}, frr.NamedInterfaceLines{Name: "ospf6", Fn: ospf.InterfaceLines6}))
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	text := string(files[r.Paths().ConfFile()].Content)
	for _, want := range []string{"router ospf6", "ospf6 router-id 10.8.0.1", "ipv6 ospf6 area 1", "ipv6 ospf6 passive", "redistribute ripng", "area 1 stub no-summary", "ip ospf message-digest-key 7 md5 fixture-ospf-key"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(string(files.Redacted()[r.Paths().ConfFile()].Content), "fixture-ospf-key") {
		t.Fatal("secret exposed")
	}
	empty, err := r.Render(context.Background(), parse(t, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(empty[r.Paths().ConfFile()].Content), "ospf") {
		t.Fatal("rollback left protocol")
	}
}
func TestOSPF6PollAndEvent(t *testing.T) {
	show := func(context.Context, frr.ShowCommand) (json.RawMessage, error) {
		return json.RawMessage(`{"neighbors":[{"neighborId":"10.8.0.2","interfaceName":"w8-lo","state":"Full","priority":1}]}`), nil
	}
	snapshot, err := ospf.PollNeighbors6(context.Background(), show)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["default|10.8.0.2|w8-lo"] != "Full" {
		t.Fatal(snapshot)
	}
	for _, p := range []string{"ospf-neighbors", "ospf6-neighbors"} {
		if (frr.Event{Poller: p, Key: "default|10.8.0.2|w8-lo", Old: "Init", New: "Full"}).ToProto().Kind != ngfwv1.EventKind_EVENT_KIND_OSPF_NEIGHBOR_CHANGED {
			t.Fatal("wrong event kind")
		}
	}
}
func TestOSPF6OwnErrorPointer(t *testing.T) {
	ds := parse(t, `{"routing":{"ospf6":{"areas":{"0":{"type":"stub"}}}}}`)
	_, err := (ospf.Section6{}).Render(&frr.RenderContext{Desired: ds})
	if err == nil || !strings.Contains(err.Error(), "routing.ospf6") {
		t.Fatal(err)
	}
	ds = parse(t, `{"routing":{"ospf6":{"interfaces":{"loop0":{"area":"0","networkType":"non-broadcast"}}}}}`)
	_, err = ospf.InterfaceLines6(&frr.RenderContext{Desired: ds})
	if err == nil {
		t.Fatal("NBMA accepted")
	}
}

func TestOSPF6PollRejectsMalformedSnapshots(t *testing.T) {
	for _, raw := range []string{`null`, `{"default":{"foo":1}}`, `{"neighbors":null}`, `{"neighbors":[{}]}`, `{"neighbors":[{"neighborId":"192.0.2.2","interfaceName":"tap0"}]}`} {
		_, err := ospf.PollNeighbors6(context.Background(), func(context.Context, frr.ShowCommand) (json.RawMessage, error) { return json.RawMessage(raw), nil })
		if err == nil {
			t.Fatalf("malformed snapshot accepted: %s", raw)
		}
	}
	for _, raw := range []string{`{}`, `{"neighbors":[]}`} {
		rows, err := ospf.PollNeighbors6(context.Background(), func(context.Context, frr.ShowCommand) (json.RawMessage, error) { return json.RawMessage(raw), nil })
		if err != nil || len(rows) != 0 {
			t.Fatalf("legitimate empty: %v %v", rows, err)
		}
	}
}
