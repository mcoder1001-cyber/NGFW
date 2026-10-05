package bfd

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
	"strings"
	"testing"
)

func TestProfileTimers(t *testing.T) {
	rc := &frr.RenderContext{Desired: &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Bfd: &ngfwv1.BfdConfig{Profiles: map[string]*ngfwv1.BfdProfile{"fast": {DesiredMinTxUs: proto.Uint32(10000), RequiredMinRxUs: proto.Uint32(20000), DetectMultiplier: proto.Uint32(5)}}}}}}
	lines, e := (Section{}).Render(rc)
	if e != nil {
		t.Fatal(e)
	}
	want := "bfd\n profile fast\n  transmit-interval 10\n  receive-interval 20\n  detect-multiplier 5\n exit\nexit"
	if strings.Join(lines, "\n") != want {
		t.Fatalf("%v", lines)
	}
	rc.Desired.Routing.Bfd.Profiles["fast"].DesiredMinTxUs = proto.Uint32(10001)
	if _, e = (Section{}).Render(rc); e == nil {
		t.Fatal("must reject precision loss")
	}
}
func TestEmptyProfiles(t *testing.T) {
	if lines, e := (Section{}).Render(&frr.RenderContext{}); e != nil || len(lines) != 0 {
		t.Fatalf("%v %v", lines, e)
	}
}
