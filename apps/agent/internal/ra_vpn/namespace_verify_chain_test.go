package ravpn

import (
	"context"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// These are owned real MNT/image shape controls for the read-only chain. They
// do not assert live VPP/unit authority or substitute for canonical acceptance.
func TestVerifyHeldTargetChainRejectsIntermediateChanges(t *testing.T) {
	var caps [2]unix.CapUserData
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	if unix.Capget(&header, &caps[0]) != nil || caps[0].Effective&(1<<unix.CAP_SYS_ADMIN) == 0 {
		t.Skip("owned child mount namespace requires CAP_SYS_ADMIN")
	}
	ctx, stop := context.WithTimeout(context.Background(), 4*time.Second)
	defer stop()
	// #nosec G204 -- fixed disposable owned child; no user input or host namespace changes.
	child := exec.CommandContext(ctx, "/usr/bin/unshare", "--mount", "/usr/bin/sleep", "20")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	id := (bootid.Reader{}).ForPID(child.Process.Pid)
	path := "/proc/" + strconv.Itoa(child.Process.Pid) + "/ns/mnt"
	var ns unix.Stat_t
	var own unix.Stat_t
	if unix.Stat("/proc/self/ns/mnt", &own) != nil {
		t.Fatal("own mount role")
	}
	for ctx.Err() == nil {
		if unix.Stat(path, &ns) == nil && ns.Ino != own.Ino {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if ctx.Err() != nil || !id.Complete() {
		t.Fatal("owned child did not obtain independent MNT")
	}
	fresh := func() *NamespaceTargetSnapshot {
		s := targetShapeFixture(t)
		if err := s.Files[0].Close(); err != nil {
			t.Fatal(err)
		}
		// #nosec G304 -- fixed read-only NS leaf for the PID of our own bounded child.
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		s.Files[0] = f
		s.Targets[0] = MountTarget{Boot: id, MountInode: ns.Ino}
		if err = s.Validate(); err != nil {
			t.Fatal(err)
		}
		return s
	}
	for _, scenario := range []string{"unchanged", "target", "source", "image", "closed", "cancelled", "second-error", "reused"} {
		t.Run(scenario, func(t *testing.T) {
			before := fresh()
			want := append([]MountTarget(nil), before.Targets[:]...)
			calls := 0
			produced := []*NamespaceTargetSnapshot{before}
			callCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			err := verifyHeldTargetChain(callCtx, want, before, func(_ context.Context, target MountTarget, current *NamespaceTargetSnapshot) (*NamespaceTargetSnapshot, error) {
				if target != want[calls] {
					t.Fatal("fixed target order")
				}
				calls++
				if scenario == "reused" {
					return current, nil
				}
				next := fresh()
				produced = append(produced, next)
				if calls == 1 {
					switch scenario {
					case "target":
						next.Targets[0].Boot.StartTime++
					case "source":
						next.Source.StartTime++
					case "image":
						if e := next.Executables[0].Close(); e != nil {
							t.Fatal(e)
						}
						f, e := os.Open("/usr/bin/false")
						if e != nil {
							t.Fatal(e)
						}
						next.Executables[0] = f
					case "closed":
						_ = next.Close()
					case "cancelled":
						cancel()
					}
				}
				if scenario == "second-error" && calls == 2 {
					return next, ErrBoundary
				}
				return next, nil
			})
			if (err == nil) != (scenario == "unchanged") {
				t.Fatalf("chain validity after %s: %v", scenario, err)
			}
			expectedCalls := 1
			if scenario == "unchanged" || scenario == "second-error" {
				expectedCalls = 2
			}
			if calls != expectedCalls {
				t.Fatalf("dispatched %d targets after %s", calls, scenario)
			}
			for _, s := range produced {
				for _, f := range append(s.Files[:], s.Executables[:]...) {
					if f != nil {
						t.Fatal("capture descriptor leaked")
					}
				}
			}
		})
	}
}
