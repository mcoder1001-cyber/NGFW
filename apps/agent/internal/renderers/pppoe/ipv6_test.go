package pppoe

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
		{"slaac", "+ipv6", true, false, "DEFAULT_ROUTE = True"},
		{"dhcpv6", "+ipv6", true, true, "DEFAULT_ROUTE = True"},
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
			body := string(files["/srv/etc/ppp/ngfw-ipv6-wan1"].Content)
			for _, want := range []string{`HOST = "wan1"`, `SYSCTL = Path("/proc/sys/net/ipv6/conf")`, tc.defrtr,
				`ROOT = Path("/srv/run/ngfw/pppoe")`} {
				if !strings.Contains(body, want) {
					t.Errorf("ipv6-up hook lacks %q", want)
				}
			}
			if !strings.Contains(body, `MODE = "`+tc.mode+`"`) {
				t.Errorf("helper lacks configured mode %s", tc.mode)
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
			if !strings.Contains(body, `CONF = "/srv/etc/ppp/ngfw-dhcpcd-wan1.conf"`) || script.Mode != 0o755 {
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
	if !strings.Contains(string(files["/srv/etc/ppp/ngfw-ipv6-wan2"].Content), `DEFAULT_ROUTE = False`) {
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
	cmd.Env = append([]string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "NGFW_PPPOE_IPV6_GENERATION=" + os.Getenv("NGFW_PPPOE_IPV6_GENERATION"), "NGFW_PPPOE_PD_ADMISSION=" + os.Getenv("NGFW_PPPOE_PD_ADMISSION")}, env...)
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
	admission := strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(paths.StateDir, "wan2.ipv6.pid"), []byte(`{"generation":"test-generation","admission":"`+admission+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.StateDir, "wan2.ipv6.admission"), []byte(admission), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NGFW_PPPOE_IPV6_GENERATION", "test-generation")
	t.Setenv("NGFW_PPPOE_PD_ADMISSION", admission)
	bound := func() {
		run(t, script, "reason=REBIND6", "interface=ppp0", "new_dhcp6_ia_pd1_prefix1=2001:db8:100::", "new_dhcp6_ia_pd1_prefix1_length=56", "new_dhcp6_ia_pd1_prefix1_vltime=3600", "new_dhcp6_ia_pd1_prefix1_pltime=1800")
	}
	bound()
	b, err := os.ReadFile(pd) //nolint:gosec // private rendered test fixture
	if err != nil || !strings.Contains(string(b), "pd=2001:db8:100::/56\n") || !strings.Contains(string(b), "pd_generation="+admission) {
		t.Fatalf("lease=%q err=%v", b, err)
	}
	writeState6(t, r, "wan2", "phase=up\n"+string(b))
	v6, err := r.ReadIPv6("wan2")
	if err != nil || !v6.Delegated.IsValid() || v6.PDGeneration != admission || v6.PDPreferredUntil.After(v6.PDValidUntil) {
		t.Fatalf("state=%+v err=%v", v6, err)
	}
	// Old daemon events cannot rebind a lease to a new admission generation.
	t.Setenv("NGFW_PPPOE_PD_ADMISSION", strings.Repeat("b", 64))
	run(t, script, "reason=EXPIRE6")
	after, err := os.ReadFile(pd) //nolint:gosec // private rendered test fixture
	if err != nil || string(after) != string(b) {
		t.Fatal("stale event changed current lease")
	}
	t.Setenv("NGFW_PPPOE_PD_ADMISSION", admission)
	// Invalid renewal must withdraw old ownership, never retain the old prefix.
	for _, fields := range [][]string{
		{"new_dhcp6_ia_pd1_prefix1=2001:db8::;reboot", "new_dhcp6_ia_pd1_prefix1_length=56"},
		{"new_dhcp6_ia_pd1_prefix1=2001:db8:200::", "new_dhcp6_ia_pd1_prefix1_length=5 6"},
		{"new_dhcp6_ia_pd1_prefix1=2001:db8:200::", "new_dhcp6_ia_pd1_prefix1_length=56", "new_dhcp6_ia_pd1_prefix1_vltime=10", "new_dhcp6_ia_pd1_prefix1_pltime=20"},
	} {
		bound()
		run(t, script, append([]string{"reason=RENEW6"}, fields...)...)
		if _, err := os.Stat(pd); !os.IsNotExist(err) {
			t.Fatal("invalid renewal kept prefix")
		}
	}
	bound()
	run(t, script, "reason=EXPIRE6")
	if _, err := os.Stat(pd); !os.IsNotExist(err) {
		t.Fatal("expired prefix kept")
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
