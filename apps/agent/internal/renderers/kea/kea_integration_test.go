package kea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration: real kea-dhcp4 / kea-dhcp6 / kea-ctrl-agent as child processes of the test,
// with test-scoped paths under /run/vrx-test/<prefix>/kea, the DHCP servers inside the
// test's own network namespace ns-<prefix>-a (veth <prefix>-a/<prefix>-b, 10.<slot>.10.1/24)
// and the ctrl-agent on 127.0.0.1:3<slot>80. Never the system units, never /etc/kea.

const ipBin = IPBin

type rig struct {
	prefix, ns, ifA, ifB string
	slot                 int
	paths                Paths
	runner               *renderers.SystemRunner
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput() //nolint:gosec // fixed test argv
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}

func setupRig(t *testing.T) *rig {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	rg := &rig{prefix: prefix, slot: slot, ns: "ns-" + prefix + "-a", ifA: prefix + "-a", ifB: prefix + "-b", paths: TestPaths(prefix, slot)}
	for _, d := range []string{rg.paths.ConfDir, rg.paths.RunDir, rg.paths.DataDir, rg.paths.LogDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	rg.runner = NewRunner(rg.paths)
	rg.runner.Allow = renderers.NewAllowlist(append(Binaries(), IPBin)...) // test-only trampoline

	if err := exec.Command(ipBin, "netns", "exec", rg.ns, "true").Run(); err == nil { //nolint:gosec // fixed argv
		t.Fatalf("namespace %s already exists (leftover of an earlier run?)", rg.ns)
	}
	run(t, ipBin, "netns", "add", rg.ns)
	t.Cleanup(func() { _ = exec.Command(ipBin, "netns", "del", rg.ns).Run() }) //nolint:gosec // fixed argv
	run(t, ipBin, "-n", rg.ns, "link", "set", "lo", "up")
	run(t, ipBin, "-n", rg.ns, "link", "add", rg.ifA, "type", "veth", "peer", "name", rg.ifB)
	run(t, ipBin, "-n", rg.ns, "addr", "add", fmt.Sprintf("10.%d.10.1/24", slot), "dev", rg.ifA)
	run(t, ipBin, "-n", rg.ns, "addr", "add", fmt.Sprintf("fd00:%d:10::1/64", slot), "dev", rg.ifA, "nodad")
	run(t, ipBin, "-n", rg.ns, "link", "set", rg.ifA, "up")
	run(t, ipBin, "-n", rg.ns, "link", "set", rg.ifB, "up")
	return rg
}

// start runs argv as a child and stops it (by its PID) in Cleanup.
func (rg *rig) start(t *testing.T, argv ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // fixed test argv
	cmd.Env = Env(rg.paths)
	log, err := os.Create(rg.paths.LogDir + "/" + strings.ReplaceAll(argv[len(argv)-1], "/", "_") + ".stderr")
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %v: %v", argv, err)
	}
	t.Logf("started pid %d: %s", cmd.Process.Pid, strings.Join(argv, " "))
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		_ = log.Close()
		t.Logf("stopped pid %d", cmd.Process.Pid)
	})
	return cmd
}

