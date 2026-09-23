package renderers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// binary returns the first existing path or skips the test.
func binary(t *testing.T, candidates ...string) string {
	t.Helper()
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skipf("none of %v present", candidates)
	return ""
}

func TestAllowlist(t *testing.T) {
	a := NewAllowlist("/usr/bin/vtysh", "/usr/bin/kea-dhcp4")
	if !a.Allowed("/usr/bin/vtysh") || a.Allowed("/usr/bin/sh") || a.Allowed("vtysh") {
		t.Fatal("Allowed is wrong")
	}
	if p := a.Paths(); len(p) != 2 || p[0] != "/usr/bin/kea-dhcp4" {
		t.Fatalf("Paths = %v", p)
	}
	for _, bad := range []string{"vtysh", "/usr/bin/../bin/vtysh", "/usr/bin/"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewAllowlist(%q) did not panic", bad)
				}
			}()
			NewAllowlist(bad)
		}()
	}
}

func TestSystemRunnerRefusesUnlistedAndMalformed(t *testing.T) {
	echo := binary(t, "/bin/echo", "/usr/bin/echo")
	r := NewSystemRunner(NewAllowlist("/usr/bin/vtysh"))
	ctx := context.Background()
	if _, err := r.Run(ctx, Command{Path: echo, Args: []string{"hi"}}); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("unlisted binary: got %v, want ErrNotAllowed", err)
	}
	for _, cmd := range []Command{
		{Path: "echo"},
		{Path: ""},
		{Path: "/bin/../bin/echo"},
		{Path: echo, Args: []string{"a\x00b"}},
	} {
		if _, err := r.Run(ctx, cmd); !errors.Is(err, ErrBadCommand) {
			t.Errorf("%+v: got %v, want ErrBadCommand", cmd, err)
		}
	}
}

func TestSystemRunnerFixedArgvNoShell(t *testing.T) {
	echo := binary(t, "/bin/echo", "/usr/bin/echo")
	r := NewSystemRunner(NewAllowlist(echo))
	// Shell metacharacters must reach the program as plain argv, not be interpreted.
	hostile := `"; rm -rf / #`
	out, err := r.Run(context.Background(), Command{Path: echo, Args: []string{hostile, "$HOME", "a b"}})
	if err != nil {
		t.Fatal(err)
	}
	want := hostile + " $HOME a b\n"
	if string(out.Stdout) != want || out.ExitCode != 0 {
		t.Fatalf("stdout = %q (exit %d), want %q", out.Stdout, out.ExitCode, want)
	}
}

func TestSystemRunnerStdinEnvExitAndTimeout(t *testing.T) {
	cat := binary(t, "/bin/cat", "/usr/bin/cat")
	falseBin := binary(t, "/bin/false", "/usr/bin/false")
	env := binary(t, "/usr/bin/env", "/bin/env")
	sleep := binary(t, "/bin/sleep", "/usr/bin/sleep")
	r := NewSystemRunner(NewAllowlist(cat, falseBin, env, sleep))
	ctx := context.Background()

	out, err := r.Run(ctx, Command{Path: cat, Stdin: []byte("from stdin")})
	if err != nil || string(out.Stdout) != "from stdin" {
		t.Fatalf("stdin: %q, %v", out.Stdout, err)
	}

	t.Setenv("VRX_LEAK_CHECK", "must-not-leak")
	out, err = r.Run(ctx, Command{Path: env})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out.Stdout), "VRX_LEAK_CHECK") || !strings.Contains(string(out.Stdout), "LC_ALL=C") {
		t.Fatalf("child environment must be the fixed default, got %q", out.Stdout)
	}

	_, err = r.Run(ctx, Command{Path: falseBin})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Output.ExitCode != 1 || exitErr.Command.Path != falseBin {
		t.Fatalf("non-zero exit: got %v", err)
	}
	if !strings.Contains(exitErr.Error(), "exited 1") {
		t.Fatalf("ExitError message = %q", exitErr.Error())
	}

	start := time.Now()
	_, err = r.Run(ctx, Command{Path: sleep, Args: []string{"5"}, Timeout: 200 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timed-out process was not killed promptly")
	}

	small := &SystemRunner{Allow: NewAllowlist(cat), MaxOutput: 4}
	out, err = small.Run(ctx, Command{Path: cat, Stdin: []byte("0123456789")})
	if err != nil || string(out.Stdout) != "0123" {
		t.Fatalf("MaxOutput: %q, %v", out.Stdout, err)
	}
}

func TestRecordingRunner(t *testing.T) {
	r := NewRecordingRunner()
	r.Succeed("/usr/bin/vtysh", "ok\n")
	r.FailWith("/usr/lib/frr/frr-reload.py", 2, "syntax error")
	ctx := context.Background()
	out, err := r.Run(ctx, Command{Path: "/usr/bin/vtysh", Args: []string{"-c", "show version"}})
	if err != nil || string(out.Stdout) != "ok\n" {
		t.Fatalf("Succeed: %q, %v", out.Stdout, err)
	}
	_, err = r.Run(ctx, Command{Path: "/usr/lib/frr/frr-reload.py", Args: []string{"--reload", "/tmp/x"}})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Output.ExitCode != 2 {
		t.Fatalf("FailWith: got %v", err)
	}
	if _, err := r.Run(ctx, Command{Path: "/bin/sh", Args: []string{"-c", "x"}}); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("unregistered path: got %v", err)
	}
	if _, err := r.Run(ctx, Command{Path: "vtysh"}); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("relative path: got %v", err)
	}
	calls := r.Calls()
	if len(calls) != 3 || calls[0].Args[1] != "show version" || calls[0].String() != "/usr/bin/vtysh -c show version" {
		t.Fatalf("Calls = %+v", calls)
	}
	r.Reset()
	if len(r.Calls()) != 0 {
		t.Fatal("Reset")
	}
}

// binaryLiteral matches string literals naming a binary under the usual system directories.
var binaryLiteral = regexp.MustCompile(`"((?:/usr)?/(?:s?bin|lib|libexec|local/s?bin)/[A-Za-z0-9_./+-]+)"`)

// TestAllowlistDocumented enforces the ALLOWLIST.md contract for every renderer package:
// every binary path literal in non-test Go source under internal/renderers must be listed
// in ALLOWLIST.md, no source outside helpers_exec.go calls exec.Command, and nobody spawns
// a shell.
func TestAllowlistDocumented(t *testing.T) {
	allow, err := os.ReadFile("ALLOWLIST.md")
	if err != nil {
		t.Fatalf("ALLOWLIST.md must live next to renderer.go: %v", err)
	}
	var missing, violations []string
	err = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || d.Name() == "templates") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(src)
		for _, m := range binaryLiteral.FindAllStringSubmatch(text, -1) {
			if !strings.Contains(string(allow), m[1]) {
				missing = append(missing, path+": "+m[1])
			}
		}
		if filepath.Base(path) != "helpers_exec.go" && strings.Contains(text, "exec.Command") {
			violations = append(violations, path+": calls exec.Command directly; use renderers.Runner")
		}
		for _, shell := range []string{`"sh", "-c"`, `"bash", "-c"`, `"sh -c`, `"bash -c`, `/bin/sh"`, `/bin/bash"`} {
			if strings.Contains(text, shell) {
				violations = append(violations, path+": spawns a shell ("+shell+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range missing {
		t.Errorf("binary not listed in ALLOWLIST.md: %s", m)
	}
	for _, v := range violations {
		t.Errorf("forbidden: %s", v)
	}
}
