package frr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ngfw/agent/internal/renderers"
)

// tempPaths returns Paths under a temp dir (pathspace w12) with the config subdir created.
func tempPaths(t *testing.T) Paths {
	t.Helper()
	base := t.TempDir()
	p := Paths{
		ConfDir: filepath.Join(base, "etc"), RunDir: filepath.Join(base, "run"), Namespace: "w12",
		BinDir: "/usr/bin", ReloadLog: filepath.Join(base, "frr-reload.log"), FileMode: 0o640,
	}
	if err := os.MkdirAll(p.ConfSubdir(), 0o750); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidateArgvAndStaging(t *testing.T) {
	p := tempPaths(t)
	rr := renderers.NewRecordingRunner()
	var staged string
	rr.On(VtyshBin, func(c renderers.Command) (renderers.Output, error) {
		// The staged copy exists while the checker runs and holds the rendered content.
		b, err := os.ReadFile(c.Args[len(c.Args)-1])
		if err != nil {
			return renderers.Output{}, err
		}
		staged = string(b)
		return renderers.Output{}, nil
	})
	r := New(rr, WithPaths(p), WithSections())
	files, err := r.Render(context.Background(), staticDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	calls := rr.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}
	a := calls[0].Args
	// vtysh --config_dir <staging>/<ConfDir> --vty_socket <RunDir> -N w12 -C -f <staging>/<ConfFile>
	if len(a) != 9 || a[0] != "--config_dir" || !strings.HasSuffix(a[1], p.ConfDir) || a[1] == p.ConfDir ||
		a[2] != "--vty_socket" || a[3] != p.RunDir || a[4] != "-N" || a[5] != "w12" || a[6] != "-C" || a[7] != "-f" ||
		!strings.HasSuffix(a[8], p.ConfFile()) || a[8] == p.ConfFile() {
		t.Fatalf("argv = %q", a)
	}
	if staged != string(files[p.ConfFile()].Content) {
		t.Error("staged file differs from the rendering")
	}
	if _, err := os.Stat(a[8]); !os.IsNotExist(err) {
		t.Errorf("staging not removed: %v", err)
	}
	if _, err := os.Stat(p.ConfFile()); !os.IsNotExist(err) {
		t.Error("Validate touched the live path")
	}
}

func TestValidateReportsCheckerOutput(t *testing.T) {
	p := tempPaths(t)
	rr := renderers.NewRecordingRunner().On(VtyshBin, func(c renderers.Command) (renderers.Output, error) {
		out := renderers.Output{Stdout: []byte("line 7: % Unknown command[4]: ip route 10.0.0.0/8 bogus\n"), ExitCode: 2}
		return out, &renderers.ExitError{Command: c, Output: out}
	})
	r := New(rr, WithPaths(p), WithSections())
	files, _ := r.Render(context.Background(), nil)
	err := r.Validate(context.Background(), files)
	if !errors.Is(err, ErrDaemon) || !strings.Contains(err.Error(), "line 7: % Unknown command") {
		t.Fatalf("err = %v", err)
	}
}

func TestFilesOwnership(t *testing.T) {
	p := tempPaths(t)
	r := New(renderers.NewRecordingRunner(), WithPaths(p), WithSections())
	files, _ := r.Render(context.Background(), nil)
	files["/etc/passwd"] = renderers.File{Mode: 0o644, Content: []byte("x")}
	if err := r.Validate(context.Background(), files); !errors.Is(err, renderers.ErrInvalidFiles) {
		t.Errorf("Validate accepted a foreign path: %v", err)
	}
	if err := r.Apply(context.Background(), files); !errors.Is(err, renderers.ErrInvalidFiles) {
		t.Errorf("Apply accepted a foreign path: %v", err)
	}
	if err := r.Apply(context.Background(), renderers.Files{}); !errors.Is(err, renderers.ErrInvalidFiles) {
		t.Errorf("Apply accepted an empty set: %v", err)
	}
}

const reloadTestOutput = `2026-09-24 00:23:13,695  INFO: Called via "Namespace(...)"

Lines To Delete
===============
no ip route 10.12.200.0/24 10.12.1.1

Lines To Add
============
ip route 10.12.200.0/24 10.12.1.9
`

func TestDryRunArgvAndDiff(t *testing.T) {
	p := tempPaths(t)
	rr := renderers.NewRecordingRunner().Succeed(ReloadBin, reloadTestOutput)
	r := New(rr, WithPaths(p), WithSections())
	files, _ := r.Render(context.Background(), nil)
	diff, err := r.DryRun(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	want := "Lines To Delete\n===============\nno ip route 10.12.200.0/24 10.12.1.1\nLines To Add\n============\nip route 10.12.200.0/24 10.12.1.9\n"
	if diff != want {
		t.Errorf("diff = %q, want %q", diff, want)
	}
	a := rr.Calls()[0].Args
	wantPrefix := []string{"--test", "--log-level", "info", "--logfile", p.ReloadLog, "--bindir", "/usr/bin",
		"--confdir", p.ConfDir, "--rundir", p.SocketDir(), "--vty_socket", p.RunDir, "--pathspace", "w12"}
	if !slices.Equal(a[:len(wantPrefix)], wantPrefix) || len(a) != len(wantPrefix)+1 || a[len(a)-1] == p.ConfFile() {
		t.Fatalf("argv = %q (the file must be the staged copy)", a)
	}
	if got := NormalizeDiff("Lines To Delete\n===============\n\nLines To Add\n============\n"); got != "" {
		t.Errorf("empty diff normalised to %q", got)
	}
}

func TestApplyWritesAtomicallyAndReloads(t *testing.T) {
	p := tempPaths(t)
	rr := renderers.NewRecordingRunner().Succeed(ReloadBin, "")
	r := New(rr, WithPaths(p), WithSections())
	files, _ := r.Render(context.Background(), staticDoc(t))
	if err := r.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	for path, f := range files {
		b, err := os.ReadFile(path) //nolint:gosec // test path
		if err != nil || string(b) != string(f.Content) {
			t.Fatalf("%s not written: %v", path, err)
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
			t.Errorf("%s mode %v", path, info.Mode().Perm())
		}
	}
	a := rr.Calls()[0].Args
	if a[0] != "--reload" || a[len(a)-1] != p.ConfFile() {
		t.Fatalf("argv = %q", a)
	}
	for _, c := range rr.Calls() {
		if strings.Contains(c.String(), "systemctl") || strings.Contains(c.String(), "restart") {
			t.Fatalf("Apply restarted something: %s", c)
		}
	}
}

func TestApplyRestoresOnReloadFailure(t *testing.T) {
	p := tempPaths(t)
	old := []byte("frr version 10.7.1\n!\nip route 10.0.0.0/8 blackhole\n!\nend\n")
	if err := os.WriteFile(p.ConfFile(), old, 0o640); err != nil { //nolint:gosec // same mode as the renderer writes
		t.Fatal(err)
	}
	reloads := 0
	var seen []string
	rr := renderers.NewRecordingRunner().On(ReloadBin, func(c renderers.Command) (renderers.Output, error) {
		reloads++
		b, _ := os.ReadFile(c.Args[len(c.Args)-1])
		seen = append(seen, string(b))
		if reloads == 1 {
			out := renderers.Output{Stderr: []byte("vtysh failed"), ExitCode: 1}
			return out, &renderers.ExitError{Command: c, Output: out}
		}
		return renderers.Output{}, nil
	})
	r := New(rr, WithPaths(p), WithSections())
	files, _ := r.Render(context.Background(), staticDoc(t))
	err := r.Apply(context.Background(), files)
	if !errors.Is(err, ErrDaemon) || !strings.Contains(err.Error(), "vtysh failed") {
		t.Fatalf("err = %v", err)
	}
	if reloads != 2 || seen[0] != string(files[p.ConfFile()].Content) || seen[1] != string(old) {
		t.Fatalf("want reload(new) then reload(restored old); got %d reloads", reloads)
	}
	if b, _ := os.ReadFile(p.ConfFile()); string(b) != string(old) {
		t.Error("frr.conf not restored")
	}
	if _, err := os.Stat(p.VtyshConf()); !os.IsNotExist(err) {
		t.Error("vtysh.conf did not exist before and was not removed by the restore")
	}
}

func TestApplyNeedsConfigDir(t *testing.T) {
	p := tempPaths(t)
	p.Namespace = "w99" // subdir not created
	r := New(renderers.NewRecordingRunner(), WithPaths(p), WithSections())
	files, _ := r.Render(context.Background(), nil)
	if err := r.Apply(context.Background(), files); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
}

// showRunner answers vtysh --command <cmd> from a table.
func showRunner(t *testing.T, answers map[ShowCommand]string) *renderers.RecordingRunner {
	t.Helper()
	return renderers.NewRecordingRunner().On(VtyshBin, func(c renderers.Command) (renderers.Output, error) {
		cmd := ShowCommand(c.Args[len(c.Args)-1])
		if c.Args[len(c.Args)-2] != "-c" {
			t.Errorf("not a -c call: %q", c.Args)
		}
		ans, ok := answers[cmd]
		if !ok {
			out := renderers.Output{Stdout: []byte("% Unknown command: " + string(cmd)), ExitCode: 1}
			return out, &renderers.ExitError{Command: c, Output: out}
		}
		return renderers.Output{Stdout: []byte(ans)}, nil
	})
}

const (
	ribV4 = `{"default":{"10.12.200.0/24":[{"prefix":"10.12.200.0/24","protocol":"static","vrfName":"default","distance":1,"selected":true,"installed":true,"nexthops":[{"ip":"10.12.1.1","interfaceName":"w12f0","active":true,"fib":true}]}],"10.12.1.0/24":[{"prefix":"10.12.1.0/24","protocol":"connected","vrfName":"default","selected":true,"installed":true,"nexthops":[{"interfaceName":"w12f0","active":true}]}]},"w12red":{"10.12.210.0/24":[{"prefix":"10.12.210.0/24","protocol":"static","vrfName":"w12red","tag":100,"distance":50,"nexthops":[{"blackhole":true,"unreachable":true,"active":true}]}]}}`
	ribV6 = `{"default":{}}`
	ifs   = `{"default":{"w12f0":{"operationalStatus":"up","description":"\"; rm -rf /","vrfName":"default"},"lo":{"operationalStatus":"up","vrfName":"default"}}}`
	rc    = "Building configuration...\n\nCurrent configuration:\n!\nfrr version 10.7.1\nfrr defaults traditional\nhostname vrx-a\n!\nip route 10.12.200.0/24 10.12.1.1\n!\nend\n"
	vrfs  = "vrf w12red id 5 table 12001\nvrf w12blue inactive (configured)\n"
	ver   = "FRRouting 10.7.1 (vrx-a) on Linux(7.0.0-31-generic).\nCopyright 1996-2005 Kunihiro Ishiguro, et al.\n"
)

func standardAnswers() map[ShowCommand]string {
	return map[ShowCommand]string{
		ShowVersion: ver, ShowRunningConfig: rc, ShowVRF: vrfs,
		ShowIPRouteAll: ribV4, ShowIPv6RouteAll: ribV6, ShowInterfaceAll: ifs,
		"show bgp summary json": `{"ipv4Unicast":{"routerId":"10.12.0.1","peers":{}}}`,
	}
}

func TestRetrieve(t *testing.T) {
	rr := showRunner(t, standardAnswers())
	r := New(rr, WithPaths(TestPaths("w12")), WithStateReaders(StateReader{Key: "bgpSummary", Command: "show bgp summary json"}))
	msg, err := r.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(msg)
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	if got["version"] != "10.7.1" {
		t.Errorf("version = %v", got["version"])
	}
	if rcLines, _ := got["runningConfig"].([]any); len(rcLines) != 3 || rcLines[2] != "ip route 10.12.200.0/24 10.12.1.1" {
		t.Errorf("runningConfig = %v", got["runningConfig"])
	}
	if v, _ := got["vrfs"].([]any); len(v) != 2 {
		t.Errorf("vrfs = %v", got["vrfs"])
	}
	for _, k := range []string{"ipv4Routes", "ipv6Routes", "interfaces", "bgpSummary"} {
		if _, ok := got[k]; !ok {
			t.Errorf("Retrieve has no %q", k)
		}
	}
	// Every vtysh call is `vtysh --config_dir … --vty_socket … -N w12 -c <constant>`.
	for _, c := range rr.Calls() {
		if c.Path != VtyshBin || c.Args[0] != "--config_dir" || c.Args[4] != "-N" || c.Args[5] != "w12" || c.Args[6] != "-c" {
			t.Errorf("unexpected call %q", c.Args)
		}
	}

	st, err := r.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	statics, err := st.StaticRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if len(statics) != 2 || statics[0].Prefix != "10.12.200.0/24" || statics[1].VRFName != "w12red" || !statics[1].Nexthops[0].Blackhole || statics[1].Tag != 100 {
		t.Fatalf("static routes = %+v", statics)
	}
	if st.VRFs[0].Name != "w12blue" || st.VRFs[0].Active || st.VRFs[1].ID != 5 || st.VRFs[1].Table != 12001 {
		t.Errorf("vrfs = %+v", st.VRFs)
	}
}

func TestShowCommandsAreConstants(t *testing.T) {
	r := New(showRunner(t, standardAnswers()), WithPaths(TestPaths("w12")))
	for _, bad := range []ShowCommand{"show ip route json\nconfigure terminal", "configure terminal", "show ip route; reboot", "show  x", ""} {
		if _, err := r.Show(context.Background(), bad); err == nil {
			t.Errorf("Show(%q) accepted", bad)
		}
	}
	if _, err := r.ShowJSON(context.Background(), ShowRunningConfig); err == nil {
		t.Error("ShowJSON accepted a non-json command")
	}
	rr := showRunner(t, map[ShowCommand]string{"show ip route json": "% Unknown command"})
	if _, err := New(rr, WithPaths(TestPaths("w12"))).ShowJSON(context.Background(), ShowIPRoute); !errors.Is(err, ErrDaemon) {
		t.Errorf("non-JSON output accepted: %v", err)
	}
	for name, sr := range map[string]StateReader{
		"newline":  {Key: "x", Command: "show x json\nwrite"},
		"not json": {Key: "x", Command: "show x"},
		"bad key":  {Key: "X y", Command: "show x json"},
		"builtin":  {Key: "version", Command: "show x json"},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: RegisterStateReader did not panic", name)
				}
			}()
			RegisterStateReader(sr)
		}()
	}
	RegisterStateReader(StateReader{Key: "bgpSummary", Command: "show bgp summary json"})
	t.Cleanup(func() { readersMu.Lock(); delete(readers, "bgpSummary"); readersMu.Unlock() })
	if got := RegisteredStateReaders(); len(got) != 1 || got[0].Key != "bgpSummary" {
		t.Errorf("RegisteredStateReaders = %v", got)
	}
}

