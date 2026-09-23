package chrony

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration: two real chronyd children with test-scoped paths under
// /run/vrx-test/<prefix>/chrony/{server,client}, both started with -x (never touch the host
// clock): the server serves local stratum 10 on 127.0.0.1:3<slot>23, the client (port 0, no
// server socket) syncs from it. Never the system unit, never /etc/chrony.

type instance struct {
	paths Paths
	r     *Renderer
	cmd   *exec.Cmd
	log   string
}

func prepare(t *testing.T, prefix, name string, sourcePort uint16, secrets SecretResolver) *instance {
	t.Helper()
	p := TestPaths(prefix, name)
	p.SourcePort = sourcePort
	u, err := user.Lookup("_chrony")
	if err != nil {
		t.Fatalf("_chrony user: %v", err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	_ = os.RemoveAll(p.ConfDir)
	for _, d := range []string{p.ConfDir, p.LogDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(d, uid, gid); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, 0o750); err != nil { //nolint:gosec // directory: _chrony must traverse it
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(p.SourceDir(), 0o755); err != nil { //nolint:gosec // chronyd reads sourcedir as _chrony
		t.Fatal(err)
	}
	// chronyd and chronyc run as _chrony after dropping root: the parent must be traversable.
	if err := os.Chmod(filepath.Dir(p.ConfDir), 0o755); err != nil { //nolint:gosec // parent must be traversable by _chrony
		t.Fatal(err)
	}
	return &instance{paths: p, r: New(NewRunner(), WithPaths(p), WithSecrets(secrets)), log: filepath.Join(p.LogDir, "chronyd.log")}
}

func (in *instance) start(t *testing.T) {
	t.Helper()
	// -x: never control the system clock (mandatory on the shared host); -n: foreground.
	argv := []string{"-f", in.paths.Conf(), "-n", "-x", "-l", in.log}
	if !slices.Contains(argv, "-x") {
		t.Fatal("chronyd argv must contain -x")
	}
	in.cmd = exec.Command(ChronydBin, argv...) //nolint:gosec // fixed test argv
	if err := in.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Logf("started pid %d: %s %s", in.cmd.Process.Pid, ChronydBin, strings.Join(argv, " "))
	cmd := in.cmd
	t.Cleanup(func() { stop(t, cmd) })
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := in.r.Chronyc(context.Background(), "-c", "tracking"); err == nil {
			return
		} else if time.Now().After(deadline) {
			log, _ := os.ReadFile(in.log)
			t.Fatalf("chronyd did not answer: %v\n%s", err, log)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func stop(t *testing.T, cmd *exec.Cmd) {
	if cmd.ProcessState != nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	t.Logf("stopped pid %d", cmd.Process.Pid)
}

func TestChronyIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	port := uint16(3000 + slot*100 + 23) //nolint:gosec // slots 1–12
	ctx := context.Background()
	secrets := func(ref string) ([]byte, error) {
		if ref == "key/rig" {
			return []byte("VRX_TEST_PSK_RF3_rig"), nil
		}
		return nil, errors.New("unknown")
	}

	srv := prepare(t, prefix, "server", 0, secrets)
	cli := prepare(t, prefix, "client", port, secrets)
	srvDesired := &vrxv1.NtpService{
		Enabled: proto.Bool(true), LocalStratum: proto.Uint32(10),
		Allow: []string{"127.0.0.1/32"}, Listen: []string{"127.0.0.1"}, Port: proto.Uint32(uint32(port)),
	}
	cliDesired := func(extra bool) *vrxv1.DesiredState {
		n := &vrxv1.NtpService{
			Enabled: proto.Bool(true), Port: proto.Uint32(0),
			Servers: []*vrxv1.NtpService_Server{{Address: proto.String("127.0.0.1"), MinPoll: proto.Int32(-2), MaxPoll: proto.Int32(0)}},
		}
		if extra {
			n.Servers = append(n.Servers, &vrxv1.NtpService_Server{Address: proto.String("192.0.2.123"), KeyRef: proto.String("key/rig")})
		}
		return &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Ntp: n}}
	}

	sf, err := srv.r.Render(ctx, srvDesired)
	if err != nil {
		t.Fatal(err)
	}
	cf, err := cli.r.Render(ctx, cliDesired(false))
	if err != nil {
		t.Fatal(err)
	}
	listenOf := func(conf []byte) []string {
		var out []string
		for _, l := range strings.Split(string(conf), "\n") {
			if strings.HasPrefix(l, "bindaddress ") || strings.HasPrefix(l, "port ") {
				out = append(out, l)
			}
		}
		return out
	}
	if got := listenOf(sf[srv.paths.Conf()].Content); !slices.Equal(got, []string{"bindaddress 127.0.0.1", fmt.Sprintf("port %d", port)}) {
		t.Fatalf("server listen list %v", got)
	}
	if got := listenOf(cf[cli.paths.Conf()].Content); !slices.Equal(got, []string{"port 0"}) {
		t.Fatalf("client listen list %v", got)
	}
	t.Logf("server listens on %v; client %v (no NTP server socket)", listenOf(sf[srv.paths.Conf()].Content), listenOf(cf[cli.paths.Conf()].Content))

	for _, x := range []struct {
		in    *instance
		files renderers.Files
	}{{srv, sf}, {cli, cf}} {
		if err := x.in.r.Validate(ctx, x.files); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	}
	t.Log("chronyd -p (chrony.conf + vrx.sources): accepted for server and client")
	bad := renderers.Files{}
	for p, f := range cf {
		bad[p] = f
	}
	b := bad[cli.paths.Sources()]
	b.Content = append(append([]byte{}, b.Content...), "server 127.0.0.1 bogusoption\n"...)
	bad[cli.paths.Sources()] = b
	if err := cli.r.Validate(ctx, bad); !errors.Is(err, ErrDaemon) {
		t.Fatalf("broken sources: want ErrDaemon, got %v", err)
	} else {
		t.Logf("chronyd -p rejected the broken sources file: %v", err)
	}

	var ar *ActionRequired
	if err := srv.r.Apply(ctx, sf); !errors.As(err, &ar) || ar.Action != "start" {
		t.Fatalf("Apply before start: want start, got %v", err)
	}
	t.Logf("Apply before start: %v", ar)
	if err := cli.r.Apply(ctx, cf); !errors.As(err, &ar) {
		t.Fatalf("client Apply before start: %v", err)
	}
	srv.start(t)
	cli.start(t)

	// The client selects the server within 30 s: sources lists it, tracking has stratum 11.
	var st State
	deadline := time.Now().Add(30 * time.Second)
	for {
		st, err = cli.r.State(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if st.Tracking != nil && st.Tracking.Stratum > 0 && len(st.Sources) == 1 && st.Sources[0].State == "*" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("client did not sync within 30 s: %+v", st)
		}
		time.Sleep(500 * time.Millisecond)
	}
	raw, _ := cli.r.Chronyc(ctx, "-c", "sources")
	t.Logf("client chronyc -c sources: %s", strings.TrimSpace(string(raw)))
	raw, _ = cli.r.Chronyc(ctx, "-c", "tracking")
	t.Logf("client chronyc -c tracking: %s", strings.TrimSpace(string(raw)))
	if st.Sources[0].Name != "127.0.0.1" || st.Tracking.Stratum != 11 {
		t.Fatalf("unexpected state %+v", st)
	}
	sst, err := srv.r.State(ctx)
	if err != nil || sst.ServerStats["ntpPacketsReceived"] == "0" {
		t.Fatalf("server stats: %+v %v", sst.ServerStats, err)
	}
	t.Logf("server serverstats: ntpPacketsReceived=%s; server tracking stratum=%d", sst.ServerStats["ntpPacketsReceived"], sst.Tracking.Stratum)
	for _, in := range []*instance{srv, cli} {
		log, _ := os.ReadFile(in.log)
		if !strings.Contains(string(log), "Disabled control of system clock") {
			t.Fatalf("%s: chronyd log lacks 'Disabled control of system clock':\n%s", in.paths.ConfDir, log)
		}
	}
	t.Log("both chronyd logs: \"Disabled control of system clock\"")

	p := cli.r.NewPoller()
	evs, err := p.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first poll: %v", evs)

	// Change sources + keys → reload sources / rekey, no restart.
	cf2, err := cli.r.Render(ctx, cliDesired(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.r.Validate(ctx, cf2); err != nil {
		t.Fatal(err)
	}
	if err := cli.r.Apply(ctx, cf2); err != nil {
		t.Fatalf("Apply (sources + keys): %v", err)
	}
	st, err = cli.r.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, s := range st.Sources {
		names = append(names, s.Name)
	}
	if !slices.Contains(names, "192.0.2.123") {
		t.Fatalf("reload sources: sources %v", names)
	}
	t.Logf("after reload sources + rekey: sources %v", names)
	evs, _ = p.Poll(ctx)
	t.Logf("poll after change: %v", evs)
	if msg, err := cli.r.Retrieve(ctx); err != nil || msg == nil {
		t.Fatal(err)
	}

	// A chrony.conf change needs a restart: typed result, the test restarts its child.
	srvDesired.LocalStratum = proto.Uint32(9)
	sf2, err := srv.r.Render(ctx, srvDesired)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.r.Apply(ctx, sf2); !errors.As(err, &ar) || ar.Action != "restart" {
		t.Fatalf("conf change: want restart, got %v", err)
	}
	t.Logf("conf change: %v", ar)
	if err := srv.r.Apply(ctx, sf2); !errors.As(err, &ar) || ar.Action != "restart" {
		t.Fatalf("second Apply before the restart: want restart again (M2), got %v", err)
	}
	t.Logf("second Apply, same files, chronyd not restarted: %v", ar)
	stop(t, srv.cmd)
	srv.start(t)
	if err := srv.r.Apply(ctx, sf2); err != nil {
		pid, perr := srv.r.daemonPid()
		st, serr := procStartTicks(pid)
		t.Fatalf("Apply after the restart: %v (pid %d %v, start %d %v, pending %+v)", err, pid, perr, st, serr, getPending(srv.paths.PendingFile))
	}
	t.Log("Apply after the restart: nil (pending request cleared: chronyd started after it)")
	deadline = time.Now().Add(30 * time.Second)
	for {
		st, err = cli.r.State(ctx)
		if err == nil && slices.ContainsFunc(st.Sources, func(s Source) bool { return s.Name == "127.0.0.1" && s.Stratum == 9 }) {
			break
		}
		if time.Now().After(deadline) {
			a, _ := srv.r.Chronyc(ctx, "-c", "tracking")
			b, _ := cli.r.Chronyc(ctx, "-c", "sources")
			c, _ := cli.r.Chronyc(ctx, "-c", "sourcestats")
			t.Fatalf("client did not follow the restarted stratum-9 server: %+v %v\nserver tracking %s\nclient sources %s\nclient sourcestats %s", st.Tracking, err, a, b, c)
		}
		time.Sleep(500 * time.Millisecond)
	}
	raw, _ = cli.r.Chronyc(ctx, "-c", "sources")
	t.Logf("after server restart (local stratum 9): client chronyc -c sources: %s", strings.ReplaceAll(strings.TrimSpace(string(raw)), "\n", " | "))

	// Rollback of the client to its first rendering.
	if err := cli.r.Apply(ctx, cf); err != nil {
		t.Fatal(err)
	}
	st, _ = cli.r.State(ctx)
	if len(st.Sources) != 1 {
		t.Fatalf("rollback: sources %+v", st.Sources)
	}
	t.Log("rollback: client back to one source")
}
