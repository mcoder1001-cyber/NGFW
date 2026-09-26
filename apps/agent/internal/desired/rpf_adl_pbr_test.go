package desired

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/abf"
	"ngfw/agent/internal/descriptors/adl"
	autosdl "ngfw/agent/internal/descriptors/auto_sdl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/urpf"
	"ngfw/agent/internal/scheduler"
)

// recSink records what a builder emits.
type recSink struct {
	kvs      []scheduler.KV
	pointers map[scheduler.Key]string
	issues   []string // "<E|W> <pointer> <rule>"
}

func newSink() *recSink { return &recSink{pointers: map[scheduler.Key]string{}} }

func (s *recSink) Add(k scheduler.Key, v proto.Message, pointer string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
	s.pointers[k] = pointer
}
func (s *recSink) Errorf(pointer, rule, _ string, _ ...any) {
	s.issues = append(s.issues, "E "+pointer+" "+rule)
}
func (s *recSink) Warnf(pointer, rule, _ string, _ ...any) {
	s.issues = append(s.issues, "W "+pointer+" "+rule)
}

func (s *recSink) keys() []string {
	out := make([]string, 0, len(s.kvs))
	for _, kv := range s.kvs {
		out = append(out, string(kv.Key))
	}
	sort.Strings(out)
	return out
}

func (s *recSink) value(k string) proto.Message {
	for _, kv := range s.kvs {
		if string(kv.Key) == k {
			return kv.Value
		}
	}
	return nil
}

var vrfs = map[string]uint32{"default": 0, "red": 3001, "allow": 3002}

func rpfVrfID(n string) (uint32, bool) {
	if n == "" {
		return 0, true
	}
	id, ok := vrfs[n]
	return id, ok
}

