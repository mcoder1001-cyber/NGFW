package desired

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/gso"
	"ngfw/agent/internal/descriptors/lldp"
	"ngfw/agent/internal/descriptors/nsim"
	"ngfw/agent/internal/descriptors/span"
	"ngfw/agent/internal/scheduler"
)

// lbgsSink records objects, errors and warnings by pointer (F-loopback-bvi-gso-lldp-span builder tests).
type lbgsSink struct {
	kvs      map[scheduler.Key]proto.Message
	errors   map[string]string
	warnings map[string]string
}

func newLbgsSink() *lbgsSink {
	return &lbgsSink{kvs: map[scheduler.Key]proto.Message{}, errors: map[string]string{}, warnings: map[string]string{}}
}
func (s *lbgsSink) Add(k scheduler.Key, v proto.Message, _ string) { s.kvs[k] = v }
func (s *lbgsSink) Errorf(p, rule, _ string, _ ...any)             { s.errors[p] = rule }
func (s *lbgsSink) Warnf(p, rule, _ string, _ ...any)              { s.warnings[p] = rule }

func lbgsParse(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

// Mirror sessions: one span.mirror per session with the configuration defaults (both, device); Retrieve
// order (sorted by the dump) is reported back in the stored document's order.
func TestMirrorRoundTripKeepsDocumentOrder(t *testing.T) {
	ds := lbgsParse(t, `{"interfaces": {"loop1": {"mirror": [
	  {"destination": "loop3", "direction": "tx", "level": "l2"},
	  {"destination": "gre7"},
	  {"destination": "loop2", "direction": "rx"}]}, "loop2": {}, "loop3": {}}}`)
	s := newLbgsSink()
	Mirror(s, ds.GetInterfaces())
	if len(s.kvs) != 3 || len(s.errors) != 0 {
		t.Fatalf("objects %v errors %v", s.kvs, s.errors)
	}
	if !proto.Equal(s.kvs[span.Key("loop1", "gre7", false)], MirrorValue("loop1", "gre7", "both", false)) {
		t.Fatalf("defaults: %v", s.kvs[span.Key("loop1", "gre7", false)])
	}
	// the dump's order (sorted keys) differs from the document's
	var kvs []scheduler.KV
	for _, k := range []scheduler.Key{span.Key("loop1", "gre7", false), span.Key("loop1", "loop2", false), span.Key("loop1", "loop3", true)} {
		kvs = append(kvs, scheduler.KV{Key: k, Value: s.kvs[k]})
	}
	back := &vrxv1.DesiredState{}
	AssembleMirror(back, kvs, ds.GetInterfaces())
	got := back.GetInterfaces()["loop1"].GetMirror()
	want := []string{"loop3/tx/l2", "gre7/both/device", "loop2/rx/device"}
	for i, m := range got {
		if i >= len(want) || m.GetDestination()+"/"+m.GetDirection()+"/"+m.GetLevel() != want[i] {
			t.Fatalf("order %v", got)
		}
	}
	// a session VPP has that the document does not name comes last (sorted)
	extra := scheduler.KV{Key: span.Key("loop1", "loop9", false), Value: MirrorValue("loop1", "loop9", "rx", false)}
	back = &vrxv1.DesiredState{}
	AssembleMirror(back, append([]scheduler.KV{extra}, kvs...), ds.GetInterfaces())
	if got := back.GetInterfaces()["loop1"].GetMirror(); len(got) != 4 || got[3].GetDestination() != "loop9" {
		t.Fatalf("extra session %v", got)
	}
}

func TestMirrorAndGsoProjectionErrors(t *testing.T) {
	ds := lbgsParse(t, `{"interfaces": {"loop1": {"gso": true, "mirror": [{"destination": "loop1"}, {"destination": "loop2", "level": "l3"}]},
	  "loop2": {"gso": false}}}`)
	s := newLbgsSink()
	Mirror(s, ds.GetInterfaces())
	Gso(s, ds.GetInterfaces())
	if s.errors["/interfaces/loop1/mirror/0/destination"] == "" || s.errors["/interfaces/loop1/mirror/1/level"] == "" {
		t.Fatalf("errors %v", s.errors)
	}
	if _, ok := s.kvs[gso.Key("loop1")]; !ok || len(s.kvs) != 1 {
		t.Fatalf("objects %v", s.kvs)
	}
	// Retrieve: on for loop1, the stored `gso: false` of loop2 reported as false, nothing for an unnamed one
	back := &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{"loop2": {}, "loop3": {}}}
	AssembleGso(back, []scheduler.KV{{Key: gso.Key("loop1"), Value: gso.Interface{Interface: "loop1"}.Proto()}}, ds.GetInterfaces())
	if !back.GetInterfaces()["loop1"].GetGso() || back.GetInterfaces()["loop2"].Gso == nil || back.GetInterfaces()["loop2"].GetGso() || back.GetInterfaces()["loop3"].Gso != nil {
		t.Fatalf("assembled %v", back.GetInterfaces())
	}
}

