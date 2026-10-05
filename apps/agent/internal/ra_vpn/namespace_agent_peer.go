package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/user"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// verifyFixedAgentPeer authenticates the fixed agent unit and its configured
// root:ngfw process identity. Actual executable attestation is a separate held
// manager-opened EXE descriptor requirement, never inferred from ExecStart.
func verifyFixedAgentPeer(ctx context.Context, peer *unix.Ucred, identity bootid.Identity) error {
	if peer == nil || peer.Uid != 0 || peer.Pid <= 1 || identity.PID != int(peer.Pid) || !identity.Complete() || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ErrBoundary
	}
	group, err := user.LookupGroup("ngfw")
	if err != nil {
		return ErrBoundary
	}
	gid, err := strconv.ParseUint(group.Gid, 10, 32)
	if err != nil || peer.Gid != uint32(gid) || validateNamespaceBrokerProcess(identity.PID, uint64(1<<unix.CAP_NET_ADMIN|1<<unix.CAP_SYS_ADMIN|1<<unix.CAP_IPC_LOCK)) != nil {
		return ErrBoundary
	}
	status, err := os.ReadFile("/proc/" + strconv.Itoa(identity.PID) + "/status")
	if err != nil || len(status) > 16384 {
		return ErrBoundary
	}
	matched := false
	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, "Gid:") {
			continue
		}
		values := strings.Fields(strings.TrimPrefix(line, "Gid:"))
		if len(values) != 4 {
			return ErrBoundary
		}
		for _, value := range values {
			if value != group.Gid {
				return ErrBoundary
			}
		}
		matched = true
	}
	if !matched {
		return ErrBoundary
	}
	const fragment = "/usr/lib/systemd/system/ngfw-agent.service"
	content, err := trustedInstallationFile(fragment, 16384, false)
	digest := sha256.Sum256(content)
	if err != nil || hex.EncodeToString(digest[:]) != "a793b8ec82260cc3fb9a0003894cf66634f9dccec2739a4f4ab6a3bad036379d" {
		return ErrBoundary
	}
	fields, err := namespaceSystemdProperties(ctx, "ngfw-agent.service", "MainPID,ControlGroup,FragmentPath,DropInPaths,User,Group,ExecStart")
	if err != nil || fields["MainPID"] != strconv.Itoa(identity.PID) || fields["ControlGroup"] != "/system.slice/ngfw-agent.service" || fields["FragmentPath"] != fragment || fields["User"] != "root" || fields["Group"] != "ngfw" || !strings.Contains(fields["ExecStart"], "path=/usr/sbin/ngfw-agent ; argv[]=/usr/sbin/ngfw-agent ;") {
		return ErrBoundary
	}
	if fields["DropInPaths"] != "" {
		const hardening = "/usr/lib/systemd/system/ngfw-agent.service.d/10-ngfw-hardening.conf"
		if fields["DropInPaths"] != hardening {
			return ErrBoundary
		}
		content, err := trustedInstallationFile(hardening, 16384, false)
		digest := sha256.Sum256(content)
		if err != nil || hex.EncodeToString(digest[:]) != "68d47c8d2d793dfbf7cc7314e597cb2e0b2455d7706b125108eac6becf6f4a2a" {
			return ErrBoundary
		}
	}
	if !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ErrBoundary
	}
	return nil
}
