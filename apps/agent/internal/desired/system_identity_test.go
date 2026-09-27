package desired

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/sysident"
	"ngfw/agent/internal/scheduler"
)

func TestSystemIdentityProjection(t *testing.T) {
	var s sink
	SystemIdentity(&s, &vrxv1.DesiredState{System: &vrxv1.SystemConfig{Hostname: proto.String("vrx-a")}})
	in, ok := s.value(sysident.Key).(*vrxv1.SystemConfig)
	if !ok || len(s.errs) != 0 || in.GetHostname() != "vrx-a" || in.GetTimezone() != "UTC" || s.pointers[sysident.Key] != "/system" {
		t.Fatalf("projection %v %v", in, s.errs)
	}
	// an absent domain still renders the defaults
	var s0 sink
	SystemIdentity(&s0, &vrxv1.DesiredState{})
	if v, _ := s0.value(sysident.Key).(*vrxv1.SystemConfig); v.GetHostname() != "vrx" {
		t.Fatalf("defaults %v", v)
	}
	// round trip through the assembler
	ds := &vrxv1.DesiredState{}
	AssembleSystemIdentity(ds, []scheduler.KV{{Key: sysident.Key, Value: in}})
	if !proto.Equal(ds.GetSystem(), in) {
		t.Fatal("assemble")
	}
	for ptr, sys := range map[string]*vrxv1.SystemConfig{
		"/system/banner/login": {Banner: &vrxv1.SystemBanner{Login: proto.String("x\x1b]0;pwned\x07")}},
		"/system/timezone":     {Timezone: proto.String("../etc/passwd")},
		"/system/dns/vrf":      {Dns: &vrxv1.SystemDns{Vrf: proto.String("mgmt")}},
	} {
		var sb sink
		SystemIdentity(&sb, &vrxv1.DesiredState{System: sys})
		if len(sb.kvs) != 0 || len(sb.errs) != 1 || !strings.HasPrefix(sb.errs[0], ptr+" "+RuleSystemIdentity) {
			t.Errorf("%s: %v %v", ptr, sb.kvs, sb.errs)
		}
	}
}