func TestNormalizeConfigDropsNoise(t *testing.T) {
	got := NormalizeConfig(rc)
	want := []string{"frr defaults traditional", "hostname vrx-a", "ip route 10.12.200.0/24 10.12.1.1"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestPollerEvents(t *testing.T) {
	answers := standardAnswers()
	var fail bool
	show := func(_ context.Context, cmd ShowCommand) (json.RawMessage, error) {
		if fail {
			return nil, errors.New("vtysh down")
		}
		return json.RawMessage(answers[cmd]), nil
	}
	p := newPoller(show)
	if ev, err := p.Step(context.Background()); err != nil || len(ev) != 0 {
		t.Fatalf("baseline: %v %v", ev, err)
	}
	answers[ShowInterfaceAll] = strings.Replace(ifs, `"w12f0":{"operationalStatus":"up"`, `"w12f0":{"operationalStatus":"down"`, 1)
	answers[ShowIPRouteAll] = `{"default":{},"w12red":{}}`
	ev, err := p.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// event(poller, key, old, new)
	event := func(f ...string) Event { return Event{Poller: f[0], Key: f[1], Old: f[2], New: f[3]} }
	want := []Event{
		event("interfaces", "w12f0", "up", "down"),
		event("routes", "ipv4/default", "2", ""),
		event("routes", "ipv4/w12red", "1", ""),
	}
	if !slices.Equal(ev, want) {
		t.Fatalf("events = %+v\nwant %+v", ev, want)
	}
	pe := ev[0].ToProto()
	if pe.GetKind().String() != "EVENT_KIND_LINK_DOWN" || pe.GetInterface() != "w12f0" || pe.GetAttributes()["source"] != "frr" {
		t.Errorf("proto event = %v", pe)
	}
	if k := ev[1].ToProto().GetKind().String(); k != "EVENT_KIND_UNSPECIFIED" {
		t.Errorf("route event kind = %s", k)
	}
	fail = true
	if ev, err := p.Step(context.Background()); err == nil || len(ev) != 0 {
		t.Fatalf("failing poll: %v %v", ev, err)
	}
	fail = false
	if ev, err := p.Step(context.Background()); err != nil || len(ev) != 0 {
		t.Fatalf("baseline kept across a failed poll: %v %v", ev, err)
	}
}

func TestWatchStopsWithContext(t *testing.T) {
	answers := standardAnswers()
	p := newPoller(func(_ context.Context, cmd ShowCommand) (json.RawMessage, error) {
		return json.RawMessage(answers[cmd]), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Watch(ctx, 0, func(Event) {}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch = %v", err)
	}
}

func TestRegisterPoller(t *testing.T) {
	RegisterPoller("bgp-neighbors", func(context.Context, ShowFunc) (map[string]string, error) {
		return map[string]string{"10.12.1.1": "Established"}, nil
	})
	t.Cleanup(func() { pollersMu.Lock(); delete(pollers, "bgp-neighbors"); pollersMu.Unlock() })
	p := New(showRunner(t, standardAnswers()), WithPaths(TestPaths("w12"))).NewPoller()
	if _, ok := p.funcs["bgp-neighbors"]; !ok {
		t.Fatal("registered poller missing")
	}
	for _, name := range []string{"routes", "bgp-neighbors", "Bad"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterPoller(%q) did not panic", name)
				}
			}()
			RegisterPoller(name, func(context.Context, ShowFunc) (map[string]string, error) { return nil, nil })
		}()
	}
}