func waitFor(t *testing.T, what string, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := fn()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s: %v", what, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (rg *rig) desired(extraSubnet bool) *vrxv1.DesiredState {
	s := func(n int) string { return fmt.Sprintf("10.%d.10.%d", rg.slot, n) }
	v4 := &vrxv1.DhcpServer{
		Description: proto.String(`rig "; rm -rf / ☃`),
		Interfaces:  []string{rg.ifA},
		Subnets: map[string]*vrxv1.DhcpSubnet{
			"lan": {
				Description:  proto.String(`"}]} {"Dhcp4": 1}`),
				Subnet:       proto.String(fmt.Sprintf("10.%d.10.0/24", rg.slot)),
				Pools:        []*vrxv1.DhcpPool{{Start: proto.String(s(100)), End: proto.String(s(150))}},
				Gateway:      proto.String(s(1)),
				DnsServers:   []string{s(1)},
				DomainName:   proto.String("rig.example.test"),
				DomainSearch: []string{"rig.example.test"},
				Options: []*vrxv1.DhcpOption{
					{Code: proto.Uint32(224), Data: proto.String("vrx text")},
					{Code: proto.Uint32(225), Data: proto.String("0x0102")},
				},
				Reservations: map[string]*vrxv1.DhcpReservation{
					"printer": {Mac: proto.String("02:00:00:00:00:01"), Ip: proto.String(s(20)), Hostname: proto.String("printer")},
				},
			},
		},
	}
	if extraSubnet {
		v4.Subnets["extra"] = &vrxv1.DhcpSubnet{
			Subnet: proto.String(fmt.Sprintf("10.%d.11.0/24", rg.slot)),
			Pools:  []*vrxv1.DhcpPool{{Start: proto.String(fmt.Sprintf("10.%d.11.10", rg.slot)), End: proto.String(fmt.Sprintf("10.%d.11.20", rg.slot))}},
		}
	}
	v6 := &vrxv1.DhcpServer{
		Family:     proto.String("ipv6"),
		Interfaces: []string{rg.ifA},
		Subnets: map[string]*vrxv1.DhcpSubnet{
			"lan6": {
				Subnet:     proto.String(fmt.Sprintf("fd00:%d:10::/64", rg.slot)),
				Pools:      []*vrxv1.DhcpPool{{Start: proto.String(fmt.Sprintf("fd00:%d:10::1000", rg.slot)), End: proto.String(fmt.Sprintf("fd00:%d:10::1fff", rg.slot))}},
				DnsServers: []string{fmt.Sprintf("fd00:%d:10::1", rg.slot)},
				Options:    []*vrxv1.DhcpOption{{Code: proto.Uint32(1000), Data: proto.String("hello v6")}},
				Reservations: map[string]*vrxv1.DhcpReservation{
					"nas": {Duid: proto.String("00:01:00:01:aa:bb:cc:dd:ee:ff"), Ip: proto.String(fmt.Sprintf("fd00:%d:10::20", rg.slot))},
				},
			},
		},
	}
	return &vrxv1.DesiredState{
		Interfaces: map[string]*vrxv1.Interface{rg.ifA: {Ipv4: []string{fmt.Sprintf("10.%d.10.1/24", rg.slot)}}},
		Services:   &vrxv1.ServicesConfig{Dhcp: &vrxv1.DhcpService{Servers: map[string]*vrxv1.DhcpServer{"rig4": v4, "rig6": v6}}},
	}
}

// assertScoped fails unless every interface in the rendered configs is the rig's.
func (rg *rig) assertScoped(t *testing.T, files renderers.Files) {
	t.Helper()
	for _, p := range []string{rg.paths.Dhcp4Conf(), rg.paths.Dhcp6Conf()} {
		var root map[string]struct {
			IC struct {
				Interfaces []string `json:"interfaces"`
			} `json:"interfaces-config"`
		}
		if err := json.Unmarshal(files[p].Content, &root); err != nil {
			t.Fatal(err)
		}
		for _, v := range root {
			for _, ifn := range v.IC.Interfaces {
				if !strings.HasPrefix(ifn, rg.prefix+"-") {
					t.Fatalf("%s binds %q: outside the test scope", p, ifn)
				}
			}
			t.Logf("%s listens on %v (inside %s)", p, v.IC.Interfaces, rg.ns)
		}
	}
	var ca ctrlAgentRoot
	if err := json.Unmarshal(files[rg.paths.CtrlAgentConf()].Content, &ca); err != nil {
		t.Fatal(err)
	}
	if ca.ControlAgent.HTTPHost != "127.0.0.1" || ca.ControlAgent.HTTPPort != rg.paths.CtrlAgentPort {
		t.Fatalf("ctrl-agent listens on %s:%d", ca.ControlAgent.HTTPHost, ca.ControlAgent.HTTPPort)
	}
	t.Logf("kea-ctrl-agent listens on %s:%d", ca.ControlAgent.HTTPHost, ca.ControlAgent.HTTPPort)
}

