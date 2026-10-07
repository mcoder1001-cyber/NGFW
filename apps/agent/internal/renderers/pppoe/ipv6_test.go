package pppoe

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"ngfw/agent/internal/renderers"
)

// IPv6 selects which files a session gets: off → none and `noipv6`; slaac → `+ipv6` and the ipv6-up/down hooks;
// dhcpv6 → additionally the dhcpcd configuration (IA_NA + IA_PD, kernel RA) and its event script.
func TestRenderIPv6Modes(t *testing.T) {
	for _, tc := range []struct {
		mode         string
		pppd         string
		hooks, dhcp6 bool
		defrtr       string
	}{
		{"off", "noipv6", false, false, ""},
		{"", "noipv6", false, false, ""},
		{"slaac", "+ipv6", true, false, "echo 1 > \"$c/accept_ra_defrtr\""},
		{"dhcpv6", "+ipv6", true, true, "echo 1 > \"$c/accept_ra_defrtr\""},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			s := minimalSession()
			s.IPv6, s.DefaultRoute = tc.mode, true
			files, err := testRenderer().Render([]Session{s})
			if err != nil {
				t.Fatal(err)
			}
			peer := string(files["/srv/etc/ppp/peers/ngfw-wan1"].Content)
			if !strings.Contains(peer, "\n"+tc.pppd+"\n") {
				t.Fatalf("peer file lacks %q:\n%s", tc.pppd, peer)
			}
			up, hasUp := files["/srv/etc/ppp/ipv6-up.d/ngfw-wan1"]
			_, hasDown := files["/srv/etc/ppp/ipv6-down.d/ngfw-wan1"]
			conf, hasConf := files["/srv/etc/ppp/ngfw-dhcpcd-wan1.conf"]
			script, hasScript := files["/srv/etc/ppp/ngfw-dhcp6-wan1"]
			if hasUp != tc.hooks || hasDown != tc.hooks || hasConf != tc.dhcp6 || hasScript != tc.dhcp6 {
				t.Fatalf("files: up=%v down=%v conf=%v script=%v; got %v", hasUp, hasDown, hasConf, hasScript, files.Paths())
			}
			if !tc.hooks {
				return
			}
			body := string(up.Content)
			for _, want := range []string{`[ "${PPP_IPPARAM:-}" = "ngfw-wan1" ] || exit 0`, `echo 2 > "$c/accept_ra"`, `echo 1 > "$c/autoconf"`, tc.defrtr,
				`f="$dir/wan1.state6"`, `dir="/srv/run/ngfw/pppoe"`} {
				if !strings.Contains(body, want) {
					t.Errorf("ipv6-up hook lacks %q", want)
				}
			}
			if strings.Contains(body, DhcpcdBin) != tc.dhcp6 {
				t.Errorf("ipv6-up hook starts dhcpcd=%v, want %v", strings.Contains(body, DhcpcdBin), tc.dhcp6)
			}
			if up.Mode != 0o755 || up.Secret {
				t.Errorf("hook mode %v secret %v", up.Mode, up.Secret)
			}
			if !tc.dhcp6 {
				return
			}
			c := string(conf.Content)
			for _, want := range []string{"\nipv6only\n", "\nnoipv6rs\n", "\nscript /srv/etc/ppp/ngfw-dhcp6-wan1\n", "\nia_na 1\n", "\nia_pd 2\n"} {
				if !strings.Contains(c, want) {
					t.Errorf("dhcpcd conf lacks %q:\n%s", want, c)
				}
			}
			if !strings.Contains(body, `-f "/srv/etc/ppp/ngfw-dhcpcd-wan1.conf"`) || script.Mode != 0o755 {
				t.Errorf("dhcpcd not started with the session conf, or script mode %v", script.Mode)
			}
			for p := range files {
				if strings.Contains(string(files[p].Content), s.Password) && !files[p].Secret {
					t.Errorf("password in %s", p)
				}
			}
		})
	}
	// A session without the default route keeps the kernel from installing the RA's IPv6 default.
	s := dhcp6Session()
	files, err := testRenderer().Render([]Session{s})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["/srv/etc/ppp/ipv6-up.d/ngfw-wan2"].Content), `echo 0 > "$c/accept_ra_defrtr"`) {
		t.Fatal("defaultRoute=false must set accept_ra_defrtr=0")
	}
}

