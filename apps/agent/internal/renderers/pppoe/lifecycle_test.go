package pppoe

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ngfw/agent/internal/renderers"
)

type ipv6Rig struct {
	r       *Renderer
	paths   Paths
	host    string
	fixture string
}

// Execute the actual rendered lifecycle with private substitutes for sysctls,
// ip and dhcpcd. No daemon, host address or host sysctl is changed.
func newIPv6Rig(t *testing.T, mode string) *ipv6Rig {
	t.Helper()
	s := minimalSession()
	s.IPv6 = mode
	r, paths := renderTo(t, s)
	x := &ipv6Rig{r: r, paths: paths, host: s.HostIf, fixture: t.TempDir()}
	sysctl := filepath.Join(x.fixture, "sysctl", "ppp0")
	if err := os.MkdirAll(sysctl, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil { //nolint:gosec // G703: paths belong to this private rendered fixture
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // G302: private fake executables/helper require owner execute permission
			t.Fatal(err)
		}
	}
	ip := filepath.Join(x.fixture, "ip")
	write(ip, "#!/usr/bin/python3\nfrom pathlib import Path\nimport sys,time\nr=Path("+strconv.Quote(x.fixture)+")\nif 'show' in sys.argv and (r/'block').exists():\n (r/'entered').touch()\n while not (r/'release').exists(): time.sleep(.01)\nif 'show' in sys.argv and 'addr' in sys.argv: print('1: ppp0 inet6 2001:db8::1/64 scope global')\n")
	dhcp := filepath.Join(x.fixture, "dhcpcd")
	write(dhcp, "#!/usr/bin/python3\nfrom pathlib import Path\nimport os,signal,subprocess,time\nr=Path("+strconv.Quote(x.fixture)+")\ndef term(*_):\n env=dict(os.environ,reason='BOUND6',new_dhcp6_ia_pd1_prefix1='2001:db8:100::',new_dhcp6_ia_pd1_prefix1_length='56')\n subprocess.run(['/usr/bin/python3',"+strconv.Quote(paths.ipv6Helper(s.HostIf))+",'event'],env=env,check=True)\n (r/'late-event').touch()\nsignal.signal(signal.SIGTERM,term)\n(r/'dhcp-ready').touch()\nwhile not (r/'child-exit').exists(): time.sleep(.02)\nraise SystemExit(7)\n")
	helper := paths.ipv6Helper(s.HostIf)
	b, err := os.ReadFile(helper) //nolint:gosec // rendered private fixture
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ReplaceAll(string(b), `IP = "/usr/sbin/ip"`, "IP = "+strconv.Quote(ip))
	body = strings.ReplaceAll(body, `SYSCTL = Path("/proc/sys/net/ipv6/conf")`, "SYSCTL = Path("+strconv.Quote(filepath.Dir(sysctl))+")")
	body = strings.ReplaceAll(body, `DHCPCD = "/usr/sbin/dhcpcd"`, "DHCPCD = "+strconv.Quote(dhcp))
	write(helper, body)
	if err := r.ResumeIPv6(s.HostIf); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := r.StopIPv6(ctx, s.HostIf); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	})
	return x
}

func (x *ipv6Rig) command(t *testing.T, action string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	args := []string{x.paths.ipv6Helper(x.host), action}
	if action != "stop" {
		args = append(args, "ppp0", "", "", strconv.Itoa(os.Getpid()), x.admission(t))
	}
	cmd := exec.CommandContext(ctx, Python3Bin, args...) //nolint:gosec // rendered private fixture
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("helper %s: %s", action, out)
		return err
	}
	return nil
}

func (x *ipv6Rig) record(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(x.paths.StateDir, x.host+".ipv6.pid")) //nolint:gosec // private fixture
	if err != nil {
		t.Fatal(err)
	}
	var item map[string]any
	if err := json.Unmarshal(b, &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func waitIPv6(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("bounded lifecycle observation timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func ipv6ProcessGone(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")) //nolint:gosec // observed owned fixture PID
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	_, fields, ok := strings.Cut(string(b), ") ")
	return ok && strings.HasPrefix(fields, "Z ")
}

