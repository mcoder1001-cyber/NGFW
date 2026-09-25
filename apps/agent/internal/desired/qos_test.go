package desired

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/policer"
	"ngfw/agent/internal/descriptors/qos"
	"ngfw/agent/internal/scheduler"
)

// qosSink records a projection.
type qosSink struct {
	kvs      []scheduler.KV
	pointers map[scheduler.Key]string
	errs     []string // "<pointer> <rule>"
	warns    []string
}

func (s *qosSink) Add(k scheduler.Key, v proto.Message, pointer string) {
	if s.pointers == nil {
		s.pointers = map[scheduler.Key]string{}
	}
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
	s.pointers[k] = pointer
}
func (s *qosSink) Errorf(pointer, rule, format string, a ...any) {
	s.errs = append(s.errs, pointer+" "+rule+": "+fmt.Sprintf(format, a...))
}
func (s *qosSink) Warnf(pointer, rule, _ string, _ ...any) {
	s.warns = append(s.warns, pointer+" "+rule)
}

func (s *qosSink) keys() []string {
	var out []string
	for _, kv := range s.kvs {
		out = append(out, string(kv.Key))
	}
	sort.Strings(out)
	return out
}

func (s *qosSink) value(t *testing.T, k scheduler.Key) proto.Message {
	t.Helper()
	for _, kv := range s.kvs {
		if kv.Key == k {
			return kv.Value
		}
	}
	t.Fatalf("no object %s (have %v)", k, s.keys())
	return nil
}

const qosDoc = `{"services": {"qos": {
  "policers": {
    "w1-gold": {"description": "customer", "type": "2r3c-rfc2698", "rateUnit": "kbps", "cir": 20000, "eir": 40000, "cb": "25000", "eb": "50000",
                "round": "closest", "colorAware": false, "conformAction": {"action": "transmit"},
                "exceedAction": {"action": "mark-and-transmit", "dscp": 10}, "violateAction": {"action": "drop"}},
    "w1-pps": {"type": "1r2c", "rateUnit": "pps", "cir": 1000, "cb": "100", "round": "closest", "colorAware": false,
               "conformAction": {"action": "transmit"}, "exceedAction": {"action": "drop"}, "violateAction": {"action": "drop"}}
  },
  "shapers": {"w1-uplink": {"description": "V3", "rateKbps": 50000}, "w1-backup": {"rateKbps": 2000, "burstBytes": "4000"}},
  "maps": {
    "w1-remark": {"description": "EF→AF41", "id": 1001, "rows": {"ip": [{"from": 46, "to": 34}, {"from": 8, "to": 0}], "vlan": [{"from": 5, "to": 34}]}},
    "w1-pcp": {"rows": {"ip": [{"from": 46, "to": 5}]}}
  },
  "interfaces": {
    "loop1001": {"description": "customer", "policer": {"input": "w1-gold"}, "shaper": "w1-uplink", "record": "vlan", "store": {"source": "ip", "value": 0}},
    "loop1002": {"policer": {"output": "w1-pps"}, "record": "ip", "mark": {"map": "w1-remark", "output": "ip"}},
    "loop1003": {"policer": {"input": "w1-pps"}}
  }
}}}`

