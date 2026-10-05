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

// Actual owned child changes prove full PID identity alone does not pin held roles.
func TestNamespaceTargetsRejectActualSamePIDChanges(t *testing.T) {
	for _, scenario := range []string{"target-exec", "source-exec", "target-mount"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "target-mount" {
				var capabilities [2]unix.CapUserData
				header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
				if unix.Capget(&header, &capabilities[0]) != nil || capabilities[0].Effective&(1<<unix.CAP_SYS_ADMIN) == 0 {
					t.Skip("owned child mount namespace requires CAP_SYS_ADMIN")
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			script := "read trigger; exec /usr/bin/python3 -c 'import time; time.sleep(20)'"
			if scenario == "target-mount" {
				script = "read trigger; exec /usr/bin/unshare --mount /usr/bin/python3 -c 'import time; time.sleep(20)'"
			}
			// #nosec G204 -- fixed fixture programs and two literal scripts; no user input.
			command := exec.CommandContext(ctx, "/bin/sh", "-c", script)
			input, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if command.Start() != nil {
				t.Fatal("owned child fixture failed")
			}
			defer func() { _ = input.Close(); _ = command.Process.Kill(); _ = command.Wait() }()
			child := (bootid.Reader{}).ForPID(command.Process.Pid)
			if !child.Complete() {
				t.Fatal("child boot unavailable")
			}
			// #nosec G304 -- PID is returned by our own child process, never caller input.
			oldExe, err := os.Open("/proc/" + strconv.Itoa(child.PID) + "/exe")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = oldExe.Close() }()
			// #nosec G304 -- same owned child PID; read only, no foreign namespace adoption.
			oldMount, err := os.Open("/proc/" + strconv.Itoa(child.PID) + "/ns/mnt")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = oldMount.Close() }()
			var oldImage, oldNamespace unix.Stat_t
			if unix.Fstat(int(oldExe.Fd()), &oldImage) != nil || unix.Fstat(int(oldMount.Fd()), &oldNamespace) != nil {
				t.Fatal("fixture identity unavailable")
			}
			if _, err = input.Write([]byte("continue\n")); err != nil {
				t.Fatal(err)
			}
			var newExe, newMount *os.File
			for ctx.Err() == nil {
				// #nosec G304 -- fixed proc leaf beneath our owned child numeric PID.
				image, e := os.Open("/proc/" + strconv.Itoa(child.PID) + "/exe")
				// #nosec G304 -- fixed namespace leaf beneath our owned child numeric PID.
				namespace, n := os.Open("/proc/" + strconv.Itoa(child.PID) + "/ns/mnt")
				var currentImage, currentNamespace unix.Stat_t
				changed := e == nil && n == nil && unix.Fstat(int(image.Fd()), &currentImage) == nil && unix.Fstat(int(namespace.Fd()), &currentNamespace) == nil && currentImage.Ino != oldImage.Ino
				if scenario == "target-mount" {
					changed = changed && currentNamespace.Ino != oldNamespace.Ino
				}
				if changed {
					newExe, newMount = image, namespace
					break
				}
				if image != nil {
					_ = image.Close()
				}
				if namespace != nil {
					_ = namespace.Close()
				}
				time.Sleep(time.Millisecond)
			}
			if newExe == nil || newMount == nil {
				t.Fatal("owned child did not change executable/namespace")
			}
			defer func() { _ = newExe.Close(); _ = newMount.Close() }()
			if !(bootid.Reader{}).ForPID(child.PID).Equal(child) {
				t.Fatal("fixture unexpectedly changed PID/starttime")
			}
			before, after := targetShapeFixture(t), targetShapeFixture(t)
			defer func() {
				if scenario == "target-exec" {
					before.Executables[0], after.Executables[0] = nil, nil
				}
				if scenario == "source-exec" {
					before.Executables[1], after.Executables[1] = nil, nil
				}
				if scenario == "target-mount" {
					before.Files[0], after.Files[0] = nil, nil
				}
			}()
			before.Targets[0] = MountTarget{Boot: child, MountInode: oldNamespace.Ino}
			after.Targets[0] = before.Targets[0]
			// Borrowed role descriptors are closed by the fixture above, not the shape snapshots.
			switch scenario {
			case "target-exec":
				if err := before.Executables[0].Close(); err != nil {
					t.Fatal(err)
				}
				if err := after.Executables[0].Close(); err != nil {
					t.Fatal(err)
				}
				before.Executables[0], after.Executables[0] = oldExe, newExe
			case "source-exec":
				before.Source, after.Source = child, child
				if err := before.Executables[1].Close(); err != nil {
					t.Fatal(err)
				}
				if err := after.Executables[1].Close(); err != nil {
					t.Fatal(err)
				}
				before.Executables[1], after.Executables[1] = oldExe, newExe
			case "target-mount":
				if err := before.Files[0].Close(); err != nil {
					t.Fatal(err)
				}
				if err := after.Files[0].Close(); err != nil {
					t.Fatal(err)
				}
				before.Files[0], after.Files[0] = oldMount, newMount
				after.Targets[0].MountInode = func() uint64 {
					var stat unix.Stat_t
					if err := unix.Fstat(int(newMount.Fd()), &stat); err != nil {
						t.Fatal(err)
					}
					return stat.Ino
				}()
			}
			// The changed source/target image or mount remains a valid role shape.
			if scenario != "target-mount" {
				var stat unix.Stat_t
				if err := unix.Fstat(int(before.Files[0].Fd()), &stat); err != nil {
					t.Fatal(err)
				}
				before.Targets[0].MountInode, after.Targets[0].MountInode = stat.Ino, stat.Ino
			}
			if before.Validate() != nil || after.Validate() != nil {
				t.Fatal("changed held shape invalid for unrelated reason")
			}
			if before.SameTargets(after) {
				t.Fatal("actual same-PID changed held role accepted")
			}
			// Retain only separately owned shape roles for its registered cleanup.
			if scenario == "target-exec" {
				before.Executables[0], after.Executables[0] = nil, nil
			}
			if scenario == "source-exec" {
				before.Executables[1], after.Executables[1] = nil, nil
			}
			if scenario == "target-mount" {
				before.Files[0], after.Files[0] = nil, nil
			}

		})
	}
}
