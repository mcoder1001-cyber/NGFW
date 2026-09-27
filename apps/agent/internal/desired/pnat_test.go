package desired_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/desired"
)

func TestPnatProjection(t *testing.T) {
	s := newSink()
	desired.Nat(s, natDoc(t, `{"pnat": {
	  "bindings": [{"name": "dns", "match": {"proto": "udp", "dst": "10.9.51.1", "dport": 53}, "rewrite": {"dst": "10.9.53.1", "dport": 5353}},
	               {"name": "web", "match": {"proto": "tcp", "dst": "10.9.52.2", "dport": 80}, "rewrite": {"src": "10.9.53.2"}}],
	  "attachments": [{"binding": "dns", "interface": "loop950", "point": "input"}, {"binding": "web", "interface": "loop950", "point": "input"},
	                  {"binding": "web", "interface": "loop951", "point": "output"}]}}`), vrfID)
	want := "pnat.attachment/loop950/input/tcp/any/any/10.9.52.2/80,pnat.attachment/loop950/input/udp/any/any/10.9.51.1/53," +
		"pnat.attachment/loop951/output/tcp/any/any/10.9.52.2/80,pnat.binding/tcp/any/any/10.9.52.2/80,pnat.binding/udp/any/any/10.9.51.1/53"
	if got := strings.Join(s.keys(), ","); got != want || len(s.errs) != 0 {
		t.Fatalf("keys %s errs %v", got, s.errs)
	}
	s = newSink()
	desired.Nat(s, natDoc(t, `{"pnat": {
	  "bindings": [{"name": "a", "match": {}, "rewrite": {"dst": "10.0.0.1"}},
	               {"name": "b", "match": {"dport": 80}, "rewrite": {"dst": "10.0.0.1"}},
	               {"name": "c", "match": {"dst": "fd00::1"}, "rewrite": {"dst": "10.0.0.1"}},
	               {"name": "d", "match": {"dst": "10.0.0.2"}, "rewrite": {}},
	               {"name": "e", "match": {"dst": "10.0.0.3"}, "rewrite": {"dst": "10.0.0.1"}},
	               {"name": "f", "match": {"dst": "10.0.0.3"}, "rewrite": {"dst": "10.0.0.4"}},
	               {"name": "g", "match": {"src": "10.0.0.5"}, "rewrite": {"dst": "10.0.0.4"}}],
	  "attachments": [{"binding": "zz", "interface": "i", "point": "input"},
	                  {"binding": "e", "interface": "i", "point": "input"},
	                  {"binding": "g", "interface": "i", "point": "input"},
	                  {"binding": "e", "interface": "i", "point": "input"}]}}`), vrfID)
	wantErr := "/nat/pnat/bindings/0/match " + desired.RulePnat + ",/nat/pnat/bindings/1/match/proto " + desired.RulePnat +
		",/nat/pnat/bindings/2/match/dst " + desired.RulePnat + ",/nat/pnat/bindings/3/rewrite " + desired.RulePnat +
		",/nat/pnat/bindings/5/match " + desired.RulePnat + ",/nat/pnat/attachments/0/binding " + desired.RulePnat +
		",/nat/pnat/attachments/2/binding " + desired.RulePnat + ",/nat/pnat/attachments/3 " + desired.RulePnat
	if got := strings.Join(s.errs, ","); got != wantErr {
		t.Fatalf("errs\n got %s\nwant %s", got, wantErr)
	}
}

// TestPnatRoundTrip: names are configuration-only labels, so the canonical document names bindings pnat-<n> in
// match-tuple order (what the assembler produces).
func TestPnatRoundTrip(t *testing.T) {
	in := `{"pnat": {"bindings": [{"name": "pnat-1", "match": {"proto": "tcp", "dst": "10.9.52.2", "dport": 80}, "rewrite": {"src": "10.9.53.2"}},
	  {"name": "pnat-2", "match": {"proto": "udp", "src": "10.9.51.1"}, "rewrite": {"dst": "10.9.53.1", "dport": 5353}}],
	  "attachments": [{"binding": "pnat-1", "interface": "loop950", "point": "input"}, {"binding": "pnat-2", "interface": "loop951", "point": "output"}]}}`
	s := newSink()
	desired.Nat(s, natDoc(t, in), vrfID)
	if len(s.errs) != 0 {
		t.Fatalf("errs %v", s.errs)
	}
	got := desired.AssembleNat(s.kvs, tableName)
	want := natDoc(t, in)
	if !proto.Equal(got, want) {
		gj, _ := protojson.Marshal(got)
		wj, _ := protojson.Marshal(want)
		t.Fatalf("round trip\n got %s\nwant %s", gj, wj)
	}
}

type infoSink struct {
	*sink
	infos []string
}

func (s *infoSink) Infof(pointer, rule, _ string, _ ...any) {
	s.infos = append(s.infos, pointer+" "+rule)
}

// TestCnatFeatureInfo: the derived cnat interface feature is reported as an info notice (questions Q4).
func TestCnatFeatureInfo(t *testing.T) {
	s := &infoSink{sink: newSink()}
	desired.Nat(s, natDoc(t, `{"cnat": {"snat": {"policy": "interface", "addresses": {"ipv4": "10.9.2.1"},
	  "interfaces": [{"interface": "host-w9w0", "table": "include-v4"}, {"interface": "host-w9l0", "table": "include-v4"}]}}}`), vrfID)
	if strings.Join(s.infos, ",") != "/nat/cnat/snat/interfaces "+desired.RuleCnatFeature || len(s.errs) != 0 {
		t.Fatalf("infos %v errs %v", s.infos, s.errs)
	}
}
