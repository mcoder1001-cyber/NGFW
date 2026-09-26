package desired

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/descriptors/mactime"
	"ngfw/agent/internal/scheduler"
)

type l2Sink struct {
	kvs    map[scheduler.Key]proto.Message
	errors map[string]string
}

func (s *l2Sink) Add(k scheduler.Key, v proto.Message, _ string) { s.kvs[k] = v }
func (s *l2Sink) Errorf(p, rule, _ string, _ ...any)             { s.errors[p] = rule }
func (s *l2Sink) Warnf(string, string, string, ...any)           {}

// MAC-filter ranges: one VPP range per day (Sunday = 0), grouped back per time window in the canonical order
// (first day in mon…sun order, then start, end); 24:00 is the end of the day.
func TestMacFilterRoundTrip(t *testing.T) {
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{"routing":{"l2":{"macFilters":{"d":{"mac":"02:00:00:00:00:AA","action":"drop",
	  "ranges":[{"days":["sat","sun"],"start":"22:00","end":"24:00"},{"days":["mon"],"start":"08:30","end":"09:00"}]}}}}}`), ds); err != nil {
		t.Fatal(err)
	}
	s := &l2Sink{kvs: map[scheduler.Key]proto.Message{}, errors: map[string]string{}}
	BridgeL2(s, ds, func(string) (uint32, bool) { return 0, true })
	v, ok := s.kvs[mactime.RangeKey("d")]
	if !ok || len(s.errors) != 0 {
		t.Fatalf("objects %v errors %v", s.kvs, s.errors)
	}
	back := &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{}}
	AssembleBridgeL2(back, []scheduler.KV{{Key: mactime.RangeKey("d"), Value: v}}, nil, func(uint32) string { return "default" })
	got := back.GetRouting().GetL2().GetMacFilters()["d"]
	want := &vrxv1.BridgeL2MacFilter{Mac: proto.String("02:00:00:00:00:aa"), Action: proto.String("drop"), Ranges: []*vrxv1.BridgeL2MacFilterRange{
		{Days: []string{"mon"}, Start: proto.String("08:30"), End: proto.String("09:00")},
		{Days: []string{"sat", "sun"}, Start: proto.String("22:00"), End: proto.String("24:00")},
	}}
	if !proto.Equal(got, want) {
		t.Fatalf("round trip = %v, want %v", got, want)
	}
}

// Tag rewrite: pop operations carry no tags and never push_dot1q (VPP reports 0); push/translate map dot1ad to !push_dot1q.
func TestTagRewriteMapping(t *testing.T) {
	for name, op := range vtrOps {
		if TagRewriteName(op) != name {
			t.Errorf("TagRewriteName(%v) = %q, want %q", op, TagRewriteName(op), name)
		}
	}
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{"interfaces":{"x":{"subinterfaces":{"10":{"vlanId":10,"l2":{"bridgeDomain":"b","tagRewrite":{"op":"pop-1","dot1ad":false}}}}},
	  "y":{"l2":{"bridgeDomain":"b","tagRewrite":{"op":"push-2","tag1":10,"tag2":20,"dot1ad":true}}}},
	  "routing":{"l2":{"bridgeDomains":{"b":{"id":7001}}}}}`), ds); err != nil {
		t.Fatal(err)
	}
	s := &l2Sink{kvs: map[scheduler.Key]proto.Message{}, errors: map[string]string{}}
	BridgeL2(s, ds, func(string) (uint32, bool) { return 0, true })
	pop := s.kvs["l2.vlan-tag-rewrite/x.10"].(*l2.VlanTagRewrite)
	push := s.kvs["l2.vlan-tag-rewrite/y"].(*l2.VlanTagRewrite)
	if pop.GetPushDot1Q() || pop.GetTag1() != 0 || pop.GetBridgeDomain() != 7001 || pop.GetOp() != l2.VtrOp_VTR_OP_POP_1 {
		t.Errorf("pop-1 → %v", pop)
	}
	if push.GetPushDot1Q() || push.GetTag1() != 10 || push.GetTag2() != 20 {
		t.Errorf("push-2 802.1ad → %v", push)
	}
	if bd := s.kvs[l2.BridgeDomainKey(7001)].(*l2.BridgeDomain); !bd.GetFlood() || !bd.GetLearn() || bd.GetName() != "b" {
		t.Errorf("bridge-domain defaults: %v", bd)
	}
}
