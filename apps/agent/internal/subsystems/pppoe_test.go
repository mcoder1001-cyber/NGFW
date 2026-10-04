package subsystems

import (
	"context"
	"errors"
	"go.fd.io/govpp/api"
	"log/slog"
	"strings"
	"testing"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	mssclamp "ngfw/agent/binapi/mss_clamp"
	"ngfw/agent/internal/descriptors/df6/df6test"
	pppoedesc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
)

// newTestRuntime builds a globals-owner runtime (real-runner stand-in = RecordingRunner) with an allow-all route
// policy, so Reconnect/Apply/Mirror exercise the happy path.
func newTestRuntime(t *testing.T) (*PppoeRuntime, *renderers.RecordingRunner, *df6test.FakeVPP) {
	t.Helper()
	paths := pppoe.PathsUnder(t.TempDir())
	ren := pppoe.New(pppoe.WithPaths(paths))
	run := renderers.NewRecordingRunner().Succeed(pppoe.SystemctlBin, "")
	f := df6test.NewFakeVPP()
	f.On("ip_address_dump", func(api.Message) ([]api.Message, error) { return nil, nil })
	rt := &PppoeRuntime{
		renderer: ren, stateDir: paths.StateDir, runner: run, vpp: f, owner: "w9", globalsOwner: true,
		allowRoute: func(uint32) bool { return true }, log: slog.Default(), applied: map[string]pppoe.Session{},
	}
	return rt, run, f
}

func TestPppoeRuntimeReconnect(t *testing.T) {
	rt, run, _ := newTestRuntime(t)
	sess := pppoe.Session{Iface: "wan0", HostIf: "wan0", Username: "u", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492} //nolint:gosec // required non-production fixture marker
	if err := rt.Apply(context.Background(), []pppoe.Session{sess}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	accepted, msg, err := rt.Reconnect(context.Background(), "wan0")
	if err != nil || !accepted {
		t.Fatalf("Reconnect wan0: accepted=%v msg=%q err=%v", accepted, msg, err)
	}
	var restarted bool
	for _, c := range run.Calls() {
		if c.Path == pppoe.SystemctlBin && len(c.Args) == 2 && c.Args[0] == "restart" && c.Args[1] == "ngfw-pppoe-wan0.service" {
			restarted = true
		}
	}
	if !restarted {
		t.Fatalf("expected systemctl restart ngfw-pppoe-wan0.service, calls=%v", run.Calls())
	}
}

func TestPppoeRuntimeReconnectUnknownInterface(t *testing.T) {
	rt, _, _ := newTestRuntime(t)
	accepted, msg, err := rt.Reconnect(context.Background(), "wan9")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if accepted || !strings.Contains(msg, "no active PPPoE session") {
		t.Fatalf("unknown interface should be not-accepted with a clear message, got accepted=%v msg=%q", accepted, msg)
	}
}

func TestPppoeRuntimeMirror(t *testing.T) {
	rt, _, f := newTestRuntime(t)
	f.Reply("sw_interface_get_table", &interfaces.SwInterfaceGetTableReply{})
	f.Reply("ip_route_add_del", &ip.IPRouteAddDelReply{})
	f.Reply("mss_clamp_enable_disable", &mssclamp.MssClampEnableDisableReply{})
	f.AddInterface("wan0", "")
	mir := pppoedesc.Mirror{Interface: "wan0", LocalIPv4: "198.51.100.5/32", PeerIPv4: "198.51.100.1", DefaultRoute: true, MSSClamp: true, MTU: 1492}
	if err := rt.Mirror(context.Background(), mir, true); err != nil {
		t.Fatalf("Mirror up: %v", err)
	}
	if err := rt.Mirror(context.Background(), mir, false); err != nil {
		t.Fatalf("Mirror down: %v", err)
	}
}

func TestPppoeRuntimeStateDownWhenNotApplied(t *testing.T) {
	rt, _, _ := newTestRuntime(t)
	st, err := rt.State("wan0", 0, "")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.GetPhase() != "down" {
		t.Fatalf("phase = %q, want down", st.GetPhase())
	}
}

func TestRegisterPppoeStoresRuntime(t *testing.T) {
	f := df6test.NewFakeVPP()
	w := &Wiring{env: Env{Client: f, Owner: "w9-regtest", Log: slog.Default(), GlobalsOwner: true}}
	w.registerPppoe()
	if PppoeOf("w9-regtest") == nil {
		t.Fatal("registerPppoe did not record the runtime for PppoeOf")
	}
}

// A non-globals-owner (slot) agent must never drive the host's systemd: it gets a refusing runner, Reconnect is
// refused, and Apply is refused by the runner (no systemctl exec).
func TestRegisterPppoeSlotRefusesSystemctl(t *testing.T) {
	t.Setenv(EnvHostServicesDir, t.TempDir()) // slot renderer writes here, not /run/ngfw-test
	f := df6test.NewFakeVPP()
	w := &Wiring{env: Env{Client: f, Owner: "w9-slot", Log: slog.Default()}} // GlobalsOwner=false
	w.registerPppoe()
	rt := PppoeOf("w9-slot")
	if rt == nil || rt.globalsOwner {
		t.Fatalf("slot runtime expected, got %+v", rt)
	}
	if _, _, err := rt.Reconnect(context.Background(), "wan0"); !errors.Is(err, ErrNotGlobalsOwner) {
		t.Fatalf("slot Reconnect must return ErrNotGlobalsOwner, got %v", err)
	}
	err := rt.Apply(context.Background(), []pppoe.Session{{Iface: "wan0", HostIf: "wan0", Username: "u", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492}}) //nolint:gosec // required non-production fixture marker
	if err != nil {
		t.Fatalf("slot Apply must render without executing systemctl: %v", err)
	}
}

// pppoeRoutePolicy: globals owner → any table; slot → only inside its id range; no id range → fail closed;
// NGFW_VPP_ID_RANGE=all → any table.
func TestPppoeRoutePolicy(t *testing.T) {
	slot := IDScope{Range: &IDRange{Lo: 9000, Hi: 9999}}
	for _, c := range []struct {
		name    string
		env     Env
		table   uint32
		allowed bool
	}{
		{"globals owner, table 0", Env{GlobalsOwner: true}, 0, true},
		{"globals owner, foreign table", Env{GlobalsOwner: true}, 500, true},
		{"slot, table in range (low edge)", Env{IDs: slot}, 9000, true},
		{"slot, table in range (high edge)", Env{IDs: slot}, 9999, true},
		{"slot, table 0 (out of range)", Env{IDs: slot}, 0, false},
		{"slot, table above range", Env{IDs: slot}, 10000, false},
		{"no id range (ErrNoIDRange): fail closed", Env{}, 9000, false},
		{"id range all", Env{IDs: IDScope{All: true}}, 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := &Wiring{env: c.env}
			if got := w.pppoeRoutePolicy()(c.table); got != c.allowed {
				t.Fatalf("table %d: allowed=%v, want %v", c.table, got, c.allowed)
			}
		})
	}
}
