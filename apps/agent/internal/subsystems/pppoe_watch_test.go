package subsystems

import (
	"context"
	"errors"
	"go.fd.io/govpp/api"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	mss "ngfw/agent/binapi/mss_clamp"
	"ngfw/agent/internal/renderers/pppoe"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPppoeRenderedHookUpDownWithdrawAndRepair(t *testing.T) {
	rt, runner, v := newTestRuntime(t)
	rt.globalsOwner = false
	// Renderer paths come from its test root; slot helpers must use the same root.
	root := t.TempDir()
	t.Setenv(EnvHostServicesDir, root)
	paths := pppoe.PathsUnder(filepath.Join(root, "pppoe"))
	rt.renderer = pppoe.New(pppoe.WithPaths(paths))
	rt.stateDir = paths.StateDir
	v.AddInterface("wan0", "")
	v.Reply("sw_interface_get_table", &interfaces.SwInterfaceGetTableReply{VrfID: 9000})
	v.Reply("mss_clamp_enable_disable", &mss.MssClampEnableDisableReply{})
	present := false
	writes := 0
	routes := 0
	v.On("ip_address_dump", func(api.Message) ([]api.Message, error) {
		if !present {
			return nil, nil
		}
		prefix, _ := ip_types.ParseAddressWithPrefix("198.51.100.5/32")
		return []api.Message{&ip.IPAddressDetails{Prefix: prefix}}, nil
	})
	v.On("sw_interface_add_del_address", func(req api.Message) ([]api.Message, error) {
		present = req.(*interfaces.SwInterfaceAddDelAddress).IsAdd
		writes++
		return []api.Message{&interfaces.SwInterfaceAddDelAddressReply{}}, nil
	})
	v.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
		if !req.(*ip.IPRouteAddDel).IsMultipath {
			t.Fatal("whole default route mutation")
		}
		routes++
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	session := pppoe.Session{Iface: "wan0", HostIf: "tap0", Username: "u", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492, DefaultRoute: true, MSSClamp: true} //nolint:gosec // required non-production fixture marker
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	hook := func(dir string) {
		t.Helper()
		cmd := exec.Command(filepath.Join(dir, "ngfw-tap0")) //nolint:gosec // only the rendered hook in this private test directory
		cmd.Env = append(os.Environ(), "PPP_IPPARAM=ngfw-tap0", "IPLOCAL=198.51.100.5", "IPREMOTE=198.51.100.1", "DNS1=192.0.2.53")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(output))
		}
	}
	hook(paths.IPUpDir)
	if err := rt.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := rt.State("wan0", 0, "")
	if err != nil || state.Phase != "up" || !present || state.Dns[0] != "192.0.2.53" {
		t.Fatal(state, err)
	}
	if err := rt.poll(context.Background()); err != nil || writes != 1 {
		t.Fatal("unchanged hook duplicated address", writes, err)
	}
	present = false // simulate VPP object loss without a host restart
	if err := rt.poll(context.Background()); err != nil || !present || writes != 2 {
		t.Fatal("runtime failed to rebuild", writes, err)
	}
	hook(paths.IPDownDir)
	if err := rt.poll(context.Background()); err != nil || present || writes != 3 {
		t.Fatal("down did not withdraw", writes, err)
	}
	hook(paths.IPUpDir)
	if err := rt.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Apply(context.Background(), nil); err != nil || present {
		t.Fatal("removal did not synchronously withdraw", err)
	}

	if _, err := os.Stat(filepath.Join(paths.StateDir, "tap0.state")); !os.IsNotExist(err) {
		t.Fatal("removed session retained hook state")
	}
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	if err := rt.poll(context.Background()); err != nil || present {
		t.Fatal("re-added session reused stale negotiated state", err)
	}
	if routes < 4 || len(runner.Calls()) != 0 {
		t.Fatal("slot supervision or missing route mirror", routes, runner.Calls())
	}
}
func TestPppoeWatcherStopsOnContext(t *testing.T) {
	rt, _, _ := newTestRuntime(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { rt.watch(ctx, func(context.Context) error { return nil }); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher leaked")
	}
}
func TestPppoeExitObservationsCountOnlyNewFailures(t *testing.T) {
	old := observeExit(pppoeFailure{}, "ExecMainStatus=19\nNRestarts=2\n")
	if old.count != 1 || old.message != "authentication to peer failed" {
		t.Fatal(old)
	}
	same := observeExit(old, "ExecMainStatus=19\nNRestarts=2\n")
	if same.count != 1 {
		t.Fatal("same exit counted repeatedly")
	}
	next := observeExit(same, "ExecMainStatus=19\nNRestarts=4\n")
	if next.count != 3 {
		t.Fatal(next)
	}
}

func TestPppoeCredentialRotationRestartsAndInvalidatesHook(t *testing.T) {
	rt, runner, _ := newTestRuntime(t)
	session := pppoe.Session{Iface: "wan0", HostIf: "tap0", Username: "u", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492} //nolint:gosec // required non-production fixture marker
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt.stateDir, "tap0.state"), []byte("phase=up\nlocal=192.0.2.4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := len(runner.Calls())
	session.Password = "NGFW_TEST_PSK_F-pppoe-client-wiring-rotation"
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	calls := runner.Calls()[before:]
	if len(calls) != 2 || calls[0].Args[0] != "stop" || calls[1].Args[0] != "restart" || calls[1].Args[1] != "ngfw-pppoe-tap0.service" {
		t.Fatal("credential rotation did not restart", calls)
	}
	if _, err := os.Stat(filepath.Join(rt.stateDir, "tap0.state")); !os.IsNotExist(err) {
		t.Fatal("rotated credentials retained earlier hook")
	}
}
func TestPppoePartialMirrorFailureIsTrackedAndWithdrawn(t *testing.T) {
	rt, _, v := newTestRuntime(t)
	rt.globalsOwner = false
	root := t.TempDir()
	t.Setenv(EnvHostServicesDir, root)
	paths := pppoe.PathsUnder(filepath.Join(root, "pppoe"))
	rt.renderer = pppoe.New(pppoe.WithPaths(paths))
	rt.stateDir = paths.StateDir
	v.AddInterface("wan0", "")
	v.Reply("sw_interface_get_table", &interfaces.SwInterfaceGetTableReply{})
	v.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
		if req.(*ip.IPRouteAddDel).IsAdd {
			return nil, errors.New("route rejected")
		}
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	session := pppoe.Session{Iface: "wan0", HostIf: "tap0", Username: "u", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492, DefaultRoute: true} //nolint:gosec // required non-production fixture marker
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.StateDir, "tap0.state"), []byte("phase=up\nlocal=192.0.2.4\npeer=192.0.2.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rt.poll(context.Background()); err == nil {
		t.Fatal("expected partial route failure")
	}
	if len(rt.mirrored) != 1 {
		t.Fatal("partial write was not tracked")
	}
	if err := rt.Apply(context.Background(), nil); err != nil || len(rt.mirrored) != 0 {
		t.Fatal("partial write not withdrawn", err)
	}
}

func TestPppoeClampOnlyEditPreservesNegotiatedHook(t *testing.T) {
	rt, runner, _ := newTestRuntime(t)
	session := pppoe.Session{Iface: "wan0", HostIf: "tap0", Username: "u", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492, MSSClamp: true} //nolint:gosec // required non-production fixture marker
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt.stateDir, "tap0.state"), []byte("phase=up\nlocal=192.0.2.4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := len(runner.Calls())
	session.MSSClamp = false
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls()) != before {
		t.Fatal("clamp-only change restarted dialer")
	}
	state, err := rt.State("wan0", 0, "")
	if err != nil || state.Phase != "up" {
		t.Fatal("clamp edit lost negotiated state", state, err)
	}
}