func TestIPv6OwnedShutdownAndLateEvent(t *testing.T) {
	x := newIPv6Rig(t, "dhcpv6")
	if err := x.command(t, "up"); err != nil {
		t.Fatal(err)
	}
	waitIPv6(t, func() bool { _, err := os.Stat(filepath.Join(x.fixture, "dhcp-ready")); return err == nil })
	item := x.record(t)
	pid := int(item["pid"].(float64))
	child := int(item["child"].(map[string]any)["pid"].(float64))
	if err := x.command(t, "down"); err != nil {
		t.Fatal(err)
	}
	if !ipv6ProcessGone(pid) || !ipv6ProcessGone(child) {
		t.Fatal("shutdown returned with an owned refresher/child alive")
	}
	if _, err := os.Stat(filepath.Join(x.fixture, "late-event")); err != nil {
		t.Fatal("late DHCP event control did not execute", err)
	}
	if _, err := os.Stat(filepath.Join(x.paths.StateDir, x.host+".pd")); !os.IsNotExist(err) {
		t.Fatal("revoked DHCP child recreated PD")
	}
	st, err := x.r.ReadState(x.host, 0, "")
	if err != nil || st.GetPhase() != "down" {
		t.Fatalf("post-shutdown state=%v error=%v", st, err)
	}
}

func TestIPv6CollectionRevocationBarrier(t *testing.T) {
	x := newIPv6Rig(t, "slaac")
	if err := x.command(t, "up"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(x.fixture, "block"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitIPv6(t, func() bool { _, err := os.Stat(filepath.Join(x.fixture, "entered")); return err == nil })
	finished := make(chan error, 1)
	go func() { finished <- x.r.StopIPv6(t.Context(), x.host) }()
	waitIPv6(t, func() bool {
		b, err := os.ReadFile(filepath.Join(x.paths.StateDir, x.host+".ipv6.pid")) //nolint:gosec // private fixture
		return err == nil && strings.Contains(string(b), `"revoked": true`)
	})
	if err := os.WriteFile(filepath.Join(x.fixture, "release"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if err := x.command(t, "down"); err != nil {
		t.Fatal(err)
	}
	st, err := x.r.ReadState(x.host, 0, "")
	if err != nil || st.GetPhase() != "down" || st.GetIpv6() != "" {
		t.Fatalf("in-flight collection published after revocation: %v %v", st, err)
	}
}

func TestIPv6PIDIdentityRefusesForeignSignals(t *testing.T) {
	x := newIPv6Rig(t, "slaac")
	foreign := exec.Command("sleep", "30")
	if err := foreign.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = foreign.Process.Kill(); _ = foreign.Wait() }()
	pidf := filepath.Join(x.paths.StateDir, x.host+".ipv6.pid")
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(foreign.Process.Pid), "stat")) //nolint:gosec // own foreign-control fixture
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(stat)[strings.LastIndex(string(stat), ")")+1:])
	var bodies []string
	bodies = append(bodies, "0\n")
	for _, identity := range []struct {
		pid   int
		start string
	}{{0, "0"}, {1, "0"}, {foreign.Process.Pid, "reused"}, {foreign.Process.Pid, fields[19]}} {
		b, err := json.Marshal(map[string]any{"pid": identity.pid, "start": identity.start, "generation": "fixture-generation",
			"argv": []string{Python3Bin, x.paths.ipv6Helper(x.host), "refresh", "fixture-generation", "ppp0", "", "", ""}})
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, string(b))
	}
	for _, body := range bodies {
		if err := os.WriteFile(pidf, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := x.command(t, "stop"); err == nil {
			t.Fatal("invalid/foreign identity accepted")
		}
		if ipv6ProcessGone(foreign.Process.Pid) {
			t.Fatal("foreign fixture received a signal")
		}
	}
	if err := os.Remove(pidf); err != nil {
		t.Fatal(err)
	}
}