func parseDS(t *testing.T, doc string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(doc), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func withRange(t *testing.T, r *df7.IDRange) {
	t.Helper()
	prev := qosMapIDRange()
	SetQoSMapIDRange(r)
	t.Cleanup(func() { SetQoSMapIDRange(prev) })
}

func TestShaperBurstBytes(t *testing.T) {
	for rate, want := range map[uint32]uint64{1: 3000, 1000: 3000, 2401: 3002, 50000: 62500, 1_000_000: 1_250_000} {
		if got := ShaperBurstBytes(rate); got != want {
			t.Errorf("ShaperBurstBytes(%d) = %d, want %d", rate, got, want)
		}
	}
}

// TestQoSProjection: every leaf of services.qos lands on its DF-7 object (and nothing else), the shaper is a 1r2c
// egress policer "shaper:<name>" with the derived ≈10 ms burst, the map without an id gets the lowest free id of the
// agent's range, and the write-only attachments are marked for the drift view.
func TestQoSProjection(t *testing.T) {
	withRange(t, &df7.IDRange{Lo: 1000, Hi: 1999})
	s := &qosSink{}
	QoS(s, parseDS(t, qosDoc).GetServices().GetQos())
	if len(s.errs) != 0 {
		t.Fatalf("errors: %v", s.errs)
	}
	want := []string{
		"policer.interface/loop1001/input", "policer.interface/loop1001/output", "policer.interface/loop1002/output",
		"policer.interface/loop1003/input",
		"policer.policer/shaper:w1-backup", "policer.policer/shaper:w1-uplink", "policer.policer/w1-gold", "policer.policer/w1-pps",
		"qos.egress-map/1000", "qos.egress-map/1001", "qos.mark/loop1002/ip", "qos.meta/services.qos",
		"qos.record/loop1001/vlan", "qos.record/loop1002/ip", "qos.store/loop1001/ip",
	}
	if got := s.keys(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("keys\n got %v\nwant %v", got, want)
	}
	up, _ := df7.Decode[policer.Policer](s.value(t, "policer.policer/shaper:w1-uplink"))
	if up.Type != policer.Type1R2C || up.CIR != 50000 || up.CB != 62500 || up.Exceed.Type != policer.ActDrop || up.Conform.Type != policer.ActTransmit || up.RateType != policer.RateKbps {
		t.Fatalf("shaper policer %+v", up)
	}
	if bk, _ := df7.Decode[policer.Policer](s.value(t, "policer.policer/shaper:w1-backup")); bk.CB != 4000 {
		t.Fatalf("explicit burst %+v", bk)
	}
	gold, _ := df7.Decode[policer.Policer](s.value(t, "policer.policer/w1-gold"))
	if gold.Type != policer.Type2R3C2698 || gold.EIR != 40000 || gold.EB != 50000 || gold.Exceed != (policer.Action{Type: policer.ActMark, DSCP: 10}) {
		t.Fatalf("gold %+v", gold)
	}
	if out, _ := df7.Decode[policer.Attachment](s.value(t, "policer.interface/loop1001/output")); out.Policer != "shaper:w1-uplink" {
		t.Fatalf("shaper attachment %+v", out)
	}
	pcp, _ := df7.Decode[qos.EgressMap](s.value(t, "qos.egress-map/1000"))
	if len(pcp.IP) != 256 || pcp.IP[46] != 5 || pcp.VLAN != nil {
		t.Fatalf("auto-id map %+v", pcp)
	}
	remark, _ := df7.Decode[qos.EgressMap](s.value(t, "qos.egress-map/1001"))
	if remark.IP[46] != 34 || remark.IP[8] != 0 || remark.VLAN[5] != 34 {
		t.Fatalf("map %+v", remark)
	}
	if mk, _ := df7.Decode[qos.Mark](s.value(t, "qos.mark/loop1002/ip")); mk.Map != 1001 {
		t.Fatalf("mark %+v", mk)
	}
	if st, _ := df7.Decode[qos.Store](s.value(t, "qos.store/loop1001/ip")); st.Value != 0 || st.Source != "ip" {
		t.Fatalf("store %+v", st)
	}
	meta, _ := df7.Decode[qos.DocMeta](s.value(t, qos.KeyMeta()))
	if meta.Maps["w1-pcp"].ID != 1000 || meta.Maps["w1-pcp"].ExplicitID || !meta.Maps["w1-remark"].ExplicitID ||
		meta.Policers["w1-gold"] != "customer" || !meta.Shapers["w1-backup"].Burst || meta.Shapers["w1-uplink"].Description != "V3" ||
		meta.Interfaces["loop1001"] != "customer" {
		t.Fatalf("meta %+v", meta)
	}
	wantWarn := []string{
		"/services/qos/interfaces/loop1001/policer agent.write-only", "/services/qos/interfaces/loop1001/shaper agent.write-only",
		"/services/qos/interfaces/loop1002/policer agent.write-only", "/services/qos/interfaces/loop1003 agent.write-only",
	}
	if strings.Join(s.warns, "|") != strings.Join(wantWarn, "|") {
		t.Fatalf("warnings\n got %v\nwant %v", s.warns, wantWarn)
	}
	// dependencies go through the interface alias and the map: the scheduler removes marks before maps
	if s.pointers["qos.mark/loop1002/ip"] != "/services/qos/interfaces/loop1002/mark" || s.pointers["policer.interface/loop1001/output"] != "/services/qos/interfaces/loop1001/shaper" ||
		s.pointers["policer.interface/loop1001/input"] != "/services/qos/interfaces/loop1001/policer/input" {
		t.Fatalf("pointers %v", s.pointers)
	}
}

// TestQoSAssembleRoundTrip: assemble(project(doc)) is the document again (defaults filled in), and projecting it
// gives the same objects — what the agent's Retrieve reports for a converged data plane (write-only attachments
// included here because the desired KVs carry them).
func TestQoSAssembleRoundTrip(t *testing.T) {
	withRange(t, &df7.IDRange{Lo: 1000, Hi: 1999})
	in := parseDS(t, qosDoc)
	s := &qosSink{}
	QoS(s, in.GetServices().GetQos())
	out := &vrxv1.DesiredState{}
	AssembleQoS(out, s.kvs)
	if !proto.Equal(out.GetServices().GetQos(), in.GetServices().GetQos()) {
		t.Fatalf("round trip\n got %s\nwant %s", protojson.Format(out.GetServices().GetQos()), protojson.Format(in.GetServices().GetQos()))
	}
	again := &qosSink{}
	QoS(again, out.GetServices().GetQos())
	if strings.Join(again.keys(), " ") != strings.Join(s.keys(), " ") {
		t.Fatal("project(assemble(project(doc))) keys differ")
	}
	for _, kv := range s.kvs {
		if !proto.Equal(kv.Value, again.value(t, kv.Key)) {
			t.Errorf("%s differs after the round trip", kv.Key)
		}
	}
}

// TestQoSAssembleRetrieve: what a Retrieve reports — no write-only attachments; an interface that only had them is
// absent (its DryRun note covers the whole entry); a map id the record does not know shows as "map-<id>" with its
// id; a shaper whose burst differs from the derived one reports it; an empty domain is `qos: {}`.
func TestQoSAssembleRetrieve(t *testing.T) {
	withRange(t, &df7.IDRange{Lo: 1000, Hi: 1999})
	s := &qosSink{}
	QoS(s, parseDS(t, qosDoc).GetServices().GetQos())
	var retrieved []scheduler.KV
	for _, kv := range s.kvs {
		if kv.Key.Descriptor() != policer.NameInterface {
			retrieved = append(retrieved, kv)
		}
	}
	retrieved = append(retrieved, df7.KV(qos.KeyEgressMap(1500), qos.EgressMap{ID: 1500, IP: func() []int { r := make([]int, 256); r[1] = 2; return r }()}, nil))
	up := QosShaperSpec("w1-uplink", &vrxv1.QosShaper{RateKbps: proto.Uint32(50000)})
	up.CB = 999 // changed behind the agent's back
	for i, kv := range retrieved {
		if kv.Key == policer.KeyPolicer("shaper:w1-uplink") {
			retrieved[i] = df7.KV(kv.Key, up, nil)
		}
	}
	out := &vrxv1.DesiredState{}
	AssembleQoS(out, retrieved)
	q := out.GetServices().GetQos()
	if _, ok := q.GetInterfaces()["loop1003"]; ok {
		t.Fatal("an interface with only write-only leaves must not be reported")
	}
	if a := q.GetInterfaces()["loop1001"]; a.Policer != nil || a.Shaper != nil || a.GetRecord() != "vlan" || a.GetDescription() != "customer" {
		t.Fatalf("loop1001 %v", a)
	}
	if m := q.GetMaps()["map-1500"]; m.GetId() != 1500 || len(m.GetRows().GetIp()) != 1 || m.GetRows().GetIp()[0].GetTo() != 2 {
		t.Fatalf("unknown map %v", m)
	}
	if sh := q.GetShapers()["w1-uplink"]; sh.GetBurstBytes() != 999 {
		t.Fatalf("drifted burst not reported: %v", sh)
	}
	empty := &vrxv1.DesiredState{}
	AssembleQoS(empty, nil)
	if !proto.Equal(empty, &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Qos: &vrxv1.QosService{}}}) {
		t.Fatalf("empty %v", empty)
	}
}

