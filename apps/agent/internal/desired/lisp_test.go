package desired

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
)

type lispSink struct {
	kvs  []scheduler.KV
	errs []string
}

func (s *lispSink) Add(k scheduler.Key, v proto.Message, _ string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
}
func (s *lispSink) Errorf(p, rule, _ string, _ ...any)   { s.errs = append(s.errs, p+" "+rule) }
func (s *lispSink) Warnf(string, string, string, ...any) {}

// Builder → assembler round trip of the L2 and negative-mapping parts (the agent tests cover the rest).
func TestLispRoundTripL2AndNegative(t *testing.T) {
	in := &vrxv1.TunnelsConfig{}
	if err := protojson.Unmarshal([]byte(`{"lisp": {"enabled": true, "gpe": true,
	  "locatorSets": {"s": {}},
	  "localEids": [{"vni": 7, "eid": "02:0B:00:00:00:01", "locatorSet": "s"}],
	  "eidTables": {"7": {"bridgeDomain": 70}},
	  "remoteMappings": [{"vni": 7, "eid": "02:0b:00:00:00:02", "action": "drop"},
	                     {"vni": 7, "eid": "02:0b:00:00:00:03", "rlocs": [{"address": "fd00::2"}, {"address": "fd00::1"}]}]}}`), in); err != nil {
		t.Fatal(err)
	}
	s := &lispSink{}
	Lisp(s, in, func(string) (uint32, bool) { return 0, true })
	if len(s.errs) != 0 {
		t.Fatal(s.errs)
	}
	ds := &vrxv1.DesiredState{}
	AssembleLisp(ds, s.kvs, func(uint32) string { return "default" })
	want := `{"enabled": true, "gpe": true, "locatorSets": {"s": {}},
	  "localEids": [{"vni": 7, "eid": "02:0b:00:00:00:01", "locatorSet": "s"}],
	  "eidTables": {"7": {"bridgeDomain": 70}},
	  "remoteMappings": [{"vni": 7, "eid": "02:0b:00:00:00:02", "action": "drop"},
	                     {"vni": 7, "eid": "02:0b:00:00:00:03", "action": "no-action",
	                      "rlocs": [{"address": "fd00::1", "priority": 1, "weight": 1}, {"address": "fd00::2", "priority": 1, "weight": 1}]}]}`
	w := &vrxv1.LispConfig{}
	if err := protojson.Unmarshal([]byte(want), w); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(ds.GetTunnels().GetLisp(), w) {
		t.Fatalf("got %s", protojson.Format(ds.GetTunnels().GetLisp()))
	}
	// Nothing retrieved: tunnels stays unset (no echo of desired state).
	empty := &vrxv1.DesiredState{}
	AssembleLisp(empty, nil, nil)
	if empty.Tunnels != nil {
		t.Fatal("assembled an empty lisp")
	}
}

func TestLispBuilderErrors(t *testing.T) {
	s := &lispSink{}
	in := &vrxv1.TunnelsConfig{}
	_ = protojson.Unmarshal([]byte(`{"lisp": {"enabled": true, "eidTables": {"5": {"vrf": "nope"}},
	  "remoteMappings": [{"vni": 1, "eid": "bogus"}], "mapResolvers": ["x"]}}`), in)
	Lisp(s, in, func(n string) (uint32, bool) { return 0, n == "default" })
	if len(s.errs) != 3 {
		t.Fatalf("errors %v", s.errs)
	}
}
