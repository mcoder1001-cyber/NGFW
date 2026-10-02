package desired

import (
	"google.golang.org/protobuf/proto"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
	"testing"
)

func TestPrometheusProjection(t *testing.T) {
	v := &vrxv1.ManagementPrometheus{Enabled: proto.Bool(true)}
	var s sink
	Prometheus(&s, &vrxv1.DesiredState{Management: &vrxv1.ManagementConfig{Prometheus: v}})
	if len(s.errs) != 0 || len(s.kvs) != 1 || s.pointers[PrometheusKey] != "/management/prometheus" {
		t.Fatal(s.errs, s.kvs)
	}
	ds := &vrxv1.DesiredState{}
	AssemblePrometheus(ds, []scheduler.KV{{Key: PrometheusKey, Value: s.value(PrometheusKey)}})
	if !proto.Equal(ds.GetManagement().GetPrometheus(), v) {
		t.Fatal(ds)
	}
	addr, err := PrometheusAddress(v)
	if err != nil || addr != "0.0.0.0:9101" {
		t.Fatal(addr, err)
	}
	for _, bad := range []*vrxv1.ManagementPrometheus{{Enabled: proto.Bool(true), Listen: proto.String("bad")}, {Enabled: proto.Bool(true), Port: proto.Uint32(0)}, {Enabled: proto.Bool(true), Port: proto.Uint32(65536)}, {Enabled: proto.Bool(true), Allow: []string{"bad"}}} {
		var invalid sink
		Prometheus(&invalid, &vrxv1.DesiredState{Management: &vrxv1.ManagementConfig{Prometheus: bad}})
		if len(invalid.errs) != 1 || len(invalid.kvs) != 0 {
			t.Fatal(invalid.errs, invalid.kvs)
		}
	}
	var disabled sink
	Prometheus(&disabled, &vrxv1.DesiredState{})
	if len(disabled.kvs) != 0 {
		t.Fatal(disabled.kvs)
	}
}
