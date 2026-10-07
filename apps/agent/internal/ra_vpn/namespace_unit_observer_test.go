package ravpn

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

func observerTestSocketPair(t *testing.T) [2]int {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fds[0]); _ = unix.Close(fds[1]) })
	if boundUnitObserverSocket(context.Background(), fds[1]) != nil {
		t.Fatal("fixture timeout")
	}
	return fds
}

func observerOpenDescriptorCount(t *testing.T) int {
	t.Helper()
	files, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func TestUnitObserverPacketRefusesMalformedPayloads(t *testing.T) {
	for _, data := range []string{"", "{", "{}{}", `{"unknown":true}`, strings.Repeat("x", unitObserverPacketLimit+1)} {
		var request unitObserverRequest
		if decodeUnitObserverPacket([]byte(data), &request) == nil {
			t.Fatal("malformed or unbounded payload accepted")
		}
	}
	want := unitObserverRequest{Instance: strings.Repeat("a", 64)}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got unitObserverRequest
	if decodeUnitObserverPacket(data, &got) != nil || got.Instance != want.Instance {
		t.Fatal("bounded structured payload refused")
	}
}

func TestUnitObserverReceivedRightsAreClosedOnEveryRefusal(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		rights      int
		wanted      int
		oversized   bool
		credentials bool
	}{
		{name: "unexpected-rights", rights: 2, wanted: 0},
		{name: "missing-rights", rights: 0, wanted: 2},
		{name: "extra-rights", rights: 3, wanted: 2},
		{name: "truncated-rights", rights: 20, wanted: 2},
		{name: "oversized-data", rights: 2, wanted: 2, oversized: true},
		{name: "credentials-before-rights", rights: 2, wanted: 2, credentials: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			pair := observerTestSocketPair(t)
			file, err := os.Open("/dev/null")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			if scenario.credentials && unix.SetsockoptInt(pair[1], unix.SOL_SOCKET, unix.SO_PASSCRED, 1) != nil {
				t.Fatal("fixture credentials")
			}
			before := observerOpenDescriptorCount(t)
			rights := make([]int, scenario.rights)
			for i := range rights {
				rights[i] = int(file.Fd())
			}
			var control []byte
			if len(rights) != 0 {
				control = unix.UnixRights(rights...)
			}
			data := []byte("OK")
			if scenario.oversized {
				data = []byte(strings.Repeat("x", unitObserverPacketLimit+1))
			}
			if err := unix.Sendmsg(pair[0], data, control, nil, 0); err != nil {
				t.Fatal(err)
			}
			_, files, err := receiveUnitObserverPacket(pair[1], scenario.wanted)
			defer closeUnitObserverFiles(files)
			if err == nil || len(files) != 0 {
				t.Fatal("unsafe packet accepted")
			}
			if after := observerOpenDescriptorCount(t); after != before {
				t.Fatalf("received descriptors leaked: before=%d after=%d", before, after)
			}
		})
	}
}

func TestUnitObserverValidRightsHaveExplicitConsumerOwnership(t *testing.T) {
	pair := observerTestSocketPair(t)
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	before := observerOpenDescriptorCount(t)
	if err := unix.Sendmsg(pair[0], []byte("{}"), unix.UnixRights(int(file.Fd()), int(file.Fd())), nil, 0); err != nil {
		t.Fatal(err)
	}
	data, files, err := receiveUnitObserverPacket(pair[1], 2)
	if err != nil || string(data) != "{}" || len(files) != 2 {
		closeUnitObserverFiles(files)
		t.Fatal("valid rights refused")
	}
	for _, file := range files {
		flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			closeUnitObserverFiles(files)
			t.Fatal("received descriptor can escape through exec")
		}
	}
	closeUnitObserverFiles(files)
	if after := observerOpenDescriptorCount(t); after != before {
		t.Fatal("consumer close did not release rights")
	}
}

func TestUnitObserverDeadlineAndCancelledContextAreBounded(t *testing.T) {
	pair := observerTestSocketPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if boundUnitObserverSocket(ctx, pair[1]) == nil {
		t.Fatal("cancelled request accepted")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if boundUnitObserverSocket(ctx, pair[1]) != nil {
		t.Fatal("finite request timeout refused")
	}
	timeout, err := unix.GetsockoptTimeval(pair[1], unix.SOL_SOCKET, unix.SO_RCVTIMEO)
	if err != nil || timeout.Sec != 0 || timeout.Usec <= 0 || timeout.Usec > 50_000 {
		t.Fatal("caller deadline ignored")
	}
}

func TestUnitObserverSnapshotRequiresTypedNamespacesAndMatchingIdentity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only ownership contract")
	}
	network, err := os.Open("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = network.Close() }()
	executable, err := os.Open("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executable.Close() }()
	var stat unix.Stat_t
	if unix.Fstat(int(network.Fd()), &stat) != nil {
		t.Fatal("fixture network identity")
	}
	boot := (bootid.Reader{}).ForPID(os.Getpid())
	instance := strings.Repeat("a", 64)
	response := unitObserverResponse{Instance: instance, Server: boot,
		Identity:     UnitIdentity{BootID: boot.BootID, PID: boot.PID, StartTicks: boot.StartTime, NamespaceInode: stat.Ino},
		ControlGroup: "/system.slice/ngfw-ra@" + instance + ".service"}
	files := []*os.File{network, executable}
	if _, err := unitObserverSnapshot(instance, response, files); err != nil {
		t.Fatal("typed snapshot shape refused; this fixture does not attest a production unit")
	}
	for _, alter := range []func(*unitObserverResponse){
		func(r *unitObserverResponse) { r.Instance = strings.Repeat("b", 64) },
		func(r *unitObserverResponse) { r.Identity.NamespaceInode++ },
		func(r *unitObserverResponse) { r.Identity.StartTicks = 0 },
		func(r *unitObserverResponse) { r.Identity.BootID = "other-boot" },
		func(r *unitObserverResponse) { r.ControlGroup = "/foreign/" + r.ControlGroup },
	} {
		bad := response
		alter(&bad)
		if _, err := unitObserverSnapshot(instance, bad, files); err == nil {
			t.Fatal("foreign or incomplete snapshot accepted")
		}
	}
	mount, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mount.Close() }()
	if unix.Fstat(int(mount.Fd()), &stat) != nil {
		t.Fatal("fixture mount identity")
	}
	response.Identity.NamespaceInode = stat.Ino
	if _, err := unitObserverSnapshot(instance, response, []*os.File{mount, executable}); err == nil {
		t.Fatal("MNT descriptor accepted as NET despite matching inode")
	}
}

