package subsystems

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

type carrierTestRunner struct {
	lease    desc.CarrierLease
	request  map[string]json.RawMessage
	receipt  carrierBrokerReceipt
	calls    []string
	onStop   func()
	failHash bool
}

func (r *carrierTestRunner) Run(_ context.Context, c renderers.Command) (renderers.Output, error) {
	if c.Path == pppoe.SystemctlBin {
		r.calls = append(r.calls, strings.Join(c.Args, " "))
		if c.Args[0] == "stop" && strings.HasPrefix(c.Args[1], "ngfw-pppoe-carrier@") {
			if r.onStop != nil {
				r.onStop()
			}
		}
		return renderers.Output{}, nil
	}
	if c.Path != pppoe.Python3Bin || len(c.Args) < 5 {
		return renderers.Output{}, errors.New("unexpected command")
	}
	var value any
	switch c.Args[2] {
	case "broker-queue":
		if err := json.Unmarshal([]byte(c.Args[len(c.Args)-1]), &r.request); err != nil {
			return renderers.Output{}, err
		}
		r.receipt = carrierBrokerReceipt{Token: c.Args[3], Nonce: c.Args[4], Boot: "boot", Expires: 100, Hash: strings.Repeat("a", 64)}
		value = r.receipt
	case "broker-result":
		var op string
		_ = json.Unmarshal(r.request["op"], &op)
		var result any
		switch op {
		case "list":
			result = []desc.CarrierLease{r.lease}
		case "prepare", "provision":
			result = r.lease
		case "withdraw", "configure", "delete":
			result = nil
		default:
			return renderers.Output{}, errors.New("unexpected broker operation")
		}
		r.receipt.OK = true
		r.receipt.Result, _ = json.Marshal(result)
		if r.failHash {
			r.receipt.Hash = strings.Repeat("b", 64)
		}
		value = r.receipt
	default:
		return renderers.Output{}, errors.New("privileged helper called directly")
	}
	body, err := json.Marshal(value)
	return renderers.Output{Stdout: body}, err
}
func TestCarrierBrokerRejectsMismatchedReceipt(t *testing.T) {
	run := &carrierTestRunner{failHash: true}
	host := &pppoeCarrierHost{runner: run}
	if _, err := host.Inventory(t.Context(), "w9"); err == nil {
		t.Fatal("foreign result admitted")
	}
}
func TestCarrierRestartReadbackStopsBeforeSecretReplacement(t *testing.T) {
	rt, _, fake := newTestRuntime(t)
	rt.carrierMode = true
	rt.carrierRoot = t.TempDir()
	rt.carrierHooks = t.TempDir()
	for _, kind := range []string{"ip-up", "ip-down", "ipv6-up", "ipv6-down"} {
		if err := os.WriteFile(filepath.Join(rt.carrierHooks, kind), []byte("#!/usr/bin/python3\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	spec, err := pppoe.NewCarrierSpec(rt.owner, "pppwan", "wanraw", 1492)
	if err != nil {
		t.Fatal(err)
	}
	fake.AddInterface("wanraw", "")
	run := &carrierTestRunner{lease: desc.CarrierLease{Spec: spec, Token: spec.Token(), Generation: strings.Repeat("a", 32), Boot: "boot", Namespace: []uint64{1, 2}}}
	rt.runner = run
	manifest := filepath.Join(t.TempDir(), "applied.pb")
	fixture := "NGFW_TEST_PSK_F-pppoe-client-wiring"
	doc := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{"pppwan": {Pppoe: &ngfwv1.Pppoe{Parent: proto.String("wanraw"), Username: proto.String("test"), PasswordRef: proto.String("password/test"), Ipv6: proto.String("off")}}}}
	doc.Interfaces["wanraw"] = &ngfwv1.Interface{Enabled: proto.Bool(true)}
	descriptor := func(runtime *PppoeRuntime) *desc.ClientConfig {
		d := desc.NewClientConfig(runtime, runtime.renderer, manifest)
		d.SetCarrierOwner(runtime.owner)
		d.SetSecretSource(func(string) ([]byte, error) { return []byte(fixture), nil })
		return d
	}
	d := descriptor(rt)
	if _, err = d.Create(t.Context(), doc); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	rt.applied = map[string]pppoe.Session{}
	rt.mu.Unlock()
	// Use actual descriptor retrieval to hydrate a restarted process.
	d = descriptor(rt)
	if kvs, err := d.Retrieve(t.Context()); err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, doc) {
		t.Fatalf("retrieve failed: %v", err)
	}
	if rt.applied["pppwan"].Carrier == nil {
		t.Fatal("restart lost carrier identity")
	}
	secret := filepath.Join(rt.carrierRoot, spec.Token(), "ppp", "chap-secrets")
	stopped := false
	run.onStop = func() {
		body, err := os.ReadFile(secret) // #nosec G304 -- Fixed chap-secrets filename under this test TempDir and deterministic carrier token.
		if err != nil || !strings.Contains(string(body), fixture) {
			t.Error("credentials replaced before stop")
		}
		stopped = true
	}
	changed := proto.Clone(doc).(*ngfwv1.DesiredState)
	changed.Interfaces["pppwan"].Pppoe.Username = proto.String("changed")
	if _, err = d.Create(t.Context(), changed); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("recovered unit was not stopped")
	}
	// Drift must retain enough applied identity for later Delete/rollback.
	if err = os.WriteFile(secret, []byte("drift"), 0600); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	rt.applied = map[string]pppoe.Session{}
	rt.mu.Unlock()
	if _, err = d.Retrieve(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rt.applied["pppwan"].Carrier == nil {
		t.Fatal("drift discarded restart teardown evidence")
	}
}

func TestCarrierNegotiatedAddressReadbackRejectsStaleHook(t *testing.T) {
	m := desc.Mirror{LocalIPv4: "192.0.2.10/32", LocalIPv6: []string{"2001:db8::1/128"}}
	if err := verifyNegotiatedCarrierAddresses(m, []string{"192.0.2.10/32", "2001:db8::1/64"}); err != nil {
		t.Fatal(err)
	}
	if err := verifyNegotiatedCarrierAddresses(m, []string{"192.0.2.11/32", "2001:db8::1/64"}); err == nil {
		t.Fatal("stale IPv4 hook admitted")
	}
	if err := verifyNegotiatedCarrierAddresses(m, []string{"192.0.2.10/32"}); err == nil {
		t.Fatal("stale IPv6 hook admitted")
	}
}
func TestCarrierNCPEpochChangesWithinPersistentProcess(t *testing.T) {
	rt, _, _ := newTestRuntime(t)
	spec, _ := pppoe.NewCarrierSpec(rt.owner, "pppwan", "wanraw", 1492)
	s := pppoe.Session{Carrier: &spec, HostIf: spec.RawHost()}
	dir := rt.sessionStateDir(s)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, s.HostIf+".state")
	write := func(generation string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("phase=up\nsession_generation="+generation+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(strings.Repeat("a", 32))
	first, err := rt.carrierSessionGeneration(s)
	if err != nil {
		t.Fatal(err)
	}
	write(strings.Repeat("b", 32))
	second, err := rt.carrierSessionGeneration(s)
	if err != nil || second == first {
		t.Fatalf("redial epoch did not change: %v", err)
	}
	write("")
	if _, err = rt.carrierSessionGeneration(s); err == nil {
		t.Fatal("missing generation accepted")
	}
}

func TestCarrierRegistrationKeepsRATapOwnershipSeparate(t *testing.T) {
	rt, _, fake := newTestRuntime(t)
	reg := scheduler.NewRegistry()
	original := tapv2.New(fake, rt.owner)
	reg.Register(original)
	w := &Wiring{env: Env{Client: fake, Owner: rt.owner, GlobalsOwner: true}}
	if err := w.registerPppoeCarrier(reg, rt); err != nil {
		t.Fatal(err)
	}
	if got, _ := reg.Get(tapv2.TapName); got != original {
		t.Fatal("RA TAP descriptor replaced")
	}
	if _, ok := reg.Get(desc.CarrierTapName); !ok {
		t.Fatal("carrier TAP descriptor not registered")
	}
	if _, ok := reg.Get(desc.CarrierNamespaceName); !ok {
		t.Fatal("carrier namespace not registered")
	}
	for _, name := range []string{desc.CarrierTapName, desc.CarrierNamespaceName} {
		found := false
		for _, member := range Domains[Interfaces] {
			found = found || member == name
		}
		if !found {
			t.Fatalf("missing interfaces domain %s", name)
		}
	}
}

func TestCarrierProductFilesFitInstalledAgentWritableScope(t *testing.T) {
	unit, err := os.ReadFile("../../../../deploy/systemd/ngfw-agent.service")
	if err != nil {
		t.Fatal(err)
	}
	rt := &PppoeRuntime{}
	root := rt.carrierRootDir()
	allowed := false
	for _, line := range strings.Split(string(unit), "\n") {
		if !strings.HasPrefix(line, "ReadWritePaths=") {
			continue
		}
		for _, prefix := range strings.Fields(strings.TrimPrefix(line, "ReadWritePaths=")) {
			prefix = strings.TrimPrefix(prefix, "-")
			if root == prefix || strings.HasPrefix(root, prefix+"/") {
				allowed = true
			}
		}
	}
	if !allowed {
		t.Fatalf("carrier configuration root %s is outside packaged agent writable paths", root)
	}
}

func TestCarrierResolverPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	if err := prepareCarrierResolver(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("nameserver 192.0.2.53\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareCarrierResolver(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != 0 || info.Mode().Perm() != 0600 {
		t.Fatalf("resolver was not reset privately: %v %v", info, err)
	}
	target := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := prepareCarrierResolver(dir); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, path); err != nil {
		t.Fatal(err)
	}
	if err := prepareCarrierResolver(dir); err == nil {
		t.Fatal("shared inode accepted")
	}
	body, err := os.ReadFile(target) // #nosec G304 -- Fixed target fixture under this test TempDir; verifies unsafe alias was not truncated.
	if err != nil || string(body) != "preserve" {
		t.Fatalf("unrelated file changed: %q %v", body, err)
	}
}

func TestCarrierWANMembershipWithdrawsBeforeRestartAndRollbackRestoresPolicy(t *testing.T) {
	rt, _, fake := newTestRuntime(t)
	rt.carrierMode, rt.carrierRoot, rt.carrierHooks = true, t.TempDir(), t.TempDir()
	for _, kind := range []string{"ip-up", "ip-down", "ipv6-up", "ipv6-down"} {
		if err := os.WriteFile(filepath.Join(rt.carrierHooks, kind), []byte("#!/usr/bin/python3\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	spec, _ := pppoe.NewCarrierSpec(rt.owner, "pppwan", "wanraw", 1492)
	fake.AddInterface("wanraw", "")
	fake.AddInterface("pppwan", "")
	fake.Reply("sw_interface_get_table", &interfaces.SwInterfaceGetTableReply{VrfID: 9000})
	withdrawn := false
	fake.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
		write := req.(*ip.IPRouteAddDel)
		if write.IsAdd || !write.IsMultipath {
			t.Fatal("membership transition changed whole route or added old default")
		}
		withdrawn = true
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	run := &carrierTestRunner{lease: namespaceLeaseForTest(spec)}
	rt.runner = run
	d := desc.NewClientConfig(rt, rt.renderer, filepath.Join(t.TempDir(), "applied.pb"))
	d.SetCarrierOwner(rt.owner)
	d.SetSecretSource(func(string) ([]byte, error) { return []byte("NGFW_TEST_PSK_F-pppoe-client-wiring"), nil })
	doc := &ngfwv1.DesiredState{Interfaces: map[string]*ngfwv1.Interface{
		"wanraw": {Enabled: proto.Bool(true)},
		"pppwan": {Pppoe: &ngfwv1.Pppoe{Parent: proto.String("wanraw"), Username: proto.String("test"), PasswordRef: proto.String("password/test"), Ipv6: proto.String("off"), MssClamp: proto.Bool(false)}},
	}}
	if _, err := d.Create(t.Context(), doc); err != nil {
		t.Fatal(err)
	}
	rt.mirrored = map[string]desc.Mirror{"pppwan": {Interface: "pppwan", LocalIPv4: "192.0.2.7/32", PeerIPv4: strings.Split(spec.Host4, "/")[0], DefaultRoute: true}}
	rt.carrierReady["pppwan"] = carrierForwarding{epoch: "old", until: time.Now().Add(time.Minute)}
	run.onStop = func() {
		if !withdrawn || len(rt.mirrored) != 0 || len(rt.carrierReady) != 0 {
			t.Error("unit stopped before automatic default/readiness withdrawal")
		}
	}
	joined := proto.Clone(doc).(*ngfwv1.DesiredState)
	joined.Routing = &ngfwv1.RoutingConfig{WanGroups: []*ngfwv1.WanGroup{{Name: proto.String("wan"), Members: []*ngfwv1.WanMember{{Interface: proto.String("pppwan")}}}}}
	if _, err := d.Create(t.Context(), joined); err != nil {
		t.Fatal(err)
	}
	if rt.applied["pppwan"].DefaultRoute {
		t.Fatal("WAN member retained automatic default")
	}
	// Returning to the previous manifest represents leave or transaction rollback.
	if _, err := d.Create(t.Context(), doc); err != nil {
		t.Fatal(err)
	}
	if !rt.applied["pppwan"].DefaultRoute {
		t.Fatal("rollback did not restore standalone default policy")
	}
}

func namespaceLeaseForTest(spec pppoe.CarrierSpec) desc.CarrierLease {
	return desc.CarrierLease{Spec: spec, Token: spec.Token(), Generation: strings.Repeat("a", 32), Boot: "boot", Namespace: []uint64{1, 2}}
}
