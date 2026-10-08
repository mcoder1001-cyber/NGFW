package pppoe

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"ngfw/agent/internal/renderers"
)

func cmds(r *renderers.RecordingRunner) []string {
	var out []string
	for _, c := range r.Calls() {
		out = append(out, strings.Join(c.Args, " "))
	}
	return out
}

func TestApplySupervisor(t *testing.T) {
	base := t.TempDir()
	rr := renderers.NewRecordingRunner().Succeed(SystemctlBin, "")
	r := New(WithPaths(PathsUnder(base)))
	ctx := context.Background()

	s0 := Session{Iface: "eth0", HostIf: "wan0", Username: "u0", Password: "p0", MTU: 1492, DefaultRoute: true, IPv6: "off", HoldoffSec: 5}
	s1 := Session{Iface: "eth1", HostIf: "wan1", Username: "u1", Password: "p1", MTU: 1480, IPv6: "off", HoldoffSec: 5}

	// initial apply: files written, daemon-reload, both units restarted (sorted)
	if err := r.Apply(ctx, rr, []Session{s1, s0}); err != nil {
		t.Fatal(err)
	}
	if got := cmds(rr); strings.Join(got, "|") != "daemon-reload|restart ngfw-pppoe-wan0.service|restart ngfw-pppoe-wan1.service" {
		t.Fatalf("initial: %v", got)
	}
	if _, err := os.Stat(base + "/etc/ppp/peers/ngfw-wan0"); err != nil {
		t.Fatalf("peer file not written: %v", err)
	}

	// no-op apply: same sessions → nothing runs, the link is left dialed
	rr.Reset()
	if err := r.Apply(ctx, rr, []Session{s0, s1}); err != nil {
		t.Fatal(err)
	}
	if got := cmds(rr); len(got) != 0 {
		t.Fatalf("no-op apply ran: %v", got)
	}

	// change s0's MTU → only wan0 restarts (daemon-reload because the unit body is unchanged but the peer changed)
	rr.Reset()
	s0b := s0
	s0b.MTU = 1400
	if err := r.Apply(ctx, rr, []Session{s0b, s1}); err != nil {
		t.Fatal(err)
	}
	if got := cmds(rr); strings.Join(got, "|") != "stop ngfw-pppoe-wan0.service|daemon-reload|restart ngfw-pppoe-wan0.service" {
		t.Fatalf("mtu change: %v", got)
	}

	// remove s1 → its unit is stopped, its files removed, daemon-reload
	rr.Reset()
	if err := r.Apply(ctx, rr, []Session{s0b}); err != nil {
		t.Fatal(err)
	}
	got := cmds(rr)
	if got[0] != "stop ngfw-pppoe-wan1.service" || got[1] != "daemon-reload" {
		t.Fatalf("removal order: %v", got)
	}
	if _, err := os.Stat(base + "/etc/ppp/peers/ngfw-wan1"); !os.IsNotExist(err) {
		t.Fatalf("wan1 peer file not removed: %v", err)
	}
	if _, err := os.Stat(base + "/etc/systemd/system/ngfw-pppoe-wan1.service"); !os.IsNotExist(err) {
		t.Fatal("wan1 unit not removed")
	}

	// remove all → last unit stopped, secrets removed
	rr.Reset()
	if err := r.Apply(ctx, rr, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base + "/etc/ppp/chap-secrets"); !os.IsNotExist(err) {
		t.Fatal("chap-secrets not removed when no sessions remain")
	}
}

func TestApplyIdenticalRetryCompletesFailedTransition(t *testing.T) {
	for _, failAt := range []string{"daemon-reload", "restart"} {
		t.Run(failAt, func(t *testing.T) {
			r := New(WithPaths(PathsUnder(t.TempDir())))
			rr := renderers.NewRecordingRunner().Succeed(SystemctlBin, "")
			s := minimalSession()
			if err := r.Apply(t.Context(), rr, []Session{s}); err != nil {
				t.Fatal(err)
			}
			s.MTU = 1400
			rr.On(SystemctlBin, func(cmd renderers.Command) (renderers.Output, error) {
				if cmd.Args[0] == failAt {
					return renderers.Output{}, errors.New("controlled transition failure")
				}
				return renderers.Output{}, nil
			})
			if err := r.Apply(t.Context(), rr, []Session{s}); err == nil {
				t.Fatal("controlled failure was ignored")
			}
			rr.Reset()
			rr.Succeed(SystemctlBin, "")
			if err := r.Apply(t.Context(), rr, []Session{s}); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(cmds(rr), "|"); got != "stop "+unitName(s.HostIf)+"|daemon-reload|restart "+unitName(s.HostIf) {
				t.Fatalf("identical retry did not resume transition: %s", got)
			}
			for _, path := range []string{r.paths.StateDir + "/" + s.HostIf + ".ipv6.blocked", r.paths.StateDir + "/ipv6-transitions/" + s.HostIf} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("successful retry retained %s", path)
				}
			}
			rr.Reset()
			if err := r.Apply(t.Context(), rr, []Session{s}); err != nil || len(rr.Calls()) != 0 {
				t.Fatalf("completed transition was not idempotent: %v %v", err, rr.Calls())
			}
		})
	}
}

func TestApplyRemovalRetryReloadsDeletedUnit(t *testing.T) {
	r := New(WithPaths(PathsUnder(t.TempDir())))
	rr := renderers.NewRecordingRunner().Succeed(SystemctlBin, "")
	s := minimalSession()
	if err := r.Apply(t.Context(), rr, []Session{s}); err != nil {
		t.Fatal(err)
	}
	rr.On(SystemctlBin, func(cmd renderers.Command) (renderers.Output, error) {
		if cmd.Args[0] == "daemon-reload" {
			return renderers.Output{}, errors.New("controlled removal reload failure")
		}
		return renderers.Output{}, nil
	})
	if err := r.Apply(t.Context(), rr, nil); err == nil {
		t.Fatal("controlled reload failure ignored")
	}
	rr.Reset()
	rr.Succeed(SystemctlBin, "")
	if err := r.Apply(t.Context(), rr, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cmds(rr), "|"); got != "daemon-reload" {
		t.Fatalf("removed unit retry did not reload: %s", got)
	}
	if _, err := os.Stat(r.paths.StateDir + "/ipv6-transitions/" + s.HostIf); !os.IsNotExist(err) {
		t.Fatal("successful removal retained pending transition")
	}
	for _, suffix := range []string{".ipv6.blocked", ".ipv6.admission"} {
		if _, err := os.Stat(r.paths.StateDir + "/" + s.HostIf + suffix); !os.IsNotExist(err) {
			t.Fatalf("successful removal retained tombstone %s", suffix)
		}
	}
	rr.Reset()
	if err := r.Apply(t.Context(), rr, nil); err != nil || len(rr.Calls()) != 0 {
		t.Fatalf("completed removal was not idempotent: %v %v", err, rr.Calls())
	}
}
