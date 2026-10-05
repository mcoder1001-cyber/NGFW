package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"strconv"
	"strings"
	"time"
)

const numericPublisherService = "/usr/lib/systemd/system/ngfw-ra-openfile.service"
const numericPublisherSocket = "/usr/lib/systemd/system/ngfw-ra-openfile.socket"

func numericPublisherInstallation() error {
	if validateNamespaceBrokerExecutable(unitObserverExecutable) != nil {
		return numericPublisherFailure(context.Background(), 4)
	}
	for _, item := range []struct{ path, digest string }{
		{numericPublisherService, "8ad98855375d4485ed58e47af83fd28b66256089019505784885294f85bab590"},
		{numericPublisherSocket, "a1d59fdd0cef428f63f6fc2504fb21ee6a542037f9f4eee445d4986a1c77ad8f"},
	} {
		content, err := trustedInstallationFile(item.path, 16384, false)
		sum := sha256.Sum256(content)
		if err != nil || hex.EncodeToString(sum[:]) != item.digest {
			return numericPublisherFailure(context.Background(), 5)
		}
	}
	return nil
}

func numericPublisherManager(ctx context.Context, server bootid.Identity) error {
	if err := numericPublisherInstallation(); err != nil {
		return err
	}
	fields, err := namespaceSystemdProperties(ctx, "ngfw-ra-openfile.socket", "FragmentPath,DropInPaths,ActiveState,SubState,Listen")
	if err != nil || fields["FragmentPath"] != numericPublisherSocket || fields["DropInPaths"] != "" || fields["ActiveState"] != "active" || fields["SubState"] != "listening" || fields["Listen"] != numericPublisherSocketPath+" (SequentialPacket)" {
		return numericPublisherFailure(ctx, 6)
	}
	fields, err = namespaceSystemdProperties(ctx, "ngfw-ra-openfile.service", "MainPID,ControlPID,ActiveState,SubState,ControlGroup,FragmentPath,DropInPaths,User,Group,CapabilityBoundingSet,NoNewPrivileges,ExecStart")
	if err != nil || fields["FragmentPath"] != numericPublisherService || fields["DropInPaths"] != "" || !numericPublisherCgroup(fields, server.Complete()) || fields["User"] != "root" || fields["Group"] != "ngfw" || fields["CapabilityBoundingSet"] != "" || fields["NoNewPrivileges"] != "yes" || !strings.Contains(fields["ExecStart"], "path="+unitObserverExecutable+" ; argv[]="+unitObserverExecutable+" --publish-openfile ;") {
		return numericPublisherFailure(ctx, 7)
	}
	if server.Complete() {
		if fields["MainPID"] != strconv.Itoa(server.PID) || !(bootid.Reader{}).ForPID(server.PID).Equal(server) || validateNamespaceBrokerProcess(server.PID, 0) != nil {
			return numericPublisherFailure(ctx, 8)
		}
		cgroup, readErr := os.ReadFile("/proc/" + strconv.Itoa(server.PID) + "/cgroup")
		if readErr != nil || len(cgroup) > 16384 || strings.TrimSpace(string(cgroup)) != "0::/system.slice/ngfw-ra-openfile.service" {
			return numericPublisherFailure(ctx, 9)
		}
		image, err := os.Open("/proc/" + strconv.Itoa(server.PID) + "/exe")
		if err != nil {
			return numericPublisherFailure(ctx, 10)
		}
		valid := sameUnitExecutable(image, unitObserverExecutable)
		closeErr := image.Close()
		if !valid || closeErr != nil || !(bootid.Reader{}).ForPID(server.PID).Equal(server) {
			return numericPublisherFailure(ctx, 10)
		}
	}
	return nil
}

// A never-started socket-activated unit has no allocated cgroup. Empty is
// accepted only with positive inactive/dead and zero main/control process proof.
func numericPublisherCgroup(fields map[string]string, activeIdentity bool) bool {
	if fields["ControlGroup"] == "/system.slice/ngfw-ra-openfile.service" {
		return true
	}
	return !activeIdentity && fields["ControlGroup"] == "" && fields["MainPID"] == "0" && fields["ControlPID"] == "0" && fields["ActiveState"] == "inactive" && fields["SubState"] == "dead"
}

func waitNumericPublisherExit(ctx context.Context, server bootid.Identity) error {
	wait, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !(bootid.Reader{}).ForPID(server.PID).Equal(server) {
			fields, err := namespaceSystemdProperties(wait, "ngfw-ra-openfile.service", "MainPID,ControlPID,ActiveState,SubState")
			if err == nil && fields["MainPID"] == "0" && fields["ControlPID"] == "0" && fields["ActiveState"] == "inactive" && fields["SubState"] == "dead" {
				return nil
			}
		}
		select {
		case <-wait.Done():
			return numericPublisherFailure(wait, 23)
		case <-ticker.C:
		}
	}
}

func sameNumericPublisherSource(first, second *os.File) bool {
	if first == nil || second == nil {
		return false
	}
	var a, b unix.Stat_t
	return unix.Fstat(int(first.Fd()), &a) == nil && unix.Fstat(int(second.Fd()), &b) == nil && a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Nlink == b.Nlink
}