func rpfParse(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

var rpfAll = map[string]bool{"interfaces": true, "vrfs": true, "routing": true, "services": true}

func TestPolicyIDs(t *testing.T) {
	r := &df2.IDRange{Lo: 10000, Hi: 10999}
	ids := PolicyIDs([]string{"b", "a", "c"}, r, nil)
	if len(ids) != 3 {
		t.Fatalf("ids = %v", ids)
	}
	seen := map[uint32]bool{}
	for n, id := range ids {
		if id < 10000 || id > 10999 || seen[id] {
			t.Fatalf("%s → %d (range, uniqueness)", n, id)
		}
		seen[id] = true
	}
	// stable: adding a name keeps the others' ids (no collision here); order of input irrelevant
	more := PolicyIDs([]string{"c", "a", "b", "d"}, r, nil)
	for n, id := range ids {
		if more[n] != id {
			t.Fatalf("%s moved %d → %d", n, id, more[n])
		}
	}
	// a range of one id: the first name (sorted) gets it, the rest stay unassigned
	one := PolicyIDs([]string{"y", "x"}, &df2.IDRange{Lo: 7, Hi: 7}, nil)
	if len(one) != 1 || one["x"] != 7 {
		t.Fatalf("tiny range = %v", one)
	}
	// collisions probe linearly: 50 names in 50 ids fill the range exactly
	var names []string
	for i := range 50 {
		names = append(names, fmt.Sprintf("p%d", i))
	}
	full := PolicyIDs(names, &df2.IDRange{Lo: 100, Hi: 149}, nil)
	seen = map[uint32]bool{}
	for _, id := range full {
		seen[id] = true
	}
	if len(full) != 50 || len(seen) != 50 {
		t.Fatalf("probing: %d names, %d distinct ids", len(full), len(seen))
	}
	// product range (nil): never 0
	if id := PolicyIDs([]string{"x"}, nil, nil)["x"]; id == 0 || id > 1<<31-1 {
		t.Fatalf("nil range id %d", id)
	}
	// sticky: recorded ids are kept even when a new, earlier-sorting name hashes onto them (review L2)
	tiny := &df2.IDRange{Lo: 100, Hi: 101}
	first := PolicyIDs([]string{"b"}, tiny, nil)
	var clash string
	for i := range 200 { // a name that sorts before "b" and hashes to b's slot
		n := fmt.Sprintf("a%d", i)
		if PolicyIDs([]string{n}, tiny, nil)[n] == first["b"] {
			clash = n
			break
		}
	}
	if clash == "" {
		t.Fatal("no clashing name found")
	}
	if moved := PolicyIDs([]string{clash, "b"}, tiny, nil); moved["b"] == first["b"] {
		t.Fatalf("without records the colliding earlier name must take b's id: %v", moved)
	}
	kept := PolicyIDs([]string{clash, "b"}, tiny, map[string]uint32{"b": first["b"]})
	if kept["b"] != first["b"] || kept[clash] == first["b"] || kept[clash] < 100 || kept[clash] > 101 {
		t.Fatalf("recorded id not kept: %v (b had %d)", kept, first["b"])
	}
	// a record outside the range, or taken twice, is ignored
	odd := PolicyIDs([]string{"p", "q"}, tiny, map[string]uint32{"p": 5, "q": 100})
	if odd["q"] != 100 || odd["p"] != 101 {
		t.Fatalf("records outside the range must be re-probed: %v", odd)
	}
}

func TestRpfAdlPbrProjection(t *testing.T) {
	ds := rpfParse(t, `{
	  "interfaces": {
	    "loop301": {"vrf": "red", "urpf": {"ipv4": "strict", "ipv6": "loose"}, "adl": {"ipv4": true, "allowVrf": "allow", "defaultAllow": true}},
	    "loop302": {"urpf": {"direction": "rx"}, "adl": {"ipv4": false, "ipv6": false, "defaultAllow": true}},
	    "GigabitEthernet0/8/0": {"urpf": {"ipv4": "strict", "direction": "rx"}}
	  },
	  "routing": {
	    "static": [{"prefix": "0.0.0.0/0", "nextHops": [{"address": "192.0.2.1", "interface": "GigabitEthernet0/8/0"}, {"address": "192.0.2.5", "interface": "loop302"}]}],
	    "pbr": {
	      "policies": {
	        "via": {"acl": "lan-b", "priority": 10, "paths": [{"address": "10.3.2.254", "interface": "loop302", "vrf": "default", "weight": 2}]},
	        "deag6": {"acl": "lan-b", "paths": [{"vrf": "red"}]}
	      },
	      "attachments": [{"policy": "via", "interface": "loop301", "family": "ipv4"}, {"policy": "deag6", "interface": "loop301", "family": "ipv6"}]
	    }
	  },
	  "services": {"autoSdl": {"enabled": true, "threshold": 7}, "snmp": {"enabled": false}}
	}`)
	s := newSink()
	RpfAdlPbr(s, ds, rpfAll, rpfVrfID, RpfAdlPbrEnv{PolicyIDs: &df2.IDRange{Lo: 3000, Hi: 3999}, AutoSdl: true})
	ids := PolicyIDs([]string{"via", "deag6"}, &df2.IDRange{Lo: 3000, Hi: 3999}, nil)
	want := []string{
		fmt.Sprintf("abf.attach/%d/loop301/ipv4", ids["via"]),
		fmt.Sprintf("abf.attach/%d/loop301/ipv6", ids["deag6"]),
		fmt.Sprintf("abf.policy/%d", ids["deag6"]),
		fmt.Sprintf("abf.policy/%d", ids["via"]),
		"adl.allowlist/loop301",
		"adl.interface/loop301",
		"auto-sdl.config/global",
		"pbr.policy/deag6",
		"pbr.policy/via",
		"urpf.interface/GigabitEthernet0/8/0/ipv4/rx",
		"urpf.interface/loop301/ipv4/rx",
		"urpf.interface/loop301/ipv6/rx",
	}
	sort.Strings(want)
	if got := s.keys(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("keys:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if v := s.value("urpf.interface/loop301/ipv4/rx").(*urpf.Interface); v.GetMode() != urpf.Interface_STRICT || v.GetTableId() != 3001 {
		t.Fatalf("urpf = %v (table = the interface's VRF)", v)
	}
	if v := s.value("adl.allowlist/loop301").(*adl.Allowlist); v.GetFibId() != 3002 || !v.GetIp4() || v.GetIp6() || v.GetDefaultAdl() {
		t.Fatalf("allowlist = %v", v)
	}
	deag := s.value(fmt.Sprintf("abf.policy/%d", ids["deag6"])).(*abf.Policy)
	if p := deag.GetPaths()[0]; p.GetTableId() != 3001 || p.GetProto() != df2.FibPath_IP6 || p.GetNextHop() != "" || p.GetWeight() != 1 {
		t.Fatalf("deag path = %v (IPv6 lookup in table 3001: the policy is attached for ipv6)", p)
	}
	via := s.value(fmt.Sprintf("abf.policy/%d", ids["via"])).(*abf.Policy)
	if p := via.GetPaths()[0]; p.GetNextHop() != "10.3.2.254" || p.GetInterface() != "loop302" || p.GetWeight() != 2 || p.GetProto() != df2.FibPath_IP4 || via.GetAcl() != "lan-b" {
		t.Fatalf("via = %v", via)
	}
	if a := s.value(fmt.Sprintf("abf.attach/%d/loop301/ipv4", ids["via"])).(*abf.Attach); a.GetPriority() != 10 || a.GetIpv6() {
		t.Fatalf("attach = %v (priority = the policy's)", a)
	}
	if a := s.value(fmt.Sprintf("abf.attach/%d/loop301/ipv6", ids["deag6"])).(*abf.Attach); a.GetPriority() != 100 || !a.GetIpv6() {
		t.Fatalf("attach = %v (default priority 100)", a)
	}
	rec, err := DecodePbrPolicyRecord(s.value("pbr.policy/via"))
	if err != nil || rec != (PbrPolicyRecord{Name: "via", PolicyID: ids["via"], Priority: 10}) {
		t.Fatalf("record = %+v %v", rec, err)
	}
	var sdl autosdl.Config
	if err := dfkit.Decode(s.value("auto-sdl.config/global"), &sdl); err != nil || sdl != (autosdl.Config{Enable: true, Threshold: 7, RemoveTimeout: 300}) {
		t.Fatalf("auto-sdl = %+v %v", sdl, err)
	}
	wantIssues := []string{
		"W /interfaces/GigabitEthernet0~18~10/urpf/ipv4 interfaces.rpf-adl-pbr-urpf-strict-ecmp",
		"W /interfaces/loop301/adl/allowVrf agent.write-only",
		"W /interfaces/loop301/adl/defaultAllow agent.write-only",
		"W /interfaces/loop301/adl/ipv4 agent.write-only",
		"W /interfaces/loop301/adl/ipv6 agent.write-only",
		"W /services/autoSdl agent.write-only",
	}
	sort.Strings(s.issues)
	if strings.Join(s.issues, "\n") != strings.Join(wantIssues, "\n") {
		t.Fatalf("issues:\n%s", strings.Join(s.issues, "\n"))
	}
	if s.pointers["adl.interface/loop301"] != "/interfaces/loop301/adl" || s.pointers["pbr.policy/via"] != "/routing/pbr/policies/via" {
		t.Fatalf("pointers %v", s.pointers)
	}

	// only the domains in scope are projected
	s = newSink()
	RpfAdlPbr(s, ds, map[string]bool{"routing": true}, rpfVrfID, RpfAdlPbrEnv{})
	for _, k := range s.keys() {
		if !strings.HasPrefix(k, "abf.") && !strings.HasPrefix(k, "pbr.") {
			t.Fatalf("routing-only projection emitted %s", k)
		}
	}
	// not the globals owner: autoSdl is a warning, not an object
	s = newSink()
	RpfAdlPbr(s, ds, map[string]bool{"services": true}, rpfVrfID, RpfAdlPbrEnv{})
	if len(s.kvs) != 0 || !strings.Contains(strings.Join(s.issues, " "), "W /services/autoSdl agent.unsupported-field") {
		t.Fatalf("non-owner: %v %v", s.keys(), s.issues)
	}
}

func TestRpfAdlPbrProjectionErrors(t *testing.T) {
	ds := rpfParse(t, `{
	  "interfaces": {
	    "loop301": {"urpf": {"ipv4": "feasible"}, "adl": {"ipv4": true, "allowVrf": "allow", "defaultAllow": false}},
	    "loop302": {"urpf": {"ipv4": "loose", "direction": "both"}, "adl": {"ipv6": true, "allowVrf": "nope"}},
	    "loop303": {"adl": {"ipv6": true}}
	  },
	  "routing": {"pbr": {
	    "policies": {
	      "a#1": {"acl": "x", "paths": [{"vrf": "red"}]},
	      "noacl": {"paths": [{"vrf": "red"}]},
	      "badvrf": {"acl": "x", "paths": [{"vrf": "blue"}]},
	      "ifvrf": {"acl": "x", "paths": [{"interface": "loop301", "vrf": "red"}]},
	      "both": {"acl": "x", "paths": [{"vrf": "red"}]},
	      "badaddr": {"acl": "x", "paths": [{"address": "10.0.0.300"}]},
	      "nopaths": {"acl": "x"}
	    },
	    "attachments": [
	      {"policy": "both", "interface": "loop301", "family": "ipv4"},
	      {"policy": "both", "interface": "loop301", "family": "ipv6"},
	      {"policy": "ghost", "interface": "loop301"},
	      {"policy": "badvrf", "interface": "loop301"}
	    ]
	  }}
	}`)
	s := newSink()
	RpfAdlPbr(s, ds, rpfAll, rpfVrfID, RpfAdlPbrEnv{})
	sort.Strings(s.issues)
	want := []string{
		"E /interfaces/loop301/adl/defaultAllow interfaces.rpf-adl-pbr-adl-non-ip",
		"E /interfaces/loop301/urpf/ipv4 interfaces.rpf-adl-pbr-urpf-mode",
		"E /interfaces/loop302/adl/allowVrf interfaces.rpf-adl-pbr-adl-vrf",
		"E /interfaces/loop302/urpf/direction interfaces.rpf-adl-pbr-urpf-mode",
		"E /interfaces/loop303/adl/allowVrf interfaces.rpf-adl-pbr-adl-vrf",
		"E /routing/pbr/attachments/2/policy routing.rpf-adl-pbr-attachment",
		"E /routing/pbr/policies/a#1 routing.rpf-adl-pbr-policy",
		"E /routing/pbr/policies/badaddr/paths/0/address routing.rpf-adl-pbr-path",
		"E /routing/pbr/policies/badvrf/paths/0/vrf routing.rpf-adl-pbr-path",
		"E /routing/pbr/policies/both/paths/0 routing.rpf-adl-pbr-path",
		"E /routing/pbr/policies/ifvrf/paths/0/vrf routing.rpf-adl-pbr-path",
		"E /routing/pbr/policies/noacl/acl routing.rpf-adl-pbr-policy",
		"E /routing/pbr/policies/nopaths/paths routing.rpf-adl-pbr-path",
	}
	if strings.Join(s.issues, "\n") != strings.Join(want, "\n") {
		t.Fatalf("issues:\n%s", strings.Join(s.issues, "\n"))
	}
	if len(s.kvs) != 0 {
		t.Fatalf("invalid objects emitted: %v", s.keys())
	}
}

func TestRpfAdlPbrAssemble(t *testing.T) {
	ids := PolicyIDs([]string{"via"}, nil, nil)
	kvs := []scheduler.KV{
		{Key: "urpf.interface/loop301/ipv4/rx", Value: &urpf.Interface{Interface: "loop301", Af: df2.AddressFamily_IPV4, Mode: urpf.Interface_STRICT, TableId: 3001}},
		{Key: "urpf.interface/loop301/ipv6/rx", Value: &urpf.Interface{Interface: "loop301", Af: df2.AddressFamily_IPV6, Mode: urpf.Interface_LOOSE, TableId: 3001}},
		{Key: "urpf.interface/Gi0/ipv4/tx", Value: &urpf.Interface{Interface: "Gi0", Direction: urpf.Interface_TX, Mode: urpf.Interface_LOOSE}},
		{Key: "adl.interface/loop301", Value: &adl.Interface{Interface: "loop301"}},
		{Key: PbrPolicyKey("via"), Value: dfkit.Encode(PbrPolicyRecord{Name: "via", PolicyID: ids["via"], Priority: 10})},
		{Key: AbfPolicyKey(ids["via"]), Value: &abf.Policy{PolicyId: ids["via"], Acl: "lan-b", Paths: []*df2.FibPath{{NextHop: "10.3.2.254", Interface: "loop302", Weight: 1}}}},
		{Key: AbfPolicyKey(3999), Value: &abf.Policy{PolicyId: 3999, Acl: "lan-b", Paths: []*df2.FibPath{{TableId: 3001, Weight: 1}}}},
		{Key: "abf.attach/3999/loop302/ipv6", Value: &abf.Attach{PolicyId: 3999, Interface: "loop302", Priority: 5, Ipv6: true}},
		{Key: "abf.attach/x/loop301/ipv4", Value: &abf.Attach{PolicyId: ids["via"], Interface: "loop301", Priority: 10}},
	}
	ds := &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{
		"loop301": {Enabled: proto.Bool(true), Vrf: proto.String("red")},
		"loop302": {Enabled: proto.Bool(true), Vrf: proto.String("default")},
	}}
	stored := map[string]*vrxv1.Interface{
		"loop302": {Urpf: &vrxv1.UrpfConfig{Direction: proto.String("rx")}, Adl: &vrxv1.AdlConfig{DefaultAllow: proto.Bool(true)}},
	}
	names := map[uint32]string{0: "default", 3001: "red"}
	RpfAdlPbrAssemble(ds, kvs, rpfAll, func(id uint32) string { return names[id] }, stored)
	want := rpfParse(t, `{
	  "interfaces": {
	    "loop301": {"enabled": true, "vrf": "red", "urpf": {"ipv4": "strict", "ipv6": "loose", "direction": "rx"}, "adl": {}},
	    "loop302": {"enabled": true, "vrf": "default", "urpf": {"direction": "rx"}, "adl": {"defaultAllow": true}},
	    "Gi0": {"enabled": false, "promiscuous": false, "vrf": "default", "urpf": {"ipv4": "loose", "direction": "tx"}}
	  },
	  "routing": {"pbr": {
	    "policies": {
	      "via": {"acl": "lan-b", "priority": 10, "paths": [{"address": "10.3.2.254", "interface": "loop302", "vrf": "default", "weight": 1}]},
	      "#3999": {"acl": "lan-b", "priority": 100, "paths": [{"vrf": "red", "weight": 1}]}
	    },
	    "attachments": [
	      {"policy": "#3999", "interface": "loop302", "family": "ipv6"},
	      {"policy": "via", "interface": "loop301", "family": "ipv4"}
	    ]
	  }},
	  "services": {}
	}`)
	if !proto.Equal(ds, want) {
		t.Fatalf("assembled:\n%s\nwant\n%s", protojson.Format(ds), protojson.Format(want))
	}
	// nothing of the feature: no pbr; services present (an implemented domain) and empty
	empty := &vrxv1.DesiredState{}
	RpfAdlPbrAssemble(empty, nil, rpfAll, func(uint32) string { return "default" }, nil)
	if !proto.Equal(empty, &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{}}) {
		t.Fatalf("empty assemble = %s", protojson.Format(empty))
	}
}