func TestLldpProjection(t *testing.T) {
	ds := lbgsParse(t, `{"services": {"lldp": {"enabled": true, "systemName": "vrx", "txHold": 4, "txIntervalSec": 30, "interfaces": [
	  {"interface": "loop1", "portDescription": "up", "mgmtIpv6": "2001:DB8::1"}, {"interface": "loop1"}, {"interface": "loop2", "mgmtIpv4": "::1"}]}}}`)
	// slot agent: per-interface only; VPP-global fields reported unsupported; the whole leaf write-only
	s := newLbgsSink()
	Lldp(s, ds.GetServices().GetLldp(), false)
	if _, ok := s.kvs[lldp.KeyGlobal()]; ok {
		t.Fatal("slot agent projected lldp.global")
	}
	if v, ok := s.kvs[lldp.KeyInterface("loop1")]; !ok || v.(interface{ String() string }).String() == "" {
		t.Fatalf("objects %v", s.kvs)
	}
	for p, rule := range map[string]string{"/services/lldp/systemName": lbgsUnsupported, "/services/lldp/txHold": lbgsUnsupported, "/services/lldp": lbgsWriteOnly} {
		if s.warnings[p] != rule {
			t.Errorf("warning %s = %q, want %q", p, s.warnings[p], rule)
		}
	}
	if s.errors["/services/lldp/interfaces/1/interface"] == "" || s.errors["/services/lldp/interfaces/2/mgmtIpv4"] == "" {
		t.Fatalf("errors %v", s.errors)
	}
	// the management IPv6 address is canonicalised (lldp.Interface.Validate wants the canonical form)
	s = newLbgsSink()
	Lldp(s, ds.GetServices().GetLldp(), true)
	if _, ok := s.kvs[lldp.KeyGlobal()]; !ok || len(s.warnings) != 1 {
		t.Fatalf("owner: objects %v warnings %v", s.kvs, s.warnings)
	}
	if got := s.kvs[lldp.KeyInterface("loop1")]; !proto.Equal(got, lldpValue("loop1", "up", "2001:db8::1")) {
		t.Fatalf("interface value %v", got)
	}
	// disabled: nothing applied, still noted write-only for the drift view
	s = newLbgsSink()
	Lldp(s, lbgsParse(t, `{"services": {"lldp": {"enabled": false, "txHold": 4}}}`).GetServices().GetLldp(), true)
	if len(s.kvs) != 0 || s.warnings["/services/lldp"] != lbgsWriteOnly {
		t.Fatalf("disabled: %v %v", s.kvs, s.warnings)
	}
}

func TestNsimProjection(t *testing.T) {
	ds := lbgsParse(t, `{"services": {"nsim": {"delayMs": 12.5, "bandwidthMbps": 1.5, "dropFraction": 0.003,
	  "crossConnect": {"a": "loop1", "b": "loop2"}, "outputInterfaces": ["loop3"]}}}`)
	c := NsimConfigOf(ds.GetServices().GetNsim())
	if c.DelayUsec != 12500 || c.BandwidthBps != 1500000 || c.PacketSize != 1500 || c.PacketsPerDrop != 333 {
		t.Fatalf("units %+v", c)
	}
	s := newLbgsSink()
	Nsim(s, ds.GetServices().GetNsim(), LoopbackBviGsoLldpSpanEnv{})
	if len(s.kvs) != 0 || s.warnings["/services/nsim"] != lbgsUnsupported {
		t.Fatalf("slot agent: %v %v", s.kvs, s.warnings)
	}
	// review M2: the globals owner without the lab gate applies nothing either
	s = newLbgsSink()
	Nsim(s, ds.GetServices().GetNsim(), LoopbackBviGsoLldpSpanEnv{GlobalsOwner: true})
	if len(s.kvs) != 0 || s.warnings["/services/nsim"] != lbgsUnsupported {
		t.Fatalf("globals owner without VRX_NSIM=lab: %v %v", s.kvs, s.warnings)
	}
	s = newLbgsSink()
	Nsim(s, ds.GetServices().GetNsim(), LoopbackBviGsoLldpSpanEnv{GlobalsOwner: true, Nsim: true})
	for _, k := range []scheduler.Key{nsim.ConfigKey(), nsim.CrossConnectKey(), nsim.OutputKey("loop3")} {
		if _, ok := s.kvs[k]; !ok {
			t.Fatalf("missing %s: %v", k, s.kvs)
		}
	}
	if s.warnings["/services/nsim"] != lbgsWriteOnly {
		t.Fatalf("owner warnings %v", s.warnings)
	}
}

// Services members: lldp and nsim are implemented, other non-empty members are unsupported (the seam).
func TestServicesMembers(t *testing.T) {
	if !ServicesMembers["lldp"] || !ServicesMembers["nsim"] {
		t.Fatalf("members %v", ServicesMembers)
	}
	s := newLbgsSink()
	ds := lbgsParse(t, `{"services": {"lldp": {"enabled": false}, "snmp": {"enabled": true}, "dhcp": {}}}`)
	LoopbackBviGsoLldpSpan(s, ds, map[string]bool{"services": true}, LoopbackBviGsoLldpSpanEnv{})
	if s.warnings["/services/snmp"] != lbgsUnsupported || s.warnings["/services/lldp"] != lbgsWriteOnly || s.warnings["/services/dhcp"] != "" {
		t.Fatalf("warnings %v", s.warnings)
	}
	back := &vrxv1.DesiredState{}
	AssembleLoopbackBviGsoLldpSpan(back, nil, map[string]bool{"services": true}, nil)
	if back.Services == nil {
		t.Fatal("services must be present when requested")
	}
}

func lldpValue(name, desc, ip6 string) proto.Message {
	return df7.Encode(lldp.Interface{Interface: name, PortDesc: desc, MgmtIP6: ip6})
}
