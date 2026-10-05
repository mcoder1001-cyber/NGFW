package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

const unitObserverExecutable = "/usr/lib/ngfw/ngfw-ra-namespace-broker"
const unitObserverService = "/usr/lib/systemd/system/ngfw-ra-observer@.service"
const unitObserverSocket = "/usr/lib/systemd/system/ngfw-ra-observer@.socket"

// SystemdUnitObservation obtains fresh manager-opened namespace and executable
// descriptors for the canonical private unit without reading target proc NS/exe.
type SystemdUnitObservation struct{}

// NewSystemdUnitObservation constructs the fixed production observation adapter.
func NewSystemdUnitObservation() UnitObservationProvider { return &SystemdUnitObservation{} }

func unitObserverInstallation() error {
	if validateNamespaceBrokerExecutable(unitObserverExecutable) != nil || numericPublisherInstallation() != nil {
		return ErrEngine
	}
	for _, item := range []struct{ path, digest string }{
		{unitObserverService, "0864b67dafc24b2ff3fb1d68ff3bb06cc3cd84c977db59b151daa3bff2455a6a"},
		{unitObserverSocket, "557d3c9283202083a7dcde69677d25928a6bbea3268d0dd702222992417ff7a8"},
		{"/usr/lib/systemd/system/ngfw-ra@.service", "bf5e89b553235d455db222039d5e8b2cdbe639a1d36054b5dab6cca3ea266d9a"},
	} {
		content, err := trustedInstallationFile(item.path, 16384, false)
		digest := sha256.Sum256(content)
		if err != nil || hex.EncodeToString(digest[:]) != item.digest {
			return ErrEngine
		}
	}
	return nil
}

// Preflight verifies installation and the source identity without starting units.
func (*SystemdUnitObservation) Preflight(ctx context.Context) error {
	pid := os.Getpid()
	gid := os.Getegid()
	if pid <= 1 || pid > math.MaxInt32 || os.Geteuid() != 0 || gid < 0 || gid > math.MaxUint32 {
		return ErrEngine
	}
	source := (bootid.Reader{}).ForPID(pid)
	if unitObserverInstallation() != nil || readSourceAgentReference(source) != nil ||
		verifyFixedAgentPeer(ctx, &unix.Ucred{Pid: int32(pid), Uid: 0, Gid: uint32(gid)}, source) != nil {
		return ErrEngine
	}
	return nil
}

// PrepareObservation activates only observation for the fixed instance, after launch.
func (p *SystemdUnitObservation) PrepareObservation(ctx context.Context, instance string) error {
	if !validUnitName("ngfw-ra@"+instance+".service") || p.Preflight(ctx) != nil {
		return ErrEngine
	}
	bounded, cancel := context.WithTimeout(ctx, NumericOpenFilePublicationBudget)
	defer cancel()
	target, _, err := unitObserverTarget(bounded, instance)
	if err != nil {
		return ErrEngine
	}
	// The canonical agent is the authenticated peer; a child helper cannot
	// substitute for its actual manager-held executable and full boot identity.
	if NewSystemdNumericOpenFilePublisher().PublishNumericOpenFile(bounded, NumericOpenFileObserver, instance, target) != nil {
		return ErrEngine
	}
	return nil
}

func unitObserverTarget(ctx context.Context, instance string) (bootid.Identity, string, error) {
	if !validUnitName("ngfw-ra@" + instance + ".service") {
		return bootid.Identity{}, "", ErrEngine
	}
	name := "ngfw-ra@" + instance + ".service"
	fields, err := namespaceSystemdProperties(ctx, name, "MainPID,ControlGroup,FragmentPath,DropInPaths,User,Group,ExecStart,NetworkNamespacePath")
	pid, parseErr := strconv.Atoi(fields["MainPID"])
	if err != nil || parseErr != nil || pid <= 1 || strconv.Itoa(pid) != fields["MainPID"] ||
		fields["FragmentPath"] != "/usr/lib/systemd/system/ngfw-ra@.service" || fields["DropInPaths"] != "" || fields["User"] != "root" || fields["Group"] != "root" ||
		!unitObserverControlGroup(instance, fields["ControlGroup"]) || fields["NetworkNamespacePath"] != "/run/ngfw/ra/"+instance+"/netns" ||
		!strings.Contains(fields["ExecStart"], "path=/usr/lib/ngfw/ngfw-ra-daemon ; argv[]=/usr/lib/ngfw/ngfw-ra-daemon "+instance+" ;") {
		return bootid.Identity{}, "", ErrEngine
	}
	identity := (bootid.Reader{}).ForPID(pid)
	if !identity.Complete() || identity.PID != pid {
		return bootid.Identity{}, "", ErrEngine
	}
	return identity, fields["ControlGroup"], nil
}

func unitObserverManager(ctx context.Context, instance string, target, server bootid.Identity) error {
	if readObserverOpenFile(instance, target) != nil {
		return ErrEngine
	}
	numeric := strconv.Itoa(target.PID)
	dropIn, err := ObserverOpenFilePath(target.PID)
	if err != nil {
		return ErrEngine
	}
	fields, err := namespaceSystemdProperties(ctx, "ngfw-ra-observer@"+numeric+".socket", "FragmentPath,DropInPaths,ActiveState,SubState,Listen")
	if err != nil || fields["FragmentPath"] != unitObserverSocket || fields["DropInPaths"] != "" || fields["ActiveState"] != "active" || fields["SubState"] != "listening" || fields["Listen"] != "/run/ngfw/ra/observers/"+numeric+".sock (SequentialPacket)" {
		return ErrEngine
	}
	fields, err = namespaceSystemdProperties(ctx, "ngfw-ra-observer@"+numeric+".service", "MainPID,FragmentPath,DropInPaths,User,Group,CapabilityBoundingSet,NoNewPrivileges,ExecStart")
	if err != nil || fields["FragmentPath"] != unitObserverService || fields["DropInPaths"] != dropIn || fields["User"] != "root" || fields["Group"] != "ngfw" || fields["CapabilityBoundingSet"] != "" || fields["NoNewPrivileges"] != "yes" ||
		!strings.Contains(fields["ExecStart"], "path="+unitObserverExecutable+" ; argv[]="+unitObserverExecutable+" --observe-unit "+numeric+" ;") {
		return ErrEngine
	}
	if server.Complete() {
		if fields["MainPID"] != strconv.Itoa(server.PID) || !(bootid.Reader{}).ForPID(server.PID).Equal(server) || validateNamespaceBrokerProcess(server.PID, 0) != nil {
			return ErrEngine
		}
		image, err := os.Open("/proc/" + strconv.Itoa(server.PID) + "/exe")
		if err != nil {
			return ErrEngine
		}
		valid := sameUnitExecutable(image, unitObserverExecutable)
		closeErr := image.Close()
		if !valid || closeErr != nil {
			return ErrEngine
		}
	}
	return readObserverOpenFile(instance, target)
}
