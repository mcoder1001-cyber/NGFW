package bgp_test

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
	"ngfw/agent/internal/renderers/frr/bgp"
	"ngfw/agent/internal/renderers/frr/policy"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// fullDoc exercises every field the two sections render (the shapes the live test proved against FRR 10.7).
const fullDoc = `{
 "interfaces": {"host-w8l0": {"lcp": {"hostIfName": "w8-l0"}}},
 "routing": {
  "policy": {
   "prefixLists": {
    "pl-low": {"description": "peer low half", "family": "ipv4", "rules": [{"seq": 5, "action": "permit", "prefix": "10.8.64.0/18", "le": 25}]},
    "pl-v6": {"family": "ipv6", "rules": [{"seq": 10, "action": "permit", "prefix": "::/0", "le": 128}, {"seq": 5, "action": "deny", "prefix": "2001:db8::/32", "ge": 48, "le": 64}]}
   },
   "routeMaps": {
    "rm-in": {"entries": [
      {"seq": 20, "action": "permit", "set": {"localPref": 150, "community": ["65080:20"], "communityAdditive": true}},
      {"seq": 10, "action": "deny", "description": "drop the low half", "match": {"prefixList": "pl-low"}}]},
    "rm-all": {"entries": [
      {"seq": 5, "action": "permit", "match": {"community": "65000:100", "asPath": "^65081_", "metric": 7, "tag": 9, "nextHopPrefixList": "pl-low", "interface": "host-w8l0"},
       "set": {"asPathPrepend": [65080, 65080], "med": 20, "weight": 100, "tag": 5, "nextHop": "10.8.9.1"}},
      {"seq": 6, "action": "permit", "match": {"prefixList": "pl-v6"}, "set": {"nextHop": "2001:db8::1"}}]}
   }
  },
  "bgp": {"asn": 65080, "routerId": "10.8.9.1", "gracefulRestart": true, "ebgpRequiresPolicy": false,
   "peerGroups": {"pg": {"remoteAs": 65081, "description": "p12 peers", "keepaliveSec": 3,
     "afi": {"ipv4Unicast": {"enabled": true, "routeMapIn": "rm-in", "routeMapOut": "rm-all", "softReconfig": true}}}},
   "neighbors": {
    "10.8.9.2": {"peerGroup": "pg", "passwordRef": "password/p12-peer", "updateSource": "host-w8l0", "description": "peer one"},
    "10.8.9.77": {"remoteAs": 65099, "shutdown": true, "ebgpMultihop": 255, "holdTimeSec": 9,
     "afi": {"ipv4Unicast": {"enabled": true, "prefixListIn": "pl-low", "maximumPrefixes": 1000, "nextHopSelf": true, "defaultOriginate": true},
             "ipv6Unicast": {"enabled": true, "prefixListIn": "pl-v6"}}},
    "2001:db8::2": {"remoteAs": 65082, "ebgpMultihop": 2, "afi": {"ipv4Unicast": {"enabled": false}, "ipv6Unicast": {}}}
   },
   "networks": [{"prefix": "10.8.250.0/24", "routeMap": "rm-all"}, {"prefix": "10.8.3.0/24"}, {"prefix": "2001:db8:8::/48"}],
   "redistribute": {"connected": {"metric": 10}, "static": {"routeMap": "rm-all"}}
  }
 }
}`

type rc struct{ secrets map[string]string }

func (r rc) Secret(ref string) (string, error) {
	if v, ok := r.secrets[ref]; ok {
		return v, nil
	}
	return "", frr.ErrNoSecretResolver
}

