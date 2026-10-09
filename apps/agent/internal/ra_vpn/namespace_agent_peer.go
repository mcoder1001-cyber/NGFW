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

// Pin exact reviewed product source fragments, independently of broker/daemon units.
const expectedAgentUnitSHA256 = "0578ae4713eaed060b6b2551633136456c400f8f4a1a980f7624305633bcc54f"
const expectedAgentHardeningSHA256 = "e2895894312a43ff2d60c4ba0e741c5b3276ec3be9b64a8b46e7c6fc65e8f7bc"

// verifyFixedAgentPeer authenticates the fixed agent unit and its configured
// root:ngfw process identity. Actual executable attestation is a separate held
// manager-opened EXE descriptor requirement, never inferred from ExecStart.
func verifyFixedAgentPeer(ctx context.Context, peer *unix.Ucred, identity bootid.Identity) error {
	return verifyFixedAgentPeerUsing(ctx, peer, identity, fixedAgentUnitProperties)
}

func fixedAgentUnitProperties(ctx context.Context) (map[string]string, error) {
	return readManagerDBusSingleRole(ctx, managerDBusSingleSource)
}

func verifyFixedAgentPeerUsing(ctx context.Context, peer *unix.Ucred, identity bootid.Identity, properties func(context.Context) (map[string]string, error)) error {
	if peer == nil || validateNamespaceBrokerProcess(int(peer.Pid), canonicalSourceCapabilities) != nil {
		return ErrBoundary
	}
	return verifyFixedAgentIdentityUsing(ctx, peer, identity, properties)
}

// verifyFixedAgentIdentity is used only before monotonic own-source capability
// normalization; external peer authorization always uses the strict wrapper.
func verifyFixedAgentIdentity(ctx context.Context, peer *unix.Ucred, identity bootid.Identity) error {
	return verifyFixedAgentIdentityUsing(ctx, peer, identity, fixedAgentUnitProperties)
}

func verifyFixedAgentIdentityUsing(ctx context.Context, peer *unix.Ucred, identity bootid.Identity, properties func(context.Context) (map[string]string, error)) error {
	if peer == nil || peer.Uid != 0 || peer.Pid <= 1 || identity.PID != int(peer.Pid) || !identity.Complete() || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ErrBoundary
	}
	group, err := user.LookupGroup("ngfw")
	if err != nil {
		return ErrBoundary
	}
	gid, err := strconv.ParseUint(group.Gid, 10, 32)
	if err != nil || peer.Gid != uint32(gid) {
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
	if err != nil || hex.EncodeToString(digest[:]) != expectedAgentUnitSHA256 {
		return ErrBoundary
	}
	fields, err := properties(ctx)
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
		if err != nil || hex.EncodeToString(digest[:]) != expectedAgentHardeningSHA256 {
			return ErrBoundary
		}
	}
	if !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ErrBoundary
	}
	return nil
}