// renderTo writes the files of sessions under a private root and returns the renderer.
func renderTo(t *testing.T, sessions ...Session) (*Renderer, Paths) {
	t.Helper()
	paths := PathsUnder(t.TempDir())
	r := New(WithPaths(paths))
	files, err := r.Render(sessions)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range paths.Dirs() {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderers.WriteFiles(files); err != nil {
		t.Fatal(err)
	}
	return r, paths
}

func run(t *testing.T, path string, env ...string) {
	t.Helper()
	cmd := exec.Command(path) //nolint:gosec // a script this test rendered into its private temp dir
	cmd.Env = append([]string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}, env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, out)
	}
}

// Every rendered shell script parses (`sh -n`); the hooks themselves need a PPP link and run on the lab host.
func TestIPv6ScriptsParse(t *testing.T) {
	_, paths := renderTo(t, fullSession(), dhcp6Session())
	for _, p := range []string{paths.IPv6UpDir + "/ngfw-wan0", paths.IPv6DownDir + "/ngfw-wan0", paths.IPv6UpDir + "/ngfw-wan2",
		paths.IPv6DownDir + "/ngfw-wan2", paths.HelperDir + "/ngfw-dhcp6-wan2", paths.IPUpDir + "/ngfw-wan0"} {
		if out, err := exec.Command("sh", "-n", p).CombinedOutput(); err != nil { //nolint:gosec // rendered test file
			t.Errorf("%s: %v %s", p, err, out)
		}
	}
}

