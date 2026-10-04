package subsystems

import (
	"bytes"
	"errors"
	"log/slog"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/desired"
	syncpim "ngfw/agent/internal/frrsync/pim"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/scheduler"
	"strings"
	"testing"
)

func TestPimRegistrationOwnership(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scope   IDScope
		enabled bool
		want    int
	}{{"global", IDScope{All: true}, true, 1}, {"slot", IDScope{Range: &IDRange{Lo: 15000, Hi: 15999}}, true, 0}, {"none", IDScope{}, true, 0}, {"off", IDScope{All: true}, false, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newFRRAt(Env{Owner: "pim-test"}, renderers.NewRecordingRunner(), frr.TestPaths("w15"), tc.enabled)
			frrRuntimes.Store("pim-test", rt)
			t.Cleanup(func() { frrRuntimes.Delete("pim-test"); rt.Close() })
			w := &Wiring{env: Env{Owner: "pim-test", IDs: tc.scope}}
			reg := scheduler.NewRegistry()
			if e := registerPim(reg, w); e != nil {
				t.Fatal(e)
			}
			if len(w.DynamicSources()) != tc.want {
				t.Fatalf("sources=%d", len(w.DynamicSources()))
			}
			if tc.want == 1 && w.DynamicSources()[0].Descriptors[0] != syncpim.Descriptor {
				t.Fatal("source must own exclusive scope")
			}
		})
	}
}
func TestPimFRRProjection(t *testing.T) {
	doc := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Multicast: &ngfwv1.MulticastConfig{Pim: &ngfwv1.PimConfig{Interfaces: []string{"wan"}}}}}
	got := desired.FRRDoc(doc, nil)
	if got.GetRouting().GetMulticast().GetPim() == nil {
		t.Fatal("PIM omitted from daemon projection")
	}
	got.Routing.Multicast.Pim.Interfaces[0] = "changed"
	if doc.Routing.Multicast.Pim.Interfaces[0] != "wan" {
		t.Fatal("projection aliased source")
	}
}

func TestPimOperatorTransitions(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	var events []*ngfwv1.Event
	report := pimStatusReporter(logger, func(ev *ngfwv1.Event) { events = append(events, ev) })
	report(nil)
	secretError := errors.New("sensitive daemon payload must never be emitted")
	report(secretError)
	report(secretError)
	report(errors.New("another failure same outage"))
	if len(events) != 1 || events[0].Kind != ngfwv1.EventKind_EVENT_KIND_ERROR || events[0].Attributes["status"] != "degraded" || events[0].Message == "" {
		t.Fatalf("failure reporting: %v", events)
	}
	if strings.Count(logs.String(), "level=WARN") != 1 {
		t.Fatal("repeated failure log spam", logs.String())
	}
	report(nil)
	report(nil)
	if len(events) != 2 || events[1].Attributes["status"] != "recovered" || events[1].Message != "PIM synchronization recovered" {
		t.Fatal("recovery not published", events)
	}
	report(secretError)
	if len(events) != 3 || strings.Count(logs.String(), "level=WARN") != 2 {
		t.Fatal("failure after recovery hidden")
	}
	if strings.Contains(logs.String(), "sensitive") {
		t.Fatal("error payload leaked to log")
	}
	for _, ev := range events {
		for _, value := range ev.Attributes {
			if strings.Contains(value, "sensitive") {
				t.Fatal("error payload leaked to event")
			}
		}
	}
}
