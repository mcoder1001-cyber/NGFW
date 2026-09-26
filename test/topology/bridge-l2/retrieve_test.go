package bridgel2

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// retrieveL2 asks the slot's agent for Retrieve(interfaces, routing) over its unix socket and returns the F-bridge-l2 half
// of the document it reports (routing.l2 and every interfaces.<if>[.subinterfaces.<id>].l2 leaf) as generic JSON. The
// API's /state/drift ignores the whole /routing domain while routing protocols are unimplemented
// (agent.unsupported-field), so the Retrieve == desired evidence for routing.l2 is taken here directly.
func retrieveL2(t *testing.T, socket string) (routingL2 any, ports map[string]any) {
	t.Helper()
	cc, err := grpc.NewClient("unix:"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := vrxv1.NewDataplaneClient(cc).Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "routing"}})
	if err != nil {
		t.Fatalf("agent Retrieve: %v", err)
	}
	raw, err := protojson.Marshal(r.GetDesiredState())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return l2Half(doc)
}

// l2Half extracts routing.l2 and the l2 leaves by logical interface name from a document (generic JSON).
func l2Half(doc map[string]any) (any, map[string]any) {
	routing, _ := doc["routing"].(map[string]any)
	ports := map[string]any{}
	ifs, _ := doc["interfaces"].(map[string]any)
	for name, v := range ifs {
		itf, _ := v.(map[string]any)
		if l2, ok := itf["l2"]; ok {
			ports[name] = l2
		}
		subs, _ := itf["subinterfaces"].(map[string]any)
		for id, sv := range subs {
			sub, _ := sv.(map[string]any)
			if l2, ok := sub["l2"]; ok {
				ports[name+"."+id] = l2
			}
		}
	}
	return routing["l2"], ports
}

// compareL2 fails when the retrieved L2 half differs from the running configuration's.
func compareL2(t *testing.T, a *api, socket string) {
	t.Helper()
	run := a.must(200, "GET", "/api/v1/config", nil)
	var running map[string]any
	if err := json.Unmarshal([]byte(run.raw), &running); err != nil {
		t.Fatal(err)
	}
	wantL2, wantPorts := l2Half(running)
	gotL2, gotPorts := retrieveL2(t, socket)
	t.Logf("Retrieve routing.l2 = %s", js(gotL2))
	var names []string
	for n := range gotPorts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t.Logf("Retrieve interfaces %s l2 = %s", n, js(gotPorts[n]))
	}
	if !reflect.DeepEqual(norm(gotL2), norm(wantL2)) {
		t.Errorf("Retrieve routing.l2 ≠ running:\n got  %s\n want %s", js(gotL2), js(wantL2))
	}
	if !reflect.DeepEqual(norm(gotPorts), norm(wantPorts)) {
		t.Errorf("Retrieve l2 leaves ≠ running:\n got  %s\n want %s", js(gotPorts), js(wantPorts))
	}
	if !t.Failed() {
		t.Logf("Retrieve == desired for routing.l2 and the l2 leaves of %s", strings.Join(names, ", "))
	}
}

// norm drops empty maps/lists (proto3 does not carry an empty repeated/map; Zod fills them) for the comparison.
func norm(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			n := norm(e)
			if m, ok := n.(map[string]any); ok && len(m) == 0 {
				continue
			}
			if l, ok := n.([]any); ok && len(l) == 0 {
				continue
			}
			out[k] = n
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = norm(e)
		}
		return out
	}
	return v
}
