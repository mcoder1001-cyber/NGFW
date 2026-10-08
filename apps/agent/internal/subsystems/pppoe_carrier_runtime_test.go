package subsystems

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
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
		if err := os.WriteFile(filepath.Join(rt.carrierHooks, kind), []byte("#!/usr/bin/python3\n"), 0755); err != nil {
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
		body, err := os.ReadFile(secret)
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
