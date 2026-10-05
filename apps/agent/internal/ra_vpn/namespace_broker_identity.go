package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"strconv"
	"strings"
)

func canonicalBrokerUnit(cgroup []byte) (string, error) {
	if len(cgroup) == 0 || len(cgroup) > 4096 {
		return "", ErrBoundary
	}
	value := strings.TrimSpace(string(cgroup))
	const direct = "0::/system.slice/"
	const nested = "0::/system.slice/system-ngfw\\x2dra\\x2dnamespace\\x2dbroker.slice/"
	name := ""
	if strings.HasPrefix(value, nested) {
		name = strings.TrimPrefix(value, nested)
	} else if strings.HasPrefix(value, direct) {
		name = strings.TrimPrefix(value, direct)
	} else {
		return "", ErrBoundary
	}
	if len(name) > 256 || !strings.HasPrefix(name, "ngfw-ra-namespace-broker@") || !strings.HasSuffix(name, ".service") {
		return "", ErrBoundary
	}
	instance := strings.TrimSuffix(strings.TrimPrefix(name, "ngfw-ra-namespace-broker@"), ".service")
	if instance == "" {
		return "", ErrBoundary
	}
	for _, char := range instance {
		if char < '0' || char > '9' {
			if !(char == '-' || char == '_' || char == ':' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z') {
				return "", ErrBoundary
			}
		}
	}
	return name, nil
}

func normalizeCanonicalBroker(ctx context.Context) error {
	identity := (bootid.Reader{}).ForPID(os.Getpid())
	if !identity.Complete() || os.Geteuid() != 0 || identity.PID <= 1 {
		return ErrBoundary
	}
	cgroup, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ErrBoundary
	}
	name, err := canonicalBrokerUnit(cgroup)
	if err != nil {
		return ErrBoundary
	}
	fields, err := namespaceSystemdProperties(ctx, name, "MainPID,ControlGroup,FragmentPath,DropInPaths,User,Group,ExecStart")
	if err != nil || fields["MainPID"] != strconv.Itoa(identity.PID) || !canonicalBrokerControlGroup(name, fields["ControlGroup"]) || strings.TrimSpace(string(cgroup)) != "0::"+fields["ControlGroup"] || fields["FragmentPath"] != namespaceBrokerUnit || fields["DropInPaths"] != "" || fields["User"] != "root" || fields["Group"] != "ngfw" || !strings.Contains(fields["ExecStart"], "path="+unitObserverExecutable+" ; argv[]="+unitObserverExecutable+" ;") {
		return ErrBoundary
	}
	unit, unitErr := trustedInstallationFile(namespaceBrokerUnit, 16384, false)
	digest := sha256.Sum256(unit)
	if unitErr != nil || hex.EncodeToString(digest[:]) != expectedNamespaceBrokerUnit {
		return ErrBoundary
	}
	if validateNamespaceBrokerExecutable(unitObserverExecutable) != nil {
		return ErrBoundary
	}
	running, err := os.Open("/proc/self/exe")
	if err != nil {
		return ErrBoundary
	}
	valid := sameUnitExecutable(running, unitObserverExecutable)
	closeErr := running.Close()
	if !valid || closeErr != nil || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ErrBoundary
	}
	if normalizeOwnCapabilities(canonicalBrokerCapabilities) != nil || validateNamespaceBrokerProcess(identity.PID, canonicalBrokerCapabilities) != nil {
		return ErrBoundary
	}
	if !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ErrBoundary
	}
	return nil
}

func canonicalBrokerControlGroup(name, group string) bool {
	return group == "/system.slice/"+name || group == "/system.slice/system-ngfw\\x2dra\\x2dnamespace\\x2dbroker.slice/"+name
}
