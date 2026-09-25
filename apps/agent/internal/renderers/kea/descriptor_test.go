package kea

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// keaModel is a stateful fake daemon pair: config-set stores the configuration, config-get returns it with a few
// defaults Kea adds (and the hash), status-get / statistic-get-all / lease4-get-page answer from the model.
type keaModel struct {
	running map[int]bool
	config  map[int]map[string]any
	stats   map[int]map[string]any
	leases  map[int][]map[string]any
	calls   []string
}

func newModel(running ...int) *keaModel {
	m := &keaModel{running: map[int]bool{}, config: map[int]map[string]any{}, stats: map[int]map[string]any{}, leases: map[int][]map[string]any{}}
	for _, f := range running {
		m.running[f] = true
	}
	return m
}

func (m *keaModel) Command(_ context.Context, fam int, cmd string, args any) (Response, error) {
	m.calls = append(m.calls, fmt.Sprintf("%s/%d", cmd, fam))
	if !m.running[fam] {
		return Response{}, ErrNotRunning
	}
	ok := func(v any) (Response, error) {
		b, _ := json.Marshal(v)
		return Response{Result: ResultSuccess, Arguments: b}, nil
	}
	switch cmd {
	case "config-set":
		var cfg map[string]any
		if err := json.Unmarshal(args.(json.RawMessage), &cfg); err != nil {
			return Response{Result: 1}, err
		}
		for _, v := range cfg { // Kea adds defaults the renderer never sets
			if o, ok := v.(map[string]any); ok {
				o["valid-lifetime"] = 7200
				o["decline-probation-period"] = 86400
			}
		}
		m.config[fam] = cfg
		return Response{Result: ResultSuccess}, nil
	case "config-get":
		out := map[string]any{"hash": "abc"}
		for k, v := range m.config[fam] {
			out[k] = v
		}
		return ok(out)
	case "status-get":
		return ok(map[string]any{"pid": 42, "uptime": 100, "reload": 7})
	case "statistic-get-all":
		return ok(m.stats[fam])
	case "lease4-get-page", "lease6-get-page":
		a := args.(map[string]any)
		from, limit := a["from"].(string), a["limit"].(int)
		var page []map[string]any
		started := from == "start"
		for _, l := range m.leases[fam] {
			if started {
				page = append(page, l)
				if len(page) == limit {
					break
				}
			}
			if l["ip-address"] == from {
				started = true
			}
		}
		if len(page) == 0 {
			return Response{Result: ResultEmpty}, nil
		}
		return ok(map[string]any{"leases": page, "count": len(page)})
	}
	return Response{Result: ResultUnsupported, Text: "unknown"}, fmt.Errorf("%w: %s", ErrCommand, cmd)
}

func descRenderer(t *testing.T, m *keaModel, opts ...Option) (*Renderer, Paths) {
	t.Helper()
	p := tmpPaths(t)
	base := []Option{WithPaths(p), WithController(m), WithLeaseCmdsHook(""), WithInterfaceMapper(IdentityMapper)}
	run := renderers.NewRecordingRunner().Succeed(Dhcp4Bin, "").Succeed(Dhcp6Bin, "")
	return New(run, append(base, opts...)...), p
}

func doc(servers map[string]*vrxv1.DhcpServer) *vrxv1.DesiredState {
	return &vrxv1.DesiredState{
		Interfaces: map[string]*vrxv1.Interface{"w0-a": {Ipv4: []string{"10.6.10.1/24"}}, "other": {Ipv4: []string{"192.0.2.1/24"}}},
		Services:   &vrxv1.ServicesConfig{Dhcp: &vrxv1.DhcpService{Servers: servers}},
	}
}

func TestInput(t *testing.T) {
	ds := doc(map[string]*vrxv1.DhcpServer{"lan": v4Server(), "lan6": v6Server(), "off": {Enabled: proto.Bool(false), Interfaces: []string{"other"}}})
	in4 := Input(ds, 4)
	if got := ServerKeys(in4); got != "lan,off" {
		t.Fatalf("v4 servers %s", got)
	}
	if len(in4.GetInterfaces()) != 2 || in4.GetInterfaces()["w0-a"].GetIpv4()[0] != "10.6.10.1/24" {
		t.Fatalf("v4 bindings %v", in4.GetInterfaces())
	}
	in6 := Input(ds, 6)
	if got := ServerKeys(in6); got != "lan6" || in6.GetInterfaces() != nil {
		t.Fatalf("v6 input %v", in6)
	}
	if Input(doc(nil), 4) != nil {
		t.Fatal("no servers → nil input")
	}
	if !proto.Equal(Input(in4, 4), in4) {
		t.Fatal("Input is not idempotent")
	}
	// the rendered file carries the input; EmbeddedInput reads it back exactly
	r := newUnit()
	files, err := r.RenderFamily(in4, 4)
	if err != nil {
		t.Fatal(err)
	}
	back, ok, err := EmbeddedInput(files[r.paths.Dhcp4Conf()].Content)
	if err != nil || !ok || !proto.Equal(back, in4) {
		t.Fatalf("embedded input: ok=%v err=%v", ok, err)
	}
	idle, _ := r.RenderFamily(nil, 4)
	if _, ok, err := EmbeddedInput(idle[r.paths.Dhcp4Conf()].Content); ok || err != nil {
		t.Fatalf("idle config has no input: ok=%v err=%v", ok, err)
	}
	if _, _, err := EmbeddedInput([]byte(`{"Dhcp4": {"user-context": {"vrx": {"input": "!!"}}}}`)); err == nil {
		t.Fatal("bad base64 must be an error")
	}
	if _, err := r.RenderFamily(in4, 5); err == nil {
		t.Fatal("family 5")
	}
}