func TestQoSProjectionErrors(t *testing.T) {
	withRange(t, &df7.IDRange{Lo: 1000, Hi: 1000})
	long := strings.Repeat("p", 57)
	doc := fmt.Sprintf(`{"services": {"qos": {
	  "policers": {"ok": {"cir": 1, "cb": "10"}, %q: {"cir": 1, "cb": "10"}},
	  "shapers": {"s": {"rateKbps": 100}},
	  "maps": {"a": {"rows": {"ip": [{"from": 1, "to": 2}]}}, "b": {"rows": {"ip": [{"from": 1, "to": 2}]}}, "c": {"id": 5, "rows": {"ip": [{"from": 1, "to": 2}]}}},
	  "interfaces": {
	    "loop1": {"store": {"source": "vlan", "value": 3}},
	    "loop2": {"shaper": "s", "policer": {"output": "ok"}},
	    "loop3": {"policer": {"input": "nope"}, "mark": {"map": "zz", "output": "ip"}}
	  }}}}`, long)
	s := &qosSink{}
	QoS(s, parseDS(t, doc).GetServices().GetQos())
	want := []string{
		"/services/qos/policers/" + long + " services.qos-flat-policer",
		"/services/qos/maps/c/id services.qos-flat-map-id",
		"/services/qos/maps/b services.qos-flat-map-id",
		"/services/qos/interfaces/loop1/store/source services.qos-flat-store-source",
		"/services/qos/interfaces/loop2/shaper services.qos-flat-egress",
		"/services/qos/interfaces/loop3/policer/input services.qos-references",
		"/services/qos/interfaces/loop3/mark/map services.qos-references",
	}
	if len(s.errs) != len(want) {
		t.Fatalf("errors %v", s.errs)
	}
	for i, w := range want {
		if !strings.HasPrefix(s.errs[i], w+":") {
			t.Errorf("error %d = %q, want %q…", i, s.errs[i], w)
		}
	}
	// the empty range (no VRX_VPP_TABLE_BASE, fail closed) owns no map id at all
	withRange(t, &df7.IDRange{Lo: 1, Hi: 0})
	s = &qosSink{}
	QoS(s, parseDS(t, `{"services": {"qos": {"maps": {"a": {"rows": {"ip": [{"from": 1, "to": 2}]}}}}}}`).GetServices().GetQos())
	if len(s.errs) != 1 || !strings.HasPrefix(s.errs[0], "/services/qos/maps/a services.qos-flat-map-id") {
		t.Fatalf("empty range: %v", s.errs)
	}
}

func TestServicesUnsupported(t *testing.T) {
	s := &qosSink{}
	ServicesUnsupported(s, parseDS(t, `{"services": {"qos": {"policers": {"a": {"cir": 1, "cb": "1"}}}, "snmp": {"enabled": true}, "lldp": {}}}`).GetServices())
	if strings.Join(s.warns, "|") != "/services/snmp agent.unsupported-field" {
		t.Fatalf("warnings %v", s.warns)
	}
}