func (rc) MapInterface(n string) (string, bool) {
	if n == "host-w8l0" {
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

func TestRenderGolden(t *testing.T) {
	ds := parse(t, fullDoc)
	lines, err := bgp.Render(ds.GetRouting().GetBgp(), rc{secrets: map[string]string{"password/p12-peer": "VRX_TEST_PSK_P12_1"}}) //nolint:gosec // G101: test fixture literal
	if err != nil {
		t.Fatal(err)
	}
	pol, err := policy.Render(ds.GetRouting().GetPolicy(), rc{}.MapInterface)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "full.golden", strings.Join(append(pol, lines...), "\n")+"\n")
}

func TestRenderThroughFramework(t *testing.T) {
	ds := parse(t, fullDoc)
	r := frr.New(renderers.NewRecordingRunner(), frr.WithPaths(frr.TestPaths("w8")), frr.WithSections(bgp.Section{}, policy.Section{}),
		frr.WithInterfaceMapper(rc{}.MapInterface), frr.WithInterfaceLines(),
		frr.WithSecretResolver(frr.SecretResolverFunc(func(context.Context, string) (string, error) { return "VRX_TEST_PSK_P12_1", nil })))
	files, err := r.Render(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	conf := files[r.Paths().ConfFile()]
	if !conf.Secret || conf.Mode != 0o640 {
		t.Fatalf("frr.conf with a password must be Secret 0640: %+v", conf.Mode)
	}
	red := string(files.Redacted()[r.Paths().ConfFile()].Content)
	if strings.Contains(red, "VRX_TEST_PSK") {
		t.Fatal("redacted files hold the password")
	}
	s := string(conf.Content)
	if i, j := strings.Index(s, "router bgp 65080"), strings.Index(s, "ip prefix-list pl-low"); i < 0 || j < i {
		t.Fatalf("router bgp (order %d) must precede the filters (order %d):\n%s", bgp.OrderBGP, policy.OrderPolicy, s)
	}
}

func TestRenderErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		doc  string
		want string
	}{
		"no asn":             {`{"routing":{"bgp":{"neighbors":{}}}}`, "asn is required"},
		"no remote as":       {`{"routing":{"bgp":{"asn":1,"neighbors":{"10.0.0.1":{}}}}}`, "remoteAs is required"},
		"unknown group":      {`{"routing":{"bgp":{"asn":1,"neighbors":{"10.0.0.1":{"peerGroup":"x"}}}}}`, `peer group "x" does not exist`},
		"bad neighbor":       {`{"routing":{"bgp":{"asn":1,"neighbors":{"224.0.0.1":{"remoteAs":2}}}}}`, "not a unicast neighbour"},
		"same address twice": {`{"routing":{"bgp":{"asn":1,"neighbors":{"2001:db8::1":{"remoteAs":2},"2001:DB8::1":{"remoteAs":2}}}}}`, "same address"},
		"no resolver":        {`{"routing":{"bgp":{"asn":1,"neighbors":{"10.0.0.1":{"remoteAs":2,"passwordRef":"password/x"}}}}}`, "no secret resolver"},
		"unmapped source":    {`{"routing":{"bgp":{"asn":1,"neighbors":{"10.0.0.1":{"remoteAs":2,"updateSource":"loop9"}}}}}`, "has no Linux interface"},
		"bad timers":         {`{"routing":{"bgp":{"asn":1,"neighbors":{"10.0.0.1":{"remoteAs":2,"keepaliveSec":10,"holdTimeSec":5}}}}}`, "timers"},
		"redistribute bgp":   {`{"routing":{"bgp":{"asn":1,"redistribute":{"bgp":{}}}}}`, "into itself"},
		"bad router id":      {`{"routing":{"bgp":{"asn":1,"routerId":"2001:db8::1"}}}`, "dotted quad"},
		"host bits network":  {`{"routing":{"bgp":{"asn":1,"networks":[{"prefix":"10.0.0.1/24"}]}}}`, "not a network prefix"},
		"hostile desc":       {`{"routing":{"bgp":{"asn":1,"neighbors":{"10.0.0.1":{"remoteAs":2,"description":"a\nrouter bgp 2"}}}}}`, "description"},
		"bad vrf":            {`{"routing":{"bgp":{"asn":1,"vrf":"a b"}}}`, "vrf name"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := bgp.Render(parse(t, tc.doc).GetRouting().GetBgp(), rc{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if !errors.Is(err, frr.ErrInput) && !errors.Is(err, renderers.ErrUnsafe) && !errors.Is(err, frr.ErrNoSecretResolver) {
				t.Fatalf("err %v is not an input error", err)
			}
		})
	}
}

func TestPolicyErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		doc  string
		want string
	}{
		"bad name":        {`{"prefixLists":{"a b":{"rules":[]}}}`, "prefix list name"},
		"family mismatch": {`{"prefixLists":{"p":{"family":"ipv6","rules":[{"seq":1,"action":"permit","prefix":"10.0.0.0/8"}]}}}`, "family"},
		"ge too small":    {`{"prefixLists":{"p":{"rules":[{"seq":1,"action":"permit","prefix":"10.0.0.0/8","ge":8}]}}}`, "ge 8"},
		"bad action":      {`{"prefixLists":{"p":{"rules":[{"seq":1,"action":"allow","prefix":"10.0.0.0/8"}]}}}`, "permit or deny"},
		"seq > 65535":     {`{"routeMaps":{"r":{"entries":[{"seq":70000,"action":"permit"}]}}}`, "1–65535"},
		"dup seq":         {`{"routeMaps":{"r":{"entries":[{"seq":1,"action":"permit"},{"seq":1,"action":"deny"}]}}}`, "used twice"},
		"missing list":    {`{"routeMaps":{"r":{"entries":[{"seq":1,"action":"permit","match":{"prefixList":"nope"}}]}}}`, "does not exist"},
		"cli pipe":        {`{"routeMaps":{"r":{"entries":[{"seq":1,"action":"permit","match":{"asPath":"^1| 2"}}]}}}`, "CLI pipe"},
		"community range": {`{"routeMaps":{"r":{"entries":[{"seq":1,"action":"permit","set":{"community":["70000:1"]}}]}}}`, "0–65535"},
		"unmapped if":     {`{"routeMaps":{"r":{"entries":[{"seq":1,"action":"permit","match":{"interface":"loop7"}}]}}}`, "no Linux interface"},
		"prepend as 0":    {`{"routeMaps":{"r":{"entries":[{"seq":1,"action":"permit","set":{"asPathPrepend":[0]}}]}}}`, "AS 0"},
	} {
		t.Run(name, func(t *testing.T) {
			pol := &vrxv1.RoutingPolicy{}
			if err := protojson.Unmarshal([]byte(tc.doc), pol); err != nil {
				t.Fatal(err)
			}
			_, err := policy.Render(pol, frr.NoMapper)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseSummary(t *testing.T) {
	raw := `{"default":{"ipv4Unicast":{"routerId":"10.8.9.1","as":65080,"vrfName":"default","peers":{
	  "10.8.9.2":{"remoteAs":65081,"state":"Established","peerState":"OK","peerUptimeMsec":61000,"pfxRcd":50,"pfxSnt":1,"connectionsEstablished":2,"connectionsDropped":1,"desc":"peer one","msgRcvd":10,"msgSent":9},
	  "10.8.9.77":{"remoteAs":65099,"state":"Idle","peerState":"Admin","pfxRcd":0,"pfxSnt":0}}},
	 "ipv6Unicast":{"routerId":"10.8.9.1","as":65080,"peers":{"10.8.9.77":{"remoteAs":65099,"state":"Idle","peerState":"Admin"}}}},
	 "red":{"ipv4Unicast":{"routerId":"10.8.9.1","as":65080,"peers":{}}}}`
	insts, err := bgp.ParseSummary([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 2 || insts[0].VRF != "default" || insts[1].VRF != "red" || insts[0].ASN != 65080 || insts[0].RouterID != "10.8.9.1" {
		t.Fatalf("instances %+v", insts)
	}
	n := insts[0].Neighbors
	if len(n) != 2 || n[0].Address != "10.8.9.2" || n[0].State != "Established" || n[0].UptimeSec != 61 || n[0].PrefixesReceived != 50 ||
		n[0].Flaps != 1 || n[0].Established != 2 || n[0].Description != "peer one" || len(n[0].AFIs) != 1 {
		t.Fatalf("neighbour %+v", n[0])
	}
	if n[1].State != "Idle (Admin)" || n[1].UptimeSec != 0 || len(n[1].AFIs) != 2 {
		t.Fatalf("shutdown neighbour %+v", n[1])
	}
	if insts, err := bgp.ParseSummary([]byte("{}")); err != nil || insts != nil {
		t.Fatalf("empty: %v %v", insts, err)
	}
	if _, err := bgp.ParseSummary([]byte("[")); err == nil {
		t.Fatal("broken JSON accepted")
	}
	snap, err := bgp.PollNeighbors(context.Background(), func(context.Context, frr.ShowCommand) (json.RawMessage, error) { return json.RawMessage(raw), nil })
	if err != nil || snap["default|10.8.9.2"] != "Established" || snap["default|10.8.9.77"] != "Idle (Admin)" {
		t.Fatalf("poll %v %v", snap, err)
	}
	if v, p := bgp.SplitNeighborKey("default|10.8.9.2"); v != "default" || p != "10.8.9.2" {
		t.Fatal(v, p)
	}
}