// ServerKeys is a test helper: the sorted server names of an input, comma-joined.
func ServerKeys(in *vrxv1.DesiredState) string {
	return strings.Join(sortedKeys(in.GetServices().GetDhcp().GetServers()), ",")
}

func TestDescriptorLifecycle(t *testing.T) {
	ctx := context.Background()
	m := newModel(4, 6)
	r, p := descRenderer(t, m)
	d4 := NewDescriptor(r, 4)
	if d4.Name() != NameDhcp4 || d4.KeyOf(nil) != "kea.dhcp4/vrx" || Key(6) != "kea.dhcp6/vrx" || NewDescriptor(r, 6).Name() != NameDhcp6 {
		t.Fatal("names/keys")
	}
	in := Input(doc(map[string]*vrxv1.DhcpServer{"lan": v4Server()}), 4)
	deps := d4.Dependencies(in)
	if len(deps) != 1 || deps[0].Key != "interface/w0-a" || !deps[0].Optional {
		t.Fatalf("deps %v", deps)
	}

	// nothing configured yet: no object
	if kvs, err := d4.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("empty: %v %v", kvs, err)
	}
	if _, err := d4.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	kvs, err := d4.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, in) {
		t.Fatalf("after create: %v %v", kvs, err)
	}

	// drift: Kea runs something else in the same (ours) configuration → a Struct value, never equal
	m.config[4]["Dhcp4"].(map[string]any)["subnet4"].([]any)[0].(map[string]any)["pools"] = []any{map[string]any{"pool": "10.6.10.100-10.6.10.101"}}
	kvs, _ = d4.Retrieve(ctx)
	sv, ok := kvs[0].Value.(*structpb.Struct)
	if !ok || !strings.Contains(sv.String(), "pool") {
		t.Fatalf("drift value %v", kvs[0].Value)
	}
	if _, err := d4.Update(ctx, sv, in, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d4.Retrieve(ctx); !proto.Equal(kvs[0].Value, in) {
		t.Fatal("update did not converge")
	}

	// not running: the file on disk is what the daemon loads; still reported (no start loop, no txn failure)
	m.running[4] = false
	kvs, err = d4.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, in) {
		t.Fatalf("not running: %v %v", kvs, err)
	}
	in2 := Input(doc(map[string]*vrxv1.DhcpServer{"lan": func() *vrxv1.DhcpServer { s := v4Server(); s.LeaseTimeSec = proto.Uint32(9000); return s }()}), 4)
	if _, err := d4.Update(ctx, in, in2, nil); err != nil {
		t.Fatalf("an inactive daemon is a start request, not an error: %v", err)
	}
	st := r.Status(ctx, 4)
	if st.Running || !st.Active || st.ActionRequired != "start" || len(st.Subnets) != 2 {
		t.Fatalf("status while stopped: %+v", st)
	}
	if kvs, _ = d4.Retrieve(ctx); !proto.Equal(kvs[0].Value, in2) {
		t.Fatal("file on disk not reported")
	}
	m.running[4] = true

	// delete → idle configuration → no object
	if err := d4.Delete(ctx, in2, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d4.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete: %v", kvs)
	}
	b, _ := os.ReadFile(p.Dhcp4Conf())
	if active(b) {
		t.Fatal("delete must leave the idle configuration")
	}

	// a foreign configuration (no embedded input) is never reported
	m.config[4] = map[string]any{"Dhcp4": map[string]any{"interfaces-config": map[string]any{"interfaces": []any{"eth9"}}}}
	if kvs, _ = d4.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("foreign config reported: %v", kvs)
	}
	// a commented (packaged) configuration file is foreign too, whether running or on disk
	m.running[4] = false
	if err := os.WriteFile(p.Dhcp4Conf(), []byte("// stock\n{\"Dhcp4\": {}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if kvs, err := d4.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("commented file reported: %v %v", kvs, err)
	}
	m.running[4] = true
	// wrong value type
	if _, err := d4.Create(ctx, &structpb.Struct{}); err == nil {
		t.Fatal("Create with a non-input value must fail")
	}
}