func TestIPv6SetupAndClientFailures(t *testing.T) {
	for _, failure := range []string{"missing-client", "exec-failure", "sysctl-failure", "state-write-failure", "child-exit"} {
		t.Run(failure, func(t *testing.T) {
			x := newIPv6Rig(t, "dhcpv6")
			switch failure {
			case "missing-client":
				if err := os.Remove(filepath.Join(x.fixture, "dhcpcd")); err != nil {
					t.Fatal(err)
				}
			case "exec-failure":
				if err := os.WriteFile(filepath.Join(x.fixture, "dhcpcd"), []byte("invalid executable\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "sysctl-failure":
				if err := os.Mkdir(filepath.Join(x.fixture, "sysctl", "ppp0", "accept_ra"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "state-write-failure":
				if err := os.Mkdir(filepath.Join(x.paths.StateDir, x.host+".state6"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			err := x.command(t, "up")
			if failure == "child-exit" {
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(x.fixture, "child-exit"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
				waitIPv6(t, func() bool { st, err := x.r.ReadState(x.host, 0, ""); return err == nil && st.GetPhase() == "failed" })
			} else if err == nil {
				t.Fatal("failed prerequisite silently reported success")
			}
			if failure != "state-write-failure" {
				st, err := x.r.ReadState(x.host, 0, "")
				if err != nil || st.GetPhase() != "failed" || st.GetLastError() == "" {
					t.Fatalf("missing failure status: %v %v", st, err)
				}
			}
		})
	}
}

func TestIPv6NaturalLossAndGenerationReplacement(t *testing.T) {
	for _, event := range []string{"link-loss", "hook-removal", "replacement"} {
		t.Run(event, func(t *testing.T) {
			x := newIPv6Rig(t, "slaac")
			if err := x.command(t, "up"); err != nil {
				t.Fatal(err)
			}
			old := int(x.record(t)["pid"].(float64))
			switch event {
			case "link-loss":
				for _, name := range []string{"accept_ra", "autoconf", "accept_ra_defrtr"} {
					if err := os.Remove(filepath.Join(x.fixture, "sysctl", "ppp0", name)); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Remove(filepath.Join(x.fixture, "sysctl", "ppp0")); err != nil {
					t.Fatal(err)
				}
			case "hook-removal":
				if err := os.Remove(filepath.Join(x.paths.IPv6UpDir, "ngfw-"+x.host)); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				if err := x.command(t, "up"); err != nil {
					t.Fatal(err)
				}
				if !ipv6ProcessGone(old) || int(x.record(t)["pid"].(float64)) == old {
					t.Fatal("replacement retained old writer")
				}
				return
			}
			waitIPv6(t, func() bool { return ipv6ProcessGone(old) })
			st, err := x.r.ReadState(x.host, 0, "")
			if err != nil || st.GetPhase() != "down" || st.GetIpv6() != "" {
				t.Fatalf("natural loss retained state: %v %v", st, err)
			}
		})
	}
}

func TestIPv6AbruptPppdLossStopsChild(t *testing.T) {
	x := newIPv6Rig(t, "dhcpv6")
	pppd := exec.Command("sleep", "30")
	if err := pppd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pppd.Process.Kill(); _ = pppd.Wait() }()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, Python3Bin, x.paths.ipv6Helper(x.host), "up", "ppp0", "", "", strconv.Itoa(pppd.Process.Pid), x.admission(t)) //nolint:gosec // own private fixture
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	item := x.record(t)
	refresher := int(item["pid"].(float64))
	child := int(item["child"].(map[string]any)["pid"].(float64))
	if err := pppd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = pppd.Wait()
	waitIPv6(t, func() bool { return ipv6ProcessGone(refresher) && ipv6ProcessGone(child) })
	st, err := x.r.ReadState(x.host, 0, "")
	if err != nil || st.GetPhase() != "down" || st.GetIpv6() != "" {
		t.Fatalf("pppd loss retained state: %v %v", st, err)
	}
}

func TestIPv6ApplyStopsBeforeReplacingOrRemovingFiles(t *testing.T) {
	for _, edit := range []string{"remove", "off", "mode", "credentials"} {
		t.Run(edit, func(t *testing.T) {
			x := newIPv6Rig(t, "dhcpv6")
			if err := x.command(t, "up"); err != nil {
				t.Fatal(err)
			}
			item := x.record(t)
			old := int(item["pid"].(float64))
			child := int(item["child"].(map[string]any)["pid"].(float64))
			s := minimalSession()
			s.IPv6 = "dhcpv6"
			var sessions []Session
			switch edit {
			case "off":
				s.IPv6 = "off"
			case "mode":
				s.IPv6 = "slaac"
			case "credentials":
				s.Username = "replacement-user"
			}
			if edit != "remove" {
				sessions = []Session{s}
			}
			// Seed the unit/peer inventory for this real rendered private session.
			rr := renderers.NewRecordingRunner().Succeed(SystemctlBin, "")
			if err := x.r.Apply(t.Context(), rr, sessions); err != nil {
				t.Fatal(err)
			}
			if !ipv6ProcessGone(old) || !ipv6ProcessGone(child) {
				t.Fatal("configuration edit retained old owned processes")
			}
			if _, err := os.Stat(filepath.Join(x.paths.StateDir, x.host+".ipv6.pid")); !os.IsNotExist(err) {
				t.Fatal("configuration edit retained old identity handle")
			}
		})
	}
}

func TestIPv6PathsRejectShellExpansion(t *testing.T) {
	for _, base := range []string{"/tmp/$(id)", "/tmp/`id`", "/tmp/space path", "/tmp/../other"} {
		if err := PathsUnder(base).Validate(); err == nil && base != "/tmp/../other" {
			t.Errorf("unsafe custom path accepted: %s", base)
		}
	}
	p := PathsUnder("/tmp/safe")
	p.StateDir = "/tmp/../other"
	if err := p.Validate(); err == nil {
		t.Fatal("unclean custom path accepted")
	}
}

func TestIPv6TransitionAdmissionRemainsFenced(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(strconv.FormatBool(running), func(t *testing.T) {
			x := newIPv6Rig(t, "slaac")
			if running {
				if err := x.command(t, "up"); err != nil {
					t.Fatal(err)
				}
			}
			if err := x.r.StopIPv6(t.Context(), x.host); err != nil {
				t.Fatal(err)
			}
			state := filepath.Join(x.paths.StateDir, x.host+".state6")
			if err := os.Remove(state); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			// This late up is the old pppd hook arriving after StopIPv6.
			if err := x.command(t, "up"); err != nil {
				t.Fatal(err)
			}
			for _, suffix := range []string{".state6", ".ipv6.pid", ".pd"} {
				if _, err := os.Stat(filepath.Join(x.paths.StateDir, x.host+suffix)); !os.IsNotExist(err) {
					t.Fatalf("fenced up recreated %s", suffix)
				}
			}
			if err := x.r.ResumeIPv6(x.host); err != nil {
				t.Fatal(err)
			}
			if err := x.command(t, "up"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(state); err != nil {
				t.Fatal("replacement admission did not reopen", err)
			}
		})
	}
}

func TestIPv6PinnedParentRejectsRecycledNumericIdentity(t *testing.T) {
	x := newIPv6Rig(t, "slaac")
	parent := exec.Command("sleep", "30")
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Process.Kill(); _ = parent.Wait() }()
	helper := x.paths.ipv6Helper(x.host)
	body, err := os.ReadFile(helper) //nolint:gosec // private rendered fixture
	if err != nil {
		t.Fatal(err)
	}
	// Simulate /proc answering for a reused numeric PID after the original
	// parent dies. The inherited pidfd still names the original process.
	body = []byte(strings.Replace(string(body), "def identity(pid):", "def identity(pid):\n    if pid == "+strconv.Itoa(parent.Process.Pid)+":\n        return 'recycled-parent-starttime'", 1))
	if err := os.WriteFile(helper, body, 0600); err != nil { //nolint:gosec // private rendered fixture
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), Python3Bin, helper, "up", "ppp0", "", "", strconv.Itoa(parent.Process.Pid), x.admission(t)) //nolint:gosec // private rendered fixture
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("up: %v %s", err, out)
	}
	refresher := int(x.record(t)["pid"].(float64))
	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = parent.Wait()
	waitIPv6(t, func() bool { return ipv6ProcessGone(refresher) })
	st, err := x.r.ReadState(x.host, 0, "")
	if err != nil || st.GetPhase() != "down" {
		t.Fatalf("recycled numeric identity retained writer: %v %v", st, err)
	}
}

func (x *ipv6Rig) admission(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(x.paths.StateDir, x.host+".ipv6.admission")) //nolint:gosec // private fixture
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestIPv6OldAdmissionRejectedAfterReopen(t *testing.T) {
	x := newIPv6Rig(t, "slaac")
	oldAdmission := x.admission(t)
	if err := x.r.StopIPv6(t.Context(), x.host); err != nil {
		t.Fatal(err)
	}
	if err := x.r.ResumeIPv6(x.host); err != nil {
		t.Fatal(err)
	}
	if x.admission(t) == oldAdmission {
		t.Fatal("transition reused admission generation")
	}
	// An old up captured its token before stop, then waited through reopen.
	cmd := exec.CommandContext(t.Context(), Python3Bin, x.paths.ipv6Helper(x.host), "up", "ppp0", "", "", strconv.Itoa(os.Getpid()), oldAdmission) //nolint:gosec // private fixture
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("late up: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(x.paths.StateDir, x.host+".ipv6.pid")); !os.IsNotExist(err) {
		t.Fatal("old admission created replacement writer")
	}
}

func TestIPv6MissingHelperPreservesProcessEvidence(t *testing.T) {
	x := newIPv6Rig(t, "slaac")
	if err := x.command(t, "up"); err != nil {
		t.Fatal(err)
	}
	pid := int(x.record(t)["pid"].(float64))
	helper := x.paths.ipv6Helper(x.host)
	body, err := os.ReadFile(helper) //nolint:gosec // private rendered fixture
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(helper); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.WriteFile(helper, body, 0600); err != nil { //nolint:gosec // private rendered fixture
			t.Error(err)
		}
	}()
	if err := x.r.StopIPv6(t.Context(), x.host); err == nil {
		t.Fatal("missing helper accepted despite process evidence")
	}
	if _, err := os.Stat(filepath.Join(x.paths.StateDir, x.host+".ipv6.pid")); err != nil {
		t.Fatal("failed stop discarded process evidence", err)
	}
	if ipv6ProcessGone(pid) {
		t.Fatal("control writer exited before fail-closed assertion")
	}
}
