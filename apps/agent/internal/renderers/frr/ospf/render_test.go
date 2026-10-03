package ospf_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/ospf"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fullDoc = `{
 "interfaces": {"host-w8l0": {"lcp": {"hostIfName": "w8-l0"}}, "loop0": {"lcp": {"hostIfName": "w8-lo"}}, "host-w8l1": {"lcp": {"hostIfName": "w8-l1"}}},
 "routing": {"ospf": {
  "routerId": "10.8.9.1", "vrf": "default", "defaultInformationOriginate": "always",
  "areas": {"0.0.0.0": {}, "51": {"type": "stub", "noSummary": true}, "0.0.0.7": {"type": "nssa"}},
  "interfaces": {
   "host-w8l0": {"area": "0", "cost": 10, "networkType": "point-to-point", "helloIntervalSec": 2, "deadIntervalSec": 8, "priority": 0, "bfd": true},
   "loop0": {"area": "0.0.0.0", "passive": true},
   "host-w8l1": {"area": "0.0.0.51"}
  },
  "redistribute": {"bgp": {"routeMap": "rm-in"}, "connected": {"metric": 20}, "static": {}}
 }}
}`

func mapIf(n string) (string, bool) {
	switch n {
	case "host-w8l0":
		return "w8-l0", true
	case "host-w8l1":
		return "w8-l1", true
	case "loop0":
		return "w8-lo", true
	case "evil":
		return "a b", true
	case "twin":
		return "w8-l0", true
	}
	return "", false
}

func parse(t *testing.T, js string) *ngfwv1.DesiredState {
	t.Helper()
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // golden file
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // golden file
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if got != string(want) {
		t.Fatalf("%s differs:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

func TestRenderThroughFramework(t *testing.T) {
	ds := parse(t, fullDoc)
	r := frr.New(renderers.NewRecordingRunner(), frr.WithPaths(frr.TestPaths("w8")), frr.WithSections(ospf.Section{}),
		frr.WithInterfaceMapper(mapIf), frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: ospf.Name, Fn: ospf.InterfaceLines}))
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "full.golden", string(files[r.Paths().ConfFile()].Content))
}

func TestVRFAndEmpty(t *testing.T) {
	lines, err := ospf.Render(parse(t, `{"routing":{"ospf":{"vrf":"red"}}}`).GetRouting().GetOspf())
	if err != nil || strings.Join(lines, "|") != "router ospf vrf red|exit" {
		t.Fatalf("%q %v", lines, err)
	}
	if lines, err := ospf.Render(nil); lines != nil || err != nil {
		t.Fatal(lines, err)
	}
	if m, err := ospf.RenderInterfaces(nil, mapIf); m != nil || err != nil {
		t.Fatal(m, err)
	}
}

func TestRenderErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		doc  string
		want string
	}{
		"undefined area":   {`{"routing":{"ospf":{"interfaces":{"loop0":{"area":"1"}}}}}`, "not defined"},
		"area 0 implied":   {`{"routing":{"ospf":{"areas":{"1":{}},"interfaces":{"loop0":{"area":"0"}}}}}`, "not defined"},
		"backbone stub":    {`{"routing":{"ospf":{"areas":{"0.0.0.0":{"type":"stub"}}}}}`, "backbone"},
		"backbone nssa":    {`{"routing":{"ospf":{"areas":{"0":{"type":"nssa"}}}}}`, "backbone"},
		"same area twice":  {`{"routing":{"ospf":{"areas":{"0":{},"0.0.0.0":{}}}}}`, "same area"},
		"bad area id":      {`{"routing":{"ospf":{"areas":{"x; router bgp 1":{}}}}}`, "area id"},
		"leading zero":     {`{"routing":{"ospf":{"areas":{"01":{}}}}}`, "area id"},
		"bad area type":    {`{"routing":{"ospf":{"areas":{"1":{"type":"totally"}}}}}`, "area type"},
		"no-summary plain": {`{"routing":{"ospf":{"areas":{"1":{"noSummary":true}}}}}`, "noSummary"},
		"unmapped":         {`{"routing":{"ospf":{"areas":{"0":{}},"interfaces":{"loop9":{"area":"0"}}}}}`, "no Linux interface"},
		"hostile ifname":   {`{"routing":{"ospf":{"areas":{"0":{}},"interfaces":{"evil":{"area":"0"}}}}}`, "interface name"},
		"same linux if":    {`{"routing":{"ospf":{"areas":{"0":{}},"interfaces":{"host-w8l0":{"area":"0"},"twin":{"area":"0"}}}}}`, "same Linux interface"},
		"bad network":      {`{"routing":{"ospf":{"areas":{"0":{}},"interfaces":{"loop0":{"area":"0","networkType":"nbma"}}}}}`, "network type"},
		"dead <= hello":    {`{"routing":{"ospf":{"areas":{"0":{}},"interfaces":{"loop0":{"area":"0","helloIntervalSec":10,"deadIntervalSec":10}}}}}`, "must exceed"},
		"priority":         {`{"routing":{"ospf":{"areas":{"0":{}},"interfaces":{"loop0":{"area":"0","priority":256}}}}}`, "priority"},
		"cost 0":           {`{"routing":{"ospf":{"areas":{"0":{}},"interfaces":{"loop0":{"area":"0","cost":0}}}}}`, "cost"},
		"into itself":      {`{"routing":{"ospf":{"redistribute":{"ospf":{}}}}}`, "into itself"},
		"bad route map":    {`{"routing":{"ospf":{"redistribute":{"static":{"routeMap":"a b"}}}}}`, "route map"},
		"bad router id":    {`{"routing":{"ospf":{"routerId":"2001:db8::1"}}}`, "dotted quad"},
		"bad vrf":          {`{"routing":{"ospf":{"vrf":"a b"}}}`, "vrf name"},
		"bad originate":    {`{"routing":{"ospf":{"defaultInformationOriginate":"maybe"}}}`, "off, on or always"},
	} {
		t.Run(name, func(t *testing.T) {
			o := parse(t, tc.doc).GetRouting().GetOspf()
			_, err := ospf.Render(o)
			if err == nil {
				_, err = ospf.RenderInterfaces(o, mapIf)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if !errors.Is(err, frr.ErrInput) && !errors.Is(err, renderers.ErrUnsafe) {
				t.Fatalf("err %v is not an input error", err)
			}
		})
	}
}

func TestParseNeighbors(t *testing.T) {
	raw := `{"default":{"vrfName":"default","vrfId":0,"neighbors":{
	   "10.8.9.2":[{"nbrState":"Full/DR","ifaceAddress":"10.8.0.2","ifaceName":"w8-l0:10.8.0.1","nbrPriority":1}],
	   "10.8.9.3":[{"state":"2-Way/DROther","address":"10.8.1.3","ifaceName":"w8-l1:10.8.1.1","priority":0}]}},
	 "red":{"vrfName":"red","neighbors":{}}}`
	ns, err := ospf.ParseNeighbors([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 2 || ns[0].RouterID != "10.8.9.2" || ns[0].State != "Full/DR" || ns[0].Interface != "w8-l0" || ns[0].Address != "10.8.0.2" ||
		ns[0].Priority != 1 || ns[1].State != "2-Way/DROther" || ns[1].Address != "10.8.1.3" {
		t.Fatalf("%+v", ns)
	}
	single, err := ospf.ParseNeighbors([]byte(`{"neighbors":{"1.1.1.1":[{"nbrState":"Init/DROther","ifaceName":"e0:10.0.0.1"}]}}`))
	if err != nil || len(single) != 1 || single[0].VRF != "default" {
		t.Fatalf("%+v %v", single, err)
	}
	if ns, err := ospf.ParseNeighbors([]byte("{}")); ns != nil || err != nil {
		t.Fatal(ns, err)
	}
	if _, err := ospf.ParseNeighbors([]byte("[")); err == nil {
		t.Fatal("broken JSON accepted")
	}
	snap, err := ospf.PollNeighbors(context.Background(), func(context.Context, frr.ShowCommand) (json.RawMessage, error) { return json.RawMessage(raw), nil })
	if err != nil || snap["default|10.8.9.2|w8-l0"] != "Full/DR" || len(snap) != 2 {
		t.Fatalf("poll %v %v", snap, err)
	}
}