func TestStatusAndLeasePage(t *testing.T) {
	ctx := context.Background()
	m := newModel(4)
	r, _ := descRenderer(t, m)
	in := Input(doc(map[string]*vrxv1.DhcpServer{"lan": v4Server()}), 4)
	if err := NewDescriptor(r, 4).apply(ctx, in); err != nil {
		t.Fatal(err)
	}
	refs, _ := SubnetsOf(mustConfig(t, r, 4))
	if len(refs) != 2 {
		t.Fatalf("subnets %v", refs)
	}
	var lanID uint32
	for _, s := range refs {
		if s.Subnet == "lan" {
			lanID = s.ID
		}
	}
	m.stats[4] = map[string]any{
		fmt.Sprintf("subnet[%d].total-addresses", lanID):    [][]any{{100, "t"}},
		fmt.Sprintf("subnet[%d].assigned-addresses", lanID): [][]any{{3, "t"}},
		fmt.Sprintf("subnet[%d].declined-addresses", lanID): [][]any{{1, "t"}},
	}
	st := r.Status(ctx, 4)
	if !st.Running || !st.Active || st.ActionRequired != "" || st.ReloadSec != 7 || st.Err != "" {
		t.Fatalf("status %+v", st)
	}
	var lan SubnetUsage
	for _, s := range st.Subnets {
		if s.Subnet == "lan" {
			lan = s
		}
	}
	if lan.Server != "lan" || lan.Total != 100 || lan.Assigned != 3 || lan.Declined != 1 || lan.Prefix != "10.6.10.0/24" {
		t.Fatalf("usage %+v", lan)
	}
	if s6 := r.Status(ctx, 6); s6.Running || s6.Active || s6.ActionRequired != "" {
		t.Fatalf("v6 without a file: %+v", s6)
	}

	for i := 0; i < 2500; i++ {
		m.leases[4] = append(m.leases[4], map[string]any{
			"ip-address": fmt.Sprintf("10.6.%d.%d", 10+i/250, i%250), "hw-address": fmt.Sprintf("AA:00:00:00:%02x:%02x", i/256, i%256),
			"hostname": fmt.Sprintf("host%d", i), "subnet-id": lanID, "valid-lft": 3600, "cltt": 1000, "state": 0,
		})
	}
	page, total, truncated, err := r.LeasePage(ctx, LeaseQuery{Offset: 100, Limit: 50})
	if err != nil || total != 2500 || truncated || len(page) != 50 || page[0].Address != "10.6.10.100" || page[0].HWAddress != "aa:00:00:00:00:64" {
		t.Fatalf("page: total=%d truncated=%v n=%d first=%+v err=%v", total, truncated, len(page), page, err)
	}
	if page[0].Expires() != 4600 || page[0].StateName() != "default" {
		t.Fatalf("lease fields %+v", page[0])
	}
	page, total, _, _ = r.LeasePage(ctx, LeaseQuery{Filter: "HOST2499", Limit: 10})
	if total != 1 || page[0].Hostname != "host2499" {
		t.Fatalf("filter: %d %v", total, page)
	}
	page, total, _, _ = r.LeasePage(ctx, LeaseQuery{SubnetIDs: map[int]map[uint32]bool{4: {lanID + 1: true}}})
	if total != 0 || len(page) != 0 {
		t.Fatalf("other subnet: %d", total)
	}
	if _, total, _, _ = r.LeasePage(ctx, LeaseQuery{Families: []int{6}}); total != 0 {
		t.Fatal("v6 not running contributes nothing")
	}
	if statUint("340282366920938463463374607431768211456") == 0 || statUint("x") != 0 || statUint("") != 0 {
		t.Fatal("statUint")
	}
	if (Lease{State: 1}).StateName() != "declined" || (Lease{State: 2}).StateName() != "expired-reclaimed" || (Lease{State: 9}).StateName() != "state-9" {
		t.Fatal("state names")
	}
}

func mustConfig(t *testing.T, r *Renderer, fam int) []byte {
	t.Helper()
	b, _, err := r.actualConfig(context.Background(), fam)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestAddressBindingOff: WithAddressBinding(false) keeps plain interface names (the lab relay rig).
func TestAddressBindingOff(t *testing.T) {
	ds := doc(map[string]*vrxv1.DhcpServer{"lan": v4Server()})
	on, _ := newUnit().RenderFamily(Input(ds, 4), 4)
	off, _ := newUnit(WithAddressBinding(false)).RenderFamily(Input(ds, 4), 4)
	p := unitPaths().Dhcp4Conf()
	if !strings.Contains(string(on[p].Content), `"w0-a/10.6.10.1"`) || strings.Contains(string(off[p].Content), `"w0-a/10.6.10.1"`) {
		t.Fatal("address binding option")
	}
}

func TestOwnershipDeclaration(t *testing.T) {
	var d any = NewDescriptor(newUnit(), 4)
	if _, ok := d.(interface{ RecordsNoOwnership() }); !ok {
		t.Fatal("kea descriptor declares no ownership mode (TD-11b guard)")
	}
}