// Actual child exec and namespace changes keep BootID/PID/starttime unchanged.
// These component fixtures prove why a second capture must compare held FDs;
// production manager OpenFile acceptance is a separate private-VM test.
func TestUnitObserverFreshCaptureRejectsSamePIDChanges(t *testing.T) {
	for _, scenario := range []string{"target-exec", "source-exec", "target-net"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "target-net" {
				var capabilities [2]unix.CapUserData
				header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
				if unix.Capget(&header, &capabilities[0]) != nil || capabilities[0].Effective&(1<<unix.CAP_SYS_ADMIN) == 0 {
					t.Skip("owned child NET namespace requires CAP_SYS_ADMIN")
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			script := "read trigger; exec /bin/sleep 20"
			if scenario == "target-net" {
				script = "read trigger; exec /usr/bin/unshare --net /bin/sleep 20"
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
			oldNet, err := os.Open("/proc/" + strconv.Itoa(child.PID) + "/ns/net")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = oldNet.Close() }()
			selfExe, err := os.Open("/proc/self/exe")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = selfExe.Close() }()
			var oldImage, oldNetwork unix.Stat_t
			if unix.Fstat(int(oldExe.Fd()), &oldImage) != nil || unix.Fstat(int(oldNet.Fd()), &oldNetwork) != nil {
				t.Fatal("fixture identity unavailable")
			}
			if _, err = input.Write([]byte("continue\n")); err != nil {
				t.Fatal(err)
			}
			var newExe, newNet *os.File
			for ctx.Err() == nil {
				// #nosec G304 -- fixed proc leaf beneath our owned child numeric PID.
				image, e := os.Open("/proc/" + strconv.Itoa(child.PID) + "/exe")
				// #nosec G304 -- fixed namespace leaf beneath our owned child numeric PID.
				network, n := os.Open("/proc/" + strconv.Itoa(child.PID) + "/ns/net")
				var currentImage, currentNetwork unix.Stat_t
				changed := e == nil && n == nil && unix.Fstat(int(image.Fd()), &currentImage) == nil && unix.Fstat(int(network.Fd()), &currentNetwork) == nil && currentImage.Ino != oldImage.Ino
				if scenario == "target-net" {
					changed = changed && currentNetwork.Ino != oldNetwork.Ino
				}
				if changed {
					newExe, newNet = image, network
					break
				}
				if image != nil {
					_ = image.Close()
				}
				if network != nil {
					_ = network.Close()
				}
				time.Sleep(time.Millisecond)
			}
			if newExe == nil || newNet == nil {
				t.Fatal("owned child did not change executable/namespace")
			}
			defer func() { _ = newExe.Close(); _ = newNet.Close() }()
			if !(bootid.Reader{}).ForPID(child.PID).Equal(child) {
				t.Fatal("fixture unexpectedly changed PID/starttime")
			}
			instance := strings.Repeat("a", 64)
			identity := UnitIdentity{BootID: child.BootID, PID: child.PID, StartTicks: child.StartTime, NamespaceInode: oldNetwork.Ino}
			first := &unitObserverCapture{Source: child, Server: bootid.Identity{BootID: child.BootID, PID: 101, StartTime: 1}, Snapshot: &UnitProcessSnapshot{Instance: instance, Identity: identity, ControlGroup: "/system.slice/ngfw-ra@" + instance + ".service", Network: oldNet, Executable: oldExe}, SourceExecutable: selfExe}
			second := &unitObserverCapture{Source: child, Server: bootid.Identity{BootID: child.BootID, PID: 102, StartTime: 2}, Snapshot: &UnitProcessSnapshot{Instance: instance, Identity: identity, ControlGroup: first.Snapshot.ControlGroup, Network: oldNet, Executable: oldExe}, SourceExecutable: selfExe}
			if unitObserverCapturesMatch(first, second) != nil {
				t.Fatal("unchanged two-capture identity refused")
			}
			switch scenario {
			case "target-exec":
				second.Snapshot.Executable = newExe
			case "source-exec":
				first.SourceExecutable = oldExe
				second.SourceExecutable = newExe
			case "target-net":
				second.Snapshot.Network = newNet
			}
			if unitObserverCapturesMatch(first, second) == nil {
				t.Fatal("same-PID changed held identity accepted")
			}
		})
	}
}