// The dhcpcd event script records only a well-formed delegated prefix and forgets it when the lease ends.
func TestDHCP6ScriptRecordsDelegatedPrefix(t *testing.T) {
	r, paths := renderTo(t, dhcp6Session())
	script := paths.HelperDir + "/ngfw-dhcp6-wan2"
	pd := filepath.Join(paths.StateDir, "wan2.pd")
	run(t, script, "reason=REBIND6", "interface=ppp0", "new_dhcp6_ia_pd1_prefix1=2001:db8:100::", "new_dhcp6_ia_pd1_prefix1_length=56")
	if b, _ := os.ReadFile(pd); string(b) != "pd=2001:db8:100::/56\n" { //nolint:gosec // test path
		t.Fatalf("pd file = %q", b)
	}
	// hostile values from the network are not recorded
	run(t, script, "reason=RENEW6", "new_dhcp6_ia_pd1_prefix1=2001:db8::;reboot", "new_dhcp6_ia_pd1_prefix1_length=56")
	run(t, script, "reason=RENEW6", "new_dhcp6_ia_pd1_prefix1=2001:db8:200::", "new_dhcp6_ia_pd1_prefix1_length=5 6")
	if b, _ := os.ReadFile(pd); string(b) != "pd=2001:db8:100::/56\n" { //nolint:gosec // test path
		t.Fatalf("hostile value changed the pd file: %q", b)
	}
	// the hook merges it into state6, which ReadIPv6 reports
	if err := os.WriteFile(filepath.Join(paths.StateDir, "wan2.state6"), []byte("phase=up\nppp_iface=ppp0\naddr=2001:db8:9::100/128\npd=2001:db8:100::/56\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v6, err := r.ReadIPv6("wan2")
	if err != nil || v6.Delegated.String() != "2001:db8:100::/56" {
		t.Fatalf("delegated %v %v", v6.Delegated, err)
	}
	run(t, script, "reason=EXPIRE6")
	if _, err := os.Stat(pd); !os.IsNotExist(err) {
		t.Fatal("expired prefix kept")
	}
}

// The ipv6-down hook stops the session's refresher (and with it dhcpcd) before it records the down state, so a late
// refresh cannot resurrect "up"; it ignores other sessions' ipparam.
func TestIPv6DownHookStopsRefresherAndRecordsDown(t *testing.T) {
	_, paths := renderTo(t, dhcp6Session())
	state := filepath.Join(paths.StateDir, "wan2.state6")
	if err := os.WriteFile(state, []byte("phase=up\naddr=2001:db8:9::100/128\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sleeper := exec.Command("sleep", "30")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = sleeper.Wait(); close(exited) }()
	defer func() { _ = sleeper.Process.Kill() }()
	pidf := filepath.Join(paths.StateDir, "wan2.ipv6.pid")
	if err := os.WriteFile(pidf, []byte(strconv.Itoa(sleeper.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hook := paths.IPv6DownDir + "/ngfw-wan2"
	run(t, hook, "PPP_IPPARAM=ngfw-other", "PPP_IFACE=ppp9")
	if b, _ := os.ReadFile(state); !strings.HasPrefix(string(b), "phase=up") { //nolint:gosec // test path
		t.Fatal("hook acted for another session")
	}
	run(t, hook, "PPP_IPPARAM=ngfw-wan2", "PPP_IFACE=ppp0", "LLLOCAL=fe80::1", "LLREMOTE=fe80::2")
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("refresher not stopped")
	}
	if ws, ok := sleeper.ProcessState.Sys().(syscall.WaitStatus); !ok || ws.Signal() != syscall.SIGTERM {
		t.Fatalf("refresher not terminated by the hook: %v", sleeper.ProcessState)
	}
	b, _ := os.ReadFile(state) //nolint:gosec // test path
	if !strings.HasPrefix(string(b), "phase=down\n") || strings.Contains(string(b), "addr=") || !strings.Contains(string(b), "llremote=fe80::2\n") {
		t.Fatalf("down state: %q", b)
	}
	if _, err := os.Stat(pidf); !os.IsNotExist(err) {
		t.Fatal("pid file kept")
	}
	// a hostile interface name is ignored entirely
	if err := os.WriteFile(state, []byte("phase=up\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, hook, "PPP_IPPARAM=ngfw-wan2", "PPP_IFACE=ppp0;id")
	if b, _ := os.ReadFile(state); string(b) != "phase=up\n" { //nolint:gosec // test path
		t.Fatalf("hostile PPP_IFACE acted: %q", b)
	}
}

// Turning IPv6 off on a kept session removes its IPv6 files (and restarts it: the peer file changed).
func TestApplyRemovesIPv6FilesWhenTurnedOff(t *testing.T) {
	base := t.TempDir()
	rr := renderers.NewRecordingRunner().Succeed(SystemctlBin, "")
	r := New(WithPaths(PathsUnder(base)))
	s := dhcp6Session()
	if err := r.Apply(t.Context(), rr, []Session{s}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/etc/ppp/ipv6-up.d/ngfw-wan2", "/etc/ppp/ngfw-dhcpcd-wan2.conf", "/etc/ppp/ngfw-dhcp6-wan2"} {
		if _, err := os.Stat(base + p); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	rr.Reset()
	s.IPv6 = "slaac"
	if err := r.Apply(t.Context(), rr, []Session{s}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base + "/etc/ppp/ngfw-dhcpcd-wan2.conf"); !os.IsNotExist(err) {
		t.Fatal("dhcpcd conf kept after dhcpv6 → slaac")
	}
	if _, err := os.Stat(base + "/etc/ppp/ipv6-up.d/ngfw-wan2"); err != nil {
		t.Fatal("slaac lost its hook")
	}
	s.IPv6 = "off"
	if err := r.Apply(t.Context(), rr, []Session{s}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/etc/ppp/ipv6-up.d/ngfw-wan2", "/etc/ppp/ipv6-down.d/ngfw-wan2", "/etc/ppp/ngfw-dhcp6-wan2"} {
		if _, err := os.Stat(base + p); !os.IsNotExist(err) {
			t.Fatalf("%s kept after IPv6 off", p)
		}
	}
	if got := cmds(rr); !strings.Contains(strings.Join(got, "|"), "restart ngfw-pppoe-wan2.service") {
		t.Fatalf("IPv6 change did not redial: %v", got)
	}
	// removing the session removes the IPv6 state too
	if err := os.WriteFile(base+"/run/ngfw/pppoe/wan2.state6", []byte("phase=down\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(t.Context(), rr, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base + "/run/ngfw/pppoe/wan2.state6"); !os.IsNotExist(err) {
		t.Fatal("state6 kept after removal")
	}
}
