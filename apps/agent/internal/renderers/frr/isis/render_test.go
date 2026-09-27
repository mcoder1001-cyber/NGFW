package isis_test

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

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/isis"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fullDoc = `{
 "interfaces": {"host-w8l0": {"lcp": {"hostIfName": "w8-l0"}}, "loop0": {"lcp": {"hostIfName": "w8-lo"}}, "host-w8l1": {"lcp": {"hostIfName": "w8-l1"}}},
 "routing": {"isis": {
  "net": "49.0001.1921.6800.1001.00", "level": "level-1-2", "vrf": "default",
  "interfaces": {
   "host-w8l0": {"metric": 10, "circuitType": "level-2", "networkType": "point-to-point", "bfd": true},
   "loop0": {"passive": true},
   "host-w8l1": {"circuitType": "level-1", "networkType": "broadcast"}
  },
  "redistribute": {"bgp": {"routeMap": "rm-in"}, "connected": {"metric": 20}, "ospf": {}}
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

func parse(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
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
	r := frr.New(renderers.NewRecordingRunner(), frr.WithPaths(frr.TestPaths("w8")), frr.WithSections(isis.Section{}),
		frr.WithInterfaceMapper(mapIf), frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: isis.Name, Fn: isis.InterfaceLines}))
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "full.golden", string(files[r.Paths().ConfFile()].Content))
}

func TestVRFAndEmpty(t *testing.T) {
	lines, err := isis.Render(parse(t, `{"routing":{"isis":{"vrf":"red","level":"level-2","net":"49.0001.0000.0000.0001.00"}}}`).GetRouting().GetIsis())
	if err != nil || strings.Join(lines, "|") != "router isis vrx vrf red| is-type level-2-only| net 49.0001.0000.0000.0001.00| metric-style wide|exit" {
		t.Fatalf("%q %v", lines, err)
	}
	if lines, err := isis.Render(nil); lines != nil || err != nil {
		t.Fatal(lines, err)
	}
	if m, err := isis.RenderInterfaces(nil, mapIf); m != nil || err != nil {
		t.Fatal(m, err)
	}
}

func TestRenderErrors(t *testing.T) {
	const n = `"net":"49.0001.0000.0000.0001.00"`
	for name, tc := range map[string]struct {
		doc  string
		want string
	}{
		"no net":         {`{"routing":{"isis":{}}}`, "net is required"},
		"bad net":        {`{"routing":{"isis":{"net":"49.0001.x; router bgp 1"}}}`, "ISO NET"},
		"nsel not 00":    {`{"routing":{"isis":{"net":"49.0001.0000.0000.0001.01"}}}`, "ISO NET"},
		"bad level":      {`{"routing":{"isis":{` + n + `,"level":"level-3"}}}`, "level"},
		"l1 IS l2 circ":  {`{"routing":{"isis":{` + n + `,"level":"level-1","interfaces":{"loop0":{"circuitType":"level-2"}}}}}`, "cannot run"},
		"l2 IS l12 circ": {`{"routing":{"isis":{` + n + `,"level":"level-2","interfaces":{"loop0":{"circuitType":"level-1-2"}}}}}`, "cannot run"},
		"bad circuit":    {`{"routing":{"isis":{` + n + `,"interfaces":{"loop0":{"circuitType":"l2"}}}}}`, "circuit type"},
		"bad network":    {`{"routing":{"isis":{` + n + `,"interfaces":{"loop0":{"networkType":"nbma"}}}}}`, "network type"},
		"metric 0":       {`{"routing":{"isis":{` + n + `,"interfaces":{"loop0":{"metric":0}}}}}`, "metric"},
		"unmapped":       {`{"routing":{"isis":{` + n + `,"interfaces":{"loop9":{}}}}}`, "no Linux interface"},
		"hostile ifname": {`{"routing":{"isis":{` + n + `,"interfaces":{"evil":{}}}}}`, "interface name"},
		"same linux if":  {`{"routing":{"isis":{` + n + `,"interfaces":{"host-w8l0":{},"twin":{}}}}}`, "same Linux interface"},
		"into itself":    {`{"routing":{"isis":{` + n + `,"redistribute":{"isis":{}}}}}`, "into itself"},
		"bad route map":  {`{"routing":{"isis":{` + n + `,"redistribute":{"static":{"routeMap":"a b"}}}}}`, "route map"},
		"redist metric":  {`{"routing":{"isis":{` + n + `,"redistribute":{"static":{"metric":16777216}}}}}`, "metric"},
		"bad vrf":        {`{"routing":{"isis":{` + n + `,"vrf":"a b"}}}`, "vrf name"},
	} {
		t.Run(name, func(t *testing.T) {
			o := parse(t, tc.doc).GetRouting().GetIsis()
			_, err := isis.Render(o)
			if err == nil {
				_, err = isis.RenderInterfaces(o, mapIf)
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
	raw := `{"vrfs":[{"vrf":"default","areas":[{"area":"vrx","circuits":[
	   {"circuit":0,"adj":"r2","interface":"w8-l0","level":2,"state":"Up","expires-in":"28s"},
	   {"circuit":1},
	   {"circuit":2,"system-id":"0000.0000.0003","interface":"w8-l1","level":"1","adj-state":"Initializing"}]}]}]}`
	as, err := isis.ParseNeighbors([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(as) != 2 || as[0].SystemID != "0000.0000.0003" || as[0].State != "Initializing" || as[0].Level != "1" ||
		as[1].SystemID != "r2" || as[1].Interface != "w8-l0" || as[1].Level != "2" || as[1].VRF != "default" || as[1].Area != "vrx" {
		t.Fatalf("%+v", as)
	}
	single, err := isis.ParseNeighbors([]byte(`{"areas":[{"area":"vrx","circuits":[{"adj":"r9","interface":"e0","level":2,"state":"Up"}]}]}`))
	if err != nil || len(single) != 1 || single[0].VRF != "default" {
		t.Fatalf("%+v %v", single, err)
	}
	if as, err := isis.ParseNeighbors([]byte("{}")); as != nil || err != nil {
		t.Fatal(as, err)
	}
	if _, err := isis.ParseNeighbors([]byte("[")); err == nil {
		t.Fatal("broken JSON accepted")
	}
	snap, err := isis.PollAdjacencies(context.Background(), func(context.Context, frr.ShowCommand) (json.RawMessage, error) { return json.RawMessage(raw), nil })
	if err != nil || snap["default|vrx|r2|w8-l0|2"] != "Up" || len(snap) != 2 {
		t.Fatalf("poll %v %v", snap, err)
	}
}
