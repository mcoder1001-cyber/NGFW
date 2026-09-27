package rip_test

import (
	"context"
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
	"ngfw/agent/internal/renderers/frr/rip"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fullDoc = `{
 "interfaces": {"host-w8l0": {"lcp": {"hostIfName": "w8-l0"}}, "loop0": {"lcp": {"hostIfName": "w8-lo"}}},
 "routing": {"rip": {
  "vrf": "default", "defaultMetric": 2, "networks": ["10.8.0.0/16", "192.0.2.0/24"],
  "interfaces": {"host-w8l0": {}, "loop0": {"passive": true}},
  "redistribute": {"bgp": {"routeMap": "rm-in"}, "connected": {"metric": 3}, "static": {}}
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
	r := frr.New(renderers.NewRecordingRunner(), frr.WithPaths(frr.TestPaths("w8")), frr.WithSections(rip.Section{}),
		frr.WithInterfaceMapper(mapIf))
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "full.golden", string(files[r.Paths().ConfFile()].Content))
}

func TestVRFAndEmpty(t *testing.T) {
	lines, err := rip.Render(parse(t, `{"routing":{"rip":{"vrf":"red"}}}`).GetRouting().GetRip(), mapIf)
	if err != nil || strings.Join(lines, "|") != "router rip vrf red| version 2|exit" {
		t.Fatalf("%q %v", lines, err)
	}
	if lines, err := rip.Render(nil, mapIf); lines != nil || err != nil {
		t.Fatal(lines, err)
	}
}

func TestRenderErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		doc  string
		want string
	}{
		"v6 network":     {`{"routing":{"rip":{"networks":["2001:db8::/32"]}}}`, "not an IPv4 prefix"},
		"hostile net":    {`{"routing":{"rip":{"networks":["10.0.0.0/8\nrouter bgp 1"]}}}`, "not an IPv4 prefix"},
		"host bits":      {`{"routing":{"rip":{"networks":["10.0.0.1/8"]}}}`, "host bits"},
		"dup network":    {`{"routing":{"rip":{"networks":["10.0.0.0/8","10.0.0.0/8"]}}}`, "twice"},
		"metric 0":       {`{"routing":{"rip":{"defaultMetric":0}}}`, "default metric"},
		"metric 17":      {`{"routing":{"rip":{"defaultMetric":17}}}`, "default metric"},
		"unmapped":       {`{"routing":{"rip":{"interfaces":{"loop9":{}}}}}`, "no Linux interface"},
		"hostile ifname": {`{"routing":{"rip":{"interfaces":{"evil":{}}}}}`, "interface name"},
		"same linux if":  {`{"routing":{"rip":{"interfaces":{"host-w8l0":{},"twin":{}}}}}`, "same Linux interface"},
		"into itself":    {`{"routing":{"rip":{"redistribute":{"rip":{}}}}}`, "into itself"},
		"redist metric":  {`{"routing":{"rip":{"redistribute":{"static":{"metric":17}}}}}`, "metric"},
		"bad route map":  {`{"routing":{"rip":{"redistribute":{"static":{"routeMap":"a b"}}}}}`, "route map"},
		"bad vrf":        {`{"routing":{"rip":{"vrf":"a b"}}}`, "vrf name"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := rip.Render(parse(t, tc.doc).GetRouting().GetRip(), mapIf)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if !errors.Is(err, frr.ErrInput) && !errors.Is(err, renderers.ErrUnsafe) {
				t.Fatalf("err %v is not an input error", err)
			}
		})
	}
}