func TestKeaIntegration(t *testing.T) {
	rg := setupRig(t)
	ctx := context.Background()
	r := New(rg.runner, WithPaths(rg.paths))
	if r.leaseCmdsHook == "" {
		t.Fatalf("libdhcp_lease_cmds.so not found under %s", rg.paths.HooksDir)
	}

	files, err := r.Render(ctx, rg.desired(false))
	if err != nil {
		t.Fatal(err)
	}
	rg.assertScoped(t, files)

	// Validate: the daemons' own checkers accept the rendering (hostile descriptions included)…
	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	t.Log("kea-dhcp4 -t / kea-dhcp6 -t / kea-ctrl-agent -t: accepted")
	// …and reject a semantically broken one (routers option with a non-address).
	bad := renderers.Files{}
	for p, f := range files {
		bad[p] = f
	}
	f := bad[rg.paths.Dhcp4Conf()]
	f.Content = []byte(strings.Replace(string(f.Content), `"data": "vrx text"`, `"data": "vrx text", "csv-format": 7`, 1))
	bad[rg.paths.Dhcp4Conf()] = f
	err = r.Validate(ctx, bad)
	if !errors.Is(err, ErrDaemon) {
		t.Fatalf("broken config: want ErrDaemon from kea-dhcp4 -t, got %v", err)
	}
	t.Logf("kea-dhcp4 -t rejected the broken file: %v", err)

	// Apply with no daemon running: files written, starting the servers is required.
	err = r.Apply(ctx, files)
	var ar *ActionRequired
	if !errors.As(err, &ar) {
		t.Fatalf("Apply before start: want ActionRequired, got %v", err)
	}
	t.Logf("Apply before start: %v", err)

	rg.start(t, ipBin, "netns", "exec", rg.ns, Dhcp4Bin, "-c", rg.paths.Dhcp4Conf())
	rg.start(t, ipBin, "netns", "exec", rg.ns, Dhcp6Bin, "-c", rg.paths.Dhcp6Conf())
	rg.start(t, CtrlAgentBin, "-c", rg.paths.CtrlAgentConf())
	for _, fam := range []int{4, 6} {
		waitFor(t, fmt.Sprintf("dhcp%d control socket", fam), func() error {
			_, err := SocketController{Paths: rg.paths}.Command(ctx, fam, "status-get", nil)
			return err
		})
	}
	waitFor(t, "ctrl-agent", func() error {
		_, err := r.agent.Command(ctx, "", "status-get", nil)
		return err
	})

	// Apply through the control channel, then compare config-get with the rendering.
	if err := r.Apply(ctx, files); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	checkDrift := func(files renderers.Files) {
		t.Helper()
		for _, fam := range []int{4, 6} {
			running, err := r.ConfigGet(ctx, fam)
			if err != nil {
				t.Fatal(err)
			}
			drift, err := ConfigDrift(files[rg.paths.conf(fam)].Content, running)
			if err != nil {
				t.Fatal(err)
			}
			if len(drift) != 0 {
				t.Fatalf("dhcp%d config-get differs from the rendering:\n%s", fam, strings.Join(drift, "\n"))
			}
			t.Logf("dhcp%d: config-get vs rendered (normalised subset diff): %d differences", fam, len(drift))
		}
	}
	checkDrift(files)

	// lease4-get-all on a fresh server is an empty list (result 3, "0 IPv4 lease(s) found.").
	resp, err := SocketController{Paths: rg.paths}.Command(ctx, 4, "lease4-get-all", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("lease4-get-all: result=%d text=%q arguments=%s", resp.Result, resp.Text, resp.Arguments)
	st, err := r.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Dhcp4.Running || !st.Dhcp6.Running || len(st.Dhcp4.Leases) != 0 || st.Dhcp4.LeasesUnsupported {
		t.Fatalf("state: %+v", st)
	}
	msg, err := r.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fields := msg.(*structpb.Struct).GetFields()
	t.Logf("Retrieve: keys dhcp4/dhcp6=%v/%v, dhcp4.running=%v, dhcp4.leases=%d", fields["dhcp4"] != nil, fields["dhcp6"] != nil, st.Dhcp4.Running, len(st.Dhcp4.Leases))

	// Through kea-ctrl-agent (HTTP on loopback, forwarded to dhcp4).
	caResp, err := r.agent.Command(ctx, "dhcp4", "config-get", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(caResp.Arguments), `"}]} {\"Dhcp4\": 1}`) {
		t.Fatalf("ctrl-agent config-get lacks the verbatim hostile description")
	}
	t.Log("kea-ctrl-agent config-get (service dhcp4): hostile description round-trips verbatim")

	// Events: first poll reports the servers running.
	p := r.NewPoller()
	evs, err := p.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first poll: %d events (e.g. %v)", len(evs), evs[0])

	// Change → Apply → Retrieve reflects it.
	files2, err := r.Render(ctx, rg.desired(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(ctx, files2); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, files2); err != nil {
		t.Fatal(err)
	}
	checkDrift(files2)
	running, _ := r.ConfigGet(ctx, 4)
	if !strings.Contains(string(running), fmt.Sprintf("10.%d.11.0/24", rg.slot)) {
		t.Fatal("added subnet missing from config-get")
	}
	t.Logf("after change: config-get has subnet 10.%d.11.0/24", rg.slot)
	evs, err = p.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("poll after change: %v", evs)

	// Rollback to the first rendering (the commit engine's Apply(previous)).
	if err := r.Apply(ctx, files); err != nil {
		t.Fatal(err)
	}
	checkDrift(files)
}
